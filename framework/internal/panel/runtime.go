package panel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Runner runs one command in dir and returns its stdout. Injectable for tests.
type Runner func(ctx context.Context, dir string, argv []string) ([]byte, error)

const maxCmdOutput = 1 << 20

// ExecRunner runs argv directly (no shell) with a limited stdout.
func ExecRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var out, errb limitedBuffer
	out.max, errb.max = maxCmdOutput, 4096
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %v: %s", argv[0], err, msg)
		}
		return nil, fmt.Errorf("%s: %v", argv[0], err)
	}
	return out.Bytes(), nil
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// Options configures one panel run.
type Options struct {
	Project    string // project root (already resolved by the caller)
	ConfigPath string // "" → <Project>/.clauductor/panel.json
	Port       int
	NoOpen     bool
	Home       string // the user's home; injectable for tests
	Out        io.Writer
	Runner     Runner
	// OnReady, if set, is called with the launch URL once serving (tests use it).
	OnReady func(url string)

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

	// v2.
	// TrustConfig trusts the config as it is now, even if it changed (--trust-config).
	TrustConfig bool
	// Notify overrides how an OS notification is shown (tests record it instead).
	Notify func(Notice) error
	// LockRunArgv overrides the lock-run command a queue RUN starts (default: this
	// binary's `lock-run`).
	LockRunArgv []string

	// PANEL-5.
	// HookCheckInterval overrides how often the hooks are verified (default 30 s).
	HookCheckInterval time.Duration
	// HookRetryBase overrides the first retry delay after a failed hook install
	// (default 1 s, doubling to HookCheckInterval).
	HookRetryBase time.Duration
}

// MarkerPath is the file whose existence tells a status-line script the panel is up.
func MarkerPath(home string) string { return filepath.Join(home, ".clauductor", "panel", "port") }

