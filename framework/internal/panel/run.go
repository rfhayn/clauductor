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
	// Project is a project to serve and make the default, registered in
	// projects.json first if it is not (PANEL-16). "" serves the registry as it is.
	Project    string
	ConfigPath string // "" → <Project>/.clauductor/panel.json
	// Only serves Project alone, not the rest of the registry (--only; tests).
	Only   bool
	Port   int
	NoOpen bool
	Home   string // the user's home; injectable for tests
	Out    io.Writer
	Runner signals.Runner
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
	// PANEL-11: the dashboard's trends, and two reads taken only while a page is in
	// view (a page says so every minute; web.Server.PageVisible).
	Trends time.Duration // the trends and the lane-state timelines (no spawn)
	Procs  time.Duration // one `ps` of the claude processes
	Git    time.Duration // one `git status` per worktree with a lane (a second only when dirty)
	// PANEL-19: the Metrics view.
	Spend  time.Duration // the spend the model observed, into the ledger (no spawn)
	Merged time.Duration // the least time between two reads of merged pull requests (gh, while in view)
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
		Trends: 5 * time.Second, Procs: 10 * time.Second, Git: 30 * time.Second,
		Spend: time.Minute, Merged: 10 * time.Minute,
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
		{&t.Trends, &d.Trends}, {&t.Procs, &d.Procs}, {&t.Git, &d.Git}, {&t.Spend, &d.Spend}, {&t.Merged, &d.Merged},
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
	if o.Project != "" {
		// The project named on the command line must load: fail before touching
		// anything, as a single-project panel always has.
		root := signals.ResolvePath(o.Project)
		cfgPath := o.ConfigPath
		if cfgPath == "" {
			cfgPath = filepath.Join(root, config.DefaultConfigRel)
		}
		if _, err := config.LoadConfig(cfgPath); err != nil {
			return err
		}
		if _, err := signals.ReadWorktrees(ctx, o.Runner, root); err != nil {
			return fmt.Errorf("%s: %w", root, err)
		}
	} else if o.Only {
		return errors.New("--only needs --project")
	}
	reg, err := config.LoadProjects(o.Home)
	if err != nil {
		return err
	}
	if o.Project == "" && len(reg.Projects) == 0 {
		return errors.New("no project to serve: run it inside a repository, pass --project <path>, or `clauductor panel add --project <path>`")
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

	// The port is ours: now the named project may be registered (a refused start
	// changes nothing, projects.json included).
	primary := ""
	if o.Project != "" {
		res, err := install.AddProject(ctx, install.AddOptions{Home: o.Home, Project: o.Project, Config: o.ConfigPath,
			Run: o.Runner, Now: clk.Now(), MakeDefault: true})
		if err != nil {
			return err
		}
		if res.Added {
			fmt.Fprintf(o.Out, "registered %s as project %q in %s (its config stays untrusted until `clauductor panel trust`)\n",
				res.Entry.Root, res.Entry.ID, config.ProjectsPath(o.Home))
		}
		primary = res.Entry.ID
		if reg, err = config.LoadProjects(o.Home); err != nil {
			return err
		}
	}
	projects, failed, err := loadProjects(ctx, o, reg, primary)
	if err != nil {
		return err
	}

	var token string
	if o.Launchd {
		token, err = install.LoadOrCreateToken(o.Home)
	} else {
		token, err = install.NewToken()
	}
	if err != nil {
		return err
	}

	// One runtime per project, each with its own model, hub, lanes and socket.
	var runtimes []*Runtime
	var def *Runtime
	hostNames := []string{}
	for _, p := range projects {
		trust := checkConfigTrust(o, p.entry.Root, p.cfgPath, p.raw, o.TrustConfig && p.entry.ID == primary)
		for _, n := range p.cfg.Notices {
			fmt.Fprintf(o.Out, "%s: %s\n", p.entry.ID, n)
		}
		lm, lanesWhy := newLaneManager(o, p.cfg, p.entry.Root, clk)
		if lm != nil {
			lm.Project = p.entry.ID
		}
		model := state.NewModel(p.cfg, p.entry.Root, clk.Now())
		hub := web.NewHub(model, clk)
		hub.TickEvery, hub.Coalesce = ticks.Hub, ticks.HubCoalesce
		wts := p.wts
		hub.Update(func(m *state.Model, now time.Time) { m.ApplyWorktrees(wts, nil, now) })
		r := newRuntime(p.entry.ID, o, p.cfg, p.entry.Root, p.cfgPath, trust, hub, lm, lanesWhy, clk, ticks)
		runtimes = append(runtimes, r)
		if p.entry.ID == primary || (primary == "" && p.entry.ID == reg.Default) {
			def = r
		}
		// A repository's config adds Host names only while it is trusted, except the
		// default project's, which always could (before PANEL-16 it was the only one).
		if trust.Trusted || r == def {
			hostNames = append(hostNames, p.cfg.HostNames...)
		}
	}
	if def == nil {
		def = runtimes[0]
	}
	var roots []string
	for _, r := range runtimes {
		roots = append(roots, r.root)
	}

	marker := install.MarkerPath(o.Home)
	// The PID and owner record sit beside the marker, not in it: status-line scripts
	// read `port` as digits only. A PID that is not running (or runs with another
	// start time) marks the files stale; SIGKILL skips the removal at exit, which
	// takes them only while they are still this panel's.
	if err := install.ClaimPanelFiles(o.Home, install.PanelOwner{PID: os.Getpid(), PStart: lease.ProcStart(os.Getpid()), Project: def.root,
		Name: def.cfg.Name, Port: port, Started: clk.Now().Unix(), Projects: roots}); err != nil {
		return err
	}
	defer install.ReleasePanelFiles(o.Home, os.Getpid())

	m := newMachine(o, clk, ticks, runtimes, def)
	// Install hooks only once the port is ours, so a refused second launch never
	// rewrites settings.json. A failed install is a banner and a retry, not a fatal
	// error; after that, every interval re-checks that they still point here.
	keeper := &hookKeeper{home: o.Home, port: port, apply: m.each, out: o.Out, interval: o.HookCheckInterval, retryBase: o.HookRetryBase}
	if keeper.interval <= 0 {
		keeper.interval = 30 * time.Second
	}
	if keeper.retryBase <= 0 {
		keeper.retryBase = time.Second
	}
	hooksOK := keeper.check()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.addHookKeeper(keeper, hooksOK)
	hooks := make(chan []byte, 256)
	status := make(chan []byte, 64)

	// The projects' menu: every project in the registry's order, the ones that did
	// not load with the reason.
	byID := map[string]web.ProjectSummary{}
	for _, r := range runtimes {
		s := web.Summarize(r.id, r.hub.View())
		s.Default = r == def
		byID[r.id] = s
	}
	for _, f := range failed {
		byID[f.ID] = f
	}
	var menu []web.ProjectSummary
	for _, e := range reg.Projects {
		if s, ok := byID[e.ID]; ok {
			menu = append(menu, s)
		}
	}
	sums := web.NewSummaries(menu)

	defOrch := def.orchestration()
	srv := &web.Server{Port: port, Token: token, Hub: def.hub, Hooks: hooks, Status: status, Refresh: def.refreshAll, Lanes: def.lanes,
		Orch: defOrch, HostNames: hostNames, TermIdleTimeout: o.TermIdleTimeout, Clock: clk, Heartbeat: ticks.Heartbeat,
		Default: def.id, Summaries: sums}
	if o.Launchd {
		srv.CookieMaxAge = int((30 * 24 * time.Hour).Seconds())
	}
	m.srv.Store(srv)
	for _, r := range runtimes {
		r := r
		orch := defOrch
		if r != def {
			orch = r.orchestration()
		}
		srv.Projects = append(srv.Projects, &web.Project{ID: r.id, Name: r.cfg.Name, Hub: r.hub, Lanes: r.lanes, Orch: orch, Refresh: r.refreshAll,
			Metrics: r.metricsReport})
		r.srv.Store(srv)
		r.hub.OnPush = func(v state.View) { sums.Set(web.Summarize(r.id, v)) }
		if r.lanes != nil {
			r.lanes.Changed = func() { kick(r.kickTmux); r.kickWorktrees(); kick(r.kickAgents) }
			r.lanes.Stopped = func(lane string) { srv.CloseTerminalsIn(r.id, lane) }
		}
	}

	var wg sync.WaitGroup
	start := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	for _, r := range runtimes {
		hub := r.hub
		start(func() { hub.Run(ctx) })
	}
	start(func() { m.ingest(ctx, hooks, status) })
	m.start(ctx, start)
	for _, r := range runtimes {
		r.start(ctx, start)
	}

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
	served := fmt.Sprintf("%s (%s)", def.cfg.Name, def.root)
	if n := len(runtimes) - 1; n > 0 {
		served += fmt.Sprintf(" and %d other project(s)", n)
	}
	if len(failed) > 0 {
		served += fmt.Sprintf("; %d project(s) not loaded (the menu says why)", len(failed))
	}
	if o.Launchd {
		// stdout is a log file under launchd: the token stays in its 0600 file.
		fmt.Fprintf(o.Out, "clauductor panel: %s\n  http://%s:%d/ (token in %s; `clauductor panel open` opens it)\n",
			served, web.PanelHost(ln6 != nil), port, install.TokenPath(o.Home))
	} else {
		fmt.Fprintf(o.Out, "clauductor panel: %s\n  %s\n  marker: %s · Ctrl-C to stop\n", served, url, marker)
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
		UploadDir: filepath.Join(config.ProjectDir(o.Home, root), "uploads"),
		Program:   o.LaneProgram, StopTimeout: o.StopTimeout, EnterDelay: 400 * time.Millisecond, FastExit: o.FastExit, Clock: clk}
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
	m.PruneImages() // images dropped more than a day ago, while the panel was down
	return m, why
}

// checkConfigTrust decides whether panel.json's commands and templates may run,
// and says so once.
func checkConfigTrust(o Options, root, cfgPath string, raw []byte, trustNow bool) config.TrustView {
	hash := install.ConfigHash(raw)
	tv, err := install.CheckTrust(o.Home, root, cfgPath, hash, trustNow)
	if err != nil {
		tv = config.TrustView{Hash: hash, Path: signals.ResolvePath(cfgPath), Note: "cannot read the trust record: " + err.Error()}
	}
	trustState := "trusted"
	if !tv.Trusted {
		if tv.Prev == "" {
			trustState = "UNTRUSTED: not trusted yet; its commands and templates are off until you review it and run `clauductor panel trust`"
		} else {
			trustState = "UNTRUSTED: it changed since you trusted " + config.ShortHash(tv.Prev) + "; its commands and templates are off until `clauductor panel trust`"
		}
	}
	if tv.Note != "" {
		trustState += " (" + tv.Note + ")"
	}
	fmt.Fprintf(o.Out, "config %s sha256 %s: %s\n", tv.Path, config.ShortHash(hash), trustState)
	return tv
}
