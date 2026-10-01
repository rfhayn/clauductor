package state

import (
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-21 (UX pass 1, findings 3 and 4): bindings made before the inputs caught up
// are corrected, and a lane that ended stays ended whatever arrives late.

const (
	bRoot = "/repo"
	sidA  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	sidB  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	sidC  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func bindModel(t *testing.T) *Model {
	t.Helper()
	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, bRoot, t0)
	m.ApplyWorktrees(rootOnly(), nil, t0)
	return m
}

func rootOnly() []signals.Worktree { return []signals.Worktree{{Path: bRoot, Branch: "main"}} }

// withLanes is the worktree list once it has each lane's new worktree.
func withLanes(ids ...string) []signals.Worktree {
	wts := rootOnly()
	for _, id := range ids {
		wts = append(wts, signals.Worktree{Path: laneWT(id), Branch: "fix/" + id})
	}
	return wts
}

func laneWT(id string) string { return bRoot + "/.wt/" + id }

func laneRec(id, sid string) types.LaneRecord {
	return types.LaneRecord{ID: id, SessionID: sid, Path: laneWT(id), Type: "fix", Branch: "fix/" + id, Mode: "new", ActionDone: true}
}

// sessionsIn: the session ids the view shows on the worktree at path.
func sessionsIn(v View, path string) []string {
	out := []string{}
	for _, l := range append(append([]LaneView{}, v.Lanes...), v.QuietWorktrees...) {
		if l.Path == path {
			for _, s := range l.Sessions {
				out = append(out, s.ID)
			}
		}
	}
	return out
}

func shownAnywhere(v View, sid string) bool {
	for _, l := range append(append([]LaneView{}, v.Lanes...), v.QuietWorktrees...) {
		for _, s := range l.Sessions {
			if s.ID == sid {
				return true
			}
		}
	}
	return false
}

func hookAt(sid, cwd, event string) signals.HookEvent {
	return signals.HookEvent{SessionID: sid, Cwd: cwd, Event: event, Prompt: "go"}
}

func statusAt(sid, cwd string) signals.StatusPayload {
	pct := 40.0
	p := signals.StatusPayload{SessionID: sid, Cwd: cwd}
	p.ContextWindow.UsedPercentage = &pct
	return p
}

// Three lanes start together. Each lane's claude shows in `claude agents` and sends
// its first hook before the worktree list has its worktree, some even before the
// tmux poll has brought its registry record in. Each session still ends up on its
// own lane, and the root lane shows none of them.
func TestLanesStartedTogetherBindToTheirOwnWorktrees(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	now := t0.Add(time.Second)
	// a: its record is in, its worktree is not listed yet.
	// b: neither is: only its cwd says where it is.
	// c: first seen in `claude agents`, record in, worktree not listed.
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Created: 1}, {ID: "c", Created: 1}},
		[]types.LaneRecord{laneRec("a", sidA), laneRec("c", sidC)}, "", nil, now)
	m.ApplyHook(hookAt(sidA, laneWT("a"), "UserPromptSubmit"), now)
	m.ApplyHook(hookAt(sidB, laneWT("b"), "UserPromptSubmit"), now)
	m.ApplyStatus(statusAt(sidB, laneWT("b")), now)
	m.ApplyAgents([]signals.Agent{{SessionID: sidC, Cwd: laneWT("c"), Status: "busy"}}, nil, now)
	if got := sessionsIn(m.Snapshot(now), bRoot); len(got) != 3 {
		t.Fatalf("before the list caught up, the root holds %v (the enclosing worktree), want all three", got)
	}
	// The worktree list catches up; b's record arrives on the next tmux poll.
	now = now.Add(500 * time.Millisecond)
	m.ApplyWorktrees(withLanes("a", "b", "c"), nil, now)
	v := m.Snapshot(now)
	for id, sid := range map[string]string{"a": sidA, "b": sidB, "c": sidC} {
		if got := sessionsIn(v, laneWT(id)); len(got) != 1 || got[0] != sid {
			t.Errorf("lane %s shows %v, want [%s]", id, got, sid)
		}
	}
	if got := sessionsIn(v, bRoot); len(got) != 0 {
		t.Errorf("the root still shows %v", got)
	}
	// Later events land on the lanes too, and so do the lanes' status and last hook.
	now = now.Add(time.Second)
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Created: 1}, {ID: "b", Created: 1}, {ID: "c", Created: 1}},
		[]types.LaneRecord{laneRec("a", sidA), laneRec("b", sidB), laneRec("c", sidC)}, "", nil, now)
	m.ApplyHook(hookAt(sidA, laneWT("a"), "Stop"), now)
	v = m.Snapshot(now)
	for _, l := range v.Lanes {
		if l.Path == laneWT("a") && (l.Status != "idle" || l.LastHookAt != ms(now)) {
			t.Errorf("lane a: status %s, last hook %d", l.Status, l.LastHookAt)
		}
		if l.Path == bRoot && len(l.Sessions) != 0 {
			t.Errorf("root: %+v", l.Sessions)
		}
	}
	for _, tv := range v.Terminals {
		if tv.Worktree != laneWT(tv.ID) {
			t.Errorf("terminal %s placed in %q", tv.ID, tv.Worktree)
		}
	}
}

