package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// Machine is what a panel holds once for the whole machine (PANEL-16), however many
// projects it serves: the hooks (one URL in ~/.claude/settings.json for every
// session), the ingest and its dispatcher, the token, Claude Code's version and the
// account (its quota, its plan), and the quota alert. Each project's own sources run
// in its Runtime; what the machine reads it applies to every project's model, so
// each project's view carries the account as it did with one project.
type Machine struct {
	o     Options
	clock clock.Clock
	ticks Ticks
	// projMu guards projects and def: since PANEL-22 a project is added or removed,
	// and the default changes, while the panel serves. Read them through list() and
	// defRT().
	projMu   sync.RWMutex
	projects []*Runtime // in the menu's order
	def      *Runtime   // the default project
	srv      atomic.Pointer[web.Server]
	sources  []*source

	malformed atomic.Int64

	// bound is each session's project once the dispatcher has placed it: a session
	// stays with its project after a `cd` elsewhere. The ingest goroutine only.
	bound map[string]*Runtime

	// quotaPath keeps the account's last quota across restarts (PANEL-12);
	// verifiedPath the Claude Code version verified from live hooks (PANEL-13).
	quotaPath, verifiedPath string
	savedQuota              int64
	savedVerified           string

	// The account's quota alert: one OS notification for the machine, not one per
	// project (each project's own notifier skips it).
	notifier   state.Notifier
	notifyPath string
	savedState string

	// economy is economy mode (PANEL-19; economy.go).
	economy *economy
}

// maxBound caps the dispatcher's session table; past it the table starts over, and
// each project's model still knows the sessions bound to it.
const maxBound = 10000

func newMachine(o Options, clk clock.Clock, ticks Ticks, projects []*Runtime, def *Runtime) *Machine {
	m := &Machine{o: o, clock: clk, ticks: ticks, projects: projects, def: def, bound: map[string]*Runtime{}}
	for _, r := range projects {
		r.machine = m
	}
	dir := config.PanelDir(o.Home)
	m.quotaPath = filepath.Join(dir, "quota.json")
	if b, err := os.ReadFile(m.quotaPath); err == nil {
		var q state.Quota
		if json.Unmarshal(b, &q) == nil {
			m.savedQuota = q.At
			m.each(func(md *state.Model, now time.Time) { md.RestoreQuota(q) })
		}
	}
	m.verifiedPath = filepath.Join(dir, "verified.json")
	if b, err := os.ReadFile(m.verifiedPath); err == nil {
		var vf savedVerification
		if json.Unmarshal(b, &vf) == nil && vf.Version != "" {
			m.savedVerified = vf.Version
			m.each(func(md *state.Model, now time.Time) { md.RestoreAutoVerified(vf.Version) })
		}
	}
	m.notifier = state.Notifier{MinInterval: def.cfg.AlertThresholds().MinInterval, Project: "clauductor panel"}
	m.notifyPath = filepath.Join(dir, "notifier.json")
	if b, err := os.ReadFile(m.notifyPath); err == nil {
		var st state.NotifierState
		if json.Unmarshal(b, &st) == nil {
			m.notifier.Restore(st)
		}
	}
	t := ticks
	m.sources = []*source{
		// Kickable (PANEL-22): a project added live has them read at once.
		{name: "version", every: t.Version, kick: make(chan struct{}, 1), poll: m.pollVersion()},
		{name: "account", every: t.Version, kick: make(chan struct{}, 1), poll: m.pollAccount()},
		{name: "saved", every: t.Trends, fixedRate: true, waitFirst: true, poll: m.saveReadings},
		{name: "quota-alert", every: t.Notify, fixedRate: true, waitFirst: true, poll: m.pollQuotaAlert},
		{name: "economy", every: t.Trends, fixedRate: true, waitFirst: true, poll: m.pollEconomy},
		// Where Remote Control is on (PANEL-19): settings.json and the install's
		// choice, re-read on the account's cadence (install restarts the agent anyway).
		{name: "remote", every: t.Version, kick: make(chan struct{}, 1), poll: func(context.Context, time.Time) (update, time.Duration) {
			mode := install.RemoteControlSummary(o.Home)
			return func(md *state.Model, _ time.Time) { md.ApplyRemoteControl(mode) }, 0
		}},
	}
	m.economy = newEconomy(o.Home, def.cfg)
	if o.Launchd {
		// `clauductor panel rotate-token` (or a reinstall) replaces the token file;
		// follow it so the old token dies in the running panel too.
		m.sources = append(m.sources, &source{name: "token", every: t.Token, fixedRate: true, waitFirst: true, poll: m.pollToken})
	}
	return m
}

