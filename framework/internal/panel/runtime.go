package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// errUntrusted is what a card shows while the config is untrusted.
var errUntrusted = errors.New("not run: panel.json changed since you trusted it; run `clauductor panel trust`")

// Default cadences (Ticks overrides each) and the windows the agents cadence reads.
const (
	agentsFast   = 2 * time.Second
	hooksFlowing = 30 * time.Second
	// agentsQuiet is the interval while the panel has no lane and has heard no hook
	// for hooksQuiet (PANEL-7): nothing is happening that a hook would not announce.
	agentsQuiet    = 15 * time.Second
	hooksQuiet     = 5 * time.Minute
	filterRecheck  = 5 * time.Minute
	versionRecheck = 10 * time.Minute
	// promptKickEvery is the fastest the first-prompt source drives `claude agents`
	// polls: the fast interval, never faster.
	promptKickEvery = agentsFast

	// tmux poll cadence (PANEL-7). Every tmux call is a process spawn, and the panel
	// is meant to idle for days.
	// tmuxFast is the list-panes interval while the socket has lanes.
	tmuxFast = 2 * time.Second
	// tmuxIdle is the interval while it has none: a lane the panel starts or
	// restores kicks the source at once, so nothing waits for this.
	tmuxIdle = 10 * time.Second
	// tmuxRecheck is how often show-environment (an API key in the server's
	// environment) and Harden run while the lane set stays the same. A lane start
	// checks the environment itself, and every viewer hardens before it attaches.
	tmuxRecheck = 30 * time.Second
	// registryReload is how often the registry is re-read from disk.
	registryReload = 30 * time.Second
)

// AgentsInterval is how long to wait before the next `claude agents` poll at the
// default ticks: slow while hooks are flowing; quiet with no lane (registered or on
// the socket) and no hook for hooksQuiet; fast otherwise. A zero lastHook is no hook
// heard.
func AgentsInterval(lastHook, now time.Time, lanes bool) time.Duration {
	return DefaultTicks().agentsInterval(lastHook, now, lanes)
}

func (t Ticks) agentsInterval(lastHook, now time.Time, lanes bool) time.Duration {
	heard := !lastHook.IsZero()
	switch {
	case heard && now.Sub(lastHook) < hooksFlowing:
		return t.AgentsSlow
	case !lanes && (!heard || now.Sub(lastHook) >= hooksQuiet):
		return t.AgentsQuiet
	}
	return t.AgentsFast
}

// update is a change to the model, applied under the hub's lock.
type update = func(m *state.Model, now time.Time)

// A source is one thing the runtime reads, or does, on a cadence. Its poll returns
// the model update (nil: none) and how long to wait before the next poll (0: every;
// < 0: never again). Every source runs the same loop (Runtime.loop): poll, drop the
// result if the panel is stopping, apply it, then wait for the next tick, a kick or
// a change in what it watches.
type source struct {
	name string
	poll func(ctx context.Context, now time.Time) (update, time.Duration)
	// every is the wait between polls; 0 polls only when kicked or watched.
	every time.Duration
	// fixedRate counts every from the start, as a ticker does, rather than from the
	// end of each poll.
	fixedRate bool
	// waitFirst waits one every before the first poll.
	waitFirst bool
	// kick, when set, polls at once; refreshAll kicks every source that has one.
	kick chan struct{}
	// watch, when set, names a path (it may spawn to find it) whose signature is
	// compared every watchEvery; a change kicks the source.
	watch      func(ctx context.Context) string
	watchEvery time.Duration
}

// fetch reads one source's value.
type fetch[T any] func(ctx context.Context) (T, error)

// commandFetch runs argv in the project root through the Runner, with a timeout,
// and parses its stdout: the shape every command source shares.
func commandFetch[T any](r *Runtime, timeout time.Duration, argv []string, parse func([]byte) (T, error)) fetch[T] {
	return func(ctx context.Context) (T, error) {
		out, err := r.exec(ctx, timeout, argv)
		if err != nil {
			var zero T
			return zero, err
		}
		return parse(out)
	}
}

// polled feeds a fetch's result (value or error) to the model.
func polled[T any](f fetch[T], apply func(*state.Model, T, error, time.Time)) func(context.Context, time.Time) (update, time.Duration) {
	return func(ctx context.Context, _ time.Time) (update, time.Duration) {
		v, err := f(ctx)
		return func(m *state.Model, now time.Time) { apply(m, v, err, now) }, 0
	}
}

