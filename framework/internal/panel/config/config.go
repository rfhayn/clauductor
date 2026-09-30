// Package config is a project's panel configuration (.clauductor/panel.json): its
// parsing and validation, templates, alert thresholds, and the validation rules of
// every name the panel turns into argv (lane ids, branches, launch options). It also
// says where the panel keeps its own files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/clauductor/clauductor/internal/panel/types"
)

// DefaultConfigRel is where a project keeps its panel config, relative to the project root.
const DefaultConfigRel = ".clauductor/panel.json"

// Config is the per-project panel configuration (.clauductor/panel.json).
type Config struct {
	// Schema is the JSON Schema the file follows, for editors (SchemaURL). Ignored.
	Schema string `json:"$schema,omitempty"`
	// Name is shown in the top bar.
	Name string `json:"name"`
	// Lanes maps a branch rule to a lane type. A key ending in "/" is a prefix
	// ("change/" matches "change/add-x"); a key ending in "*" is a prefix without the
	// star ("change/propose-*"); any other key matches one branch exactly ("main").
	// The longest matching rule wins.
	Lanes map[string]string `json:"lanes"`
	// Cards are project commands whose stdout renders as a card.
	Cards []CardConfig `json:"cards"`

	// Lanes the panel starts itself. All optional; see the Default* constants.

	// TmuxSocket is the name of the panel's own tmux server (`tmux -L <name>`), so
	// lanes never mix with the user's own tmux sessions.
	TmuxSocket string `json:"tmux_socket"`
	// WorktreeDir is where a new lane's worktree is created: relative to the project
	// root (and inside it), or absolute.
	WorktreeDir string `json:"worktree_dir"`
	// Base is the commit-ish a new lane's branch starts from.
	Base string `json:"base"`
	// LaneTypes adds per-type launch options (model, effort) keyed by lane type.
	LaneTypes map[string]LaneTypeConfig `json:"lane_types"`

	// Orchestration: version 2 keys, all optional; see TemplateConfig and below.

	// Version is the config version the file is written for (MinVersion to
	// LatestVersion); 0 when the file declares none, which reads as LatestVersion.
	// Which key needs which version is in Fields.
	Version int `json:"version,omitempty"`
	// Templates are lane recipes offered in the Start dialog.
	Templates []TemplateConfig `json:"templates"`
	// Queues are shared resources held as an on-disk lease: a full test gate, say,
	// that binds a fixed port and so must never run twice at once.
	Queues []types.QueueConfig `json:"queues"`
	// Alerts sets the alert thresholds and the OS notifications.
	Alerts *AlertConfig `json:"alerts"`
	// QuotaGuard refuses to start a lane above a 5-hour quota threshold.
	QuotaGuard *QuotaGuardConfig `json:"quota_guard"`
	// HostNames are extra names the panel answers to, each "<label>.localhost"
	// (clauductor.localhost always works). No wildcards.
	HostNames []string `json:"host_names"`

	// Metrics: version 4 (PANEL-19).

	// Metrics names the project's metrics command, whose JSON the Metrics view draws
	// beside what the panel computes itself (package metrics).
	Metrics *MetricsConfig `json:"metrics"`

	// Notices are what loading the file has to say once (no version declared). The
	// panel prints them at start.
	Notices []string `json:"-"`
}

// LaneTypeConfig holds the launch options of one lane type.
type LaneTypeConfig struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// Defaults for the lane keys.
const (
	DefaultTmuxSocket  = "clauductor"
	DefaultWorktreeDir = ".claude/worktrees"
	DefaultBase        = "origin/main"
)

var (
	SocketNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// A model or effort value becomes one argv element of `claude`; keep it to the
	// characters real values use ("opus", "claude-opus-4-5[1m]", "high").
	LaunchOptRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`)
	baseRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@{}^~-]{0,199}$`)
	// nameRe: a name starts, after any spaces, with a character that is neither
	// white space nor "-", so it is never blank and never read as an option. The
	// class is spelt out: Go's and ECMAScript's \s differ, and the schema carries it.
	nameRe = regexp.MustCompile(`^ *[^ \t\n\f\r\v-]`)
	// refreshRe is the shape of a card refresh rule; ParseRefresh adds what a
	// pattern cannot say (the watch path stays inside the project).
	refreshRe = regexp.MustCompile(`^(watch:.+|interval:0*[1-9][0-9]*)$`)
)

