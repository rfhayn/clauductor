package state

import (
	"strconv"
	"strings"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-48 (D7, D13): a session launched with a context window (launch.context_window)
// is measured against that window, not the model's: the auto_compact_window its status
// line posts, else the window on the lane record holding its session id.

// compactBuffer is how far below its window Claude Code compacts a conversation, in
// tokens. Probe P1 saw compaction fire at the window less 33k on Sonnet, and /context
// read the same 33k buffer under --model opus at 100k, 200k, 300k and 1m windows, on
// Claude Code 2.1.302 (changes/panel-48-lane-context-policy/evidence/README.md, "Probe
// results" and "P1 on Opus"). The status line has no field for it: a release that
// changes the buffer moves compaction and not the tick.
const compactBuffer = 33000

// alertMargin is how many points below its tick a windowed lane's context alert sits
// at most (D13), so the alert fires before the lane compacts.
const alertMargin = 5

// ctxTick is where a lane's bar marks auto-compaction: (window − 33000) / window of the
// bar for a lane launched with a window, computed in one division so 83.5 and 89 come
// out exact; AutocompactPct for a lane without one.
func ctxTick(window int) float64 {
	if window <= 0 {
		return AutocompactPct
	}
	return float64(window-compactBuffer) * 100 / float64(window)
}

// ctxAlertPct is a lane's context alert threshold: alerts.context_pct for a lane
// without a window, else the lower of it and the tick less alertMargin (D13). An
// alerts.context_pct of 0 is off, and stays 0 (off) for every lane, never "the lower
// of 0 and the tick".
func ctxAlertPct(contextPct float64, window int) float64 {
	if contextPct <= 0 || window <= 0 {
		return contextPct
	}
	return min(contextPct, ctxTick(window)-alertMargin)
}

// ctxReading is the context figure one status post gives a session launched with
// window: current_usage's input, cache-creation and cache-read tokens over the window;
// for a session without one, Claude Code's used_percentage. nil when the post carries
// no figure for it (a windowed session's post without current_usage keeps the last).
func ctxReading(p signals.StatusPayload, window int) *float64 {
	cw := p.ContextWindow
	if window <= 0 {
		if cw.UsedPercentage == nil {
			return nil
		}
		v := *cw.UsedPercentage
		return &v
	}
	u := cw.CurrentUsage
	if u == nil {
		return nil
	}
	v := windowPct(u.InputTokens+u.CacheCreationInputTokens+u.CacheReadInputTokens, window)
	return &v
}

// windowPct is a windowed session's context figure: its input, cache-creation and
// cache-read tokens over the window it was launched with. A status post and a
// transcript reading (PANEL-49 D4) both go through it, so the two never disagree.
func windowPct(tokens int64, window int) float64 {
	return float64(tokens) * 100 / float64(window)
}

// recordWindow is the window a status post is measured against (D7, amended with P6):
// the window the session posted (its status line's auto_compact_window) when it is in
// Claude Code's range; else the window of the lane record carrying its session id;
// else 0, which keeps Claude Code's used_percentage. The session reports its own window
// because /clear gives the same process a new session id that no record holds, while
// its environment, and so its window, stays the lane's (P6). There is no fallback by
// folder: a session the owner starts by hand in a windowed lane's worktree runs without
// that window, and measured against it would read high and raise an alert that never
// clears.
func (m *Model) recordWindow(sessionID string, posted signals.PostedInt) int {
	if posted >= config.MinContextWindow && posted <= config.MaxContextWindow {
		return int(posted)
	}
	if sessionID != "" {
		for _, rec := range m.laneRecords {
			if rec.SessionID == sessionID {
				return recWindow(rec)
			}
		}
	}
	return 0
}

// recWindow is a record's window, or 0 for a corrupt record: the registry marks one
// corrupt for a window outside Claude Code's range, which no figure is divided by.
func recWindow(rec types.LaneRecord) int {
	if rec.Corrupt != "" {
		return 0
	}
	return rec.ContextWindow
}

// ctxMarks are the tick and the context alert threshold of a figure measured against
// window, under the configured alerts.context_pct.
func (m *Model) ctxMarks(window int) (tick float64, alert *float64) {
	a := ctxAlertPct(m.cfg.AlertThresholds().ContextPct, window)
	return ctxTick(window), &a
}

// pctText is a threshold as the alert's text shows it: to one decimal where it has
// one ("78.5"), whole otherwise ("85").
func pctText(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}
