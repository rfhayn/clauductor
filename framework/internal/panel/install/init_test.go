package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testbin"
)

func initGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// newRepo makes a repository with one commit on main and the given files.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := filepath.Join(t.TempDir(), "acme-web")
	os.MkdirAll(root, 0o755)
	root = signals.ResolvePath(root)
	initGit(t, root, "init", "-q", "-b", "main")
	for name, body := range files {
		os.WriteFile(filepath.Join(root, name), []byte(body), 0o644)
	}
	initGit(t, root, "add", "-A")
	initGit(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

func TestInitConfigFromARepository(t *testing.T) {
	root := newRepo(t, map[string]string{
		"package.json":   `{"scripts":{"build":"tsc","test":"vitest","check":"tsc && vitest"}}`,
		"pnpm-lock.yaml": "",
		"Makefile":       ".PHONY: ci\nci:\n\tpnpm run check\nX := 1\n",
	})
	origin := filepath.Join(t.TempDir(), "origin.git")
	initGit(t, root, "init", "-q", "--bare", origin)
	initGit(t, root, "remote", "add", "origin", origin)
	initGit(t, root, "push", "-q", "origin", "main")
	initGit(t, root, "remote", "set-head", "origin", "main")
	for _, b := range []string{"feature/login", "feature/cart", "fix/typo"} {
		initGit(t, root, "branch", b)
	}
	initGit(t, root, "worktree", "add", "-q", filepath.Join(root, ".worktrees", "login"), "feature/login")

	res, err := InitConfig(context.Background(), signals.ExecRunner, filepath.Join(root, "sub-dir-is-fine")) // not yet created: git fails
	if err == nil {
		t.Fatal("init outside a repository succeeded")
	}
	res, err = InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != filepath.Join(root, ".clauductor", "panel.json") {
		t.Fatalf("path %s", res.Path)
	}
	c, err := config.LoadConfig(res.Path)
	if err != nil {
		t.Fatalf("the panel cannot read what init wrote: %v", err)
	}
	if c.Schema != config.SchemaURL || c.Version != config.LatestVersion || len(c.Notices) != 0 {
		t.Errorf("$schema %q version %d notices %q", c.Schema, c.Version, c.Notices)
	}
	if c.Name != "acme-web" || c.Base != "origin/main" || c.WorktreeDir != ".worktrees" {
		t.Errorf("name %q base %q worktree_dir %q", c.Name, c.Base, c.WorktreeDir)
	}
	// PANEL-48 D3: init writes the lane context policy, as attach does (its scenario's
	// test is TestAttachedLanesLaunchWithThePolicy).
	if l := c.Launch; l == nil || l.ContextWindow == nil || *l.ContextWindow != 200000 || l.ClaudeaiConnectors == nil || *l.ClaudeaiConnectors {
		t.Errorf("launch %+v, want context_window 200000 and claudeai_connectors false", l)
	}
	want := map[string]string{"main": "orchestrator", "feature/": "feature", "fix/": "fix"}
	if !reflect.DeepEqual(c.Lanes, want) {
		t.Errorf("lanes %v", c.Lanes)
	}
	// The best-named gate the project defines, from whichever file: `make ci`.
	if len(c.Queues) != 1 || !reflect.DeepEqual(c.Queues[0].Command, []string{"make", "ci"}) || c.Queues[0].Lock != "clauductor/gate.lock" {
		t.Fatalf("queues %+v", c.Queues)
	}
	// Nothing that runs by itself.
	if len(c.Cards) != 0 || len(c.Templates) != 0 {
		t.Errorf("init wrote cards or templates: %+v %+v", c.Cards, c.Templates)
	}
	notes := strings.Join(res.Notes, "\n")
	for _, s := range []string{"only when you press RUN", "`pnpm run check`", "`pnpm run test`", "launch        context_window 200000, claudeai_connectors false"} {
		if !strings.Contains(notes, s) {
			t.Errorf("the notes do not say %q:\n%s", s, notes)
		}
	}
	if strings.Contains(notes, "build") {
		t.Errorf("a build script is not a gate:\n%s", notes)
	}
}

func TestInitConfigRefusesToOverwrite(t *testing.T) {
	root := newRepo(t, nil)
	path := filepath.Join(root, ".clauductor", "panel.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("mine"), 0o644)
	if _, err := InitConfig(context.Background(), signals.ExecRunner, root); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Fatalf("overwrote the existing config: %q", b)
	}
	// A dangling symlink is a file too.
	os.Remove(path)
	os.Symlink(filepath.Join(root, "nowhere"), path)
	if _, err := InitConfig(context.Background(), signals.ExecRunner, root); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("wrote through a dangling symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nowhere")); err == nil {
		t.Fatal("created the symlink's target")
	}
}