// ResolvePath makes a path comparable with worktree paths: absolute, symlinks
// resolved (macOS /tmp is /private/tmp). A path that no longer exists is cleaned only.
func ResolvePath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Run serves the panel until ctx is cancelled.
func Run(ctx context.Context, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Runner == nil {
		o.Runner = ExecRunner
	}
	if o.Home == "" {
		return errors.New("cannot determine the home directory")
	}
	root := ResolvePath(o.Project)
	cfgPath := o.ConfigPath
	if cfgPath == "" {
		cfgPath = filepath.Join(root, DefaultConfigRel)
	}
	cfg, rawCfg, err := LoadConfigRaw(cfgPath)
	if err != nil {
		return err
	}
	// The worktree list is the event filter's authority; without it every event
	// would be dropped, so a failure here is fatal rather than a quiet empty panel.
	wts, err := readWorktrees(ctx, o.Runner, root)
	if err != nil {
		return fmt.Errorf("%s: %w", root, err)
	}

	// One panel per machine (singleton.go): refuse before touching the port, the
	// hooks or the marker files, so a refused start changes nothing. The machine
	// lock comes first, so two panels started at the same instant cannot both pass;
	// the pid-file check then catches a panel from before the lock.
	lock, err := lockMachine(o.Home)
	var held *OtherPanelError
	if errors.As(err, &held) && o.Launchd {
		// Under launchd, a panel started by hand holds the machine: wait for it to
		// exit, then take over (PANEL-7). Exiting instead would leave the login agent
		// down after that panel stops, since KeepAlive restarts only a failed exit.
		fmt.Fprintf(o.Out, "waiting for the running panel to exit (%s)\n", held.Owner.describe())
		lock, err = waitMachineLock(ctx, o.Home)
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
		if other := RunningPanel(ctx, o.Home, os.Getpid(), LiveProc); other != nil {
			err = &OtherPanelError{Owner: *other}
		}
	}
	var refused *OtherPanelError
	if errors.As(err, &refused) && o.Launchd {
		// KeepAlive restarts the agent only after a non-zero exit: exit 0, so launchd
		// does not retry every 30 s, and say why once.
		fmt.Fprintf(o.Out, "not starting: %v\n", err)
		return nil
	}
	if err != nil {
		return err
	}

	ln, ln6, v6why, err := ListenLoopback(o.Port)
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

	marker := MarkerPath(o.Home)
	// The PID and owner record sit beside the marker, not in it: status-line scripts
	// read `port` as digits only. A PID that is not running (or runs with another
	// start time) marks the files stale; SIGKILL skips the removal at exit, which
	// takes them only while they are still this panel's.
	if err := claimPanelFiles(o.Home, PanelOwner{PID: os.Getpid(), PStart: ProcStart(os.Getpid()), Project: root,
		Name: cfg.Name, Port: port, Started: time.Now().Unix()}); err != nil {
		return err
	}
	defer releasePanelFiles(o.Home, os.Getpid())

	var token string
	if o.Launchd {
		token, err = LoadOrCreateToken(o.Home)
	} else {
		token, err = NewToken()
	}
	if err != nil {
		return err
	}
	if o.TmuxSocket != "" {
		if !socketNameRe.MatchString(o.TmuxSocket) {
			return fmt.Errorf("tmux socket %q must match %s", o.TmuxSocket, socketNameRe)
		}
		cfg.TmuxSocket = o.TmuxSocket
	}
	trust := checkConfigTrust(o, root, cfgPath, rawCfg)
	lanes, lanesWhy := newLaneManager(o, cfg, root)
	model := NewModel(cfg, root, time.Now())
	hub := NewHub(model, time.Now)
	hub.Update(func(m *Model, now time.Time) { m.ApplyWorktrees(wts, nil, now) })

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
	p := &pollers{hub: hub, run: o.Runner, root: root, cfg: cfg,
		kickWT: make(chan struct{}, 1), kickAgents: make(chan struct{}, 1), kickPRs: make(chan struct{}, 1), kickTmux: make(chan struct{}, 1)}
	if lanes != nil {
		p.registry = lanes.Registry // set before ingest starts reading it
	}
	p.x = newRuntimeV2(o, cfg, root, cfgPath, trust, hub, p, lanes)
	hooks := make(chan []byte, 256)
	status := make(chan []byte, 64)

	var wg sync.WaitGroup
	start := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	start(func() { hub.Run(ctx) })
	start(func() { p.ingest(ctx, hooks, status) })
	start(func() { p.worktreeLoop(ctx) })
	start(func() { p.agentsLoop(ctx) })
	start(func() { p.prLoop(ctx) })
	start(func() { p.tmuxLoop(ctx, lanes, lanesWhy) })
	start(func() { keeper.loop(ctx, hooksOK) })
	if lanes != nil {
		lanes.Changed = func() { kick(p.kickTmux); p.kickWorktrees(); kick(p.kickAgents) }
	}
	for _, c := range cfg.Cards {
		c := c
		kick := make(chan struct{}, 1)
		p.cardKicks = append(p.cardKicks, kick)
		start(func() { p.cardLoop(ctx, c, kick) })
	}
	p.x.start(ctx, start)

	srv := &Server{Port: port, Token: token, Hub: hub, Hooks: hooks, Status: status, Refresh: p.refreshAll, Lanes: lanes}
	srv.Orch = p.x.orchestration()
	srv.HostNames = cfg.HostNames
	p.x.srv.Store(srv)
	srv.TermIdleTimeout = o.TermIdleTimeout
	if lanes != nil {
		lanes.Stopped = srv.closeTerminals
	}
	if o.Launchd {
		// `clauductor panel rotate-token` (or a reinstall) replaces the token file;
		// follow it so the old token dies in the running panel too.
		start(func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if tok := readToken(o.Home); tok != "" && tok != srv.currentToken() {
						srv.Rotate(tok)
						fmt.Fprintln(o.Out, "token rotated: old cookies, terminals and event streams are closed")
					}
				}
			}
		})
	}
	if o.Launchd {
		srv.CookieMaxAge = int((30 * 24 * time.Hour).Seconds())
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

	url := fmt.Sprintf("http://%s:%d/?t=%s", PanelHost(ln6 != nil), port, token)
	if o.Launchd {
		// stdout is a log file under launchd: the token stays in its 0600 file.
		fmt.Fprintf(o.Out, "clauductor panel: %s (%s)\n  http://%s:%d/ (token in %s; `clauductor panel open` opens it)\n",
			cfg.Name, root, PanelHost(ln6 != nil), port, TokenPath(o.Home))
	} else {
		fmt.Fprintf(o.Out, "clauductor panel: %s (%s)\n  %s\n  marker: %s · Ctrl-C to stop\n", cfg.Name, root, url, marker)
	}
	if o.OnReady != nil {
		o.OnReady(url)
	}
	open := openBrowser
	if o.OpenBrowser != nil {
		open = o.OpenBrowser
	}
	switch {
	case o.Launchd:
		if shouldOpenAtLogin(o.Home, time.Now()) {
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

func openBrowser(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	_ = exec.Command(name, url).Start()
}

func readWorktrees(ctx context.Context, run Runner, root string) ([]Worktree, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := run(cctx, root, []string{"git", "worktree", "list", "--porcelain"})
	if err != nil {
		return nil, err
	}
	wts, err := ParseWorktreePorcelain(out)
	if err != nil {
		return nil, err
	}
	for i := range wts {
		wts[i].Path = ResolvePath(wts[i].Path)
	}
	return wts, nil
}

type pollers struct {
	hub        *Hub
	run        Runner
	root       string
	cfg        *Config
	kickWT     chan struct{}
	kickAgents chan struct{}
	kickPRs    chan struct{}
	kickTmux   chan struct{}
	cardKicks  []chan struct{}
	registry   *Registry // nil when lanes are unavailable
	x          *runtimeV2

	mu         sync.Mutex
	lastWTKick time.Time
}

func kick(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (p *pollers) refreshAll() {
	kick(p.kickWT)
	kick(p.kickAgents)
	kick(p.kickPRs)
	kick(p.kickTmux)
	for _, k := range p.cardKicks {
		kick(k)
	}
}

// kickWorktrees re-reads the worktree authority early (rate-limited), e.g. when an
// event arrives from a cwd the current list does not know — likely a new worktree.
func (p *pollers) kickWorktrees() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.lastWTKick) < 2*time.Second {
		return
	}
	p.lastWTKick = time.Now()
	kick(p.kickWT)
}

// loop runs f now, then every d, and whenever kicked.
func loop(ctx context.Context, d time.Duration, kickCh <-chan struct{}, f func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		f()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-kickCh:
		}
	}
}

