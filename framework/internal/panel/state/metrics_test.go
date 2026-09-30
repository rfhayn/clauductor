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
