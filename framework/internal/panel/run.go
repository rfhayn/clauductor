package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// Options configures one panel run.
type Options struct {
	Project    string // project root (already resolved by the caller)
	ConfigPath string // "" → <Project>/.clauductor/panel.json
	Port       int
	NoOpen     bool
	Home       string // the user's home; injectable for tests
	Out        io.Writer
	Runner     signals.Runner
	// OnReady, if set, is called with the launch URL once serving (tests use it).
	OnReady func(url string)
	// OnPoll, if set, is called with a source's name each time one of its polls has
	// been applied to the model: a test counts a source's iterations with it, rather
	// than sleeping to see that something did not happen.
	OnPoll func(source string)

	// Launchd marks a run under the launchd login agent: the token persists in
	// TokenPath, the cookie lasts 30 days, and the browser opens once per login
	// rather than on every start.
	Launchd bool
	// TmuxSocket overrides the config's tmux_socket (tests use a throwaway socket).
	TmuxSocket string
	// LaneProgram overrides the lane program, `claude` (tests run sh or cat).
	LaneProgram []string
	// StopTimeout overrides how long a lane stop waits for /exit (default 10 s).
	StopTimeout time.Duration
	// FastExit overrides how long a restarted claude must stay up (default 3 s).
	FastExit time.Duration
	// TermIdleTimeout overrides how long a silent terminal stays open (default 5 min).
	TermIdleTimeout time.Duration
	// OpenBrowser overrides how the page is opened (tests record the URL instead).
	OpenBrowser func(url string)

	// TrustConfig trusts the config as it is now, even if it changed (--trust-config).
	TrustConfig bool
	// Notify overrides how an OS notification is shown (tests record it instead).
	Notify func(state.Notice) error
	// LockRunArgv overrides the lock-run command a queue RUN starts (default: this
	// binary's `lock-run`).
	LockRunArgv []string

	// HookCheckInterval overrides how often the hooks are verified (default 30 s).
	HookCheckInterval time.Duration
	// HookRetryBase overrides the first retry delay after a failed hook install
	// (default 1 s, doubling to HookCheckInterval).
	HookRetryBase time.Duration

	// Clock is the panel's one clock: the hub, the notifier, the lanes, every source
	// and the queue view read it. Nil is the system clock.
	Clock clock.Clock
	// Ticks overrides the cadence of the sources and periodic tasks; a zero field
	// keeps its default (DefaultTicks).
	Ticks Ticks
}

// Ticks are the panel's tick lengths. Every process the panel spawns while it idles
// is paced by one of them (PANEL-7), so they are in one place.
type Ticks struct {
	Worktrees     time.Duration // `git worktree list`
	WorktreeWatch time.Duration // git's worktree registry directory, for a change
	WorktreeKick  time.Duration // the least time between two early worktree reads (a cwd it does not know)
	PRs           time.Duration // `gh pr list`
	Version       time.Duration // `claude --version`
	AgentsFast    time.Duration // `claude agents`: a lane, and no hook flowing
	AgentsSlow    time.Duration // ... while hooks flow
	AgentsQuiet   time.Duration // ... with no lane and no hook for a while
	AgentsFilter  time.Duration // the --cwd filter's cross-check with the unfiltered list
	TmuxFast      time.Duration // list-panes while the socket has lanes
	TmuxIdle      time.Duration // ... while it has none (a lane start kicks it at once)
	TmuxRecheck   time.Duration // show-environment and Harden, while the lane set stays the same
	RegistryLoad  time.Duration // the lane registry re-read from disk
	CardWatch     time.Duration // a watch: card's path, for a change
	Queues        time.Duration // the queues' leases
	Prompt        time.Duration // the first-prompt decisions
	PromptKick    time.Duration // the most often those drive a `claude agents` poll
	Obs           time.Duration // the observability counters, into the model
	Notify        time.Duration // alerts, into OS notifications
	Trust         time.Duration // an untrusted config, for `clauductor panel trust`
	Token         time.Duration // the token file, for a rotation (launchd)
	Hub           time.Duration // the hub's re-derivation when nothing arrives
	HubCoalesce   time.Duration // how long an update waits for more before the push
	Heartbeat     time.Duration // an open /events stream's heartbeat
}