// A registry record owns its session id: once the record's own worktree is listed,
// the session moves there even when its first event came from somewhere else.
func TestARecordMovesItsSessionOnceItsWorktreeIsListed(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	m.ApplyHook(hookAt(sidA, bRoot+"/sub", "UserPromptSubmit"), t0) // cd'd before its first hook
	m.ApplyWorktrees(withLanes("a"), nil, t0)
	if got := sessionsIn(m.Snapshot(t0), bRoot); len(got) != 1 {
		t.Fatalf("with no record, a session bound by cwd stays: root shows %v", got)
	}
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Created: 1}}, []types.LaneRecord{laneRec("a", sidA)}, "", nil, t0)
	v := m.Snapshot(t0)
	if got := sessionsIn(v, laneWT("a")); len(got) != 1 {
		t.Fatalf("lane a shows %v once its record is in", got)
	}
	if got := sessionsIn(v, bRoot); len(got) != 0 {
		t.Fatalf("root still shows %v", got)
	}
}

// Correcting a provisional binding does not break "a later cd does not move a
// session": only the cwd at first sight counts, and only while the binding is new.
func TestALaterCdStillDoesNotMoveASession(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	// First seen in the root itself; later it runs `cd` into a lane's new worktree.
	m.ApplyHook(hookAt("ext", bRoot, "UserPromptSubmit"), t0)
	m.ApplyWorktrees(withLanes("x"), nil, t0.Add(time.Second))
	m.ApplyHook(hookAt("ext", laneWT("x"), "Stop"), t0.Add(2*time.Second))
	m.ApplyWorktrees(withLanes("x"), nil, t0.Add(3*time.Second))
	v := m.Snapshot(t0.Add(3 * time.Second))
	if got := sessionsIn(v, bRoot); len(got) != 1 || got[0] != "ext" {
		t.Fatalf("a cd moved the session: root %v, x %v", got, sessionsIn(v, laneWT("x")))
	}
	// First seen inside a worktree the list did not have yet, which appears only
	// after provisionalWindow: the session stays where it was bound.
	m.ApplyHook(hookAt("late", laneWT("y"), "UserPromptSubmit"), t0)
	m.ApplyWorktrees(withLanes("x", "y"), nil, t0.Add(provisionalWindow+time.Second))
	v = m.Snapshot(t0.Add(provisionalWindow + time.Second))
	if got := sessionsIn(v, laneWT("y")); len(got) != 0 {
		t.Fatalf("a worktree listed long after the binding took its session: %v", got)
	}
}

