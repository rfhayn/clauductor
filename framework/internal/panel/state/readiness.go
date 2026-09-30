package state

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Merge readiness (PANEL-20): one box per lane that says whether its branch is ready
// to merge, and if not, why. It reads only what the panel already has or reads while
// a page is in view: the open pull request and its checks and review decision (`gh pr
// list`), its unresolved review threads (one `gh api graphql` per pull request, at most
// every two minutes), the change's tasks.md in the lane's worktree, and the gate's
// receipt in the lane's git dir. It is read-only: the panel never merges.

// LaneExtra is what the runtime reads for one lane's worktree.
type LaneExtra struct {
	// Receipt is the worktree's ci-receipt; HasReceipt false when there is none.
	Receipt    signals.Receipt
	HasReceipt bool
	// Tasks are the change's tasks.md in this worktree (Tasks false: none found).
	Change    string
	Tasks     bool
	TasksOpen int
	TasksDone int
	// Threads for the lane's pull request: unresolved and total; ThreadsErr when the
	// read failed; ThreadsPR the pull request they are of (0: not read).
	ThreadsPR  int
	Unresolved int
	Threads    int
	ThreadsErr string
}

// ApplyLaneExtras records the runtime's per-worktree reads, and whether the project
// keeps a gate receipt (so a missing one is a reason, not an irrelevance).
func (m *Model) ApplyLaneExtras(x map[string]LaneExtra, gateReceipts bool) {
	m.laneExtras, m.gateReceipts = x, gateReceipts
}

// LaneExtraOf is one worktree's extra reads (the runtime keeps what it has).
func (m *Model) LaneExtraOf(path string) (LaneExtra, bool) {
	x, ok := m.laneExtras[path]
	return x, ok
}

// Readiness is the box.
type Readiness struct {
	Ready   bool        `json:"ready"`
	Reasons []string    `json:"reasons"`
	PR      *signals.PR `json:"pr,omitempty"`
	// Rows are the box's lines: what each check found.
	Rows []ReadyRow `json:"rows"`
}

// ReadyRow is one line of the box: a check and what it found; OK false marks it as a
// reason; Unknown means it could not be told yet.
type ReadyRow struct {
	Name    string `json:"name"`
	Text    string `json:"text"`
	OK      bool   `json:"ok"`
	Unknown bool   `json:"unknown,omitempty"`
}

// readiness is a lane's box, or nil for a lane with no branch of its own.
func (m *Model) readiness(l LaneView) *Readiness {
	if l.Branch == "" || l.Path == m.root || l.Branch == m.cfg.BaseRef() || "origin/"+l.Branch == m.cfg.BaseRef() {
		return nil
	}
	r := &Readiness{Reasons: []string{}, Rows: []ReadyRow{}}
	add := func(name, text string, ok, unknown bool, reason string) {
		r.Rows = append(r.Rows, ReadyRow{Name: name, Text: text, OK: ok, Unknown: unknown})
		if !ok {
			r.Reasons = append(r.Reasons, reason)
		}
	}
	var pr *signals.PR
	for i := range m.prs {
		if m.prs[i].HeadRef == l.Branch {
			pr = &m.prs[i]
		}
	}
	switch {
	case !m.prsSrc.OK && m.prs == nil:
		add("Pull request", "not read yet", false, true, "the pull requests are not read yet")
	case pr == nil:
		add("Pull request", "none open for "+l.Branch, false, false, "no open pull request")
	case pr.IsDraft:
		add("Pull request", fmt.Sprintf("#%d, a draft", pr.Number), false, false, fmt.Sprintf("#%d is a draft", pr.Number))
	default:
		add("Pull request", fmt.Sprintf("#%d, open", pr.Number), true, false, "")
	}
	r.PR = pr
	if pr != nil {
		c := fmt.Sprintf("%d passed, %d failed, %d pending", pr.ChecksPass, pr.ChecksFail, pr.ChecksWait)
		switch {
		case pr.ChecksFail > 0:
			add("Checks", c, false, false, fmt.Sprintf("%d check(s) failed", pr.ChecksFail))
		case pr.ChecksWait > 0:
			add("Checks", c, false, false, fmt.Sprintf("%d check(s) pending", pr.ChecksWait))
		case pr.ChecksPass == 0:
			add("Checks", "none reported", true, false, "")
		default:
			add("Checks", c, true, false, "")
		}
		switch pr.Review {
		case "APPROVED":
			add("Review", "approved", true, false, "")
		case "CHANGES_REQUESTED":
			add("Review", "changes requested", false, false, "changes requested")
		case "REVIEW_REQUIRED":
			add("Review", "review required", false, false, "a review is required")
		default:
			add("Review", "no review decision", true, false, "")
		}
	}
	x, have := m.laneExtras[l.Path]
	if pr != nil {
		switch {
		case !have || x.ThreadsPR != pr.Number:
			add("Review threads", "not read yet", false, true, "review threads not read yet")
		case x.ThreadsErr != "":
			add("Review threads", "cannot read: "+x.ThreadsErr, false, true, "review threads cannot be read")
		case x.Unresolved > 0:
			add("Review threads", fmt.Sprintf("%d of %d unresolved", x.Unresolved, x.Threads), false, false, fmt.Sprintf("%d unresolved review thread(s)", x.Unresolved))
		default:
			add("Review threads", fmt.Sprintf("all %d resolved", x.Threads), true, false, "")
		}
	}
	if have && x.Tasks {
		if x.TasksOpen > 0 {
			add("Tasks", fmt.Sprintf("%d of %d ticked in %s/tasks.md", x.TasksDone, x.TasksDone+x.TasksOpen, x.Change), false, false, fmt.Sprintf("%d unticked task(s)", x.TasksOpen))
		} else {
			add("Tasks", fmt.Sprintf("all %d ticked in %s/tasks.md", x.TasksDone, x.Change), true, false, "")
		}
	}
	head := ""
	if l.Git != nil {
		head = l.Git.Head
	}
	switch {
	case head == "":
		if m.gateReceipts || (have && x.HasReceipt) {
			add("Gate receipt", "HEAD not read yet", false, true, "HEAD not read yet")
		}
	case have && x.HasReceipt && x.Receipt.SHA == head && x.Receipt.Clean:
		add("Gate receipt", "clean full run of "+short(head), true, false, "")
	case have && x.HasReceipt && x.Receipt.SHA == head:
		add("Gate receipt", "for "+short(head)+", but the tree was dirty", false, false, "the gate receipt is for a dirty tree")
	case have && x.HasReceipt:
		add("Gate receipt", "for "+short(x.Receipt.SHA)+", not HEAD "+short(head), false, false, "no gate receipt for HEAD")
	case m.gateReceipts:
		add("Gate receipt", "none for "+short(head), false, false, "no gate receipt for HEAD")
	}
	r.Ready = len(r.Reasons) == 0
	return r
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}
