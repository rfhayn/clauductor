package cmd

import (
	"os"
	"os/signal"
	"slices"
	"syscall"
	"testing"
)

// A closed terminal's hangup stops the panel through its shutdown, since its claude
// calls, in sessions of their own, no longer get the hangup themselves (#143); under
// nohup the hangup stays ignored.
func TestPanelStopsOnAHangupUnlessNohupIgnoresIt(t *testing.T) {
	if signal.Ignored(syscall.SIGHUP) {
		t.Skip("this test process was started with SIGHUP ignored (nohup), so the hangup case cannot be set up")
	}
	has := func(sigs []os.Signal, s os.Signal) bool { return slices.Contains(sigs, s) }
	sigs := panelStopSignals()
	if !has(sigs, syscall.SIGHUP) || !has(sigs, os.Interrupt) || !has(sigs, syscall.SIGTERM) {
		t.Fatalf("stop signals %v, want Ctrl-C, SIGTERM and the hangup", sigs)
	}
	signal.Ignore(syscall.SIGHUP) // as nohup leaves it
	t.Cleanup(func() { signal.Reset(syscall.SIGHUP) })
	sigs = panelStopSignals()
	if has(sigs, syscall.SIGHUP) || !has(sigs, os.Interrupt) || !has(sigs, syscall.SIGTERM) {
		t.Fatalf("stop signals with SIGHUP ignored %v, want Ctrl-C and SIGTERM alone", sigs)
	}
}
