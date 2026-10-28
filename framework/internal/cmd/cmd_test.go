package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/clauductor/clauductor/internal/leakcheck"
	"github.com/clauductor/clauductor/internal/panel"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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
		if os.Getenv("CLAUDUCTOR_CMD_HELPER_RESTART") != "" {
			// The panel's run as it ends after rebuilding itself (OPS-118 D4), under the
			// login agent only: what `clauductor panel` does with that is the test.
			runPanel = func(_ context.Context, o panel.Options) error {
				if !o.Launchd {
					return errors.New("the run was not told it is the login agent")
				}
				return &panel.RestartError{Code: panel.ExitRestart, From: strings.Repeat("a", 40), To: strings.Repeat("b", 40)}
			}
		}
		if w := os.Getenv("CLAUDUCTOR_CMD_HELPER_HANGUP"); w != "" {
			// A closed terminal's hangup, through `clauductor panel`'s own stop signals
			// (#162): what stops the run is the test (hangupRun).
			window, _ := time.ParseDuration(w)
			runPanel = func(ctx context.Context, _ panel.Options) error { return hangupRun(ctx, window) }
		}
		os.Exit(Main(os.Stderr)) // as main does
	}
	os.Exit(leakcheck.Main(m)) // fails the run if a helper process outlives it
}

type cliResult struct {
	stdout, stderr string
	code           int
}

// cli runs `clauductor args...` with HOME set to home: nothing reaches the real
// ~/.claude or ~/.clauductor, or a panel running on this machine.
func cli(t *testing.T, home string, args ...string) cliResult {
	t.Helper()
	return cliIn(t, "", home, args...)
}

// cliIn is cli run from dir ("" for the test's own directory).
func cliIn(t *testing.T, dir, home string, args ...string) cliResult {
	t.Helper()
	return cliEnv(t, dir, home, nil, args...)
}

// cliEnv is cliIn with env added to the command's environment.
func cliEnv(t *testing.T, dir, home string, env []string, args ...string) cliResult {
	t.Helper()
	b, _ := json.Marshal(args)
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Dir = dir
	c.Env = append(append(gitFreeEnv(), "CLAUDUCTOR_CMD_HELPER_ARGS="+string(b), "HOME="+home, "CLAUDUCTOR_LANE=", quickExit()), env...)
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

// gitFreeEnv is this process's environment without the variables that point git at
// a repository: run under a git hook, they would aim a test's git calls at the outer
// repository instead of its temporary one.
func gitFreeEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "GIT_DIR" && k != "GIT_WORK_TREE" && k != "GIT_INDEX_FILE" {
			env = append(env, kv)
		}
	}
	return env
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
		{"install with no project registered", []string{"panel", "install"}, "no project registered"},
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
	if r.code != 0 || !strings.Contains(r.stdout, "Trusted panel config ") || !strings.Contains(r.stdout, "(sha256 ") ||
		!strings.Contains(r.stdout, "It runs:") {
		t.Fatalf("trust: exit %d, %q %q", r.code, r.stdout, r.stderr)
	}
}

// quickExit is the GORACE setting for a helper process this test binary starts as
// a child: a -race binary sleeps a second at exit (atexit_sleep_ms) to flush race
// reports, and a helper's stderr is its test's to read, not a report's.
func quickExit() string {
	return "GORACE=" + strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0")
}

