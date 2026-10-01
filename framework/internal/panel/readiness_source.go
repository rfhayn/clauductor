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

// PANEL-20: merge readiness's reads, only while a page is in view (as the dashboard's
// git reads): each lane worktree's gate receipt (a file in its git dir), the change's
// tasks.md in that worktree, and its pull request's review threads, one `gh api
// graphql` per pull request at most every threadsEvery.

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
}

func (r *Runtime) pollReadiness(ctx context.Context, now time.Time) (update, time.Duration) {
	if !r.pageVisible(now) {
		return nil, 0
	}
	c := &r.ready
	c.mu.Lock()
	if c.gitDirs == nil {
		c.gitDirs, c.threads = map[string]string{}, map[int]threadsRead{}
	}
	c.mu.Unlock()
	v := r.hub.View()
	dirs := signals.ChangesDirs(r.root)
	extras := map[string]state.LaneExtra{}
	for _, l := range v.Lanes {
		if l.Branch == "" || l.Path == r.root || ctx.Err() != nil {
			continue
		}
		x := state.LaneExtra{}
		c.mu.Lock()
		gd := c.gitDirs[l.Path]
		c.mu.Unlock()
		if gd == "" {
			if out, err := r.exec(ctx, 10*time.Second, []string{"git", "-C", l.Path, "rev-parse", "--absolute-git-dir"}); err == nil {
				gd = strings.TrimSpace(string(out))
				c.mu.Lock()
				c.gitDirs[l.Path] = gd
				c.mu.Unlock()
			}
		}
		if gd != "" {
			x.Receipt, x.HasReceipt = signals.ReadReceipt(gd)
		}
		// The change this branch builds: its tasks.md as this worktree has it.
		id := l.Branch
		if i := strings.LastIndexByte(id, '/'); i >= 0 {
			id = id[i+1:]
		}
		for _, d := range dirs {
			if b, err := signals.ReadTasks(filepath.Join(l.Path, d, id, "tasks.md")); err == nil {
				x.Change, x.Tasks = filepath.Join(d, id), true
				x.TasksOpen, x.TasksDone = signals.CountTasks(b)
				break
			}
		}
		for _, pr := range v.PRs {
			if pr.HeadRef != l.Branch || pr.IsDraft {
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
		extras[l.Path] = x
	}
	gate := len(r.cfg.Queues) > 0
	if _, err := os.Stat(filepath.Join(r.root, "scripts", "ci", "run-local.sh")); err == nil {
		gate = true
	}
	return func(m *state.Model, _ time.Time) { m.ApplyLaneExtras(extras, gate) }, 0
}
