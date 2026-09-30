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
