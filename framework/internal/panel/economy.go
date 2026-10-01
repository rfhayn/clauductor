package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// Economy mode (PANEL-19). The quota is the machine's, so economy mode is too: while
// the account's 5-hour quota is at or above the default project's
// quota_economy.five_hour_pct, the panel writes ~/.clauductor/panel/economy.json with
// "economy": true, and false once the quota falls economyHysteresis points below.
// The operating model's build-change reads the file and drops the roles its
// model-roles.json names under "economy" one tier. The file is a documented contract
// (docs/panel.md, Economy mode): exactly {"economy", "since", "reason"}.

// economyHysteresis keeps a quota hovering at the threshold from flapping the mode.
const economyHysteresis = 3

// EconomyFile is economy.json.
type EconomyFile struct {
	Economy bool   `json:"economy"`
	Since   int64  `json:"since"` // unix seconds of the last switch
	Reason  string `json:"reason"`
}

// EconomyPath is where economy.json lives.
func EconomyPath(home string) string { return filepath.Join(config.PanelDir(home), "economy.json") }

// economyNext decides the mode from the last one and the 5-hour quota now (nil: no
// reading, which keeps the mode as it is). threshold 0 is off.
// The file is written only when the mode switches (or has never been written), so its
// reason is the reading that switched it.
func economyNext(prev EconomyFile, pct *float64, threshold float64, now time.Time) (EconomyFile, bool) {
	var next EconomyFile
	switch {
	case threshold <= 0:
		if !prev.Economy {
			return prev, false
		}
		next = EconomyFile{Economy: false, Reason: "quota_economy is not set"}
	case pct == nil:
		return prev, false
	case *pct >= threshold:
		next = EconomyFile{Economy: true, Reason: fmt.Sprintf("5-hour quota %.0f%% ≥ %.0f%%", *pct, threshold)}
	case prev.Economy && *pct > threshold-economyHysteresis:
		return prev, false // still on: below the threshold, not yet by the hysteresis
	default:
		next = EconomyFile{Economy: false, Reason: fmt.Sprintf("5-hour quota %.0f%% < %.0f%% (on at %.0f%%)", *pct, threshold-economyHysteresis, threshold)}
	}
	if next.Economy == prev.Economy && prev.Since != 0 {
		return prev, false
	}
	next.Since = now.Unix()
	return next, true
}

// economy is the machine's economy source: it reads the account's 5-hour quota, and
// writes economy.json when the mode switches (or on the first reading, so a file left
// by an earlier run is brought up to date).
type economy struct {
	path      string
	threshold float64
	cur       EconomyFile
}

func newEconomy(home string, cfg *config.Config) *economy {
	e := &economy{path: EconomyPath(home), threshold: cfg.EconomyPct()}
	if b, err := os.ReadFile(e.path); err == nil {
		_ = json.Unmarshal(b, &e.cur)
	}
	return e
}

func (m *Machine) pollEconomy(_ context.Context, now time.Time) (update, time.Duration) {
	e := m.economy
	var pct *float64
	m.def.hub.Read(func(md *state.Model, now time.Time) {
		if w := md.QuotaReading().Window("five_hour"); w != nil && w.Pct != nil && !w.Expired {
			v := *w.Pct
			pct = &v
		}
	})
	next, changed := economyNext(e.cur, pct, e.threshold, now)
	if changed {
		b, _ := json.Marshal(next)
		if err := writePrivate(e.path, b); err != nil {
			fmt.Fprintf(m.o.Out, "economy mode: cannot write %s: %v\n", e.path, err)
			return nil, 0
		}
		if next.Economy != e.cur.Economy {
			fmt.Fprintf(m.o.Out, "economy mode %s: %s\n", map[bool]string{true: "on", false: "off"}[next.Economy], next.Reason)
		}
		e.cur = next
	}
	if e.threshold <= 0 {
		return nil, 0
	}
	v := &state.EconomyView{Active: e.cur.Economy, Since: e.cur.Since * 1000, Reason: e.cur.Reason, Threshold: e.threshold}
	return func(md *state.Model, _ time.Time) { md.ApplyEconomy(v) }, 0
}

// economyRoles reads the project's .claude/model-roles.json "economy" mapping: role →
// the tier it drops to, {"model", "effort"} or "model/effort". Keys starting with "_"
// are comments; the same object may sit under "roles".
func economyRoles(root string) ([]state.EconomyRole, string) {
	b, err := os.ReadFile(filepath.Join(root, ".claude", "model-roles.json"))
	if err != nil {
		return nil, "no .claude/model-roles.json in this project, so no role is named"
	}
	var doc struct {
		Economy map[string]json.RawMessage `json:"economy"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, ".claude/model-roles.json does not parse: " + err.Error()
	}
	if len(doc.Economy) == 0 {
		return nil, ".claude/model-roles.json has no \"economy\" mapping, so no role is named"
	}
	m := doc.Economy
	if raw, ok := m["roles"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(raw, &inner) == nil {
			m = inner
		}
	}
	var out []state.EconomyRole
	for role, raw := range m {
		if strings.HasPrefix(role, "_") {
			continue
		}
		var s string
		var o struct{ Model, Effort string }
		switch {
		case json.Unmarshal(raw, &s) == nil:
			out = append(out, state.EconomyRole{Role: role, To: strings.ReplaceAll(s, "/", ", ")})
		case json.Unmarshal(raw, &o) == nil && (o.Model != "" || o.Effort != ""):
			out = append(out, state.EconomyRole{Role: role, To: strings.Trim(o.Model+", "+o.Effort, ", ")})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out, ""
}