// DefaultTicks are the tick lengths the panel runs with.
func DefaultTicks() Ticks {
	return Ticks{
		Worktrees: 10 * time.Second, WorktreeWatch: 2 * time.Second, WorktreeKick: 2 * time.Second,
		PRs: 60 * time.Second, Version: versionRecheck,
		AgentsFast: agentsFast, AgentsSlow: state.AgentsSlow, AgentsQuiet: agentsQuiet, AgentsFilter: filterRecheck,
		TmuxFast: tmuxFast, TmuxIdle: tmuxIdle, TmuxRecheck: tmuxRecheck, RegistryLoad: registryReload,
		CardWatch: 2 * time.Second, Queues: time.Second, Prompt: time.Second, PromptKick: promptKickEvery,
		Obs: time.Second, Notify: 2 * time.Second, Trust: 5 * time.Second, Token: 2 * time.Second,
		Hub: 5 * time.Second, HubCoalesce: 150 * time.Millisecond, Heartbeat: web.HeartbeatEvery,
	}
}

// withDefaults fills every zero tick with its default.
func (t Ticks) withDefaults() Ticks {
	d := DefaultTicks()
	for _, f := range []struct{ v, def *time.Duration }{
		{&t.Worktrees, &d.Worktrees}, {&t.WorktreeWatch, &d.WorktreeWatch}, {&t.WorktreeKick, &d.WorktreeKick},
		{&t.PRs, &d.PRs}, {&t.Version, &d.Version}, {&t.AgentsFast, &d.AgentsFast}, {&t.AgentsSlow, &d.AgentsSlow},
		{&t.AgentsQuiet, &d.AgentsQuiet}, {&t.AgentsFilter, &d.AgentsFilter}, {&t.TmuxFast, &d.TmuxFast},
		{&t.TmuxIdle, &d.TmuxIdle}, {&t.TmuxRecheck, &d.TmuxRecheck}, {&t.RegistryLoad, &d.RegistryLoad},
		{&t.CardWatch, &d.CardWatch}, {&t.Queues, &d.Queues}, {&t.Prompt, &d.Prompt}, {&t.PromptKick, &d.PromptKick},
		{&t.Obs, &d.Obs}, {&t.Notify, &d.Notify}, {&t.Trust, &d.Trust}, {&t.Token, &d.Token}, {&t.Hub, &d.Hub},
		{&t.HubCoalesce, &d.HubCoalesce}, {&t.Heartbeat, &d.Heartbeat},
	} {
		if *f.v <= 0 {
			*f.v = *f.def
		}
	}
	return t
}

