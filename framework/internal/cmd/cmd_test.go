package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary act as the clauductor CLI (as cmd/clauductor/main
// runs it), so a test sees what a shell sees: the exit status, stdout and stderr.
// lock-run leaves by os.Exit, which only a separate process can observe.
func TestMain(m *testing.M) {
	if args := os.Getenv("CLAUDUCTOR_CMD_HELPER_ARGS"); args != "" {
		var argv []string
		if err := json.Unmarshal([]byte(args), &argv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Args = append([]string{"clauductor"}, argv...)
		if err := Execute(); err != nil { // as main does
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliResult struct {
	stdout, stderr string
	code           int
}

// cli runs `clauductor args...` with HOME set to home: nothing reaches the real
// ~/.claude or ~/.clauductor, or a panel running on this machine.
func cli(t *testing.T, home string, args ...string) cliResult {
	t.Helper()
	b, _ := json.Marshal(args)
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "CLAUDUCTOR_CMD_HELPER_ARGS="+string(b), "HOME="+home, "CLAUDUCTOR_LANE=", quickExit())
	var stdout, stderr strings.Builder
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return cliResult{stdout.String(), stderr.String(), code}
}

func skipShortLockRun(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs a command under a real lease")
	}
}

// ---- lock-run ----

func TestLockRunExitsWithTheCommandsStatus(t *testing.T) {
	t.Parallel()
	skipShortLockRun(t)
	dir := t.TempDir()
	lock := filepath.Join(dir, "gate.lock")
	r := cli(t, t.TempDir(), "lock-run", lock, "--", "/bin/sh", "-c", "echo ran; exit 7")
	if r.code != 7 || strings.TrimSpace(r.stdout) != "ran" {
		t.Fatalf("exit %d, stdout %q (stderr %s); want the command's 7 and its output", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("the lease was not released")
	}
	// A command ended by a signal: 128 + the signal, as a shell reports it.
	r = cli(t, t.TempDir(), "lock-run", lock, "--", "/bin/sh", "-c", "kill -TERM $$")
	if r.code != 128+15 {
		t.Fatalf("a command killed by SIGTERM: exit %d, want 143 (stderr %s)", r.code, r.stderr)
	}
}

// --lane and --ttl reach the lease the command runs under: the command reads its own
// owner.json through CLAUDUCTOR_LOCK_HELD.
func TestLockRunPutsLaneAndTTLInTheLease(t *testing.T) {
	t.Parallel()
	skipShortLockRun(t)
	lock := filepath.Join(t.TempDir(), "gate.lock")
	r := cli(t, t.TempDir(), "lock-run", "--lane", "fix-7", "--ttl", "90s", lock, "--",
		"/bin/sh", "-c", `cat "$CLAUDUCTOR_LOCK_HELD/owner.json"`)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	var o struct {
		Lane string `json:"lane"`
		TTL  int64  `json:"ttl"`
		PID  int    `json:"pid"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil {
		t.Fatalf("owner.json %q: %v", r.stdout, err)
	}
	if o.Lane != "fix-7" || o.TTL != 90 || o.PID == 0 {
		t.Fatalf("lease %+v, want lane fix-7, ttl 90 s and the holder's pid", o)
	}
}

// Everything before `--` is lock-run's; exactly one lock directory comes before it.
// Anything else is a usage error, and no command runs.
func TestLockRunRefusesMisplacedArguments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	touch := []string{"/usr/bin/touch", marker}
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no --", append([]string{"lock-run", filepath.Join(dir, "l")}, touch...), "usage: clauductor lock-run"},
		{"two locks", append([]string{"lock-run", filepath.Join(dir, "l"), "extra", "--"}, touch...), "usage: clauductor lock-run"},
		{"no lock", append([]string{"lock-run", "--"}, touch...), "usage: clauductor lock-run"},
		{"no command", []string{"lock-run", filepath.Join(dir, "l")}, "requires at least 2 arg(s)"},
		{"bad ttl", append([]string{"lock-run", "--ttl", "soon", filepath.Join(dir, "l"), "--"}, touch...), `invalid argument "soon"`},
	} {
		r := cli(t, t.TempDir(), c.args...)
		if r.code != 1 || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%s: exit %d, stderr %q; want 1 and %q", c.name, r.code, r.stderr, c.want)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%s: the command ran", c.name)
		}
	}
}

// ---- panel ----

func TestPanelFlagDefaults(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		cmd       string
		flag, def string
		hidden    bool
	}{
		{"panel", "port", "4393", false},
		{"panel", "no-open", "false", false},
		{"panel", "launchd", "false", true}, // set only by the login agent's plist
		{"panel", "trust-config", "false", false},
		{"panel install", "port", "4393", false},
		{"panel open", "port", "0", false}, // 0: the running panel's marker, else 4393
		{"lock-run", "ttl", "10m0s", false},
	} {
		cmd, _, err := rootCmd.Find(strings.Fields(c.cmd))
		if err != nil {
			t.Fatal(err)
		}
		f := cmd.Flags().Lookup(c.flag)
		if f == nil || f.DefValue != c.def || f.Hidden != c.hidden {
			t.Errorf("%s --%s: %+v; want default %s, hidden %v", c.cmd, c.flag, f, c.def, c.hidden)
		}
	}
}

// A usage mistake exits 1 and runs nothing; the home stays untouched.
func TestPanelRefusesBadInvocations(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"install without --project", []string{"panel", "install"}, `required flag(s) "project" not set`},
		{"a stray argument", []string{"panel", "stray"}, "stray"},
		{"a port that is not a number", []string{"panel", "--port", "http"}, `invalid argument "http"`},
		{"an unknown command", []string{"no-such-command"}, "unknown command"},
	} {
		home := t.TempDir()
		r := cli(t, home, c.args...)
		if r.code != 1 || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%s: exit %d, stderr %q; want 1 and %q", c.name, r.code, r.stderr, c.want)
		}
		if entries, _ := os.ReadDir(home); len(entries) != 0 {
			t.Errorf("%s: wrote into the home: %v", c.name, entries)
		}
	}
}

// A project without panel.json is refused before the panel touches the hooks, the
// port or the marker files.
func TestPanelWithoutAConfigChangesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r := cli(t, home, "panel", "--project", t.TempDir(), "--no-open", "--port", "0")
	if r.code != 1 || !strings.Contains(r.stderr, "no panel config") {
		t.Fatalf("exit %d, stderr %q; want 1 and the missing config", r.code, r.stderr)
	}
	for _, p := range []string{".claude", ".clauductor"} {
		if _, err := os.Stat(filepath.Join(home, p)); !os.IsNotExist(err) {
			t.Errorf("a refused start created ~/%s", p)
		}
	}
}

func TestPanelHousekeepingCommands(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r := cli(t, home, "panel", "--uninstall-hooks")
	if r.code != 0 || !strings.Contains(r.stdout, "No panel hooks in "+filepath.Join(home, ".claude", "settings.json")) {
		t.Fatalf("--uninstall-hooks with none installed: exit %d, %q %q", r.code, r.stdout, r.stderr)
	}
	r = cli(t, home, "panel", "rotate-token")
	token := filepath.Join(home, ".clauductor", "panel", "token")
	fi, err := os.Stat(token)
	if r.code != 0 || err != nil || fi.Mode().Perm() != 0o600 || !strings.Contains(r.stdout, "Rotated "+token) {
		t.Fatalf("rotate-token: exit %d, %q %q, token %v %v", r.code, r.stdout, r.stderr, fi, err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".clauductor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".clauductor", "panel.json"), []byte(`{"name":"P"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r = cli(t, home, "panel", "trust", "--project", project)
	if r.code != 0 || !strings.Contains(r.stdout, "Trusted panel config (sha256 ") {
		t.Fatalf("trust: exit %d, %q %q", r.code, r.stdout, r.stderr)
	}
}

// quickExit is the GORACE setting for a helper process this test binary starts as
// a child: a -race binary sleeps a second at exit (atexit_sleep_ms) to flush race
// reports, and a helper's stderr is its test's to read, not a report's.
func quickExit() string {
	return "GORACE=" + strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0")
}
