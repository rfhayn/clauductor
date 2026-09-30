package state

import (
	"time"

	"github.com/clauductor/clauductor/internal/panel/metrics"
)

// PANEL-19: what the model hands the Metrics view, and what it shows of it. The
// view's figures are the runtime's (package metrics); the model gives it the spend
// the status line reports and the lanes in flight, and carries the Flow card.

// maxSpendObs caps the observations waiting for the runtime to take them; each
// session keeps only its latest (its total is cumulative), so this is sessions.
const maxSpendObs = 2000

// observeSpend keeps a session's latest cumulative cost for the spend ledger, with
// where it ran: the lane type, branch and model it had at that post.
func (m *Model) observeSpend(sessionID string, total float64, now time.Time) {
	if m.spendObs == nil {
		m.spendObs = map[string]metrics.Observation{}
	}
	if _, ok := m.spendObs[sessionID]; !ok && len(m.spendObs) >= maxSpendObs {
		return
	}
	o := metrics.Observation{Session: sessionID, TotalUSD: total, Day: now.Local().Format("2006-01-02"), At: now}
	if s := m.sessions[sessionID]; s != nil {
		o.Model = s.Model
		if wt := m.worktreeByPath(s.Lane); wt.Path != "" {
			o.Branch = wt.Branch
			o.Type, _ = m.cfg.LaneFor(wt.Branch)
		}
	}
	m.spendObs[sessionID] = o
}

// DrainSpend returns the spend observations since the last call, and forgets them.
func (m *Model) DrainSpend() []metrics.Observation {
	out := make([]metrics.Observation, 0, len(m.spendObs))
	for _, o := range m.spendObs {
		out = append(out, o)
	}
	m.spendObs = nil
	return out
}

// WIP is the work in flight for the Metrics view: every registered lane on a branch
// of its own (not the project root's checkout), with how long the lane has run.
func (m *Model) WIP(now time.Time) []metrics.WIPItem {
	out := []metrics.WIPItem{}
	for _, tv := range m.TerminalViews(now) {
		if !tv.Registered || tv.Branch == "" || tv.Worktree == "" || tv.Worktree == m.root || tv.Created == 0 {
			continue
		}
		age := now.Sub(time.Unix(tv.Created, 0)).Hours() / 24
		if age < 0 {
			age = 0
		}
		out = append(out, metrics.WIPItem{ID: tv.ID, Title: tv.Branch, AgeDays: float64(int(age*10)) / 10, Stage: tv.Type})
	}
	return out
}

// ApplyFlow sets the side panel's Flow card; nil hides it.
func (m *Model) ApplyFlow(c *metrics.Card) { m.flow = c }
