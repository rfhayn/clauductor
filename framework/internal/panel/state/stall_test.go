package state

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-51: the stall rule (design D1, D3, D4) and its Needs-you row (D11), on a
// fixed clock. Times are written as the scenarios write them, on 2026-10-18 in the
// panel's local time (UTC here); eve(h, m) is the evening before.

var stallDay = time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)

func hm(h, m int) time.Time {
	return stallDay.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
}
func eve(h, m int) time.Time   { return hm(h-24, m) }
func clock(t time.Time) string { return t.UTC().Format("Jan 2 15:04") }

const (
	headX = "1a2b3c4d5e6f7a8b9c0d1a2b3c4d5e6f7a8b9c0d"
	headY = "2b3c4d5e6f7a8b9c0d1a2b3c4d5e6f7a8b9c0d1a"
)

// stallRig is a model with its own `claude agents` listing, polled before each view
// so the reading is current unless a test says otherwise.
type stallRig struct {
	t      *testing.T
	m      *Model
	agents map[string]signals.Agent
}

func newStallRig(t *testing.T, start time.Time, cfgExtra string, wts ...signals.Worktree) *stallRig {
	t.Helper()
	m := NewModel(v2Config(t, cfgExtra), "/repo", start)
	m.stall.loc = time.UTC
	if len(wts) == 0 {
		wts = []signals.Worktree{{Path: "/repo", Branch: "main", Head: headY}, {Path: "/repo/w/x", Branch: "change/x", Head: headX}}
	}
	m.ApplyWorktrees(wts, nil, start)
	m.ApplyLaneExtras(map[string]LaneExtra{}, false) // the receipt read has answered: no receipts
	return &stallRig{t: t, m: m, agents: map[string]signals.Agent{}}
}

// list lists the session; listed again, it keeps its process (PID and start).
func (r *stallRig) list(id, cwd, status string, waitingFor ...string) {
	a := signals.Agent{SessionID: id, Cwd: cwd, Status: status, PID: 100 + len(r.agents)}
	if was, ok := r.agents[id]; ok {
		a.PID, a.StartedAt = was.PID, was.StartedAt
	}
	if len(waitingFor) > 0 {
		a.WaitingFor = waitingFor[0]
	}
	r.agents[id] = a
}

// proc gives a listed session another process: `claude --resume` under the same id.
func (r *stallRig) proc(id string, pid int, started time.Time) {
	a := r.agents[id]
	a.PID, a.StartedAt = pid, started.UnixMilli()
	r.agents[id] = a
}

func (r *stallRig) poll(at time.Time) {
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := []signals.Agent{}
	for _, id := range ids {
		list = append(list, r.agents[id])
	}
	r.m.ApplyAgents(list, nil, at)
}

func (r *stallRig) prs(at time.Time, prs ...signals.PR) { r.m.ApplyPRs(prs, nil, at) }

// hook applies a hook body built from its event, session, cwd and extra JSON fields.
func (r *stallRig) hook(at time.Time, event, session, cwd, extra string) {
	r.t.Helper()
	body := `{"hook_event_name":"` + event + `","session_id":"` + session + `","cwd":"` + cwd + `"`
	if extra != "" {
		body += "," + extra
	}
	ev, err := signals.ParseHook([]byte(body + "}"))
	if err != nil {
		r.t.Fatal(err)
	}
	r.m.ApplyHook(ev, at)
}

func (r *stallRig) view(at time.Time) View {
	r.poll(at)
	return r.m.Snapshot(at)
}

func laneIn(t *testing.T, v View, path string) LaneView {
	t.Helper()
	for _, l := range append(append([]LaneView{}, v.Lanes...), v.QuietWorktrees...) {
		if l.Path == path {
			return l
		}
	}
	t.Fatalf("no lane %s in the view", path)
	return LaneView{}
}

func stalledAlert(v View, path string) *AlertView {
	for i := range v.Alerts {
		if v.Alerts[i].Key == AlertStalled+":"+path {
			return &v.Alerts[i]
		}
	}
	return nil
}

func notStalled(t *testing.T, v View, path, when string) {
	t.Helper()
	if l := laneIn(t, v, path); l.Stall != nil {
		t.Errorf("%s: %s is stalled: %+v", when, path, *l.Stall)
	}
	if a := stalledAlert(v, path); a != nil {
		t.Errorf("%s: a stalled alert is active for %s: %+v", when, path, *a)
	}
}

// stalledSince checks the lane's stall and its alert, both with since.
func stalledSince(t *testing.T, v View, path string, since time.Time, when string) StallView {
	t.Helper()
	l := laneIn(t, v, path)
	if l.Stall == nil {
		t.Fatalf("%s: %s is not stalled", when, path)
	}
	if got := time.UnixMilli(l.Stall.Since); !got.Equal(since) {
		t.Errorf("%s: %s stalled since %s, want %s", when, path, clock(got), clock(since))
	}
	a := stalledAlert(v, path)
	if a == nil {
		t.Fatalf("%s: no stalled alert for %s", when, path)
	}
	if a.Severity != signals.SevBlock || a.Since != since.UnixMilli() {
		t.Errorf("%s: alert %+v, want severity block since %s", when, *a, clock(since))
	}
	return *l.Stall
}

func hasWork(s StallView, want string) bool {
	for _, w := range s.OpenWork {
		if strings.Contains(w, want) {
			return true
		}
	}
	return false
}

func prX(n int) signals.PR { return signals.PR{Number: n, HeadRef: "change/x"} }

// [STALL-1-S1] An idle lane with an open pull request stalls at the stall time.
func TestStallIdleLaneWithOpenPR(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	// The Notification that reports the stop arrives after it, and is not movement (D1).
	r.hook(hm(0, 1), "Notification", "s", "/repo/w/x", `"notification_type":"idle_prompt","message":"Claude is waiting for your input"`)
	notStalled(t, r.view(hm(0, 29)), "/repo/w/x", "00:29")
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30")
	if !hasWork(st, "#152 open") {
		t.Errorf("openWork %v, want #152 open", st.OpenWork)
	}
}

// [STALL-1-S2] A busy lane is never stalled.
func TestStallBusyLaneNever(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "busy")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	v := r.view(hm(2, 0))
	notStalled(t, v, "/repo/w/x", "after two busy hours")
	if alertKinds(v)[AlertStalled] != "" {
		t.Errorf("a stalled alert is active: %+v", v.Alerts)
	}
}

const gateQueue = `,"queues":[{"id":"gate","title":"Gate","lock":"clauductor/gate.lock"}]`

func gateHeld(lane string, alive bool) []types.QueueView { return gateRun(lane, "n-"+lane, alive) }

// gateRun is the gate held by one gate run, told apart from another run of the same lane
// by its lease's nonce.
func gateRun(lane, nonce string, alive bool) []types.QueueView {
	return []types.QueueView{{ID: "gate", Held: true, Holder: &types.LeaseView{Nonce: nonce, PID: 42, Lane: lane, Alive: alive}, Waiters: []types.LeaseView{}}}
}

func gateFree() []types.QueueView {
	return []types.QueueView{{ID: "gate", Waiters: []types.LeaseView{}}}
}

// [STALL-1-S3] A lane that holds or waits for the gate is working, and its clock
// starts when it lets go.
func TestStallLeaseHoldsAndReleases(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), gateQueue)
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.m.ApplyQueues(gateHeld("x", true), nil, hm(0, 0))
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, lease held")
	r.m.ApplyQueues(gateFree(), nil, hm(0, 45))
	notStalled(t, r.view(hm(1, 14)), "/repo/w/x", "01:14")
	stalledSince(t, r.view(hm(1, 15)), "/repo/w/x", hm(0, 45), "01:15")
	// A waiter counts as holding.
	w := newStallRig(t, hm(0, 0), gateQueue)
	w.list("s", "/repo/w/x", "idle")
	w.poll(hm(0, 0))
	w.prs(hm(0, 0), prX(152))
	w.m.ApplyQueues([]types.QueueView{{ID: "gate", Waiters: []types.LeaseView{{Nonce: "w", Lane: "x", Alive: true}}}}, nil, hm(0, 0))
	notStalled(t, w.view(hm(1, 0)), "/repo/w/x", "01:00, waiting for the lease")
}

// [STALL-1-S4] An idle lane with no open work is not stalled.
func TestStallNoOpenWork(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	v := r.view(hm(3, 0))
	notStalled(t, v, "/repo/w/x", "03:00")
	for _, a := range v.Alerts {
		if a.Lane == "/repo/w/x" && a.Kind != AlertIdle {
			t.Errorf("an alert other than idle is active: %+v", a)
		}
	}
	if alertKinds(v)[AlertIdle] == "" {
		t.Errorf("the idle alert is not active: %+v", v.Alerts)
	}
}

func cleanReceipt(head string) LaneExtra {
	return LaneExtra{HasReceipt: true, Receipt: signals.Receipt{SHA: head, Kind: "full", Clean: true}}
}

// [STALL-1-S5] A gate that passed with nothing after it is open work, and a merge
// ends it.
func TestStallGatePassedNoPR(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}, true)
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true}})
	st := stalledSince(t, r.view(hm(0, 40)), "/repo/w/x", hm(0, 0), "00:40")
	if !hasWork(st, "gate passed for "+headX[:10]+", no PR") {
		t.Errorf("openWork %v, want the gate's pass for HEAD and no PR", st.OpenWork)
	}
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true, PR: 153}})
	notStalled(t, r.view(hm(0, 41)), "/repo/w/x", "after #153 merged")
}

// [STALL-1-S6] A session on the main checkout that waits on a question stalls with no
// branch of its own.
func TestStallMainCheckoutQuestion(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("o", "/repo", "busy")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.hook(hm(0, 0), "PermissionRequest", "o", "/repo", `"tool_name":"AskUserQuestion"`)
	r.list("o", "/repo", "waiting", "input needed")
	v := r.view(hm(0, 30))
	st := stalledSince(t, v, "/repo", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "input needed") {
		t.Errorf("waitedOn %q does not name the question", st.WaitedOn)
	}
	if alertKinds(v)[AlertWaiting] != signals.SevBlock {
		t.Errorf("the waiting alert is not active: %+v", v.Alerts)
	}
}

// [STALL-1-S7] Movement clears a stall, and the next one is a new stretch.
func TestStallMovementClears(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	stalledSince(t, r.view(hm(0, 40)), "/repo/w/x", hm(0, 0), "00:40")
	r.hook(hm(1, 0), "UserPromptSubmit", "s", "/repo/w/x", `"prompt":"go on"`)
	r.list("s", "/repo/w/x", "busy")
	r.poll(hm(1, 0))
	r.hook(hm(1, 5), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(1, 5))
	notStalled(t, r.view(hm(1, 20)), "/repo/w/x", "01:20")
	stalledSince(t, r.view(hm(1, 35)), "/repo/w/x", hm(1, 5), "01:35")
}

// [STALL-1-S9] A session the panel did not launch is judged by the same rule.
func TestStallHandStartedSession(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/9-x", Branch: "fix/9-x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("h", "/repo/w/9-x/src", "idle") // bound by its cwd, inside the worktree
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/9-x"})
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/9-x", hm(0, 0), "00:30")
	if !hasWork(st, "#160 open") {
		t.Errorf("openWork %v, want #160 open", st.OpenWork)
	}
}

func launched(id, session, path string) types.LaneRecord {
	return types.LaneRecord{ID: id, SessionID: session, Path: path, Type: "build", Action: "start", ActionDone: true}
}

// [STALL-1-S10] A lane whose claude exited with open work is stalled.
func TestStallClaudeExited(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, recs, "", nil, eve(23, 50))
	r.list("sx", "/repo/w/x", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152))
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "1"}}, recs, "", nil, hm(0, 0))
	delete(r.agents, "sx")
	r.poll(hm(0, 0))
	// The same dead pane at the next tmux polls is not movement (review round 6).
	for _, at := range []time.Time{hm(0, 10), hm(0, 20)} {
		r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "1"}}, recs, "", nil, at)
	}
	notStalled(t, r.view(hm(0, 29)), "/repo/w/x", "00:29")
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30")
	if !hasWork(st, "#152 open") || !hasWork(st, "claude exited") {
		t.Errorf("openWork %v, want #152 open and claude exited", st.OpenWork)
	}
	if !strings.Contains(st.WaitedOn, "exited (status 1)") {
		t.Errorf("waitedOn %q does not name exit status 1", st.WaitedOn)
	}
	// The push names the exit once: it is both the kind of wait and a piece of the
	// open work (group 4's review round 1, L3).
	a := stalledAlert(r.view(hm(0, 30)), "/repo/w/x")
	if a == nil {
		t.Fatal("no stalled alert at 00:30")
	}
	if _, body := PushText("P", *a, hm(0, 30)); strings.Count(body, "claude exited") != 1 || !strings.Contains(body, "#152 open") {
		t.Errorf("the push body %q, want \"claude exited\" once, and #152 open", body)
	}
}

// [STALL-1-S11] A stall from a reading that is not current is approximate.
func TestStallApproxWhenNotCurrent(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	recs := []types.LaneRecord{launched("y", "sy", "/repo/w/y")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "y", Path: "/repo/w/y", Created: 1}}, recs, "", nil, hm(0, 0))
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152), signals.PR{Number: 153, HeadRef: "change/y"})
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	r.m.ApplyTmux([]types.TmuxLane{{ID: "y", Path: "/repo/w/y", Created: 1, Dead: true, DeadStatus: "0"}}, recs, "", nil, hm(0, 5))
	r.poll(hm(0, 20))
	fail := errors.New("claude agents: timed out")
	for at := hm(0, 21); !at.After(hm(0, 30)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	v := r.m.Snapshot(hm(0, 30))
	st := stalledSince(t, v, "/repo/w/x", hm(0, 0), "00:30, polls failing 10 min")
	if !st.Approx || !stalledAlert(v, "/repo/w/x").Approx {
		t.Errorf("the stall %+v or its alert is not approximate", st)
	}
	if l := stalledNeed(t, v, "/repo/w/x", "00:30").Label; !strings.Contains(l, "claude agents") {
		t.Errorf("00:30: row labelled %q, want the stale reading as the reason", l)
	}
	for at := hm(0, 31); !at.After(hm(1, 0)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	v = r.m.Snapshot(hm(1, 0))
	notStalled(t, v, "/repo/w/x", "01:00, its session forgotten")
	if st := stalledSince(t, v, "/repo/w/y", hm(0, 5), "01:00, the launched lane"); !st.Approx {
		t.Errorf("the launched lane's stall is not approximate: %+v", st)
	}
}

// [STALL-1-S12] A lane whose hand-started session has ended is not stalled.
func TestStallEndedSessionNotStalled(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/9-x", Branch: "fix/9-x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("h", "/repo/w/9-x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/9-x"})
	r.hook(hm(0, 0), "SessionEnd", "h", "/repo/w/9-x", `"reason":"exit"`)
	delete(r.agents, "h")
	r.poll(hm(0, 1))
	v := r.view(hm(2, 0))
	notStalled(t, v, "/repo/w/9-x", "two hours after its SessionEnd")
	// Without the SessionEnd and the listing gone, the same lane stalls: the control.
	c := newStallRig(t, hm(0, 0), "", wts...)
	c.list("h", "/repo/w/9-x", "idle")
	c.poll(hm(0, 0))
	c.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/9-x"})
	stalledSince(t, c.view(hm(2, 0)), "/repo/w/9-x", hm(0, 0), "the control, its session live")
}

