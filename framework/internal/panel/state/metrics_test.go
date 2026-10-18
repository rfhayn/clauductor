package state

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/metrics"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-19: a status post's cost reaches the spend ledger with the lane type, branch
// and model it ran as; each session keeps only its latest total until drained.
func TestSpendObservations(t *testing.T) {
	m := alertModel(t, "")
	post := func(sid, cwd string, usd float64) {
		var p signals.StatusPayload
		p.SessionID, p.Cwd = sid, cwd
		p.Model.DisplayName = "Opus"
		p.Cost.TotalCostUSD = &usd
		m.ApplyStatus(p, t0)
	}
	post("s1", "/repo/w/x", 1)
	post("s1", "/repo/w/x", 2.5)
	post("s2", "/repo", 0.5)
	obs := m.DrainSpend()
	if len(obs) != 2 {
		t.Fatalf("observations %+v", obs)
	}
	for _, o := range obs {
		if o.Session == "s1" && (o.TotalUSD != 2.5 || o.Type != "build" || o.Branch != "change/x" || o.Model != "Opus" || o.Day != t0.Local().Format("2006-01-02")) {
			t.Errorf("s1: %+v", o)
		}
		if o.Session == "s2" && (o.Type != "orchestrator" || o.Branch != "main") {
			t.Errorf("s2: %+v", o)
		}
	}
	if len(m.DrainSpend()) != 0 {
		t.Error("a drain keeps what it returned")
	}
}

// The work in flight is each registered lane on a branch of its own.
func TestWIP(t *testing.T) {
	m := alertModel(t, "")
	created := t0.Add(-36 * time.Hour)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}, {ID: "main", Path: "/repo"}},
		[]types.LaneRecord{{ID: "x", Path: "/repo/w/x", Type: "build", Created: created.UnixMilli()}, {ID: "main", Path: "/repo", Type: "orchestrator", Created: created.UnixMilli()}},
		"", nil, t0)
	w := m.WIP(t0)
	if len(w) != 1 || w[0].ID != "x" || w[0].Title != "change/x" || w[0].AgeDays != 1.5 || w[0].Stage != "build" {
		t.Fatalf("WIP %+v", w)
	}
}

// PANEL-19: the Needs-you signals. A proposal with no Approved line waiting past the
// threshold, a change over its budget (every branch of it counts), and a lane with no
// commit for stale_days; each carries its lane when one builds it, and 0 turns it off.
func TestMetricsAlerts(t *testing.T) {
	budget := 10.0
	changes := []signals.Change{
		{ID: "x", Written: t0.Add(-30 * time.Hour), BudgetUSD: &budget},
		{ID: "y", Written: t0.Add(-30 * time.Hour), Approved: true},
		{ID: "z", Written: t0.Add(-2 * time.Hour)},
	}
	setup := func(extra string) *Model {
		m := alertModel(t, extra)
		created := t0.Add(-5 * 24 * time.Hour)
		m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{{ID: "x", Path: "/repo/w/x", Type: "build", Created: created.UnixMilli()}}, "", nil, t0)
		m.ApplyGit("/repo/w/x", signals.GitStat{Branch: "change/x", Head: "abc", LastCommitAt: t0.Add(-4 * 24 * time.Hour).UnixMilli()}, nil, t0)
		m.ApplyChanges(changes, map[string]float64{"change/x": 7, "fix/x": 4.5, "change/other": 100})
		return m
	}
	v := setup("").Snapshot(t0)
	byKey := map[string]AlertView{}
	for _, a := range v.Alerts {
		byKey[a.Key] = a
	}
	ap, ok := byKey["approval_wait:x"]
	if !ok || ap.Severity != signals.SevWarn || ap.Terminal != "x" || ap.Since != t0.Add(-30*time.Hour).UnixMilli() ||
		ap.Text != "change x has waited 1d 6h for approval (no Approved line; alert at 1d)" {
		t.Fatalf("approval %+v", ap)
	}
	if _, ok := byKey["approval_wait:y"]; ok {
		t.Error("an approved proposal raised an approval alert")
	}
	if _, ok := byKey["approval_wait:z"]; ok {
		t.Error("a proposal under the threshold raised an approval alert")
	}
	if b, ok := byKey["budget:x"]; !ok || b.Text != "change x has spent $11.50 of its $10.00 budget" || b.Terminal != "x" {
		t.Fatalf("budget %+v", b)
	}
	if s, ok := byKey["stale:x"]; !ok || s.Text != "no commit on change/x for 4d (alert at 3d)" {
		t.Fatalf("stale %+v", s)
	}
	// The lane's header gets the budget bar.
	var lb *BudgetView
	for _, l := range v.Lanes {
		if l.Branch == "change/x" {
			lb = l.Budget
		}
	}
	if lb == nil || lb.USD != 10 || lb.Spent != 11.5 || lb.Change != "x" {
		t.Fatalf("budget bar %+v", lb)
	}
	// None of them interrupts.
	for _, a := range v.Alerts {
		if interrupts(a) {
			t.Errorf("%s interrupts", a.Key)
		}
	}
	// 0 turns the approval and stale alerts off.
	v = setup(`,"version":4,"alerts":{"approval_wait_hours":0,"stale_days":0}`).Snapshot(t0)
	for _, a := range v.Alerts {
		if a.Kind == AlertApproval || a.Kind == AlertStale {
			t.Errorf("an alert turned off still shows: %+v", a)
		}
	}
	// A lane whose git has not been read is not stale.
	m := alertModel(t, "")
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{{ID: "x", Path: "/repo/w/x", Type: "build", Created: t0.Add(-9 * 24 * time.Hour).UnixMilli()}}, "", nil, t0)
	for _, a := range m.Snapshot(t0).Alerts {
		if a.Kind == AlertStale {
			t.Errorf("stale with no git read: %+v", a)
		}
	}
}

