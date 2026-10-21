package panel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-20: merge readiness's reads: each worktree's gate receipt (a file in its git
// dir), the change's tasks.md in a lane's worktree, and its pull request's review
// threads, one `gh api graphql` per pull request at most every threadsEvery.
//
// PANEL-51 (D10): the receipt read runs whether or not a page is in view, for every
// worktree with a branch of its own (QuietWorktrees included), since a stall matters
// most when nobody is looking; tasks.md and the review threads stay page-only. The
// merged read for D3 (b) runs here too, with no page.

const threadsEvery = 2 * time.Minute

type threadsRead struct {
	at                time.Time
	unresolved, total int
	err               string
}

type readinessCache struct {
	mu      sync.Mutex
	gitDirs map[string]string
	threads map[int]threadsRead
	// merged is the merged read's last result per worktree path, and open the
	// branches with an open pull request at the last poll (D10); the readiness
	// source's goroutine only.
	merged map[string]mergedEntry
	open   map[string]bool
}

// mergedEntry is one worktree's merged read: the last result that answered (a failed
// read keeps it), when a read was last tried and for which branch and HEAD, and
// whether that branch has left the open-PR list since, so it is read again at once.
type mergedEntry struct {
	read         state.MergedRead
	at           time.Time
	branch, head string
	again        bool
}

func (r *Runtime) pollReadiness(ctx context.Context, now time.Time) (update, time.Duration) {
	visible := r.pageVisible(now)
	if !visible && r.cfg.AlertThresholds().Stall <= 0 {
		// With no page the reads serve the stall rule alone (D10), so with stalls off
		// the source pauses, as it did before PANEL-51.
		return nil, 0
	}
	c := &r.ready
	c.mu.Lock()
	if c.gitDirs == nil {
		c.gitDirs, c.threads = map[string]string{}, map[int]threadsRead{}
	}
	c.mu.Unlock()
	v := r.hub.View()
	var wts []signals.Worktree
	prev := map[string]state.LaneExtra{}
	r.hub.Read(func(m *state.Model, _ time.Time) {
		wts = m.Worktrees()
		for _, wt := range wts {
			if x, ok := m.LaneExtraOf(wt.Path); ok {
				prev[wt.Path] = x
			}
		}
	})
	lanes := map[string]bool{} // the page's lanes: only they have tasks.md and threads
	for _, l := range v.Lanes {
		lanes[l.Path] = true
	}
	dirs := signals.ChangesDirs(r.root)
	extras := map[string]state.LaneExtra{}
	for _, wt := range wts {
		if ctx.Err() != nil {
			break
		}
		if wt.Bare || wt.Branch == "" || wt.Path == r.root {
			continue
		}
		x := state.LaneExtra{}
		if !visible {
			// With no page in view the page-only reads stay as last read, as they did
			// when this whole source paused with no page.
			x = prev[wt.Path]
		}
		x.Receipt, x.HasReceipt = signals.Receipt{}, false
		if gd := r.gitDirOf(ctx, wt.Path); gd != "" {
			x.Receipt, x.HasReceipt = signals.ReadReceipt(gd)
		}
		if visible && lanes[wt.Path] {
			r.readPageOnly(ctx, now, wt, dirs, v.PRs, &x)
		}
		extras[wt.Path] = x
	}
	r.pruneGitDirs(wts)
	var due []signals.Worktree
	r.hub.Read(func(m *state.Model, _ time.Time) { due = m.MergedReadsDue(extras) })
	reads := r.readMerged(ctx, now, v.PRs, wts, due)
	gate := len(r.cfg.Queues) > 0
	if _, err := os.Stat(filepath.Join(r.root, "scripts", "ci", "run-local.sh")); err == nil {
		gate = true
	}
	return func(m *state.Model, _ time.Time) {
		m.ApplyLaneExtras(extras, gate)
		m.ApplyMergedReads(reads)
	}, 0
}

// gitDirOf is the worktree's git dir, cached ("" while it cannot be read).
func (r *Runtime) gitDirOf(ctx context.Context, path string) string {
	c := &r.ready
	c.mu.Lock()
	gd := c.gitDirs[path]
	c.mu.Unlock()
	if gd != "" {
		return gd
	}
	// A listed worktree whose directory is gone (git calls it prunable) has no git dir
	// to read: rev-parse there only fails, and with nothing cached it would be spawned
	// again at every poll, page or no page (D10).
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	out, err := r.exec(ctx, 10*time.Second, servingGit(path, "rev-parse", "--absolute-git-dir"))
	if err != nil {
		return ""
	}
	gd = strings.TrimSpace(string(out))
	c.mu.Lock()
	c.gitDirs[path] = gd
	c.mu.Unlock()
	return gd
}

// pruneGitDirs drops the cached git dir of each worktree no longer listed, so the
// cache holds the list the model has, not every worktree the panel has seen.
func (r *Runtime) pruneGitDirs(wts []signals.Worktree) {
	live := make(map[string]bool, len(wts))
	for _, wt := range wts {
		live[wt.Path] = true
	}
	c := &r.ready
	c.mu.Lock()
	defer c.mu.Unlock()
	for path := range c.gitDirs {
		if !live[path] {
			delete(c.gitDirs, path)
		}
	}
}

// readPageOnly reads what merge readiness shows only on the page: the change's
// tasks.md as the lane's worktree has it, and its pull request's review threads.
func (r *Runtime) readPageOnly(ctx context.Context, now time.Time, wt signals.Worktree, dirs []string, prs []signals.PR, x *state.LaneExtra) {
	c := &r.ready
	// The change this branch builds: its tasks.md as this worktree has it.
	id := wt.Branch
	if i := strings.LastIndexByte(id, '/'); i >= 0 {
		id = id[i+1:]
	}
	for _, d := range dirs {
		if b, err := signals.ReadTasks(filepath.Join(wt.Path, d, id, "tasks.md")); err == nil {
			x.Change, x.Tasks = filepath.Join(d, id), true
			x.TasksOpen, x.TasksDone = signals.CountTasks(b)
			break
		}
	}
	for _, pr := range prs {
		if pr.HeadRef != wt.Branch || pr.IsDraft {
			continue
		}
		c.mu.Lock()
		t, ok := c.threads[pr.Number]
		c.mu.Unlock()
		if !ok || now.Sub(t.at) >= threadsEvery {
			out, err := r.exec(ctx, 30*time.Second, signals.ReviewThreadsArgv(pr.Number))
			t = threadsRead{at: now}
			if err == nil {
				t.unresolved, t.total, err = signals.ParseReviewThreads(out)
			}
			if err != nil {
				t.err = signals.Clip(err.Error(), 120)
			}
			c.mu.Lock()
			c.threads[pr.Number] = t
			c.mu.Unlock()
		}
		x.ThreadsPR, x.Unresolved, x.Threads, x.ThreadsErr = pr.Number, t.unresolved, t.total, t.err
	}
}

// readMerged runs the merged read for each worktree due one (a clean receipt for its
// HEAD, no open pull request: state.MergedReadsDue) and returns every worktree's last
// result (D10). Only a merge found is kept for good, per branch and HEAD: a squash
// merge leaves the branch's HEAD as it was, so a "none" read before the pull request
// was opened would otherwise stand after it merged. "None" is read again when the
// branch leaves the open-PR list (tracked here from the view's PRs, since auto-close's
// tracking runs only when it is on, and only for tmux lanes), and otherwise at most
// every Ticks.MergedNone. A read that fails keeps the last result; with none, the
// receipt stays unknown, never "none", and the read waits Ticks.MergedNone too, as
// "none" does (review round 3, L1), so a gh that keeps failing costs one read per lane
// per wait rather than one per lane per poll.
func (r *Runtime) readMerged(ctx context.Context, now time.Time, prs []signals.PR, wts, due []signals.Worktree) map[string]state.MergedRead {
	c := &r.ready
	if c.merged == nil {
		c.merged = map[string]mergedEntry{}
	}
	openNow := map[string]bool{}
	for _, pr := range prs {
		openNow[pr.HeadRef] = true
	}
	for path, e := range c.merged {
		if b := e.branch; b != "" && c.open[b] && !openNow[b] {
			e.again = true
			c.merged[path] = e
		}
	}
	c.open = openNow
	for _, wt := range due {
		if ctx.Err() != nil {
			break
		}
		e := c.merged[wt.Path]
		found := e.read.Read && e.read.PR > 0 && e.read.Branch == wt.Branch && e.read.Head == wt.Head
		tried := e.branch == wt.Branch && e.head == wt.Head // the last read, answered or not, was for this branch and HEAD
		if found || (tried && !e.again && now.Sub(e.at) < r.ticks.MergedNone) {
			continue
		}
		n, err := r.mergedPRRead(ctx, wt.Branch, wt.Head)
		e.at, e.branch, e.head = now, wt.Branch, wt.Head
		switch {
		case err == nil:
			// At is when the read answered, not the poll's now: a pull-request update
			// applied while it ran stamps leftOpen later than now, and a "none" stamped
			// before it would count for nothing for a whole wait (round 2, L1).
			e.read, e.again = state.MergedRead{Branch: wt.Branch, Head: wt.Head, Read: true, PR: n, At: r.clock.Now()}, false
		case e.read.Branch != wt.Branch || e.read.Head != wt.Head:
			// No result for this branch and HEAD to correct, so (b) is unknown either way:
			// the wait applies. With a "none" kept from before the branch left the open-PR
			// list, again stays set, since that "none" may now be a merge (a false stall).
			e.again = false
		}
		c.merged[wt.Path] = e
	}
	live := map[string]bool{}
	for _, wt := range wts {
		live[wt.Path] = true
	}
	out := map[string]state.MergedRead{}
	for path, e := range c.merged {
		if !live[path] {
			delete(c.merged, path)
			continue
		}
		if e.read.Read {
			out[path] = e.read
		}
	}
	return out
}
