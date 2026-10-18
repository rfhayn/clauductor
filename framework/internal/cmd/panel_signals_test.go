package cmd

import (
	"os"
	"slices"
	"syscall"
	"testing"
)

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
