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
	for _, s := range []string{"only when you press RUN", "`pnpm run check`", "`pnpm run test`"} {
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
