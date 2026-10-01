package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// cfgRepo is a repository with this panel.json (not committed).
func cfgRepo(t *testing.T, cfg string) string {
	t.Helper()
	root := newRepo(t, nil)
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), cfg)
	return root
}

// refusedAs runs Inspect and returns the refusal's code, or "" when it passed.
func refusedAs(t *testing.T, home, path string) (Candidate, string) {
	t.Helper()
	c, err := Inspect(context.Background(), InspectOptions{Home: home, Path: path, Run: signals.ExecRunner})
	if err == nil {
		return c, ""
	}
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("Inspect(%s): %v is not a refusal", path, err)
	}
	return c, r.Code
}

// PANEL-22: what the page's "Add a project…" checks before anything is written. Every
// refusal is one `panel add` makes; a repository that passes says what adding it
// would do, and nothing ran that the repository names.
func TestInspectRefusesWhatAddWouldNot(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	ctx := context.Background()
	reg := cfgRepo(t, `{"name":"Registered","lanes":{"main":"orchestrator"}}`)
	if _, err := AddProject(ctx, AddOptions{Home: home, Project: reg, Run: signals.ExecRunner, Now: time.Now(), Strict: true}); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	initGit(t, reg, "worktree", "add", "-q", "-b", "x", linked)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(reg, link); err != nil {
		t.Fatal(err)
	}
	plain := t.TempDir()
	file := filepath.Join(plain, "file")
	writeFile(t, file, "x")
	// A config whose socket is the registered project's (the historical "clauductor").
	clash := cfgRepo(t, `{"name":"Clash","lanes":{"main":"orchestrator"},"tmux_socket":"clauductor"}`)
	for _, c := range []struct{ path, code string }{
		{"", "not-absolute"},
		{"relative/path", "not-absolute"},
		{"~someone/x", "not-absolute"},
		{"/no/such/dir-p22", "not-found"},
		{file, "not-dir"},
		{plain, "not-git"},
		{linked, "linked-worktree"},
		{reg, "registered"},
		{link, "registered"},                              // a symlinked duplicate
		{filepath.Join(reg, ".clauductor"), "registered"}, // a path inside it
		{clash, "socket-taken"},
	} {
		if _, code := refusedAs(t, home, c.path); code != c.code {
			t.Errorf("Inspect(%q) = %q, want %q", c.path, code, c.code)
		}
	}
	if os.Getuid() != 0 {
		locked := t.TempDir()
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(locked, 0o755) })
		if _, code := refusedAs(t, home, locked); code != "unreadable" {
			t.Errorf("an unreadable directory: %q", code)
		}
	}
}

func TestInspectSaysWhatAddingWouldDo(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	// No config yet: it names the root, the common dir and the id init's name gives.
	bare := newRepo(t, nil)
	c, code := refusedAs(t, home, bare)
	if code != "" || c.Root != bare || c.HasConfig || c.ID != "acme-web" || c.Socket != config.DefaultTmuxSocket ||
		c.CommonDir != filepath.Join(bare, ".git") || c.ConfigPath != filepath.Join(bare, config.DefaultConfigRel) {
		t.Fatalf("no config: %+v %s", c, code)
	}
	// ~ is the home directory.
	inHome := filepath.Join(home, "repo")
	if err := os.Rename(bare, inHome); err != nil {
		t.Fatal(err)
	}
	if c, code := refusedAs(t, home, "~/repo"); code != "" || c.Root != inHome {
		t.Fatalf("~/repo: %+v %s", c, code)
	}
	// A config: its trust report, untrusted, and what it runs.
	cfgd := cfgRepo(t, `{"name":"Beta One","lanes":{"main":"orchestrator"},
		"cards":[{"id":"c","title":"C","command":["touch","ran"],"refresh":"interval:60"}]}`)
	c, code = refusedAs(t, home, cfgd)
	if code != "" || !c.HasConfig || c.ConfigError != "" || c.ID != "beta-one" || c.Name != "Beta One" || c.Trusted || len(c.Hash) != 64 ||
		len(c.Runs) != 1 || !strings.Contains(c.Runs[0], `card c runs ["touch" "ran"]`) {
		t.Fatalf("config: %+v %s", c, code)
	}
	if _, err := os.Stat(filepath.Join(cfgd, "ran")); err == nil {
		t.Fatal("Inspect ran the card's command")
	}
	// A config that does not load says why, and passes Inspect (the page shows it).
	broken := cfgRepo(t, `{"name":"B","no_such_key":1}`)
	if c, code := refusedAs(t, home, broken); code != "" || !c.HasConfig || !strings.Contains(c.ConfigError, "no_such_key") {
		t.Fatalf("broken: %+v %s", c, code)
	}
}

// The init preview writes nothing; writing it never overwrites.
func TestPlanInitWritesNothing(t *testing.T) {
	t.Parallel()
	root := newRepo(t, nil)
	res, err := PlanInit(context.Background(), signals.ExecRunner, root)
	if err != nil || len(res.Body) == 0 || len(res.Notes) == 0 {
		t.Fatalf("plan: %+v %v", res, err)
	}
	if _, err := os.Lstat(res.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the preview wrote %s", res.Path)
	}
	if err := WriteInit(res); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(res.Path); string(b) != string(res.Body) {
		t.Fatal("WriteInit wrote something else")
	}
	if err := WriteInit(res); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("a second write: %v", err)
	}
	if _, err := PlanInit(context.Background(), signals.ExecRunner, root); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("a plan over an existing config: %v", err)
	}
}

// "Trust and add" trusts exactly the bytes its report showed.
func TestTrustExactRefusesChangedBytes(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := cfgRepo(t, `{"name":"T","lanes":{"main":"orchestrator"}}`)
	path := filepath.Join(root, config.DefaultConfigRel)
	_, raw, _ := config.LoadConfigRaw(path)
	shown := ConfigHash(raw)
	writeFile(t, path, `{"name":"T","lanes":{"main":"orchestrator"},"cards":[{"id":"x","title":"X","command":["rm","-rf","~"],"refresh":"interval:60"}]}`)
	if _, err := TrustExact(home, root, path, shown); !errors.Is(err, ErrConfigChanged) {
		t.Fatalf("changed bytes: %v", err)
	}
	if _, raw, _ := config.LoadConfigRaw(path); TrustedNow(home, root, path, ConfigHash(raw)) {
		t.Fatal("the changed config was trusted")
	}
	_, raw, _ = config.LoadConfigRaw(path)
	if tv, err := TrustExact(home, root, path, ConfigHash(raw)); err != nil || !tv.Trusted {
		t.Fatalf("the bytes now: %+v %v", tv, err)
	}
}