// [STALL-1-S13] A restart keeps a stall's quiet time and starts an unknown clock at
// the start.
func TestStallRestartCarriesClocks(t *testing.T) {
	t.Parallel()
	before := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/a", Branch: "change/a", Head: headX},
		{Path: "/repo/w/c", Branch: "change/c", Head: headY}}
	a := newStallRig(t, hm(0, 0), "", before...)
	a.list("sa", "/repo/w/a", "idle")
	a.list("sc", "/repo/w/c", "idle")
	a.poll(hm(0, 0))
	prs := []signals.PR{{Number: 1, HeadRef: "change/a"}, {Number: 2, HeadRef: "change/b"}, {Number: 3, HeadRef: "change/c"}}
	a.prs(hm(0, 0), prs...)
	a.hook(hm(0, 0), "Stop", "sa", "/repo/w/a", `"background_tasks":[],"session_crons":[]`)
	a.hook(hm(0, 40), "Stop", "sc", "/repo/w/c", `"background_tasks":[],"session_crons":[]`)
	stalledSince(t, a.view(hm(0, 30)), "/repo/w/a", hm(0, 0), "before the restart")
	a.poll(hm(1, 0))
	saved := roundTrip(t, a.m.StallState(hm(1, 0)))

	after := append(before, signals.Worktree{Path: "/repo/w/b", Branch: "change/b", Head: headX})
	b := newStallRig(t, hm(2, 0), "", after...)
	b.m.ApplyStallState(saved)
	b.prs(hm(2, 0), prs...)
	b.list("sa", "/repo/w/a", "idle")
	b.list("sa2", "/repo/w/a", "idle") // not in stall.json: the first poll's reading, not a start
	b.list("sb", "/repo/w/b", "idle")
	b.list("sc", "/repo/w/c", "idle")
	b.poll(hm(2, 1)) // the first poll, a minute after the start
	v := b.view(hm(2, 1))
	if v.StallsPending {
		t.Fatal("every source has answered, yet stalls are pending")
	}
	stalledSince(t, v, "/repo/w/a", hm(0, 0), "02:01, the lane stalled at the save")
	notStalled(t, b.view(hm(2, 9)), "/repo/w/c", "02:09")
	stalledSince(t, b.view(hm(2, 10)), "/repo/w/c", hm(1, 40), "02:10, the lane quiet 20 min at the save")
	notStalled(t, b.view(hm(2, 29)), "/repo/w/b", "02:29")
	stalledSince(t, b.view(hm(2, 30)), "/repo/w/b", hm(2, 0), "02:30, the lane with no saved movement")
}

// [STALL-3-S7] Review round 2, M1 (D1): a save made after a restart, before the sources
// a way rests on have answered (here `claude agents` has, the pull requests and the
// queues have not), keeps what the restart restored for that way while its clock has
// not moved: lane x's open-work stall and session sy's background-work stall, each with
// its time. Lane w, whose session moved after the restart, is saved not stalled. The
// next start then shows both stalls with the since they had, exact.
func TestStallSaveBeforeItsSourcesKeepsRestoredMarks(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}, {Path: "/repo/w/w", Branch: "change/w", Head: headY}}
	prs := []signals.PR{prX(152), {Number: 153, HeadRef: "change/w"}}
	shell := `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`
	a := newStallRig(t, hm(0, 0), gateQueue, wts...)
	a.list("sx", "/repo/w/x", "idle")
	a.list("sy", "/repo/w/y", "idle")
	a.list("sw", "/repo/w/w", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prs...)
	a.m.ApplyQueues([]types.QueueView{}, nil, hm(0, 0))
	a.hook(hm(0, 0), "Stop", "sy", "/repo/w/y", shell)
	v := a.view(hm(1, 0))
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "before the restart, lane x")
	stalledSince(t, v, "/repo/w/y", hm(0, 0), "before the restart, lane y")
	stalledSince(t, v, "/repo/w/w", hm(0, 0), "before the restart, lane w")
	saved := roundTrip(t, a.m.StallState(hm(1, 0)))

	b := newStallRig(t, hm(2, 0), gateQueue, wts...)
	b.m.ApplyStallState(saved)
	b.list("sx", "/repo/w/x", "idle")
	b.list("sy", "/repo/w/y", "idle")
	b.list("sw", "/repo/w/w", "idle")
	b.poll(hm(2, 0))
	b.hook(hm(2, 0), "UserPromptSubmit", "sw", "/repo/w/w", `"prompt":"go on"`)
	mid := roundTrip(t, b.m.StallState(hm(2, 1)))
	if l := mid.Lanes["/repo/w/x"]; !l.Stalled || l.Approx || l.Moved != hm(0, 0).UnixMilli() {
		t.Errorf("lane x saved before the pull requests answered: %+v, want stalled, exact, moved 00:00", l)
	}
	if sm := mid.Sessions["sy"]; !sm.Stalled || sm.Approx || sm.Moved != hm(0, 0).UnixMilli() {
		t.Errorf("session sy saved before the queues answered: stalled %v, approx %v, moved %d; want stalled, exact, moved 00:00",
			sm.Stalled, sm.Approx, sm.Moved)
	}
	if l := mid.Lanes["/repo/w/w"]; l.Stalled || l.Moved != hm(2, 0).UnixMilli() {
		t.Errorf("lane w, moved after the restart: %+v, want not stalled, moved 02:00", l)
	}

	c := newStallRig(t, hm(3, 0), gateQueue, wts...)
	c.m.ApplyStallState(mid)
	c.list("sx", "/repo/w/x", "idle")
	c.list("sy", "/repo/w/y", "idle")
	c.poll(hm(3, 0))
	c.prs(hm(3, 0), prs...)
	c.m.ApplyQueues([]types.QueueView{}, nil, hm(3, 0))
	v = c.view(hm(3, 1))
	for _, p := range []string{"/repo/w/x", "/repo/w/y"} {
		if st := stalledSince(t, v, p, hm(0, 0), "after the second restart"); st.Approx {
			t.Errorf("after the second restart %s's stall is approximate: it was carried stalled", p)
		}
	}
}

// roundTrip writes the state as stall.json would hold it and reads it back.
func roundTrip(t *testing.T, st StallState) StallState {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var out StallState
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// [STALL-1-S14] A lease is bound to a lane by its tmux id or a unique worktree
// basename.
func TestStallLeaseBinding(t *testing.T) {
	t.Parallel()
	base := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/second", Branch: "change/second", Head: headX}}
	shared := append(append([]signals.Worktree{}, base...), signals.Worktree{Path: "/repo/v/second", Branch: "change/other", Head: headY})
	for _, tc := range []struct {
		name, lane string
		wts        []signals.Worktree
		stalled    bool
	}{
		{"its tmux id", "t:second", base, false},
		{"its unique basename", "second", base, false},
		{"a basename two worktrees share", "second", shared, true},
	} {
		r := newStallRig(t, hm(0, 0), gateQueue, tc.wts...)
		r.m.ApplyTmux([]types.TmuxLane{{ID: "t:second", Path: "/repo/w/second", Created: 1}}, nil, "", nil, hm(0, 0))
		r.list("s", "/repo/w/second", "idle")
		r.poll(hm(0, 0))
		r.prs(hm(0, 0), signals.PR{Number: 7, HeadRef: "change/second"})
		r.m.ApplyQueues(gateHeld(tc.lane, true), nil, hm(0, 0))
		v := r.view(hm(1, 0))
		if tc.stalled {
			stalledSince(t, v, "/repo/w/second", hm(0, 0), tc.name)
		} else {
			notStalled(t, v, "/repo/w/second", tc.name)
		}
	}
}

// [STALL-1-S15] A session started by hand beside an old pull request is a start, not
// a stall.
func TestStallHandStartIsMovement(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/9-x", Branch: "fix/9-x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.m.ApplyStallState(StallState{SavedAt: hm(0, 0).UnixMilli(),
		Lanes: map[string]StallLaneMark{"/repo/w/9-x": {Moved: hm(0, 0).Add(-48 * time.Hour).UnixMilli()}}})
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/9-x"})
	notStalled(t, r.view(hm(2, 59)), "/repo/w/9-x", "02:59, no session")
	r.list("h", "/repo/w/9-x", "idle")
	r.poll(hm(3, 0))
	notStalled(t, r.view(hm(3, 1)), "/repo/w/9-x", "03:01")
	stalledSince(t, r.view(hm(3, 30)), "/repo/w/9-x", hm(3, 0), "03:30")
}

// [STALL-1-S16] A source that never succeeds leaves the other ways of stalling
// judged.
func TestStallFailingSourceLeavesQuestionJudged(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("o", "/repo", "waiting", "input needed")
	r.poll(hm(0, 0))
	r.hook(hm(0, 0), "PermissionRequest", "o", "/repo", `"tool_name":"AskUserQuestion"`)
	for at := hm(0, 0); !at.After(hm(0, 30)); at = at.Add(time.Minute) {
		r.m.ApplyPRs(nil, errors.New("gh: no GitHub remote"), at)
	}
	v := r.view(hm(0, 30))
	stalledSince(t, v, "/repo", hm(0, 0), "00:30")
	found := false
	for _, w := range v.Warnings {
		if strings.Contains(w, "pull requests") && strings.Contains(w, "not read") &&
			strings.Contains(w, "a pull request cannot make a lane stalled until they are") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning names the pull requests as not read: %q", v.Warnings)
	}
}

// D1: the view says stalls are pending until every source has answered once, or 10
// minutes have passed; past them a source still failing is named in a warning.
func TestStallsPendingUntilSourcesAnswer(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	if v := r.view(hm(0, 1)); !v.StallsPending {
		t.Error("00:01, `gh pr list` not answered: stalls are not pending")
	}
	r.prs(hm(0, 2), prX(152))
	if v := r.view(hm(0, 2)); v.StallsPending {
		t.Error("00:02, every source answered: stalls are still pending")
	}

	f := newStallRig(t, hm(0, 0), "")
	f.list("s", "/repo/w/x", "idle")
	for at := hm(0, 0); !at.After(hm(0, 10)); at = at.Add(time.Minute) {
		f.m.ApplyPRs(nil, errors.New("gh: no GitHub remote"), at)
	}
	if v := f.view(hm(0, 9)); !v.StallsPending {
		t.Error("00:09, `gh pr list` failing: stalls are not pending")
	}
	if v := f.view(hm(0, 10)); v.StallsPending {
		t.Error("00:10, 10 minutes passed: stalls are still pending")
	}
}

// [STALL-1-S16] tmux failing from the start, where the panel has launched a lane:
// stalls wait for it, and after 10 minutes a warning names it (D1).
func TestStallTmuxFailingAtStart(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	fail := errors.New("tmux: no server running")
	r.m.ApplyTmux(nil, recs, "", fail, hm(0, 0))
	r.list("sx", "/repo/w/x", "idle")
	r.prs(hm(0, 0), prX(152))
	if v := r.view(hm(0, 1)); !v.StallsPending {
		t.Error("00:01, tmux not answered: stalls are not pending")
	}
	r.m.ApplyTmux(nil, recs, "", fail, hm(0, 10))
	v := r.view(hm(0, 10))
	if v.StallsPending {
		t.Error("00:10: stalls are still pending")
	}
	found := false
	for _, w := range v.Warnings {
		found = found || strings.Contains(w, "tmux has not answered")
	}
	if !found {
		t.Errorf("00:10: no warning names tmux: %q", v.Warnings)
	}
}

// [STALL-1-S17] A waiting session stalls while a sibling in its lane works.
func TestStallQuestionBesideBusySibling(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("q", "/repo", "waiting", "input needed")
	r.list("o", "/repo", "busy")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	for at := hm(0, 1); !at.After(hm(0, 30)); at = at.Add(time.Minute) {
		r.hook(at, "PreToolUse", "o", "/repo", `"tool_name":"Agent"`)
		r.poll(at)
	}
	st := stalledSince(t, r.view(hm(0, 30)), "/repo", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "input needed") {
		t.Errorf("waitedOn %q does not name the question", st.WaitedOn)
	}
}

// [STALL-1-S18] A stale lease holder does not keep its lane working.
//
// The 00:05 read is the baseline with no lease, so 00:10 is the holder's first sight,
// dead or stale (S18's GIVEN): only the dead-holder rule can move the lane there, not
// a lease the lane was seen holding and has let go (review round 7's H1).
func TestStallDeadLeaseHolder(t *testing.T) {
	t.Parallel()
	stale := gateHeld("x", true)
	stale[0].Holder.Stale = true
	for _, c := range []struct {
		name   string
		holder []types.QueueView
	}{
		{"the holder dead", gateHeld("x", false)},
		{"the holder alive but stale, its lease past its time", stale},
	} {
		r := newStallRig(t, hm(0, 0), gateQueue)
		r.list("s", "/repo/w/x", "idle")
		r.poll(hm(0, 0))
		r.prs(hm(0, 0), prX(7))
		r.m.ApplyQueues(gateFree(), nil, hm(0, 5))
		r.m.ApplyQueues(c.holder, nil, hm(0, 10))
		// lease.ReadQueue keeps the dead holder listed until a waiter reclaims it: the same
		// reading again is not movement (review round 6).
		r.m.ApplyQueues(c.holder, nil, hm(0, 20))
		r.m.ApplyQueues(c.holder, nil, hm(0, 30))
		notStalled(t, r.view(hm(0, 39)), "/repo/w/x", c.name+" at 00:39")
		stalledSince(t, r.view(hm(0, 40)), "/repo/w/x", hm(0, 10), c.name+" at 00:40")
	}

	// A second dead holder from another gate run (another nonce) in place of the first is
	// a holder first seen dead: movement at its first sight, then the same again is not.
	r := newStallRig(t, hm(0, 0), gateQueue)
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(7))
	r.m.ApplyQueues(gateFree(), nil, hm(0, 5))
	r.m.ApplyQueues(gateRun("x", "run-1", false), nil, hm(0, 10))
	r.m.ApplyQueues(gateRun("x", "run-2", false), nil, hm(0, 20))
	r.m.ApplyQueues(gateRun("x", "run-2", false), nil, hm(0, 30))
	notStalled(t, r.view(hm(0, 49)), "/repo/w/x", "another run's dead holder at 00:49")
	stalledSince(t, r.view(hm(0, 50)), "/repo/w/x", hm(0, 20), "another run's dead holder at 00:50")
}

// [STALL-1-S19] A failed merged read keeps the last result, and is never a "none".
func TestStallMergedReadUnknown(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("sx", "/repo/w/x", "idle")
	r.list("sy", "/repo/w/y", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX), "/repo/w/y": cleanReceipt(headY)}, true)
	// The runtime keeps a lane's last result through a failed read (D10): the first
	// lane's "none" stands; the second has none yet.
	reads := map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true}}
	r.m.ApplyMergedReads(reads)
	stalledSince(t, r.view(hm(1, 0)), "/repo/w/x", hm(0, 0), "01:00")
	r.m.ApplyMergedReads(reads) // the next reads fail: nothing changes
	v := r.view(hm(1, 1))
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "after the failed read")
	notStalled(t, v, "/repo/w/y", "no merged read has succeeded")
	reads["/repo/w/y"] = MergedRead{Branch: "change/y", Head: headY, Read: true}
	r.m.ApplyMergedReads(reads)
	stalledSince(t, r.view(hm(1, 2)), "/repo/w/y", hm(0, 0), "the control: its read succeeds")
}

