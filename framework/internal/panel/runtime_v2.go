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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// errUntrusted is what a card shows while the config is untrusted.
var errUntrusted = errors.New("not run: panel.json changed since you trusted it; run `clauductor panel trust`")

// Poll tunables.
const (
	agentsFast = 2 * time.Second
	// agentsSlow is the `claude agents` interval while hooks are flowing: the hooks
	// already carry the state, and each poll spawns a ~100 ms, ~150 MB process
	// (measured on 2.1.284: p50 98 ms wall, ~105 ms CPU).
	agentsSlow   = 5 * time.Second
	hooksFlowing = 30 * time.Second
	// agentsQuiet is the interval while the panel has no lane and has heard no hook
	// for hooksQuiet (PANEL-7): nothing is happening that a hook would not announce.
	agentsQuiet    = 15 * time.Second
	hooksQuiet     = 5 * time.Minute
	filterRecheck  = 5 * time.Minute
	versionRecheck = 10 * time.Minute
	// promptKickEvery is the fastest the first-prompt loop drives `claude agents`
	// polls: the fast interval, never faster.
	promptKickEvery = agentsFast
)

// AgentsInterval is how long to wait before the next `claude agents` poll: slow
// while hooks are flowing; quiet with no lane (registered or on the socket) and no
// hook for hooksQuiet; fast otherwise. A zero lastHook is no hook heard.
func AgentsInterval(lastHook, now time.Time, lanes bool) time.Duration {
	heard := !lastHook.IsZero()
	switch {
	case heard && now.Sub(lastHook) < hooksFlowing:
		return agentsSlow
	case !lanes && (!heard || now.Sub(lastHook) >= hooksQuiet):
		return agentsQuiet
	}
	return agentsFast
}

// Orchestration is what the HTTP layer needs from the v2 runtime.
type Orchestration struct {
	Trusted    func() bool
	QuotaGuard func() string
	CancelWait func(queue, nonce string) error
	RunQueue   func(ctx context.Context, queue, worktree string) (*lease.QueueRun, error)
}

func (o *Orchestration) startGate() StartGate {
	if o == nil {
		return StartGate{}
	}
	return StartGate{Trusted: o.Trusted, QuotaGuard: o.QuotaGuard}
}

type runtimeV2 struct {
	o       Options
	cfg     *Config
	root    string
	cfgPath string
	hub     *Hub
	p       *pollers
	lanes   *LaneManager
	srv     atomic.Pointer[Server]

	trust     atomic.Bool
	trustView TrustView
	malformed atomic.Int64
	lastHook  atomic.Int64 // unix ns of the last hook the ingest accepted
	// agentsQuietNow: the agents loop is sleeping the quiet interval (hookSeen kicks it).
	agentsQuietNow atomic.Bool

	// procs caches pid start times for the queue view (PANEL-7).
	procs lease.ProcCache
	// lastPromptKick is when the first-prompt loop last kicked a `claude agents`
	// poll (promptLoop's goroutine only).
	lastPromptKick time.Time

	mu       sync.Mutex
	obs      Obs
	lastObs  Obs
	pollSum  int64
	notifier Notifier
	runs     map[string]*lease.QueueRun
	// notifyPath persists the notifier's state, so a restart never re-notifies.
	notifyPath string
	savedState string
	gitDir     string
	lastQ      string
}

func checkConfigTrust(o Options, root, cfgPath string, raw []byte) TrustView {
	hash := ConfigHash(raw)
	tv, err := CheckTrust(o.Home, root, cfgPath, hash, o.TrustConfig)
	if err != nil {
		tv = TrustView{Hash: hash, Path: signals.ResolvePath(cfgPath), Note: "cannot read the trust record: " + err.Error()}
	}
	state := "trusted"
	if !tv.Trusted {
		state = "UNTRUSTED: it changed since you trusted " + short(tv.Prev) + "; its commands and templates are off until `clauductor panel trust`"
	}
	if tv.Note != "" {
		state += " (" + tv.Note + ")"
	}
	fmt.Fprintf(o.Out, "config %s sha256 %s: %s\n", tv.Path, short(hash), state)
	return tv
}

