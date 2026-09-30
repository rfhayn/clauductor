package metrics

import (
	"sort"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// What the panel computes itself, from what it already reads (no model token, and
// no network call but the `gh pr list` it makes for merged pull requests on the
// pull requests' own source): merge frequency and PR cycle time from merged pull
// requests, spend from the ledger, and the lanes in flight. The rest of the view
// needs the project's command.

// MergedPR is a merged pull request as `gh pr list --state merged` reports it.
type MergedPR = signals.MergedPR

// Inputs are what the built-in figures are computed from.
type Inputs struct {
	Now       time.Time
	ProjectID string
	Project   string // the project's name: the one line of cost by project
	// Merged are the merged pull requests of the last 90 days, newest first as gh
	// lists them, and the state of their read.
	Merged       []MergedPR
	MergedStatus Status
	// Days is the spend ledger (Ledger.Days).
	Days map[string][]Spend
	// WIP is the work in flight: each lane on a branch of its own, and its age.
	WIP []WIPItem
	// Waiting is how long (hours) each change awaiting its owner's approval has
	// waited so far (PANEL-19, the change directory); nil when none is read.
	Waiting []float64
	// Budgets are the changes' budgets in US dollars, by branch.
	Budgets map[string]float64
}

// bucketing splits a window into equal buckets for its series: days per bucket.
var bucketing = map[string]struct{ n, days int }{"7d": {7, 1}, "30d": {10, 3}, "90d": {15, 6}}

const needsGH = "Read from gh while this page is in view, every 10 minutes."

// Builtin computes the panel's own figures per window. A figure it cannot compute
// has Missing set; one it never computes is absent.
func Builtin(in Inputs) map[string]map[string]*Metric {
	out := map[string]map[string]*Metric{}
	for _, w := range Windows {
		m := map[string]*Metric{}
		mergedFigures(in, w, m)
		spendFigures(in, w, m)
		if len(in.WIP) > 0 {
			items := append([]WIPItem(nil), in.WIP...)
			sort.Slice(items, func(i, j int) bool { return items[i].AgeDays > items[j].AgeDays })
			m["flow.aging_wip"] = &Metric{Items: items, N: len(items), Source: FromBuiltin, Note: "Lanes on a branch of their own, by the age of the lane."}
		}
		if in.Waiting != nil {
			if v, ok := median(in.Waiting); ok {
				m["flow.approval_wait"] = &Metric{Value: ptr(round(v, 1)), N: len(in.Waiting), Source: FromBuiltin,
					Note: "Proposals waiting for approval now, and how long so far; the same in every window."}
			}
		}
		out[w] = m
	}
	return out
}

func mergedFigures(in Inputs, w string, m map[string]*Metric) {
	st := in.MergedStatus
	if !st.OK {
		why := needsGH
		if st.Error != "" {
			why = "gh pr list --state merged failed: " + st.Error
		} else if st.Pending {
			why = "Reading merged pull requests from gh. " + needsGH
		}
		m["flow.merge_frequency"] = &Metric{Missing: why}
		m["flow.cycle_time"] = &Metric{Missing: why}
		return
	}
	days := WindowDays[w]
	b := bucketing[w]
	start := in.Now.Add(-time.Duration(days) * 24 * time.Hour)
	bucketLen := time.Duration(b.days) * 24 * time.Hour
	counts := make([]float64, b.n)
	hours := make([][]float64, b.n)
	var all []float64
	n := 0
	for _, pr := range in.Merged {
		if pr.MergedAt.Before(start) || pr.MergedAt.After(in.Now) {
			continue
		}
		i := int(pr.MergedAt.Sub(start) / bucketLen)
		if i >= b.n {
			i = b.n - 1
		}
		counts[i]++
		n++
		if !pr.CreatedAt.IsZero() && !pr.MergedAt.Before(pr.CreatedAt) {
			h := pr.MergedAt.Sub(pr.CreatedAt).Hours()
			hours[i] = append(hours[i], h)
			all = append(all, h)
		}
	}
	note := ""
	if st.Limited {
		note = "Only the newest merges gh returned; the oldest part of the window may be missing some."
	}
	freq := &Metric{Value: ptr(round(float64(n)*7/float64(days), 2)), N: n, Source: FromBuiltin, Note: note}
	for _, c := range counts {
		freq.Series = append(freq.Series, ptr(round(c*7/float64(b.days), 2)))
	}
	m["flow.merge_frequency"] = freq
	cyc := &Metric{Source: FromBuiltin, N: len(all), Note: note}
	if v, ok := median(all); ok {
		cyc.Value = ptr(round(v, 1))
		if cyc.Note == "" {
			cyc.Note = "Pull request opened to merged, median."
		}
		for _, hs := range hours {
			if v, ok := median(hs); ok {
				cyc.Series = append(cyc.Series, ptr(round(v, 1)))
			} else {
				cyc.Series = append(cyc.Series, nil)
			}
		}
	} else {
		cyc.Missing = "No pull request was merged in the last " + w + "."
	}
	m["flow.cycle_time"] = cyc
}

func spendFigures(in Inputs, w string, m map[string]*Metric) {
	first := FirstDay(in.Days)
	if first == "" {
		why := "No spend recorded yet: it comes from status-line posts (docs/panel.md, The status line)."
		for _, k := range []string{"cost.total", "cost.per_week", "cost.by_role", "cost.by_model", "cost.by_change", "cost.by_project"} {
			m[k] = &Metric{Missing: why}
		}
		return
	}
	days := WindowDays[w]
	b := bucketing[w]
	byType, byModel, byBranch := map[string]float64{}, map[string]float64{}, map[string]float64{}
	buckets := make([]float64, b.n)
	total := 0.0
	// The window's days: the last `days` local days, today included, oldest first.
	span := b.n * b.days
	for i := 0; i < span; i++ {
		day := in.Now.AddDate(0, 0, -(span - 1 - i)).Format("2006-01-02")
		for _, s := range in.Days[day] {
			buckets[i/b.days] += s.USD
			if i >= span-days {
				total += s.USD
				byType[orNone(s.Type, "(no lane type)")] += s.USD
				byModel[orNone(s.Model, "(model not reported)")] += s.USD
				byBranch[orNone(s.Branch, "(detached)")] += s.USD
			}
		}
	}
	note := ""
	start := in.Now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	if first > start {
		note = "Since " + first + ", when the panel began keeping spend."
	}
	m["cost.total"] = &Metric{Value: ptr(round(total, 2)), Source: FromBuiltin, Note: note}
	pw := &Metric{Value: ptr(round(total*7/float64(days), 2)), Source: FromBuiltin, Note: note}
	for _, v := range buckets {
		pw.Series = append(pw.Series, ptr(round(v*7/float64(b.days), 2)))
	}
	m["cost.per_week"] = pw
	m["cost.by_role"] = amounts(byType, nil, "By lane type: the panel knows a lane's type, not the role a skill switched to.")
	m["cost.by_model"] = amounts(byModel, nil, "")
	m["cost.by_change"] = amounts(byBranch, in.Budgets, "By branch.")
	proj := in.Project
	if proj == "" {
		proj = "this project"
	}
	m["cost.by_project"] = amounts(map[string]float64{proj: total}, nil, "")
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}

func amounts(sum map[string]float64, budgets map[string]float64, note string) *Metric {
	items := []Amount{}
	for k, v := range sum {
		a := Amount{Name: k, USD: round(v, 2)}
		if b, ok := budgets[k]; ok {
			a.BudgetUSD = ptr(b)
		}
		items = append(items, a)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].USD != items[j].USD {
			return items[i].USD > items[j].USD
		}
		return items[i].Name < items[j].Name
	})
	return &Metric{Items: items, N: len(items), Source: FromBuiltin, Note: note}
}

// CardItem is one figure of the Flow card.
type CardItem struct {
	Key     string     `json:"key"`
	Value   *float64   `json:"value"`
	Series  []*float64 `json:"series,omitempty"`
	Source  string     `json:"source,omitempty"`
	Missing string     `json:"missing,omitempty"`
}

// Card is the side panel's Flow card: four figures of the 30-day window.
type Card struct {
	Window string     `json:"window"`
	Items  []CardItem `json:"items"`
	// Any: at least one figure has a value, so there are metrics to show.
	Any bool `json:"any"`
}

// FlowKeys are the card's figures, in its order.
var FlowKeys = []string{"flow.cycle_time", "flow.merge_frequency", "flow.change_fail_rate", "cost.per_week"}

// FlowCard picks the card's figures from a report.
func FlowCard(r Report) *Card {
	const w = "30d"
	f := &Card{Window: w}
	for _, k := range FlowKeys {
		m := r.Windows[w][k]
		if m == nil {
			continue
		}
		it := CardItem{Key: k, Value: m.Value, Series: m.Series, Source: m.Source, Missing: m.Missing}
		f.Any = f.Any || m.Value != nil
		f.Items = append(f.Items, it)
	}
	return f
}
