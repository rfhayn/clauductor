package lanes

import (
	"context"
	"slices"
	"testing"
)

// PANEL-19: a lane starts with --remote-control only in lanes mode, before -n, and a
// restart keeps it; otherwise the flag never appears.
func TestLaneCommandRemoteControl(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	if got := m.LaneCommand("add-x", "build", sid, false); slices.Contains(got, "--remote-control") {
		t.Fatalf("no mode: %q", got)
	}
	on := false
	m.RemoteControl = func() bool { return on }
	if got := m.LaneCommand("add-x", "build", sid, false); slices.Contains(got, "--remote-control") {
		t.Fatalf("off: %q", got)
	}
	on = true
	for _, resume := range []bool{false, true} {
		got := m.LaneCommand("add-x", "build", sid, resume)
		i := slices.Index(got, "--remote-control")
		if i < 0 || got[i+1] != "-n" || got[i+2] != "add-x" {
			t.Fatalf("lanes mode (resume %v): %q", resume, got)
		}
	}
}

// Remote control types nothing unless lanes mode is on and the lane is registered.
func TestConnectRemoteRefuses(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	if e := m.ConnectRemote(context.Background(), "add-x"); e == nil || e.Code != "remote-off" {
		t.Fatalf("not in lanes mode: %+v", e)
	}
	m.RemoteControl = func() bool { return true }
	if e := m.ConnectRemote(context.Background(), "BAD id"); e == nil || e.Code != "invalid" {
		t.Fatalf("bad id: %+v", e)
	}
	if e := m.ConnectRemote(context.Background(), "nope"); e == nil || e.Code != "not-found" {
		t.Fatalf("unregistered: %+v", e)
	}
}