// Run serves the panel until ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Runner == nil {
		o.Runner = signals.ExecRunner
	}
	if o.Home == "" {
		return errors.New("cannot determine the home directory")
	}
	clk := o.Clock
	if clk == nil {
		clk = clock.System // the one default: Run is where the clock is chosen
	}
	ticks := o.Ticks.withDefaults()
	root := signals.ResolvePath(o.Project)
	cfgPath := o.ConfigPath
	if cfgPath == "" {
		cfgPath = filepath.Join(root, config.DefaultConfigRel)
	}
	cfg, rawCfg, err := config.LoadConfigRaw(cfgPath)
	if err != nil {
		return err
	}
	// The worktree list is the event filter's authority; without it every event
	// would be dropped, so a failure here is fatal rather than a quiet empty panel.
	wts, err := signals.ReadWorktrees(ctx, o.Runner, root)
	if err != nil {
		return fmt.Errorf("%s: %w", root, err)
	}

	// One panel per machine (install/singleton.go): refuse before touching the port,
	// the hooks or the marker files, so a refused start changes nothing. The machine
	// lock comes first, so two panels started at the same instant cannot both pass;
	// the pid-file check then catches a panel from before the lock.
	lock, err := install.LockMachine(o.Home)
	var held *install.OtherPanelError
	if errors.As(err, &held) && o.Launchd {
		// Under launchd, a panel started by hand holds the machine: wait for it to
		// exit, then take over (PANEL-7). Exiting instead would leave the login agent
		// down after that panel stops, since KeepAlive restarts only a failed exit.
		fmt.Fprintf(o.Out, "waiting for the running panel to exit (%s)\n", held.Owner.Describe())
		lock, err = install.WaitMachineLock(ctx, o.Home)
		if ctx.Err() != nil {
			if lock != nil {
				lock.Close()
			}
			return nil
		}
		if err == nil {
			fmt.Fprintln(o.Out, "the running panel exited; taking over")
		}
	}
	if err == nil {
		defer lock.Close()
		if other := install.RunningPanel(ctx, o.Home, os.Getpid(), lease.LiveProc); other != nil {
			err = &install.OtherPanelError{Owner: *other}
		}
	}
	var refused *install.OtherPanelError
	if errors.As(err, &refused) && o.Launchd {
		// KeepAlive restarts the agent only after a non-zero exit: exit 0, so launchd
		// does not retry every 30 s, and say why once.
		fmt.Fprintf(o.Out, "not starting: %v\n", err)
		return nil
	}
	if err != nil {
		return err
	}

	ln, ln6, v6why, err := web.ListenLoopback(o.Port)
	if err != nil {
		return err
	}
	defer ln.Close()
	if ln6 != nil {
		defer ln6.Close()
	} else {
		fmt.Fprintf(o.Out, "no IPv6 loopback (%s): serving 127.0.0.1 only, so open the panel at 127.0.0.1\n", v6why)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	marker := install.MarkerPath(o.Home)
	// The PID and owner record sit beside the marker, not in it: status-line scripts
	// read `port` as digits only. A PID that is not running (or runs with another
	// start time) marks the files stale; SIGKILL skips the removal at exit, which
	// takes them only while they are still this panel's.
	if err := install.ClaimPanelFiles(o.Home, install.PanelOwner{PID: os.Getpid(), PStart: lease.ProcStart(os.Getpid()), Project: root,
		Name: cfg.Name, Port: port, Started: clk.Now().Unix()}); err != nil {
		return err
	}
	defer install.ReleasePanelFiles(o.Home, os.Getpid())

	var token string
	if o.Launchd {
		token, err = install.LoadOrCreateToken(o.Home)
	} else {
		token, err = install.NewToken()
	}
	if err != nil {
		return err
	}
	if o.TmuxSocket != "" {
		if !config.SocketNameRe.MatchString(o.TmuxSocket) {
			return fmt.Errorf("tmux socket %q must match %s", o.TmuxSocket, config.SocketNameRe)
		}
		cfg.TmuxSocket = o.TmuxSocket
	}
	trust := checkConfigTrust(o, root, cfgPath, rawCfg)
	for _, n := range cfg.Notices {
		fmt.Fprintln(o.Out, n)
	}
	lm, lanesWhy := newLaneManager(o, cfg, root, clk)
	model := state.NewModel(cfg, root, clk.Now())
	hub := web.NewHub(model, clk)
	hub.TickEvery, hub.Coalesce = ticks.Hub, ticks.HubCoalesce
	hub.Update(func(m *state.Model, now time.Time) { m.ApplyWorktrees(wts, nil, now) })

	// Install hooks only once the port is ours, so a refused second launch never
	// rewrites settings.json. A failed install is a banner and a retry, not a fatal
	// error; after that, every interval re-checks that they still point here.
	keeper := &hookKeeper{home: o.Home, port: port, hub: hub, out: o.Out, interval: o.HookCheckInterval, retryBase: o.HookRetryBase}
	if keeper.interval <= 0 {
		keeper.interval = 30 * time.Second
	}
	if keeper.retryBase <= 0 {
		keeper.retryBase = time.Second
	}
	hooksOK := keeper.check()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := newRuntime(o, cfg, root, cfgPath, trust, hub, lm, lanesWhy, clk, ticks)
	r.addHookKeeper(keeper, hooksOK)
	hooks := make(chan []byte, 256)
	status := make(chan []byte, 64)

	srv := &web.Server{Port: port, Token: token, Hub: hub, Hooks: hooks, Status: status, Refresh: r.refreshAll, Lanes: lm,
		Orch: r.orchestration(), HostNames: cfg.HostNames, TermIdleTimeout: o.TermIdleTimeout, Clock: clk, Heartbeat: ticks.Heartbeat}
	if o.Launchd {
		srv.CookieMaxAge = int((30 * 24 * time.Hour).Seconds())
	}
	r.srv.Store(srv)
	if lm != nil {
		lm.Changed = func() { kick(r.kickTmux); r.kickWorktrees(); kick(r.kickAgents) }
		lm.Stopped = srv.CloseTerminals
	}

	var wg sync.WaitGroup
	start := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	start(func() { hub.Run(ctx) })
	start(func() { r.ingest(ctx, hooks, status) })
	r.start(ctx, start)

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// SSE handlers watch the request context; tying it to ctx lets shutdown end them.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	serveErr := make(chan error, 2)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	if ln6 != nil { // [::1]: clauductor.localhost resolves there first
		go func() { serveErr <- httpSrv.Serve(ln6) }()
	}

	url := fmt.Sprintf("http://%s:%d/?t=%s", web.PanelHost(ln6 != nil), port, token)
	if o.Launchd {
		// stdout is a log file under launchd: the token stays in its 0600 file.
		fmt.Fprintf(o.Out, "clauductor panel: %s (%s)\n  http://%s:%d/ (token in %s; `clauductor panel open` opens it)\n",
			cfg.Name, root, web.PanelHost(ln6 != nil), port, install.TokenPath(o.Home))
	} else {
		fmt.Fprintf(o.Out, "clauductor panel: %s (%s)\n  %s\n  marker: %s · Ctrl-C to stop\n", cfg.Name, root, url, marker)
	}
	if o.OnReady != nil {
		o.OnReady(url)
	}
	open := install.OpenBrowser
	if o.OpenBrowser != nil {
		open = o.OpenBrowser
	}
	switch {
	case o.Launchd:
		if install.ShouldOpenAtLogin(o.Home, clk.Now()) {
			open(url)
		}
	case !o.NoOpen:
		open(url)
	}

	select {
	case <-ctx.Done():
	case err = <-serveErr:
	}
	cancel()
	shutCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = httpSrv.Shutdown(shutCtx)
	wg.Wait()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newLaneManager wires lane control, or returns why it is unavailable. A missing tmux