// each applies an update to every project's model.
func (m *Machine) each(up update) {
	for _, r := range m.list() {
		r.hub.Update(up)
	}
}

// list is the projects served now, in the menu's order (a copy).
func (m *Machine) list() []*Runtime {
	m.projMu.RLock()
	defer m.projMu.RUnlock()
	return append([]*Runtime(nil), m.projects...)
}

// defRT is the default project now.
func (m *Machine) defRT() *Runtime {
	m.projMu.RLock()
	defer m.projMu.RUnlock()
	return m.def
}

// addProject serves one more project's runtime (PANEL-22). It takes the account's
// last quota and Claude Code's verified version from the default project's model, so
// its view carries them before the machine's next reads.
func (m *Machine) addProject(r *Runtime) {
	r.machine = m
	def := m.defRT()
	if def != nil {
		var q *state.Quota
		verified := ""
		def.hub.Read(func(md *state.Model, _ time.Time) { q, verified = md.QuotaReading(), md.AutoVerified() })
		r.hub.Update(func(md *state.Model, _ time.Time) {
			if q != nil {
				md.RestoreQuota(*q)
			}
			if verified != "" {
				md.RestoreAutoVerified(verified)
			}
		})
	}
	m.projMu.Lock()
	m.projects = append(m.projects, r)
	if m.def == nil {
		m.def = r
	}
	m.projMu.Unlock()
	m.kickAll() // the version, the account, the hooks and remote control, read for it now
}

// removeProject stops routing anything to a runtime. The ingest's session table
// forgets it on its next look (route).
func (m *Machine) removeProject(r *Runtime) {
	m.projMu.Lock()
	defer m.projMu.Unlock()
	kept := m.projects[:0:0]
	for _, x := range m.projects {
		if x != r {
			kept = append(kept, x)
		}
	}
	m.projects = kept
}

// setDefault makes r the default project: the quota alert's thresholds, economy
// mode's threshold and the commands the machine runs (claude --version) follow it.
func (m *Machine) setDefault(r *Runtime) {
	m.projMu.Lock()
	m.def = r
	m.projMu.Unlock()
}

// kickAll polls every machine source that can be kicked, now.
func (m *Machine) kickAll() {
	for _, s := range m.sources {
		if s.kick != nil {
			kick(s.kick)
		}
	}
}

// addHookKeeper adds the hooks check: it ran once at start and reported ok; it
// runs again every interval, or sooner with backoff after a failure.
func (m *Machine) addHookKeeper(k *hookKeeper, ok bool) {
	m.sources = append(m.sources, &source{name: "hooks", every: k.nextWait(ok), waitFirst: true, kick: make(chan struct{}, 1),
		poll: func(context.Context, time.Time) (update, time.Duration) { return nil, k.nextWait(k.check()) }})
}

// start runs every machine source in its own goroutine.
func (m *Machine) start(ctx context.Context, start func(func())) {
	for _, s := range m.sources {
		s := s
		start(func() { runLoop(ctx, m.clock, s, m.pollOnce) })
	}
}

// pollOnce is Runtime.pollOnce for the machine: its update goes to every project.
func (m *Machine) pollOnce(ctx context.Context, s *source) (time.Duration, bool) {
	up, next := s.poll(ctx, m.clock.Now())
	if ctx.Err() != nil {
		return 0, false
	}
	if up != nil {
		m.each(up)
	}
	if m.o.OnPoll != nil {
		m.o.OnPoll(s.name)
	}
	if next == 0 {
		next = s.every
	}
	return next, true
}

// run runs argv through the Runner in the default project's root, with a timeout.
func (m *Machine) run(ctx context.Context, timeout time.Duration, argv []string) ([]byte, error) {
	return m.defRT().exec(ctx, timeout, argv)
}