// notifyTick is one tick of the runtime's notify source over the view (its
// ProcessView), with no terminal focused.
func notifyTick(n *Notifier, v View, at time.Time) []Notice { return n.ProcessView(v, nil, at) }

// Group 3, review round 1, M1 (D6: once per stretch; D10): a pull request closed
// unmerged leaves the lane's merged read due and unanswered for up to one readiness
// interval (30 s). Its (b) is unknown meanwhile, so the lane is not shown stalled, but
// the notifier keeps its mark (D1: a stall's mark is kept while it is not yet judged),
// and the "none" that follows brings back the same stretch, sent once.
func TestStallClosedPRUnmergedSendsNothingAgain(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}, true)
	n := &Notifier{MinInterval: config.DefaultNotifyInterval * time.Second}
	v := r.view(hm(0, 40))
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "00:40, #152 open")
	if got := notifyTick(n, v, hm(0, 40)); len(got) != 1 {
		t.Fatalf("00:40: %d notices, want 1", len(got))
	}
	// 01:40: #152 is closed unmerged; gh pr list no longer lists it. The lane was never
	// due a merged read while its pull request was open, so it has no result.
	r.prs(hm(1, 40))
	at := hm(1, 40).Add(2 * time.Second) // the next notify tick
	v = r.view(at)
	notStalled(t, v, "/repo/w/x", "01:40:02, its merged read unanswered")
	if got := notifyTick(n, v, at); len(got) != 0 {
		t.Fatalf("01:40:02: %+v sent", got)
	}
	if since, ok := n.State().Notified[AlertStalled+":/repo/w/x"]; !ok || since != hm(0, 0).UnixMilli() {
		t.Errorf("01:40:02, the merged read unanswered: the notifier's mark is %d (held %v), want 00:00's", since, ok)
	}
	// 01:40:30: the merged read answers "none".
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true, At: hm(1, 40).Add(30 * time.Second)}})
	at = hm(1, 40).Add(32 * time.Second)
	v = r.view(at)
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "01:40:32, read none")
	if got := notifyTick(n, v, at); len(got) != 0 {
		t.Errorf("01:40:32: the stall since 00:00 sent again: %+v", got)
	}
}

// Group 3, review round 1, M1, its mirror (D10: "none" is read again when the branch
// leaves the open-PR list): a "none" read before the pull request was opened is no
// result once the branch leaves the list, so a lane that reaches the stall time before
// the merged read answers is not stalled, and not pushed, as "gate passed, no PR" for
// a pull request that merged.
func TestStallMergedPRStaleNoneIsNoResult(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "busy")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}, true)
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true, At: hm(0, 0)}})
	n := &Notifier{MinInterval: config.DefaultNotifyInterval * time.Second}
	notStalled(t, r.view(hm(0, 5)), "/repo/w/x", "00:05, busy")
	r.prs(hm(0, 10), prX(152))
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 50)) // movement: the session goes idle
	// 01:19:50: #152 merged; gh pr list no longer lists it. The runtime still hands the
	// model its "none" of 00:00 until its next read.
	r.prs(hm(1, 19).Add(50 * time.Second))
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true, At: hm(0, 0)}})
	for _, at := range []time.Time{hm(1, 20).Add(2 * time.Second), hm(1, 20).Add(4 * time.Second)} {
		v := r.view(at)
		notStalled(t, v, "/repo/w/x", clock(at)+", the merged read due again")
		if got := notifyTick(n, v, at); len(got) != 0 {
			t.Fatalf("%s: a merged pull request's lane pushed: %+v", clock(at), got)
		}
	}
	// 01:20:20: the merged read finds #152.
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true, PR: 152, At: hm(1, 20).Add(20 * time.Second)}})
	at := hm(1, 20).Add(22 * time.Second)
	v := r.view(at)
	notStalled(t, v, "/repo/w/x", "01:20:22, #152 read merged")
	if got := notifyTick(n, v, at); len(got) != 0 {
		t.Errorf("01:20:22: %+v sent", got)
	}
}

// [STALL-1-S20] A restart keeps a waiting session's stall.
func TestStallRestartKeepsQuestion(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, hm(0, 0), "")
	a.list("o", "/repo", "waiting", "input needed")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0))
	n := &Notifier{MinInterval: time.Minute}
	va := a.view(hm(0, 30))
	stalledSince(t, va, "/repo", hm(0, 0), "00:30")
	if got := n.Process(va.Alerts, nil, hm(0, 30)); len(got) != 1 {
		t.Fatalf("before the restart: %d notices, want 1", len(got))
	}
	a.poll(hm(1, 0))
	saved := roundTrip(t, a.m.StallState(hm(1, 0)))

	b := newStallRig(t, hm(1, 0), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(1, 0))
	b.list("o", "/repo", "waiting", "input needed")
	vb := b.view(hm(1, 0))
	if st := stalledSince(t, vb, "/repo", hm(0, 0), "after the restart"); st.Approx {
		t.Error("after the restart: a question stalled at the save is approximate (STALL-1-S25)")
	}
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(n.State())
	if got := n2.Process(vb.Alerts, nil, hm(1, 0)); len(got) != 0 {
		t.Errorf("after the restart: %+v sent again", got)
	}
}

// [STALL-1-S21] A restart keeps what a stalled turn ended waiting on.
func TestStallRestartKeepsTurn(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, eve(23, 50), "")
	a.list("o", "/repo", "idle")
	a.poll(eve(23, 50))
	a.prs(eve(23, 50))
	a.hook(eve(23, 55), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
	a.hook(hm(0, 0), "SubagentStop", "o", "/repo", `"agent_id":"a1","agent_type":"general-purpose"`)
	n := &Notifier{MinInterval: time.Minute}
	va := a.view(hm(0, 30))
	stalledSince(t, va, "/repo", hm(0, 0), "00:30")
	n.Process(va.Alerts, nil, hm(0, 30))
	a.poll(hm(1, 0))
	saved := roundTrip(t, a.m.StallState(hm(1, 0)))

	b := newStallRig(t, hm(1, 0), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(1, 0))
	b.list("o", "/repo", "idle")
	vb := b.view(hm(1, 0))
	st := stalledSince(t, vb, "/repo", hm(0, 0), "after the restart")
	if st.Approx {
		t.Error("after the restart: a turn stalled at the save is approximate (STALL-1-S25)")
	}
	for _, want := range []string{"1 subagent", "Review group 2", "finished and woke nothing"} {
		if !strings.Contains(st.WaitedOn, want) {
			t.Errorf("waitedOn %q lacks %q", st.WaitedOn, want)
		}
	}
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(n.State())
	if got := n2.Process(vb.Alerts, nil, hm(1, 0)); len(got) != 0 {
		t.Errorf("after the restart: %+v sent again", got)
	}
}

// [STALL-1-S22] A session's background-work stall is judged on its own clock,
// whatever a sibling does.
func TestStallOwnClockBesideBusySibling(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("h", "/repo", "idle")
	r.list("o", "/repo", "busy")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(eve(23, 59), "Stop", "o", "/repo", `"background_tasks":[],"session_crons":[{"id":"c","schedule":"*/5 * * * *","recurring":true}]`)
	r.hook(hm(0, 0), "Stop", "h", "/repo",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	for at := hm(0, 1); !at.After(hm(0, 30)); at = at.Add(time.Minute) {
		r.hook(at, "UserPromptSubmit", "o", "/repo", `"prompt":"tick"`)
		r.poll(at)
	}
	notStalled(t, r.view(hm(0, 29)), "/repo", "00:29")
	st := stalledSince(t, r.view(hm(0, 30)), "/repo", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "1 shell") || !strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("waitedOn %q does not name the second session's shell", st.WaitedOn)
	}
}

// [STALL-1-S23] A question is timed from the ask, whatever the session's own
// background agents do.
func TestStallQuestionTimedFromAsk(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(eve(23, 55), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"w1","type":"workflow","status":"running","description":"Build group 2"}],"session_crons":[]`)
	r.hook(hm(0, 0), "PermissionRequest", "o", "/repo", `"tool_name":"AskUserQuestion"`)
	r.list("o", "/repo", "waiting", "input needed")
	r.poll(hm(0, 0))
	for _, h := range []struct {
		at    time.Time
		event string
	}{{hm(0, 10), "SubagentStart"}, {hm(0, 15), "SubagentStop"}, {hm(0, 20), "SubagentStart"}, {hm(0, 25), "SubagentStop"}} {
		r.hook(h.at, h.event, "o", "/repo", `"agent_id":"a7","agent_type":"workflow-subagent"`)
	}
	notStalled(t, r.view(hm(0, 29)), "/repo", "00:29")
	st := stalledSince(t, r.view(hm(0, 30)), "/repo", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "input needed") {
		t.Errorf("waitedOn %q does not name the question", st.WaitedOn)
	}
}

// [STALL-1-S24] A session resumed beside an old pull request is a start, not a stall.
func TestStallResumeIsMovement(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/h", Branch: "fix/160-h", Head: headY},
		{Path: "/repo/w/x", Branch: "change/x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, recs, "", nil, hm(0, 0))
	r.list("h", "/repo/w/h", "idle")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/160-h"}, prX(161))
	r.hook(hm(1, 0), "SessionEnd", "h", "/repo/w/h", `"reason":"exit"`)
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "0"}}, recs, "", nil, hm(1, 0))
	delete(r.agents, "h")
	delete(r.agents, "sx")
	for at := hm(1, 0); at.Before(hm(3, 0)); at = at.Add(10 * time.Minute) {
		r.poll(at)
	}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 2}}, recs, "", nil, hm(3, 0))
	r.list("h", "/repo/w/h", "idle") // claude --resume <h>: the same id
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(3, 0))
	v := r.view(hm(3, 1))
	notStalled(t, v, "/repo/w/h", "03:01")
	notStalled(t, v, "/repo/w/x", "03:01")
	v = r.view(hm(3, 30))
	stalledSince(t, v, "/repo/w/h", hm(3, 0), "03:30, the resumed session")
	stalledSince(t, v, "/repo/w/x", hm(3, 0), "03:30, the resumed lane")
}

// [STALL-1-S24] [STALL-2-S19] A resumed session is a start like a new one: what the process that
// exited listed in its last Stop (a recurring cron, a shell) died with it, so it holds
// nothing off and is nobody's open work (D1, D4).
func TestStallResumeDropsPreExitStop(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/h", Branch: "fix/160-h", Head: headY},
		{Path: "/repo/w/x", Branch: "change/x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, recs, "", nil, hm(0, 0))
	r.list("h", "/repo/w/h", "idle")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), signals.PR{Number: 160, HeadRef: "fix/160-h"}, prX(161))
	r.hook(hm(0, 50), "Stop", "h", "/repo/w/h", `"background_tasks":[],"session_crons":[{"schedule":"*/5 * * * *","recurring":true}]`)
	r.hook(hm(0, 50), "Stop", "sx", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	r.hook(hm(0, 55), "SubagentStop", "h", "/repo/w/h", `"agent_id":"a1","agent_type":"reviewer"`)
	r.hook(hm(1, 0), "SessionEnd", "h", "/repo/w/h", `"reason":"exit"`)
	// SessionEnd drops the Stop at once, before any poll misses the session (L7); the
	// SubagentStop ids are kept, since they are unique.
	if st := r.m.sessions["h"].stall; st.stop != nil || len(st.stops) != 1 {
		t.Errorf("01:00, after SessionEnd: h keeps %+v, want no Stop and its one SubagentStop id", st)
	}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "0"}}, recs, "", nil, hm(1, 0))
	delete(r.agents, "h")
	delete(r.agents, "sx")
	// Resumed within forgetSessionAge of the exit, while the state still holds both.
	for at := hm(1, 0); at.Before(hm(1, 20)); at = at.Add(5 * time.Minute) {
		r.poll(at)
	}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 2}}, recs, "", nil, hm(1, 20))
	r.list("h", "/repo/w/h", "idle")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(1, 20))
	v := r.view(hm(1, 21))
	notStalled(t, v, "/repo/w/h", "01:21")
	notStalled(t, v, "/repo/w/x", "01:21")
	v = r.view(hm(1, 50))
	stalledSince(t, v, "/repo/w/h", hm(1, 20), "01:50, the resumed session, its pre-exit cron gone")
	st := stalledSince(t, v, "/repo/w/x", hm(1, 20), "01:50, the resumed lane")
	if hasWork(st, "turn ended on") || strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("the dead process's shell is still the lane's work: %+v", st)
	}
}

// [STALL-2-S1] A turn that ended with a shell in flight says so, and the command is
// nowhere.
func TestStallShellInFlightText(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"type":"shell","status":"running","description":"Run checks","command":"sh .claude/checks/run.sh"}]`)
	v := r.view(hm(0, 30))
	st := stalledSince(t, v, "/repo/w/x", hm(0, 0), "00:30")
	if st.WaitedOn != "its turn ended waiting on 1 shell: Run checks (running)" {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sh .claude/checks/run.sh") {
		t.Error("the view, its feed or /api/state holds the command")
	}
	if strings.Contains(string(r.m.StallState(hm(0, 30)).json(t)), "run.sh") {
		t.Error("the saved state holds the command")
	}
}

func (st StallState) json(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// [STALL-2-S2] A turn that ended with nothing in flight says so, with its last line.
func TestStallNothingInFlightText(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[],"session_crons":[],"last_assistant_message":"Waiting for merge-train's reply."`)
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30")
	if want := "its turn ended with nothing in flight to wake it; last said: Waiting for merge-train's reply."; st.WaitedOn != want {
		t.Errorf("waitedOn %q, want %q", st.WaitedOn, want)
	}
}

// [STALL-2-S3] A scheduled wakeup holds the stall off until it is due.
func TestStallWakeupHoldsOff(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("sx", "/repo/w/x", "idle")
	r.list("sy", "/repo/w/y", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152), signals.PR{Number: 153, HeadRef: "change/y"})
	r.hook(hm(0, 0), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[{"id":"c1","schedule":"0 1 18 10 *","recurring":false,"prompt":"wake"}]`)
	r.hook(hm(0, 0), "Stop", "sy", "/repo/w/y", `"background_tasks":[],"session_crons":[{"id":"c2","schedule":"*/10 * * * *","recurring":true}]`)
	v := r.view(hm(1, 4))
	notStalled(t, v, "/repo/w/x", "01:04")
	notStalled(t, v, "/repo/w/y", "01:04")
	v = r.view(hm(1, 5))
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "01:05")
	notStalled(t, v, "/repo/w/y", "01:05, a recurring cron")
}