func newRuntimeV2(o Options, cfg *Config, root, cfgPath string, tv TrustView, hub *Hub, p *pollers, lanes *LaneManager) *runtimeV2 {
	x := &runtimeV2{o: o, cfg: cfg, root: root, cfgPath: cfgPath, hub: hub, p: p, lanes: lanes, trustView: tv,
		runs: map[string]*lease.QueueRun{}}
	x.trust.Store(tv.Trusted)
	x.notifier = Notifier{MinInterval: cfg.AlertThresholds().MinInterval, Project: cfg.Name}
	x.notifyPath = filepath.Join(filepath.Dir(RegistryPath(o.Home, root)), "notifier.json")
	if b, err := os.ReadFile(x.notifyPath); err == nil {
		var st NotifierState
		if json.Unmarshal(b, &st) == nil {
			x.notifier.Restore(st)
		}
	}
	hub.Update(func(m *Model, now time.Time) { m.ApplyTrust(tv) })
	return x
}

func (x *runtimeV2) trusted() bool { return x == nil || x.trust.Load() }

func (x *runtimeV2) hookSeen(ev signals.HookEvent) {
	x.lastHook.Store(time.Now().UnixNano())
	// A hook ends the quiet interval: poll now rather than up to 15 s later (once;
	// the next interval is computed with this hook in it).
	if x.agentsQuietNow.CompareAndSwap(true, false) && x.p != nil {
		kick(x.p.kickAgents)
	}
}

func (x *runtimeV2) orchestration() *Orchestration {
	return &Orchestration{
		Trusted: x.trusted,
		QuotaGuard: func() string {
			why := ""
			x.hub.Read(func(m *Model, now time.Time) { why = m.QuotaGuard(now) })
			return why
		},
		CancelWait: x.cancelWait,
		RunQueue:   x.runQueue,
	}
}

func (x *runtimeV2) start(ctx context.Context, start func(func())) {
	start(func() { x.versionLoop(ctx) })
	start(func() { x.obsLoop(ctx) })
	start(func() { x.notifyLoop(ctx) })
	if len(x.cfg.Queues) > 0 {
		start(func() { x.queueLoop(ctx) })
	}
	if x.lanes != nil {
		start(func() { x.promptLoop(ctx) })
	}
	if !x.trustView.Trusted {
		start(func() { x.trustLoop(ctx) })
	}
}

// trustLoop lifts the untrusted mode once `clauductor panel trust` records the
// loaded config's hash.
func (x *runtimeV2) trustLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if trustedNow(x.o.Home, x.root, x.cfgPath, x.trustView.Hash) {
				x.trust.Store(true)
				tv := x.trustView
				tv.Trusted, tv.Note = true, "trusted by `clauductor panel trust`"
				x.hub.Update(func(m *Model, now time.Time) { m.ApplyTrust(tv) })
				fmt.Fprintln(x.o.Out, "config trusted: cards, queue commands and templates are on")
				for _, k := range x.p.cardKicks {
					kick(k)
				}
				return
			}
		}
	}
}

func (x *runtimeV2) setObs(f func(o *Obs)) {
	x.mu.Lock()
	f(&x.obs)
	x.mu.Unlock()
}

// lastHookAt is when the ingest last accepted a hook; zero if it never has.
func (x *runtimeV2) lastHookAt() time.Time {
	if ns := x.lastHook.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
}

// hasLanes: the panel has a lane, registered or on its socket. The registry is
// read directly: a lane start records it before it kicks this loop, while the
// model learns of it only from the next tmux poll.
func (x *runtimeV2) hasLanes() bool {
	if x.p.registry != nil && len(x.p.registry.List()) > 0 {
		return true
	}
	n := 0
	x.hub.Read(func(m *Model, _ time.Time) { n = len(m.laneRecords) + len(m.tmuxLanes) })
	return n > 0
}