// or claude disables starting lanes; the panel still watches.
func newLaneManager(o Options, cfg *config.Config, root string, clk clock.Clock) (*lanes.LaneManager, string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return nil, "tmux was not found on the panel's PATH, so lanes cannot start or be shown here"
	}
	reg, err := lanes.OpenRegistry(o.Home, root)
	if err != nil {
		return nil, "the lane registry cannot be read, so lanes are not managed: " + err.Error()
	}
	m := &lanes.LaneManager{TmuxPath: tmuxPath, Socket: cfg.Socket(), Root: root, Cfg: cfg, Registry: reg, Run: o.Runner,
		Program: o.LaneProgram, StopTimeout: o.StopTimeout, EnterDelay: 400 * time.Millisecond, FastExit: o.FastExit, Clock: clk}
	if m.FastExit == 0 {
		m.FastExit = 3 * time.Second
	}
	if m.StopTimeout == 0 {
		m.StopTimeout = 10 * time.Second
	}
	why := ""
	if len(m.Program) == 0 {
		if claude, err := exec.LookPath("claude"); err == nil {
			m.Program = []string{claude}
		} else {
			m.Program = []string{"claude"}
			why = "claude was not found on the panel's PATH"
		}
	}
	return m, why
}

// checkConfigTrust decides whether panel.json's commands and templates may run,
// and says so once.
func checkConfigTrust(o Options, root, cfgPath string, raw []byte) config.TrustView {
	hash := install.ConfigHash(raw)
	tv, err := install.CheckTrust(o.Home, root, cfgPath, hash, o.TrustConfig)
	if err != nil {
		tv = config.TrustView{Hash: hash, Path: signals.ResolvePath(cfgPath), Note: "cannot read the trust record: " + err.Error()}
	}
	trustState := "trusted"
	if !tv.Trusted {
		trustState = "UNTRUSTED: it changed since you trusted " + config.ShortHash(tv.Prev) + "; its commands and templates are off until `clauductor panel trust`"
	}
	if tv.Note != "" {
		trustState += " (" + tv.Note + ")"
	}
	fmt.Fprintf(o.Out, "config %s sha256 %s: %s\n", tv.Path, config.ShortHash(hash), trustState)
	return tv
}
