package panel

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// TemplateConfig is one lane recipe: a lane type, a branch pattern and the first
// prompt the panel types into the lane once claude is ready.
type TemplateConfig struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// LaneType must be one of the config's lane types.
	LaneType string `json:"lane_type"`
	// BranchPattern names the new branch, e.g. "change/{name}". "" means the lane
	// type's own prefix followed by the name. It must contain {name}.
	BranchPattern string `json:"branch_pattern"`
	// FirstPrompt is one line of text with {name} and {issue} placeholders.
	FirstPrompt string `json:"first_prompt"`
	// Model and Effort override the lane type's launch options.
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// QueueConfig is one shared resource held as a lease on disk (see lease.go).
type QueueConfig struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Lock is the lease directory, relative to the git common dir, so every worktree
	// of the project agrees on one path.
	Lock string `json:"lock"`
	// Command, if set, is an argv the panel can run through the queue (RUN).
	Command []string `json:"command"`
}

// AlertConfig sets alert thresholds. A missing key takes the default; 0 turns that
// alert off.
type AlertConfig struct {
	IdleMinutes        *float64 `json:"idle_minutes"`
	ContextPct         *float64 `json:"context_pct"`
	FiveHourPct        *float64 `json:"five_hour_pct"`
	WaitingSeconds     *float64 `json:"waiting_seconds"`
	Notify             *bool    `json:"notify"`
	MinIntervalSeconds *float64 `json:"min_interval_seconds"`
}

// QuotaGuardConfig refuses new lanes at or above a 5-hour quota percentage.
type QuotaGuardConfig struct {
	FiveHourPct *float64 `json:"five_hour_pct"`
}

// Alert and guard defaults. The panel interrupts for these only; everything else
// waits on the page to be pulled.
const (
	DefaultIdleMinutes    = 30
	DefaultContextPct     = 85
	DefaultFiveHourPct    = 90
	DefaultWaitingSeconds = 120
	DefaultNotifyInterval = 300
	DefaultQuotaGuardPct  = 95
)

// Thresholds are the resolved alert settings. A zero duration or percentage is off.
type Thresholds struct {
	Idle        time.Duration `json:"-"`
	IdleMinutes float64       `json:"idleMinutes"`
	ContextPct  float64       `json:"contextPct"`
	FiveHourPct float64       `json:"fiveHourPct"`
	Waiting     time.Duration `json:"-"`
	WaitingSecs float64       `json:"waitingSeconds"`
	Notify      bool          `json:"notify"`
	MinInterval time.Duration `json:"-"`
	GuardPct    float64       `json:"quotaGuardPct"`
}

func orDefault(p *float64, d float64) float64 {
	if p == nil {
		return d
	}
	return *p
}

// AlertThresholds resolves the alert config against the defaults.
func (c *Config) AlertThresholds() Thresholds {
	a := c.Alerts
	if a == nil {
		a = &AlertConfig{}
	}
	t := Thresholds{
		IdleMinutes: orDefault(a.IdleMinutes, DefaultIdleMinutes),
		ContextPct:  orDefault(a.ContextPct, DefaultContextPct),
		FiveHourPct: orDefault(a.FiveHourPct, DefaultFiveHourPct),
		WaitingSecs: orDefault(a.WaitingSeconds, DefaultWaitingSeconds),
		Notify:      a.Notify == nil || *a.Notify,
	}
	t.Idle = time.Duration(t.IdleMinutes * float64(time.Minute))
	t.Waiting = time.Duration(t.WaitingSecs * float64(time.Second))
	t.MinInterval = time.Duration(orDefault(a.MinIntervalSeconds, DefaultNotifyInterval) * float64(time.Second))
	if c.QuotaGuard != nil {
		t.GuardPct = orDefault(c.QuotaGuard.FiveHourPct, DefaultQuotaGuardPct)
	} else {
		t.GuardPct = DefaultQuotaGuardPct
	}
	return t
}