// agentsLoop polls `claude agents --json`: filtered by --cwd once a cross-check with
// the unfiltered list shows the filter drops nothing in the project, at the
// interval AgentsInterval gives, and at once when kicked.
func (x *runtimeV2) agentsLoop(ctx context.Context) {
	var filter []string
	var lastCheck time.Time
	for {
		now := time.Now()
		iterStart := now
		var wts []signals.Worktree
		x.hub.Read(func(m *Model, _ time.Time) { wts = m.Worktrees() })
		if lastCheck.IsZero() || now.Sub(lastCheck) >= filterRecheck {
			lastCheck = now
			filter = x.checkFilter(ctx, wts)
		}
		argv := append([]string{"claude", "agents", "--json"}, filter...)
		t0 := time.Now()
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := x.p.run(cctx, x.root, argv)
		cancel()
		dur := time.Since(t0)
		var agents []signals.Agent
		if err == nil {
			agents, err = signals.ParseAgents(out)
		}
		if ctx.Err() != nil {
			return
		}
		for i := range agents {
			agents[i].Cwd = signals.ResolvePath(agents[i].Cwd)
			// v1: a session seen busy has a conversation, so a restart can --resume it.
			if agents[i].Status == "busy" && x.p.registry != nil {
				_, _ = x.p.registry.MarkConversation(agents[i].SessionID)
			}
		}
		iter := time.Since(iterStart) // the filter cross-check included: agentsFresh allows for it
		interval := AgentsInterval(x.lastHookAt(), time.Now(), x.hasLanes())
		x.agentsQuietNow.Store(interval == agentsQuiet)
		x.hub.Update(func(m *Model, now time.Time) {
			m.agentsNext = interval // agentsFresh allows for the wait until the next poll
			m.ApplyAgentsTimed(agents, err, iter, now)
		})
		x.mu.Lock()
		x.obs.AgentsPolls++
		x.pollSum += dur.Milliseconds()
		x.obs.AgentsPollMs = dur.Milliseconds()
		x.obs.AgentsPollAvgMs = x.pollSum / x.obs.AgentsPolls
		if dur.Milliseconds() > x.obs.AgentsPollMaxMs {
			x.obs.AgentsPollMaxMs = dur.Milliseconds()
		}
		x.obs.AgentsInterval = interval.Milliseconds()
		x.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		case <-x.p.kickAgents:
		}
	}
}

// checkFilter decides whether `--cwd <dir>` may be used: only when the filtered list
// holds every in-project session the unfiltered one does.
func (x *runtimeV2) checkFilter(ctx context.Context, wts []signals.Worktree) []string {
	dir := signals.AgentsFilterDir(x.root, wts)
	set := func(desc string) { x.setObs(func(o *Obs) { o.AgentsFilter = desc }) }
	if dir == "" {
		set("unfiltered: the worktrees share no directory but /")
		return nil
	}
	run := func(argv []string) ([]signals.Agent, error) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := x.p.run(cctx, x.root, argv)
		if err != nil {
			return nil, err
		}
		return signals.ParseAgents(out)
	}
	all, err := run([]string{"claude", "agents", "--json"})
	if err != nil {
		set("unfiltered: cannot cross-check (" + signals.Clip(err.Error(), 80) + ")")
		return nil
	}
	filtered, err := run([]string{"claude", "agents", "--json", "--cwd", dir})
	if err != nil {
		set("unfiltered: --cwd failed (" + signals.Clip(err.Error(), 80) + ")")
		return nil
	}
	var recs map[string]bool
	x.hub.Read(func(m *Model, _ time.Time) {
		recs = map[string]bool{}
		for _, r := range m.laneRecords {
			recs[r.SessionID] = true
		}
	})
	missed := signals.MissedByFilter(all, filtered, func(a signals.Agent) bool {
		return recs[a.SessionID] || signals.MatchWorktree(wts, signals.ResolvePath(a.Cwd)) >= 0
	})
	if len(missed) > 0 {
		set(fmt.Sprintf("unfiltered: --cwd %s missed %d session(s) of this project", dir, len(missed)))
		return nil
	}
	set("--cwd " + dir + " (cross-checked every " + filterRecheck.String() + ")")
	return []string{"--cwd", dir}
}

func (x *runtimeV2) versionLoop(ctx context.Context) {
	for {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := x.p.run(cctx, x.root, []string{"claude", "--version"})
		cancel()
		v := ""
		if err == nil {
			v, err = signals.ParseClaudeVersion(out)
		}
		if ctx.Err() != nil {
			return
		}
		x.hub.Update(func(m *Model, now time.Time) { m.ApplyClaudeVersion(v, err, now) })
		if err == nil && v != HeuristicsVerifiedOn {
			fmt.Fprintf(x.o.Out, "warning: Claude Code %s differs from %s, which the subagent heuristics and fixtures were verified on; subagent lists are approximate\n", v, HeuristicsVerifiedOn)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(versionRecheck):
		}
	}
}

// obsLoop copies the counters into the model, only when they change (a model
// update broadcasts to every page).
func (x *runtimeV2) obsLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		x.mu.Lock()
		x.obs.MalformedDrops = x.malformed.Load()
		if s := x.srv.Load(); s != nil {
			x.obs.OverflowDrops = s.Overflow()
		}
		o, changed := x.obs, x.obs != x.lastObs
		// Latency moves every poll; publish it with the rest, at most once a second.
		x.lastObs = x.obs
		x.mu.Unlock()
		if changed {
			x.hub.Update(func(m *Model, now time.Time) { m.ApplyObs(o) })
		}
	}
}

