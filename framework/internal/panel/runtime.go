package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/claudecall"
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
	// openPRLimit is the `--limit` of the open-PR fetch: gh's default is 30, and a
	// list cut short reads as pull requests closed. 1000 is past any real project's
	// open count; a list at exactly the limit is not treated as cut short.
	openPRLimit = "1000"
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

// polled feeds a fetch's result (value or error) to the model. A call the claude
// gate held back (a card or suggest command running claude, D9) applies no update,
// so it is never shown failed for it, and retries soon; an interval card is
// fixed-rate, so it runs again at its next tick instead.
func polled[T any](f fetch[T], apply func(*state.Model, T, error, time.Time)) func(context.Context, time.Time) (update, time.Duration) {
	return func(ctx context.Context, _ time.Time) (update, time.Duration) {
		v, err := f(ctx)
		if wait, held := claudecall.RetryAfter(err); held {
			return nil, wait
		}
		return func(m *state.Model, now time.Time) { apply(m, v, err, now) }, 0
	}
}

// Runtime reads one project's sources into its hub on their cadence and runs the
// project's periodic tasks. Since PANEL-16 a panel runs one per registered project;
// what is the machine's (the account, the hooks, the ingest, the token) is Machine's.
type Runtime struct {
	id      string   // the project's id in projects.json
	machine *Machine // nil in tests that build a Runtime alone
	o       Options
	cfg     *config.Config
	root    string
	cfgPath string
	// outsideConfig is cfgPath when it is outside root (an attached project's sidecar,
	// PANEL-42), else "": the project menu's entry carries it (summary), and it adds
	// the transcripts source (PANEL-49 D3, D6). newRuntime takes it, never set later.
	outsideConfig string
	clock         clock.Clock
	ticks         Ticks
	run           signals.Runner
	hub           *web.Hub
	lanes         *lanes.LaneManager // nil when lanes are unavailable
	registry      *lanes.Registry    // the lanes' registry, nil with them
	srv           atomic.Pointer[web.Server]
	sources       []*source

	kickWT, kickAgents, kickPRs, kickTmux chan struct{}
	cardKicks                             []chan struct{}
	kickMu                                sync.Mutex
	lastWTKick                            time.Time

	trust     atomic.Bool
	trustView config.TrustView
	raw       []byte // the config's bytes as loaded: what trust is recorded for
	// onTrusted, when set, runs once the config is trusted (PANEL-22: the Host names
	// follow trust). It must not block or take the live registry's lock.
	onTrusted func()
	// removed: the project was removed live (PANEL-22); the ingest stops routing to it.
	removed  atomic.Bool
	lastHook atomic.Int64 // unix ns of the last hook the ingest routed here
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
	// stallPath keeps the stall rule's clocks across a restart (PANEL-51 D1:
	// stall.json, beside notifier.json). The stall source and the clean stop both save,
	// so stallMu orders them.
	stallPath string
	stallMu   sync.Mutex

	// mstore is the Metrics view's data (PANEL-19; metrics_source.go).
	mstore *metricsStore
	// ready caches merge readiness's reads (PANEL-20; readiness_source.go).
	ready readinessCache
	// autoClose is lanes_auto_close's state (PANEL-20; autoclose.go); its source's
	// goroutine only.
	autoClose autoCloser
	// onPRLeft is called, never blocking, when a pull request leaves the open list:
	// catch-up's trigger (OPS-118 D3; the liveSet wires it). prsOpen is the list the
	// last successful read held, nil before the first; the prs source's goroutine only.
	onPRLeft func()
	prsOpen  map[int]bool
}

// openPRsRead compares a successful open-PR read with the last one and calls onPRLeft
// when a pull request left. The first read is the baseline: nothing left it.
func (r *Runtime) openPRsRead(prs []signals.PR) {
	now := make(map[int]bool, len(prs))
	for _, p := range prs {
		now[p.Number] = true
	}
	left := false
	for n := range r.prsOpen {
		if !now[n] {
			left = true
		}
	}
	r.prsOpen = now
	if left && r.onPRLeft != nil {
		r.onPRLeft()
	}
}