var (
	placeholderRe = regexp.MustCompile(`\{([A-Za-z_]+)\}`)
	lockPathRe    = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]{0,199}$`)
)

// knownPlaceholders are the only placeholders a template may use.
var knownPlaceholders = map[string]bool{"name": true, "issue": true}

// maxFirstPrompt caps a rendered first prompt; it is typed into claude's input box.
const maxFirstPrompt = 4000

// maxPlaceholderValue caps a value typed into the dialog.
const maxPlaceholderValue = 200

func (c *Config) validateV2() error {
	if c.Version < 0 || c.Version > 2 {
		return fmt.Errorf("panel config: version %d is not supported (this panel reads 1 and 2)", c.Version)
	}
	seen := map[string]bool{}
	for i, t := range c.Templates {
		if !cardIDRe.MatchString(t.ID) {
			return fmt.Errorf("panel config: templates[%d].id %q must match %s", i, t.ID, cardIDRe)
		}
		if seen[t.ID] {
			return fmt.Errorf("panel config: duplicate template id %q", t.ID)
		}
		seen[t.ID] = true
		if !c.HasLaneType(t.LaneType) {
			return fmt.Errorf("panel config: template %q: lane_type %q is not a lane type in lanes or lane_types", t.ID, t.LaneType)
		}
		if t.BranchPattern == "" && c.BranchPrefix(t.LaneType) == "" {
			return fmt.Errorf("panel config: template %q: lane type %q has no branch prefix, so the template needs a branch_pattern", t.ID, t.LaneType)
		}
		if t.BranchPattern != "" {
			if !strings.Contains(t.BranchPattern, "{name}") {
				return fmt.Errorf("panel config: template %q: branch_pattern %q must contain {name}", t.ID, t.BranchPattern)
			}
			if err := checkPlaceholders(t.BranchPattern); err != nil {
				return fmt.Errorf("panel config: template %q: branch_pattern: %w", t.ID, err)
			}
			if b := fillPlaceholders(t.BranchPattern, map[string]string{"name": "x", "issue": "1"}); !branchRe.MatchString(b) {
				return fmt.Errorf("panel config: template %q: branch_pattern %q does not make a valid branch name", t.ID, t.BranchPattern)
			}
		}
		if strings.TrimSpace(t.FirstPrompt) == "" {
			return fmt.Errorf("panel config: template %q needs a first_prompt", t.ID)
		}
		if err := checkPlaceholders(t.FirstPrompt); err != nil {
			return fmt.Errorf("panel config: template %q: first_prompt: %w", t.ID, err)
		}
		if err := typableText(t.FirstPrompt, maxFirstPrompt); err != nil {
			return fmt.Errorf("panel config: template %q: first_prompt %w", t.ID, err)
		}
		for key, v := range map[string]string{"model": t.Model, "effort": t.Effort} {
			if v != "" && !launchOptRe.MatchString(v) {
				return fmt.Errorf("panel config: template %q: %s %q must match %s", t.ID, key, v, launchOptRe)
			}
		}
	}
	qseen := map[string]bool{}
	for i, q := range c.Queues {
		if !cardIDRe.MatchString(q.ID) {
			return fmt.Errorf("panel config: queues[%d].id %q must match %s", i, q.ID, cardIDRe)
		}
		if qseen[q.ID] {
			return fmt.Errorf("panel config: duplicate queue id %q", q.ID)
		}
		qseen[q.ID] = true
		clean := filepath.Clean(q.Lock)
		if !lockPathRe.MatchString(q.Lock) || filepath.IsAbs(q.Lock) || clean == "." || clean == ".." ||
			strings.HasPrefix(clean, "../") || strings.Contains(q.Lock, "..") {
			return fmt.Errorf("panel config: queue %q: lock %q must be a relative path inside the git common dir", q.ID, q.Lock)
		}
		if q.Command != nil && (len(q.Command) == 0 || strings.TrimSpace(q.Command[0]) == "") {
			return fmt.Errorf("panel config: queue %q: command must be a non-empty argv list", q.ID)
		}
	}
	if a := c.Alerts; a != nil {
		for key, p := range map[string]*float64{"idle_minutes": a.IdleMinutes, "context_pct": a.ContextPct,
			"five_hour_pct": a.FiveHourPct, "waiting_seconds": a.WaitingSeconds, "min_interval_seconds": a.MinIntervalSeconds} {
			if p != nil && *p < 0 {
				return fmt.Errorf("panel config: alerts.%s must be 0 (off) or positive", key)
			}
		}
	}
	if g := c.QuotaGuard; g != nil && g.FiveHourPct != nil && (*g.FiveHourPct < 0 || *g.FiveHourPct > 100) {
		return fmt.Errorf("panel config: quota_guard.five_hour_pct must be 0 (off) to 100")
	}
	return nil
}

func checkPlaceholders(s string) error {
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		if !knownPlaceholders[m[1]] {
			return fmt.Errorf("unknown placeholder {%s} (only {name} and {issue})", m[1])
		}
	}
	return nil
}

func fillPlaceholders(s string, vals map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		return vals[m[1:len(m)-1]]
	})
}

// usesPlaceholder reports whether a template needs a value for key.
func (t TemplateConfig) usesPlaceholder(key string) bool {
	return strings.Contains(t.BranchPattern, "{"+key+"}") || strings.Contains(t.FirstPrompt, "{"+key+"}")
}

// typableText rejects what must never be typed into a lane: a newline (it would
// submit early), any other control character (an escape sequence drives the TUI),
// and invisible format characters such as bidi overrides.
func typableText(s string, max int) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("is not valid UTF-8")
	}
	if utf8.RuneCountInString(s) > max {
		return fmt.Errorf("is longer than %d characters", max)
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
			return fmt.Errorf("contains a newline or control character (%U); it must be one line of plain text", r)
		}
	}
	return nil
}

// RenderedTemplate is a template filled in for one lane.
type RenderedTemplate struct {
	Template    string `json:"template"`
	LaneType    string `json:"laneType"`
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	FirstPrompt string `json:"firstPrompt"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
}