// notifyLoop turns alerts into OS notifications through the Notifier.
func (x *runtimeV2) notifyLoop(ctx context.Context) {
	th := x.cfg.AlertThresholds()
	send := x.o.Notify
	if send == nil {
		send = func(n Notice) error { return SendNotice(ctx, n) }
	}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var last NotifierStats
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		v := x.hub.View()
		focused := map[string]bool{}
		if s := x.srv.Load(); s != nil {
			focused = s.FocusedLanes()
		}
		x.mu.Lock()
		// The project name comes from panel.json, so it is config like any other: an
		// untrusted config does not get to title OS notifications.
		x.notifier.Project = "clauductor panel"
		if x.trusted() {
			x.notifier.Project = x.cfg.Name
		}
		notices := x.notifier.Process(v.Alerts, focused, time.Now())
		stats := x.notifier.Stats()
		st, _ := json.Marshal(x.notifier.State())
		x.mu.Unlock()
		if string(st) != x.savedState {
			// Saved before sending: a crash mid-send loses one notification rather
			// than repeating it at every restart.
			if err := ensurePrivateDir(filepath.Dir(x.notifyPath)); err == nil && writeAtomic(x.notifyPath, st, 0o600) == nil {
				x.savedState = string(st)
			}
		}
		for _, n := range notices {
			if !th.Notify {
				continue
			}
			err := send(n)
			x.setObs(func(o *Obs) {
				if err != nil {
					o.NotifyFailed++
					o.NotifyError = signals.Clip(err.Error(), 160)
				} else {
					o.NotifySent++
				}
			})
			if err != nil {
				fmt.Fprintf(x.o.Out, "notification failed: %v\n", err)
			}
		}
		if stats != last {
			last = stats
			x.hub.Update(func(m *Model, now time.Time) { m.ApplyNotifier(stats) })
		}
	}
}

// promptLoop types each template lane's first prompt once claude is ready, and
// records whether it landed. Every decision comes from DecideFirstPrompt.
func (x *runtimeV2) promptLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		x.promptTick(ctx, time.Now())
	}
}

// promptTick acts on every template lane's first-prompt decision once. A lane that
// waits on a `claude agents` reading kicks a poll, at most every promptKickEvery; a
// lane that waits on anything else (its tmux session is gone) kicks nothing.
func (x *runtimeV2) promptTick(ctx context.Context, clock time.Time) {
	var ds map[string]PromptDecision
	x.hub.Read(func(m *Model, now time.Time) { ds = m.PromptDecisions(now) })
	changed, poll := false, false
	for id, d := range ds {
		switch d.Action {
		case "send":
			stillReady := func() string {
				var d PromptDecision
				x.hub.Read(func(m *Model, now time.Time) { d = m.PromptDecisions(now)[id] })
				if d.Action != "send" {
					return "no longer ready (" + d.Why + ")"
				}
				return ""
			}
			if err := x.lanes.DeliverFirstPrompt(ctx, id, stillReady); err != nil {
				fmt.Fprintf(x.o.Out, "lane %s: first prompt: %v\n", id, err)
			}
			changed = true
		case "skip":
			_ = x.lanes.SetPromptState(id, "pending", "skipped")
			changed = true
		case "delivered":
			_ = x.lanes.SetPromptState(id, "sent", "delivered")
			changed = true
		case "wait":
			// Readiness is read from `claude agents`; a wait on anything else (the
			// lane's tmux session is not there) is not helped by a poll.
			poll = poll || d.Poll
		}
	}
	switch {
	case changed:
		// Once per transition: see its effect at once.
		kick(x.p.kickTmux)
		kick(x.p.kickAgents)
		x.lastPromptKick = clock
	case poll && clock.Sub(x.lastPromptKick) >= promptKickEvery:
		kick(x.p.kickAgents)
		x.lastPromptKick = clock
	}
}