// No remote, no prefixed branches, no gate: init guesses no command and says so.
func TestInitConfigBareRepository(t *testing.T) {
	root := newRepo(t, map[string]string{"package.json": `{"scripts":{"start":"node .","build":"tsc"}}`})
	res, err := InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if c.Base != "main" || c.WorktreeDir != config.DefaultWorktreeDir || len(c.Queues) != 0 {
		t.Errorf("base %q worktree_dir %q queues %+v", c.Base, c.WorktreeDir, c.Queues)
	}
	if c.Lanes["main"] != "orchestrator" || c.Lanes["feature/"] != "feature" {
		t.Errorf("lanes %v", c.Lanes)
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "no gate script found") {
		t.Errorf("notes %q", res.Notes)
	}
}

// PANEL-18: a repository without the operating model gets no card and no template, and
// one line saying where they come from.
func TestInitConfigHintsAtCardsAndTemplates(t *testing.T) {
	root := newRepo(t, nil)
	res, err := InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Cards) != 0 || len(c.Templates) != 0 {
		t.Fatalf("cards %+v templates %+v", c.Cards, c.Templates)
	}
	notes := strings.Join(res.Notes, "\n")
	if strings.Count(notes, "clauductor install") != 1 || !strings.Contains(notes, "add them by hand") {
		t.Fatalf("the hint is not there once:\n%s", notes)
	}
}

// OPS-111: init's starter templates keep the shipped config's Up next refresh and ops prompt.
// panel-suggest.sh reads origin/main, which a working-tree watch never sees move, and an ops lane
// named after a queued row builds that row; a starter that kept the old values would do neither.
func TestInitTemplatesMatchTheShippedConfig(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "template", ".clauductor", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := config.ParseConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	root := newRepo(t, nil)
	for _, name := range []string{".claude/owner-queue.sh", ".claude/roadmap-queue.sh", ".claude/panel-suggest.sh"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
		testbin.Write(t, filepath.Join(root, name), "#!/bin/sh\n")
	}
	res, err := InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.ParseConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]config.TemplateConfig{}
	for _, tp := range shipped.Templates {
		want[tp.ID] = tp
	}
	for _, tp := range got.Templates {
		w, ok := want[tp.ID]
		if !ok || tp.Suggest == nil || w.Suggest == nil {
			t.Errorf("template %s: shipped %v, suggest %+v", tp.ID, ok, tp.Suggest)
			continue
		}
		if tp.Suggest.Refresh != w.Suggest.Refresh {
			t.Errorf("template %s refreshes %q, the shipped config %q", tp.ID, tp.Suggest.Refresh, w.Suggest.Refresh)
		}
		if tp.ID == "ops" && tp.FirstPrompt != w.FirstPrompt {
			t.Errorf("the ops prompt differs from the shipped config's:\n%s\n%s", tp.FirstPrompt, w.FirstPrompt)
		}
	}
}

