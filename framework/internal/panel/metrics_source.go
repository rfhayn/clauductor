package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/metrics"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-19: the Metrics view's sources. The project's metrics command runs like a
// card (only while the config is trusted); merged pull requests come from one more
// `gh pr list`, on the pull requests' own cadence and only while a page is in view;
// spend comes from the status-line posts the model already folds, kept in the spend
// ledger. None of them costs a model token.

// metricsStore is what the view is built from; the metrics sources write it and the
// route reads it.
type metricsStore struct {
	mu        sync.Mutex
	cmd       metrics.Command
	merged    []metrics.MergedPR
	mergedSt  metrics.Status
	mergedAt  time.Time // the last read of merged pull requests, good or not
	ledger    *metrics.Ledger
	lastCard  string
	mergedDue atomic.Bool // Refresh asks for a read at the next poll
	// From the change directory: how long each unapproved proposal has waited (hours),
	// each change branch's budget, and whether the project has changes at all.
	waiting     []float64
	budgets     map[string]float64
	haveChanges bool
}

func newMetricsStore(o Options, root string, cfg *config.Config, trusted bool) *metricsStore {
	st := &metricsStore{ledger: metrics.OpenLedger(metrics.LedgerPath(o.Home, root))}
	st.cmd = metrics.Command{Configured: len(cfg.MetricsCommand()) > 0, Trusted: trusted, Pending: true}
	st.mergedSt = metrics.Status{Pending: true}
	return st
}

// metricsSources are the metrics command (when the config names one), the merged
// pull requests and the spend ledger.
func (r *Runtime) metricsSources() []*source {
	var out []*source
	if cmd := r.cfg.MetricsCommand(); len(cmd) > 0 {
		rule, _ := config.ParseRefresh(r.cfg.MetricsRefresh()) // validated at load
		fetch := commandFetch(r, 30*time.Second, cmd, metrics.Parse)
		s := &source{name: "metrics", kick: make(chan struct{}, 1),
			poll: func(ctx context.Context, now time.Time) (update, time.Duration) {
				trusted := r.trusted()
				var p *metrics.Payload
				var err error
				if trusted {
					p, err = fetch(ctx)
					if ctx.Err() != nil {
						return nil, 0
					}
				}
				r.mstore.mu.Lock()
				r.mstore.cmd.Trusted = trusted
				if trusted {
					r.mstore.cmd.SetResult(p, err, now)
				}
				r.mstore.mu.Unlock()
				return r.flowUpdate(now), 0
			}}
		r.cardKicks = append(r.cardKicks, s.kick) // trusting it runs it at once, as a card
		if rule.Interval > 0 {
			s.every, s.fixedRate = rule.Interval, true
		} else {
			s.watch = func(context.Context) string { return filepath.Join(r.root, rule.WatchRel) }
			s.watchEvery = r.ticks.CardWatch
		}
		out = append(out, s)
	}
	out = append(out,
		&source{name: "merged", every: r.ticks.PRs, fixedRate: true, kick: make(chan struct{}, 1), poll: r.pollMerged},
		&source{name: "spend", every: r.ticks.Spend, fixedRate: true, waitFirst: true, poll: r.pollSpend},
		&source{name: "changes", every: r.ticks.Spend, fixedRate: true, kick: make(chan struct{}, 1), poll: r.pollChanges},
	)
	return out
}

// pollChanges reads the changes' proposals in every worktree (files only, no
// command) for the approval, budget and budget-bar signals, with what each branch
// has spent.
func (r *Runtime) pollChanges(_ context.Context, now time.Time) (update, time.Duration) {
	var paths []string
	r.hub.Read(func(m *state.Model, _ time.Time) {
		for _, wt := range m.Worktrees() {
			if !wt.Bare && wt.Path != "" {
				paths = append(paths, wt.Path)
			}
		}
	})
	if len(paths) == 0 {
		paths = []string{r.root}
	}
	cs := signals.ReadChanges(paths, signals.ChangesDirs(r.root))
	spent := metrics.ByBranch(r.mstore.ledger.Days())
	var waiting []float64
	budgets := map[string]float64{}
	for _, c := range cs {
		if !c.Approved && !c.Written.IsZero() {
			waiting = append(waiting, now.Sub(c.Written).Hours())
		}
		if c.BudgetUSD != nil {
			for b := range spent {
				if signals.BranchOfChange(b, c.ID) {
					budgets[b] = *c.BudgetUSD
				}
			}
		}
	}
	r.mstore.mu.Lock()
	r.mstore.waiting, r.mstore.budgets, r.mstore.haveChanges = waiting, budgets, len(cs) > 0
	r.mstore.mu.Unlock()
	return func(m *state.Model, _ time.Time) { m.ApplyChanges(cs, spent) }, 0
}

