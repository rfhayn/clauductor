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
)

// Close a lane when its pull request merges (PANEL-20; lanes_auto_close "on_merge").
// It is Close lane, planned exactly as the page's confirmation plans it, done by the
// panel only when that plan removes the worktree and the branch (clean, and merged at
// the branch's tip) and claude is idle or gone. Otherwise the lane asks in Needs you,
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
		idle := !tv.Running || tv.Dead || (tv.Status == "idle" && !tv.Approx)
		if !idle {
			why = append(why, "claude is "+statusWords(tv.Status, tv.Approx))
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
			res, lerr := r.lanes.Close(ctx, tv.ID, lanes.CloseRequest{Worktree: true, Branch: true})
			if lerr != nil {
				why = append(why, "Close failed: "+lerr.Msg)
			} else {
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
	if !config.BranchRe.MatchString(branch) {
		return 0, false
	}
	out, err := r.exec(ctx, 30*time.Second, []string{"gh", "pr", "list", "--head", branch, "--state", "merged", "--json", "number", "--limit", "5"})
	if err != nil {
		return 0, false
	}
	var prs []struct {
		Number int `json:"number"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(string(out))), &prs) != nil || len(prs) == 0 {
		return 0, false
	}
	n := 0
	for _, p := range prs {
		if p.Number > n {
			n = p.Number
		}
	}
	return n, true
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

func statusWords(st string, approx bool) string {
	w := map[string]string{"busy": "working", "waiting": "waiting on you", "running": "starting"}[st]
	if w == "" {
		w = st
	}
	if approx {
		w += " (not a current reading)"
	}
	return w
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
