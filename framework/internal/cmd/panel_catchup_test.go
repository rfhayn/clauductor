package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testgit"
)

// catchUpRepo is a repository with one commit and a panel.json of its own named name.
func catchUpRepo(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, ".clauductor"), 0o755); err != nil {
		t.Fatal(err)
	}
	root = signals.ResolvePath(root)
	if err := os.WriteFile(filepath.Join(root, ".clauductor", "panel.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "one"}} {
		if out, err := testgit.Command(root, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return root
}

// mustCLI runs the command and fails the test unless it exits 0.
func mustCLI(t *testing.T, home string, args ...string) cliResult {
	t.Helper()
	r := cli(t, home, args...)
	if r.code != 0 {
		t.Fatalf("%v: exit %d\nstdout: %s\nstderr: %s", args, r.code, r.stdout, r.stderr)
	}
	return r
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// [CATCHUP-1-S1] `panel catch-up --project <A> on` lists A's root in catch-up.json and
// not B's, leaves projects.json byte for byte as it was, and the prints say "on" for A
// and "off" for B; after `off`, A's print says "off".
func TestPanelCatchUpPerProject(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	a, b := catchUpRepo(t, "alpha"), catchUpRepo(t, "beta")
	mustCLI(t, home, "panel", "add", "--project", a)
	mustCLI(t, home, "panel", "add", "--project", b)
	registry := readFile(t, config.ProjectsPath(home))
	if _, err := os.Stat(config.CatchUpPath(home)); !os.IsNotExist(err) {
		t.Fatalf("catch-up.json before any catch-up command: %v", err)
	}

	mustCLI(t, home, "panel", "catch-up", "--project", a, "on")
	c, err := config.LoadCatchUp(home)
	if err != nil {
		t.Fatal(err)
	}
	if !c.On(a) || c.On(b) {
		t.Fatalf("catch-up.json roots %q: want %s listed and not %s", c.Roots, a, b)
	}
	if got := readFile(t, config.ProjectsPath(home)); !bytes.Equal(got, registry) {
		t.Fatalf("projects.json changed:\nbefore %s\nafter  %s", registry, got)
	}
	if got := mustCLI(t, home, "panel", "catch-up", "--project", a).stdout; got != "on\n" {
		t.Fatalf("print for A: %q, want on", got)
	}
	if got := mustCLI(t, home, "panel", "catch-up", "--project", b).stdout; got != "off\n" {
		t.Fatalf("print for B: %q, want off", got)
	}
	// By id too: alpha is A's id, the slug of its config's name.
	if got := mustCLI(t, home, "panel", "catch-up", "--project", "alpha").stdout; got != "on\n" {
		t.Fatalf("print for A by id: %q, want on", got)
	}

	mustCLI(t, home, "panel", "catch-up", "--project", a, "off")
	if got := mustCLI(t, home, "panel", "catch-up", "--project", a).stdout; got != "off\n" {
		t.Fatalf("print for A after off: %q, want off", got)
	}
	if got := readFile(t, config.ProjectsPath(home)); !bytes.Equal(got, registry) {
		t.Fatalf("projects.json changed by off:\nbefore %s\nafter  %s", registry, got)
	}

	// Neither a word but on or off, nor a missing --project, is taken.
	if r := cli(t, home, "panel", "catch-up", "--project", a, "yes"); r.code == 0 {
		t.Fatalf("catch-up yes: exit 0 (%s)", r.stdout)
	}
	if r := cli(t, home, "panel", "catch-up", "on"); r.code == 0 || !strings.Contains(r.stderr, "--project") {
		t.Fatalf("catch-up with no --project: exit %d stderr %q", r.code, r.stderr)
	}
}

// [CATCHUP-1-S2] An attached project, its config outside its repository, cannot turn
// catch-up on: the command exits non-zero naming the config outside the repository,
// writes no catch-up.json, and leaves projects.json byte for byte as it was.
func TestPanelCatchUpRefusedForAttachedProject(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := attachRepo(t)
	mustCLI(t, home, "panel", "attach", root)
	registry := readFile(t, config.ProjectsPath(home))
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	e := reg.Find(root)
	if e == nil {
		t.Fatalf("attach did not register %s", root)
	}

	r := cli(t, home, "panel", "catch-up", "--project", root, "on")
	if r.code == 0 {
		t.Fatalf("catch-up on for an attached project: exit 0 (%s)", r.stdout)
	}
	if !strings.Contains(r.stderr, e.ConfigPath()) || !strings.Contains(r.stderr, "outside the repository") {
		t.Fatalf("error %q does not name the config %s outside the repository", r.stderr, e.ConfigPath())
	}
	if _, err := os.Stat(config.CatchUpPath(home)); !os.IsNotExist(err) {
		t.Fatalf("catch-up.json written by a refused on: %v", err)
	}
	if got := readFile(t, config.ProjectsPath(home)); !bytes.Equal(got, registry) {
		t.Fatalf("projects.json changed:\nbefore %s\nafter  %s", registry, got)
	}
	if got := mustCLI(t, home, "panel", "catch-up", "--project", root).stdout; got != "off\n" {
		t.Fatalf("print after the refusal: %q, want off", got)
	}
}

// [CATCHUP-1-S7] `panel remove` clears the root: a repository registered again at the
// same root starts with catch-up off, and catch-up.json does not list it.
func TestPanelCatchUpClearedByRemove(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := catchUpRepo(t, "gamma")
	mustCLI(t, home, "panel", "add", "--project", root)
	mustCLI(t, home, "panel", "catch-up", "--project", root, "on")
	if got := mustCLI(t, home, "panel", "catch-up", "--project", root).stdout; got != "on\n" {
		t.Fatalf("print before remove: %q, want on", got)
	}

	mustCLI(t, home, "panel", "remove", root)
	mustCLI(t, home, "panel", "add", "--project", root)
	c, err := config.LoadCatchUp(home)
	if err != nil {
		t.Fatal(err)
	}
	if c.On(root) {
		t.Fatalf("catch-up.json still lists %s after remove and add: %q", root, c.Roots)
	}
	if got := mustCLI(t, home, "panel", "catch-up", "--project", root).stdout; got != "off\n" {
		t.Fatalf("print after remove and add: %q, want off", got)
	}
}

// When the panel's run ends with the rebuild's restart (OPS-118 D4), `clauductor panel
// --launchd` exits with the restart's status, 75, not the 1 main gives every error: the
// status is what tells the login agent to start the new binary.
func TestPanelExitsWithTheRestartsStatus(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	r := cliEnv(t, "", home, []string{"CLAUDUCTOR_CMD_HELPER_RESTART=1"}, "panel", "--launchd")
	if panel.ExitRestart != 75 || r.code != 75 {
		t.Fatalf("exit %d, want 75\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "exits 75") {
		t.Fatalf("stdout %q does not say the panel exits 75 for the login agent", r.stdout)
	}
}

// `panel config-check --config <file>` prints the config as this binary loads it, as
// one JSON object: its name, alerts.notify and run list, an empty list as []. A config
// that does not load exits non-zero with the load's error and prints nothing on stdout,
// and no --config is refused.
func TestPanelConfigCheckPrintsTheConfigAsItLoads(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	card := write("card.json", `{"name":"T","lanes":{"main":"orchestrator"},"alerts":{"notify":false},"tmux_socket":"x-sock",`+
		`"cards":[{"id":"x","title":"x","command":["sh",".claude/x.sh"],"refresh":"interval:1"}]}`)
	var chk config.Check
	if err := json.Unmarshal([]byte(mustCLI(t, home, "panel", "config-check", "--config", card).stdout), &chk); err != nil {
		t.Fatal(err)
	}
	if chk.Name != "T" || chk.Notify || len(chk.Runs) != 1 || !strings.Contains(chk.Runs[0], ".claude/x.sh") || chk.Socket != "x-sock" {
		t.Fatalf("config-check of a card config: %+v, want T, notify false, its one card and its socket", chk)
	}

	plain := write("plain.json", `{"name":"E","lanes":{"main":"orchestrator"}}`)
	if got := mustCLI(t, home, "panel", "config-check", "--config", plain).stdout; got != `{"name":"E","notify":true,"runs":[],"tmux_socket":""}`+"\n" {
		t.Fatalf("config-check of a config that runs nothing: %q", got)
	}

	bad := write("bad.json", `{"version":999,"name":"B","lanes":{"main":"orchestrator"}}`)
	if r := cli(t, home, "panel", "config-check", "--config", bad); r.code == 0 || r.stdout != "" || !strings.Contains(r.stderr, "Error:") {
		t.Fatalf("config-check of a config that does not load: exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	if r := cli(t, home, "panel", "config-check"); r.code == 0 || !strings.Contains(r.stderr, "--config") {
		t.Fatalf("config-check with no --config: exit %d stderr %q", r.code, r.stderr)
	}
}