// newRuntime builds a project's runtime and its table of sources.
// outsideConfig is the config's path when it is outside root, else "" (live.go's
// install.OutsideConfig): it decides whether the transcripts source is added.
func newRuntime(id string, o Options, cfg *config.Config, root, cfgPath, outsideConfig string, tv config.TrustView, hub *web.Hub,
	lm *lanes.LaneManager, lanesWhy string, clk clock.Clock, ticks Ticks) *Runtime {
	r := &Runtime{id: id, o: o, cfg: cfg, root: root, cfgPath: cfgPath, outsideConfig: outsideConfig, clock: clk, ticks: ticks, run: o.Runner, hub: hub, lanes: lm,
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
	hub.Update(func(m *state.Model, now time.Time) { m.SetProjectID(id); m.ApplyTrust(tv) })
	// PANEL-51 (D1): the stall rule's saved clocks, handed to the model before any
	// source runs, so before the first `claude agents` poll.
	r.stallPath = filepath.Join(config.ProjectDir(o.Home, root), "stall.json")
	stalls := cfg.AlertThresholds().Stall > 0
	if stalls {
		if b, err := os.ReadFile(r.stallPath); err == nil {
			var st state.StallState
			if json.Unmarshal(b, &st) == nil {
				hub.Update(func(m *state.Model, _ time.Time) { m.ApplyStallState(st) })
			}
		}
	} else {
		// With stalls off nothing saves, so the last save would wait on disk until they
		// are turned on, and restore a stall from then with its old since, exact, while
		// the notifier has dropped its mark: a push for a stall the panel saw end. Off,
		// the rule keeps no state, and turned on it starts as from no file (D1).
		_ = os.Remove(r.stallPath)
	}

	t := ticks
	worktrees := func(ctx context.Context) ([]signals.Worktree, error) {
		return signals.ReadWorktrees(ctx, r.run, r.root)
	}
	// gh lists 30 rows unless asked for more, and the stall rule reads this list as every
	// open pull request (D3, D10): a lane's pull request past the 30th would read as none.
	openPRs := commandFetch(r, 30*time.Second, []string{"gh", "pr", "list", "--json", "number,title,headRefName,author,isDraft,statusCheckRollup,reviewDecision",
		"--limit", openPRLimit}, signals.ParsePRs)
	var prs fetch[[]signals.PR] = func(ctx context.Context) ([]signals.PR, error) {
		v, err := openPRs(ctx)
		if err == nil {
			r.openPRsRead(v) // a failed read is no news: nothing left the list
		}
		return v, err
	}
	// The table of sources. Each runs in its own goroutine, through Runtime.loop.
	r.sources = []*source{
		// Re-read within a watch tick of git's own worktree registry changing (a
		// worktree added or removed), and early for a cwd the list does not know.
		{name: "worktrees", every: t.Worktrees, fixedRate: true, kick: r.kickWT, watch: r.worktreeRegistryDir, watchEvery: t.WorktreeWatch,
			poll: polled(worktrees, (*state.Model).ApplyWorktrees)},
		r.agentsSource(),
		{name: "prs", every: t.PRs, fixedRate: true, kick: r.kickPRs, poll: polled(prs, (*state.Model).ApplyPRs)},
		{name: "tmux", every: t.TmuxIdle, kick: r.kickTmux, poll: newTmuxPoller(lm, lanesWhy, clk, t).poll},
		{name: "obs", every: t.Obs, fixedRate: true, waitFirst: true, poll: r.pollObs},
		{name: "notify", every: t.Notify, fixedRate: true, waitFirst: true, poll: r.pollNotify()},
		{name: "trends", every: t.Trends, fixedRate: true, poll: func(context.Context, time.Time) (update, time.Duration) {
			return func(m *state.Model, now time.Time) { m.Sample(now) }, 0
		}},
		{name: "procs", every: t.Procs, fixedRate: true, waitFirst: true, poll: r.pollProcs},
		// Refresh re-reads git too (PANEL-18): after a pull, the cards' stale note goes at once.
		{name: "git", every: t.Git, fixedRate: true, kick: make(chan struct{}, 1), poll: r.pollGit},
		// PANEL-20: merge readiness, on git's cadence. PANEL-51 (D10): with no page in
		// view it still reads the receipts and the merged read, for the stall rule, while
		// stalls are on; tasks.md and the review threads only while a page is in view.
		{name: "readiness", every: t.Git, fixedRate: true, kick: make(chan struct{}, 1), poll: r.pollReadiness},
	}
	for _, c := range cfg.Cards {
		r.sources = append(r.sources, r.cardSource(c))
	}
	for _, t := range cfg.Templates {
		if t.Suggest != nil {
			r.sources = append(r.sources, r.suggestSource(t.ID, *t.Suggest))
		}
	}
	if cfg.QuotaAutoResume && lm != nil {
		r.sources = append(r.sources, &source{name: "autoresume", every: t.Notify, fixedRate: true, waitFirst: true, poll: r.pollAutoResume})
	}
	if cfg.AnyAutoClose() && lm != nil {
		r.sources = append(r.sources, &source{name: "autoclose", every: t.PRs, fixedRate: true, waitFirst: true, poll: r.pollAutoClose})
	}
	r.mstore = newMetricsStore(o, root, cfg, tv.Trusted)
	r.sources = append(r.sources, r.metricsSources()...)
	if len(cfg.Queues) > 0 {
		r.sources = append(r.sources, &source{name: "queues", every: t.Queues, fixedRate: true, poll: r.pollQueues})
	}
	if stalls {
		// Once every StallSave (D1: at most every 30 s; the state carries the time of the
		// save, so it has always changed since the last); start saves once more at a
		// clean stop.
		r.sources = append(r.sources, &source{name: "stall", every: t.StallSave, fixedRate: true, waitFirst: true,
			poll: func(context.Context, time.Time) (update, time.Duration) { r.saveStall(); return nil, 0 }})
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
	if outsideConfig != "" {
		// PANEL-49 D3: an attached project's status line does not post to the panel, so
		// its sessions are measured from their transcripts; a project with its config in
		// its repository never adds this source, and reads no transcript.
		hub.Update(func(m *state.Model, _ time.Time) { m.MeasureFromTranscripts() })
		r.sources = append(r.sources, r.transcriptsSource())
	}
	return r
}

// start runs every source in its own goroutine.
func (r *Runtime) start(ctx context.Context, start func(func())) {
	for _, s := range r.sources {
		s := s
		start(func() { r.loop(ctx, s) })
	}
	if r.cfg.AlertThresholds().Stall > 0 {
		// The clean stop's save of stall.json (D1), counted with the sources, so a
		// runtime stopped and started again reads what this one last held.
		start(func() { <-ctx.Done(); r.saveStall() })
	}
}

// saveStall writes the stall rule's state to stall.json: 0600, atomically, as
// notifier.json is (PANEL-51 D1). The state carries the time of the save, so each
// call writes, and each tick's save keeps a lane's carried quiet time within a tick of
// the truth: a crash then moves a clock back by at most a tick, never forward. A way
// whose sources have not answered since the start is saved as the restart restored it
// (Model.StallState).
func (r *Runtime) saveStall() {
	// Held across the read too: a tick's save that read before the clean stop's must not
	// write after it, putting an older state on disk last.
	r.stallMu.Lock()
	defer r.stallMu.Unlock()
	var st state.StallState
	read := false
	r.hub.Read(func(m *state.Model, now time.Time) {
		// StallState keeps the lanes of the worktree list the model holds: saved before
		// the first list is read, it would drop every lane's restored clock.
		if read = len(m.Worktrees()) > 0; read {
			st = m.StallState(now)
		}
	})
	b, err := json.Marshal(st)
	if !read || err != nil {
		return
	}
	if config.EnsurePrivateDir(filepath.Dir(r.stallPath)) == nil {
		_ = config.WriteAtomic(r.stallPath, b, 0o600)
	}
}

// loop runs one source until ctx ends.
func (r *Runtime) loop(ctx context.Context, s *source) {
	runLoop(ctx, r.clock, s, r.pollOnce)
}

// runLoop runs one source until ctx ends: poll, wait for the next tick, a kick or a
// change in what it watches, and again.
func runLoop(ctx context.Context, clk clock.Clock, s *source, pollOnce func(context.Context, *source) (time.Duration, bool)) {
	if s.watch != nil {
		path := s.watch(ctx)
		// Waited for, so a runtime stopped live (PANEL-22) has no goroutine left.
		done := make(chan struct{})
		go func() { defer close(done); watchLoop(ctx, clk, path, s) }()
		defer func() { <-done }()
	}
	var ticks <-chan time.Time
	if s.fixedRate && s.every > 0 {
		t := clk.NewTicker(s.every)
		defer t.Stop()
		ticks = t.C()
	}
	next, ok := s.every, true
	if !s.waitFirst {
		next, ok = pollOnce(ctx, s)
	}
	for ok && next >= 0 {
		var wait <-chan time.Time
		var timer clock.Timer
		switch {
		case s.fixedRate:
			wait = ticks
		case next > 0:
			timer = clk.NewTimer(next)
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
		next, ok = pollOnce(ctx, s)
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
	r.polled(s.name)
	if next == 0 {
		next = s.every
	}
	return next, true
}

// polled reports one poll to Options.OnPoll: by its name, and by <project>/<name>,
// so a test of several projects counts each one's polls apart.
func (r *Runtime) polled(name string) {
	if r.o.OnPoll != nil {
		r.o.OnPoll(name)
		if r.id != "" {
			r.o.OnPoll(r.id + "/" + name)
		}
	}
}

// watchLoop kicks s whenever path's signature changes.
func watchLoop(ctx context.Context, clk clock.Clock, path string, s *source) {
	last := pathSignature(path)
	t := clk.NewTicker(s.watchEvery)
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

// exec runs argv in the project root through the Runner, with a timeout. Every call
// through it is a poll's (the agents poll, checkFilter, cards and suggest commands,
// the machine's): the gate gives a poll a short wait and a quick retry (PANEL-28).
func (r *Runtime) exec(ctx context.Context, timeout time.Duration, argv []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(signals.WithPoll(ctx), timeout)
	defer cancel()
	return r.run(cctx, r.root, argv)
}

func kick(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// refreshAll polls every kickable source now (the page's refresh). With machine (the
// owner's Refresh button) it kicks the machine's sources too: the version, read
// otherwise only on its 10-minute timer, and the account, which no timer reads
// (PANEL-28, D5). The page's refresh after a lane start passes false, so a session
// starting never has `claude auth status` run beside it.
func (r *Runtime) refreshAll(machine bool) {
	if r.mstore != nil {
		r.mstore.mergedDue.Store(true)
	}
	for _, s := range r.sources {
		if s.kick != nil {
			kick(s.kick)
		}
	}
	if machine && r.machine != nil {
		r.machine.kickAll()
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
	out, err := r.run(ctx, r.root, servingGit("", "rev-parse", "--git-common-dir"))
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
	run := polled(r.cardFetch(c.Command), func(m *state.Model, co *signals.CardOutput, err error, now time.Time) {
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

// cardFetch runs a card's command. A sidecar's card (a config outside its repository)
// shows its stderr after its stdout, since its argv cannot say 2>&1 (PANEL-42,
// design.md D5, Output). The merge is here, at the card's call site, and not in
// commandFetch, which suggest, metrics, gh and claude share: their stdout is parsed.
func (r *Runtime) cardFetch(argv []string) fetch[*signals.CardOutput] {
	return func(ctx context.Context) (*signals.CardOutput, error) {
		var stderr []byte
		if r.outsideConfig != "" {
			ctx = signals.WithStderr(ctx, func(b []byte) { stderr = b })
		}
		return commandFetch(r, 30*time.Second, argv, func(out []byte) (*signals.CardOutput, error) {
			co := signals.ParseCardOutput(out).WithStderr(stderr)
			return &co, nil
		})(ctx)
	}
}

// suggestSource runs a template's suggest command on its refresh rule, exactly as a
// card runs (same timeout, same trust gate, kicked by REFRESH too), and hands the
// dialog what it prints.
func (r *Runtime) suggestSource(id string, sg config.SuggestConfig) *source {
	rule, _ := config.ParseRefresh(sg.Refresh) // validated at load
	run := polled(commandFetch(r, 30*time.Second, sg.Command, func(out []byte) (*signals.Suggestions, error) {
		s, err := signals.ParseSuggestions(out)
		return &s, err
	}), func(m *state.Model, s *signals.Suggestions, err error, now time.Time) {
		m.ApplySuggestions(id, s, err, now)
	})
	s := &source{name: "suggest " + id, kick: make(chan struct{}, 1),
		poll: func(ctx context.Context, now time.Time) (update, time.Duration) {
			if !r.trusted() {
				return func(m *state.Model, now time.Time) { m.ApplySuggestions(id, nil, errUntrusted, now) }, 0
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
		// Its time is recorded only after a cross-check that ran (PANEL-28): one the
		// gate held back keeps the filter as it was, and is tried again with the poll.
		filter, err := r.checkFilter(ctx, wts)
		if wait, held := claudecall.RetryAfter(err); held {
			return heldUpdate("agents", err), wait
		}
		r.agentsFilterCheck, r.agentsFilter = now, filter
	}
	argv := append([]string{"claude", "agents", "--json"}, r.agentsFilter...)
	// Timed from when the gate lets it run: the poll's figures (AgentsPollAvgMs, D3's
	// live estimate) are the call's, not its wait for the slot or a lock.
	t0 := r.clock.Now()
	out, err := r.exec(claudecall.WithAcquired(ctx, func() { t0 = r.clock.Now() }), 10*time.Second, argv)
	dur := r.clock.Now().Sub(t0)
	if wait, held := claudecall.RetryAfter(err); held {
		return heldUpdate("agents", err), wait
	}
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

// heldUpdate is the update for a machine or agents poll the claude gate held back
// (PANEL-28): none for a busy slot; for a live lock, the pause note on its source
// alone, which leaves its last reading, its success time and its error as they were.
func heldUpdate(source string, err error) update {
	if !errors.Is(err, claudecall.ErrPaused) {
		return nil
	}
	return func(m *state.Model, _ time.Time) { m.PauseSource(source, claudecall.ErrPaused.Error()) }
}

// checkFilter decides whether `--cwd <dir>` may be used: only when the filtered list
// holds every in-project session the unfiltered one does. Its error is the claude
// gate's refusal alone (the cross-check did not run); a failed call is a filter of nil.
func (r *Runtime) checkFilter(ctx context.Context, wts []signals.Worktree) ([]string, error) {
	dir := signals.AgentsFilterDir(r.root, wts)
	set := func(desc string) { r.setObs(func(o *state.Obs) { o.AgentsFilter = desc }) }
	if dir == "" {
		set("unfiltered: the worktrees share no directory but /")
		return nil, nil
	}
	list := commandFetch(r, 10*time.Second, []string{"claude", "agents", "--json"}, signals.ParseAgents)
	all, err := list(ctx)
	if _, held := claudecall.RetryAfter(err); held {
		return nil, err
	}
	if err != nil {
		set("unfiltered: cannot cross-check (" + signals.Clip(err.Error(), 80) + ")")
		return nil, nil
	}
	filtered, err := commandFetch(r, 10*time.Second, []string{"claude", "agents", "--json", "--cwd", dir}, signals.ParseAgents)(ctx)
	if _, held := claudecall.RetryAfter(err); held {
		return nil, err
	}
	if err != nil {
		set("unfiltered: --cwd failed (" + signals.Clip(err.Error(), 80) + ")")
		return nil, nil
	}
	var recs map[string]bool
	r.hub.Read(func(m *state.Model, _ time.Time) { recs = m.LaneSessions() })
	missed := signals.MissedByFilter(all, filtered, func(a signals.Agent) bool {
		return recs[a.SessionID] || signals.MatchWorktree(wts, signals.ResolvePath(a.Cwd)) >= 0
	})
	if len(missed) > 0 {
		set(fmt.Sprintf("unfiltered: --cwd %s missed %d session(s) of this project", dir, len(missed)))
		return nil, nil
	}
	set("--cwd " + dir + " (cross-checked every " + r.ticks.AgentsFilter.String() + ")")
	return []string{"--cwd", dir}, nil
}

// pollObs copies the counters into the model, only when they change (a model
// update is pushed to every page).
func (r *Runtime) pollObs(context.Context, time.Time) (update, time.Duration) {
	r.mu.Lock()
	if r.machine != nil {
		r.obs.MalformedDrops = r.machine.malformed.Load()
	}
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
	refused := "" // the push refusal this project's model holds: the source's goroutine only
	return func(ctx context.Context, now time.Time) (update, time.Duration) {
		// The machine's push command, or its refusal, as it stands at this tick (D7).
		var push pushCmd
		if r.machine != nil {
			push = r.machine.push.current()
		}
		var up update
		if push.refused != refused {
			refused = push.refused
			why := refused
			up = func(m *state.Model, _ time.Time) { m.ApplyPushRefused(why) }
		}
		send := r.o.Notify
		if send == nil {
			send = func(n state.Notice) error { return SendNotice(ctx, n) }
		}
		v := r.hub.View()
		focused := map[string]bool{}
		if s := r.srv.Load(); s != nil {
			focused = s.FocusedLanesIn(r.id)
		}
		if r.machine != nil {
			// The quota is the account's, and the login locks (PANEL-28) the
			// machine's: the machine alone notifies them, once, not once per project
			// (PANEL-16). The page still shows them in every project.
			v.Alerts = withoutQuota(v.Alerts)
		}
		r.mu.Lock()
		// The project name comes from panel.json, so it is config like any other: an
		// untrusted config does not get to title OS notifications.
		r.notifier.Project = "clauductor panel"
		if r.trusted() {
			r.notifier.Project = r.cfg.Name
		}
		notices := r.notifier.ProcessView(v, focused, now)
		project := r.notifier.Project
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
			// A stall also goes to the owner's push command, after the macOS
			// notification and whatever became of it (D7).
			if pushed, perr := pushNotice(ctx, push, project, n, now); pushed && perr != nil {
				r.setObs(func(o *state.Obs) {
					o.NotifyFailed++
					o.NotifyError = signals.Clip(perr.Error(), 160)
				})
				fmt.Fprintf(r.o.Out, "push failed: %v\n", perr)
			}
		}
		if stats == r.notified {
			return up, 0
		}
		r.notified = stats
		return both(up, func(m *state.Model, now time.Time) { m.ApplyNotifier(stats) }), 0
	}
}

// pollTrust lifts the untrusted mode once `clauductor panel trust` records the
// loaded config's hash, then stops.
func (r *Runtime) pollTrust(context.Context, time.Time) (update, time.Duration) {
	if r.trusted() { // trusted from the page meanwhile (markTrusted)
		return nil, -1
	}
	if !install.TrustedNow(r.o.Home, r.root, r.cfgPath, r.trustView.Hash) {
		return nil, 0
	}
	return r.markTrusted("trusted by `clauductor panel trust`"), -1
}

// markTrusted turns the loaded config's commands on: its trust was just recorded (by
// `panel trust`, or by the page's Trust config…, PANEL-22). It returns the model
// update, and tells the live registry, whose Host names follow trust.
func (r *Runtime) markTrusted(note string) update {
	if r.trust.Swap(true) {
		return nil
	}
	tv := r.trustView
	tv.Trusted, tv.Note = true, note
	fmt.Fprintf(r.o.Out, "%s: config trusted: cards, queue commands and templates are on\n", r.id)
	for _, k := range r.cardKicks {
		kick(k)
	}
	if r.onTrusted != nil {
		r.onTrusted()
	}
	return func(m *state.Model, now time.Time) { m.ApplyTrust(tv) }
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
//
// Serving writes nothing into a checkout (PANEL-42, ATTACH-2): every read is a
// servingGit argv. The diff stat is `diff-index`, not `diff HEAD`: git 2.54's `diff`
// refreshes and rewrites the index even with --no-optional-locks. diff-index does not
// refresh, so it sees a file whose stat alone changed as changed, and counts a binary
// one (it cannot compare a binary pair by lines, and the worktree side has no hash).
// Its pathspec is therefore the tracked paths status found changed: status re-hashed
// each stat-only file in memory and left it out (gitStatArgv).
func (r *Runtime) pollGit(ctx context.Context, now time.Time) (update, time.Duration) {
	if !r.pageVisible(now) {
		return nil, 0
	}
	var paths []string
	heads := map[string]string{}
	times := map[string]int64{}
	r.hub.Read(func(m *state.Model, now time.Time) {
		paths = m.LaneWorktrees(now)
		// The checkout the cards run in, whether or not a lane does (PANEL-18): when
		// it is behind its upstream the page says the cards may be stale. The same one
		// `git status`, and no fetch: behind is as of the last fetch.
		if c := m.CardsCheckout(); c != "" && !slices.Contains(paths, c) {
			paths = append(paths, c)
		}
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
		out, err := r.exec(ctx, 10*time.Second, servingGit(p, "status", "--porcelain=v2", "--branch"))
		var g signals.GitStat
		if err == nil {
			g, err = signals.ParseGitStatusV2(out)
		}
		// Untracked files alone leave the stat at zero, as `diff HEAD` did: it never
		// counted them.
		if err == nil && g.Changed+g.Conflicts > 0 {
			if d, derr := r.exec(ctx, 10*time.Second, gitStatArgv(p, out)); derr == nil {
				g.Files, g.Insertions, g.Deletions = signals.ParseShortstat(d)
			}
		}
		if err == nil && g.Head != "" && g.Head != "(initial)" {
			if g.Head == heads[p] {
				g.LastCommitAt = times[p]
			} else if c, cerr := r.exec(ctx, 10*time.Second, servingGit(p, "log", "-1", "--format=%ct")); cerr == nil {
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

// servingGit is the argv of every git read the panel makes of a served checkout,
// so no read can leave a flag out (PANEL-42, ATTACH-2: serving writes nothing).
// --no-optional-locks: git takes no index.lock to write back a refreshed .git/index.
// core.fsmonitor= (empty): a repository that sets it would have git start its fsmonitor
// daemon, which writes .git/fsmonitor--daemon/ (or run the hook it names). Empty, not
// `false`: before git 2.36 the value names a hook program, so `false` would run
// /usr/bin/false on every poll (Ubuntu 22.04 ships 2.34); an empty value is off on
// every version. dir is the checkout (-C); "" runs in the runner's directory.
func servingGit(dir string, args ...string) []string {
	argv := []string{"git", "--no-optional-locks", "-c", "core.fsmonitor="}
	if dir != "" {
		argv = append(argv, "-C", dir)
	}
	return append(argv, args...)
}

// maxStatPathspec bounds the bytes of the paths gitStatArgv passes as arguments, well
// under any OS's argument limit (macOS: 1 MiB, environment included).
const maxStatPathspec = 64 << 10

// gitStatArgv is the dirty-tree stat of checkout dir, given its `git status
// --porcelain=v2` output: `diff-index -M --shortstat HEAD` over the tracked paths that
// status found changed, taken literally. -M counts a staged rename once, as `diff
// HEAD` did. When status lists more path bytes than maxStatPathspec, or a path that
// does not unquote, it reads the whole tree, and a binary file whose stat alone
// changed may then count.
func gitStatArgv(dir string, status []byte) []string {
	args := []string{"--literal-pathspecs", "diff-index", "-M", "--shortstat", "HEAD"}
	paths, ok := signals.TrackedChangedPaths(status)
	size := 0
	for _, p := range paths {
		size += len(p) + 1
	}
	if ok && len(paths) > 0 && size <= maxStatPathspec {
		args = append(append(args, "--"), paths...)
	}
	return servingGit(dir, args...)
}