// gitCommonDir is where queue locks live: shared by every worktree of the project.
func (x *runtimeV2) gitCommonDir(ctx context.Context) (string, error) {
	x.mu.Lock()
	d := x.gitDir
	x.mu.Unlock()
	if d != "" {
		return d, nil
	}
	out, err := x.p.run(ctx, x.root, []string{"git", "rev-parse", "--git-common-dir"})
	if err != nil {
		return "", err
	}
	d = strings.TrimSpace(string(out))
	if !filepath.IsAbs(d) {
		d = filepath.Join(x.root, d)
	}
	d = signals.ResolvePath(d)
	x.mu.Lock()
	x.gitDir = d
	x.mu.Unlock()
	return d, nil
}

func (x *runtimeV2) queueLock(ctx context.Context, id string) (lease.QueueConfig, string, error) {
	for _, q := range x.cfg.Queues {
		if q.ID == id {
			d, err := x.gitCommonDir(ctx)
			if err != nil {
				return q, "", err
			}
			return q, filepath.Join(d, filepath.Clean(q.Lock)), nil
		}
	}
	return lease.QueueConfig{}, "", fmt.Errorf("no queue %q", id)
}

// queueLoop reads every queue's lease once a second. It only reads.
func (x *runtimeV2) queueLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		qs, err := x.readQueues(ctx, time.Now())
		b, _ := json.Marshal(qs)
		sig := string(b)
		if err != nil {
			sig = err.Error()
		}
		if sig != x.lastQ {
			x.lastQ = sig
			x.hub.Update(func(m *Model, now time.Time) { m.ApplyQueues(qs, err, now) })
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// readQueues reads every queue once. Liveness goes through the pid cache: kill(pid,
// 0) every time, ps once per process (PANEL-7).
func (x *runtimeV2) readQueues(ctx context.Context, now time.Time) ([]lease.QueueView, error) {
	var qs []lease.QueueView
	for _, q := range x.cfg.Queues {
		_, lock, err := x.queueLock(ctx, q.ID)
		if err != nil {
			return qs, err
		}
		v := lease.ReadQueue(q, lock, now, x.procs.Check)
		x.mu.Lock()
		if r := x.runs[q.ID]; r != nil {
			rc := *r
			v.Run = &rc
		}
		x.mu.Unlock()
		qs = append(qs, v)
	}
	return qs, nil
}

func (x *runtimeV2) cancelWait(queue, nonce string) error {
	_, lock, err := x.queueLock(context.Background(), queue)
	if err != nil {
		return err
	}
	return lease.CancelWait(lock, nonce)
}

// runQueue starts the queue's command through lock-run in one of the project's
// worktrees, detached, with its output in a log file. It waits its turn like any
// other gate run; the panel never skips the queue.
func (x *runtimeV2) runQueue(ctx context.Context, queue, worktree string) (*lease.QueueRun, error) {
	if !x.trusted() {
		return nil, errUntrusted
	}
	q, lock, err := x.queueLock(ctx, queue)
	if err != nil {
		return nil, err
	}
	if len(q.Command) == 0 {
		return nil, fmt.Errorf("queue %q has no command", queue)
	}
	wts, err := signals.ReadWorktrees(ctx, x.p.run, x.root)
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
	x.mu.Lock()
	if r := x.runs[queue]; r != nil && r.Exit == nil {
		x.mu.Unlock()
		return nil, fmt.Errorf("a RUN of %s started by the panel is still going (pid %d)", queue, r.PID)
	}
	x.mu.Unlock()
	lane := "panel"
	var terms []TermLaneView
	x.hub.Read(func(m *Model, now time.Time) { terms = m.terminalViews(now) })
	for _, t := range terms {
		if t.Running && t.Worktree == dir {
			lane = t.ID
		}
	}
	argv := x.o.LockRunArgv
	if len(argv) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		argv = []string{exe, "lock-run"}
	}
	argv = append(append(append([]string{}, argv...), "--lane", lane, lock, "--"), q.Command...)
	logDir := filepath.Join(filepath.Dir(RegistryPath(x.o.Home, x.root)), "queue-logs")
	if err := ensurePrivateDir(logDir); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", queue, time.Now().Format("20060102-150405")))
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
	run := &lease.QueueRun{PID: cmd.Process.Pid, Worktree: dir, Log: logPath, Started: time.Now().UnixMilli()}
	x.mu.Lock()
	x.runs[queue] = run
	x.mu.Unlock()
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
		x.mu.Lock()
		run.Ended, run.Exit = time.Now().UnixMilli(), &code
		x.mu.Unlock()
	}()
	x.mu.Lock()
	rc := *run
	x.mu.Unlock()
	return &rc, nil
}
