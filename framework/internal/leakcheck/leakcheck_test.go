package leakcheck

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testwait"
)

// The guard is the claude a test meets on PATH (#93): it refuses, says why, and
// records each call as it was made. The quote in the directory's name is in the path
// the script writes to.
func TestGuardClaudeRefusesAndRecords(t *testing.T) {
	// A decoy claude stands where a real one would be on PATH: unless guardClaude puts
	// its own FIRST (not at all, or after the decoy: #93's failure mode), the decoy
	// answers with exit 7, the assertions below fail, and no real claude is reached.
	decoy := t.TempDir()
	testbin.Write(t, filepath.Join(decoy, "claude"), "#!/bin/sh\nexit 7\n")
	t.Setenv("PATH", decoy+":/usr/bin:/bin")
	mark := filepath.Join(t.TempDir(), "it's")
	if err := os.Mkdir(mark, 0o755); err != nil {
		t.Fatal(err)
	}
	calls, err := guardClaude(mark)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"x y"}, {`a\cb`}, {"-n"}, {}} {
		var stderr strings.Builder
		c := exec.Command("claude", args...)
		c.Stderr = &stderr
		err = c.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			t.Fatalf("claude %q: %v, want exit status 1", args, err)
		}
		if !strings.Contains(stderr.String(), "a test launched the real claude (#93)") {
			t.Fatalf("claude %q: stderr %q does not name #93", args, stderr.String())
		}
	}
	if b, _ := os.ReadFile(calls); string(b) != "x y\n"+`a\cb`+"\n-n\n\n" {
		t.Fatalf("calls file %q, want one line per call, as made", b)
	}
}

// TestMain runs the package's tests plainly. With LEAKCHECK_HELPER_MAIN=1 it is a
// package of the panel's kind instead: Main around a test that launches claude.
func TestMain(m *testing.M) {
	if os.Getenv("LEAKCHECK_HELPER_MAIN") == "1" {
		os.Exit(Main(m))
	}
	os.Exit(m.Run())
}

// TestHelperLaunchesClaude is the test Main fails the run for, when it runs as a helper.
func TestHelperLaunchesClaude(t *testing.T) {
	if os.Getenv("LEAKCHECK_HELPER_MAIN") != "1" {
		t.Skip("run by TestMainFailsARunThatLaunchedClaude")
	}
	_ = exec.Command("claude", "probe").Run()
}

// Main acts on the guard's record (#93): a run whose test launched claude exits 1 and
// names the call, though the test itself passed.
func TestMainFailsARunThatLaunchedClaude(t *testing.T) {
	// No claude on this PATH but the guard's: a regressed Main reaches none.
	code, stderr := helperMain(t, "^TestHelperLaunchesClaude$", nil, "PATH=/usr/bin:/bin")
	if code != 1 {
		t.Fatalf("the run: exit %d, want 1\n%s", code, stderr)
	}
	if want := "leakcheck: a test launched the real claude (#93): claude probe;"; !strings.Contains(stderr, want) {
		t.Fatalf("stderr does not report the call (%q):\n%s", want, stderr)
	}
}

