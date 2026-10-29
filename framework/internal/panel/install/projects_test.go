package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// projectDir is a repository root with a panel.json, and a linked worktree inside it,
// as git lists them (the runner stands in for git).
func projectDir(t *testing.T, name, cfgExtra string) (root string, run signals.Runner) {
	t.Helper()
	root = signals.ResolvePath(t.TempDir())
	linked := filepath.Join(root, ".claude", "worktrees", "x")
	if err := os.MkdirAll(linked, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"`+name+`","lanes":{"main":"orchestrator"}`+cfgExtra+`}`)
	return root, func(_ context.Context, dir string, argv []string) ([]byte, error) {
		if strings.Join(argv, " ") != "git worktree list --porcelain" {
			return nil, fmt.Errorf("unexpected %v", argv)
		}
		return []byte(fmt.Sprintf("worktree %s\nHEAD a\nbranch refs/heads/main\n\nworktree %s\nHEAD b\nbranch refs/heads/x\n\n", root, linked)), nil
	}
}

// PANEL-16: adding a project registers its main worktree once, gives it an id and a
// socket of its own, and never trusts its config.
func TestAddProject(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ctx := context.Background()
	now := time.Unix(1_790_000_000, 0)
	a, runA := projectDir(t, "Standing T", "")
	res, err := AddProject(ctx, AddOptions{Home: home, Project: a, Run: runA, Now: now, Strict: true})
	if err != nil || !res.Added || res.Entry.ID != "standing-t" || res.Entry.TmuxSocket != "clauductor" || res.Entry.Root != a {
		t.Fatalf("first add: %+v %v", res.Entry, err)
	}
	if TrustedNow(home, a, res.Entry.ConfigPath(), "anything") {
		t.Fatal("add trusted the config")
	}
	// The same project again, by a path inside it or through a symlink, is the same
	// project; a linked worktree is refused (Strict) or names its main one.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, link, filepath.Join(a, ".clauductor")} {
		if res, err := AddProject(ctx, AddOptions{Home: home, Project: p, Run: runA, Now: now, Strict: true}); err != nil || res.Added {
			t.Fatalf("%s added again: %+v %v", p, res, err)
		}
	}
	linked := filepath.Join(a, ".claude", "worktrees", "x")
	if _, err := AddProject(ctx, AddOptions{Home: home, Project: linked, Run: runA, Now: now, Strict: true}); err == nil || !strings.Contains(err.Error(), "linked worktree") {
		t.Fatalf("a linked worktree: %v", err)
	}
	if res, err := AddProject(ctx, AddOptions{Home: home, Project: linked, Run: runA, Now: now}); err != nil || res.Added || res.Entry.Root != a {
		t.Fatalf("a bare panel in a linked worktree names its main one: %+v %v", res.Entry, err)
	}
	// A second project gets clauductor-<id>; its name taken, the id gets a suffix.
	b, runB := projectDir(t, "Standing T", "")
	res, err = AddProject(ctx, AddOptions{Home: home, Project: b, Run: runB, Now: now, Strict: true})
	if err != nil || res.Entry.ID != "standing-t-2" || res.Entry.TmuxSocket != "clauductor-standing-t-2" {
		t.Fatalf("second add: %+v %v", res.Entry, err)
	}
	reg, _ := config.LoadProjects(home)
	if reg.Default != "standing-t" {
		t.Fatalf("the default moved without --default: %s", reg.Default)
	}
	// A config naming a socket another project has is refused.
	c, runC := projectDir(t, "C", `,"tmux_socket":"clauductor"`)
	if _, err := AddProject(ctx, AddOptions{Home: home, Project: c, Run: runC, Now: now, Strict: true}); err == nil || !strings.Contains(err.Error(), "socket") {
		t.Fatalf("a shared socket: %v", err)
	}
	// And one whose config does not load is not registered.
	d := signals.ResolvePath(t.TempDir())
	if _, err := AddProject(ctx, AddOptions{Home: home, Project: d, Run: func(context.Context, string, []string) ([]byte, error) {
		return []byte("worktree " + d + "\nHEAD a\nbranch refs/heads/main\n\n"), nil
	}, Now: now, Strict: true}); err == nil || !strings.Contains(err.Error(), "no panel config") {
		t.Fatalf("no config: %v", err)
	}
	// MakeDefault moves the default.
	if _, err := AddProject(ctx, AddOptions{Home: home, Project: b, Run: runB, Now: now, MakeDefault: true}); err != nil {
		t.Fatal(err)
	}
	if reg, _ = config.LoadProjects(home); reg.Default != "standing-t-2" || len(reg.Projects) != 2 {
		t.Fatalf("make default: %+v", reg)
	}
}

// Removing a project stops nothing and keeps its lane registry; with lanes still
// registered it needs --force.
func TestRemoveProject(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ctx := context.Background()
	a, runA := projectDir(t, "A", "")
	b, runB := projectDir(t, "B", "")
	for _, p := range []struct {
		root string
		run  signals.Runner
	}{{a, runA}, {b, runB}} {
		if _, err := AddProject(ctx, AddOptions{Home: home, Project: p.root, Run: p.run, Now: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := lanes.OpenRegistry(home, a)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := reg.Begin(types.LaneRecord{ID: "lane-a", SessionID: "00000000-0000-4000-8000-000000000001", Path: a, Type: "build", Mode: "root"}, "start", clock.System.Now())
	_ = reg.Done(rec)
	if _, err := RemoveProject(home, "a", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("remove with lanes: %v", err)
	}
	if _, err := RemoveProject(home, "nope", false); err == nil {
		t.Fatal("an unknown project was removed")
	}
	if e, err := RemoveProject(home, a, true); err != nil || e.ID != "a" {
		t.Fatalf("forced remove by path: %+v %v", e, err)
	}
	p, _ := config.LoadProjects(home)
	if len(p.Projects) != 1 || p.Default != "b" {
		t.Fatalf("after removing the default: %+v", p)
	}
	if again, _ := lanes.OpenRegistry(home, a); len(again.List()) != 1 {
		t.Fatal("removing the project dropped its lane registry")
	}
	var out strings.Builder
	if err := ListProjects(&out, home); err != nil || !strings.Contains(out.String(), "* b") || !strings.Contains(out.String(), "untrusted") {
		t.Fatalf("list:\n%s %v", out.String(), err)
	}
}

// ATTACH-4's requirement, the outside mark (design.md D7): a root that does not exist
// (yet, or any more) under a symlinked ancestor, as /var is to /private/var on macOS,
// is resolved as its config is, so a config inside it is not marked outside. Both
// sides go through one resolver; a root resolved as a whole falls back to the path as
// given once a component is missing, while its config resolves the ancestor.
func TestConfigOutsideResolvesAMissingRootAsItsConfig(t *testing.T) {
	t.Parallel()
	dir := signals.ResolvePath(t.TempDir())
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(link, "gone")
	for _, e := range []config.ProjectEntry{
		{ID: "own", Root: root},
		{ID: "given", Root: root, Config: filepath.Join(link, "gone", "cfg", "panel.json")},
		{ID: "real", Root: root, Config: filepath.Join(real, "gone", config.DefaultConfigRel)},
	} {
		if ConfigOutside(e) {
			t.Errorf("%s: %s is marked outside %s", e.ID, e.ConfigPath(), e.Root)
		}
	}
	if e := (config.ProjectEntry{ID: "side", Root: root, Config: filepath.Join(dir, "side", "panel.json")}); !ConfigOutside(e) {
		t.Errorf("a config beside the root is not marked outside")
	}
}

// ATTACH-4's requirement, `panel list`'s config line (design.md D7): every project's
// config path; one outside its repository is marked so, with a line when the
// repository's own .clauductor/panel.json is there and not read. A config reached
// through a symlink into the repository is inside it. (The scenario's test, which also
// loads the project as the panel does, is TestAttachedProjectReadsItsSidecar.)
func TestListProjectsNamesEachConfig(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	ctx := context.Background()
	own, runOwn := projectDir(t, "Own", "")
	both, runBoth := projectDir(t, "Both", "")     // its own panel.json, and a sidecar
	sideOnly, runSide := projectDir(t, "Bare", "") // a sidecar, and no panel.json of its own
	if err := os.Remove(filepath.Join(sideOnly, config.DefaultConfigRel)); err != nil {
		t.Fatal(err)
	}
	linked, runLinked := projectDir(t, "Linked", "") // --config through a symlink into the repository
	link := filepath.Join(t.TempDir(), "in")
	if err := os.Symlink(filepath.Join(linked, ".clauductor"), link); err != nil {
		t.Fatal(err)
	}
	sidecar := func(id, name string) string {
		p := config.SidecarPath(home, id)
		writeFile(t, p, `{"name":"`+name+`","lanes":{"main":"orchestrator"}}`)
		return p
	}
	bothCfg, sideCfg := sidecar("both", "Both sidecar"), sidecar("bare", "Bare sidecar")
	for _, a := range []AddOptions{
		{Project: own, Run: runOwn},
		{Project: both, Run: runBoth, Config: bothCfg, ID: "both"},
		{Project: sideOnly, Run: runSide, Config: sideCfg, ID: "bare"},
		{Project: linked, Run: runLinked},
	} {
		a.Home, a.Now = home, time.Now()
		if _, err := AddProject(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	// AddProject resolves a --config; a registry written by hand may not.
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	reg.Find(linked).Config = filepath.Join(link, "panel.json")
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := ListProjects(&out, home); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	ownFile := func(root string) string { return filepath.Join(root, config.DefaultConfigRel) }
	for _, want := range []string{
		"    config " + ownFile(own) + "\n",
		"both             Both sidecar\n",
		"    config " + bothCfg + " (outside the repository)\n    " + ownFile(both) + " is not read: the registry names the config above\n",
		"    config " + sideCfg + " (outside the repository)\n    socket ",
		"    config " + filepath.Join(link, "panel.json") + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("list lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "outside the repository"); n != 2 {
		t.Errorf("%d configs marked outside, want 2:\n%s", n, got)
	}
}

// CheckAdd is every check AddProject makes before it saves, so that a caller asking "would this
// be refused" (clauductor doctor, to pick the fix it prints) cannot read the rule differently
// from the command. Each refusal AddProject makes has the same text from CheckAdd, and neither
// writes the home directory when it refuses.
func TestCheckAddRefusesWhatAddProjectRefusesAndWritesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_790_000_000, 0)

	// registered is a home holding project "a" (name "A", socket clauductor).
	registered := func(t *testing.T) (home, a string, runA signals.Runner) {
		t.Helper()
		home = t.TempDir()
		a, runA = projectDir(t, "A", "")
		if _, err := AddProject(ctx, AddOptions{Home: home, Project: a, Run: runA, Now: now, Strict: true}); err != nil {
			t.Fatal(err)
		}
		return home, a, runA
	}
	cases := []struct {
		name  string
		want  string // a part of the refusal
		setup func(t *testing.T) AddOptions
	}{
		{"not a directory", "is not a directory", func(t *testing.T) AddOptions {
			return AddOptions{Home: t.TempDir(), Project: filepath.Join(t.TempDir(), "missing"), Now: now}
		}},
		{"git fails", "git exploded", func(t *testing.T) AddOptions {
			return AddOptions{Home: t.TempDir(), Project: t.TempDir(), Now: now, Run: func(context.Context, string, []string) ([]byte, error) {
				return nil, fmt.Errorf("git exploded")
			}}
		}},
		{"git lists no worktree", "returned no worktrees", func(t *testing.T) AddOptions {
			return AddOptions{Home: t.TempDir(), Project: t.TempDir(), Now: now, Run: func(context.Context, string, []string) ([]byte, error) {
				return []byte("\n"), nil
			}}
		}},
		{"a linked worktree", "is a linked worktree of", func(t *testing.T) AddOptions {
			root, run := projectDir(t, "L", "")
			return AddOptions{Home: t.TempDir(), Project: filepath.Join(root, ".claude", "worktrees", "x"), Run: run, Now: now, Strict: true}
		}},
		{"the registry does not load", "projects.json", func(t *testing.T) AddOptions {
			home := t.TempDir()
			writeFile(t, config.ProjectsPath(home), "not json")
			root, run := projectDir(t, "R", "")
			return AddOptions{Home: home, Project: root, Run: run, Now: now, Strict: true}
		}},
		{"the repository is registered under another path", "registered already as", func(t *testing.T) AddOptions {
			home, _, runA := registered(t)
			b, runB := projectDir(t, "B", "")
			// One git common dir for both roots: b is a second path to a's repository.
			return AddOptions{Home: home, Project: b, Now: now, Strict: true, Run: func(ctx context.Context, dir string, argv []string) ([]byte, error) {
				if strings.Join(argv, " ") == "git rev-parse --git-common-dir" {
					return []byte("/shared/.git\n"), nil
				}
				if dir == b {
					return runB(ctx, dir, argv)
				}
				return runA(ctx, dir, argv)
			}}
		}},
		{"no config", "no panel config", func(t *testing.T) AddOptions {
			d := signals.ResolvePath(t.TempDir())
			return AddOptions{Home: t.TempDir(), Project: d, Now: now, Strict: true, Run: func(context.Context, string, []string) ([]byte, error) {
				return []byte("worktree " + d + "\nHEAD a\nbranch refs/heads/main\n\n"), nil
			}}
		}},
		{"a registered project whose config no longer loads", "panel config", func(t *testing.T) AddOptions {
			home, a, runA := registered(t)
			writeFile(t, filepath.Join(a, config.DefaultConfigRel), `{"name":"A","bogus":1}`)
			return AddOptions{Home: home, Project: a, Run: runA, Now: now, Strict: true}
		}},
		{"an id that is not one", "must match", func(t *testing.T) AddOptions {
			root, run := projectDir(t, "B", "")
			return AddOptions{Home: t.TempDir(), Project: root, Run: run, Now: now, Strict: true, ID: "Not An ID"}
		}},
		{"an id that is taken", "is taken", func(t *testing.T) AddOptions {
			home, _, _ := registered(t)
			b, runB := projectDir(t, "B", "")
			return AddOptions{Home: home, Project: b, Run: runB, Now: now, Strict: true, ID: "a"}
		}},
		{"a tmux socket another project holds", "give this project its own tmux_socket", func(t *testing.T) AddOptions {
			home, _, _ := registered(t)
			c, runC := projectDir(t, "C", `,"tmux_socket":"clauductor"`)
			return AddOptions{Home: home, Project: c, Run: runC, Now: now, Strict: true}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.setup(t)
			before := treesnap.Take(t, o.Home)
			_, checkErr := CheckAdd(ctx, o)
			if d := treesnap.Diff(before, treesnap.Take(t, o.Home)); len(d) > 0 {
				t.Fatalf("CheckAdd wrote the home directory: %v", d)
			}
			_, addErr := AddProject(ctx, o)
			if checkErr == nil || addErr == nil || !strings.Contains(checkErr.Error(), tc.want) {
				t.Fatalf("want a refusal holding %q from both, got CheckAdd %v, AddProject %v", tc.want, checkErr, addErr)
			}
			if checkErr.Error() != addErr.Error() {
				t.Errorf("the refusals differ:\n CheckAdd:   %v\n AddProject: %v", checkErr, addErr)
			}
			if d := treesnap.Diff(before, treesnap.Take(t, o.Home)); len(d) > 0 {
				t.Errorf("a refused AddProject wrote the home directory: %v", d)
			}
		})
	}

	t.Run("a project it would add", func(t *testing.T) {
		home := t.TempDir()
		root, run := projectDir(t, "Fine", "")
		o := AddOptions{Home: home, Project: root, Run: run, Now: now, Strict: true}
		before := treesnap.Take(t, home)
		if _, err := CheckAdd(ctx, o); err != nil {
			t.Fatal(err)
		}
		if d := treesnap.Diff(before, treesnap.Take(t, home)); len(d) > 0 {
			t.Fatalf("CheckAdd wrote the home directory: %v", d)
		}
		if res, err := AddProject(ctx, o); err != nil || !res.Added {
			t.Fatalf("AddProject after a passing CheckAdd: %+v %v", res, err)
		}
	})
}
