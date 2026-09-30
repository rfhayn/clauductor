package state

import (
	"strconv"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-20: what the lane lifecycle hands the page. A lane whose pull request merged
// but that Close cannot take on its own (claude busy, the worktree changed, commits
// after the merge) asks in Needs you; what the panel did by itself (closed a lane,
// typed a resume line) goes in the lane's activity.

// MergeAsk is one lane whose pull request merged and that waits for you to close it.
type MergeAsk struct {
	PR  int      `json:"pr"`
	Why []string `json:"why"` // why the panel did not close it itself
	At  int64    `json:"at"`  // when the merge was noticed, unix ms
}

// ApplyMergeAsks records the lanes waiting for "close lane?" (by lane id).
func (m *Model) ApplyMergeAsks(a map[string]MergeAsk) { m.mergeAsks = a }

// NoteLane adds a line to a lane's activity feed: what the panel did on its own.
func (m *Model) NoteLane(worktree, lane, event, detail string, now time.Time) {
	m.pushFeed(FeedEvent{At: ms(now), Lane: worktree, Name: lane, Event: event, Detail: detail})
}

// mergeNeeds adds the "PR merged: close lane?" rows.
func (m *Model) mergeNeeds(v *View) {
	for _, tv := range v.Terminals {
		a, ok := m.mergeAsks[tv.ID]
		if !ok {
			continue
		}
		v.NeedsYou = append(v.NeedsYou, NeedView{Lane: tv.Worktree, Name: tv.ID, Kind: "merged", Label: "PR merged: close lane?",
			Severity: signals.SevWarn, At: a.At, Terminal: tv.ID,
			Text: "its pull request #" + strconv.Itoa(a.PR) + " merged; the panel did not close it because " + strings.Join(a.Why, "; ") +
				". Close lane (in the lane's menu) asks first and removes only what loses nothing."})
	}
}