// TestHelperBlocks is a long-lived copy of this test binary: it says "ready", then
// runs until its stdin closes.
func TestHelperBlocks(t *testing.T) {
	if os.Getenv("LEAKCHECK_BLOCK") != "1" {
		t.Skip("run by the process-listing tests")
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// startBlocker starts c (a TestHelperBlocks, or a command that runs one) and returns
// once the helper has said "ready".
func startBlocker(t *testing.T, c *exec.Cmd) {
	t.Helper()
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(out).ReadString('\n'); line != "ready\n" {
		t.Fatalf("the helper said %q (%v), want ready", line, err)
	}
}

// needPS fails the test where ps cannot list processes (a sandbox): Main finds no
// helper there, so a test of what it finds would prove nothing.
func needPS(t *testing.T) {
	t.Helper()
	if err := exec.Command("ps", "-A", "-o", "pid=").Run(); err != nil {
		t.Fatalf("ps cannot list processes here (%v): leakcheck finds no helper without it", err)
	}
}

// helperMain runs this binary as a package of the panel's kind (Main around it) with
// the test run selects, env added, and returns its exit code and stderr.
func helperMain(t *testing.T, run string, stdin io.Reader, env ...string) (int, string) {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run="+run, "-test.timeout=60s")
	c.Env = append(append(os.Environ(), "LEAKCHECK_HELPER_MAIN=1"), env...)
	c.Stdin = stdin
	var stderr strings.Builder
	c.Stderr = &stderr
	err := c.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), stderr.String()
	} else if err != nil {
		t.Fatal(err)
	}
	return 0, stderr.String()
}

// #319: two runs of one binary path (two lanes, two scripts) are strangers. A run's
// Main neither reports nor kills a copy of its binary it did not start: before the
// fix it killed it, as it did OPS-99's measured run.
func TestMainLeavesAnotherRunsCopyAlone(t *testing.T) {
	needPS(t)
	other := exec.Command(os.Args[0], "-test.run=^TestHelperBlocks$")
	other.Env = append(os.Environ(), "LEAKCHECK_BLOCK=1")
	stop, err := other.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	startBlocker(t, other)
	waited := false
	t.Cleanup(func() {
		if !waited {
			stop.Close()
			other.Wait()
		}
	})

	code, stderr := helperMain(t, "^$", nil)
	if code != 0 || strings.Contains(stderr, "helper process") {
		t.Errorf("the run: exit %d, want 0 and no helper reported:\n%s", code, stderr)
	}
	stop.Close()
	waited = true
	if err := other.Wait(); err != nil {
		t.Fatalf("the other run's copy: %v, want it to end at its stdin's close, untouched", err)
	}
}

// TestHelperLeaksAHelper leaves a helper behind for Main to find: a shell that runs
// this binary again, both on the run's stdin, so only a kill ends them while the
// caller keeps that open.
func TestHelperLeaksAHelper(t *testing.T) {
	if os.Getenv("LEAKCHECK_HELPER_MAIN") != "1" {
		t.Skip("run by TestMainKillsAHelperItLeft")
	}
	c := exec.Command("/bin/sh", "-c", `"$0" -test.run='^TestHelperBlocks$'; :`, os.Args[0])
	c.Env = append(os.Environ(), "LEAKCHECK_HELPER_MAIN=0", "LEAKCHECK_BLOCK=1")
	c.Stdin = os.Stdin
	startBlocker(t, c)
}

