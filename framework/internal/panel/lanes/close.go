package lanes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// Close lane (PANEL-17) is Stop, then the lane's worktree and branch when removing
// them loses nothing. Stop never removes a worktree, so without Close a finished lane
// left one behind for every branch. What Close removes is decided from git's own
// state, twice: once for the page's confirmation (Plan), which lists exactly what will
// go and what will stay, and again when it acts, after claude has exited (exiting can
// write files). It removes only what the confirmation listed AND is still safe then.
//
// It never forces anything:
//   - the worktree goes only with `git worktree remove` (no --force): never the main
//     worktree, never a path outside the project's worktree_dir or not in `git
//     worktree list`, never a locked one, never one another lane runs in, and never one
//     with a change or an untracked file (`git status --porcelain` not empty), and
//     never a detached one whose commit no branch, remote-tracking branch or tag holds
//     (its HEAD goes with the folder, and nothing would refer to the commit then);
//   - the branch goes only after its worktree, and only when its tip is in the
//     configured base, or it is the head of a merged pull request (a squash merge
//     leaves no commit of the branch in the base, so `git branch -d` would refuse).
//     It is deleted by `git update-ref -d <ref> <the tip checked>`, which does nothing
//     if the branch moved meanwhile.

// ClosePlan is what Close would do to a lane now.
type ClosePlan struct {
	ID         string `json:"id"`
	Running    bool   `json:"running"`
	Registered bool   `json:"registered"`
	Path       string `json:"path,omitempty"`
	Branch     string `json:"branch,omitempty"`
	// Worktree and DeleteBranch are what the confirmation offers to remove; the page
	// sends them back as the consent.
	Worktree     bool     `json:"worktree"`
	DeleteBranch bool     `json:"deleteBranch"`
	Remove       []string `json:"remove"`
	Keep         []string `json:"keep"`
	Notes        []string `json:"notes,omitempty"`
}

// CloseRequest is the body of POST …/lanes/{id}/close. DryRun asks for the plan
// only. Otherwise Worktree and Branch are what the confirmation offered to remove:
// nothing it did not offer is removed.
type CloseRequest struct {
	DryRun   bool `json:"dryRun"`
	Worktree bool `json:"worktree"`
	Branch   bool `json:"branch"`
}

// CloseResult reports a close.
type CloseResult struct {
	ID      string   `json:"id"`
	Removed []string `json:"removed"`
	Kept    []string `json:"kept"`
}

// closeTarget is the lane's directory and whether it can be trusted as one.
func (m *LaneManager) closeTarget(ctx context.Context, id string) (path string, rec types.LaneRecord, registered, running bool) {
	rec, registered = m.Registry.Get(id)
	lane, running := m.find(ctx, id)
	switch {
	case registered && rec.Corrupt == "":
		path = rec.Path
	case !registered && running:
		path = lane.Path
	}
	return path, rec, registered, running
}

// worktreeVerdict says why the worktree at path must stay, or "" when `git worktree
// remove` may take it. w is its `git worktree list` entry; held, for a detached one
// that may go, names a ref that holds its commit.
func (m *LaneManager) worktreeVerdict(ctx context.Context, path, id string) (w signals.Worktree, held, why string) {
	w, why = m.worktreeCheck(ctx, path, id)
	if why != "" || w.Branch != "" {
		return w, "", why
	}
	held, why = m.detachedVerdict(ctx, w.Head)
	return w, held, why
}