// pollMerged reads merged pull requests at most every Ticks.Merged, and only while a
// page is in view: the figures are for the page, and gh is a network call.
func (r *Runtime) pollMerged(ctx context.Context, now time.Time) (update, time.Duration) {
	st := r.mstore
	st.mu.Lock()
	last := st.mergedAt
	st.mu.Unlock()
	force := st.mergedDue.Swap(false)
	if !r.pageVisible(now) || (!force && !last.IsZero() && now.Sub(last) < r.ticks.Merged) {
		return nil, 0
	}
	since := now.AddDate(0, 0, -90).Format("2006-01-02")
	out, err := r.exec(ctx, 30*time.Second, signals.MergedArgv(since))
	if ctx.Err() != nil {
		return nil, 0
	}
	var prs []signals.MergedPR
	if err == nil {
		prs, err = signals.ParseMergedPRs(out)
	}
	st.mu.Lock()
	st.mergedAt = now
	if err != nil {
		// The last good list stays for the figures; the page says the read failed.
		st.mergedSt = metrics.Status{OK: st.merged != nil, Error: err.Error(), At: now.UnixMilli()}
		if st.merged == nil {
			st.mergedSt.OK = false
		}
	} else {
		st.merged = prs
		st.mergedSt = metrics.Status{OK: true, At: now.UnixMilli(), Limited: len(prs) >= signals.MergedLimit}
	}
	st.mu.Unlock()
	return r.flowUpdate(now), 0
}

// pollSpend moves the spend the model observed into the ledger and saves it.
func (r *Runtime) pollSpend(_ context.Context, now time.Time) (update, time.Duration) {
	var obs []metrics.Observation
	r.hub.Update(func(m *state.Model, _ time.Time) { obs = m.DrainSpend() })
	for _, o := range obs {
		r.mstore.ledger.Observe(o)
	}
	if err := r.mstore.ledger.Save(now); err != nil {
		fmt.Fprintf(r.o.Out, "%s: the spend ledger cannot be saved: %v\n", r.id, err)
	}
	return r.flowUpdate(now), 0
}

// metricsInputs gathers what the built-in figures are computed from.
func (r *Runtime) metricsInputs(now time.Time) metrics.Inputs {
	in := metrics.Inputs{Now: now, ProjectID: r.id, Project: r.cfg.Name}
	r.hub.Read(func(m *state.Model, now time.Time) { in.WIP = m.WIP(now) })
	st := r.mstore
	st.mu.Lock()
	in.Merged, in.MergedStatus = st.merged, st.mergedSt
	in.Budgets = st.budgets
	if st.haveChanges {
		in.Waiting = append([]float64{}, st.waiting...)
	}
	st.mu.Unlock()
	in.Days = st.ledger.Days()
	return in
}

// metricsReport is the project's Metrics view now.
func (r *Runtime) metricsReport() metrics.Report {
	now := r.clock.Now()
	in := r.metricsInputs(now)
	r.mstore.mu.Lock()
	cmd := r.mstore.cmd
	r.mstore.mu.Unlock()
	return metrics.Build(in, cmd)
}

// flowUpdate recomputes the Flow card, and returns the model update that shows it,
// or nil when it has not changed (a model update is pushed to every page).
func (r *Runtime) flowUpdate(time.Time) update {
	card := metrics.FlowCard(r.metricsReport())
	b, _ := json.Marshal(card)
	r.mstore.mu.Lock()
	same := string(b) == r.mstore.lastCard
	r.mstore.lastCard = string(b)
	r.mstore.mu.Unlock()
	if same {
		return nil
	}
	return func(m *state.Model, _ time.Time) { m.ApplyFlow(card) }
}