// Runtime reads every source into the hub on its cadence and runs the panel's
// periodic tasks. It is the one owner of the panel's clock, ticks and kicks.
type Runtime struct {
	o        Options
	cfg      *config.Config
	root     string
	cfgPath  string
	clock    clock.Clock
	ticks    Ticks
	run      signals.Runner
	hub      *web.Hub
	lanes    *lanes.LaneManager // nil when lanes are unavailable
	registry *lanes.Registry    // the lanes' registry, nil with them
	srv      atomic.Pointer[web.Server]
	sources  []*source

	kickWT, kickAgents, kickPRs, kickTmux chan struct{}
	cardKicks                             []chan struct{}
	kickMu                                sync.Mutex
	lastWTKick                            time.Time

	trust     atomic.Bool
	trustView config.TrustView
	malformed atomic.Int64
	lastHook  atomic.Int64 // unix ns of the last hook the ingest accepted
	// agentsQuietNow: the agents source is waiting the quiet interval (hookSeen kicks it).
	agentsQuietNow atomic.Bool
	// agentsFilter is the --cwd filter and when it was last cross-checked (the
	// agents source's goroutine only).
	agentsFilter      []string
	agentsFilterCheck time.Time

	// procs caches pid start times for the queue view (PANEL-7).
	procs lease.ProcCache
	// lastPromptKick is when the first-prompt source last kicked a `claude agents`
	// poll (its goroutine only).
	lastPromptKick time.Time

	mu       sync.Mutex
	obs      state.Obs
	lastObs  state.Obs
	pollSum  int64
	notifier state.Notifier
	notified state.NotifierStats // the notifier's counters last put in the model
	runs     map[string]*types.QueueRun
	// notifyPath persists the notifier's state, so a restart never re-notifies.
	notifyPath string
	savedState string
	gitDir     string
	lastQ      string
}

// newRuntime builds the runtime and its table of sources.
func newRuntime(o Options, cfg *config.Config, root, cfgPath string, tv config.TrustView, hub *web.Hub,
	lm *lanes.LaneManager, lanesWhy string, clk clock.Clock, ticks Ticks) *Runtime {
	r := &Runtime{o: o, cfg: cfg, root: root, cfgPath: cfgPath, clock: clk, ticks: ticks, run: o.Runner, hub: hub, lanes: lm,
		trustView: tv, runs: map[string]*types.QueueRun{},
		kickWT: make(chan struct{}, 1), kickAgents: make(chan struct{}, 1), kickPRs: make(chan struct{}, 1), kickTmux: make(chan struct{}, 1)}
	if lm != nil {
		r.registry = lm.Registry // set before the ingest starts reading it
	}
	r.procs.Now = clk.Now
	r.trust.Store(tv.Trusted)
	r.notifier = state.Notifier{MinInterval: cfg.AlertThresholds().MinInterval, Project: cfg.Name}
	r.notifyPath = filepath.Join(config.ProjectDir(o.Home, root), "notifier.json")
	if b, err := os.ReadFile(r.notifyPath); err == nil {
		var st state.NotifierState
		if json.Unmarshal(b, &st) == nil {
			r.notifier.Restore(st)
		}
	}
	hub.Update(func(m *state.Model, now time.Time) { m.ApplyTrust(tv) })

	t := ticks
	worktrees := func(ctx context.Context) ([]signals.Worktree, error) {
		return signals.ReadWorktrees(ctx, r.run, r.root)
	}
	prs := commandFetch(r, 30*time.Second, []string{"gh", "pr", "list", "--json", "number,title,headRefName,author,isDraft,statusCheckRollup,reviewDecision"},
		signals.ParsePRs)
	// The table of sources. Each runs in its own goroutine, through Runtime.loop.
	r.sources = []*source{
		// Re-read within a watch tick of git's own worktree registry changing (a
		// worktree added or removed), and early for a cwd the list does not know.
		{name: "worktrees", every: t.Worktrees, fixedRate: true, kick: r.kickWT, watch: r.worktreeRegistryDir, watchEvery: t.WorktreeWatch,
			poll: polled(worktrees, (*state.Model).ApplyWorktrees)},
		r.agentsSource(),
		{name: "prs", every: t.PRs, fixedRate: true, kick: r.kickPRs, poll: polled(prs, (*state.Model).ApplyPRs)},
		{name: "tmux", every: t.TmuxIdle, kick: r.kickTmux, poll: newTmuxPoller(lm, lanesWhy, clk, t).poll},
		{name: "version", every: t.Version, poll: r.pollVersion()},
		{name: "obs", every: t.Obs, fixedRate: true, waitFirst: true, poll: r.pollObs},
		{name: "notify", every: t.Notify, fixedRate: true, waitFirst: true, poll: r.pollNotify()},
		{name: "trends", every: t.Trends, fixedRate: true, poll: func(context.Context, time.Time) (update, time.Duration) {
			return func(m *state.Model, now time.Time) { m.Sample(now) }, 0
		}},
		{name: "procs", every: t.Procs, fixedRate: true, waitFirst: true, poll: r.pollProcs},
		{name: "git", every: t.Git, fixedRate: true, poll: r.pollGit},
	}
	for _, c := range cfg.Cards {
		r.sources = append(r.sources, r.cardSource(c))
	}
	if len(cfg.Queues) > 0 {
		r.sources = append(r.sources, &source{name: "queues", every: t.Queues, fixedRate: true, poll: r.pollQueues})
	}
	if lm != nil {
		r.sources = append(r.sources, &source{name: "prompts", every: t.Prompt, fixedRate: true, waitFirst: true,
			poll: func(ctx context.Context, now time.Time) (update, time.Duration) {
				r.promptTick(ctx, now)
				return nil, 0
			}})
	}
	if !tv.Trusted {
		r.sources = append(r.sources, &source{name: "trust", every: t.Trust, fixedRate: true, waitFirst: true, poll: r.pollTrust})
	}
	if o.Launchd {
		// `clauductor panel rotate-token` (or a reinstall) replaces the token file;
		// follow it so the old token dies in the running panel too.
		r.sources = append(r.sources, &source{name: "token", every: t.Token, fixedRate: true, waitFirst: true, poll: r.pollToken})
	}
	return r
}