// [STALL-2-S4] A Claude Code that sends no in-flight fields still stalls.
func TestStallFieldsNotReported(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"last_assistant_message":"ok"`)
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "not reported by this Claude Code version") {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// [STALL-2-S6] A SubagentStop changes nothing the lane last waited on.
func TestStallSubagentStopKeepsNothing(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[],"last_assistant_message":"Done for now."`)
	r.hook(hm(0, 1), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a1","agent_type":"reviewer",`+
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"x"}],"last_assistant_message":"reviewer finished"`)
	st := r.m.sessions["s"].stall.stop
	if st == nil || len(st.tasks) != 0 || st.last != "Done for now." {
		t.Errorf("kept %+v, want no tasks and the last line Done for now.", st)
	}
}

// [STALL-2-S7] A scheduled wakeup does not hold off a session waiting on the owner.
func TestStallWakeupDoesNotHoldQuestion(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(eve(23, 55), "Stop", "o", "/repo", `"background_tasks":[],"session_crons":[{"id":"c","schedule":"*/5 * * * *","recurring":true}]`)
	r.hook(hm(0, 0), "PermissionRequest", "o", "/repo", `"tool_name":"AskUserQuestion"`)
	r.list("o", "/repo", "waiting", "input needed")
	r.poll(hm(0, 0))
	st := stalledSince(t, r.view(hm(0, 30)), "/repo", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "input needed") {
		t.Errorf("waitedOn %q does not name the question", st.WaitedOn)
	}
}

// [STALL-2-S8] A background subagent at work holds the stall off until it ends.
func TestStallSubagentHoldsUntilPairedStop(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
	r.hook(hm(0, 1), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a9","agent_type":""`)
	notStalled(t, r.view(hm(1, 0)), "/repo/w/x", "01:00")
	r.hook(hm(1, 10), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a1","agent_type":"general-purpose"`)
	notStalled(t, r.view(hm(1, 39)), "/repo/w/x", "01:39")
	stalledSince(t, r.view(hm(1, 40)), "/repo/w/x", hm(1, 10), "01:40")
}

// [STALL-2-S9] A SubagentStop that arrives before its Stop still ends the task.
func TestStallStopBeforeItsStop(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152))
	r.hook(hm(0, 0), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a1","agent_type":"general-purpose"`)
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
	st := stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30")
	if !strings.Contains(st.WaitedOn, "finished and woke nothing") {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// [STALL-2-S10] A task whose end is never heard stops holding the stall off.
func TestStallUnheardEnd(t *testing.T) {
	t.Parallel()
	stop := `"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", stop)

	// The second lane: restored from a stall.json saved at 01:00, restarted at once.
	p := newStallRig(t, hm(0, 0), "")
	p.list("s", "/repo/w/x", "idle")
	p.poll(hm(0, 0))
	p.prs(hm(0, 0), prX(152))
	p.hook(hm(0, 0), "Stop", "s", "/repo/w/x", stop)
	p.poll(hm(1, 0))
	b := newStallRig(t, hm(1, 0), "")
	b.m.ApplyStallState(roundTrip(t, p.m.StallState(hm(1, 0))))
	b.prs(hm(1, 0), prX(152))
	b.list("s", "/repo/w/x", "idle")
	b.poll(hm(1, 0))

	for name, x := range map[string]*stallRig{"the live lane": r, "the restored lane": b} {
		notStalled(t, x.view(hm(1, 59)), "/repo/w/x", name+" at 01:59")
		st := stalledSince(t, x.view(hm(2, 0)), "/repo/w/x", hm(0, 0), name+" at 02:00")
		if !strings.Contains(st.WaitedOn, "Review group 2, whose end was never heard") {
			t.Errorf("%s: waitedOn %q", name, st.WaitedOn)
		}
	}
}

// [STALL-2-S11] A ScheduleWakeup one-shot holds its own session's stall off until 5
// minutes past its next fire time.
func TestStallOneShotWakeup(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, eve(23, 0), "", wts...)
	r.list("o", "/repo", "busy")
	r.list("h", "/repo", "idle")
	r.list("sx", "/repo/w/x", "idle")
	r.list("sy", "/repo/w/y", "idle")
	r.poll(eve(23, 0))
	r.prs(eve(23, 0), prX(152), signals.PR{Number: 153, HeadRef: "change/y"})
	r.hook(eve(23, 30), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[{"id":"c1","schedule":"20 0 * * *","recurring":false}]`)
	r.hook(eve(23, 40), "Stop", "h", "/repo",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],`+
			`"session_crons":[{"id":"c2","schedule":"35 0 * * *","recurring":false}]`)
	r.hook(hm(0, 5), "Stop", "sy", "/repo/w/y", `"background_tasks":[],"session_crons":[{"id":"c3","schedule":"40 0 * * *","recurring":false}]`)

	// #154: restored from a stall.json saved at 00:22 by a panel restarted at once.
	wz := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/z", Branch: "change/z", Head: headX}}
	p := newStallRig(t, eve(23, 0), "", wz...)
	p.list("sz", "/repo/w/z", "idle")
	p.poll(eve(23, 0))
	p.prs(eve(23, 0), signals.PR{Number: 154, HeadRef: "change/z"})
	p.hook(eve(23, 30), "Stop", "sz", "/repo/w/z", `"background_tasks":[],"session_crons":[{"id":"c4","schedule":"20 0 * * *","recurring":false}]`)
	p.poll(hm(0, 22))
	b := newStallRig(t, hm(0, 22), "", wz...)
	b.m.ApplyStallState(roundTrip(t, p.m.StallState(hm(0, 22))))
	b.prs(hm(0, 22), signals.PR{Number: 154, HeadRef: "change/z"})
	b.list("sz", "/repo/w/z", "idle")
	b.poll(hm(0, 22))

	v, vb := r.view(hm(0, 24)), b.view(hm(0, 24))
	for _, p := range []string{"/repo", "/repo/w/x", "/repo/w/y"} {
		notStalled(t, v, p, "00:24")
	}
	notStalled(t, vb, "/repo/w/z", "00:24")
	stalledSince(t, r.view(hm(0, 25)), "/repo/w/x", eve(23, 30), "00:25, #152")
	stalledSince(t, b.view(hm(0, 25)), "/repo/w/z", eve(23, 30), "00:25, #154")
	notStalled(t, r.view(hm(0, 35)), "/repo/w/y", "00:35")
	notStalled(t, r.view(hm(0, 39)), "/repo", "00:39")
	st := stalledSince(t, r.view(hm(0, 40)), "/repo", eve(23, 40), "00:40")
	if !strings.Contains(st.WaitedOn, "1 shell") || !strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("waitedOn %q does not name the second session's shell", st.WaitedOn)
	}
	notStalled(t, r.view(hm(0, 44)), "/repo/w/y", "00:44")
	stalledSince(t, r.view(hm(0, 45)), "/repo/w/y", hm(0, 5), "00:45")
}

// [STALL-2-S21] A one-shot cron of neither shape fireTime resolves (`0 9 * 10 *`: a
// day of `*` under a month that is a number) holds off its own session's
// background-work stall and its lane's open-work stall while its Stop's list stands,
// however long that is; a later Stop that lists no cron ends the hold, and the stall
// counts from that Stop.
func TestStallOneShotOfAnotherShapeHoldsOff(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("o", "/repo", "busy")
	r.list("h", "/repo", "idle")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	const shell = `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}]`
	const cron = `"session_crons":[{"id":"c1","schedule":"0 9 * 10 *","recurring":false}]`
	r.hook(hm(0, 0), "Stop", "h", "/repo", shell+","+cron)
	r.hook(hm(0, 0), "Stop", "sx", "/repo/w/x", `"background_tasks":[],`+cron)
	v := r.view(hm(5, 0))
	notStalled(t, v, "/repo", "05:00, the second session's own cron listed")
	notStalled(t, v, "/repo/w/x", "05:00, #152's session's own cron listed")

	r.hook(hm(5, 0), "Stop", "h", "/repo", shell+`,"session_crons":[]`)
	r.hook(hm(5, 0), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	v = r.view(hm(5, 29))
	notStalled(t, v, "/repo", "05:29")
	notStalled(t, v, "/repo/w/x", "05:29")
	v = r.view(hm(5, 30))
	st := stalledSince(t, v, "/repo", hm(5, 0), "05:30, the main checkout")
	if !strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("05:30: the main checkout's waitedOn %q does not name the second session's shell", st.WaitedOn)
	}
	stalledSince(t, v, "/repo/w/x", hm(5, 0), "05:30, #152")
}

// [STALL-2-S12] A background Monitor, listed as a shell, is read as one.
func TestStallMonitorReadAsShell(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(hm(0, 0), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"b7","type":"shell","status":"running","description":"Watch the gate's log","command":"tail -f gate.log | grep --line-buffered FAIL"}]`)
	notStalled(t, r.view(hm(0, 29)), "/repo", "00:29")
	v := r.view(hm(0, 30))
	st := stalledSince(t, v, "/repo", hm(0, 0), "00:30")
	if st.WaitedOn != "its turn ended waiting on 1 shell: Watch the gate's log (running)" {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "tail -f gate.log") {
		t.Error("the view holds the Monitor's command")
	}
}