// A failing command prints its error once (#251): Main prints it, and cobra does not print it
// first. A command's own failure prints no usage; a mistyped flag still does.
func TestFailingCommandPrintsItsErrorOnce(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	for _, c := range []struct {
		name  string
		args  []string
		usage bool
	}{
		{"a command's refusal", []string{"install"}, false},
		{"an unknown flag", []string{"install", "--bogus"}, true},
		{"an unknown command", []string{"nope"}, false},
	} {
		r := cliIn(t, t.TempDir(), home, c.args...)
		if r.code != 1 {
			t.Errorf("%s: exit %d, want 1\n%s", c.name, r.code, r.stderr)
		}
		if n := strings.Count(r.stderr, "Error: "); n != 1 {
			t.Errorf("%s: %d Error: lines, want 1:\n%s", c.name, n, r.stderr)
		}
		if got := strings.Contains(r.stderr, "Usage:"); got != c.usage {
			t.Errorf("%s: usage printed %v, want %v:\n%s", c.name, got, c.usage, r.stderr)
		}
	}
	if r := cliIn(t, t.TempDir(), home, "nope"); !strings.Contains(r.stderr, "Run 'clauductor --help' for usage.") {
		t.Errorf("an unknown command does not point at --help:\n%s", r.stderr)
	}
	// cobra.NoArgs refuses an argument in the same words as an unknown command, but cobra printed
	// the pointer only for a command it could not find: the subcommand's own usage is already
	// there, and the root's help is not the one to read (#269).
	if r := cliIn(t, t.TempDir(), home, "doctor", "extra"); strings.Contains(r.stderr, "--help") {
		t.Errorf("an extra argument points at --help:\n%s", r.stderr)
	}
}

// The root's PersistentPreRun silences usage for a command's own failure. Cobra runs only the
// nearest one (or each from the root down under cobra.EnableTraverseRunHooks), so a subcommand's
// own could bring usage back for its subtree. What is held is the outcome, not the shape: once a
// command's arguments pass, the hooks cobra runs have silenced its usage (#269).
func TestUsageIsSilencedOnceTheArgumentsPass(t *testing.T) {
	for _, path := range usageStaysOn(rootCmd, cobra.EnableTraverseRunHooks) {
		t.Errorf("%s would print usage for its own failure: a PersistentPreRun replaces the root's without silencing it", path)
	}

	// The judge itself, on trees of its own: a subcommand hook that silences usage passes, one that
	// does not fails, unless traverse hooks run the root's as well.
	silence := func(c *cobra.Command, _ []string) { c.SilenceUsage = true }
	tree := func(sub func(*cobra.Command, []string)) *cobra.Command {
		root := &cobra.Command{Use: "r", PersistentPreRun: silence}
		mid := &cobra.Command{Use: "m", PersistentPreRun: sub}
		mid.AddCommand(&cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}})
		root.AddCommand(mid)
		return root
	}
	for _, c := range []struct {
		name     string
		sub      func(*cobra.Command, []string)
		traverse bool
		want     []string
	}{
		{"a subcommand hook that silences usage", silence, false, nil},
		{"a subcommand hook that does not", func(*cobra.Command, []string) {}, false, []string{"r m leaf"}},
		{"the same under traverse hooks", func(*cobra.Command, []string) {}, true, nil},
		{"no subcommand hook", nil, false, nil},
	} {
		if got := usageStaysOn(tree(c.sub), c.traverse); !slices.Equal(got, c.want) {
			t.Errorf("%s: usage stays on for %q, want %q", c.name, got, c.want)
		}
	}
}

// usageStaysOn names each runnable command under root whose usage cobra would still print for its
// own failure: the PersistentPreRun hooks cobra runs once its arguments pass, the nearest one or
// (traverse) each from the root down, leave SilenceUsage off. The hooks run here as cobra would
// run them; each command's SilenceUsage is put back afterwards.
func usageStaysOn(root *cobra.Command, traverse bool) []string {
	var bad []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if !c.Runnable() {
			return
		}
		var chain []*cobra.Command
		for p := c; p != nil; p = p.Parent() {
			chain = append([]*cobra.Command{p}, chain...)
		}
		if !traverse {
			slices.Reverse(chain) // nearest first, and only the first that has a hook runs
		}
		saved := map[*cobra.Command]bool{}
		for _, p := range chain {
			saved[p] = p.SilenceUsage
		}
		for _, p := range chain {
			ran := true
			switch {
			case p.PersistentPreRunE != nil:
				_ = p.PersistentPreRunE(c, nil)
			case p.PersistentPreRun != nil:
				p.PersistentPreRun(c, nil)
			default:
				ran = false
			}
			if ran && !traverse {
				break
			}
		}
		if !c.SilenceUsage && !root.SilenceUsage {
			bad = append(bad, c.CommandPath())
		}
		for p, s := range saved {
			p.SilenceUsage = s
		}
	}
	walk(root)
	return bad
}