// addHookKeeper adds the hooks check: it ran once at start and reported ok; it
// runs again every interval, or sooner with backoff after a failure.
func (r *Runtime) addHookKeeper(k *hookKeeper, ok bool) {
	r.sources = append(r.sources, &source{name: "hooks", every: k.nextWait(ok), waitFirst: true,
		poll: func(context.Context, time.Time) (update, time.Duration) { return nil, k.nextWait(k.check()) }})
}

// start runs every source in its own goroutine.
func (r *Runtime) start(ctx context.Context, start func(func())) {
	for _, s := range r.sources {
		s := s
		start(func() { r.loop(ctx, s) })
	}
}

// loop runs one source until ctx ends.
func (r *Runtime) loop(ctx context.Context, s *source) {
	if s.watch != nil {
		path := s.watch(ctx)
		go r.watchLoop(ctx, path, s)
	}
	var ticks <-chan time.Time
	if s.fixedRate && s.every > 0 {
		t := r.clock.NewTicker(s.every)
		defer t.Stop()
		ticks = t.C()
	}
	next, ok := s.every, true
	if !s.waitFirst {
		next, ok = r.pollOnce(ctx, s)
	}
	for ok && next >= 0 {
		var wait <-chan time.Time
		var timer clock.Timer
		switch {
		case s.fixedRate:
			wait = ticks
		case next > 0:
			timer = r.clock.NewTimer(next)
			wait = timer.C()
		}
		select {
		case <-ctx.Done():
		case <-wait:
		case <-s.kick: // a nil kick never fires
		}
		if timer != nil {
			timer.Stop()
		}
		if ctx.Err() != nil {
			return
		}
		next, ok = r.pollOnce(ctx, s)
	}
}

// pollOnce polls s and applies its update, unless the panel is stopping. It
// returns the wait before the next poll, and false once the panel is stopping.
func (r *Runtime) pollOnce(ctx context.Context, s *source) (time.Duration, bool) {
	up, next := s.poll(ctx, r.clock.Now())
	if ctx.Err() != nil {
		return 0, false
	}
	if up != nil {
		r.hub.Update(up)
	}
	if r.o.OnPoll != nil {
		r.o.OnPoll(s.name)
	}
	if next == 0 {
		next = s.every
	}
	return next, true
}

// watchLoop kicks s whenever path's signature changes.
func (r *Runtime) watchLoop(ctx context.Context, path string, s *source) {
	last := pathSignature(path)
	t := r.clock.NewTicker(s.watchEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
			if sig := pathSignature(path); sig != last {
				last = sig
				kick(s.kick)
			}
		}
	}
}

// exec runs argv in the project root through the Runner, with a timeout.
func (r *Runtime) exec(ctx context.Context, timeout time.Duration, argv []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.run(cctx, r.root, argv)
}

