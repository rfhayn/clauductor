package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/lanes"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/types"
	"github.com/rfhayn/stridal/internal/testbin"
)

func attach(t *testing.T, home, project, id string) (AttachResult, error) {
	t.Helper()
	return Attach(context.Background(), AttachOptions{Home: home, Project: project, ID: id, Run: signals.ExecRunner, Now: time.Unix(1_790_000_000, 0)})
}

// [ATTACH-1-S1] Attach writes ~/.clauductor/projects/<id>/panel.json, registers the
// repository with that file as its config, and leaves it untrusted; the project loads.
func TestAttachWritesTheSidecarAndRegisters(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	res, err := attach(t, home, root, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".clauductor", "projects", "acme-web", "panel.json")
	if res.Add.Entry.ID != "acme-web" || res.Add.Entry.Config != path || res.Init.Path != path || !res.Add.Added || res.InPlace {
		t.Fatalf("entry %+v init path %s in place %v", res.Add.Entry, res.Init.Path, res.InPlace)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != string(res.Init.Body) {
		t.Fatalf("the sidecar: %v\n%s", err, b)
	}
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	e := reg.Find(root)
	if e == nil || e.Config != path || e.ConfigPath() != path {
		t.Fatalf("projects.json: %+v", reg)
	}
	cfg, raw, err := config.LoadConfigRaw(e.ConfigPath())
	if err != nil || cfg.Name != "acme-web" {
		t.Fatalf("the project does not load: %v", err)
	}
	if TrustedNow(home, root, path, ConfigHash(raw)) {
		t.Fatal("attach trusted the config")
	}
	if _, err := os.Lstat(filepath.Join(root, config.DefaultConfigRel)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("attach wrote a config into the repository")
	}
}

// [ATTACH-1-S2] The sidecar is init's reading of each repository, with argv commands:
// one with the operating model's queue scripts and one without.
func TestAttachWritesInitsReading(t *testing.T) {
	t.Parallel()
	for _, files := range []map[string]string{
		{".claude/owner-queue.sh": "#!/bin/sh\n", ".claude/roadmap-queue.sh": "#!/bin/sh\n"},
		{"Makefile": "gate:\n\ttrue\n"},
	} {
		home := signals.ResolvePath(t.TempDir())
		root := newRepo(t, nil)
		for name, body := range files {
			os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
			testbin.Write(t, filepath.Join(root, name), body)
		}
		res, err := attach(t, home, root, "")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := PlanInit(context.Background(), signals.ExecRunner, root, InitTarget{Path: filepath.Join(t.TempDir(), "p.json"), Sidecar: true})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(res.Add.Entry.Config)
		if string(b) != string(plan.Body) || !reflect.DeepEqual(res.Init.Notes, plan.Notes) || len(plan.Notes) == 0 {
			t.Fatalf("the sidecar is not init's plan for an attach:\n%s\n%s", b, plan.Body)
		}
		cfg, err := config.ParseConfig(b)
		if err != nil {
			t.Fatal(err)
		}
		// The sidecar command rule (design.md D5) marks no line of it.
		if refused := cfg.SidecarRefusals(root); len(refused) != 0 {
			t.Errorf("the sidecar holds commands a sidecar may not: %+v", refused)
		}
		if _, om := files[".claude/owner-queue.sh"]; om {
			if len(cfg.Cards) != 2 || !reflect.DeepEqual(cfg.Cards[0].Command, []string{"sh", ".claude/owner-queue.sh"}) ||
				!reflect.DeepEqual(cfg.Cards[1].Command, []string{"sh", ".claude/roadmap-queue.sh", "--text"}) {
				t.Fatalf("the operating model's cards: %+v", cfg.Cards)
			}
		} else if len(cfg.Cards) != 0 || len(cfg.Queues) != 1 || !reflect.DeepEqual(cfg.Queues[0].Command, []string{"make", "gate"}) {
			t.Fatalf("cards %+v queues %+v", cfg.Cards, cfg.Queues)
		}
	}
}

// [LANECTX-3-S1] An attached repository's lanes launch with the policy: the sidecar
// declares the latest config version and holds the default launch object, and a lane
// started under it carries LANECTX-1-S5's --settings. (Modified by PANEL-51, design D5:
// attach writes the latest version, 7 there, where it once wrote a literal 6.)
func TestAttachedLanesLaunchWithThePolicy(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	res, err := attach(t, home, root, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(res.Add.Entry.Config)
	if err != nil {
		t.Fatal(err)
	}
	// The file as written, not the parsed config: the scenario is about the keys the
	// sidecar holds, and a parse would not tell a missing key from a misspelt one.
	var raw struct {
		Version int                        `json:"version"`
		Launch  map[string]json.RawMessage `json:"launch"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	wantLaunch := map[string]string{"context_window": "200000", "claudeai_connectors": "false"}
	gotLaunch := map[string]string{}
	for k, v := range raw.Launch {
		gotLaunch[k] = string(v)
	}
	if raw.Version != config.LatestVersion || !reflect.DeepEqual(gotLaunch, wantLaunch) {
		t.Fatalf("LANECTX-3-S1: the sidecar declares version %d and launch %v, want %d and %v:\n%s", raw.Version, gotLaunch, config.LatestVersion, wantLaunch, b)
	}

	// A lane started in the attached repository, under the config the registry names.
	cfg, _, err := config.LoadConfigRaw(res.Add.Entry.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(home, root)
	if err != nil {
		t.Fatal(err)
	}
	m := &lanes.LaneManager{Clock: clock.System, TmuxPath: "/usr/bin/tmux", Socket: cfg.Socket(), Root: root, Cfg: cfg, Registry: reg,
		Program: []string{"/usr/local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
	for _, laneType := range cfg.Lanes {
		argv := m.LaneCommand("x", laneType, sid, false)
		settings := ""
		for i, a := range argv {
			if a == "--settings" && i+1 < len(argv) {
				if settings != "" {
					t.Fatalf("LANECTX-3-S1: --settings twice in %q", argv)
				}
				settings = argv[i+1]
			}
		}
		var s struct {
			Env map[string]string `json:"env"`
		}
		dec := json.NewDecoder(strings.NewReader(settings))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			t.Fatalf("LANECTX-3-S1: a %s lane's --settings %q is not an env object alone: %v", laneType, settings, err)
		}
		want := map[string]string{"ENABLE_CLAUDEAI_MCP_SERVERS": "false", "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "200000"}
		if !reflect.DeepEqual(s.Env, want) {
			t.Errorf("LANECTX-3-S1: a %s lane's settings env %v, want exactly %v (LANECTX-1-S5)", laneType, s.Env, want)
		}
	}
}

// [ATTACH-1-S3] A sidecar already at the path is never overwritten: attach is refused,
// naming the file, how to serve it as it is, and --id.
func TestAttachNeverOverwritesASidecar(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	path := config.SidecarPath(home, "acme-web")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte("someone else's"), 0o600)
	_, err := attach(t, home, root, "")
	if err == nil {
		t.Fatal("attached over an existing sidecar")
	}
	for _, want := range []string{path, "clauductor panel add --project " + root + " --config " + path, "--id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if b, _ := os.ReadFile(path); string(b) != "someone else's" {
		t.Fatalf("the sidecar changed: %q", b)
	}
	if reg, _ := config.LoadProjects(home); len(reg.Projects) != 0 {
		t.Fatalf("registered anyway: %+v", reg.Projects)
	}
	// Another id writes a new one beside it.
	if res, err := attach(t, home, root, "acme-2"); err != nil || res.Add.Entry.Config != config.SidecarPath(home, "acme-2") {
		t.Fatalf("--id: %+v %v", res.Add.Entry, err)
	}
}

// PANEL-47: the command lines attach's refusals print run as printed. The home and
// the root hold a space and a quote, so each path is one shell word only if quoted.
func TestAttachRefusalsQuoteTheirCommandLines(t *testing.T) {
	t.Parallel()
	home := filepath.Join(signals.ResolvePath(t.TempDir()), "o'k home")
	quoted := func(name string) string {
		t.Helper()
		root := newRepo(t, nil)
		moved := filepath.Join(filepath.Dir(root), name)
		if err := os.Rename(root, moved); err != nil {
			t.Fatal(err)
		}
		return moved
	}
	// A working project: remove it, or add it with another config.
	working := quoted("o'k web")
	own := filepath.Join(working, config.DefaultConfigRel)
	os.MkdirAll(filepath.Dir(own), 0o755)
	os.WriteFile(own, []byte(`{"name":"Web"}`), 0o644)
	if _, err := AddProject(context.Background(), AddOptions{Home: home, Project: working, Run: signals.ExecRunner, Strict: true}); err != nil {
		t.Fatal(err)
	}
	_, err := attach(t, home, working, "")
	if want := "`clauductor panel remove web` first, or `clauductor panel add --project " + shq(working) + " --config <file>`"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("a working project's refusal does not say %q: %v", want, err)
	}
	// A sidecar there already: add it as it is.
	other := quoted("o'k other")
	path := config.SidecarPath(home, "other")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte("someone else's"), 0o600)
	_, err = attach(t, home, other, "other")
	if want := "`clauductor panel add --project " + shq(other) + " --config " + shq(path) + "`"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("an existing sidecar's refusal does not say %q: %v", want, err)
	}
}

// [ATTACH-1-S4] A registered project whose registry config is empty and whose
// repository has no .clauductor/panel.json (it cannot load) is attached in place: the
// same id and socket, its lane registry unchanged, and it loads.
func TestAttachInPlace(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	reg := &config.Projects{Version: config.ProjectsVersion, Default: "standingt",
		Projects: []config.ProjectEntry{{ID: "standingt", Root: root, TmuxSocket: "clauductor-standingt", Added: 1}}}
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
	lr, err := lanes.OpenRegistry(home, root)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := lr.Begin(types.LaneRecord{ID: "lane-a", SessionID: "00000000-0000-4000-8000-000000000001", Path: root, Type: "build", Mode: "root"}, "start", clock.System.Now())
	_ = lr.Done(rec)
	before := lr.List()

	if _, err := attach(t, home, root, "other"); err == nil || !strings.Contains(err.Error(), "keeps that id") {
		t.Fatalf("--id on an in-place attach: %v", err)
	}
	// A sidecar already at the in-place path: --id is no way out, since D4 keeps the
	// id, so the refusal offers only serving the file as it is.
	path := config.SidecarPath(home, "standingt")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte("someone else's"), 0o600)
	_, err = attach(t, home, root, "")
	if err == nil || !strings.Contains(err.Error(), "clauductor panel add --project "+root+" --config "+path) || strings.Contains(err.Error(), "--id") {
		t.Fatalf("an in-place attach over a sidecar: %v", err)
	}
	os.Remove(path)
	res, err := attach(t, home, root, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := config.LoadProjects(home)
	if !res.InPlace || res.Add.Added || len(got.Projects) != 1 {
		t.Fatalf("in place %v added %v: %+v", res.InPlace, res.Add.Added, got)
	}
	if e := got.Projects[0]; e.ID != "standingt" || e.TmuxSocket != "clauductor-standingt" || e.Config != path || e.Root != root || got.Default != "standingt" {
		t.Fatalf("the entry: %+v", e)
	}
	if _, err := config.LoadConfig(got.Projects[0].ConfigPath()); err != nil {
		t.Fatalf("the project does not load: %v", err)
	}
	again, _ := lanes.OpenRegistry(home, root)
	if !reflect.DeepEqual(again.List(), before) || len(before) != 1 {
		t.Fatalf("the lane registry changed: %+v → %+v", before, again.List())
	}
}

// [ATTACH-1-S5] A registered project whose config loads is refused with its id and
// the config it reads, and nothing is written under ~/.clauductor/projects/. A linked
// worktree is refused as `panel add` refuses it.
func TestAttachRefusesAWorkingProject(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	own := filepath.Join(root, config.DefaultConfigRel)
	os.MkdirAll(filepath.Dir(own), 0o755)
	os.WriteFile(own, []byte(`{"name":"Web"}`), 0o644)
	if _, err := AddProject(context.Background(), AddOptions{Home: home, Project: root, Run: signals.ExecRunner, Strict: true}); err != nil {
		t.Fatal(err)
	}
	_, err := attach(t, home, root, "")
	if err == nil || !strings.Contains(err.Error(), `registered already as "web", reading `+own) {
		t.Fatalf("a working project: %v", err)
	}
	initGit(t, root, "worktree", "add", "-q", "-b", "x", filepath.Join(root, ".worktrees", "x"))
	if _, err := attach(t, home, filepath.Join(root, ".worktrees", "x"), ""); err == nil || !strings.Contains(err.Error(), "linked worktree") {
		t.Fatalf("a linked worktree: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".clauductor", "projects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused attach wrote under ~/.clauductor/projects: %v", err)
	}
}

// [ATTACH-1-S6] A registration refused after the sidecar is written (its tmux socket
// is another project's) is refused with `panel add`'s reason, and leaves no sidecar
// file and no registry entry.
func TestAttachRefusedRegistrationLeavesNothing(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	other := newRepo(t, nil)
	own := filepath.Join(other, config.DefaultConfigRel)
	os.MkdirAll(filepath.Dir(own), 0o755)
	os.WriteFile(own, []byte(`{"name":"Other","tmux_socket":"clauductor-acme-web"}`), 0o644)
	if _, err := AddProject(context.Background(), AddOptions{Home: home, Project: other, Run: signals.ExecRunner, Strict: true}); err != nil {
		t.Fatal(err)
	}
	root := newRepo(t, nil)
	_, err := attach(t, home, root, "")
	if err == nil || !strings.Contains(err.Error(), `tmux socket "clauductor-acme-web" is other's already`) {
		t.Fatalf("a taken socket: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".clauductor", "projects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused attach left its sidecar folder: %v", err)
	}
	reg, _ := config.LoadProjects(home)
	if len(reg.Projects) != 1 || reg.Find(root) != nil {
		t.Fatalf("the refused attach left an entry: %+v", reg.Projects)
	}
}

// ATTACH-1's requirement, the cleanup behind its refused-registration scenario: a
// sidecar whose write fails part way leaves no file and no folder,
// so the next attach is not refused as "exists already". Not parallel: it swaps
// writeBody, and parallel tests start only once the serial ones are done.
func TestAttachFailedWriteLeavesNothing(t *testing.T) {
	home := signals.ResolvePath(t.TempDir())
	root := newRepo(t, nil)
	orig := writeBody
	writeBody = func(f *os.File, body []byte) error {
		f.Write(body[:len(body)/2])
		return errors.New("disk full")
	}
	_, err := attach(t, home, root, "")
	writeBody = orig
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("a failed write: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".clauductor", "projects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the failed attach left its sidecar or its folder: %v", err)
	}
	if reg, _ := config.LoadProjects(home); len(reg.Projects) != 0 {
		t.Fatalf("the failed attach left an entry: %+v", reg.Projects)
	}
	if _, err := attach(t, home, root, ""); err != nil {
		t.Fatalf("the attach after a failed one: %v", err)
	}
}

// ATTACH-2's requirement, the home-inside-the-repository guard: a home inside the
// repository (dotfiles kept in a work tree at ~) would
// put the sidecar under the root: attach is refused before it writes anything, naming
// the sidecar and the root. So is a home whose ~/.clauductor is a symlink into it,
// since the path is judged with symlinks evaluated.
func TestAttachRefusesAHomeInsideTheRepository(t *testing.T) {
	t.Parallel()
	root := newRepo(t, nil)
	inside := filepath.Join(root, "home")
	os.MkdirAll(inside, 0o755)
	linked := signals.ResolvePath(t.TempDir())
	target := filepath.Join(root, "dot-clauductor")
	os.MkdirAll(target, 0o755)
	if err := os.Symlink(target, filepath.Join(linked, ".clauductor")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ home, sidecar, written string }{
		{inside, config.SidecarPath(inside, "acme-web"), filepath.Join(inside, ".clauductor")},
		{linked, filepath.Join(target, "projects", "acme-web", "panel.json"), target},
	} {
		_, err := attach(t, c.home, root, "")
		if err == nil {
			t.Errorf("home %s: attached with the sidecar inside the repository", c.home)
			continue
		}
		for _, want := range []string{c.sidecar + " is inside the repository " + root, "writes nothing in a repository"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("home %s: the refusal does not say %q: %v", c.home, want, err)
			}
		}
		entries, err := os.ReadDir(c.written)
		if len(entries) != 0 || err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("home %s: the refused attach wrote %v (%v)", c.home, entries, err)
		}
	}
}
