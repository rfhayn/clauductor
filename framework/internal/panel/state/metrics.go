package state

import (
	"fmt"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/metrics"
	"github.com/clauductor/clauductor/internal/panel/signals"
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

// ApplyChanges records the project's changes (their proposals) and what each branch
// has spent (the spend ledger), for the approval, budget and budget-bar signals.
func (m *Model) ApplyChanges(cs []signals.Change, spent map[string]float64) {
	m.changes, m.spentByBranch = cs, spent
}

// BudgetView is a lane's change budget, as its header draws it.
type BudgetView struct {
	Change string  `json:"change"`
	USD    float64 `json:"usd"`
	Spent  float64 `json:"spent"`
}

// changeOf is the change a branch is building, or nil.
func (m *Model) changeOf(branch string) *signals.Change {
	for i := range m.changes {
		if signals.BranchOfChange(branch, m.changes[i].ID) {
			return &m.changes[i]
		}
	}
	return nil
}

// changeSpent is what every branch of a change has spent.
func (m *Model) changeSpent(id string) float64 {
	sum := 0.0
	for b, v := range m.spentByBranch {
		if signals.BranchOfChange(b, id) {
			sum += v
		}
	}
	return sum
}

// budgetOf is the budget bar of a lane on this branch, or nil.
func (m *Model) budgetOf(branch string) *BudgetView {
	c := m.changeOf(branch)
	if c == nil || c.BudgetUSD == nil {
		return nil
	}
	return &BudgetView{Change: c.ID, USD: *c.BudgetUSD, Spent: m.changeSpent(c.ID)}
}

// Needs-you signals from the metrics (PANEL-19). They are warnings, so they show on the
// page and never interrupt (the notifier sends only what blocks).
const (
	AlertApproval = "approval_wait" // a proposal waiting for approval longer than approval_wait_hours
	AlertBudget   = "budget"        // a change that has spent more than its budget
	AlertStale    = "stale"         // a lane's branch with no commit for stale_days
)

// metricsAlerts are the approval, budget and stale alerts. A lane building the change
// (a branch named for it) gives the alert its lane, so the page can jump there.
func (m *Model) metricsAlerts(v *View, th config.Thresholds, now time.Time) []AlertView {
	var out []AlertView
	laneOf := func(branch func(string) bool) AlertView {
		for _, l := range v.Lanes {
			if branch(l.Branch) {
				return AlertView{Lane: l.Path, Name: l.Name, Terminal: l.Terminal}
			}
		}
		return AlertView{}
	}
	for _, c := range m.changes {
		id := c.ID
		base := laneOf(func(b string) bool { return signals.BranchOfChange(b, id) })
		if th.ApprovalWaitHours > 0 && !c.Approved && !c.Written.IsZero() {
			if w := now.Sub(c.Written); w >= time.Duration(th.ApprovalWaitHours*float64(time.Hour)) {
				a := base
				a.Key, a.Kind, a.Severity, a.Since = AlertApproval+":"+id, AlertApproval, signals.SevWarn, ms(c.Written)
				a.Text = fmt.Sprintf("change %s has waited %s for approval (no Approved line; alert at %s)", id, span(w), span(time.Duration(th.ApprovalWaitHours*float64(time.Hour))))
				out = append(out, a)
			}
		}
		if c.BudgetUSD != nil {
			if spent := m.changeSpent(id); spent > *c.BudgetUSD {
				a := base
				a.Key, a.Kind, a.Severity = AlertBudget+":"+id, AlertBudget, signals.SevWarn
				a.Text = fmt.Sprintf("change %s has spent $%.2f of its $%.2f budget", id, spent, *c.BudgetUSD)
				out = append(out, a)
			}
		}
	}
	if th.StaleDays > 0 {
		limit := time.Duration(th.StaleDays * 24 * float64(time.Hour))
		for _, tv := range v.Terminals {
			if !tv.Registered || tv.Worktree == "" || tv.Worktree == m.root || tv.Branch == "" {
				continue
			}
			last := time.Unix(tv.Created, 0)
			if g := m.trendGit(tv.Worktree); g != nil && g.LastCommitAt > 0 && time.UnixMilli(g.LastCommitAt).After(last) {
				last = time.UnixMilli(g.LastCommitAt)
			} else if g == nil || g.Error != "" {
				continue // not read yet: no commit time is not a stale lane
			}
			if d := now.Sub(last); d >= limit {
				out = append(out, AlertView{Key: AlertStale + ":" + tv.ID, Kind: AlertStale, Severity: signals.SevWarn, Terminal: tv.ID,
					Lane: tv.Worktree, Name: tv.ID, Since: ms(last),
					Text: fmt.Sprintf("no commit on %s for %s (alert at %s)", tv.Branch, span(d), span(limit))})
			}
		}
	}
	return out
}

// EconomyRole is one role economy mode moves to a cheaper tier, as the project's
// .claude/model-roles.json maps it ("economy").
type EconomyRole struct {
	Role string `json:"role"`
	To   string `json:"to,omitempty"` // "sonnet, low"; "" when the mapping does not say
}

// EconomyView is economy mode (PANEL-19): on while the 5-hour quota is at or above
// quota_economy.five_hour_pct. The page shows a badge by the quota while Active.
type EconomyView struct {
	Active    bool          `json:"active"`
	Since     int64         `json:"since,omitempty"` // unix ms of the last switch
	Reason    string        `json:"reason,omitempty"`
	Threshold float64       `json:"threshold"`
	Roles     []EconomyRole `json:"roles"`
	// RolesNote says why no role is named (no model-roles.json, or no economy mapping).
	RolesNote string `json:"rolesNote,omitempty"`
}

// ApplyEconomy records economy mode's state (the machine's); nil: not configured.
func (m *Model) ApplyEconomy(e *EconomyView) {
	if e == nil {
		m.economy = nil
		return
	}
	c := *e
	if m.economy != nil {
		c.Roles, c.RolesNote = m.economy.Roles, m.economy.RolesNote
	}
	m.economy = &c
}

// ApplyEconomyRoles records the project's economy mapping.
func (m *Model) ApplyEconomyRoles(roles []EconomyRole, note string) {
	if m.economy == nil {
		m.economy = &EconomyView{}
	}
	m.economy.Roles, m.economy.RolesNote = roles, note
}

// economyView is the badge's data, while economy mode is on.
func (m *Model) economyView() *EconomyView {
	if m.economy == nil || !m.economy.Active {
		return nil
	}
	c := *m.economy
	if c.Roles == nil {
		c.Roles = []EconomyRole{}
	}
	return &c
}

// trendGit is a worktree's last git read, or nil.
func (m *Model) trendGit(path string) *GitView {
	if m.trend == nil {
		return nil
	}
	return m.trend.git[path]
}

// span is a duration in days and hours, or hours and minutes under a day.
func span(d time.Duration) string {
	if d >= 24*time.Hour {
		days := int(d / (24 * time.Hour))
		if h := int((d % (24 * time.Hour)) / time.Hour); h > 0 {
			return fmt.Sprintf("%dd %dh", days, h)
		}
		return fmt.Sprintf("%dd", days)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}