// A lane stopped or closed while its session still sends status posts and hooks
// (no SessionEnd yet, or never: a crash) is gone at once, and stays gone.
func TestAnEndedLaneIsNotBroughtBackByLatePosts(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	m.ApplyWorktrees(withLanes("a", "b"), nil, t0)
	recs := []types.LaneRecord{laneRec("a", sidA), laneRec("b", sidB)}
	live := []types.TmuxLane{{ID: "a", Created: 1}, {ID: "b", Created: 1}}
	m.ApplyTmux(live, recs, "", nil, t0)
	for _, sid := range []string{sidA, sidB} {
		id := map[string]string{sidA: "a", sidB: "b"}[sid]
		m.ApplyHook(hookAt(sid, laneWT(id), "UserPromptSubmit"), t0)
		m.ApplyStatus(statusAt(sid, laneWT(id)), t0)
	}
	m.ApplyAgents([]signals.Agent{{SessionID: sidA, Cwd: laneWT("a"), Status: "busy"}, {SessionID: sidB, Cwd: laneWT("b"), Status: "busy"}}, nil, t0)

	// a is stopped (record and tmux session gone); b crashes (tmux gone, record kept).
	now := t0.Add(2 * time.Second)
	m.ApplyTmux(nil, []types.LaneRecord{laneRec("b", sidB)}, "", nil, now)
	// Late posts and hooks keep arriving, and `claude agents` still lists them for a
	// moment (an exiting claude lingers).
	for i := 0; i < 3; i++ {
		now = now.Add(time.Second)
		m.ApplyStatus(statusAt(sidA, laneWT("a")), now)
		m.ApplyHook(hookAt(sidA, laneWT("a"), "Stop"), now)
		m.ApplyHook(hookAt(sidB, laneWT("b"), "PermissionRequest"), now)
		m.ApplyAgents([]signals.Agent{{SessionID: sidA, Cwd: laneWT("a"), Status: "idle"}}, nil, now)
	}
	v := m.Snapshot(now)
	for _, sid := range []string{sidA, sidB} {
		if shownAnywhere(v, sid) {
			t.Errorf("session %s still shows after its lane ended: %+v", sid, v.Lanes)
		}
	}
	for _, l := range v.Lanes {
		if !(l.Terminal != "") {
			t.Errorf("a lane with no terminal shows: %s (%s)", l.Path, l.Status)
		}
	}
	if len(v.NeedsYou) != 0 {
		t.Errorf("a late permission request from a crashed lane raised Needs you: %+v", v.NeedsYou)
	}
	if v.Observe.DroppedEnded == 0 || v.Observe.DroppedForeign != 0 {
		t.Errorf("late events: %d set aside, %d foreign", v.Observe.DroppedEnded, v.Observe.DroppedForeign)
	}
	for _, tv := range v.Terminals {
		if tv.ID == "b" && tv.Status != "orphaned" {
			t.Errorf("crashed lane b: %+v", tv)
		}
	}

	// Closed: a's worktree is removed, and the session is forgotten long after. A
	// late post from inside the removed worktree is still not re-bound by its cwd
	// to the enclosing worktree (the root).
	m.ApplyWorktrees(withLanes("b"), nil, now)
	m.ApplyHook(hookAt(sidA, laneWT("a"), "UserPromptSubmit"), now.Add(time.Second))
	if got := sessionsIn(m.Snapshot(now.Add(time.Second)), bRoot); len(got) != 0 {
		t.Fatalf("a closed lane's late hook came back on the root: %v", got)
	}
	if !m.OwnsSession(sidA) {
		t.Fatal("an ended session must stay this project's, or another project would bind it by cwd")
	}

	// b is resumed: a new tmux session on the same session id takes it back.
	now = now.Add(5 * time.Second)
	m.ApplyTmux([]types.TmuxLane{{ID: "b", Created: 9}}, []types.LaneRecord{laneRec("b", sidB)}, "", nil, now)
	m.ApplyHook(hookAt(sidB, laneWT("b"), "UserPromptSubmit"), now)
	if got := sessionsIn(m.Snapshot(now), laneWT("b")); len(got) != 1 {
		t.Fatalf("resumed lane b shows %v", got)
	}
}

// Restart kills the lane's tmux session and starts a new one on the same session id
// between two polls: the old process's state goes, and the new one's is applied.
func TestARestartedLaneKeepsItsSession(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	m.ApplyWorktrees(withLanes("a"), nil, t0)
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Created: 1}}, []types.LaneRecord{laneRec("a", sidA)}, "", nil, t0)
	m.ApplyHook(hookAt(sidA, laneWT("a"), "PermissionRequest"), t0)
	now := t0.Add(3 * time.Second)
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Created: 4}}, []types.LaneRecord{laneRec("a", sidA)}, "", nil, now)
	if v := m.Snapshot(now); len(v.NeedsYou) != 0 {
		t.Fatalf("the old process's prompt survived the restart: %+v", v.NeedsYou)
	}
	m.ApplyHook(hookAt(sidA, laneWT("a"), "UserPromptSubmit"), now)
	v := m.Snapshot(now)
	if got := sessionsIn(v, laneWT("a")); len(got) != 1 {
		t.Fatalf("restarted lane shows %v", got)
	}
}

// A session started outside the panel that ends with no SessionEnd drops out once
// `claude agents` stops listing it; what it sends in the next moments is set aside.
func TestASessionGoneFromAgentsDoesNotLinger(t *testing.T) {
	t.Parallel()
	m := bindModel(t)
	m.ApplyWorktrees(withLanes("x"), nil, t0)
	m.ApplyHook(hookAt("ext", laneWT("x"), "UserPromptSubmit"), t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "ext", Cwd: laneWT("x"), Status: "busy"}}, nil, t0)
	now := t0.Add(2 * time.Second)
	m.ApplyAgents(nil, nil, now) // crashed
	m.ApplyStatus(statusAt("ext", laneWT("x")), now.Add(time.Second))
	m.ApplyHook(hookAt("ext", laneWT("x"), "Stop"), now.Add(2*time.Second))
	if v := m.Snapshot(now.Add(2 * time.Second)); shownAnywhere(v, "ext") {
		t.Fatalf("a crashed session lingers: %+v", v.Lanes)
	}
	// Listed again (it was only missing from one poll), it is back at once.
	m.ApplyAgents([]signals.Agent{{SessionID: "ext", Cwd: laneWT("x"), Status: "busy"}}, nil, now.Add(3*time.Second))
	if v := m.Snapshot(now.Add(3 * time.Second)); !shownAnywhere(v, "ext") {
		t.Fatal("a session listed again stays hidden")
	}
}
