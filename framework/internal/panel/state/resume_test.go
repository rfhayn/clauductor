package state

import (
	"fmt"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-20: a lane the usage limit stopped is due its resume line once, after the
// window that stopped it resets, and only while claude is idle by a current reading
// and waits on nothing.
func TestResumeCandidates(t *testing.T) {
	m := alertModel(t, "")
	const sid = "s-x"
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{{ID: "x", SessionID: sid, Path: "/repo/w/x", Type: "build", Created: t0.UnixMilli()}}, "", nil, t0)
	reset := t0.Add(2 * time.Hour).Truncate(time.Second)
	post := func(at time.Time, resets time.Time) {
		p, err := signals.ParseStatus([]byte(fmt.Sprintf(`{"session_id":"q","cwd":"/elsewhere","rate_limits":{"five_hour":{"used_percentage":100,"resets_at":%d}}}`, resets.Unix())))
		if err != nil {
			t.Fatal(err)
		}
		m.ApplyStatus(p, at)
	}
	idle := func(at time.Time) {
		m.ApplyAgents([]signals.Agent{{PID: 1, Cwd: "/repo/w/x", SessionID: sid, Status: "idle"}}, nil, at)
	}
	post(t0, reset)
	m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: "/repo/w/x", Event: "StopFailure", ErrorType: "rate_limit"}, t0)
	idle(t0)
	if c := m.ResumeCandidates(t0.Add(time.Hour)); len(c) != 0 {
		t.Fatalf("due before the reset: %+v", c)
	}
	// After the reset the status line reports the next window: its reset time is not
	// taken for the one that stopped the lane.
	after := reset.Add(time.Minute)
	post(after, reset.Add(5*time.Hour))
	idle(after)
	c := m.ResumeCandidates(after)
	if len(c) != 1 || c[0].Lane != "x" || !c[0].Reset.Equal(reset) {
		t.Fatalf("after the reset: %+v", c)
	}
	if why := m.ResumeStillDue(sid, after); why != "" {
		t.Fatalf("still due: %q", why)
	}
	m.MarkResumed(c[0], "continue", "", after)
	if c := m.ResumeCandidates(after.Add(time.Minute)); len(c) != 0 {
		t.Fatal("resumed twice for one stop")
	}
	if v := m.Snapshot(after); v.Feed[0].Event != "Auto-resume" || v.Feed[0].Name != "x" {
		t.Fatalf("feed %+v", v.Feed[0])
	}
	// Waiting on a permission: never.
	m2 := alertModel(t, "")
	m2.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{{ID: "x", SessionID: sid, Path: "/repo/w/x", Type: "build", Created: t0.UnixMilli()}}, "", nil, t0)
	m = m2
	post(t0, reset)
	m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: "/repo/w/x", Event: "StopFailure", ErrorType: "rate_limit"}, t0)
	m.ApplyAgents([]signals.Agent{{PID: 1, Cwd: "/repo/w/x", SessionID: sid, Status: "waiting", WaitingFor: "permission: Bash"}}, nil, after)
	if c := m.ResumeCandidates(after); len(c) != 0 {
		t.Fatalf("typed into a waiting lane: %+v", c)
	}
}
