package metrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Metric is one figure as the page draws it: a value (with its series and count),
// or a list of items, and where it came from, the project's command ("project") or
// the panel's own reading ("builtin"). With neither, Missing says why, and the page
// shows "—" beside it.
type Metric struct {
	Value   *float64   `json:"value"`
	Series  []*float64 `json:"series,omitempty"`
	N       int        `json:"n,omitempty"`
	Note    string     `json:"note,omitempty"`
	Items   any        `json:"items,omitempty"`
	Source  string     `json:"source,omitempty"`
	Missing string     `json:"missing,omitempty"`
}

// Sources of a figure.
const (
	FromProject = "project"
	FromBuiltin = "builtin"
)

// Keys are every figure the view has, section.name, in the order it shows them.
var Keys = []string{
	"flow.cycle_time", "flow.merge_frequency", "flow.lead_time", "flow.approval_wait", "flow.change_fail_rate", "flow.aging_wip",
	"cost.total", "cost.per_week", "cost.by_role", "cost.by_model", "cost.by_change", "cost.by_project",
	"quality.review_rounds", "quality.reviewer_recall", "quality.escaped_defects",
	"outcomes.hypotheses",
}

// additive figures add up across projects; the others are medians or rates.
var additive = map[string]bool{"flow.merge_frequency": true, "cost.total": true, "cost.per_week": true, "quality.escaped_defects": true}