// The detection the fix keeps: a helper this run started and left, its child and the
// binary run again below it, is reported and killed, and fails the run.
func TestMainKillsAHelperItLeft(t *testing.T) {
	needPS(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close() // the helpers' stdin: a helper Main did not kill ends here
	code, stderr := helperMain(t, "^TestHelperLeaksAHelper$", r)
	r.Close()
	if code != 1 {
		t.Errorf("the run: exit %d, want 1:\n%s", code, stderr)
	}
	const report = "leakcheck: helper process still running after the tests: "
	var pids []int
	for _, want := range []string{" /bin/sh -c ", " " + os.Args[0] + " -test.run=^TestHelperBlocks$"} {
		found := false
		for _, line := range strings.Split(stderr, "\n") {
			rest, ok := strings.CutPrefix(line, report)
			if !ok || !strings.Contains(rest, want) {
				continue
			}
			pid, err := strconv.Atoi(strings.Fields(rest)[0])
			if err != nil {
				t.Fatalf("report line %q: %v", line, err)
			}
			pids, found = append(pids, pid), true
		}
		if !found {
			t.Errorf("no report of the helper running %q:\n%s", want, stderr)
		}
	}
	// Killed is gone, reaped or not: a zombie waits on whoever adopted it.
	for _, pid := range pids {
		testwait.For(t, "leaked helper "+strconv.Itoa(pid)+" to be gone", 10*time.Second, func() bool {
			out, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
			stat := strings.TrimSpace(string(out))
			return stat == "" || strings.HasPrefix(stat, "Z")
		})
	}
}

// leftovers over a fixed listing: this run is pid 100, its parent (go test, which runs
// the same binary) is 90, and 200 is another run of the binary under its own parent.
// 106 names an earlier run's mark (the same pid, a longer suffix); 400 is a helper of
// this run's whose parent exited, a known limit (processes): neither is listed.
func TestLeftoversListsOnlyWhatThisRunStarted(t *testing.T) {
	const bin, mark = "/b/x.test", "/tmp/leakcheck-100-7"
	ps := `
  1     0 02-03:00:00 /sbin/launchd
 90     1    05:00 /b/x.test -test.count=1
100    90    04:59 /b/x.test -test.run=^$
101   100    04:00 /bin/sleep 9
102   100    03:00 /bin/sh -c x
103   102    02:00 /b/x.test -test.run=Helper
104   102    02:00 /bin/cat
105     1    01:00 /usr/bin/tail -f /tmp/leakcheck-100-7/log
150     1    10:00 /bin/zsh
200   150    09:00 /b/x.test -test.count=50
201   200    08:00 /b/x.test -test.run=TheirHelper
110   300 1-00:00:00 /b/x.test -test.run=Reused
106     1    00:30 /usr/bin/tail -f /tmp/leakcheck-100-71234/log
107     1    00:30 /usr/bin/tail -f /tmp/leakcheck-100-71234/x /tmp/leakcheck-100-7
400     1    00:30 /b/x.test -test.run=Orphan
300   100    00:01 /bin/sh
120   100    00:00 ps -A -ww -o pid=
`
	got := leftovers(ps, 100, bin, mark)
	want := []string{
		"101 /bin/sleep 9",
		"102 /bin/sh -c x",
		"103 /b/x.test -test.run=Helper",
		"105 /usr/bin/tail -f /tmp/leakcheck-100-7/log",
		"107 /usr/bin/tail -f /tmp/leakcheck-100-71234/x /tmp/leakcheck-100-7",
		"300 /bin/sh",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("leftovers:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestElapsed(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"00:05":       5 * time.Second,
		"12:34":       12*time.Minute + 34*time.Second,
		"01:02:03":    time.Hour + 2*time.Minute + 3*time.Second,
		"2-03:04:05":  51*time.Hour + 4*time.Minute + 5*time.Second,
		"02-03:46:23": 51*time.Hour + 46*time.Minute + 23*time.Second,
	} {
		if got, ok := elapsed(in); !ok || got != want {
			t.Errorf("elapsed(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "5", "x:05", "1:2:3:4", "a-01:02"} {
		if _, ok := elapsed(in); ok {
			t.Errorf("elapsed(%q) read, want refused", in)
		}
	}
}

// A recorded call fails the run (Main's code), one report line per call; no file, or
// an empty one, is a clean run.
func TestReportClaudeCalls(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, content string
		lines         int
	}{
		{"missing", "", 0},
		{"empty", "", 0},
		{"two calls", "auth status --json\nagents\n", 2},
		{"two bare calls", "\n\n", 2},
	} {
		path := filepath.Join(dir, tc.name)
		if tc.name != "missing" {
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var out strings.Builder
		got := reportClaudeCalls(path, &out)
		if got != (tc.lines > 0) {
			t.Errorf("%s: reportClaudeCalls = %v, want %v", tc.name, got, tc.lines > 0)
		}
		if n := strings.Count(out.String(), "a test launched the real claude (#93)"); n != tc.lines {
			t.Errorf("%s: %d report lines naming #93, want %d:\n%s", tc.name, n, tc.lines, out.String())
		}
	}
}