// The Flow card shows while it has a value, unless metrics.card is false.
func TestFlowCardInTheView(t *testing.T) {
	v5 := 5.0
	card := &metrics.Card{Window: "30d", Any: true, Items: []metrics.CardItem{{Key: "flow.cycle_time", Value: &v5}}}
	m := alertModel(t, "")
	m.ApplyFlow(card)
	if v := m.Snapshot(t0); v.Flow == nil || *v.Flow.Items[0].Value != 5 {
		t.Fatalf("flow %+v", v.Flow)
	}
	m.ApplyFlow(&metrics.Card{Window: "30d"})
	if v := m.Snapshot(t0); v.Flow != nil {
		t.Fatal("a card with nothing to show is shown")
	}
	m = alertModel(t, `,"version":4,"metrics":{"card":false}`)
	m.ApplyFlow(card)
	if v := m.Snapshot(t0); v.Flow != nil {
		t.Fatal("metrics.card false still shows the card")
	}
}

// ctxPost is session sid's status post from /repo/w/x as Claude Code sends it on a
// 1M-token model: used_percentage, context_window_size 1000000, and a current_usage
// whose input, cache-creation and cache-read tokens sum to tokens. Its output tokens
// are not context, so they are not in the sum.
func ctxPost(t *testing.T, sid, cwd string, used float64, tokens int64) signals.StatusPayload {
	t.Helper()
	var p signals.StatusPayload
	b := fmt.Sprintf(`{"session_id":%q,"cwd":%q,"context_window":{"used_percentage":%v,"context_window_size":1000000,`+
		`"current_usage":{"input_tokens":2000,"output_tokens":7000,"cache_creation_input_tokens":8000,"cache_read_input_tokens":%d}}}`,
		sid, cwd, used, tokens-10000)
	if err := json.Unmarshal([]byte(b), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// windowRec is lane id's registry record: session sid in path, launched with window
// (0: none).
func windowRec(id, sid, path string, window int) types.LaneRecord {
	return types.LaneRecord{ID: id, SessionID: sid, Path: path, Type: "build", ActionDone: true, ContextWindow: window}
}

// ctxOf is lane path's context as the view shows it: the session's figure, the lane
// row's, the terminal lane's and the lane's metrics', and the metrics' window.
type ctxOf struct {
	session, lane, term, metrics float64
	window                       int64
	lv                           LaneView
	tv                           TermLaneView
}

func readCtx(t *testing.T, v View, path string) ctxOf {
	t.Helper()
	f := func(p *float64) float64 {
		if p == nil {
			return -1
		}
		return *p
	}
	var c ctxOf
	found := false
	for _, lv := range v.Lanes {
		if lv.Path == path && len(lv.Sessions) > 0 {
			c.lv, found = lv, true
			c.session, c.lane, c.metrics, c.window = f(lv.Sessions[0].CtxPct), f(lv.CtxPct), f(lv.Metrics.CtxPct), lv.Metrics.CtxWindow
		}
	}
	for _, tv := range v.Terminals {
		if tv.Path == path {
			c.tv, c.term = tv, f(tv.CtxPct)
		}
	}
	if !found {
		t.Fatalf("no lane with a session at %s: %+v", path, v.Lanes)
	}
	return c
}

// [LANECTX-2-S1] A lane launched with a 200000 window, on a 1M-token model whose
// status line reads used_percentage 18 at 180000 tokens, reads 90% as the session's
// figure and as the lane's metrics' figure, its metrics read a 200000 window, and the
// context alert fires (alerts.context_pct 85).
func TestLANECTX2S1WindowedLaneReadsAgainstItsWindow(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":85}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 18, 180000), t0)
	v := m.Snapshot(t0)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 90 || c.lane != 90 || c.term != 90 || c.metrics != 90 {
		t.Fatalf("context: session %v, lane %v, terminal %v, metrics %v; want 90 each", c.session, c.lane, c.term, c.metrics)
	}
	if c.window != 200000 {
		t.Fatalf("metrics window %d, want 200000 (not context_window_size)", c.window)
	}
	if alertKinds(v)[AlertContext] != signals.SevWarn {
		t.Fatalf("no context alert at 90%%: %+v", v.Alerts)
	}
}

// [LANECTX-2-S2] A lane launched with no window keeps Claude Code's used_percentage:
// 42 reads 42%, whatever its tokens sum to.
func TestLANECTX2S2LaneWithoutWindowKeepsClaudeCodesFigure(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 0)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 42, 180000), t0)
	c := readCtx(t, m.Snapshot(t0), "/repo/w/x")
	if c.session != 42 || c.lane != 42 || c.term != 42 || c.metrics != 42 {
		t.Fatalf("context: session %v, lane %v, terminal %v, metrics %v; want 42 each", c.session, c.lane, c.term, c.metrics)
	}
	if c.window != 1000000 {
		t.Fatalf("metrics window %d, want context_window_size 1000000", c.window)
	}
	// A session the registry does not hold is measured as Claude Code measures it too.
	m.ApplyStatus(ctxPost(t, "other", "/repo", 7, 180000), t0)
	if c := readCtx(t, m.Snapshot(t0), "/repo"); c.session != 7 || c.metrics != 7 {
		t.Fatalf("an unregistered session reads %v / %v, want 7", c.session, c.metrics)
	}
}