// Command is the state of the project's metrics command, as the view reports it.
type Command struct {
	Configured bool   `json:"configured"`
	Trusted    bool   `json:"trusted"`
	Pending    bool   `json:"pending,omitempty"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	At         int64  `json:"at,omitempty"`          // unix ms of the last run
	Generated  int64  `json:"generatedAt,omitempty"` // unix ms the payload says it was computed
	payload    *Payload
}

// SetResult records a run of the command.
func (c *Command) SetResult(p *Payload, err error, now time.Time) {
	c.Pending, c.At = false, now.UnixMilli()
	if err != nil {
		c.OK, c.Error = false, err.Error()
		return
	}
	c.OK, c.Error, c.payload, c.Generated = true, "", p, 0
	if p.GeneratedAt > 0 {
		c.Generated = p.GeneratedAt * 1000
	}
}

// Payload returns the last good payload, or nil. A failed run keeps the last good
// one's figures off the page: a failure is shown, never old data as current.
func (c *Command) Payload() *Payload {
	if !c.OK {
		return nil
	}
	return c.payload
}

// Status of one built-in source.
type Status struct {
	OK      bool   `json:"ok"`
	Pending bool   `json:"pending,omitempty"`
	Error   string `json:"error,omitempty"`
	At      int64  `json:"at,omitempty"`
	// Limited: the read stopped at its limit, so the oldest part of a window may lack data.
	Limited bool `json:"limited,omitempty"`
}

// Report is the Metrics view of one project, or of all of them.
type Report struct {
	Project  string   `json:"project,omitempty"`
	Name     string   `json:"name"`
	Projects []string `json:"projects,omitempty"` // scope all: the projects in it
	At       int64    `json:"at"`
	Command  Command  `json:"command"`
	Merged   Status   `json:"merged"`
	// SpendSince is the first day the spend ledger has (2006-01-02), "" with none.
	SpendSince string                        `json:"spendSince,omitempty"`
	Windows    map[string]map[string]*Metric `json:"windows"`
	// Errors are each project's command error, for scope all.
	Errors []string `json:"errors,omitempty"`
}

// Build assembles a project's report: for each window and figure, the project's
// value when its command reports one, else the panel's own, else why there is none.
func Build(in Inputs, cmd Command) Report {
	built := Builtin(in)
	r := Report{Project: in.ProjectID, Name: in.Project, At: in.Now.UnixMilli(), Command: cmd, Merged: in.MergedStatus,
		SpendSince: FirstDay(in.Days), Windows: map[string]map[string]*Metric{}}
	p := cmd.Payload()
	for _, w := range Windows {
		var pw *Window
		if p != nil {
			pw = p.Windows[w]
		}
		proj := projectMetrics(pw)
		out := map[string]*Metric{}
		for _, k := range Keys {
			switch {
			case proj[k] != nil:
				m := proj[k]
				m.Source = FromProject
				out[k] = m
			case built[w][k] != nil && built[w][k].Missing == "":
				out[k] = built[w][k]
			default:
				m := &Metric{Missing: missingWhy(cmd, p != nil, pw != nil, k, w)}
				if b := built[w][k]; b != nil && b.Missing != "" {
					m.Missing = b.Missing
				}
				out[k] = m
			}
		}
		r.Windows[w] = out
	}
	return r
}

// missingWhy says why the project's command gives no figure for k.
func missingWhy(cmd Command, havePayload, haveWindow bool, k, w string) string {
	switch {
	case !cmd.Configured:
		return "Only a project's metrics command reports this (metrics.command in panel.json; docs/panel.md, Metrics)."
	case !cmd.Trusted:
		return "The metrics command is off until you trust panel.json (clauductor panel trust)."
	case cmd.Pending:
		return "The metrics command has not reported yet."
	case !havePayload:
		return "The metrics command failed: " + cmd.Error
	case !haveWindow:
		return "The metrics command reports no " + w + " window."
	}
	return "The metrics command does not report " + strings.ReplaceAll(k, "_", " ") + "."
}

func statMetric(s *Stat) *Metric {
	if s == nil {
		return nil
	}
	m := &Metric{Value: s.Value, Series: s.Series, N: s.N, Note: s.Note}
	if s.Value == nil {
		m.Missing = "The metrics command reports no value"
		if s.Note != "" {
			m.Missing += ": " + s.Note
		}
		m.Missing += "."
	}
	return m
}

func listMetric[T any](items []T) *Metric {
	if items == nil {
		return nil
	}
	return &Metric{Items: items, N: len(items)}
}

// projectMetrics flattens a payload's window into the view's keys; a key the
// payload does not set is absent.
func projectMetrics(w *Window) map[string]*Metric {
	out := map[string]*Metric{}
	if w == nil {
		return out
	}
	set := func(k string, m *Metric) {
		if m != nil {
			out[k] = m
		}
	}
	if f := w.Flow; f != nil {
		set("flow.lead_time", statMetric(f.LeadTime))
		set("flow.cycle_time", statMetric(f.CycleTime))
		set("flow.approval_wait", statMetric(f.ApprovalWait))
		set("flow.merge_frequency", statMetric(f.MergeFrequency))
		set("flow.change_fail_rate", statMetric(f.ChangeFailRate))
		set("flow.aging_wip", listMetric(f.AgingWIP))
	}
	if c := w.Cost; c != nil {
		if c.TotalUSD != nil {
			v := *c.TotalUSD
			set("cost.total", &Metric{Value: &v})
		}
		set("cost.per_week", statMetric(c.PerWeek))
		set("cost.by_role", listMetric(c.ByRole))
		set("cost.by_model", listMetric(c.ByModel))
		set("cost.by_change", listMetric(c.ByChange))
		set("cost.by_project", listMetric(c.ByProject))
	}
	if q := w.Quality; q != nil {
		set("quality.review_rounds", statMetric(q.ReviewRounds))
		set("quality.reviewer_recall", listMetric(q.ReviewerRecall))
		set("quality.escaped_defects", statMetric(q.EscapedDefects))
	}
	if o := w.Outcomes; o != nil {
		set("outcomes.hypotheses", listMetric(o.Hypotheses))
	}
	return out
}

// Combine is the report of every project: additive figures (merges, spend, escaped
// defects) are summed, and their series with them when every project's has the same
// length; a median or a rate cannot be summed, so it is the projects' values
// weighted by how many items each summarises, and says so; lists are joined, each
// item named with its project. A figure no project has keeps the first reason.
func Combine(reps []Report, now time.Time) Report {
	out := Report{Name: "All projects", At: now.UnixMilli(), Windows: map[string]map[string]*Metric{}}
	for _, r := range reps {
		out.Projects = append(out.Projects, r.Name)
		if r.Command.Configured && !r.Command.OK && r.Command.Error != "" {
			out.Errors = append(out.Errors, r.Name+": "+r.Command.Error)
		}
		if r.SpendSince != "" && (out.SpendSince == "" || r.SpendSince < out.SpendSince) {
			out.SpendSince = r.SpendSince
		}
	}
	for _, w := range Windows {
		win := map[string]*Metric{}
		for _, k := range Keys {
			var have []*Metric
			var names []string
			var first *Metric
			for _, r := range reps {
				m := r.Windows[w][k]
				if m == nil {
					continue
				}
				if first == nil {
					first = m
				}
				if m.Missing == "" {
					have = append(have, m)
					names = append(names, r.Name)
				}
			}
			// Cost by project is each project's total, whatever the projects say of it.
			var byProject []Amount
			var totals []*Metric
			if k == "cost.by_project" {
				for _, r := range reps {
					if t := r.Windows[w]["cost.total"]; t != nil && t.Value != nil {
						byProject = append(byProject, Amount{Name: r.Name, USD: *t.Value})
						totals = append(totals, t)
					}
				}
				sort.Slice(byProject, func(i, j int) bool { return byProject[i].USD > byProject[j].USD })
			}
			switch {
			case len(byProject) > 0:
				win[k] = &Metric{Items: byProject, N: len(byProject), Source: mixedSource(totals)}
			case len(have) == 0:
				if first == nil {
					first = &Metric{Missing: "No project reports this."}
				}
				win[k] = &Metric{Missing: first.Missing}
			default:
				win[k] = combineOne(k, have, names)
			}
		}
		out.Windows[w] = win
	}
	return out
}

func mixedSource(ms []*Metric) string {
	src := ""
	for _, m := range ms {
		if src == "" {
			src = m.Source
		} else if m.Source != src {
			return "mixed"
		}
	}
	return src
}

func combineOne(k string, have []*Metric, names []string) *Metric {
	m := &Metric{Source: mixedSource(have)}
	if have[0].Items != nil {
		var items []map[string]any
		for i, h := range have {
			for _, it := range toMaps(h.Items) {
				it["project"] = names[i]
				items = append(items, it)
			}
		}
		m.Items, m.N = items, len(items)
		return m
	}
	if additive[k] {
		sum, n := 0.0, 0
		sameLen := true
		for _, h := range have {
			if h.Value != nil {
				sum += *h.Value
			}
			n += h.N
			sameLen = sameLen && len(h.Series) == len(have[0].Series)
		}
		m.Value, m.N = &sum, n
		if sameLen && len(have[0].Series) > 0 {
			m.Series = make([]*float64, len(have[0].Series))
			for i := range m.Series {
				var s *float64
				for _, h := range have {
					if v := h.Series[i]; v != nil {
						if s == nil {
							s = new(float64)
						}
						*s += *v
					}
				}
				m.Series[i] = s
			}
		}
		return m
	}
	if len(have) == 1 {
		c := *have[0]
		c.Note = strings.TrimSpace(c.Note + " " + names[0] + " only.")
		return &c
	}
	sum, weight, n := 0.0, 0.0, 0
	for _, h := range have {
		if h.Value == nil {
			continue
		}
		w := float64(h.N)
		if w <= 0 {
			w = 1
		}
		sum += *h.Value * w
		weight += w
		n += h.N
	}
	if weight == 0 {
		return &Metric{Missing: "No project has a value."}
	}
	v := sum / weight
	m.Value, m.N = &v, n
	m.Note = "The projects' values weighted by how many each summarises, not one median over all of them."
	return m
}

// toMaps turns a typed list into maps, so combined items can carry their project.
func toMaps(items any) []map[string]any {
	var out []map[string]any
	switch v := items.(type) {
	case []map[string]any:
		for _, it := range v {
			c := map[string]any{}
			for k, x := range it {
				c[k] = x
			}
			out = append(out, c)
		}
	case []WIPItem:
		for _, it := range v {
			out = append(out, map[string]any{"id": it.ID, "title": it.Title, "age_days": it.AgeDays, "stage": it.Stage})
		}
	case []Amount:
		for _, it := range v {
			m := map[string]any{"name": it.Name, "usd": it.USD}
			if it.BudgetUSD != nil {
				m["budget_usd"] = *it.BudgetUSD
			}
			out = append(out, m)
		}
	case []Recall:
		for _, it := range v {
			out = append(out, map[string]any{"model": it.Model, "pct": it.Pct, "n": it.N})
		}
	case []Hypothesis:
		for _, it := range v {
			out = append(out, map[string]any{"change": it.Change, "hypothesis": it.Hypothesis, "due": it.Due, "checked": it.Checked, "result": it.Result})
		}
	}
	return out
}

// ---- small maths ----

func ptr(v float64) *float64 { return &v }

func median(v []float64) (float64, bool) {
	if len(v) == 0 {
		return 0, false
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2], true
	}
	return (s[n/2-1] + s[n/2]) / 2, true
}

func round(v float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	return math.Round(v*p) / p
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