// [STALL-2-S13] A background workflow holds the stall off until a later Stop no
// longer lists it.
func TestStallWorkflowHoldsUntilDropped(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, eve(23, 50), "", wts...)
	r.list("s1", "/repo/w/x", "idle")
	r.list("s2", "/repo/w/y", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152), signals.PR{Number: 153, HeadRef: "change/y"})
	r.hook(hm(0, 0), "Stop", "s1", "/repo/w/x", `"background_tasks":[{"id":"w1","type":"workflow","status":"running","description":"Build group 2"}],"session_crons":[]`)
	r.hook(hm(0, 0), "Stop", "s2", "/repo/w/y", `"background_tasks":[{"id":"w2","type":"workflow","status":"running","description":"Build group 3"}],"session_crons":[]`)
	r.hook(hm(0, 1), "SubagentStart", "s1", "/repo/w/x", `"agent_id":"a7","agent_type":"workflow-subagent"`)
	r.hook(hm(0, 1), "SubagentStart", "s2", "/repo/w/y", `"agent_id":"a8","agent_type":"workflow-subagent"`)
	v := r.view(hm(0, 35))
	notStalled(t, v, "/repo/w/x", "00:35")
	notStalled(t, v, "/repo/w/y", "00:35")
	r.hook(hm(0, 40), "SubagentStop", "s1", "/repo/w/x", `"agent_id":"a7","agent_type":"workflow-subagent"`)
	r.hook(hm(0, 40), "SubagentStop", "s2", "/repo/w/y", `"agent_id":"a8","agent_type":"workflow-subagent"`)
	r.view(hm(0, 40))
	for id, w := range map[string]string{"s1": "w1", "s2": "w2"} {
		st := r.m.sessions[id].stall.stop
		if st == nil || len(st.tasks) != 1 || st.tasks[0].id != w || st.tasks[0].ended {
			t.Errorf("00:40: %s keeps %+v, want %s kept and not ended", id, st, w)
		}
	}
	r.hook(hm(0, 41), "Stop", "s1", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	notStalled(t, r.view(hm(1, 10)), "/repo/w/x", "01:10")
	stalledSince(t, r.view(hm(1, 11)), "/repo/w/x", hm(0, 41), "01:11")
	notStalled(t, r.view(hm(2, 39)), "/repo/w/y", "02:39")
	st := stalledSince(t, r.view(hm(2, 40)), "/repo/w/y", hm(0, 40), "02:40")
	if !strings.Contains(st.WaitedOn, "Build group 3, whose end was never heard") {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// [STALL-2-S14] A teammate is part of its lead's session: its hooks move the lead,
// and its listed task is open work, never working.
func TestStallTeammateIsLeadsOpenWork(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("lead", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(hm(0, 0), "Stop", "lead", "/repo",
		`"background_tasks":[{"id":"t1","type":"teammate","status":"running","description":"Probe the stall rule"}],"session_crons":[]`)
	r.hook(hm(0, 10), "SubagentStop", "lead", "/repo", `"agent_id":"aprobe-mate-1","agent_type":"probe-mate"`)
	notStalled(t, r.view(hm(0, 39)), "/repo", "00:39")
	st := stalledSince(t, r.view(hm(0, 40)), "/repo", hm(0, 10), "00:40")
	if st.WaitedOn != "its turn ended waiting on 1 teammate: Probe the stall rule" {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// [STALL-2-S15] Only the allowlisted task types count; Claude Code's own
// housekeeping is not open work.
func TestStallTaskTypeAllowlist(t *testing.T) {
	t.Parallel()
	housekeeping := `{"id":"d1","type":"dream","status":"running"},{"id":"m1","type":"memory import","status":"running"}`
	r := newStallRig(t, eve(23, 50), "")
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[`+housekeeping+`],"session_crons":[]`)
	notStalled(t, r.view(hm(1, 0)), "/repo", "01:00, housekeeping alone")

	c := newStallRig(t, eve(23, 50), "")
	c.list("o", "/repo", "idle")
	c.poll(eve(23, 50))
	c.prs(eve(23, 50))
	c.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[`+housekeeping+
		`,{"id":"s1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	st := stalledSince(t, c.view(hm(0, 30)), "/repo", hm(0, 0), "00:30, with a shell")
	if st.WaitedOn != "its turn ended waiting on 1 shell: Run checks (running)" {
		t.Errorf("waitedOn %q, want the shell alone", st.WaitedOn)
	}

	// A task listed as `monitor` is open work too (D3 (d), amended 2026-10-19).
	mo := newStallRig(t, eve(23, 50), "")
	mo.list("o", "/repo", "idle")
	mo.poll(eve(23, 50))
	mo.prs(eve(23, 50))
	mo.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[`+housekeeping+
		`,{"id":"w1","type":"monitor","status":"running","description":"Watch the gate"}],"session_crons":[]`)
	st = stalledSince(t, mo.view(hm(0, 30)), "/repo", hm(0, 0), "00:30, with a monitor")
	if st.WaitedOn != "its turn ended waiting on 1 monitor: Watch the gate (running)" {
		t.Errorf("waitedOn %q, want the monitor alone", st.WaitedOn)
	}
}

// [STALL-2-S16] A turn that ends with no Stop (interrupted with Esc, or a StopFailure)
// leaves its session's background-work stall unknown until the next Stop (D4, #186).
func TestStallUnreportedTurnLeavesWorkUnknown(t *testing.T) {
	t.Parallel()
	shell := `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`
	r := newStallRig(t, eve(23, 50), "")
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(hm(0, 0), "Stop", "o", "/repo", shell)
	r.hook(hm(0, 5), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
	for _, at := range []time.Time{hm(0, 35), hm(1, 30)} {
		v := r.view(at)
		notStalled(t, v, "/repo", clock(at)+", the turn after the Stop unreported")
		if b, _ := json.Marshal(v); strings.Contains(string(b), "Run checks") {
			t.Errorf("%s: the view names the shell the turn before listed", clock(at))
		}
	}
	r.hook(hm(1, 31), "Stop", "o", "/repo", shell)
	notStalled(t, r.view(hm(2, 0)), "/repo", "02:00")
	st := stalledSince(t, r.view(hm(2, 1)), "/repo", hm(1, 31), "02:01, the next Stop listing the shell")
	if st.WaitedOn != "its turn ended waiting on 1 shell: Run checks (running)" {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// #186's mark is part of the kept Stop's lifecycle: a restart (L12, L3) and a record
// heard while its id is saved (L13) keep it, so a restart in the middle of an unreported
// turn does not bring the false push back; a lane stalled by its pull request names no
// work the unreported turn's Stop listed; and a teammate's UserPromptSubmit, under its
// lead's id (D9), leaves the lead's turn judged, as rule (b) leaves its Stop.
func TestStallUnreportedTurnAcrossTheLifecycle(t *testing.T) {
	t.Parallel()
	shell := `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`
	for _, c := range []struct {
		name  string
		heard bool // the UserPromptSubmit is heard after the restart, before the first poll
	}{{"before the save", false}, {"after the restart, before the first poll", true}} {
		a := newStallRig(t, eve(23, 50), "")
		a.list("o", "/repo", "idle")
		a.poll(eve(23, 50))
		a.prs(eve(23, 50))
		a.hook(hm(0, 0), "Stop", "o", "/repo", shell)
		if !c.heard {
			a.hook(hm(0, 5), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
		}
		a.poll(hm(0, 10))
		b := newStallRig(t, hm(0, 10), "")
		b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 10))))
		b.prs(hm(0, 10))
		if c.heard {
			b.hook(hm(0, 11), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
		}
		b.list("o", "/repo", "idle")
		b.poll(hm(0, 12))
		notStalled(t, b.view(hm(0, 45)), "/repo", "00:45, "+c.name)
		notStalled(t, b.view(hm(1, 30)), "/repo", "01:30, "+c.name)
		if sm := roundTrip(t, b.m.StallState(hm(1, 30))).Sessions["o"]; !sm.Unreported || sm.Stop == nil || len(sm.Stop.Tasks) != 1 {
			t.Errorf("%s: saved %+v, want the kept Stop and its turn unreported", c.name, sm)
		}
	}

	// [STALL-2-S20] A lane stalled on its pull request after an unreported turn names
	// none of the work that turn's kept Stop listed.
	p := newStallRig(t, eve(23, 50), "")
	p.list("s", "/repo/w/x", "idle")
	p.poll(eve(23, 50))
	p.prs(eve(23, 50), prX(152))
	p.hook(hm(0, 0), "Stop", "s", "/repo/w/x", shell)
	p.hook(hm(0, 5), "UserPromptSubmit", "s", "/repo/w/x", `"prompt":"go on"`)
	st := stalledSince(t, p.view(hm(0, 35)), "/repo/w/x", hm(0, 5), "00:35, the pull request open")
	if strings.Contains(st.WaitedOn, "Run checks") || hasWork(st, "turn ended on") {
		t.Errorf("the lane's stall names the unreported turn's shell: %+v", st)
	}
	if !strings.Contains(st.WaitedOn, "not known") {
		t.Errorf("waitedOn %q does not say what it waited on is not known", st.WaitedOn)
	}

	// The other half of L13's merge: a Stop heard while the mark is set aside or saved
	// ends the unreported turn, so (d) is judged on it once the session is listed again.
	t.Run("a turn that ends in a StopFailure", func(t *testing.T) {
		r := newStallRig(t, eve(23, 50), "")
		r.list("o", "/repo", "idle")
		r.poll(eve(23, 50))
		r.prs(eve(23, 50))
		r.hook(hm(0, 0), "Stop", "o", "/repo", shell)
		r.hook(hm(0, 5), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
		r.hook(hm(0, 5), "StopFailure", "o", "/repo", `"error":"rate_limit"`)
		notStalled(t, r.view(hm(0, 35)), "/repo", "00:35, after a StopFailure")
		notStalled(t, r.view(hm(1, 30)), "/repo", "01:30, after a StopFailure")
	})
	t.Run("a Stop heard while set aside", func(t *testing.T) {
		fail := errors.New("claude agents: timed out")
		r := newStallRig(t, hm(0, 0), "")
		r.list("o", "/repo", "idle")
		r.poll(hm(0, 0))
		r.prs(hm(0, 0))
		r.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[],"session_crons":[]`)
		r.hook(hm(0, 1), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
		for at := hm(0, 2); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
			r.m.ApplyAgents(nil, fail, at)
		}
		if _, ok := r.m.sessions["o"]; ok {
			t.Fatal("00:39: the session is still held; the test needs it set aside")
		}
		r.hook(hm(0, 40), "Stop", "o", "/repo", shell)
		r.poll(hm(0, 41))
		notStalled(t, r.view(hm(1, 9)), "/repo", "01:09")
		stalledSince(t, r.view(hm(1, 10)), "/repo", hm(0, 40), "01:10, the Stop heard while set aside")
	})
	t.Run("a Stop heard after a restart, before the first poll", func(t *testing.T) {
		a := newStallRig(t, eve(23, 50), "")
		a.list("o", "/repo", "idle")
		a.poll(eve(23, 50))
		a.prs(eve(23, 50))
		a.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[],"session_crons":[]`)
		a.hook(hm(0, 5), "UserPromptSubmit", "o", "/repo", `"prompt":"go on"`)
		a.poll(hm(0, 10))
		b := newStallRig(t, hm(0, 10), "")
		b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 10))))
		b.prs(hm(0, 10))
		b.hook(hm(0, 11), "Stop", "o", "/repo", shell)
		b.list("o", "/repo", "idle")
		b.poll(hm(0, 12))
		notStalled(t, b.view(hm(0, 40)), "/repo", "00:40")
		stalledSince(t, b.view(hm(0, 41)), "/repo", hm(0, 11), "00:41, the Stop heard after the restart")
	})

	// [STALL-1-S27] A teammate's UserPromptSubmit under its lead's id is the lead's
	// movement and marks nothing: the lead's turn stays judged.
	m := newStallRig(t, eve(23, 50), "")
	m.list("lead", "/repo", "idle")
	m.poll(eve(23, 50))
	m.prs(eve(23, 50))
	m.hook(hm(0, 0), "Stop", "lead", "/repo", shell)
	m.hook(hm(0, 5), "UserPromptSubmit", "lead", "/repo", `"agent_id":"aprobe-mate-1","agent_type":"probe-mate","prompt":"from the lead"`)
	stalledSince(t, m.view(hm(0, 35)), "/repo", hm(0, 5), "00:35, a teammate's prompt")
}

// D1: a session forgetSessions dropped while the polls failed takes its clock back
// when a poll lists it again; it is not a start.
func TestStallAsideTakesItsClockBack(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	fail := errors.New("claude agents: timed out")
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	if _, ok := r.m.sessions["s"]; ok {
		t.Fatal("00:39: the session is still held; the test needs it forgotten")
	}
	stalledSince(t, r.view(hm(0, 40)), "/repo/w/x", hm(0, 0), "00:40, listed again after the polls recover")
}

// D1: the lane's lease holds off a session's background-work stall (D3 (d)), which is
// judged on the session's own clock once the lease is given up.
func TestStallLeaseHoldsOffTurnWork(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), gateQueue)
	r.list("o", "/repo", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50))
	r.hook(hm(0, 0), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run the gate"}],"session_crons":[]`)
	r.m.ApplyQueues(gateHeld("repo", true), nil, hm(0, 0)) // run-local.sh's basename "$PWD"
	notStalled(t, r.view(hm(0, 30)), "/repo", "00:30, the lease held")
	notStalled(t, r.view(hm(0, 44)), "/repo", "00:44, the lease held")
	r.m.ApplyQueues(gateFree(), nil, hm(0, 45))
	st := stalledSince(t, r.view(hm(0, 45)), "/repo", hm(0, 0), "00:45, the lease given up")
	if st.WaitedOn != "its turn ended waiting on 1 shell: Run the gate (running)" {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// D3 (d): a subagent or workflow task listed pending is working, as one running is.
func TestStallPendingTaskIsWorking(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}}
	r := newStallRig(t, eve(23, 50), "", wts...)
	r.list("sx", "/repo/w/x", "idle")
	r.list("sy", "/repo/w/y", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152), signals.PR{Number: 153, HeadRef: "change/y"})
	r.hook(hm(0, 0), "Stop", "sx", "/repo/w/x",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"pending","description":"Review group 2"}],"session_crons":[]`)
	r.hook(hm(0, 0), "Stop", "sy", "/repo/w/y",
		`"background_tasks":[{"id":"w1","type":"workflow","status":"pending","description":"Build group 2"}],"session_crons":[]`)
	v := r.view(hm(1, 0))
	notStalled(t, v, "/repo/w/x", "01:00, a pending subagent")
	notStalled(t, v, "/repo/w/y", "01:00, a pending workflow")
	r.hook(hm(1, 10), "SubagentStop", "sx", "/repo/w/x", `"agent_id":"a1","agent_type":"general-purpose"`)
	stalledSince(t, r.view(hm(1, 40)), "/repo/w/x", hm(1, 10), "01:40, its subagent ended")
}

// D3 (d): a SubagentStop heard after a restart but before the first poll ends the
// task the restored Stop lists.
func TestStallRestoredStopPairsEarlyStop(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, eve(23, 50), "")
	a.list("o", "/repo", "idle")
	a.poll(eve(23, 50))
	a.prs(eve(23, 50))
	a.hook(hm(0, 0), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
	a.poll(hm(0, 10))
	saved := roundTrip(t, a.m.StallState(hm(0, 10)))

	b := newStallRig(t, hm(0, 10), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(0, 10))
	b.hook(hm(0, 11), "SubagentStop", "o", "/repo", `"agent_id":"a1","agent_type":"general-purpose"`)
	b.list("o", "/repo", "idle")
	b.poll(hm(0, 12))
	notStalled(t, b.view(hm(0, 40)), "/repo", "00:40")
	st := stalledSince(t, b.view(hm(0, 41)), "/repo", hm(0, 11), "00:41")
	if !strings.Contains(st.WaitedOn, "Review group 2, which finished and woke nothing") {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// [STALL-2-S17] The owner's rule (a), 2026-10-20: a Stop whose background_tasks field is absent (an
// older Claude Code) ends no workflow and changes no kept task; absent is not empty,
// so an absent session_crons changes no kept cron either.
func TestStallAbsentFieldsKeepTheKeptStop(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"id":"w1","type":"workflow","status":"running","description":"Build group 2"},`+
		`{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],`+
		`"session_crons":[{"id":"c","schedule":"*/5 * * * *","recurring":true}]`)
	r.hook(hm(0, 5), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a1","agent_type":"general-purpose"`)
	r.hook(hm(0, 10), "Stop", "s", "/repo/w/x", `"last_assistant_message":"still building"`)
	st := r.m.sessions["s"].stall.stop
	if st == nil || len(st.tasks) != 2 || st.tasks[0].id != "w1" || st.tasks[0].ended || st.tasks[1].id != "a1" || !st.tasks[1].ended {
		t.Fatalf("00:10: kept %+v, want w1 open and a1 ended, as before the Stop with no fields", st)
	}
	if len(st.crons) != 1 || !st.crons[0].recurring {
		t.Errorf("00:10: kept crons %+v, want the recurring cron kept", st.crons)
	}
	if st.last != "still building" {
		t.Errorf("00:10: last line %q, want the session's own last line", st.last)
	}
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, the workflow still working")

	// With tasks present and session_crons absent, the tasks are replaced and the
	// cron is kept.
	r.hook(hm(0, 45), "Stop", "s", "/repo/w/x", `"background_tasks":[]`)
	if st := r.m.sessions["s"].stall.stop; len(st.tasks) != 0 || len(st.crons) != 1 {
		t.Errorf("00:45: kept %+v, want no tasks and the cron kept", st)
	}
}

// [STALL-2-S18] The owner's rule (b), 2026-10-20: a Stop that carries an agent_id is never the
// session's own Stop (a teammate's hooks arrive under its lead's session_id: D9), so
// it ends no workflow, replaces no kept task or cron and changes no last line. It is
// still the session's movement. A guard: P3 saw a teammate's turn end as a
// SubagentStop, and whether a teammate ever sends a Stop is unprobed.
func TestStallAgentStopIsNotTheSessions(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, eve(23, 50), "")
	r.list("lead", "/repo/w/x", "idle")
	r.poll(eve(23, 50))
	r.prs(eve(23, 50), prX(152))
	r.hook(hm(0, 0), "Stop", "lead", "/repo/w/x", `"background_tasks":[{"id":"w1","type":"workflow","status":"running","description":"Build group 2"}],`+
		`"session_crons":[{"id":"c","schedule":"20 3 * * *","recurring":false}],"last_assistant_message":"building"`)
	r.hook(hm(0, 10), "Stop", "lead", "/repo/w/x", `"agent_id":"aprobe-mate-1","agent_type":"probe-mate",`+
		`"background_tasks":[],"session_crons":[],"last_assistant_message":"teammate done"`)
	st := r.m.sessions["lead"].stall.stop
	if st == nil || len(st.tasks) != 1 || st.tasks[0].id != "w1" || len(st.crons) != 1 || st.last != "building" || !st.at.Equal(hm(0, 0)) {
		t.Fatalf("00:10: kept %+v, want the lead's own Stop of 00:00 unchanged", st)
	}
	if got := r.m.sessions["lead"].stall.moved; !got.Equal(hm(0, 10)) {
		t.Errorf("00:10: the lead last moved %s, want 00:10: a teammate's hook is the lead's movement", clock(got))
	}
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, the lead's workflow still working")
}

// ---- a session's lifecycle (stall.go; the Decision log's table, rows L1 to L13) ----

// L2: the sessions the first successful poll after the start lists are not movement;
// their clocks start at the panel's start.
func TestStallLifeFirstPollIsBaseline(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.prs(hm(0, 0), prX(152))
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 5))
	notStalled(t, r.view(hm(0, 29)), "/repo/w/x", "00:29")
	stalledSince(t, r.view(hm(0, 30)), "/repo/w/x", hm(0, 0), "00:30, from the panel's start")
}

// L6: a live session's reading changing is movement, with no hook at all (a session
// whose settings turn hooks off).
func TestStallLifeReadingChangeMoves(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.list("s", "/repo/w/x", "busy")
	r.poll(hm(0, 20))
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 21))
	notStalled(t, r.view(hm(0, 50)), "/repo/w/x", "00:50")
	stalledSince(t, r.view(hm(0, 51)), "/repo/w/x", hm(0, 21), "00:51")
}

// L3: a restart's saved SubagentStop ids join the ids heard since, so a Stop heard
// after the restart pairs with a stop heard before it.
func TestStallLifeRestoredRingPairs(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, eve(23, 50), "")
	a.list("o", "/repo", "idle")
	a.poll(eve(23, 50))
	a.prs(eve(23, 50))
	a.hook(hm(0, 0), "SubagentStop", "o", "/repo", `"agent_id":"a1","agent_type":"general-purpose"`)
	a.poll(hm(0, 5))
	saved := roundTrip(t, a.m.StallState(hm(0, 5)))

	b := newStallRig(t, hm(0, 5), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(0, 5))
	b.hook(hm(0, 6), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
	b.list("o", "/repo", "idle")
	b.poll(hm(0, 7))
	notStalled(t, b.view(hm(0, 35)), "/repo", "00:35")
	st := stalledSince(t, b.view(hm(0, 36)), "/repo", hm(0, 6), "00:36")
	if !strings.Contains(st.WaitedOn, "Review group 2, which finished and woke nothing") {
		t.Errorf("waitedOn %q", st.WaitedOn)
	}
}