// Socket returns the tmux socket name, defaulted.
func (c *Config) Socket() string {
	if c.TmuxSocket == "" {
		return DefaultTmuxSocket
	}
	return c.TmuxSocket
}

// BaseRef returns the base commit-ish for new lanes, defaulted.
func (c *Config) BaseRef() string {
	if c.Base == "" {
		return DefaultBase
	}
	return c.Base
}

// WorktreeRoot returns the absolute directory new lane worktrees go in.
func (c *Config) WorktreeRoot(projectRoot string) string {
	d := c.WorktreeDir
	if d == "" {
		d = DefaultWorktreeDir
	}
	if filepath.IsAbs(d) {
		return filepath.Clean(d)
	}
	return filepath.Join(projectRoot, d)
}

// LaneTypeInfo describes one lane type for the Start dialog.
type LaneTypeInfo struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"` // branch prefix for a new branch; "" when the type has none
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// LaneTypeList returns every lane type the config names (the values of lanes and
// the keys of lane_types), sorted.
func (c *Config) LaneTypeList() []LaneTypeInfo {
	names := map[string]bool{}
	for _, v := range c.Lanes {
		names[v] = true
	}
	for k := range c.LaneTypes {
		names[k] = true
	}
	out := make([]LaneTypeInfo, 0, len(names))
	for n := range names {
		lt := c.LaneTypes[n]
		out = append(out, LaneTypeInfo{Name: n, Prefix: c.BranchPrefix(n), Model: lt.Model, Effort: lt.Effort})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasLaneType reports whether the config names this lane type.
func (c *Config) HasLaneType(name string) bool {
	for _, lt := range c.LaneTypeList() {
		if lt.Name == name {
			return true
		}
	}
	return false
}

// BranchPrefix returns the prefix a new branch of this lane type gets: the shortest
// prefix rule ("fix/", or "change/propose-" for "change/propose-*") that maps to it,
// or "" if only exact rules (such as "main") map to it.
func (c *Config) BranchPrefix(laneType string) string {
	best := ""
	for k, v := range c.Lanes {
		if v != laneType {
			continue
		}
		var p string
		switch {
		case strings.HasSuffix(k, "*"):
			p = strings.TrimSuffix(k, "*")
		case strings.HasSuffix(k, "/"):
			p = k
		default:
			continue
		}
		if best == "" || len(p) < len(best) || (len(p) == len(best) && p < best) {
			best = p
		}
	}
	return best
}

// CardConfig is one project card. Command is an argv list, run in the project root
// without a shell.
type CardConfig struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Command []string `json:"command"`
	// Refresh is "watch:<relpath>" (re-run when that file or directory changes) or
	// "interval:<seconds>".
	Refresh string `json:"refresh"`
	// Pin also shows the card in the side panel (version 3): the project's own
	// "where things stand", beside the lanes rather than in the drawer.
	Pin bool `json:"pin"`
}

// MetricsConfig is the project's metrics command (version 4): run like a card, only
// while the config is trusted, its stdout the JSON contract in docs/panel.md
// ("Metrics"). Without a command the panel shows only the metrics it computes itself.
type MetricsConfig struct {
	Command []string `json:"command"`
	// Refresh is a card's refresh rule; DefaultMetricsRefresh when empty.
	Refresh string `json:"refresh"`
	// Card shows the Flow card in the side panel while there are metrics to show
	// (default true).
	Card *bool `json:"card"`
}

// DefaultMetricsRefresh is how often a metrics command runs when its config says
// nothing: its figures move by the day, so every 15 minutes is plenty.
const DefaultMetricsRefresh = "interval:900"

// MetricsRefresh returns the metrics command's refresh rule, defaulted.
func (c *Config) MetricsRefresh() string {
	if c.Metrics == nil || c.Metrics.Refresh == "" {
		return DefaultMetricsRefresh
	}
	return c.Metrics.Refresh
}

// MetricsCommand returns the project's metrics argv, or nil when it names none.
func (c *Config) MetricsCommand() []string {
	if c.Metrics == nil {
		return nil
	}
	return c.Metrics.Command
}

// FlowCard reports whether the side panel may show the Flow card.
func (c *Config) FlowCard() bool {
	return c.Metrics == nil || c.Metrics.Card == nil || *c.Metrics.Card
}

// SuggestConfig is a template's list of what to start next (version 3): a command,
// run like a card, whose output the Start dialog offers as lane names.
type SuggestConfig struct {
	Command []string `json:"command"`
	Refresh string   `json:"refresh"`
}

// RefreshRule is a parsed CardConfig.Refresh.
type RefreshRule struct {
	WatchRel string        // set for watch rules
	Interval time.Duration // set for interval rules
}

var cardIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// minCardInterval stops a typo ("interval:1") from turning a card into a busy loop.
const minCardInterval = 5 * time.Second

// LoadConfig reads and validates a panel config file.
func LoadConfig(path string) (*Config, error) {
	c, _, err := LoadConfigRaw(path)
	return c, err
}

// LoadConfigRaw reads and validates a panel config and also returns the exact bytes
// it parsed, so the trust hash covers what runs rather than a second read.
func LoadConfigRaw(path string) (*Config, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no panel config at %s (create one with `clauductor panel init`, or pass --config; see docs/panel.md)", path)
		}
		return nil, nil, err
	}
	c, err := parseConfig(raw)
	return c, raw, err
}

