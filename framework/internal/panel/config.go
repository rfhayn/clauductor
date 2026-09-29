// Package panel implements `clauductor panel`: a standalone, read-only, loopback-only
// web dashboard over the Claude Code sessions working in one project.
//
// It is deliberately independent of the rest of Clauductor. It needs no `clauductor
// install`, no template, no skills, no SQLite database and no file locks. Its only
// inputs are Claude Code's own signals (HTTP hooks, the status line's stdin,
// `claude agents --json`), git, gh, and the commands a project names in its
// .clauductor/panel.json.
package panel

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
)

// DefaultConfigRel is where a project keeps its panel config, relative to the project root.
const DefaultConfigRel = ".clauductor/panel.json"

// Config is the per-project panel configuration (.clauductor/panel.json).
type Config struct {
	// Name is shown in the top bar.
	Name string `json:"name"`
	// Lanes maps a branch rule to a lane type. A key ending in "/" is a prefix
	// ("change/" matches "change/add-x"); a key ending in "*" is a prefix without the
	// star ("change/propose-*"); any other key matches one branch exactly ("main").
	// The longest matching rule wins.
	Lanes map[string]string `json:"lanes"`
	// Cards are project commands whose stdout renders as a card.
	Cards []CardConfig `json:"cards"`

	// v1: lanes the panel starts itself. All optional; see the Default* constants.

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

	// v2: orchestration. All optional; see config_v2.go.

	// Version is the config schema version: 0 (absent), 1 or 2.
	Version int `json:"version,omitempty"`
	// Templates are lane recipes offered in the Start dialog.
	Templates []TemplateConfig `json:"templates"`
	// Queues are shared resources held as an on-disk lease (the gate on port 3100).
	Queues []QueueConfig `json:"queues"`
	// Alerts sets the alert thresholds and the OS notifications.
	Alerts *AlertConfig `json:"alerts"`
	// QuotaGuard refuses to start a lane above a 5-hour quota threshold.
	QuotaGuard *QuotaGuardConfig `json:"quota_guard"`
	// HostNames are extra names the panel answers to, each "<label>.localhost"
	// (clauductor.localhost always works). No wildcards.
	HostNames []string `json:"host_names"`
}

// LaneTypeConfig holds the launch options of one lane type.
type LaneTypeConfig struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// Defaults for the v1 lane keys.
const (
	DefaultTmuxSocket  = "clauductor"
	DefaultWorktreeDir = ".claude/worktrees"
	DefaultBase        = "origin/main"
)

var (
	socketNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// A model or effort value becomes one argv element of `claude`; keep it to the
	// characters real values use ("opus", "claude-opus-4-5[1m]", "high").
	launchOptRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`)
	baseRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@{}^~-]{0,199}$`)
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
			return nil, nil, fmt.Errorf("no panel config at %s (create it, or pass --config; see docs/panel.md)", path)
		}
		return nil, nil, err
	}
	c, err := ParseConfig(raw)
	return c, raw, err
}

// ParseConfig parses and validates panel config bytes. Unknown keys are refused so a
// misspelt key fails loudly instead of being silently ignored.
func ParseConfig(raw []byte) (*Config, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("panel config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks the config's invariants.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("panel config: name is required")
	}
	// The name reaches OS notification titles and the page: one line of plain text,
	// never anything a command line could read as an option.
	if err := typableText(c.Name, 80); err != nil {
		return fmt.Errorf("panel config: name %w", err)
	}
	if strings.HasPrefix(strings.TrimSpace(c.Name), "-") {
		return fmt.Errorf("panel config: name must not start with \"-\"")
	}
	for k, v := range c.Lanes {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			return fmt.Errorf("panel config: lanes entries need a non-empty branch rule and lane type")
		}
	}
	if c.TmuxSocket != "" && !socketNameRe.MatchString(c.TmuxSocket) {
		return fmt.Errorf("panel config: tmux_socket %q must match %s", c.TmuxSocket, socketNameRe)
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
			if v != "" && !launchOptRe.MatchString(v) {
				return fmt.Errorf("panel config: lane_types.%s.%s %q must match %s", name, key, v, launchOptRe)
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