// L3, L11: a saved session the first poll lists under another process (resumed
// while the panel was down) is not restored: its saved clock and Stop were the old
// process's.
func TestStallLifeRestartOtherProcess(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, eve(23, 50), "")
	a.list("o", "/repo", "idle")
	a.proc("o", 100, eve(23, 0))
	a.poll(eve(23, 50))
	a.prs(eve(23, 50))
	a.hook(hm(0, 0), "Stop", "o", "/repo",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	a.poll(hm(0, 10))
	saved := roundTrip(t, a.m.StallState(hm(0, 10)))

	b := newStallRig(t, hm(0, 10), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(0, 10))
	b.list("o", "/repo", "idle")
	b.proc("o", 200, hm(0, 8))
	b.poll(hm(0, 10))
	notStalled(t, b.view(hm(1, 0)), "/repo", "01:00, the old process's shell is no one's work")
	if st := b.m.sessions["o"].stall.stop; st != nil {
		t.Errorf("the old process's Stop was restored: %+v", st)
	}
}

// L4, L8 (round 3's M1): a session that ended (SessionEnd) and was resumed under the
// same id before any poll missed it is a start, not ended for ever; the listing of
// the exiting process under its own PID is not.
func TestStallLifeResumeBeforeAPollMisses(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.proc("s", 100, eve(23, 0))
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 50), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	r.hook(hm(1, 0), "SessionEnd", "s", "/repo/w/x", `"reason":"exit"`)
	r.poll(hm(1, 0)) // the exiting process still listed
	if r.m.sessions["s"].live() {
		t.Fatal("01:00: the exiting process's own listing brought the session back")
	}
	r.proc("s", 200, hm(1, 4)) // claude --resume s
	r.poll(hm(1, 5))
	if !r.m.sessions["s"].live() {
		t.Fatal("01:05: the resumed session is not live")
	}
	notStalled(t, r.view(hm(1, 34)), "/repo/w/x", "01:34")
	st := stalledSince(t, r.view(hm(1, 35)), "/repo/w/x", hm(1, 5), "01:35")
	if strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("the exited process's shell is still what it waited on: %q", st.WaitedOn)
	}
}

// L8: another process listed under the id with no SessionEnd heard (a crash, then a
// resume between two polls) ends the old one: its Stop is dropped, a Stop heard since
// the new process started is kept, and the listing is a start.
func TestStallLifeProcessChanged(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		newStop bool
	}{{"no Stop since the resume", false}, {"a Stop since the resume", true}} {
		r := newStallRig(t, hm(0, 0), "")
		r.list("s", "/repo/w/x", "idle")
		r.proc("s", 100, eve(23, 0))
		r.poll(hm(0, 0))
		r.prs(hm(0, 0), prX(152))
		r.hook(hm(0, 50), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[{"schedule":"*/5 * * * *","recurring":true}]`)
		if c.newStop {
			r.hook(hm(1, 4).Add(30*time.Second), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[],"last_assistant_message":"resumed"`)
		}
		r.proc("s", 200, hm(1, 4))
		r.poll(hm(1, 5))
		st := r.m.sessions["s"].stall.stop
		switch {
		case !c.newStop && st != nil:
			t.Errorf("%s: the old process's Stop is kept: %+v", c.name, st)
		case c.newStop && (st == nil || st.last != "resumed"):
			t.Errorf("%s: kept %+v, want the new process's Stop", c.name, st)
		}
		notStalled(t, r.view(hm(1, 34)), "/repo/w/x", c.name+" at 01:34")
		stalledSince(t, r.view(hm(1, 35)), "/repo/w/x", hm(1, 5), c.name+" at 01:35, the old cron gone")
	}
}

// L5, L9: a session set aside while the polls failed and listed again under another
// process is a start, not its old clock.
func TestStallLifeAsideOtherProcess(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.proc("s", 100, eve(23, 0))
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	fail := errors.New("claude agents: timed out")
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	if _, ok := r.m.stall.aside["s"]; !ok {
		t.Fatal("00:39: the session is not set aside; the test needs it so")
	}
	r.proc("s", 200, hm(0, 30))
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, another process")
	stalledSince(t, r.view(hm(1, 10)), "/repo/w/x", hm(0, 40), "01:10")
}

// L10 (round 3's M2): a session that ended and is then forgotten while the polls
// fail is gone, not set aside: resumed after the polls recover, it is a start.
func TestStallLifeEndedIsNotSetAside(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 5), "SessionEnd", "s", "/repo/w/x", `"reason":"exit"`)
	fail := errors.New("claude agents: timed out")
	for at := hm(0, 6); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	if _, ok := r.m.sessions["s"]; ok {
		t.Fatal("00:39: the session is still held; the test needs it forgotten")
	}
	if _, ok := r.m.stall.aside["s"]; ok {
		t.Error("00:39: an ended session was set aside")
	}
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, resumed after the polls recover")
	stalledSince(t, r.view(hm(1, 10)), "/repo/w/x", hm(0, 40), "01:10")
}

// L11: what was set aside or saved is used by the next successful poll or not at all:
// a session that poll does not list, listed later, is a start.
func TestStallLifeUnlistedKeepIsDropped(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	delete(r.agents, "s")
	r.poll(hm(0, 40))
	if _, ok := r.m.stall.aside["s"]; ok {
		t.Error("00:40: a poll that did not list the session kept it aside")
	}

	// The same for what a restart saved, at the first poll after it.
	a := newStallRig(t, hm(0, 0), "")
	a.list("s", "/repo/w/x", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prX(152))
	b := newStallRig(t, hm(0, 10), "")
	b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 10))))
	b.prs(hm(0, 10), prX(152))
	b.poll(hm(0, 10))
	if len(b.m.stall.saved) != 0 {
		t.Errorf("00:10: the first poll did not list the session, and it stays saved: %+v", b.m.stall.saved)
	}

	for name, x := range map[string]*stallRig{"set aside": r, "saved": b} {
		x.list("s", "/repo/w/x", "idle")
		x.poll(hm(0, 50))
		notStalled(t, x.view(hm(1, 19)), "/repo/w/x", name+" at 01:19")
		stalledSince(t, x.view(hm(1, 20)), "/repo/w/x", hm(0, 50), name+" at 01:20, a start at 00:50")
	}
}

// L13 (review round 4's M1): a record heard while its id is set aside continues that
// entry: forgotten while the polls still fail, what it heard is merged back, not lost.
func TestStallLifeHeardContinuesTheAside(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	r.hook(hm(0, 40), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	for at := hm(0, 41); !at.After(hm(1, 15)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	if _, ok := r.m.sessions["s"]; ok {
		t.Fatal("01:15: the heard record is still held; the test needs it forgotten")
	}
	st := stalledSince(t, r.view(hm(1, 16)), "/repo/w/x", hm(0, 40), "01:16, listed again")
	if !strings.Contains(st.WaitedOn, "nothing in flight") || hasWork(st, "turn ended on") {
		t.Errorf("the shell that ended at 00:40 is still what it waited on: %+v", st)
	}
}

// L13, L12: a record heard while its id is saved (a restart, before the first poll)
// is what the next save writes, and forgotten before that poll it is merged back.
func TestStallLifeHeardContinuesTheSaved(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	a := newStallRig(t, eve(23, 50), "")
	a.list("s", "/repo", "idle")
	a.poll(eve(23, 50))
	a.prs(eve(23, 50))
	a.hook(hm(0, 0), "Stop", "s", "/repo",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	a.poll(hm(0, 5))

	b := newStallRig(t, hm(0, 5), "")
	b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 5))))
	b.prs(hm(0, 5))
	b.hook(hm(0, 6), "Stop", "s", "/repo", `"background_tasks":[],"session_crons":[]`)
	if sm := roundTrip(t, b.m.StallState(hm(0, 7))).Sessions["s"]; sm.Stop == nil || len(sm.Stop.Tasks) != 0 {
		t.Errorf("00:07: saved %+v, want the Stop heard at 00:06, with no task", sm.Stop)
	}
	for at := hm(0, 8); !at.After(hm(0, 40)); at = at.Add(time.Minute) {
		b.m.ApplyAgents(nil, fail, at)
	}
	if _, ok := b.m.sessions["s"]; ok {
		t.Fatal("00:40: the heard record is still held; the test needs it forgotten")
	}
	b.list("s", "/repo", "idle")
	notStalled(t, b.view(hm(0, 41)), "/repo", "00:41, the shell ended at 00:06")
	notStalled(t, b.view(hm(1, 30)), "/repo", "01:30")
}

// L7, L13: a session that ends while its id is set aside drops the entry: resumed after
// the polls recover, it is a start, with nothing of the process that ended.
func TestStallLifeEndWhileAside(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	r.hook(hm(0, 40), "SessionEnd", "s", "/repo/w/x", `"reason":"exit"`)
	if _, ok := r.m.stall.aside["s"]; ok {
		t.Error("00:40: the entry set aside outlived the session's end")
	}
	r.poll(hm(0, 45)) // resumed: listed again
	notStalled(t, r.view(hm(1, 14)), "/repo/w/x", "01:14")
	st := stalledSince(t, r.view(hm(1, 15)), "/repo/w/x", hm(0, 45), "01:15, a start at 00:45")
	if strings.Contains(st.WaitedOn, "Run checks") {
		t.Errorf("the ended process's shell is still what it waited on: %q", st.WaitedOn)
	}
}

// D1 (review round 5's M1): a restart keeps a clock's time only when the way resting on
// that clock held; a lane stalled by its session's question carries its own quiet time,
// so the time the panel was down does not count.
func TestStallRestartCarriesOnlyTheWaysClock(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, hm(0, 0), "")
	a.list("s", "/repo/w/x", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prX(152))
	a.hook(hm(0, 10), "PermissionRequest", "s", "/repo/w/x", `"tool_name":"Bash"`)
	a.list("s", "/repo/w/x", "waiting", "permission: Bash")
	a.poll(hm(0, 10))
	a.hook(hm(0, 35), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a9","agent_type":""`)
	stalledSince(t, a.view(hm(0, 45)), "/repo/w/x", hm(0, 10), "00:45, by the question")
	saved := roundTrip(t, a.m.StallState(hm(0, 45)))
	if saved.Lanes["/repo/w/x"].Stalled || saved.Sessions["s"].Stalled || !saved.Sessions["s"].AskStalled {
		t.Errorf("saved %+v / %+v: want only the ask's clock marked stalled", saved.Lanes["/repo/w/x"], saved.Sessions["s"])
	}

	b := newStallRig(t, hm(2, 0), "")
	b.m.ApplyStallState(saved)
	b.prs(hm(2, 0), prX(152))
	b.list("s", "/repo/w/x", "idle") // answered while the panel was down
	b.poll(hm(2, 0))
	notStalled(t, b.view(hm(2, 19)), "/repo/w/x", "02:19")
	stalledSince(t, b.view(hm(2, 20)), "/repo/w/x", hm(1, 50), "02:20, the lane's 10 minutes of quiet carried")
}

// D1, D3 (b) (review round 5's M2): "gate passed, no PR" rests on the pull requests
// too, so it is not judged until `gh pr list` has answered.
func TestStallGatePassedWaitsForPRs(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}, true)
	r.m.ApplyMergedReads(map[string]MergedRead{"/repo/w/x": {Branch: "change/x", Head: headX, Read: true}})
	r.m.ApplyPRs(nil, errors.New("gh: not logged in"), hm(0, 1))
	notStalled(t, r.view(hm(0, 40)), "/repo/w/x", "00:40, the pull requests never read")
	r.prs(hm(0, 41))
	stalledSince(t, r.view(hm(0, 41)), "/repo/w/x", hm(0, 0), "00:41, read: none open")
}

// D11 (review round 5's L1): a stalled waiting row whose ask is approximate says so
// once.
func TestStallApproxQuestionLabelledOnce(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.hook(hm(0, 0), "PermissionRequest", "s", "/repo/w/x", `"tool_name":"Bash"`) // only a hook says so
	v := r.view(hm(0, 30))
	stalledSince(t, v, "/repo/w/x", hm(0, 0), "00:30")
	for _, n := range v.NeedsYou {
		if n.Lane == "/repo/w/x" && strings.Count(n.Label, "≈") != 1 {
			t.Errorf("label %q, want one ≈ mark", n.Label)
		}
	}
}

// L13 with the owner's rule (a) (review round 5's L2): a record heard while its id is
// set aside merges with the entry field by field, so a Stop it heard with no
// background_tasks keeps the entry's tasks.
func TestStallLifeMergeKeepsAbsentFields(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	r.hook(hm(0, 0), "Stop", "s", "/repo/w/x",
		`"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks"}],"session_crons":[]`)
	for at := hm(0, 1); !at.After(hm(0, 39)); at = at.Add(time.Minute) {
		r.m.ApplyAgents(nil, fail, at)
	}
	r.hook(hm(0, 40), "Stop", "s", "/repo/w/x", `"last_assistant_message":"still going"`)
	r.poll(hm(0, 45))
	st := r.m.sessions["s"].stall.stop
	if st == nil || len(st.tasks) != 1 || st.tasks[0].id != "b1" || st.last != "still going" {
		t.Errorf("restored %+v, want the entry's shell and the record's last line", st)
	}
}

// D1 (review round 5's L4): the merged read, once it has answered for every lane that
// needed it, is not named in a warning again when a lane comes to need one later.
func TestStallMergedWarningOnlyBeforeItAnswers(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.m.ApplyMergedReads(map[string]MergedRead{})
	r.m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}, true) // clean at 00:20
	for _, w := range r.view(hm(0, 20)).WarningItems {
		if strings.HasPrefix(w.Key, "stall-source:") {
			t.Errorf("00:20: warning %q, but every source has answered", w.Text)
		}
	}
}

// D10 (group 3's review round 2, L3): a branch gh is never asked about (outside
// config.BranchRe, which mergedPRRead refuses before gh runs) is due no merged read,
// so its receipt stays unknown: the lane is not stalled on it, and no warning says the
// merged read has not answered for it, for good.
func TestStallBranchGhIsNotAskedAboutIsDueNoMergedRead(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/p", Branch: "change/x+y", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	r.list("s", "/repo/w/p", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	x := map[string]LaneExtra{"/repo/w/p": cleanReceipt(headX)}
	if due := r.m.MergedReadsDue(x); len(due) != 0 {
		t.Errorf("merged reads due for %+v, want none: gh is never asked about change/x+y", due)
	}
	r.m.ApplyLaneExtras(x, true)
	r.m.ApplyMergedReads(map[string]MergedRead{}) // the read refused the branch: no result
	v := r.view(hm(0, 45))
	for _, w := range v.WarningItems {
		if strings.HasPrefix(w.Key, "stall-source:") {
			t.Errorf("00:45: warning %q, but no source a way rests on is unanswered", w.Text)
		}
	}
	notStalled(t, v, "/repo/w/p", "00:45")
}

