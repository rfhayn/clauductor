package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// Close a lane when its pull request merges (PANEL-20; lanes_auto_close "on_merge").
// It is Close lane, planned exactly as the page's confirmation plans it, done by the
// panel only when that plan removes the worktree and the branch (clean, and merged at
// the branch's tip) and its work has ended (TermLaneView.CloseHeld, PANEL-57: claude idle
// on a current reading with no background work, or gone, and no lease held or waited
// for), checked again under the lane lock. Otherwise the lane asks in Needs you,
// "PR merged: close lane?", with why. Nothing is forced, and each is notified once.
//
// A lane is checked for a merge when the panel first sees it, when its branch's open
// pull request leaves the open list, and while it asks (every askRecheck, in case what
// held it back cleared): one `gh pr list --head <branch> --state merged` each time.

const askRecheck = 5 * time.Minute

type autoCloser struct {
	seen     map[string]bool // lane id → checked once this run
	open     map[string]bool // branch → had an open pull request at the last poll
	asks     map[string]state.MergeAsk
	askedAt  map[string]time.Time // lane id → last check while asking
	notified map[string]bool      // lane id + merged PR → notified
}

func (r *Runtime) pollAutoClose(ctx context.Context, now time.Time) (update, time.Duration) {
	if r.lanes == nil || !r.trusted() {
		return nil, 0 // the mode is the config's: an untrusted config turns nothing on
	}
	a := &r.autoClose
	if a.seen == nil {
		a.seen, a.open, a.asks, a.askedAt, a.notified = map[string]bool{}, map[string]bool{}, map[string]state.MergeAsk{}, map[string]time.Time{}, map[string]bool{}
	}
	v := r.hub.View()
	openNow := map[string]bool{}
	for _, pr := range v.PRs {
		openNow[pr.HeadRef] = true
	}
	var notes []func(*state.Model, time.Time)
	live := map[string]bool{}
	for _, tv := range v.Terminals {
		if !tv.Registered || tv.Branch == "" || tv.Worktree == "" || tv.Worktree == r.root || r.cfg.AutoCloseOf(tv.Type) != config.AutoCloseOnMerge {
			continue
		}
		live[tv.ID] = true
		_, asking := a.asks[tv.ID]
		due := !a.seen[tv.ID] || (a.open[tv.Branch] && !openNow[tv.Branch]) || (asking && now.Sub(a.askedAt[tv.ID]) >= askRecheck)
		if !due || ctx.Err() != nil {
			continue
		}
		a.seen[tv.ID], a.askedAt[tv.ID] = true, now
		pr, ok := r.mergedPR(ctx, tv.Branch)
		if !ok {
			delete(a.asks, tv.ID)
			continue
		}
		var why []string
		if tv.CloseHeld != "" { // PANEL-57 D2: the verdict the command reads too
			why = append(why, tv.CloseHeld)
		}
		plan, lerr := r.lanes.ClosePlan(ctx, tv.ID)
		switch {
		case lerr != nil:
			why = append(why, "its close could not be planned: "+lerr.Msg)
		case !plan.Worktree || !plan.DeleteBranch:
			why = append(why, keptReasons(plan)...)
		}
		key := fmt.Sprintf("%s#%d", tv.ID, pr)
		if len(why) == 0 {
			// The verdict again under the lane lock: the plan took a while, and claude
			// may have started work since the view above was read.
			id, heldWhy := tv.ID, ""
			held := func(live types.LiveLane) string { heldWhy = r.hub.LaneCloseHeld(id, &live); return heldWhy }
			res, lerr := r.lanes.CloseIf(ctx, tv.ID, lanes.CloseRequest{Worktree: true, Branch: true}, held)
			switch {
			case lerr != nil && lerr.Code == "held" && heldWhy != "":
				why = append(why, heldWhy) // the verdict's own words, as the view's are given
			case lerr != nil:
				why = append(why, "Close failed: "+lerr.Msg)
			default:
				delete(a.asks, tv.ID)
				wt, id, detail := tv.Worktree, tv.ID, fmt.Sprintf("PR #%d merged; closed by lanes_auto_close: removed %s", pr, strings.Join(res.Removed, ", "))
				notes = append(notes, func(m *state.Model, now time.Time) { m.NoteLane(wt, id, "Lane closed", detail, now) })
				r.notifyOnce(ctx, key, "Lane "+tv.ID+" closed", fmt.Sprintf("Its pull request #%d merged; its worktree and branch are removed.", pr))
				continue
			}
		}
		if old, ok := a.asks[tv.ID]; ok && old.PR == pr {
			a.asks[tv.ID] = state.MergeAsk{PR: pr, Why: why, At: old.At}
		} else {
			a.asks[tv.ID] = state.MergeAsk{PR: pr, Why: why, At: now.UnixMilli()}
		}
		r.notifyOnce(ctx, key, "PR merged: close lane "+tv.ID+"?", "Its pull request #"+fmt.Sprint(pr)+" merged, but "+strings.Join(why, "; ")+".")
	}
	for id := range a.asks {
		if !live[id] {
			delete(a.asks, id)
		}
	}
	a.open = openNow
	asks := make(map[string]state.MergeAsk, len(a.asks))
	for k, x := range a.asks {
		asks[k] = x
	}
	return func(m *state.Model, now time.Time) {
		m.ApplyMergeAsks(asks)
		for _, n := range notes {
			n(m, now)
		}
	}, 0
}

