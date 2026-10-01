// Package metrics is the Metrics view's data (PANEL-19): the JSON contract a
// project's metrics command prints, and its strict validation; the metrics the panel
// computes itself from what it already reads (merged pull requests from `gh`, spend
// from status-line posts, the lanes in flight); the spend ledger that keeps spend
// across restarts; and the report the page draws, which marks every figure as the
// project's or the panel's own, or says why it has none.
//
// It does no I/O but the ledger's file, and never reads the clock: every function
// takes `now`.
package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ContractVersion is the version of the metrics JSON this panel reads.
const ContractVersion = 1

// Windows are the ranges a payload may report, in the order the view offers them.
var Windows = []string{"7d", "30d", "90d"}

// WindowDays is each window's length in days.
var WindowDays = map[string]int{"7d": 7, "30d": 30, "90d": 90}

// Limits of a payload: past them it is refused, never drawn in part.
const (
	MaxPayload = 1 << 20 // bytes; the runner truncates a command's stdout at 1 MB
	maxItems   = 500     // entries in any one list
	maxSeries  = 120     // points in one series
	maxText    = 300     // characters in one string
)

// Payload is what a metrics command prints: per window, four sections, every one
// optional. Figures have fixed units, so a payload carries numbers, never units:
// durations in hours, frequencies per week, rates in percent, money in US dollars.
type Payload struct {
	Version int `json:"version"`
	// GeneratedAt is when the command computed it (unix seconds), shown as the age.
	GeneratedAt int64              `json:"generated_at,omitempty"`
	Windows     map[string]*Window `json:"windows"`
}

// Window is one range's sections.
type Window struct {
	Flow     *Flow     `json:"flow,omitempty"`
	Cost     *Cost     `json:"cost,omitempty"`
	Quality  *Quality  `json:"quality,omitempty"`
	Outcomes *Outcomes `json:"outcomes,omitempty"`
}

// Stat is one figure: its value over the window (null: none), optionally a series
// of the same figure over equal buckets of the window, oldest first (a null point
// is a bucket with no data), how many items it summarises, and a note.
type Stat struct {
	Value  *float64   `json:"value"`
	Series []*float64 `json:"series,omitempty"`
	N      int        `json:"n,omitempty"`
	Note   string     `json:"note,omitempty"`
}

// Flow is DORA and flow: lead and cycle time and approval wait (median hours),
// merge frequency (per week), change-fail rate (percent) and aging work in progress.
type Flow struct {
	LeadTime       *Stat     `json:"lead_time,omitempty"`
	CycleTime      *Stat     `json:"cycle_time,omitempty"`
	ApprovalWait   *Stat     `json:"approval_wait,omitempty"`
	MergeFrequency *Stat     `json:"merge_frequency,omitempty"`
	ChangeFailRate *Stat     `json:"change_fail_rate,omitempty"`
	AgingWIP       []WIPItem `json:"aging_wip,omitempty"`
}

// WIPItem is one piece of work in flight and how long it has been.
type WIPItem struct {
	ID      string  `json:"id"`
	Title   string  `json:"title,omitempty"`
	AgeDays float64 `json:"age_days"`
	Stage   string  `json:"stage,omitempty"`
}

// Cost is spend in US dollars: the window's total, per week, and broken down.
type Cost struct {
	TotalUSD  *float64 `json:"total_usd,omitempty"`
	PerWeek   *Stat    `json:"per_week,omitempty"`
	ByRole    []Amount `json:"by_role,omitempty"`
	ByModel   []Amount `json:"by_model,omitempty"`
	ByChange  []Amount `json:"by_change,omitempty"`
	ByProject []Amount `json:"by_project,omitempty"`
}

// Amount is one line of a breakdown. BudgetUSD is a change's budget, when it has one.
type Amount struct {
	Name      string   `json:"name"`
	USD       float64  `json:"usd"`
	BudgetUSD *float64 `json:"budget_usd,omitempty"`
}

// Quality is review rounds per change (median), the reviewer's eval recall by
// model (percent), and defects that escaped to main (a count).
type Quality struct {
	ReviewRounds   *Stat    `json:"review_rounds,omitempty"`
	ReviewerRecall []Recall `json:"reviewer_recall,omitempty"`
	EscapedDefects *Stat    `json:"escaped_defects,omitempty"`
}

// Recall is the reviewer's recall on the seeded-defect suite for one model.
type Recall struct {
	Model string  `json:"model"`
	Pct   float64 `json:"pct"`
	N     int     `json:"n,omitempty"`
}

// Outcomes are each change's hypothesis: when to look, and whether anyone did.
type Outcomes struct {
	Hypotheses []Hypothesis `json:"hypotheses,omitempty"`
}

// Hypothesis is one change's "How we'll know".
type Hypothesis struct {
	Change     string `json:"change"`
	Hypothesis string `json:"hypothesis"`
	Due        string `json:"due,omitempty"` // YYYY-MM-DD
	Checked    bool   `json:"checked"`
	Result     string `json:"result,omitempty"`
}

var dueRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Parse reads a metrics command's stdout strictly: unknown keys, a wrong version,
// a window other than 7d, 30d or 90d, a negative or non-finite figure, a percent
// above 100, text with control characters, and lists past their limits are all
// refused with the path of what is wrong. A payload is drawn whole or not at all.
func Parse(out []byte) (*Payload, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("the metrics command printed nothing (want the JSON in docs/panel.md, Metrics)")
	}
	if len(out) >= MaxPayload {
		return nil, fmt.Errorf("the metrics output is %d bytes or more; the limit is %d", MaxPayload, MaxPayload-1)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.DisallowUnknownFields()
	var p Payload
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("metrics JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("metrics JSON: more than one value")
	}
	if p.Version != ContractVersion {
		return nil, fmt.Errorf("metrics JSON: version %d is not supported (this panel reads version %d)", p.Version, ContractVersion)
	}
	if p.GeneratedAt < 0 {
		return nil, fmt.Errorf("metrics JSON: generated_at must be unix seconds")
	}
	if p.Windows == nil {
		return nil, fmt.Errorf("metrics JSON: windows is required (an object keyed 7d, 30d, 90d)")
	}
	for k, w := range p.Windows {
		if _, ok := WindowDays[k]; !ok {
			return nil, fmt.Errorf("metrics JSON: windows.%s: a window is 7d, 30d or 90d", k)
		}
		if w == nil {
			return nil, fmt.Errorf("metrics JSON: windows.%s is null", k)
		}
		if err := w.check("windows." + k); err != nil {
			return nil, fmt.Errorf("metrics JSON: %w", err)
		}
	}
	return &p, nil
}

type checker struct{ err error }

func (c *checker) fail(path, format string, a ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("%s: %s", path, fmt.Sprintf(format, a...))
	}
}

func (c *checker) num(path string, v float64, maxV float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		c.fail(path, "must be a number, 0 or more")
	} else if maxV > 0 && v > maxV {
		c.fail(path, "must be at most %g", maxV)
	}
}

func (c *checker) text(path, s string, required bool) {
	switch {
	case required && strings.TrimSpace(s) == "":
		c.fail(path, "is required")
	case !utf8.ValidString(s):
		c.fail(path, "is not valid UTF-8")
	case utf8.RuneCountInString(s) > maxText:
		c.fail(path, "is longer than %d characters", maxText)
	default:
		for _, r := range s {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				c.fail(path, "contains a control character (%U)", r)
				return
			}
		}
	}
}

func (c *checker) list(path string, n int) {
	if n > maxItems {
		c.fail(path, "has %d entries; the limit is %d", n, maxItems)
	}
}

func (c *checker) stat(path string, s *Stat, maxV float64) {
	if s == nil {
		return
	}
	if s.Value != nil {
		c.num(path+".value", *s.Value, maxV)
	}
	if len(s.Series) > maxSeries {
		c.fail(path+".series", "has %d points; the limit is %d", len(s.Series), maxSeries)
	}
	for i, v := range s.Series {
		if v != nil {
			c.num(fmt.Sprintf("%s.series[%d]", path, i), *v, maxV)
		}
	}
	if s.N < 0 {
		c.fail(path+".n", "must be 0 or more")
	}
	c.text(path+".note", s.Note, false)
}

func (c *checker) amounts(path string, as []Amount) {
	c.list(path, len(as))
	for i, a := range as {
		p := fmt.Sprintf("%s[%d]", path, i)
		c.text(p+".name", a.Name, true)
		c.num(p+".usd", a.USD, 0)
		if a.BudgetUSD != nil {
			c.num(p+".budget_usd", *a.BudgetUSD, 0)
		}
	}
}

func (w *Window) check(path string) error {
	c := &checker{}
	if f := w.Flow; f != nil {
		p := path + ".flow"
		c.stat(p+".lead_time", f.LeadTime, 0)
		c.stat(p+".cycle_time", f.CycleTime, 0)
		c.stat(p+".approval_wait", f.ApprovalWait, 0)
		c.stat(p+".merge_frequency", f.MergeFrequency, 0)
		c.stat(p+".change_fail_rate", f.ChangeFailRate, 100)
		c.list(p+".aging_wip", len(f.AgingWIP))
		for i, it := range f.AgingWIP {
			q := fmt.Sprintf("%s.aging_wip[%d]", p, i)
			c.text(q+".id", it.ID, true)
			c.text(q+".title", it.Title, false)
			c.text(q+".stage", it.Stage, false)
			c.num(q+".age_days", it.AgeDays, 0)
		}
	}
	if k := w.Cost; k != nil {
		p := path + ".cost"
		if k.TotalUSD != nil {
			c.num(p+".total_usd", *k.TotalUSD, 0)
		}
		c.stat(p+".per_week", k.PerWeek, 0)
		c.amounts(p+".by_role", k.ByRole)
		c.amounts(p+".by_model", k.ByModel)
		c.amounts(p+".by_change", k.ByChange)
		c.amounts(p+".by_project", k.ByProject)
	}
	if q := w.Quality; q != nil {
		p := path + ".quality"
		c.stat(p+".review_rounds", q.ReviewRounds, 0)
		c.stat(p+".escaped_defects", q.EscapedDefects, 0)
		c.list(p+".reviewer_recall", len(q.ReviewerRecall))
		for i, r := range q.ReviewerRecall {
			s := fmt.Sprintf("%s.reviewer_recall[%d]", p, i)
			c.text(s+".model", r.Model, true)
			c.num(s+".pct", r.Pct, 100)
			if r.N < 0 {
				c.fail(s+".n", "must be 0 or more")
			}
		}
	}
	if o := w.Outcomes; o != nil {
		p := path + ".outcomes.hypotheses"
		c.list(p, len(o.Hypotheses))
		for i, h := range o.Hypotheses {
			s := fmt.Sprintf("%s[%d]", p, i)
			c.text(s+".change", h.Change, true)
			c.text(s+".hypothesis", h.Hypothesis, true)
			c.text(s+".result", h.Result, false)
			if h.Due != "" && !dueRe.MatchString(h.Due) {
				c.fail(s+".due", "must be a date, YYYY-MM-DD")
			}
		}
	}
	return c.err
}
