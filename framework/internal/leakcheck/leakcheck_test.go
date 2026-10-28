package leakcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testgit"
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
	testgit.Strip() // a git hook's GIT_DIR reaches no process these tests start (#302)
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
	c := exec.Command(os.Args[0], "-test.run=^TestHelperLaunchesClaude$", "-test.timeout=30s")
	// No claude on this PATH but the guard's: a regressed Main reaches none.
	c.Env = append(os.Environ(), "LEAKCHECK_HELPER_MAIN=1", "PATH=/usr/bin:/bin")
	var stderr strings.Builder
	c.Stderr = &stderr
	err := c.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("the run: %v, want exit status 1\n%s", err, stderr.String())
	}
	if want := "leakcheck: a test launched the real claude (#93): claude probe;"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr does not report the call (%q):\n%s", want, stderr.String())
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