// mergedPR asks gh whether the branch has a merged pull request; the newest one's number.
func (r *Runtime) mergedPR(ctx context.Context, branch string) (int, bool) {
	prs, err := r.mergedPRList(ctx, branch, "number", "5")
	n := 0
	for _, p := range prs {
		if p.Number > n {
			n = p.Number
		}
	}
	return n, err == nil && n > 0
}

// mergedPRRead is mergedPR's read for D3 (b), with its error kept apart (PANEL-51
// D10): the number of the merged pull request whose head commit is head, 0 for none,
// and an error when gh could not say, so a failed read is never taken for "none". A
// merge whose headRefOid is not head is not counted (#188): a branch name reused after
// a squash merge would otherwise have its old merge counted for the new HEAD. It is
// lanes/close.go's mergedBranch rule for Close, with that read's 20 rows, so a few
// older merges of the branch cannot push HEAD's out of the list.
func (r *Runtime) mergedPRRead(ctx context.Context, branch, head string) (int, error) {
	prs, err := r.mergedPRList(ctx, branch, "number,headRefOid", "20")
	if err != nil {
		return 0, err
	}
	for _, p := range prs {
		if head != "" && p.HeadRefOid == head {
			return p.Number, nil
		}
	}
	return 0, nil
}

// mergedRow is the part of a `gh pr list --state merged` row the merged reads use.
type mergedRow struct {
	Number     int    `json:"number"`
	HeadRefOid string `json:"headRefOid"`
}

// mergedPRList runs `gh pr list --head <branch> --state merged` for the fields given;
// an error when gh could not say, or its answer could not be read.
func (r *Runtime) mergedPRList(ctx context.Context, branch, fields, limit string) ([]mergedRow, error) {
	if !config.BranchRe.MatchString(branch) {
		return nil, fmt.Errorf("branch %q is not a name gh is asked about", signals.Clip(branch, 80))
	}
	out, err := r.exec(ctx, 30*time.Second, []string{"gh", "pr", "list", "--head", branch, "--state", "merged", "--json", fields, "--limit", limit})
	if err != nil {
		return nil, err
	}
	var prs []mergedRow
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &prs); err != nil {
		return nil, fmt.Errorf("gh pr list --state merged: %w", err)
	}
	return prs, nil
}

func keptReasons(p lanes.ClosePlan) []string {
	var out []string
	for _, k := range p.Keep {
		if strings.HasPrefix(k, "the conversation") {
			continue
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		out = append(out, "Close would not remove the worktree and the branch")
	}
	return out
}

// notifyOnce sends one OS notification per key for this panel run, when notifications
// are on.
func (r *Runtime) notifyOnce(ctx context.Context, key, title, body string) {
	a := &r.autoClose
	if a.notified[key] || !r.cfg.AlertThresholds().Notify {
		return
	}
	a.notified[key] = true
	send := r.o.Notify
	if send == nil {
		send = func(n state.Notice) error { return SendNotice(ctx, n) }
	}
	name := "clauductor panel"
	if r.trusted() {
		name = r.cfg.Name
	}
	if err := send(state.Notice{Group: "lifecycle", Title: name + ": " + title, Body: signals.Clip(body, 240)}); err != nil {
		fmt.Fprintf(r.o.Out, "notification failed: %v\n", err)
	}
}
