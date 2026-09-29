package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// orchestration is what the HTTP layer needs from the runtime.
func (r *Runtime) orchestration() *web.Orchestration {
	return &web.Orchestration{
		Trusted: r.trusted,
		QuotaGuard: func() string {
			why := ""
			r.hub.Read(func(m *state.Model, now time.Time) { why = m.QuotaGuard(now) })
			return why
		},
		CancelWait: r.cancelWait,
		RunQueue:   r.runQueue,
	}
}

// gitCommonDir is where queue locks live: shared by every worktree of the project.
func (r *Runtime) gitCommonDir(ctx context.Context) (string, error) {
	r.mu.Lock()
	d := r.gitDir
	r.mu.Unlock()
	if d != "" {
		return d, nil
	}
	out, err := r.run(ctx, r.root, []string{"git", "rev-parse", "--git-common-dir"})
	if err != nil {
		return "", err
	}
	d = strings.TrimSpace(string(out))
	if !filepath.IsAbs(d) {
		d = filepath.Join(r.root, d)
	}
	d = signals.ResolvePath(d)
	r.mu.Lock()
	r.gitDir = d
	r.mu.Unlock()
	return d, nil
}

func (r *Runtime) queueLock(ctx context.Context, id string) (lease.QueueConfig, string, error) {
	for _, q := range r.cfg.Queues {
		if q.ID == id {
			d, err := r.gitCommonDir(ctx)
			if err != nil {
				return q, "", err
			}
			return q, filepath.Join(d, filepath.Clean(q.Lock)), nil
		}
	}
	return lease.QueueConfig{}, "", fmt.Errorf("no queue %q", id)
}

// pollQueues reads every queue's lease. It only reads, and updates the model only
// when what it read changed.
func (r *Runtime) pollQueues(ctx context.Context, now time.Time) (update, time.Duration) {
	qs, err := r.readQueues(ctx, now)
	b, _ := json.Marshal(qs)
	sig := string(b)
	if err != nil {
		sig = err.Error()
	}
	if sig == r.lastQ {
		return nil, 0
	}
	r.lastQ = sig
	return func(m *state.Model, now time.Time) { m.ApplyQueues(qs, err, now) }, 0
}

// readQueues reads every queue once. Liveness goes through the pid cache: kill(pid,
// 0) every time, ps once per process (PANEL-7).
func (r *Runtime) readQueues(ctx context.Context, now time.Time) ([]lease.QueueView, error) {
	var qs []lease.QueueView
	for _, q := range r.cfg.Queues {
		_, lock, err := r.queueLock(ctx, q.ID)
		if err != nil {
			return qs, err
		}
		v := lease.ReadQueue(q, lock, now, r.procs.Check)
		r.mu.Lock()
		if run := r.runs[q.ID]; run != nil {
			rc := *run
			v.Run = &rc
		}
		r.mu.Unlock()
		qs = append(qs, v)
	}
	return qs, nil
}

func (r *Runtime) cancelWait(queue, nonce string) error {
	_, lock, err := r.queueLock(context.Background(), queue)
	if err != nil {
		return err
	}
	return lease.CancelWait(lock, nonce)
}

// runQueue starts the queue's command through lock-run in one of the project's
// worktrees, detached, with its output in a log file. It waits its turn like any
// other gate run; the panel never skips the queue.
func (r *Runtime) runQueue(ctx context.Context, queue, worktree string) (*lease.QueueRun, error) {
	if !r.trusted() {
		return nil, errUntrusted
	}
	q, lock, err := r.queueLock(ctx, queue)
	if err != nil {
		return nil, err
	}
	if len(q.Command) == 0 {
		return nil, fmt.Errorf("queue %q has no command", queue)
	}
	wts, err := signals.ReadWorktrees(ctx, r.run, r.root)
	if err != nil {
		return nil, err
	}
	dir := ""
	for _, w := range wts {
		if !w.Bare && w.Path == signals.ResolvePath(worktree) {
			dir = w.Path
		}
	}
	if dir == "" {
		return nil, fmt.Errorf("%q is not one of this project's worktrees", worktree)
	}
	r.mu.Lock()
	if run := r.runs[queue]; run != nil && run.Exit == nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("a RUN of %s started by the panel is still going (pid %d)", queue, run.PID)
	}
	r.mu.Unlock()
	lane := "panel"
	var terms []state.TermLaneView
	r.hub.Read(func(m *state.Model, now time.Time) { terms = m.TerminalViews(now) })
	for _, t := range terms {
		if t.Running && t.Worktree == dir {
			lane = t.ID
		}
	}
	argv := r.o.LockRunArgv
	if len(argv) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		argv = []string{exe, "lock-run"}
	}
	argv = append(append(append([]string{}, argv...), "--lane", lane, lock, "--"), q.Command...)
	logDir := filepath.Join(config.ProjectDir(r.o.Home, r.root), "queue-logs")
	if err := config.EnsurePrivateDir(logDir); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", queue, r.clock.Now().Format("20060102-150405")))
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = append(os.Environ(), "CLAUDUCTOR_LANE="+lane)
	// Its own process group: a panel restart or Ctrl-C does not kill a gate run.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, err
	}
	run := &lease.QueueRun{PID: cmd.Process.Pid, Worktree: dir, Log: logPath, Started: r.clock.Now().UnixMilli()}
	r.mu.Lock()
	r.runs[queue] = run
	r.mu.Unlock()
	go func() {
		err := cmd.Wait()
		logf.Close()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			code = -1
		}
		r.mu.Lock()
		run.Ended, run.Exit = r.clock.Now().UnixMilli(), &code
		r.mu.Unlock()
	}()
	r.mu.Lock()
	rc := *run
	r.mu.Unlock()
	return &rc, nil
}