// RenderTemplate fills a template's placeholders. Every value is validated before it
// is used: the name is a lane id; the issue is one line of plain text, and must also
// make a valid branch name when the pattern uses it.
func (c *Config) RenderTemplate(id, name, issue string) (RenderedTemplate, error) {
	var t *TemplateConfig
	for i := range c.Templates {
		if c.Templates[i].ID == id {
			t = &c.Templates[i]
		}
	}
	if t == nil {
		return RenderedTemplate{}, fmt.Errorf("unknown template %q", id)
	}
	if !ValidLaneID(name) {
		return RenderedTemplate{}, fmt.Errorf("lane name %q must match %s", name, laneIDRe)
	}
	issue = strings.TrimSpace(issue)
	if t.usesPlaceholder("issue") {
		if issue == "" {
			return RenderedTemplate{}, fmt.Errorf("template %q needs an issue", id)
		}
		if err := typableText(issue, maxPlaceholderValue); err != nil {
			return RenderedTemplate{}, fmt.Errorf("issue %w", err)
		}
	} else if issue != "" {
		return RenderedTemplate{}, fmt.Errorf("template %q takes no issue", id)
	}
	vals := map[string]string{"name": name, "issue": issue}
	r := RenderedTemplate{Template: t.ID, LaneType: t.LaneType, Name: name, Model: t.Model, Effort: t.Effort}
	if t.BranchPattern != "" {
		r.Branch = fillPlaceholders(t.BranchPattern, vals)
	} else {
		r.Branch = c.BranchPrefix(t.LaneType) + name
	}
	if !branchRe.MatchString(r.Branch) || strings.Contains(r.Branch, "..") {
		return RenderedTemplate{}, fmt.Errorf("branch %q is not a valid branch name", r.Branch)
	}
	r.FirstPrompt = fillPlaceholders(t.FirstPrompt, vals)
	if err := typableText(r.FirstPrompt, maxFirstPrompt); err != nil {
		return RenderedTemplate{}, fmt.Errorf("first prompt %w", err)
	}
	return r, nil
}

// TemplateInfo describes one template for the Start dialog.
type TemplateInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	LaneType    string `json:"laneType"`
	Branch      string `json:"branchPattern"`
	FirstPrompt string `json:"firstPrompt"`
	NeedsIssue  bool   `json:"needsIssue"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
}

// TemplateList returns the templates for the Start dialog.
func (c *Config) TemplateList() []TemplateInfo {
	out := make([]TemplateInfo, 0, len(c.Templates))
	for _, t := range c.Templates {
		b := t.BranchPattern
		if b == "" {
			b = c.BranchPrefix(t.LaneType) + "{name}"
		}
		out = append(out, TemplateInfo{ID: t.ID, Title: t.Title, LaneType: t.LaneType, Branch: b,
			FirstPrompt: t.FirstPrompt, NeedsIssue: t.usesPlaceholder("issue"), Model: t.Model, Effort: t.Effort})
	}
	return out
}
