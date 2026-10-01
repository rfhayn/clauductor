package lanes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Remove a worktree (PANEL-18) is Close lane's cleanup for a worktree that has no lane:
// a clean worktree a closed session left behind, detached or on a branch. It decides
// from git's own state exactly as Close does (worktreeVerdict, branchVerdict), twice:
// once for the page's confirmation, again when it acts. On top of Close's rules it
// refuses a worktree any lane is registered in (running or orphaned: Close lane is
// that lane's control), and one a claude session runs in, per `claude agents`, where
// a session started in a terminal of your own shows. An unreadable `claude agents`
// refuses, since such a session could not be ruled out.
//
// The page names the worktree by the key its state gave it (a worktree's key is its
// resolved path), and the key must be an entry of `git worktree list` of this project
// as it is now: a path the list does not have is refused before anything runs in it.

// WorktreePlan is what Remove would do to a worktree now.
type WorktreePlan struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Head   string `json:"head,omitempty"`
	// Worktree and DeleteBranch are what the confirmation offers to remove; the page
	// sends them back as the consent.
	Worktree     bool     `json:"worktree"`
	DeleteBranch bool     `json:"deleteBranch"`
	Remove       []string `json:"remove"`
	Keep         []string `json:"keep"`
	Notes        []string `json:"notes,omitempty"`
}

// RemoveWorktreeRequest is the body of POST …/worktrees/remove. Worktree is the
// worktree's key from the page's state. DryRun asks for the plan only; otherwise
// Remove and Branch are what the confirmation offered: nothing it did not offer goes.
type RemoveWorktreeRequest struct {
	Worktree string `json:"worktree"`
	DryRun   bool   `json:"dryRun"`
	Remove   bool   `json:"remove"`
	Branch   bool   `json:"branch"`
}

// WorktreeResult reports a removal.
type WorktreeResult struct {
	Path    string   `json:"path"`
	Removed []string `json:"removed"`
	Kept    []string `json:"kept"`
	Notes   []string `json:"notes,omitempty"`
}

// findWorktree returns the entry of `git worktree list` whose path is key, exactly.
func (m *LaneManager) findWorktree(ctx context.Context, key string) (signals.Worktree, *LaneError) {
	if key == "" || len(key) > 4096 || !strings.HasPrefix(key, "/") || strings.ContainsRune(key, 0) {
		return signals.Worktree{}, laneErr(400, "invalid", "worktree must be a worktree's key from the page's state")
	}
	wts, err := signals.ReadWorktrees(ctx, m.Run, m.Root)
	if err != nil {
		return signals.Worktree{}, laneErr(503, "git", "cannot read git worktree list: %v", err)
	}
	for _, w := range wts {
		if !w.Bare && w.Path == key {
			return w, nil
		}
	}
	return signals.Worktree{}, laneErr(404, "not-found", "%s is not one of this project's worktrees (git worktree list)", key)
}