func kick(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// refreshAll polls every kickable source now (the page's refresh).
func (r *Runtime) refreshAll() {
	for _, s := range r.sources {
		if s.kick != nil {
			kick(s.kick)
		}
	}
}

// kickWorktrees re-reads the worktree authority early (rate-limited), e.g. when an
// event arrives from a cwd the current list does not know — likely a new worktree.
func (r *Runtime) kickWorktrees() {
	r.kickMu.Lock()
	defer r.kickMu.Unlock()
	now := r.clock.Now()
	if now.Sub(r.lastWTKick) < r.ticks.WorktreeKick {
		return
	}
	r.lastWTKick = now
	kick(r.kickWT)
}

// ---- sources ----

// worktreeRegistryDir is git's own worktree registry, <git common dir>/worktrees:
// it changes when a worktree is added or removed.
func (r *Runtime) worktreeRegistryDir(ctx context.Context) string {
	out, err := r.run(ctx, r.root, []string{"git", "rev-parse", "--git-common-dir"})
	if err != nil {
		return ""
	}
	d := strings.TrimSpace(string(out))
	if !filepath.IsAbs(d) {
		d = filepath.Join(r.root, d)
	}
	return filepath.Join(d, "worktrees")
}

// cardSource runs a card's command on its refresh rule: every interval, or when
// its watched path changes. An untrusted config runs no card.
func (r *Runtime) cardSource(c config.CardConfig) *source {
	rule, _ := config.ParseRefresh(c.Refresh) // validated at load
	run := polled(commandFetch(r, 30*time.Second, c.Command, func(out []byte) (*signals.CardOutput, error) {
		co := signals.ParseCardOutput(out)
		return &co, nil
	}), func(m *state.Model, co *signals.CardOutput, err error, now time.Time) {
		m.ApplyCard(c.ID, co, err, now)
	})
	s := &source{name: "card " + c.ID, kick: make(chan struct{}, 1),
		poll: func(ctx context.Context, now time.Time) (update, time.Duration) {
			if !r.trusted() {
				return func(m *state.Model, now time.Time) { m.ApplyCard(c.ID, nil, errUntrusted, now) }, 0
			}
			return run(ctx, now)
		}}
	r.cardKicks = append(r.cardKicks, s.kick)
	if rule.Interval > 0 {
		s.every, s.fixedRate = rule.Interval, true
	} else {
		s.watch = func(context.Context) string { return filepath.Join(r.root, rule.WatchRel) }
		s.watchEvery = r.ticks.CardWatch
	}
	return s
}

// pollVersion reads `claude --version`, and warns once per read when it is not the
// version the heuristics were verified on.
func (r *Runtime) pollVersion() func(context.Context, time.Time) (update, time.Duration) {
	read := commandFetch(r, 10*time.Second, []string{"claude", "--version"}, signals.ParseClaudeVersion)
	return func(ctx context.Context, _ time.Time) (update, time.Duration) {
		v, err := read(ctx)
		if ctx.Err() == nil && err == nil && v != state.HeuristicsVerifiedOn {
			fmt.Fprintf(r.o.Out, "warning: Claude Code %s differs from %s, which the subagent heuristics and fixtures were verified on; subagent lists are approximate\n", v, state.HeuristicsVerifiedOn)
		}
		return func(m *state.Model, now time.Time) { m.ApplyClaudeVersion(v, err, now) }, 0
	}
}

// agentsSource polls `claude agents` (pollAgents).
func (r *Runtime) agentsSource() *source {
	return &source{name: "agents", every: r.ticks.AgentsFast, kick: r.kickAgents, poll: r.pollAgents}
}

// pollAgents reads `claude agents --json`: filtered by --cwd once a cross-check with
// the unfiltered list shows the filter drops nothing in the project, and waits the
// interval the ticks' cadence gives (kicked at once by a hook in a quiet stretch).
func (r *Runtime) pollAgents(ctx context.Context, now time.Time) (update, time.Duration) {
	iterStart := now
	var wts []signals.Worktree
	r.hub.Read(func(m *state.Model, _ time.Time) { wts = m.Worktrees() })
	if r.agentsFilterCheck.IsZero() || now.Sub(r.agentsFilterCheck) >= r.ticks.AgentsFilter {
		r.agentsFilterCheck = now
		r.agentsFilter = r.checkFilter(ctx, wts)
	}
	argv := append([]string{"claude", "agents", "--json"}, r.agentsFilter...)
	t0 := r.clock.Now()
	out, err := r.exec(ctx, 10*time.Second, argv)
	dur := r.clock.Now().Sub(t0)
	var agents []signals.Agent
	if err == nil {
		agents, err = signals.ParseAgents(out)
	}
	if ctx.Err() != nil {
		return nil, 0
	}
	for i := range agents {
		agents[i].Cwd = signals.ResolvePath(agents[i].Cwd)
		// A session seen busy has a conversation, so a restart can --resume it.
		if agents[i].Status == "busy" && r.registry != nil {
			_, _ = r.registry.MarkConversation(agents[i].SessionID)
		}
	}
	end := r.clock.Now()
	iter := end.Sub(iterStart) // the filter cross-check included: agentsFresh allows for it
	interval := r.ticks.agentsInterval(r.lastHookAt(), end, r.hasLanes())
	r.agentsQuietNow.Store(interval == r.ticks.AgentsQuiet)
	r.mu.Lock()
	r.obs.AgentsPolls++
	r.pollSum += dur.Milliseconds()
	r.obs.AgentsPollMs = dur.Milliseconds()
	r.obs.AgentsPollAvgMs = r.pollSum / r.obs.AgentsPolls
	if dur.Milliseconds() > r.obs.AgentsPollMaxMs {
		r.obs.AgentsPollMaxMs = dur.Milliseconds()
	}
	r.obs.AgentsInterval = interval.Milliseconds()
	r.mu.Unlock()
	slow := r.ticks.AgentsSlow
	return func(m *state.Model, now time.Time) {
		m.SetAgentsCadence(interval, slow) // agentsFresh allows for the wait until the next poll
		m.ApplyAgentsTimed(agents, err, iter, now)
	}, interval
}

// checkFilter decides whether `--cwd <dir>` may be used: only when the filtered list
// holds every in-project session the unfiltered one does.
func (r *Runtime) checkFilter(ctx context.Context, wts []signals.Worktree) []string {
	dir := signals.AgentsFilterDir(r.root, wts)
	set := func(desc string) { r.setObs(func(o *state.Obs) { o.AgentsFilter = desc }) }
	if dir == "" {
		set("unfiltered: the worktrees share no directory but /")
		return nil
	}
	list := commandFetch(r, 10*time.Second, []string{"claude", "agents", "--json"}, signals.ParseAgents)
	all, err := list(ctx)
	if err != nil {
		set("unfiltered: cannot cross-check (" + signals.Clip(err.Error(), 80) + ")")
		return nil
	}
	filtered, err := commandFetch(r, 10*time.Second, []string{"claude", "agents", "--json", "--cwd", dir}, signals.ParseAgents)(ctx)
	if err != nil {
		set("unfiltered: --cwd failed (" + signals.Clip(err.Error(), 80) + ")")
		return nil
	}
	var recs map[string]bool
	r.hub.Read(func(m *state.Model, _ time.Time) { recs = m.LaneSessions() })
	missed := signals.MissedByFilter(all, filtered, func(a signals.Agent) bool {
		return recs[a.SessionID] || signals.MatchWorktree(wts, signals.ResolvePath(a.Cwd)) >= 0
	})
	if len(missed) > 0 {
		set(fmt.Sprintf("unfiltered: --cwd %s missed %d session(s) of this project", dir, len(missed)))
		return nil
	}
	set("--cwd " + dir + " (cross-checked every " + r.ticks.AgentsFilter.String() + ")")
	return []string{"--cwd", dir}
}

// pollObs copies the counters into the model, only when they change (a model
// update is pushed to every page).
func (r *Runtime) pollObs(context.Context, time.Time) (update, time.Duration) {
	r.mu.Lock()
	r.obs.MalformedDrops = r.malformed.Load()
	if s := r.srv.Load(); s != nil {
		r.obs.OverflowDrops = s.Overflow()
	}
	o, changed := r.obs, r.obs != r.lastObs
	// Latency moves every poll; publish it with the rest, at most once a tick.
	r.lastObs = r.obs
	r.mu.Unlock()
	if !changed {
		return nil, 0
	}
	return func(m *state.Model, now time.Time) { m.ApplyObs(o) }, 0
}

// pollNotify turns alerts into OS notifications through the Notifier, on the
// panel's clock: the view it reads and the notifier's `now` are the same time.
func (r *Runtime) pollNotify() func(context.Context, time.Time) (update, time.Duration) {
	th := r.cfg.AlertThresholds()
	return func(ctx context.Context, now time.Time) (update, time.Duration) {
		send := r.o.Notify
		if send == nil {
			send = func(n state.Notice) error { return SendNotice(ctx, n) }
		}
		v := r.hub.View()
		focused := map[string]bool{}
		if s := r.srv.Load(); s != nil {
			focused = s.FocusedLanes()
		}
		r.mu.Lock()
		// The project name comes from panel.json, so it is config like any other: an
		// untrusted config does not get to title OS notifications.
		r.notifier.Project = "clauductor panel"
		if r.trusted() {
			r.notifier.Project = r.cfg.Name
		}
		notices := r.notifier.Process(v.Alerts, focused, now)
		stats := r.notifier.Stats()
		st, _ := json.Marshal(r.notifier.State())
		r.mu.Unlock()
		if string(st) != r.savedState {
			// Saved before sending: a crash mid-send loses one notification rather
			// than repeating it at every restart.
			if err := config.EnsurePrivateDir(filepath.Dir(r.notifyPath)); err == nil && config.WriteAtomic(r.notifyPath, st, 0o600) == nil {
				r.savedState = string(st)
			}
		}
		for _, n := range notices {
			if !th.Notify {
				continue
			}
			err := send(n)
			r.setObs(func(o *state.Obs) {
				if err != nil {
					o.NotifyFailed++
					o.NotifyError = signals.Clip(err.Error(), 160)
				} else {
					o.NotifySent++
				}
			})
			if err != nil {
				fmt.Fprintf(r.o.Out, "notification failed: %v\n", err)
			}
		}
		if stats == r.notified {
			return nil, 0
		}
		r.notified = stats
		return func(m *state.Model, now time.Time) { m.ApplyNotifier(stats) }, 0
	}
}

// pollTrust lifts the untrusted mode once `clauductor panel trust` records the
// loaded config's hash, then stops.
func (r *Runtime) pollTrust(context.Context, time.Time) (update, time.Duration) {
	if !install.TrustedNow(r.o.Home, r.root, r.cfgPath, r.trustView.Hash) {
		return nil, 0
	}
	r.trust.Store(true)
	tv := r.trustView
	tv.Trusted, tv.Note = true, "trusted by `clauductor panel trust`"
	fmt.Fprintln(r.o.Out, "config trusted: cards, queue commands and templates are on")
	for _, k := range r.cardKicks {
		kick(k)
	}
	return func(m *state.Model, now time.Time) { m.ApplyTrust(tv) }, -1
}

// pollToken follows the token file: a rotated token closes every cookie, terminal
// and event stream of the old one.
func (r *Runtime) pollToken(context.Context, time.Time) (update, time.Duration) {
	srv := r.srv.Load()
	if srv == nil {
		return nil, 0
	}
	if tok := install.ReadToken(r.o.Home); tok != "" && tok != srv.CurrentToken() {
		srv.Rotate(tok)
		fmt.Fprintln(r.o.Out, "token rotated: old cookies, terminals and event streams are closed")
	}
	return nil, 0
}

// ---- the ingest ----

// ingest folds hook and status-line bodies into the model as they arrive.
func (r *Runtime) ingest(ctx context.Context, hooks, status <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case body := <-hooks:
			ev, err := signals.ParseHook(body)
			if err != nil {
				r.malformed.Add(1)
				continue
			}
			r.hookSeen(ev)
			ev.Cwd = signals.ResolvePath(ev.Cwd)
			// A prompt, or a finished turn, means the session has a conversation to
			// --resume. Hooks can be dropped, so busy in `claude agents` counts too.
			if (ev.Event == "UserPromptSubmit" || ev.Event == "Stop") && r.registry != nil {
				_, _ = r.registry.MarkConversation(ev.SessionID)
			}
			kept := true
			r.hub.Update(func(m *state.Model, now time.Time) { kept = m.ApplyHook(ev, now) })
			if !kept {
				r.kickWorktrees()
			}
		case body := <-status:
			st, err := signals.ParseStatus(body)
			if err != nil {
				r.malformed.Add(1)
				continue
			}
			st.Cwd = signals.ResolvePath(st.Cwd)
			r.hub.Update(func(m *state.Model, now time.Time) { m.ApplyStatus(st, now) })
		}
	}
}