func (p *pollers) ingest(ctx context.Context, hooks, status <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case body := <-hooks:
			ev, err := ParseHook(body)
			if err != nil {
				p.x.malformed.Add(1)
				continue
			}
			p.x.hookSeen(ev)
			ev.Cwd = ResolvePath(ev.Cwd)
			// A prompt, or a finished turn, means the session has a conversation to
			// --resume. Hooks can be dropped, so busy in `claude agents` counts too.
			if (ev.Event == "UserPromptSubmit" || ev.Event == "Stop") && p.registry != nil {
				_, _ = p.registry.MarkConversation(ev.SessionID)
			}
			kept := true
			p.hub.Update(func(m *Model, now time.Time) { kept = m.ApplyHook(ev, now) })
			if !kept {
				p.kickWorktrees()
			}
		case body := <-status:
			st, err := ParseStatus(body)
			if err != nil {
				p.x.malformed.Add(1)
				continue
			}
			st.Cwd = ResolvePath(st.Cwd)
			p.hub.Update(func(m *Model, now time.Time) { m.ApplyStatus(st, now) })
		}
	}
}

// worktreeLoop polls every 10 s, and also re-reads within ~2 s when git's own
// worktree registry directory changes (a worktree added or removed).
func (p *pollers) worktreeLoop(ctx context.Context) {
	regDir := ""
	if out, err := p.run(ctx, p.root, []string{"git", "rev-parse", "--git-common-dir"}); err == nil {
		d := strings.TrimSpace(string(out))
		if !filepath.IsAbs(d) {
			d = filepath.Join(p.root, d)
		}
		regDir = filepath.Join(d, "worktrees")
	}
	go func() {
		last := pathSignature(regDir)
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if sig := pathSignature(regDir); sig != last {
					last = sig
					kick(p.kickWT)
				}
			}
		}
	}()
	loop(ctx, 10*time.Second, p.kickWT, func() {
		wts, err := readWorktrees(ctx, p.run, p.root)
		if ctx.Err() != nil {
			return
		}
		p.hub.Update(func(m *Model, now time.Time) { m.ApplyWorktrees(wts, err, now) })
	})
}