// D1 (review round 6): the first queue read and the first tmux poll after a restart are
// baselines, so a dead holder or a dead pane they already see keeps the saved since.
func TestStallRestartFirstReadsAreBaselines(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, hm(0, 0), gateQueue)
	a.list("s", "/repo/w/x", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prX(7))
	a.m.ApplyQueues(gateHeld("x", true), nil, hm(0, 5))
	a.m.ApplyQueues(gateHeld("x", false), nil, hm(0, 10))
	stalledSince(t, a.view(hm(0, 45)), "/repo/w/x", hm(0, 10), "00:45, the holder dead")
	b := newStallRig(t, hm(1, 0), gateQueue)
	b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 45))))
	b.prs(hm(1, 0), prX(7))
	b.m.ApplyQueues(gateHeld("x", false), nil, hm(1, 0))
	b.list("s", "/repo/w/x", "idle")
	stalledSince(t, b.view(hm(1, 0)), "/repo/w/x", hm(0, 10), "01:00, the restart's first queue read")

	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	dead := []types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "1"}}
	c := newStallRig(t, eve(23, 50), "")
	c.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, recs, "", nil, eve(23, 50))
	c.list("sx", "/repo/w/x", "idle")
	c.poll(eve(23, 50))
	c.prs(eve(23, 50), prX(152))
	c.m.ApplyTmux(dead, recs, "", nil, hm(0, 0))
	delete(c.agents, "sx")
	stalledSince(t, c.view(hm(0, 45)), "/repo/w/x", hm(0, 0), "00:45, claude exited")
	d := newStallRig(t, hm(1, 0), "")
	d.m.ApplyStallState(roundTrip(t, c.m.StallState(hm(0, 45))))
	d.prs(hm(1, 0), prX(152))
	d.m.ApplyTmux(dead, recs, "", nil, hm(1, 0))
	stalledSince(t, d.view(hm(1, 0)), "/repo/w/x", hm(0, 0), "01:00, the restart's first tmux poll")
}

// D1 (review round 6's L1, round 7's L1): a lane's pane coming back is the lane's
// movement on its own, with no listing changing. The panel's Resume is `respawn-pane`
// (lanes.go), which keeps the session's session_created; a tmux session killed and
// recreated within one poll interval reads alive at both polls, and only its
// session_created tells its going and coming back.
func TestStallPaneComingBackMoves(t *testing.T) {
	t.Parallel()
	alive := func(created int64) []types.TmuxLane {
		return []types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: created}}
	}
	dead := []types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "0"}}
	for _, c := range []struct {
		name  string
		polls map[time.Time][]types.TmuxLane
	}{
		{"Resume: the pane dead, then respawned in its session", map[time.Time][]types.TmuxLane{hm(0, 5): dead, hm(0, 20): alive(1)}},
		{"its tmux session killed and recreated between two polls", map[time.Time][]types.TmuxLane{hm(0, 5): alive(1), hm(0, 20): alive(2)}},
	} {
		r := newStallRig(t, hm(0, 0), "")
		recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
		r.m.ApplyTmux(alive(1), recs, "", nil, hm(0, 0))
		r.list("h", "/repo/w/x", "idle") // a session started by hand in the lane, listed throughout
		r.poll(hm(0, 0))
		r.prs(hm(0, 0), prX(152))
		for _, at := range []time.Time{hm(0, 5), hm(0, 20)} {
			r.m.ApplyTmux(c.polls[at], recs, "", nil, at)
		}
		notStalled(t, r.view(hm(0, 49)), "/repo/w/x", c.name+" at 00:49")
		stalledSince(t, r.view(hm(0, 50)), "/repo/w/x", hm(0, 20), c.name+" at 00:50, 30 minutes after the pane came back")
	}
}

// [STALL-1-S25] [STALL-1-S30] A lane stalled at the save keeps its since and its mark (D1 as amended
// 2026-10-20, option B), even when its claude was resumed while the panel was down
// (issue #185, review round 7's M1, which the amendment settles): the sessions of the
// first `claude agents` poll after a start are not movement (D1), and the first tmux
// poll is a baseline (this change's Decision log, not D1's words), so nothing moves the
// lane, the stall stays exact, and nothing is sent again.
func TestStallRestartResumedWhileDownKeepsSince(t *testing.T) {
	t.Parallel()
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x")}
	alive := []types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}
	a := newStallRig(t, eve(23, 50), "")
	a.m.ApplyTmux(alive, recs, "", nil, eve(23, 50))
	a.list("sx", "/repo/w/x", "idle")
	a.poll(eve(23, 50))
	a.prs(eve(23, 50), prX(152))
	a.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "1"}}, recs, "", nil, hm(0, 0))
	delete(a.agents, "sx")
	va := a.view(hm(0, 45))
	stalledSince(t, va, "/repo/w/x", hm(0, 0), "00:45, claude exited")
	n := &Notifier{MinInterval: time.Minute}
	if got := n.Process(va.Alerts, nil, hm(0, 45)); len(got) != 1 {
		t.Fatalf("00:45: %d notices, want 1", len(got))
	}

	b := newStallRig(t, hm(1, 0), "")
	b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 45))))
	b.prs(hm(1, 0), prX(152))
	b.m.ApplyTmux(alive, recs, "", nil, hm(1, 0)) // resumed while the panel was down
	b.list("sx", "/repo/w/x", "idle")
	b.proc("sx", 200, hm(0, 55)) // its new process started after the save
	vb := b.view(hm(1, 0))
	if st := stalledSince(t, vb, "/repo/w/x", hm(0, 0), "01:00, the restart's first polls"); st.Approx {
		t.Error("01:00: x, stalled at the save, is approximate")
	}
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(n.State())
	if got := n2.Process(vb.Alerts, nil, hm(1, 0)); len(got) != 0 {
		t.Errorf("01:00: %+v sent again", got)
	}
}

// [STALL-1-S31] The first queue poll after a start is a baseline: a lane stalled at
// the save, whose gate lease the first poll finds held by a dead holder (taken and
// left while the panel was down), has not moved, so it stays stalled with its saved
// since, exact, and nothing is sent again; a later poll that finds the same dead
// holder is no movement either.
func TestStallRestartFirstQueuePollIsNoMovement(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, hm(0, 0), gateQueue)
	a.list("sx", "/repo/w/x", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prX(152))
	a.m.ApplyQueues(gateFree(), nil, hm(0, 0))
	va := a.view(hm(0, 45))
	stalledSince(t, va, "/repo/w/x", hm(0, 0), "00:45")
	n := &Notifier{MinInterval: time.Minute}
	if got := n.Process(va.Alerts, nil, hm(0, 45)); len(got) != 1 {
		t.Fatalf("00:45: %d notices, want 1", len(got))
	}

	b := newStallRig(t, hm(1, 0), gateQueue)
	b.m.ApplyStallState(roundTrip(t, a.m.StallState(hm(0, 45))))
	b.prs(hm(1, 0), prX(152))
	b.list("sx", "/repo/w/x", "idle")
	b.poll(hm(1, 0))
	b.m.ApplyQueues(gateHeld("x", false), nil, hm(1, 0)) // its holder already dead
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(n.State())
	for _, at := range []time.Time{hm(1, 0), hm(1, 1)} {
		if at.After(hm(1, 0)) {
			b.m.ApplyQueues(gateHeld("x", false), nil, at)
		}
		v := b.view(at)
		if st := stalledSince(t, v, "/repo/w/x", hm(0, 0), clock(at)+", the dead holder the first queue poll found"); st.Approx {
			t.Errorf("%s: x, stalled at the save, is approximate", clock(at))
		}
		if got := n2.Process(v.Alerts, nil, at); len(got) != 0 {
			t.Errorf("%s: %+v sent again", clock(at), got)
		}
	}
}

// [STALL-1-S25] A stall that begins across a restart is approximate until the panel
// has watched for one stall time (D1 amended 2026-10-20, option B). Two instances of
// the class: x holds the gate's lease at the save, not stalled, and the lease is given
// up while the panel is down (round 8's M1); z's session is busy at the save and idle
// at the first poll after it. y is stalled at the save and keeps its since and its mark.
// Every source succeeds at 01:01, so the window rests on the start (01:00), not on the
// sources' first answers. A stall still in its window is saved stalled and
// approximate, so a second restart keeps its since and does not make it exact.
func TestStallRestartBeginsApprox(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main", Head: headY}, {Path: "/repo/w/x", Branch: "change/x", Head: headX},
		{Path: "/repo/w/y", Branch: "change/y", Head: headY}, {Path: "/repo/w/z", Branch: "change/z", Head: headX}}
	prs := []signals.PR{prX(152), {Number: 160, HeadRef: "change/y"}, {Number: 161, HeadRef: "change/z"}}
	a := newStallRig(t, hm(0, 0), gateQueue, wts...)
	a.list("sx", "/repo/w/x", "idle")
	a.list("sy", "/repo/w/y", "idle")
	a.list("sz", "/repo/w/z", "busy")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0), prs...)
	a.m.ApplyQueues(gateHeld("x", true), nil, hm(0, 0))
	n := &Notifier{MinInterval: time.Minute}
	va := a.view(hm(0, 45))
	notStalled(t, va, "/repo/w/x", "00:45, x holds the lease")
	notStalled(t, va, "/repo/w/z", "00:45, z busy")
	stalledSince(t, va, "/repo/w/y", hm(0, 0), "00:45, y")
	if got := n.Process(va.Alerts, nil, hm(0, 45)); len(got) != 1 {
		t.Fatalf("00:45: %d notices, want 1 (y)", len(got))
	}
	saved := roundTrip(t, a.m.StallState(hm(0, 45)))

	b := newStallRig(t, hm(1, 0), gateQueue, wts...)
	b.m.ApplyStallState(saved)
	b.list("sx", "/repo/w/x", "idle")
	b.list("sy", "/repo/w/y", "idle")
	b.list("sz", "/repo/w/z", "idle") // gone idle while the panel was down
	b.prs(hm(1, 1), prs...)
	b.m.ApplyQueues(gateFree(), nil, hm(1, 1)) // given up while the panel was down
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(n.State())
	for _, at := range []time.Time{hm(1, 1), hm(1, 29)} {
		v := b.view(at)
		if v.StallsPending {
			t.Fatalf("%s: every source has answered, yet stalls are pending", clock(at))
		}
		for _, p := range []string{"/repo/w/x", "/repo/w/z"} {
			if st := stalledSince(t, v, p, hm(0, 15), clock(at)+", "+p); !st.Approx {
				t.Errorf("%s: %s's stall began across the restart, yet it is not approximate", clock(at), p)
			}
			if a := stalledAlert(v, p); a != nil && !a.Approx {
				t.Errorf("%s: %s's alert is not approximate: %+v", clock(at), p, *a)
			}
			if l := stalledNeed(t, v, p, clock(at)).Label; strings.Contains(l, "claude agents") || !strings.Contains(l, "restart") {
				t.Errorf("%s: %s's row labelled %q, want the restart as the reason, not claude agents", clock(at), p, l)
			}
		}
		if st := stalledSince(t, v, "/repo/w/y", hm(0, 0), clock(at)+", y"); st.Approx {
			t.Errorf("%s: y, stalled at the save, is approximate", clock(at))
		}
		if got := n2.Process(v.Alerts, nil, at); len(got) != 0 {
			t.Errorf("%s: %+v sent", clock(at), got)
		}
	}
	mid := roundTrip(t, b.m.StallState(hm(1, 29)))
	x, y, z := mid.Lanes["/repo/w/x"], mid.Lanes["/repo/w/y"], mid.Lanes["/repo/w/z"]
	if !x.Stalled || !x.Approx || !z.Stalled || !z.Approx || !y.Stalled || y.Approx {
		t.Errorf("saved at 01:29: x %+v, z %+v, y %+v; want all three stalled, x and z approximate", x, z, y)
	}
	// A second restart inside the window: x keeps its since, still approximate.
	c := newStallRig(t, hm(1, 35), gateQueue, wts...)
	c.m.ApplyStallState(mid)
	c.list("sx", "/repo/w/x", "idle")
	c.list("sy", "/repo/w/y", "idle")
	c.list("sz", "/repo/w/z", "idle")
	c.prs(hm(1, 35), prs...)
	c.m.ApplyQueues(gateFree(), nil, hm(1, 35))
	vc := c.view(hm(1, 35))
	if st := stalledSince(t, vc, "/repo/w/x", hm(0, 15), "01:35, a second restart"); !st.Approx {
		t.Error("01:35: x, approximate at the second save, is exact after it")
	}
	if st := stalledSince(t, vc, "/repo/w/y", hm(0, 0), "01:35, a second restart"); st.Approx {
		t.Error("01:35: y, exact at the second save, is approximate after it")
	}
	v := b.view(hm(1, 30))
	var keys []string
	for _, p := range []string{"/repo/w/x", "/repo/w/z"} {
		if st := stalledSince(t, v, p, hm(0, 15), "01:30, "+p); st.Approx {
			t.Errorf("01:30: the panel has watched for one stall time, yet %s is approximate", p)
		}
	}
	for _, nt := range n2.Process(v.Alerts, nil, hm(1, 30)) {
		keys = append(keys, nt.Keys...)
	}
	sort.Strings(keys)
	if want := []string{AlertStalled + ":/repo/w/x", AlertStalled + ":/repo/w/z"}; strings.Join(keys, " ") != strings.Join(want, " ") {
		t.Errorf("01:30: notified %v, want %v, once each", keys, want)
	}
	if got := n2.Process(b.view(hm(1, 31)).Alerts, nil, hm(1, 31)); len(got) != 0 {
		t.Errorf("01:31: %+v sent again", got)
	}
}

// stalledNeed is the lane's stalled Needs-you row.
func stalledNeed(t *testing.T, v View, path, when string) NeedView {
	t.Helper()
	for _, n := range v.NeedsYou {
		if n.Lane == path && (n.Kind == AlertStalled || strings.HasPrefix(n.Label, "Stalled")) {
			return n // a waiting row whose question stalled the lane is relabelled, not added
		}
	}
	t.Fatalf("%s: no stalled row for %s in Needs you", when, path)
	return NeedView{}
}

// stallNotices counts the notices that carry the lane's stalled alert (a waiting
// session's own banner is another alert, and another count).
func stallNotices(ns []Notice, path string) int {
	c := 0
	for _, nt := range ns {
		for _, k := range nt.Keys {
			if k == AlertStalled+":"+path {
				c++
			}
		}
	}
	return c
}