// pollVersion reads `claude --version`, and warns once per read when it is not the
// version the heuristics were verified on.
func (m *Machine) pollVersion() func(context.Context, time.Time) (update, time.Duration) {
	return func(ctx context.Context, _ time.Time) (update, time.Duration) {
		out, err := m.run(ctx, 10*time.Second, []string{"claude", "--version"})
		v := ""
		if err == nil {
			v, err = signals.ParseClaudeVersion(out)
		}
		if ctx.Err() == nil && err == nil && v != state.HeuristicsVerifiedOn {
			fmt.Fprintf(m.o.Out, "warning: Claude Code %s differs from %s, which the subagent heuristics and fixtures were verified on; subagent lists are approximate\n", v, state.HeuristicsVerifiedOn)
		}
		return func(md *state.Model, now time.Time) { md.ApplyClaudeVersion(v, err, now) }, 0
	}
}

// pollAccount reads `claude auth status --json` on the version's cadence (PANEL-15):
// how the account signs in and its plan, which decide what the quota's place shows.
// It runs with a lane's environment (no API key), so it names the login lanes use.
// It costs no token; nothing personal it prints is kept (signals.ParseAuthStatus).
func (m *Machine) pollAccount() func(context.Context, time.Time) (update, time.Duration) {
	argv := lanes.ScrubbedArgv("claude", "auth", "status", "--json")
	return func(ctx context.Context, _ time.Time) (update, time.Duration) {
		out, err := m.run(ctx, 10*time.Second, argv)
		var a signals.AuthStatus
		if err == nil {
			a, err = signals.ParseAuthStatus(out)
		}
		return func(md *state.Model, now time.Time) { md.ApplyAccount(a, err, now) }, 0
	}
}

// savedVerification is verified.json: the Claude Code version the subagent pairing was
// verified on from live hooks, and when.
type savedVerification struct {
	Version string `json:"version"`
	At      int64  `json:"at"`
}

// saveReadings writes what the panel keeps across a restart, when it changed: the
// quota, and a version verified from live hooks. One panel runs per machine, so each
// file has one writer. Every project's model folds every status post, so any one of
// them holds the account's quota; a version any project verified holds for all.
func (m *Machine) saveReadings(_ context.Context, now time.Time) (update, time.Duration) {
	var q *state.Quota
	m.defRT().hub.Read(func(md *state.Model, _ time.Time) { q = md.QuotaReading() })
	verified := ""
	for _, r := range m.list() {
		r.hub.Read(func(md *state.Model, _ time.Time) {
			if v := md.AutoVerified(); v != "" && verified == "" {
				verified = v
			}
		})
	}
	if q != nil && q.At != m.savedQuota {
		if b, err := json.Marshal(q); err == nil && writePrivate(m.quotaPath, b) == nil {
			m.savedQuota = q.At
		}
	}
	if verified != "" && verified != m.savedVerified {
		b, _ := json.Marshal(savedVerification{Version: verified, At: now.UnixMilli()})
		if writePrivate(m.verifiedPath, b) == nil {
			m.savedVerified = verified
			fmt.Fprintf(m.o.Out, "Claude Code %s: the subagent pairing held on this machine's own hooks; recorded as verified\n", verified)
			return func(md *state.Model, now time.Time) { md.RestoreAutoVerified(verified) }, 0
		}
	}
	return nil, 0
}

func writePrivate(path string, b []byte) error {
	if err := config.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	return config.WriteAtomic(path, b, 0o600)
}

// withoutQuota drops the account's quota alert: a project's notifier leaves it to
// the machine.
func withoutQuota(as []state.AlertView) []state.AlertView {
	out := as[:0:0]
	for _, a := range as {
		if a.Kind != state.AlertQuota {
			out = append(out, a)
		}
	}
	return out
}

// pollQuotaAlert notifies the account's quota alert once for the machine. The alert
// is derived in the default project's view, against its thresholds.
func (m *Machine) pollQuotaAlert(ctx context.Context, now time.Time) (update, time.Duration) {
	def := m.defRT()
	v := def.hub.View()
	var qa []state.AlertView
	for _, a := range v.Alerts {
		if a.Kind == state.AlertQuota {
			qa = append(qa, a)
		}
	}
	notices := m.notifier.Process(qa, nil, now)
	if st, _ := json.Marshal(m.notifier.State()); string(st) != m.savedState {
		if writePrivate(m.notifyPath, st) == nil {
			m.savedState = string(st)
		}
	}
	if !def.cfg.AlertThresholds().Notify {
		return nil, 0
	}
	send := m.o.Notify
	if send == nil {
		send = func(n state.Notice) error { return SendNotice(ctx, n) }
	}
	for _, n := range notices {
		err := send(n)
		def.setObs(func(o *state.Obs) {
			if err != nil {
				o.NotifyFailed++
				o.NotifyError = signals.Clip(err.Error(), 160)
			} else {
				o.NotifySent++
			}
		})
		if err != nil {
			fmt.Fprintf(m.o.Out, "notification failed: %v\n", err)
		}
	}
	return nil, 0
}