func (p *pollers) agentsLoop(ctx context.Context) {
	if p.x != nil {
		// v2: --cwd, backoff while hooks flow, latency. It keeps v1's duty below:
		// a session seen busy is marked as having a conversation.
		p.x.agentsLoop(ctx)
		return
	}
	loop(ctx, 2*time.Second, p.kickAgents, func() {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := p.run(cctx, p.root, []string{"claude", "agents", "--json"})
		var agents []Agent
		if err == nil {
			agents, err = ParseAgents(out)
		}
		if ctx.Err() != nil {
			return
		}
		for i := range agents {
			agents[i].Cwd = ResolvePath(agents[i].Cwd)
			if agents[i].Status == "busy" && p.registry != nil {
				_, _ = p.registry.MarkConversation(agents[i].SessionID)
			}
		}
		p.hub.Update(func(m *Model, now time.Time) { m.ApplyAgents(agents, err, now) })
	})
}

func (p *pollers) prLoop(ctx context.Context) {
	loop(ctx, 60*time.Second, p.kickPRs, func() {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := p.run(cctx, p.root, []string{"gh", "pr", "list", "--json", "number,title,headRefName,author,isDraft,statusCheckRollup"})
		var prs []PR
		if err == nil {
			prs, err = ParsePRs(out)
		}
		if ctx.Err() != nil {
			return
		}
		p.hub.Update(func(m *Model, now time.Time) { m.ApplyPRs(prs, err, now) })
	})
}

func (p *pollers) cardLoop(ctx context.Context, c CardConfig, kickCh chan struct{}) {
	rule, _ := ParseRefresh(c.Refresh) // validated at load
	runCard := func() {
		if !p.x.trusted() {
			p.hub.Update(func(m *Model, now time.Time) { m.ApplyCard(c.ID, nil, errUntrusted, now) })
			return
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := p.run(cctx, p.root, c.Command)
		if ctx.Err() != nil {
			return
		}
		var co *CardOutput
		if err == nil {
			parsed := ParseCardOutput(out)
			co = &parsed
		}
		p.hub.Update(func(m *Model, now time.Time) { m.ApplyCard(c.ID, co, err, now) })
	}
	if rule.Interval > 0 {
		loop(ctx, rule.Interval, kickCh, runCard)
		return
	}
	watched := filepath.Join(p.root, rule.WatchRel)
	last := pathSignature(watched)
	runCard()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-kickCh:
			runCard()
		case <-t.C:
			if sig := pathSignature(watched); sig != last {
				last = sig
				runCard()
			}
		}
	}
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

// newLaneManager wires lane control, or returns why it is unavailable. A missing tmux
// or claude disables starting lanes; the panel still watches.
func newLaneManager(o Options, cfg *Config, root string) (*LaneManager, string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return nil, "tmux was not found on the panel's PATH, so lanes cannot start or be shown here"
	}
	reg, err := OpenRegistry(o.Home, root)
	if err != nil {
		return nil, "the lane registry cannot be read, so lanes are not managed: " + err.Error()
	}
	m := &LaneManager{TmuxPath: tmuxPath, Socket: cfg.Socket(), Root: root, Cfg: cfg, Registry: reg, Run: o.Runner,
		Program: o.LaneProgram, StopTimeout: o.StopTimeout, EnterDelay: 400 * time.Millisecond, FastExit: o.FastExit}
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