// PANEL-18: a repository with the operating model's files gets its pinned cards, its
// four lane templates (with Up next when its suggest script is there) and its full
// gate, each printed with why; the file validates, and names only scripts it has.
func TestInitConfigDetectsTheOperatingModel(t *testing.T) {
	for _, withSuggest := range []bool{true, false} {
		files := map[string]string{
			"package.json":             `{"scripts":{"test":"vitest"}}`,
			".claude/owner-queue.sh":   "#!/bin/sh\n",
			".claude/roadmap-queue.sh": "#!/bin/sh\n",
			"scripts/ci/run-local.sh":  "#!/bin/sh\n",
		}
		if withSuggest {
			files[".claude/panel-suggest.sh"] = "#!/bin/sh\n"
		}
		root := newRepo(t, nil)
		for name, body := range files {
			os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
			testbin.Write(t, filepath.Join(root, name), body)
		}
		initGit(t, root, "branch", "change/old-name")
		res, err := InitConfig(context.Background(), signals.ExecRunner, root)
		if err != nil {
			t.Fatal(err)
		}
		c, err := config.ParseConfig(res.Body)
		if err != nil {
			t.Fatalf("suggest=%v: the starter does not validate: %v\n%s", withSuggest, err, res.Body)
		}
		if len(c.Cards) != 2 || c.Cards[0].ID != "owner-queue" || c.Cards[1].ID != "change-queue" || !c.Cards[0].Pin || !c.Cards[1].Pin ||
			!reflect.DeepEqual(c.Cards[0].Command, []string{"sh", "-c", "sh .claude/owner-queue.sh 2>&1"}) || c.Cards[0].Refresh != "watch:docs/owner-queue.md" ||
			!reflect.DeepEqual(c.Cards[1].Command, []string{"sh", "-c", "sh .claude/roadmap-queue.sh --text 2>&1"}) || c.Cards[1].Refresh != "watch:docs/roadmap.md" {
			t.Fatalf("cards %+v", c.Cards)
		}
		var ids []string
		for _, tp := range c.Templates {
			ids = append(ids, tp.ID)
			if (tp.Suggest != nil) != withSuggest {
				t.Errorf("suggest=%v: template %s suggest %+v", withSuggest, tp.ID, tp.Suggest)
			}
			if tp.Suggest != nil && !reflect.DeepEqual(tp.Suggest.Command, []string{"sh", ".claude/panel-suggest.sh", tp.ID}) {
				t.Errorf("template %s suggest %v", tp.ID, tp.Suggest.Command)
			}
		}
		if !reflect.DeepEqual(ids, []string{"build", "propose", "fix", "ops"}) {
			t.Fatalf("templates %v", ids)
		}
		if c.Lanes["change/"] != "build" || c.Lanes["fix/"] != "fix" || c.Lanes["ops/"] != "ops" || c.Lanes["main"] != "orchestrator" {
			t.Fatalf("lanes %v", c.Lanes)
		}
		if len(c.Queues) != 1 || !reflect.DeepEqual(c.Queues[0].Command, []string{"scripts/ci/run-local.sh"}) {
			t.Fatalf("queues %+v", c.Queues)
		}
		notes := strings.Join(res.Notes, "\n")
		for _, s := range []string{"Owner queue, pinned", "Change queue, pinned", "templates     build, propose, fix, ops", "`scripts/ci/run-local.sh`",
			"also found `npm run test`", "change/ → build, not change", "ops/ → ops added"} {
			if !strings.Contains(notes, s) {
				t.Errorf("suggest=%v: the notes do not say %q:\n%s", withSuggest, s, notes)
			}
		}
		if strings.Contains(notes, "clauductor install") || strings.Count(notes, "queues ") != 1 {
			t.Errorf("suggest=%v: the hint, or two queue lines:\n%s", withSuggest, notes)
		}
	}

	// Only the owner queue's script: its card and the templates; the gate is the one
	// the project defines, and no script the repository lacks is named.
	root := newRepo(t, map[string]string{"Makefile": "ci:\n\ttrue\n"})
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	testbin.Write(t, filepath.Join(root, ".claude/owner-queue.sh"), "#!/bin/sh\n")
	res, err := InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Cards) != 1 || c.Cards[0].ID != "owner-queue" || len(c.Templates) != 4 || len(c.Queues) != 1 || !reflect.DeepEqual(c.Queues[0].Command, []string{"make", "ci"}) {
		t.Fatalf("cards %+v templates %d queues %+v", c.Cards, len(c.Templates), c.Queues)
	}
	if strings.Contains(string(res.Body), "roadmap-queue.sh --text 2>&1") || strings.Contains(string(res.Body), "run-local") || strings.Contains(string(res.Body), "panel-suggest") {
		t.Fatalf("names a script the repository lacks:\n%s", res.Body)
	}
}