func (r *Runtime) hookSeen(ev signals.HookEvent) {
	r.lastHook.Store(r.clock.Now().UnixNano())
	// A hook ends the quiet interval: poll now rather than up to 15 s later (once;
	// the next interval is computed with this hook in it).
	if r.agentsQuietNow.CompareAndSwap(true, false) {
		kick(r.kickAgents)
	}
}

// lastHookAt is when the ingest last accepted a hook; zero if it never has.
func (r *Runtime) lastHookAt() time.Time {
	if ns := r.lastHook.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
}

// hasLanes: the panel has a lane, registered or on its socket. The registry is
// read directly: a lane start records it before it kicks the agents source, while
// the model learns of it only from the next tmux poll.
func (r *Runtime) hasLanes() bool {
	if r.registry != nil && len(r.registry.List()) > 0 {
		return true
	}
	n := 0
	r.hub.Read(func(m *state.Model, _ time.Time) { n = m.LaneCount() })
	return n > 0
}

func (r *Runtime) trusted() bool { return r.trust.Load() }

func (r *Runtime) setObs(f func(o *state.Obs)) {
	r.mu.Lock()
	f(&r.obs)
	r.mu.Unlock()
}

// ---- first prompts ----

// promptTick acts on every template lane's first-prompt decision once. A lane that
// waits on a `claude agents` reading kicks a poll, at most every PromptKick; a lane
// that waits on anything else (its tmux session is gone) kicks nothing.
func (r *Runtime) promptTick(ctx context.Context, clock time.Time) {
	var ds map[string]state.PromptDecision
	r.hub.Read(func(m *state.Model, now time.Time) { ds = m.PromptDecisions(now) })
	changed, poll := false, false
	for id, d := range ds {
		switch d.Action {
		case "send":
			stillReady := func() string {
				var d state.PromptDecision
				r.hub.Read(func(m *state.Model, now time.Time) { d = m.PromptDecisions(now)[id] })
				if d.Action != "send" {
					return "no longer ready (" + d.Why + ")"
				}
				return ""
			}
			if err := r.lanes.DeliverFirstPrompt(ctx, id, stillReady); err != nil {
				fmt.Fprintf(r.o.Out, "lane %s: first prompt: %v\n", id, err)
			}
			changed = true
		case "skip":
			_ = r.lanes.SetPromptState(id, "pending", "skipped")
			changed = true
		case "delivered":
			_ = r.lanes.SetPromptState(id, "sent", "delivered")
			changed = true
		case "wait":
			// Readiness is read from `claude agents`; a wait on anything else (the
			// lane's tmux session is not there) is not helped by a poll.
			poll = poll || d.Poll
		}
	}
	every := r.ticks.PromptKick
	if every <= 0 {
		every = promptKickEvery
	}
	switch {
	case changed:
		// Once per transition: see its effect at once.
		kick(r.kickTmux)
		kick(r.kickAgents)
		r.lastPromptKick = clock
	case poll && clock.Sub(r.lastPromptKick) >= every:
		kick(r.kickAgents)
		r.lastPromptKick = clock
	}
}