// detachedVerdict names a ref that holds a detached worktree's commit, or says why the
// worktree must stay: removing the folder removes the HEAD that refers to the commit,
// and git deletes a commit nothing refers to. A failed read keeps it too.
func (m *LaneManager) detachedVerdict(ctx context.Context, head string) (held, why string) {
	if head == "" {
		return "", "the panel couldn't tell whether its commit is on a branch (git worktree list gave no HEAD), so it stays"
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Not refs/stash: a stash is not where anyone looks for a commit.
	out, err := m.Run(cctx, m.Root, []string{"git", "for-each-ref", "--contains=" + head, "--format=%(refname)%09%(symref)", "refs/heads", "refs/remotes", "refs/tags"})
	if err != nil {
		return "", "the panel couldn't tell whether its commit " + short(head) + " is on a branch (" + err.Error() + "), so it stays"
	}
	// Sorted by refname: refs/heads, then refs/remotes, then refs/tags, each by name. A
	// symbolic ref (origin/HEAD) is skipped: the ref it names is listed itself.
	for _, l := range nonEmptyLines(out) {
		ref, sym, _ := strings.Cut(l, "\t")
		switch {
		case sym != "":
		case strings.HasPrefix(ref, "refs/heads/"):
			return strings.TrimPrefix(ref, "refs/heads/"), ""
		case strings.HasPrefix(ref, "refs/remotes/"):
			return strings.TrimPrefix(ref, "refs/remotes/"), ""
		case strings.HasPrefix(ref, "refs/tags/"):
			return "tag " + strings.TrimPrefix(ref, "refs/tags/"), ""
		}
	}
	return "", "its commit " + short(head) + " is on no branch or tag, so removing the folder would lose it. To keep it, put it on a branch first: git branch <name> " + short(head)
}

// detachedRecheck says why a detached worktree must stay after all, read just before
// `git worktree remove`: a commit made in it since the check (a terminal, a teardown)
// would otherwise go with the folder. It narrows that window; nothing can close it
// while something outside the panel can commit there.
func (m *LaneManager) detachedRecheck(ctx context.Context, w signals.Worktree) string {
	if w.Branch != "" {
		return ""
	}
	out, err := m.Run(ctx, w.Path, []string{"git", "rev-parse", "--verify", "--quiet", "HEAD"})
	head := strings.TrimSpace(string(out))
	if err != nil || head == "" {
		return "the panel couldn't read its HEAD again before removing it, so it stays"
	}
	if head == w.Head {
		return ""
	}
	_, why := m.detachedVerdict(ctx, head)
	return why
}

// worktreeCheck is worktreeVerdict before the detached commit's check.
func (m *LaneManager) worktreeCheck(ctx context.Context, path, id string) (w signals.Worktree, why string) {
	if path == "" {
		return w, "the lane's directory is not known (a corrupt registry record is not trusted)"
	}
	wts, err := signals.ReadWorktrees(ctx, m.Run, m.Root)
	if err != nil {
		return w, "cannot read git worktree list: " + err.Error()
	}
	want := signals.ResolvePath(path)
	idx := -1
	for i, x := range wts {
		if !x.Bare && x.Path == want {
			idx, w = i, x
		}
	}
	switch {
	case idx < 0:
		if _, err := os.Stat(want); os.IsNotExist(err) {
			return w, "it no longer exists"
		}
		return w, "it is not one of this project's worktrees"
	case idx == 0 || want == signals.ResolvePath(m.Root):
		return w, "it is the project's main worktree, which is never removed"
	}
	root := signals.ResolvePath(m.Cfg.WorktreeRoot(m.Root))
	if !strings.HasPrefix(want, root+string(filepath.Separator)) {
		return w, "it is outside the project's worktree_dir (" + root + "), and the panel removes only what it would create"
	}
	if w.Locked {
		return w, "it is locked (git worktree unlock it first if it should go)"
	}
	if other := m.pathTaken(ctx, want, id); other != "" {
		return w, "lane " + other + " runs in it"
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := m.Run(cctx, want, []string{"git", "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none"})
	if err != nil {
		return w, "cannot read git status: " + err.Error()
	}
	if lines := nonEmptyLines(out); len(lines) > 0 {
		names := make([]string, 0, 3)
		for _, l := range lines {
			if len(names) == 3 {
				names = append(names, "…")
				break
			}
			if len(l) > 3 {
				names = append(names, strings.TrimSpace(l[3:]))
			}
		}
		return w, fmt.Sprintf("it has %d uncommitted or untracked file%s (%s)", len(lines), plural(len(lines)), strings.Join(names, ", "))
	}
	return w, ""
}

func nonEmptyLines(b []byte) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// mergedPR is the part of a `gh pr list --state merged` row the branch check reads.
type mergedPR struct {
	Number     int    `json:"number"`
	HeadRefOid string `json:"headRefOid"`
}

// branchVerdict says whether a branch may be deleted: its tip (head) and, when it
// may, why (merged into the base, or by which pull request); when it must stay, why.
func (m *LaneManager) branchVerdict(ctx context.Context, branch string) (head, ok, why string) {
	base := m.Cfg.BaseRef()
	if !config.BranchRe.MatchString(branch) {
		return "", "", "it is not a branch name the panel would create"
	}
	if branch == base || branch == strings.TrimPrefix(base, "origin/") {
		return "", "", "it is the base branch"
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := m.Run(cctx, m.Root, []string{"git", "rev-parse", "--verify", "--quiet", "refs/heads/" + branch + "^{commit}"})
	head = strings.TrimSpace(string(out))
	if err != nil || head == "" {
		return "", "", "cannot read its tip"
	}
	out, err = m.Run(cctx, m.Root, []string{"git", "for-each-ref", "--merged=" + base, "--format=%(objectname)", "refs/heads/" + branch})
	if err != nil {
		return head, "", "cannot check it against " + base + ": " + err.Error()
	}
	if strings.TrimSpace(string(out)) == head {
		return head, "every commit of it is in " + base, ""
	}
	// A squash (or rebase) merge leaves none of the branch's commits in the base: ask
	// the pull requests, as the panel's PR source does, and trust one only when its
	// head is the branch's tip now.
	out, err = m.Run(cctx, m.Root, []string{"gh", "pr", "list", "--head", branch, "--state", "merged", "--json", "number,headRefOid", "--limit", "20"})
	if err != nil {
		return head, "", "it is not merged into " + base + ", and gh could not say whether its pull request was merged (" + err.Error() + ")"
	}
	var prs []mergedPR
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &prs); err != nil {
		return head, "", "it is not merged into " + base + ", and gh's answer could not be read"
	}
	later := 0
	for _, p := range prs {
		if p.HeadRefOid == head {
			return head, fmt.Sprintf("its pull request #%d was merged", p.Number), ""
		}
		later = p.Number
	}
	if later != 0 {
		return head, "", fmt.Sprintf("pull request #%d was merged, but the branch has commits since", later)
	}
	return head, "", "it is not merged into " + base + " and has no merged pull request"
}

// planLocked is the plan from the state now. Its lines are the confirmation's.
func (m *LaneManager) planLocked(ctx context.Context, id string) (ClosePlan, *LaneError) {
	path, rec, registered, running := m.closeTarget(ctx, id)
	if !registered && !running {
		return ClosePlan{}, laneErr(404, "not-found", "no lane %q", id)
	}
	p := ClosePlan{ID: id, Running: running, Registered: registered, Path: path, Remove: []string{}, Keep: []string{}}
	if running {
		p.Remove = append(p.Remove, "claude and the tmux session of lane "+id+", ended as Stop lane ends them")
	}
	if registered {
		p.Remove = append(p.Remove, "the lane's registry record")
	}
	if n := m.imageCount(id); n > 0 {
		p.Remove = append(p.Remove, fmt.Sprintf("%d image%s dropped on the lane", n, plural(n)))
	}
	w, held, why := m.worktreeVerdict(ctx, path, id)
	p.Branch = w.Branch
	shown := path
	if shown == "" {
		shown = "(unknown)"
	}
	if why != "" {
		p.Keep = append(p.Keep, "the worktree "+shown+": "+why)
	} else {
		p.Worktree = true
		p.Remove = append(p.Remove, "the worktree "+w.Path+" (clean: no changes, no untracked files)")
		if h := m.Cfg.WorktreeTeardown; h != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("worktree_teardown (%s) runs in it first; if it fails or leaves files, the worktree stays", strings.Join(h.Command, " ")))
		}
	}
	switch {
	case w.Branch == "":
		if held != "" {
			p.Notes = append(p.Notes, detachedNote(w, held))
		}
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
	if registered && rec.SessionID != "" {
		p.Keep = append(p.Keep, "the conversation (claude --resume "+rec.SessionID+" still opens it)")
	}
	return p, nil
}

func (m *LaneManager) imageCount(id string) int {
	dir := m.ImageDir(id)
	if dir == "" {
		return 0
	}
	fs, _ := os.ReadDir(dir)
	return len(fs)
}

// ClosePlan is what Close would do now, for the page's confirmation. It fetches first,
// so a branch merged upstream is seen as merged; a failed fetch is a note, and only
// makes the plan keep more.
func (m *LaneManager) ClosePlan(ctx context.Context, id string) (ClosePlan, *LaneError) {
	if !config.ValidLaneID(id) {
		return ClosePlan{}, laneErr(400, "invalid", "invalid lane id")
	}
	m.mu.Lock()
	_, _, registered, running := m.closeTarget(ctx, id)
	m.mu.Unlock()
	if !registered && !running {
		return ClosePlan{}, laneErr(404, "not-found", "no lane %q", id)
	}
	// Outside the lane lock: a slow remote must not hold up every other lane.
	fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	_, ferr := m.Run(fctx, m.Root, []string{"git", "fetch", "--quiet"})
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	p, lerr := m.planLocked(ctx, id)
	if lerr == nil && ferr != nil {
		p.Notes = append(p.Notes, fmt.Sprintf("git fetch failed (%v): merged is judged against the last fetched %s", ferr, m.Cfg.BaseRef()))
	}
	return p, lerr
}

// Close stops the lane exactly as Stop does (or forgets an orphan), removes its
// dropped images, then removes its worktree and branch where the consent and a fresh
// check both allow. A worktree or branch that stays is reported with why; the lane is
// stopped either way.
func (m *LaneManager) Close(ctx context.Context, id string, consent CloseRequest) (CloseResult, *LaneError) {
	res := CloseResult{ID: id, Removed: []string{}, Kept: []string{}}
	if !config.ValidLaneID(id) {
		return res, laneErr(400, "invalid", "invalid lane id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, rec, registered, running := m.closeTarget(ctx, id)
	if !registered && !running {
		return res, laneErr(404, "not-found", "no lane %q", id)
	}
	// The teardown's environment is read now: stopAndForgetLocked deletes the lane's
	// record, and with it the port the teardown is documented to get.
	port := rec.Port
	images := m.imageCount(id)
	if lerr := m.stopAndForgetLocked(ctx, id); lerr != nil {
		return res, lerr
	}
	if running {
		res.Removed = append(res.Removed, "claude and the tmux session of lane "+id)
	}
	if registered {
		res.Removed = append(res.Removed, "the lane's registry record")
	}
	if images > 0 {
		res.Removed = append(res.Removed, fmt.Sprintf("%d dropped image%s", images, plural(images)))
	}
	defer m.changed()

	// Checked again now that claude has exited: its exit may have written files.
	w, _, why := m.worktreeVerdict(ctx, path, id)
	shown := path
	if shown == "" {
		shown = "(unknown)"
	}
	switch {
	case why != "":
		res.Kept = append(res.Kept, "the worktree "+shown+": "+why)
		if w.Branch != "" {
			res.Kept = append(res.Kept, "the branch "+w.Branch+": its worktree stays")
		}
		return res, nil
	case !consent.Worktree:
		res.Kept = append(res.Kept, "the worktree "+w.Path+": the confirmation did not include it")
		if w.Branch != "" {
			res.Kept = append(res.Kept, "the branch "+w.Branch+": its worktree stays")
		}
		return res, nil
	}
	// PANEL-20: the worktree's teardown, then the check again (teardown may leave files).
	if ran, err := m.runHook(ctx, "worktree_teardown", m.Cfg.WorktreeTeardown, w.Path, id, port); err != nil {
		res.Kept = append(res.Kept, "the worktree "+w.Path+": "+err.Error())
		if w.Branch != "" {
			res.Kept = append(res.Kept, "the branch "+w.Branch+": its worktree stays")
		}
		return res, nil
	} else if ran {
		res.Removed = append(res.Removed, "worktree_teardown ran in "+w.Path)
		if _, _, why := m.worktreeVerdict(ctx, w.Path, id); why != "" {
			res.Kept = append(res.Kept, "the worktree "+w.Path+": after worktree_teardown, "+why)
			if w.Branch != "" {
				res.Kept = append(res.Kept, "the branch "+w.Branch+": its worktree stays")
			}
			return res, nil
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if why := m.detachedRecheck(cctx, w); why != "" {
		res.Kept = append(res.Kept, "the worktree "+w.Path+": "+why)
		return res, nil
	}
	// No --force: git refuses a dirty or locked worktree itself, a second guard.
	if _, err := m.Run(cctx, m.Root, []string{"git", "worktree", "remove", w.Path}); err != nil {
		res.Kept = append(res.Kept, "the worktree "+w.Path+": git worktree remove refused ("+err.Error()+")")
		return res, nil
	}
	res.Removed = append(res.Removed, "the worktree "+w.Path)
	if w.Branch == "" {
		return res, nil
	}
	if removed, kept := m.deleteBranch(cctx, w.Branch, consent.Branch); removed != "" {
		res.Removed = append(res.Removed, removed)
	} else {
		res.Kept = append(res.Kept, kept)
	}
	return res, nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