// [ATTACH-1-S2] PANEL-42: the plan for an attach is init's reading of the repository,
// unchanged, with the target moved and every command in argv form: no `sh -c`, no
// `2>&1`. Its notes are init's. A config already in the repository does not refuse it
// (the panel will read the sidecar, D7), and a sidecar already at the target does.
func TestPlanInitForASidecar(t *testing.T) {
	om := map[string]string{
		".claude/owner-queue.sh":   "#!/bin/sh\n",
		".claude/roadmap-queue.sh": "#!/bin/sh\n",
		".claude/panel-suggest.sh": "#!/bin/sh\n",
		"scripts/ci/run-local.sh":  "#!/bin/sh\n",
	}
	plain := map[string]string{"package.json": `{"scripts":{"gate":"x","test":"y"}}`, "bun.lock": ""}
	for _, files := range []map[string]string{om, plain} {
		root := newRepo(t, nil)
		for name, body := range files {
			os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
			testbin.Write(t, filepath.Join(root, name), body)
		}
		ctx := context.Background()
		inRepo, err := PlanInit(ctx, signals.ExecRunner, root, InitTarget{})
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "projects", "acme-web", "panel.json")
		side, err := PlanInit(ctx, signals.ExecRunner, root, InitTarget{Path: target, Sidecar: true})
		if err != nil {
			t.Fatal(err)
		}
		if side.Path != target || !side.Sidecar || inRepo.Sidecar {
			t.Fatalf("path %s sidecar %v (in-repository %v)", side.Path, side.Sidecar, inRepo.Sidecar)
		}
		if !reflect.DeepEqual(side.Notes, inRepo.Notes) {
			t.Errorf("the notes differ:\n%q\n%q", side.Notes, inRepo.Notes)
		}
		want, err := config.ParseConfig(inRepo.Body)
		if err != nil {
			t.Fatal(err)
		}
		got, err := config.ParseConfig(side.Body)
		if err != nil {
			t.Fatalf("the sidecar does not validate: %v\n%s", err, side.Body)
		}
		// The one difference: the operating model's cards, in argv form.
		argv := map[string][]string{
			"owner-queue":  {"sh", ".claude/owner-queue.sh"},
			"change-queue": {"sh", ".claude/roadmap-queue.sh", "--text"},
		}
		for i := range want.Cards {
			want.Cards[i].Command = argv[want.Cards[i].ID]
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the sidecar is not init's reading with argv commands:\n%s\nin the repository:\n%s", side.Body, inRepo.Body)
		}
		var cmds [][]string
		for _, c := range got.Cards {
			cmds = append(cmds, c.Command)
		}
		for _, q := range got.Queues {
			cmds = append(cmds, q.Command)
		}
		for _, tp := range got.Templates {
			if tp.Suggest != nil {
				cmds = append(cmds, tp.Suggest.Command)
			}
		}
		if len(cmds) == 0 {
			t.Fatalf("no command to check:\n%s", side.Body)
		}
		// The sidecar command rule (design.md D5) marks no line of it.
		if refused := got.SidecarRefusals(root); len(refused) != 0 {
			t.Errorf("the attach plan holds commands a sidecar may not: %+v", refused)
		}
		for _, c := range cmds {
			for _, w := range c {
				if w == "-c" || strings.Contains(w, "2>&1") || strings.Contains(w, " ") {
					t.Errorf("a shell form in %q", c)
				}
			}
		}
		if _, bun := files["bun.lock"]; bun {
			if len(got.Queues) != 1 || !reflect.DeepEqual(got.Queues[0].Command, []string{"bun", "run", "gate"}) {
				t.Errorf("the detected gate: %+v", got.Queues)
			}
		} else if len(got.Cards) != 2 || len(got.Templates) != 4 {
			t.Errorf("the operating model's cards and templates: %+v %+v", got.Cards, got.Templates)
		}

		// The repository's own config is not a refusal for a sidecar, only a note.
		own := filepath.Join(root, config.DefaultConfigRel)
		os.MkdirAll(filepath.Dir(own), 0o755)
		os.WriteFile(own, []byte(`{"name":"own"}`), 0o644)
		if _, err := PlanInit(ctx, signals.ExecRunner, root, InitTarget{}); !errors.Is(err, ErrConfigExists) {
			t.Fatalf("init over the repository's config: %v", err)
		}
		side2, err := PlanInit(ctx, signals.ExecRunner, root, InitTarget{Path: target, Sidecar: true})
		if err != nil || !strings.Contains(strings.Join(side2.Notes, "\n"), own+" is here too, and the panel will not read it") {
			t.Fatalf("attach beside the repository's config: %v %q", err, side2.Notes)
		}
		// A sidecar is the user's alone, and is never planned over.
		if err := WriteInit(side); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("the sidecar's mode: %v %v", fi, err)
		}
		if fi, err := os.Stat(filepath.Dir(target)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("the sidecar's folder's mode: %v %v", fi, err)
		}
		if _, err := PlanInit(ctx, signals.ExecRunner, root, InitTarget{Path: target, Sidecar: true}); !errors.Is(err, ErrConfigExists) {
			t.Fatalf("a plan over a sidecar: %v", err)
		}
		// A sidecar folder that is there already, looser, is tightened to 0700 (D2).
		loose := filepath.Join(t.TempDir(), "projects", "loose", "panel.json")
		os.MkdirAll(filepath.Dir(loose), 0o755)
		os.Chmod(filepath.Dir(loose), 0o755)
		side.Path = loose
		if err := WriteInit(side); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(filepath.Dir(loose)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("an existing sidecar folder's mode: %v %v", fi.Mode(), err)
		}
	}
}