// ---- tmux ----

// tmuxPoller reconciles the lane registry with the socket, one poll at a time: at
// start (so lanes that outlived a panel restart reappear, and lanes a reboot killed
// show as orphans), every TmuxFast while the socket has lanes (TmuxIdle while it
// has none), and right after a lane command. Every RegistryLoad it also re-reads the
// registry file from disk rather than trusting its in-memory copy. The reducer
// matches the result against claude agents (by session id) and the worktree list.
type tmuxPoller struct {
	lanes *lanes.LaneManager
	why   string // lanes are unavailable (from newLaneManager)
	clock clock.Clock
	ticks Ticks

	lastReload time.Time
	lastCheck  time.Time // the last show-environment / Harden
	checkedSig string    // the lane set they ran against
	tmuxWhy    string    // what show-environment said then
}

func newTmuxPoller(lm *lanes.LaneManager, why string, clk clock.Clock, ticks Ticks) *tmuxPoller {
	return &tmuxPoller{lanes: lm, why: why, clock: clk, ticks: ticks.withDefaults(), lastReload: clk.Now()}
}

// laneSetSig names the lane set: whether the socket has a server, and each lane.
func laneSetSig(up bool, ls []types.TmuxLane) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%t", up)
	for _, l := range ls {
		b.WriteString("|" + l.ID)
	}
	return b.String()
}