// tmux poll cadence (PANEL-7). Every tmux call is a process spawn, and the panel
// is meant to idle for days.
const (
	// tmuxFast is the list-panes interval while the socket has lanes.
	tmuxFast = 2 * time.Second
	// tmuxIdle is the interval while it has none: a lane the panel starts or
	// restores kicks the loop at once, so nothing waits for this.
	tmuxIdle = 10 * time.Second
	// tmuxRecheck is how often show-environment (an API key in the server's
	// environment) and Harden run while the lane set stays the same. A lane start
	// checks the environment itself, and every viewer hardens before it attaches.
	tmuxRecheck = 30 * time.Second
	// registryReload is how often the registry is re-read from disk.
	registryReload = 30 * time.Second
)

// tmuxPoller reconciles the lane registry with the socket, one tick at a time.
type tmuxPoller struct {
	lanes *LaneManager
	why   string // lanes are unavailable (from newLaneManager)
	now   func() time.Time

	lastReload time.Time
	lastCheck  time.Time // the last show-environment / Harden
	checkedSig string    // the lane set they ran against
	tmuxWhy    string    // what show-environment said then
}

func newTmuxPoller(lanes *LaneManager, why string, now func() time.Time) *tmuxPoller {
	return &tmuxPoller{lanes: lanes, why: why, now: now, lastReload: now()}
}

// laneSetSig names the lane set: whether the socket has a server, and each lane.
func laneSetSig(up bool, ls []TmuxLane) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%t", up)
	for _, l := range ls {
		b.WriteString("|" + l.ID)
	}
	return b.String()
}

// tick polls once, and returns the model update and how long to wait before the
// next tick. list-panes (one call covers every lane) runs every tick;
// show-environment and Harden only when the lane set changed or tmuxRecheck
// passed, and never without a server: no server has no environment to check.
func (t *tmuxPoller) tick(ctx context.Context) (func(m *Model, now time.Time), time.Duration) {
	if t.lanes == nil {
		why := t.why
		return func(m *Model, now time.Time) { m.ApplyTmux(nil, nil, why, errors.New(why), now) }, tmuxIdle
	}
	now := t.now()
	if now.Sub(t.lastReload) >= registryReload {
		t.lastReload = now
		if err := t.lanes.Registry.Reload(); err != nil {
			why := t.why
			return func(m *Model, now time.Time) { m.ApplyTmux(nil, nil, why, err, now) }, tmuxFast
		}
	}
	ls, up, err := t.lanes.ListServer(ctx)
	if err == nil {
		sig := laneSetSig(up, ls)
		if t.lastCheck.IsZero() || sig != t.checkedSig || now.Sub(t.lastCheck) >= tmuxRecheck {
			t.lastCheck, t.checkedSig, t.tmuxWhy = now, sig, ""
			if up {
				if len(ls) > 0 {
					_ = t.lanes.Harden(ctx)
				}
				t.tmuxWhy = t.lanes.tmuxEnvBlocked(ctx)
			}
		}
	}
	blocked := t.why
	if blocked == "" {
		blocked = t.lanes.envBlocked()
	}
	if blocked == "" {
		blocked = t.tmuxWhy
	}
	next := tmuxIdle
	if len(ls) > 0 || err != nil {
		next = tmuxFast
	}
	recs, problems := t.lanes.Registry.List(), t.lanes.Registry.Problems()
	return func(m *Model, now time.Time) {
		m.ApplyTmux(ls, recs, blocked, err, now)
		m.ApplyRegistryProblems(problems)
	}, next
}

// tmuxLoop reconciles the lane registry with the socket: at start (so lanes that
// outlived a panel restart reappear, and lanes a reboot killed show as orphans),
// every tmuxFast while the socket has lanes (tmuxIdle while it has none), and right
// after a lane command. Every 30 s it also re-reads the registry file from disk
// rather than trusting its in-memory copy. The reducer matches the result against
// claude agents (by session id) and the worktree list.
func (p *pollers) tmuxLoop(ctx context.Context, lanes *LaneManager, why string) {
	t := newTmuxPoller(lanes, why, time.Now)
	for {
		update, next := t.tick(ctx)
		if ctx.Err() != nil {
			return
		}
		p.hub.Update(update)
		timer := time.NewTimer(next)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-p.kickTmux:
			timer.Stop()
		}
	}
}
