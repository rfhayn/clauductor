package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/testwait"
)

// hangupRun is the panel's run in the hangup test's helper: it sends its own process
// the hangup and reports what stopped it, the hangup within window or the SIGTERM it
// sends after. A hangup with no handler ends the process instead, with no report.
func hangupRun(ctx context.Context, window time.Duration) error {
	_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	select {
	case <-ctx.Done():
		fmt.Println("stopped by the hangup")
		return nil
	case <-time.After(window):
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	<-ctx.Done()
	fmt.Println("stopped by SIGTERM")
	return nil
}

// #162: the production wiring (panel.go's signal.NotifyContext over
// panelStopSignals(signal.Ignored)), run as `clauductor panel` itself with its run
// stubbed. From a terminal the hangup stops the run through its shutdown; under nohup,
// which leaves the hangup ignored for the process it starts, it does not, and SIGTERM
// still does. Under nohup the window is short: a wiring that took the hangup would
// stop the run well inside it, and a slow machine can only make that pass, never fail.
func TestPanelHangupWiringFromATerminalAndUnderNohup(t *testing.T) {
	t.Parallel()
	run := func(nohup bool, window time.Duration) cliResult {
		t.Helper()
		b, _ := json.Marshal([]string{"panel", "--launchd"})
		argv := []string{os.Args[0], "-test.run=^$"}
		if nohup {
			argv = append([]string{"nohup"}, argv...)
		}
		c := exec.Command(argv[0], argv[1:]...)
		c.Env = append(gitFreeEnv(), "CLAUDUCTOR_CMD_HELPER_ARGS="+string(b), "HOME="+t.TempDir(), "CLAUDUCTOR_LANE=",
			quickExit(), "CLAUDUCTOR_CMD_HELPER_HANGUP="+window.String())
		var stdout, stderr strings.Builder
		c.Stdout, c.Stderr = &stdout, &stderr
		err := c.Run()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode() // -1 when a signal ended it
		} else if err != nil {
			t.Fatal(err)
		}
		return cliResult{strings.TrimSpace(stdout.String()), stderr.String(), code}
	}
	// A test run that ignores the hangup itself (`nohup go test`) passes the ignored state
	// on to the helper, which is then no terminal's.
	if signal.Ignored(syscall.SIGHUP) {
		t.Log("this test run ignores the hangup: the case from a terminal is not run")
	} else if r := run(false, testwait.Scale(30*time.Second)); r.code != 0 || r.stdout != "stopped by the hangup" {
		t.Errorf("from a terminal: exit %d, %q (stderr %q), want the hangup to stop the run", r.code, r.stdout, r.stderr)
	}
	if r := run(true, 500*time.Millisecond); r.code != 0 || r.stdout != "stopped by SIGTERM" {
		t.Errorf("under nohup: exit %d, %q (stderr %q), want the hangup ignored and SIGTERM to stop the run", r.code, r.stdout, r.stderr)
	}
}

// A closed terminal's hangup stops the panel through its shutdown, since its claude
// calls, in sessions of their own, no longer get the hangup themselves (#143); under
// nohup the hangup stays ignored. The ignored state is the test's own: setting the
// process's (signal.Ignore) cannot be undone, and every later test would inherit it.
func TestPanelStopsOnAHangupUnlessNohupIgnoresIt(t *testing.T) {
	has := func(sigs []os.Signal, s os.Signal) bool { return slices.Contains(sigs, s) }
	sigs := panelStopSignals(func(os.Signal) bool { return false })
	if !has(sigs, syscall.SIGHUP) || !has(sigs, os.Interrupt) || !has(sigs, syscall.SIGTERM) {
		t.Fatalf("stop signals %v, want Ctrl-C, SIGTERM and the hangup", sigs)
	}
	nohup := func(s os.Signal) bool { return s == syscall.SIGHUP }
	sigs = panelStopSignals(nohup)
	if has(sigs, syscall.SIGHUP) || !has(sigs, os.Interrupt) || !has(sigs, syscall.SIGTERM) {
		t.Fatalf("stop signals with SIGHUP ignored %v, want Ctrl-C and SIGTERM alone", sigs)
	}
}