// removeVerdict says why the worktree at path must stay, or "" when it may go: Close's
// rules, then no lane of any kind and no claude session in it.
func (m *LaneManager) removeVerdict(ctx context.Context, wts []signals.Worktree, path string) string {
	if _, why := m.worktreeVerdict(ctx, path, ""); why != "" {
		return why
	}
	in := func(dir string) bool { return dir == path || strings.HasPrefix(dir, path+"/") }
	for _, rec := range m.Registry.List() {
		if rec.Path != "" && in(signals.ResolvePath(rec.Path)) {
			return "lane " + rec.ID + " is registered in it (Close lane on that lane removes its worktree)"
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := m.Run(cctx, m.Root, []string{"claude", "agents", "--json"})
	var agents []signals.Agent
	if err == nil {
		agents, err = signals.ParseAgents(out)
	}
	if err != nil {
		return "cannot read claude agents (" + err.Error() + "), so a claude session in it cannot be ruled out"
	}
	for _, a := range agents {
		if a.Cwd == "" {
			continue
		}
		// The deepest worktree containing the session's directory: a session in the
		// main checkout is not in a worktree nested inside it, and the reverse.
		if i := signals.MatchWorktree(wts, signals.ResolvePath(a.Cwd)); i >= 0 && wts[i].Path == path {
			return fmt.Sprintf("a claude session runs in it (pid %d, %s)", a.PID, a.Status)
		}
	}
	return ""
}

func detachedNote(w signals.Worktree) string {
	return "no branch: the worktree is detached (HEAD at " + short(w.Head) + "), so there is no branch to delete"
}

// RemoveWorktreePlan is what Remove would do now, for the page's confirmation. It
// fetches first, as ClosePlan does, so a branch merged upstream is seen as merged.
func (m *LaneManager) RemoveWorktreePlan(ctx context.Context, key string) (WorktreePlan, *LaneError) {
	if _, lerr := m.findWorktree(ctx, key); lerr != nil {
		return WorktreePlan{}, lerr
	}
	fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	_, ferr := m.Run(fctx, m.Root, []string{"git", "fetch", "--quiet"})
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	w, lerr := m.findWorktree(ctx, key)
	if lerr != nil {
		return WorktreePlan{}, lerr
	}
	wts, err := signals.ReadWorktrees(ctx, m.Run, m.Root)
	if err != nil {
		return WorktreePlan{}, laneErr(503, "git", "cannot read git worktree list: %v", err)
	}
	p := WorktreePlan{Path: w.Path, Branch: w.Branch, Head: w.Head, Remove: []string{}, Keep: []string{}}
	if why := m.removeVerdict(ctx, wts, w.Path); why != "" {
		p.Keep = append(p.Keep, "the worktree "+w.Path+": "+why)
	} else {
		p.Worktree = true
		p.Remove = append(p.Remove, "the worktree "+w.Path+" (clean: no changes, no untracked files; git worktree remove, without --force)")
	}
	switch {
	case w.Branch == "":
		p.Notes = append(p.Notes, detachedNote(w))
	case !p.Worktree:
		p.Keep = append(p.Keep, "the branch "+w.Branch+": its worktree stays")
	default:
		if _, ok, why := m.branchVerdict(ctx, w.Branch); ok != "" {
			p.DeleteBranch = true
			p.Remove = append(p.Remove, "the local branch "+w.Branch+": "+ok)
		} else {
			p.Keep = append(p.Keep, "the local branch "+w.Branch+": "+why)
		}
	}
	if ferr != nil {
		p.Notes = append(p.Notes, fmt.Sprintf("git fetch failed (%v): merged is judged against the last fetched %s", ferr, m.Cfg.BaseRef()))
	}
	return p, nil
}

// RemoveWorktree removes a lane-less worktree, and its branch, where the consent and a
// fresh check both allow. What stays is reported with why.
func (m *LaneManager) RemoveWorktree(ctx context.Context, key string, consent RemoveWorktreeRequest) (WorktreeResult, *LaneError) {
	res := WorktreeResult{Path: key, Removed: []string{}, Kept: []string{}}
	m.mu.Lock()
	defer m.mu.Unlock()
	w, lerr := m.findWorktree(ctx, key)
	if lerr != nil {
		return res, lerr
	}
	wts, err := signals.ReadWorktrees(ctx, m.Run, m.Root)
	if err != nil {
		return res, laneErr(503, "git", "cannot read git worktree list: %v", err)
	}
	keepBranch := func(why string) {
		if w.Branch != "" {
			res.Kept = append(res.Kept, "the branch "+w.Branch+": "+why)
		}
	}
	switch why := m.removeVerdict(ctx, wts, w.Path); {
	case why != "":
		res.Kept = append(res.Kept, "the worktree "+w.Path+": "+why)
		keepBranch("its worktree stays")
		return res, nil
	case !consent.Remove:
		res.Kept = append(res.Kept, "the worktree "+w.Path+": the confirmation did not include it")
		keepBranch("its worktree stays")
		return res, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// No --force: git refuses a dirty or locked worktree itself, a second guard.
	if _, err := m.Run(cctx, m.Root, []string{"git", "worktree", "remove", w.Path}); err != nil {
		res.Kept = append(res.Kept, "the worktree "+w.Path+": git worktree remove refused ("+err.Error()+")")
		keepBranch("its worktree stays")
		return res, nil
	}
	defer m.changed()
	res.Removed = append(res.Removed, "the worktree "+w.Path)
	if w.Branch == "" {
		res.Notes = append(res.Notes, detachedNote(w))
		return res, nil
	}
	if removed, kept := m.deleteBranch(cctx, w.Branch, consent.Branch); removed != "" {
		res.Removed = append(res.Removed, removed)
	} else {
		res.Kept = append(res.Kept, kept)
	}
	return res, nil
}

// deleteBranch deletes a local branch whose worktree is gone, when branchVerdict
// allows it now and the confirmation offered it: by `git update-ref -d` at the tip
// just checked, so a branch that moved meanwhile stays. It returns the line for
// Removed, or else the line for Kept.
func (m *LaneManager) deleteBranch(ctx context.Context, branch string, consent bool) (removed, kept string) {
	head, ok, why := m.branchVerdict(ctx, branch)
	switch {
	case ok == "":
		return "", "the local branch " + branch + ": " + why
	case !consent:
		return "", "the local branch " + branch + ": the confirmation did not include it"
	}
	if _, err := m.Run(ctx, m.Root, []string{"git", "update-ref", "-d", "refs/heads/" + branch, head}); err != nil {
		return "", "the local branch " + branch + ": git update-ref refused (" + err.Error() + ")"
	}
	// Its upstream and other settings go with it, as `git branch -d` does.
	_, _ = m.Run(ctx, m.Root, []string{"git", "config", "--remove-section", "branch." + branch})
	return "the local branch " + branch + " (at " + short(head) + "; " + ok + ")", ""
}