// parseConfig parses and validates panel config bytes. Unknown keys are refused so a
// misspelt key fails loudly instead of being silently ignored, and a key newer than
// the declared version is refused so the version means what it says.
func parseConfig(raw []byte) (*Config, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("panel config: %w", err)
	}
	notices, err := checkVersion(raw, &c)
	if err != nil {
		return nil, err
	}
	c.Notices = notices
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// ParseConfig parses and validates panel config bytes (what LoadConfig reads).
func ParseConfig(raw []byte) (*Config, error) { return parseConfig(raw) }

// Validate checks the config's invariants.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("panel config: name is required")
	}
	// The name reaches OS notification titles and the page: one line of plain text,
	// never anything a command line could read as an option.
	if err := TypableText(c.Name, 80); err != nil {
		return fmt.Errorf("panel config: name %w", err)
	}
	if !nameRe.MatchString(c.Name) {
		return fmt.Errorf("panel config: name must not be blank or start with \"-\" (it must match %s)", nameRe)
	}
	for k, v := range c.Lanes {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			return fmt.Errorf("panel config: lanes entries need a non-empty branch rule and lane type")
		}
	}
	if c.TmuxSocket != "" && !SocketNameRe.MatchString(c.TmuxSocket) {
		return fmt.Errorf("panel config: tmux_socket %q must match %s", c.TmuxSocket, SocketNameRe)
	}
	if c.Base != "" && !baseRe.MatchString(c.Base) {
		return fmt.Errorf("panel config: base %q must match %s", c.Base, baseRe)
	}
	if d := c.WorktreeDir; d != "" && !filepath.IsAbs(d) {
		clean := filepath.Clean(d)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("panel config: worktree_dir %q must be absolute, or relative and inside the project", d)
		}
	}
	for name, lt := range c.LaneTypes {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("panel config: lane_types needs non-empty type names")
		}
		for key, v := range map[string]string{"model": lt.Model, "effort": lt.Effort} {
			if v != "" && !LaunchOptRe.MatchString(v) {
				return fmt.Errorf("panel config: lane_types.%s.%s %q must match %s", name, key, v, LaunchOptRe)
			}
		}
	}
	seen := map[string]bool{}
	for i, card := range c.Cards {
		if !cardIDRe.MatchString(card.ID) {
			return fmt.Errorf("panel config: cards[%d].id %q must match %s", i, card.ID, cardIDRe)
		}
		if seen[card.ID] {
			return fmt.Errorf("panel config: duplicate card id %q", card.ID)
		}
		seen[card.ID] = true
		if len(card.Command) == 0 || strings.TrimSpace(card.Command[0]) == "" {
			return fmt.Errorf("panel config: card %q needs a command (argv list)", card.ID)
		}
		if _, err := ParseRefresh(card.Refresh); err != nil {
			return fmt.Errorf("panel config: card %q: %w", card.ID, err)
		}
	}
	return c.validateV2()
}