func (t *tmuxPoller) poll(ctx context.Context, _ time.Time) (update, time.Duration) {
	return t.tick(ctx)
}

// tick polls once, and returns the model update and how long to wait before the
// next tick. list-panes (one call covers every lane) runs every tick;
// show-environment and Harden only when the lane set changed or TmuxRecheck
// passed, and never without a server: no server has no environment to check.
func (t *tmuxPoller) tick(ctx context.Context) (update, time.Duration) {
	if t.lanes == nil {
		why := t.why
		return func(m *state.Model, now time.Time) { m.ApplyTmux(nil, nil, why, errors.New(why), now) }, t.ticks.TmuxIdle
	}
	now := t.clock.Now()
	if now.Sub(t.lastReload) >= t.ticks.RegistryLoad {
		t.lastReload = now
		if err := t.lanes.Registry.Reload(); err != nil {
			why := t.why
			return func(m *state.Model, now time.Time) { m.ApplyTmux(nil, nil, why, err, now) }, t.ticks.TmuxFast
		}
	}
	ls, up, err := t.lanes.ListServer(ctx)
	if err == nil {
		sig := laneSetSig(up, ls)
		if t.lastCheck.IsZero() || sig != t.checkedSig || now.Sub(t.lastCheck) >= t.ticks.TmuxRecheck {
			t.lastCheck, t.checkedSig, t.tmuxWhy = now, sig, ""
			if up {
				if len(ls) > 0 {
					_ = t.lanes.Harden(ctx)
				}
				t.tmuxWhy = t.lanes.TmuxEnvBlocked(ctx)
			}
		}
	}
	blocked := t.why
	if blocked == "" {
		blocked = t.lanes.EnvBlocked()
	}
	if blocked == "" {
		blocked = t.tmuxWhy
	}
	next := t.ticks.TmuxIdle
	if len(ls) > 0 || err != nil {
		next = t.ticks.TmuxFast
	}
	recs, problems := t.lanes.Registry.List(), t.lanes.Registry.Problems()
	return func(m *state.Model, now time.Time) {
		m.ApplyTmux(ls, recs, blocked, err, now)
		m.ApplyRegistryProblems(problems)
	}, next
}

