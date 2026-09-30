package state

import (
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
