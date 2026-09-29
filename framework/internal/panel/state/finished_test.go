package state

import (
	"fmt"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-11: the lane workspace lists a lane's finished subagents under its running
// ones. A stop, and every clear (idle, gone, SessionEnd), retires an agent to that
// list, newest first, with when it ended; nothing else invents one.
func TestFinishedSubagentsAreListedNewestFirst(t *testing.T) {
	t.Parallel()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	hook := func(ev, id, typ string) signals.HookEvent {
		return signals.HookEvent{SessionID: "s", Cwd: cwd, Event: ev, AgentID: id, AgentType: typ}
	}
	lane := func(m *Model, at time.Time) LaneView {
		t.Helper()
		for _, l := range m.Snapshot(at).Lanes {
			if l.Path == cwd {
				return l
			}
		}
		t.Fatal("the lane is not in the view")
		return LaneView{}
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "busy", StartedAt: t0.Add(-time.Hour).UnixMilli()}}, nil, t0)
	m.ApplyHook(hook("SubagentStart", "a1", "Explore"), t0)
	m.ApplyHook(hook("SubagentStart", "a2", "reviewer"), t0.Add(time.Second))
	m.ApplyHook(hook("SubagentStart", "a3", "builder"), t0.Add(2*time.Second))
	m.ApplyHook(hook("SubagentStop", "a1", "Explore"), t0.Add(3*time.Second))

	l := lane(m, t0.Add(4*time.Second))
	if len(l.Subagents) != 2 || len(l.FinishedSubagents) != 1 {
		t.Fatalf("running %d, finished %d; want 2 and 1", len(l.Subagents), len(l.FinishedSubagents))
	}
	f := l.FinishedSubagents[0]
	if f.ID != "a1" || f.Type != "Explore" || f.Since != ms(t0) || f.Ended != ms(t0.Add(3*time.Second)) {
		t.Fatalf("finished %+v", f)
	}
	if len(l.Sessions) != 1 || l.Sessions[0].StartedAt != t0.Add(-time.Hour).UnixMilli() {
		t.Fatalf("the session's start time from claude agents is not in the view: %+v", l.Sessions)
	}
	if l.Head == "" {
		t.Error("the worktree's HEAD is not in the view")
	}

	// Going idle for 10 s retires the rest, oldest first, so a2 is older than a3 in
	// the list (newest first: a3, a2, then a1).
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(5*time.Second))
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(16*time.Second))
	l = lane(m, t0.Add(17*time.Second))
	var got []string
	for _, a := range l.FinishedSubagents {
		got = append(got, a.ID)
	}
	if len(l.Subagents) != 0 || fmt.Sprint(got) != "[a3 a2 a1]" {
		t.Fatalf("running %d, finished %v; want 0 and [a3 a2 a1]", len(l.Subagents), got)
	}

	// The list is capped: the oldest fall off.
	for i := 0; i < maxFinished+5; i++ {
		at := t0.Add(time.Minute + time.Duration(i)*time.Second)
		m.ApplyHook(hook("SubagentStart", fmt.Sprintf("x%02d", i), "builder"), at)
		m.ApplyHook(hook("SubagentStop", fmt.Sprintf("x%02d", i), "builder"), at)
	}
	l = lane(m, t0.Add(2*time.Minute))
	if len(l.FinishedSubagents) != maxFinished || l.FinishedSubagents[0].ID != fmt.Sprintf("x%02d", maxFinished+4) {
		t.Fatalf("finished %d, newest %q; want %d, x%02d", len(l.FinishedSubagents), l.FinishedSubagents[0].ID, maxFinished, maxFinished+4)
	}
}

// PANEL-11: the lane dashboard's figures come from the status-line post the panel
// already receives (the recorded one, from Claude Code 2.1.284), summed per lane.
func TestMetricsFromTheStatusLine(t *testing.T) {
	t.Parallel()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	p, err := signals.ParseStatus(fixture(t, "statusline.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "s1", Cwd: cwd, Status: "busy"}, {SessionID: "s2", Cwd: cwd, Status: "idle"}}, nil, t0)
	p.SessionID, p.Cwd = "s1", cwd
	m.ApplyStatus(p, t0.Add(time.Second))
	p.SessionID = "s2"
	m.ApplyStatus(p, t0.Add(2*time.Second))
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: cwd, Event: "SubagentStart", AgentID: "a1", AgentType: "reviewer"}, t0.Add(3*time.Second))

	var l *LaneView
	v := m.Snapshot(t0.Add(4 * time.Second))
	for i := range v.Lanes {
		if v.Lanes[i].Path == cwd {
			l = &v.Lanes[i]
		}
	}
	if l == nil || len(l.Sessions) != 2 {
		t.Fatalf("lane %+v", l)
	}
	s := l.Sessions[0].Metrics
	if s.CtxWindow != 200000 || s.InputTokens != 36983 || s.OutputTokens != 63 || s.CacheRead != 26394 || s.CacheWrite != 10579 ||
		s.DurationMs != 2878 || s.APIDurationMs != 4615 || s.CacheHitRatio == nil || *s.CacheHitRatio < 0.71 || s.CacheExpires != 1790644359000 ||
		s.Thinking == nil || !*s.Thinking || s.OutputStyle != "default" || s.CostUSD == nil || s.SubagentsRunning != 1 || s.State != "busy" || s.StateSince != ms(t0) {
		t.Fatalf("session metrics %+v", s)
	}
	lm := l.Metrics
	if lm.InputTokens != 2*36983 || lm.DurationMs != 2*2878 || lm.CostUSD == nil || *lm.CostUSD < 0.052 || lm.SubagentsRunning != 1 || lm.State != "busy" || lm.CtxPct == nil || *lm.CtxPct != 18 {
		t.Fatalf("lane metrics %+v", lm)
	}
}