// pathSignature summarises a file, or a directory and its direct entries, by name,
// size and mtime. Polling a signature needs no dependency and no platform watcher.
func pathSignature(path string) string {
	if path == "" {
		return ""
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d:%d", fi.Size(), fi.ModTime().UnixNano())
	if fi.IsDir() {
		entries, _ := os.ReadDir(path)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				names = append(names, fmt.Sprintf("%s:%d:%d", e.Name(), info.Size(), info.ModTime().UnixNano()))
			}
		}
		sort.Strings(names)
		b.WriteString("|" + strings.Join(names, "|"))
	}
	return b.String()
}

// SendNotice shows a notification with osascript.
func SendNotice(ctx context.Context, nt state.Notice) error {
	argv := state.NotifyArgv(nt.Title, nt.Body)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pageVisible is whether a page has said in the last 90 s that it is in view: the
// dashboard's reads (ps, git) run only then, so an idle panel spawns nothing for them.
func (r *Runtime) pageVisible(now time.Time) bool {
	srv := r.srv.Load()
	return srv != nil && srv.PageVisible(now)
}

// pollProcs reads the claude processes' CPU and memory with one ps (PANEL-11).
func (r *Runtime) pollProcs(ctx context.Context, now time.Time) (update, time.Duration) {
	if !r.pageVisible(now) {
		return nil, 0
	}
	var pids []int
	r.hub.Read(func(m *state.Model, _ time.Time) { pids = m.ClaudePIDs() })
	if len(pids) == 0 {
		return nil, 0
	}
	list := make([]string, len(pids))
	for i, p := range pids {
		list[i] = strconv.Itoa(p)
	}
	out, err := r.exec(ctx, 5*time.Second, []string{"ps", "-o", "pid=,pcpu=,rss=", "-p", strings.Join(list, ",")})
	if ctx.Err() != nil {
		return nil, 0
	}
	procs := signals.ParsePS(out)
	if err != nil && (len(procs) > 0 || len(strings.TrimSpace(string(out))) == 0) {
		err = nil // ps exits 1 when a pid is gone (or all are): what it listed is still good
	}
	return func(m *state.Model, now time.Time) { m.ApplyProcs(procs, err, now) }, 0
}

// pollGit reads each lane's worktree with one `git status`; a second call only when
// the tree is dirty (the diff stat) or HEAD moved (the commit's time) (PANEL-11).
func (r *Runtime) pollGit(ctx context.Context, now time.Time) (update, time.Duration) {
	if !r.pageVisible(now) {
		return nil, 0
	}
	var paths []string
	heads := map[string]string{}
	times := map[string]int64{}
	r.hub.Read(func(m *state.Model, now time.Time) {
		paths = m.LaneWorktrees(now)
		for _, p := range paths {
			heads[p], times[p] = m.GitHead(p)
		}
	})
	type read struct {
		path string
		g    signals.GitStat
		err  error
	}
	var reads []read
	for _, p := range paths {
		if ctx.Err() != nil {
			return nil, 0
		}
		out, err := r.exec(ctx, 10*time.Second, []string{"git", "-C", p, "status", "--porcelain=v2", "--branch"})
		var g signals.GitStat
		if err == nil {
			g, err = signals.ParseGitStatusV2(out)
		}
		if err == nil && g.Dirty() > 0 {
			if d, derr := r.exec(ctx, 10*time.Second, []string{"git", "-C", p, "diff", "HEAD", "--shortstat"}); derr == nil {
				g.Files, g.Insertions, g.Deletions = signals.ParseShortstat(d)
			}
		}
		if err == nil && g.Head != "" && g.Head != "(initial)" {
			if g.Head == heads[p] {
				g.LastCommitAt = times[p]
			} else if c, cerr := r.exec(ctx, 10*time.Second, []string{"git", "-C", p, "log", "-1", "--format=%ct"}); cerr == nil {
				if sec, perr := strconv.ParseInt(strings.TrimSpace(string(c)), 10, 64); perr == nil {
					g.LastCommitAt = sec * 1000
				}
			}
		}
		reads = append(reads, read{p, g, err})
	}
	if len(reads) == 0 {
		return nil, 0
	}
	return func(m *state.Model, now time.Time) {
		for _, rd := range reads {
			m.ApplyGit(rd.path, rd.g, rd.err, now)
		}
	}, 0
}