// ParseRefresh parses a card refresh rule.
func ParseRefresh(s string) (RefreshRule, error) {
	if !refreshRe.MatchString(s) {
		return RefreshRule{}, fmt.Errorf("refresh %q: want \"watch:<relpath>\" or \"interval:<seconds>\" (a positive whole number); it must match %s", s, refreshRe)
	}
	switch {
	case strings.HasPrefix(s, "watch:"):
		rel := strings.TrimPrefix(s, "watch:")
		clean := filepath.Clean(rel)
		if rel == "" || filepath.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") {
			return RefreshRule{}, fmt.Errorf("refresh %q: watch path must be relative to the project and stay inside it", s)
		}
		return RefreshRule{WatchRel: clean}, nil
	case strings.HasPrefix(s, "interval:"):
		n, err := strconv.Atoi(strings.TrimPrefix(s, "interval:"))
		if err != nil || n <= 0 {
			return RefreshRule{}, fmt.Errorf("refresh %q: interval must be a positive number of seconds", s)
		}
		d := time.Duration(n) * time.Second
		if d < minCardInterval {
			d = minCardInterval
		}
		return RefreshRule{Interval: d}, nil
	default:
		return RefreshRule{}, fmt.Errorf("refresh %q: want \"watch:<relpath>\" or \"interval:<seconds>\"", s)
	}
}

// LaneFor maps a branch to (lane type, display name). The display name is the branch
// with its matched prefix removed, so "change/add-x" shows as "add-x".
func (c *Config) LaneFor(branch string) (laneType, display string) {
	if branch == "" {
		return "detached", "(detached)"
	}
	keys := make([]string, 0, len(c.Lanes))
	for k := range c.Lanes {
		keys = append(keys, k)
	}
	// Longest rule first, so "change/propose-" can refine "change/".
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		prefix, isPrefix := k, strings.HasSuffix(k, "/")
		if strings.HasSuffix(k, "*") {
			prefix, isPrefix = strings.TrimSuffix(k, "*"), true
		}
		if isPrefix {
			if strings.HasPrefix(branch, prefix) {
				name := strings.TrimPrefix(branch, prefix)
				if name == "" {
					name = branch
				}
				return c.Lanes[k], name
			}
		} else if branch == k {
			return c.Lanes[k], branch
		}
	}
	return "other", branch
}

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
	// Suggest lists what this template could start next (signals.ParseSuggestions).
	Suggest *SuggestConfig `json:"suggest"`
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

// MaxFirstPrompt caps a rendered first prompt; it is typed into claude's input box.
const MaxFirstPrompt = 4000

// maxPlaceholderValue caps a value typed into the dialog.
const maxPlaceholderValue = 200