// [STALL-1-S25] A stall approximate at the save (its `claude agents` reading not
// current) was never exact, so it is not carried exact across a restart (review round
// 9's M1): it is approximate through the window, its clock keeping its time. Case
// "never sent": nothing was notified, so it sends once when the window ends. Case
// "sent, then stale": notified at 00:30, its mark kept while the polls failed, so the
// same since sends nothing at the window's end either (a since moved by the restart
// would send it again). Its row says why it is approximate: the restart, not a stale
// reading (round 9's M2). Each way of stalling, by the clock stall.json marks for it:
// the lane's own (#152 open), a session's turn (a shell in flight, no PR), a question.
func TestStallRestartApproxAtSaveStaysApprox(t *testing.T) {
	t.Parallel()
	fail := errors.New("claude agents: timed out")
	lane := func(r *stallRig, at time.Time) {
		r.list("s", "/repo/w/x", "idle")
		r.prs(at, prX(152))
	}
	turn := func(r *stallRig, at time.Time) {
		r.list("s", "/repo/w/x", "idle")
		r.prs(at)
	}
	ask := func(r *stallRig, at time.Time) {
		r.list("s", "/repo", "waiting", "input needed")
		r.prs(at)
	}
	for _, c := range []struct {
		name     string
		path     string
		setup    func(*stallRig, time.Time)
		goodTo   time.Time // the last successful poll before the save
		sent     int       // notified before the save
		atWindow int       // notified at 01:30
	}{
		{"lane, never sent", "/repo/w/x", lane, hm(0, 19), 0, 1},
		{"lane, sent, then stale", "/repo/w/x", lane, hm(0, 30), 1, 0},
		{"turn, never sent", "/repo/w/x", turn, hm(0, 19), 0, 1},
		{"ask, never sent", "/repo", ask, hm(0, 19), 0, 1},
	} {
		a := newStallRig(t, hm(0, 0), "")
		c.setup(a, hm(0, 0))
		a.poll(hm(0, 0))
		if c.name == "turn, never sent" {
			a.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"id":"b1","type":"shell","status":"running"}],"session_crons":[]`)
		}
		n := &Notifier{MinInterval: time.Minute}
		sent := 0
		for at := hm(0, 1); !at.After(c.goodTo); at = at.Add(time.Minute) {
			v := a.view(at)
			sent += stallNotices(n.Process(v.Alerts, nil, at), c.path)
		}
		for at := c.goodTo.Add(time.Minute); !at.After(hm(0, 45)); at = at.Add(time.Minute) {
			a.m.ApplyAgents(nil, fail, at)
			sent += stallNotices(n.Process(a.m.Snapshot(at).Alerts, nil, at), c.path)
		}
		va := a.m.Snapshot(hm(0, 45))
		if st := stalledSince(t, va, c.path, hm(0, 0), c.name+", 00:45"); !st.Approx {
			t.Fatalf("%s, 00:45: the polls fail, yet the stall is exact", c.name)
		}
		if sent != c.sent {
			t.Fatalf("%s: %d notices before the save, want %d", c.name, sent, c.sent)
		}
		saved := roundTrip(t, a.m.StallState(hm(0, 45)))

		b := newStallRig(t, hm(1, 0), "")
		b.m.ApplyStallState(saved)
		c.setup(b, hm(1, 0))
		n2 := &Notifier{MinInterval: time.Minute}
		n2.Restore(n.State())
		for _, at := range []time.Time{hm(1, 0), hm(1, 29)} {
			v := b.view(at)
			if st := stalledSince(t, v, c.path, hm(0, 0), c.name+", "+clock(at)); !st.Approx {
				t.Errorf("%s, %s: approximate at the save, yet exact inside the window", c.name, clock(at))
			}
			if l := stalledNeed(t, v, c.path, c.name).Label; strings.Contains(l, "claude agents") || !strings.Contains(l, "restart") {
				t.Errorf("%s, %s: row labelled %q, want the restart as the reason, not claude agents", c.name, clock(at), l)
			}
			if got := n2.Process(v.Alerts, nil, at); stallNotices(got, c.path) != 0 {
				t.Errorf("%s, %s: %+v sent", c.name, clock(at), got)
			}
		}
		v := b.view(hm(1, 30))
		if st := stalledSince(t, v, c.path, hm(0, 0), c.name+", 01:30"); st.Approx {
			t.Errorf("%s, 01:30: the window has ended, yet the stall is approximate", c.name)
		}
		if got := stallNotices(n2.Process(v.Alerts, nil, hm(1, 30)), c.path); got != c.atWindow {
			t.Errorf("%s, 01:30: %d stall notices, want %d", c.name, got, c.atWindow)
		}
	}
}

// L13 (review round 10's L1): building the state to save changes nothing it saves. A
// SubagentStop heard before the first poll after a restart continues the saved entry in
// the save, but the saved entry itself keeps its task open until the poll restores it.
func TestStallSaveChangesNothing(t *testing.T) {
	t.Parallel()
	a := newStallRig(t, hm(0, 0), "")
	a.list("o", "/repo", "idle")
	a.poll(hm(0, 0))
	a.prs(hm(0, 0))
	a.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[{"id":"a1","type":"subagent","status":"running"}],"session_crons":[]`)
	saved := roundTrip(t, a.m.StallState(hm(0, 10)))

	b := newStallRig(t, hm(1, 0), "")
	b.m.ApplyStallState(saved)
	b.hook(hm(1, 0), "SubagentStop", "o", "/repo", `"agent_id":"a1","agent_type":"general-purpose"`)
	st := b.m.StallState(hm(1, 0))
	if got := st.Sessions["o"].Stop; got == nil || len(got.Tasks) != 1 || !got.Tasks[0].Ended {
		t.Errorf("saved %+v: want a1 ended, continued by the stop heard since", got)
	}
	if k := b.m.stall.saved["o"]; k.stop == nil || len(k.stop.tasks) != 1 || k.stop.tasks[0].ended {
		t.Errorf("the restored entry %+v was changed by building the save", k.stop)
	}
}

// D3 (d) (review round 10's L2): a task id and the SubagentStop agent id it pairs with
// are bounded alike, so an id past the clip still pairs.
func TestStallLongIDsPair(t *testing.T) {
	t.Parallel()
	id := strings.Repeat("a", stallClip+10)
	r := newStallRig(t, hm(0, 0), "")
	r.list("o", "/repo", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.hook(hm(0, 0), "Stop", "o", "/repo", `"background_tasks":[{"id":"`+id+`","type":"subagent","status":"running"}],"session_crons":[]`)
	r.hook(hm(0, 5), "SubagentStop", "o", "/repo", `"agent_id":"`+id+`","agent_type":"general-purpose"`)
	r.poll(hm(0, 10))
	if r.m.sessionWorking(r.m.sessions["o"], hm(0, 10)) {
		t.Error("00:10: the subagent's stop arrived, yet its session is still working")
	}
}

// D11 (review round 10's L3): an approximate stall's row says why from the way that made
// it so: a question a hook alone reports waiting, with `claude agents` current and
// saying idle (a busy reading clears the note), is not "claude agents hasn't answered".
func TestStallLabelSaysWhy(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0))
	r.hook(hm(0, 0), "Notification", "s", "/repo", `"notification_type":"permission_prompt","message":"Claude needs your permission"`)
	r.poll(hm(0, 30))
	ls := r.m.judgeStalls(hm(0, 30))["/repo"]
	if ls == nil || !ls.approx {
		t.Fatalf("00:30: %+v, want an approximate stall", ls)
	}
	if l := stallLabel(ls); !strings.Contains(l, "a hook says waiting, claude agents says idle") {
		t.Errorf("labelled %q, want the question's own reason", l)
	}
}

// [STALL-4-S2] /api/state carries the stall for the next page.
func TestStallInAPIState(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), "")
	r.list("s", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.prs(hm(0, 0), prX(152))
	b, err := json.Marshal(r.view(hm(0, 30)))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Lanes []struct {
			Path  string                     `json:"path"`
			Stall map[string]json.RawMessage `json:"stall"`
		} `json:"lanes"`
		NeedsYou []NeedView  `json:"needsYou"`
		Alerts   []AlertView `json:"alerts"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range got.Lanes {
		if l.Path != "/repo/w/x" {
			continue
		}
		found = true
		for _, k := range []string{"since", "waitedOn", "openWork", "approx"} {
			if _, ok := l.Stall[k]; !ok {
				t.Errorf("the lane's stall has no %q: %v", k, l.Stall)
			}
		}
	}
	if !found {
		t.Fatal("the lane is not in /api/state")
	}
	need, alert := false, false
	for _, n := range got.NeedsYou {
		need = need || n.Lane == "/repo/w/x" && n.Kind == AlertStalled
	}
	for _, a := range got.Alerts {
		alert = alert || a.Lane == "/repo/w/x" && a.Kind == AlertStalled && a.Severity == signals.SevBlock
	}
	if !need || !alert {
		t.Errorf("needsYou has a stalled item: %v; alerts a stalled alert of severity block: %v", need, alert)
	}
}

// [STALL-4-S3] One Needs-you row per stalled lane.
func TestStallOneNeedPerLane(t *testing.T) {
	t.Parallel()
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/a", Branch: "change/a", Head: headX},
		{Path: "/repo/w/b", Branch: "change/b", Head: headY}, {Path: "/repo/w/c", Branch: "change/c", Head: headX}}
	r := newStallRig(t, hm(0, 0), "", wts...)
	recs := []types.LaneRecord{launched("b", "sb", "/repo/w/b")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "b", Path: "/repo/w/b", Created: 1}}, recs, "", nil, hm(0, 0))
	r.list("a1", "/repo/w/a", "idle")
	r.list("a2", "/repo/w/a", "idle")
	r.list("sb", "/repo/w/b", "idle")
	r.list("sc", "/repo/w/c", "waiting", "permission: Bash(npm test)")
	r.poll(hm(0, 0))
	r.hook(hm(0, 0), "PermissionRequest", "sc", "/repo/w/c", `"tool_name":"Bash"`)
	r.prs(hm(0, 0), signals.PR{Number: 1, HeadRef: "change/a"}, signals.PR{Number: 2, HeadRef: "change/b"}, signals.PR{Number: 3, HeadRef: "change/c"})
	r.m.ApplyTmux([]types.TmuxLane{{ID: "b", Path: "/repo/w/b", Created: 1, Dead: true, DeadStatus: "0"}}, recs, "", nil, hm(0, 0))
	delete(r.agents, "sb")
	v := r.view(hm(0, 30))
	for _, p := range []string{"/repo/w/a", "/repo/w/b", "/repo/w/c"} {
		stalledSince(t, v, p, hm(0, 0), p)
	}
	items := map[string][]NeedView{}
	for _, n := range v.NeedsYou {
		items[n.Lane] = append(items[n.Lane], n)
	}
	for _, p := range []string{"/repo/w/a", "/repo/w/b"} {
		if len(items[p]) != 1 || items[p][0].Kind != AlertStalled {
			t.Errorf("%s: needsYou %+v, want exactly one item of kind stalled", p, items[p])
		}
	}
	if c := items["/repo/w/c"]; len(c) != 1 || c[0].Kind == AlertStalled || !strings.HasPrefix(c[0].Label, "Stalled") {
		t.Errorf("/repo/w/c: needsYou %+v, want exactly its waiting item, labelled stalled", c)
	}
}

// Review round 1, L4 (D10): the merged read serves the stall rule alone, so with
// stall_minutes 0 no lane is due one, page or no page; with stalls on, the lane with a
// clean receipt for its HEAD and no pull request is.
func TestStallsOffDueNoMergedRead(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, extra string
		due         int
	}{{"on", "", 1}, {"off", `,"alerts":{"stall_minutes":0}`, 0}} {
		r := newStallRig(t, hm(0, 0), c.extra)
		r.prs(hm(0, 0))
		if got := r.m.MergedReadsDue(map[string]LaneExtra{"/repo/w/x": cleanReceipt(headX)}); len(got) != c.due {
			t.Errorf("stalls %s: %d lanes due a merged read, want %d", c.name, len(got), c.due)
		}
	}
}

// [STALL-3-S1] The push is built from the stalled alert's own fields (D7): how long
// the lane has been quiet, the open work and the kind of wait, in fixed words, never a
// task's description, the turn's last line or the question's words.
func TestStalledAlertCarriesThePushFields(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, path, wait string
		setup            func(r *stallRig)
		never            []string
	}{
		{"a shell", "/repo/w/x", "a shell in flight", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"last_assistant_message":"Checks are running.",`+
				`"background_tasks":[{"type":"shell","status":"running","description":"Run checks","command":"sh .claude/checks/run.sh"}]`)
		}, []string{"Run checks", "run.sh", "Checks are running"}},
		// A task listed with a status beyond running or pending: the page appends it to
		// the description, and the push's kind of wait follows it, never "in flight"
		// (group 4's review round 1, L2); a status outside the allowlist of ended
		// statuses is not carried at all (group 5's review round 2, L5).
		{"a shell failed", "/repo/w/x", "a shell failed", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"type":"shell","status":"failed","description":"Run checks"}],"session_crons":[]`)
		}, []string{"Run checks", "in flight"}},
		// One plain lowercase word, but not a status Claude Code is known to set: an MCP
		// monitor's status was never probed and could be any word, so only an allowlisted
		// status leaves the machine (group 5's review round 2, 5).
		{"a monitor of a word not allowlisted", "/repo/w/x", "a monitor no longer in flight", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"type":"monitor","status":"hunter","description":"tail"}],"session_crons":[]`)
		}, []string{"hunter", "tail"}},
		{"a monitor of an odd status", "/repo/w/x", "a monitor no longer in flight", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"type":"monitor","status":"Watch the deploy log","description":"tail"}],"session_crons":[]`)
		}, []string{"Watch the deploy", "tail"}},
		{"nothing", "/repo/w/x", "nothing in flight", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[],"session_crons":[],"last_assistant_message":"Waiting for merge-train's reply."`)
		}, []string{"merge-train"}},
		{"a subagent ended", "/repo/w/x", "a subagent that ended and woke nothing", func(r *stallRig) {
			r.list("s", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0), prX(152))
			r.hook(hm(0, 0), "Stop", "s", "/repo/w/x", `"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`)
			r.hook(hm(0, 0), "SubagentStop", "s", "/repo/w/x", `"agent_id":"a1","agent_type":"general-purpose"`)
		}, []string{"Review group 2"}},
		{"a question", "/repo", "a question", func(r *stallRig) {
			r.list("o", "/repo", "busy")
			r.poll(hm(0, 0))
			r.prs(hm(0, 0))
			r.hook(hm(0, 0), "PermissionRequest", "o", "/repo", `"tool_name":"Bash"`)
			r.list("o", "/repo", "waiting", "wants to use Bash")
		}, []string{"Bash"}},
	} {
		r := newStallRig(t, hm(0, 0), "")
		c.setup(r)
		v := r.view(hm(0, 30))
		a := stalledAlert(v, c.path)
		if a == nil {
			t.Fatalf("%s: no stalled alert for %s: %+v", c.name, c.path, v.Alerts)
		}
		if a.Wait != c.wait {
			t.Errorf("%s: the alert's kind of wait %q, want %q", c.name, a.Wait, c.wait)
		}
		title, body := PushText("P", *a, hm(0, 30))
		if c.path == "/repo/w/x" && !strings.Contains(body, "#152") {
			t.Errorf("%s: the push body %q does not name #152", c.name, body)
		}
		if !strings.Contains(body, "quiet for 30m") || !strings.Contains(body, c.wait) {
			t.Errorf("%s: the push body %q, want the quiet time and %q", c.name, body, c.wait)
		}
		if !strings.HasPrefix(title, "P · ") {
			t.Errorf("%s: the push title %q, want the project and the lane", c.name, title)
		}
		for _, w := range c.never {
			if strings.Contains(title+body, w) {
				t.Errorf("%s: the push (%q, %q) carries %q, the model's or the owner's words", c.name, title, body, w)
			}
		}
	}
}