// [LANECTX-2-S3] A config edit does not move a running lane's measure: launched at
// 200000 under a config now saying 300000, its 180000 tokens still read 90%; once
// its record says 300000 (a restart), the next post reads against 300000.
func TestLANECTX2S3ConfigEditDoesNotMoveARunningLane(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"version":6,"launch":{"context_window":300000}`)
	if m.cfg.ContextWindowOf("build") != 300000 {
		t.Fatal("the config does not say 300000")
	}
	tl := []types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}
	m.ApplyTmux(tl, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 18, 180000), t0)
	if c := readCtx(t, m.Snapshot(t0), "/repo/w/x"); c.session != 90 || c.metrics != 90 || c.window != 200000 {
		t.Fatalf("before the restart: %v%% / %v%% of %d, want 90%% of 200000", c.session, c.metrics, c.window)
	}
	m.ApplyTmux(tl, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 300000)}, "", nil, t0.Add(time.Second))
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 18, 180000), t0.Add(2*time.Second))
	if c := readCtx(t, m.Snapshot(t0.Add(2*time.Second)), "/repo/w/x"); c.session != 60 || c.metrics != 60 || c.window != 300000 {
		t.Fatalf("after the restart: %v%% / %v%% of %d, want 60%% of 300000", c.session, c.metrics, c.window)
	}
}

// [LANECTX-2-S5] The auto-compact tick is per lane, where Claude Code compacts: a
// 200000 lane's at 83.5, a 100000 lane's at 67, and a lane with no window keeps 95.
// It is on the terminal lane from its record, and on the lane row and its metrics
// once the lane posts.
func TestLANECTX2S5TickSitsWhereClaudeCodeCompacts(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}, {Path: "/repo/w/y", Branch: "change/y"}}, nil, t0)
	m.ApplyTmux([]types.TmuxLane{{ID: "a", Path: "/repo/w/x"}, {ID: "b", Path: "/repo/w/y"}, {ID: "c", Path: "/repo"}},
		[]types.LaneRecord{windowRec("a", "sa", "/repo/w/x", 200000), windowRec("b", "sb", "/repo/w/y", 100000), windowRec("c", "sc", "/repo", 0)}, "", nil, t0)
	want := map[string]float64{"/repo/w/x": 83.5, "/repo/w/y": 67, "/repo": 95}
	for _, tv := range m.Snapshot(t0).Terminals {
		if tv.CtxTick != want[tv.Path] {
			t.Errorf("terminal lane %s: tick %v before it posts, want %v", tv.Path, tv.CtxTick, want[tv.Path])
		}
	}
	for sid, path := range map[string]string{"sa": "/repo/w/x", "sb": "/repo/w/y", "sc": "/repo"} {
		m.ApplyStatus(ctxPost(t, sid, path, 5, 50000), t0)
	}
	v := m.Snapshot(t0)
	for path, tick := range want {
		c := readCtx(t, v, path)
		if c.lv.CtxTick != tick || c.lv.Metrics.CtxTick != tick || c.tv.CtxTick != tick {
			t.Errorf("%s: tick lane %v, metrics %v, terminal %v; want %v", path, c.lv.CtxTick, c.lv.Metrics.CtxTick, c.tv.CtxTick, tick)
		}
	}
	if ctxTick(300000) != 89 || ctxTick(0) != AutocompactPct {
		t.Errorf("ctxTick(300000) %v, want 89; no window %v, want %v", ctxTick(300000), ctxTick(0), AutocompactPct)
	}
}