// pollToken follows the token file: a rotated token closes every cookie, terminal
// and event stream of the old one.
func (m *Machine) pollToken(context.Context, time.Time) (update, time.Duration) {
	srv := m.srv.Load()
	if srv == nil {
		return nil, 0
	}
	if tok := install.ReadToken(m.o.Home); tok != "" && tok != srv.CurrentToken() {
		srv.Rotate(tok)
		fmt.Fprintln(m.o.Out, "token rotated: old cookies, terminals and event streams are closed")
	}
	return nil, 0
}

// ---- the ingest and its dispatcher ----

// ingest folds hook and status-line bodies into the projects' models as they arrive.
func (m *Machine) ingest(ctx context.Context, hooks, status <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case body := <-hooks:
			ev, err := signals.ParseHook(body)
			if err != nil {
				m.malformed.Add(1)
				continue
			}
			ev.Cwd = signals.ResolvePath(ev.Cwd)
			m.applyHook(ev)
		case body := <-status:
			st, err := signals.ParseStatus(body)
			if err != nil {
				m.malformed.Add(1)
				continue
			}
			st.Cwd = signals.ResolvePath(st.Cwd)
			m.applyStatus(st)
		}
	}
}

// route finds the project a hook or status body belongs to: first by its session id
// (a lane record of the project, or a session already placed), then by the deepest
// worktree of any project that holds its cwd. Nil: no project's.
func (m *Machine) route(sessionID, cwd string) *Runtime {
	projects := m.list()
	if sessionID != "" {
		if r := m.bound[sessionID]; r != nil {
			if !r.removed.Load() {
				return r
			}
			delete(m.bound, sessionID) // its project was removed live (PANEL-22)
		}
		for _, r := range projects {
			owns := false
			if r.registry != nil {
				for _, rec := range r.registry.List() {
					owns = owns || rec.SessionID == sessionID
				}
			}
			if !owns {
				r.hub.Read(func(md *state.Model, _ time.Time) { owns = md.OwnsSession(sessionID) })
			}
			if owns {
				m.bind(sessionID, r)
				return r
			}
		}
	}
	var best *Runtime
	bestLen := -1
	for _, r := range projects {
		d := -1
		r.hub.Read(func(md *state.Model, _ time.Time) { d = md.WorktreeDepth(cwd) })
		if d > bestLen {
			best, bestLen = r, d
		}
	}
	if best != nil && sessionID != "" {
		m.bind(sessionID, best)
	}
	return best
}

func (m *Machine) bind(sessionID string, r *Runtime) {
	if len(m.bound) >= maxBound {
		m.bound = map[string]*Runtime{}
	}
	m.bound[sessionID] = r
}

// applyHook hands a hook to its project. One that is no project's goes to every
// project, which each counts as from another project and re-reads its worktrees
// early (the cwd may be a worktree just added).
func (m *Machine) applyHook(ev signals.HookEvent) {
	r := m.route(ev.SessionID, ev.Cwd)
	targets := m.list()
	if r != nil {
		targets = []*Runtime{r}
		r.hookSeen(ev)
		// A prompt, or a finished turn, means the session has a conversation to
		// --resume. Hooks can be dropped, so busy in `claude agents` counts too.
		if (ev.Event == "UserPromptSubmit" || ev.Event == "Stop") && r.registry != nil {
			_, _ = r.registry.MarkConversation(ev.SessionID)
		}
	}
	for _, t := range targets {
		kept := true
		t.hub.Update(func(md *state.Model, now time.Time) { kept = md.ApplyHook(ev, now) })
		if !kept {
			t.kickWorktrees()
		}
	}
}

// applyStatus folds a status post: its quota into every project (the quota is the
// account's), the rest into its own project only.
func (m *Machine) applyStatus(st signals.StatusPayload) {
	r := m.route(st.SessionID, st.Cwd)
	for _, t := range m.list() {
		if r == nil || t == r {
			t.hub.Update(func(md *state.Model, now time.Time) { md.ApplyStatus(st, now) })
		} else {
			t.hub.Update(func(md *state.Model, now time.Time) { md.FoldQuota(st, now) })
		}
	}
}
