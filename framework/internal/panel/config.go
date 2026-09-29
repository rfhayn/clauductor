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
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no panel config at %s (create it, or pass --config; see docs/panel.md)", path)
		}
		return nil, err
	}
	return ParseConfig(raw)
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
	for k, v := range c.Lanes {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			return fmt.Errorf("panel config: lanes entries need a non-empty branch rule and lane type")
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
	return nil
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