func (c *Config) validateV2() error {
	if c.Version != 0 && (c.Version < MinVersion || c.Version > LatestVersion) {
		return fmt.Errorf("panel config: version %d is not supported (this panel reads %d to %d)", c.Version, MinVersion, LatestVersion)
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
			if b := fillPlaceholders(t.BranchPattern, map[string]string{"name": "x", "issue": "1"}); !BranchRe.MatchString(b) {
				return fmt.Errorf("panel config: template %q: branch_pattern %q does not make a valid branch name", t.ID, t.BranchPattern)
			}
		}
		if strings.TrimSpace(t.FirstPrompt) == "" {
			return fmt.Errorf("panel config: template %q needs a first_prompt", t.ID)
		}
		if err := checkPlaceholders(t.FirstPrompt); err != nil {
			return fmt.Errorf("panel config: template %q: first_prompt: %w", t.ID, err)
		}
		if err := TypableText(t.FirstPrompt, MaxFirstPrompt); err != nil {
			return fmt.Errorf("panel config: template %q: first_prompt %w", t.ID, err)
		}
		for key, v := range map[string]string{"model": t.Model, "effort": t.Effort} {
			if v != "" && !LaunchOptRe.MatchString(v) {
				return fmt.Errorf("panel config: template %q: %s %q must match %s", t.ID, key, v, LaunchOptRe)
			}
		}
		if sg := t.Suggest; sg != nil {
			if len(sg.Command) == 0 || strings.TrimSpace(sg.Command[0]) == "" {
				return fmt.Errorf("panel config: template %q: suggest needs a command (argv list)", t.ID)
			}
			if _, err := ParseRefresh(sg.Refresh); err != nil {
				return fmt.Errorf("panel config: template %q: suggest: %w", t.ID, err)
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
	for _, n := range c.HostNames {
		if !ValidHostName(n) {
			return fmt.Errorf("panel config: host_names entry %q must be one lower-case label followed by .localhost (no wildcards)", n)
		}
	}
	if g := c.QuotaGuard; g != nil && g.FiveHourPct != nil && (*g.FiveHourPct < 0 || *g.FiveHourPct > 100) {
		return fmt.Errorf("panel config: quota_guard.five_hour_pct must be 0 (off) to 100")
	}
	if mt := c.Metrics; mt != nil {
		if mt.Command != nil && (len(mt.Command) == 0 || strings.TrimSpace(mt.Command[0]) == "") {
			return fmt.Errorf("panel config: metrics.command must be a non-empty argv list")
		}
		if mt.Refresh != "" {
			if len(mt.Command) == 0 {
				return fmt.Errorf("panel config: metrics.refresh needs a metrics.command to refresh")
			}
			if _, err := ParseRefresh(mt.Refresh); err != nil {
				return fmt.Errorf("panel config: metrics: %w", err)
			}
		}
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

// TypableText rejects what must never be typed into a lane: a newline (it would
// submit early), any other control character (an escape sequence drives the TUI),
// and invisible format characters such as bidi overrides.
func TypableText(s string, max int) error {
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
		return RenderedTemplate{}, fmt.Errorf("lane name %q must match %s", name, LaneIDRe)
	}
	issue = strings.TrimSpace(issue)
	if t.usesPlaceholder("issue") {
		if issue == "" {
			return RenderedTemplate{}, fmt.Errorf("template %q needs an issue", id)
		}
		if err := TypableText(issue, maxPlaceholderValue); err != nil {
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
	if !BranchRe.MatchString(r.Branch) || strings.Contains(r.Branch, "..") {
		return RenderedTemplate{}, fmt.Errorf("branch %q is not a valid branch name", r.Branch)
	}
	r.FirstPrompt = fillPlaceholders(t.FirstPrompt, vals)
	if err := TypableText(r.FirstPrompt, MaxFirstPrompt); err != nil {
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
	// Suggests: the template has a suggest command, so the dialog shows its list.
	Suggests bool `json:"suggests,omitempty"`
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
			FirstPrompt: t.FirstPrompt, NeedsIssue: t.usesPlaceholder("issue"), Model: t.Model, Effort: t.Effort,
			Suggests: t.Suggest != nil})
	}
	return out
}

// TrustView says whether the config's argv may run (install.CheckTrust).
type TrustView struct {
	Trusted bool   `json:"trusted"`
	Hash    string `json:"hash"`
	Prev    string `json:"prev,omitempty"`
	Path    string `json:"path"`
	Note    string `json:"note,omitempty"`
}

var LaneIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// ValidLaneID reports whether id can name a lane (and so a tmux session and a
// worktree directory). It is also what keeps an id safe inside a tmux target and a
// Terminal.app command.
func ValidLaneID(id string) bool { return LaneIDRe.MatchString(id) }

// BranchRe is checked before `git check-ref-format`, so nothing that looks like an
// option (a leading "-") ever reaches git.
var BranchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// DefaultHostName is the name the panel is opened at.
const DefaultHostName = "clauductor.localhost"

// localhostNameRe is the only shape an extra host name may have: one DNS label under
// .localhost, lower case.
var localhostNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.localhost$`)

// ValidHostName reports whether a configured extra host name is allowed.
func ValidHostName(n string) bool { return localhostNameRe.MatchString(n) }

// RunList describes, one line each, everything the config makes the panel run or
// type: each card's argv (run on its refresh), each queue's argv (run on RUN), each
// template's first prompt (typed into a new lane) and each template's suggest argv
// (run on its refresh) and the metrics command (run on its refresh). Trusting a config trusts exactly these, so trust and install
// print them.
func (c *Config) RunList() []string {
	var out []string
	for _, card := range c.Cards {
		out = append(out, fmt.Sprintf("card %s runs %q (%s)", card.ID, card.Command, card.Refresh))
	}
	for _, q := range c.Queues {
		if len(q.Command) > 0 {
			out = append(out, fmt.Sprintf("queue %s runs %q on RUN", q.ID, q.Command))
		}
	}
	for _, t := range c.Templates {
		out = append(out, fmt.Sprintf("template %s types %q", t.ID, t.FirstPrompt))
		if t.Suggest != nil {
			out = append(out, fmt.Sprintf("template %s suggests from %q (%s)", t.ID, t.Suggest.Command, t.Suggest.Refresh))
		}
	}
	if cmd := c.MetricsCommand(); len(cmd) > 0 {
		out = append(out, fmt.Sprintf("metrics runs %q (%s)", cmd, c.MetricsRefresh()))
	}
	return out
}

func ShortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	if h == "" {
		return "none"
	}
	return h
}
