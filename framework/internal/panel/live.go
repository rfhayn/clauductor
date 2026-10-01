package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// PANEL-22: the projects a running panel serves follow projects.json. A project added
// (from the page's "Add a project…", or `clauductor panel add`) starts its runtime at
// once: its sources, its lane manager and its hub, wired into the server, the ingest
// and the menu. A project removed stops its runtime and waits for every one of its
// goroutines; its lanes are tmux's and keep running. One reconcile does both, for the
// page's requests and for the watch on projects.json alike, so the page and the CLI
// share the registry code (install.AddProject, install.RemoveProject) and its rules.

// liveSet is the panel's live project registry.
type liveSet struct {
	// mu serialises every change: a page request, the watch, a trust. It is never
	// taken by a runtime's own goroutines, so stopping one under it cannot deadlock.
	mu      sync.Mutex
	ctx     context.Context
	o       Options
	clk     clock.Clock
	ticks   Ticks
	m       *Machine
	srv     *web.Server
	sums    *web.Summaries
	primary string // the project named on the command line: its --tmux-socket override holds
	// startDef is the default the panel started with (set before anything runs, then
	// only read): its Host names count untrusted, as they always have.
	startDef *Runtime
	owner    install.PanelOwner

	rts    map[string]*liveRT
	failed map[string]web.ProjectSummary
	sig    string // projects.json's signature at the last reconcile
	order  []string
	regErr string // the last error reading projects.json, said once
}

// liveRT is one served project's runtime and what stops it.
type liveRT struct {
	r      *Runtime
	entry  config.ProjectEntry
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// buildRuntime makes a loaded project's runtime: its trust, lane manager, model and
// hub. Nothing runs until launch.
func buildRuntime(o Options, clk clock.Clock, ticks Ticks, p loadedProject, trustNow bool) *Runtime {
	trust := checkConfigTrust(o, p.entry.Root, p.cfgPath, p.raw, trustNow)
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
	r.raw = p.raw
	if lm != nil {
		lm.Trusted = r.trusted // PANEL-20: worktree_setup and worktree_teardown are the config's commands
	}
	return r
}

// wire connects a runtime to the server and the menu, and returns the server's view
// of it.
func (ls *liveSet) wire(r *Runtime) *web.Project {
	r.srv.Store(ls.srv)
	r.hub.OnPush = func(v state.View) { ls.sums.Set(web.Summarize(r.id, v)) }
	r.onTrusted = ls.refreshHosts
	if r.lanes != nil {
		// A lane action re-reads the worktrees at once, not through kickWorktrees'
		// throttle: lanes started together each add a worktree, and one read skipped
		// leaves their sessions' first events with no worktree to bind to (PANEL-21).
		// The kick channel coalesces, so a burst costs at most one extra read.
		r.lanes.Changed = func() { kick(r.kickTmux); kick(r.kickWT); kick(r.kickAgents) }
		id := r.id
		r.lanes.Stopped = func(lane string) { ls.srv.CloseTerminalsIn(id, lane) }
	}
	return &web.Project{ID: r.id, Name: r.cfg.Name, Hub: r.hub, Lanes: r.lanes, Orch: r.orchestration(), Refresh: r.refreshAll,
		Metrics: r.metricsReport}
}

// launch starts a runtime's hub and sources under a context of its own, every
// goroutine counted, so stop can wait for all of them.
func (ls *liveSet) launch(r *Runtime, e config.ProjectEntry) *liveRT {
	ctx, cancel := context.WithCancel(ls.ctx)
	lr := &liveRT{r: r, entry: e, cancel: cancel}
	start := func(f func()) { lr.wg.Add(1); go func() { defer lr.wg.Done(); f() }() }
	start(func() { r.hub.Run(ctx) })
	r.start(ctx, start)
	ls.rts[r.id] = lr
	return lr
}

// stop stops serving a project: the ingest, the routes and the menu forget it, then
// its goroutines are cancelled and waited for. ls.mu is held.
func (ls *liveSet) stop(id string) { ls.halt(id, false) }

// restartStop stops a project that is started again at once under the same id (a
// trust reload, a changed root or config path): its menu entry stays, so a page on it
// reconnects to the new runtime instead of moving to the default; startEntry replaces
// the entry.
func (ls *liveSet) restartStop(id string) { ls.halt(id, true) }

func (ls *liveSet) halt(id string, keepMenu bool) {
	lr := ls.rts[id]
	if lr == nil {
		return
	}
	delete(ls.rts, id)
	lr.r.removed.Store(true)
	ls.m.removeProject(lr.r)
	// The menu first, so the project's own open streams carry the menu without it
	// (the page then moves to the default) before they end.
	if !keepMenu {
		ls.sums.Remove(id)
	}
	ls.srv.RemoveProject(id)
	lr.cancel()
	lr.wg.Wait()
	fmt.Fprintf(ls.o.Out, "project %s: no longer served (its lanes keep running in tmux)\n", id)
}

// stopAll stops every runtime (the panel is exiting).
func (ls *liveSet) stopAll() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, lr := range ls.rts {
		lr.cancel()
	}
	for _, lr := range ls.rts {
		lr.wg.Wait()
	}
}

// canStart says whether a registered entry would load now, its socket free of every
// other served project's, without starting anything. A restart checks it first, so
// it never stops a runtime for one that cannot start. ls.mu is held.
func (ls *liveSet) canStart(e config.ProjectEntry) error {
	p, err := loadProject(ls.ctx, ls.o, e, e.ID == ls.primary)
	if err != nil {
		return err
	}
	for other, olr := range ls.rts {
		if other != e.ID && olr.r.cfg.Socket() == p.cfg.Socket() {
			return fmt.Errorf("its tmux socket %q is %s's", p.cfg.Socket(), other)
		}
	}
	return nil
}

// startEntry loads a registered project and serves it, or records why it cannot be.
// ls.mu is held.
func (ls *liveSet) startEntry(e config.ProjectEntry, isDefault bool) error {
	p, err := loadProject(ls.ctx, ls.o, e, e.ID == ls.primary)
	if err == nil {
		for id, lr := range ls.rts {
			if lr.r.cfg.Socket() == p.cfg.Socket() {
				err = fmt.Errorf("tmux socket %q is %s's already; give this project its own tmux_socket", p.cfg.Socket(), id)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(ls.o.Out, "project %s not loaded: %v\n", e.ID, err)
		f := web.ProjectSummary{ID: e.ID, Name: e.ID, Error: signals.Clip(err.Error(), 200), Default: isDefault}
		ls.failed[e.ID] = f
		ls.sums.Add(f)
		return err
	}
	delete(ls.failed, e.ID)
	r := buildRuntime(ls.o, ls.clk, ls.ticks, p, false)
	proj := ls.wire(r)
	ls.m.addProject(r) // sets r.machine before any of its sources runs
	ls.launch(r, e)
	ls.srv.AddProject(proj)
	s := web.Summarize(r.id, r.hub.View())
	s.Default = isDefault
	ls.sums.Add(s)
	fmt.Fprintf(ls.o.Out, "project %s: serving %s (%s)\n", e.ID, r.cfg.Name, e.Root)
	return nil
}

// pollRegistry is the watch on projects.json: a change (`panel add`, `panel remove`,
// another page) is reconciled within a tick.
func (ls *liveSet) pollRegistry(context.Context, time.Time) (update, time.Duration) {
	if pathSignature(config.ProjectsPath(ls.o.Home)) == ls.signature() {
		return nil, 0
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	_ = ls.reconcile()
	return nil, 0
}

func (ls *liveSet) signature() string {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.sig
}

// reconcile makes the served projects what projects.json lists: it stops the ones it
// no longer lists, starts the new ones, follows its default and its order. A project
// that cannot load is a menu item with the reason, as at start. The last served
// project is never stopped: the panel always serves one. ls.mu is held.
func (ls *liveSet) reconcile() error {
	if ls.o.Only {
		return nil
	}
	ls.sig = pathSignature(config.ProjectsPath(ls.o.Home))
	reg, err := config.LoadProjects(ls.o.Home)
	if err != nil {
		if msg := err.Error(); msg != ls.regErr {
			ls.regErr = msg
			fmt.Fprintf(ls.o.Out, "projects.json not followed: %v\n", err)
		}
		return err
	}
	ls.regErr = ""
	listed := map[string]config.ProjectEntry{}
	for _, e := range reg.Projects {
		listed[e.ID] = e
	}
	for id, lr := range ls.rts {
		e, ok := listed[id]
		if ok && e.Root == lr.entry.Root && e.ConfigPath() == lr.entry.ConfigPath() {
			continue
		}
		if ok { // the same id, another root or config: started again below
			if err := ls.canStart(e); err != nil && len(ls.rts) == 1 {
				// Never trade the last served project for one that will not load.
				fmt.Fprintf(ls.o.Out, "projects.json moves %s to %s, which does not load (%v); the panel keeps serving it as it was until it restarts\n", id, e.Root, err)
				continue
			}
			ls.restartStop(id)
			continue
		}
		if len(ls.rts) == 1 {
			fmt.Fprintf(ls.o.Out, "projects.json no longer lists %s, the last project this panel serves; it keeps serving it until it restarts\n", id)
			continue
		}
		ls.stop(id)
	}
	for id := range ls.failed {
		if _, ok := listed[id]; !ok {
			delete(ls.failed, id)
			ls.sums.Remove(id)
		}
	}
	for _, e := range reg.Projects {
		if ls.rts[e.ID] != nil {
			continue
		}
		if _, failed := ls.failed[e.ID]; failed {
			// It failed before; the file changed, so try again (its config may be fixed).
			delete(ls.failed, e.ID)
			ls.sums.Remove(e.ID)
		}
		_ = ls.startEntry(e, false)
	}
	ls.order = nil
	for _, e := range reg.Projects {
		ls.order = append(ls.order, e.ID)
	}
	ls.sums.Reorder(ls.order)
	ls.settleDefault(reg.Default)
	return nil
}

// settle re-chooses the default, the Host names and the owner record from what is
// served now and the registry's default. ls.mu is held.
func (ls *liveSet) settle() {
	want := ""
	if reg, err := config.LoadProjects(ls.o.Home); err == nil {
		want = reg.Default
	} else if d := ls.m.defRT(); d != nil {
		want = d.id
	}
	ls.settleDefault(want)
}

// settleDefault: the registry's default if served, else the first served in the
// registry's order; then the Host names and the owner record. ls.mu is held.
func (ls *liveSet) settleDefault(want string) {
	def := ls.rts[want]
	for _, id := range ls.order {
		if def == nil && ls.rts[id] != nil {
			def = ls.rts[id]
		}
	}
	if def == nil { // only the kept last project is left
		for _, lr := range ls.rts {
			def = lr
		}
	}
	if def != nil && def.r != ls.m.defRT() {
		ls.m.setDefault(def.r)
		ls.srv.SetDefault(def.r.id)
		fmt.Fprintf(ls.o.Out, "default project: %s\n", def.r.id)
	}
	if def != nil {
		ls.sums.SetDefault(def.r.id)
	}
	ls.refreshHosts()
	ls.writeOwner()
}

// refreshHosts sets the Host names: every trusted project's, and the startup default's
// even untrusted (before PANEL-16 it was the only project, and its names always
// counted). A project that becomes the default live adds its names only once trusted,
// so removing a trusted default never lets an untrusted config add a name.
func (ls *liveSet) refreshHosts() {
	var names []string
	for _, r := range ls.m.list() {
		if r.trusted() || r == ls.startDef {
			names = append(names, r.cfg.HostNames...)
		}
	}
	ls.srv.SetHostNames(names)
}

// writeOwner records the projects served in owner.json: `panel add` and `panel
// remove` wait for it to say the change was taken.
func (ls *liveSet) writeOwner() {
	def := ls.m.defRT()
	if def == nil {
		return
	}
	o := ls.owner
	o.Project, o.Name = def.root, def.cfg.Name
	o.Projects = nil
	for _, r := range ls.m.list() {
		o.Projects = append(o.Projects, r.root)
	}
	sort.Strings(o.Projects)
	if err := install.UpdatePanelOwner(ls.o.Home, o); err != nil {
		fmt.Fprintf(ls.o.Out, "cannot update %s: %v\n", "owner.json", err)
	}
}

// ---- the page's routes (web.ProjectAdmin) ----

func lerr(status int, code, format string, a ...any) *lanes.LaneError {
	return &lanes.LaneError{Status: status, Code: code, Msg: fmt.Sprintf(format, a...)}
}

// refusal maps what Inspect and the registry refuse to the page's vocabulary.
func refusal(err error) *lanes.LaneError {
	var r *install.Refusal
	if errors.As(err, &r) {
		return lerr(http.StatusUnprocessableEntity, r.Code, "%s", r.Msg)
	}
	if errors.Is(err, install.ErrConfigChanged) {
		return lerr(http.StatusConflict, "config-changed", "%v", err)
	}
	if errors.Is(err, install.ErrConfigExists) {
		return lerr(http.StatusConflict, "config-exists", "%v", err)
	}
	return lerr(http.StatusUnprocessableEntity, "refused", "%v", err)
}

func (ls *liveSet) onlyRefused() *lanes.LaneError {
	if ls.o.Only {
		return lerr(http.StatusConflict, "only", "this panel serves one project (--only); restart it without --only to add or remove projects")
	}
	return nil
}

func (ls *liveSet) inspect(ctx context.Context, path string) (install.Candidate, *lanes.LaneError) {
	c, err := install.Inspect(ctx, install.InspectOptions{Home: ls.o.Home, Path: path, Run: ls.o.Runner})
	if err != nil {
		return c, refusal(err)
	}
	return c, nil
}

// validation is Validate's answer: what adding would do, or why it cannot be added.
// A refusal is an answer to the question asked, not a failed request, so it comes
// back with 200 (the page checks as someone types; every refusal would otherwise be a
// failed request in the browser's console).
type validation struct {
	install.Candidate
	Refused *refused `json:"refused,omitempty"`
}

type refused struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// Validate serves POST /api/projects/validate.
func (ls *liveSet) Validate(ctx context.Context, path string) (any, *lanes.LaneError) {
	if e := ls.onlyRefused(); e != nil {
		return nil, e
	}
	c, e := ls.inspect(ctx, path)
	if e != nil {
		if e.Status >= 500 {
			return nil, e
		}
		return validation{Candidate: c, Refused: &refused{Code: e.Code, Error: e.Msg}}, nil
	}
	return validation{Candidate: c}, nil
}

// initPlan is what the page shows of `panel init`.
type initPlan struct {
	Root  string   `json:"root"`
	Path  string   `json:"path"`
	Body  string   `json:"body"`
	Notes []string `json:"notes"`
}

func (ls *liveSet) plan(ctx context.Context, path string) (install.InitResult, *lanes.LaneError) {
	if e := ls.onlyRefused(); e != nil {
		return install.InitResult{}, e
	}
	c, e := ls.inspect(ctx, path)
	if e != nil {
		return install.InitResult{}, e
	}
	if c.HasConfig {
		return install.InitResult{}, lerr(http.StatusConflict, "config-exists", "%s has a panel config already", c.Root)
	}
	res, err := install.PlanInit(ctx, ls.o.Runner, c.Root)
	if err != nil {
		return res, refusal(err)
	}
	return res, nil
}

// InitPreview serves POST /api/projects/init-preview: it writes nothing.
func (ls *liveSet) InitPreview(ctx context.Context, path string) (any, *lanes.LaneError) {
	res, e := ls.plan(ctx, path)
	if e != nil {
		return nil, e
	}
	return initPlan{Root: filepath.Dir(filepath.Dir(res.Path)), Path: res.Path, Body: string(res.Body), Notes: res.Notes}, nil
}

// Init serves POST /api/projects/init: it writes the plan, never over a file.
func (ls *liveSet) Init(ctx context.Context, path string) (any, *lanes.LaneError) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	res, e := ls.plan(ctx, path)
	if e != nil {
		return nil, e
	}
	if err := install.WriteInit(res); err != nil {
		return nil, refusal(err)
	}
	fmt.Fprintf(ls.o.Out, "wrote %s from the page (untrusted until it is trusted)\n", res.Path)
	return initPlan{Root: filepath.Dir(filepath.Dir(res.Path)), Path: res.Path, Body: string(res.Body), Notes: res.Notes}, nil
}

// added is what the page learns once a project is added.
type added struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Root    string `json:"root"`
	Trusted bool   `json:"trusted"`
	Live    bool   `json:"live"`
	Error   string `json:"error,omitempty"`
}

// Add serves POST /api/projects/add: "Trust and add" or "Add without trusting".
func (ls *liveSet) Add(ctx context.Context, req web.AddProjectRequest) (any, *lanes.LaneError) {
	if e := ls.onlyRefused(); e != nil {
		return nil, e
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c, e := ls.inspect(ctx, req.Path)
	if e != nil {
		return nil, e
	}
	switch {
	case !c.HasConfig:
		return nil, lerr(http.StatusConflict, "no-config", "%s has no panel config: create one first", c.Root)
	case c.ConfigError != "":
		return nil, lerr(http.StatusConflict, "bad-config", "its config does not load: %s", c.ConfigError)
	case req.Trust && req.Hash != c.Hash:
		return nil, refusal(install.ErrConfigChanged)
	}
	res, err := install.AddProject(ctx, install.AddOptions{Home: ls.o.Home, Project: c.Root, Run: ls.o.Runner, Now: ls.clk.Now(), Strict: true})
	if err != nil {
		return nil, refusal(err) // nothing recorded: no entry, no trust
	}
	if req.Trust {
		// Trusted before the runtime starts (ls.mu keeps the watch from starting it
		// first), so its commands are on from its first poll; exactly the bytes the
		// report showed. Refused, the add is undone: a refused request changes nothing.
		if _, err := install.TrustExact(ls.o.Home, c.Root, c.ConfigPath, req.Hash); err != nil {
			if _, rerr := install.RemoveProject(ls.o.Home, res.Entry.ID, true); rerr != nil {
				fmt.Fprintf(ls.o.Out, "cannot undo the add of %s: %v\n", res.Entry.ID, rerr)
			}
			return nil, refusal(err)
		}
	}
	fmt.Fprintf(ls.o.Out, "registered %s as project %q from the page (%s)\n", res.Entry.Root, res.Entry.ID,
		map[bool]string{true: "trusted", false: "untrusted"}[req.Trust || c.Trusted])
	_ = ls.reconcile()
	out := added{ID: res.Entry.ID, Name: res.Cfg.Name, Root: res.Entry.Root}
	if lr := ls.rts[res.Entry.ID]; lr != nil {
		out.Live, out.Trusted = true, lr.r.trusted()
	} else if f, ok := ls.failed[res.Entry.ID]; ok {
		out.Error = f.Error
	}
	return out, nil
}

// removePlan is what the remove confirmation lists.
type removePlan struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Root       string     `json:"root"`
	ConfigPath string     `json:"configPath"`
	Lanes      []planLane `json:"lanes"`
	Default    bool       `json:"default"`
	// NextDefault is the project that becomes the default when this one goes.
	NextDefault     string `json:"nextDefault,omitempty"`
	NextDefaultName string `json:"nextDefaultName,omitempty"`
	// Refused says why it cannot be removed now ("" when it can).
	Refused string `json:"refused,omitempty"`
	Removed bool   `json:"removed,omitempty"`
}

type planLane struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (ls *liveSet) removePlan(id string) (removePlan, *lanes.LaneError) {
	reg, err := config.LoadProjects(ls.o.Home)
	if err != nil {
		return removePlan{}, lerr(http.StatusInternalServerError, "registry", "%v", err)
	}
	e := reg.Find(id)
	if e == nil || e.ID != id {
		return removePlan{}, lerr(http.StatusNotFound, "no-project", "no registered project %s", id)
	}
	p := removePlan{ID: id, Name: id, Root: e.Root, ConfigPath: e.ConfigPath(), Lanes: []planLane{}, Default: reg.Default == id}
	seen := map[string]bool{}
	if lr := ls.rts[id]; lr != nil {
		p.Name = lr.r.cfg.Name
		var terms []state.TermLaneView
		lr.r.hub.Read(func(m *state.Model, now time.Time) { terms = m.TerminalViews(now) })
		for _, t := range terms {
			if t.Running || t.Registered {
				p.Lanes = append(p.Lanes, planLane{ID: t.ID, Status: t.Status})
				seen[t.ID] = true
			}
		}
	}
	if r, err := lanes.OpenRegistry(ls.o.Home, e.Root); err == nil {
		for _, rec := range r.List() {
			if !seen[rec.ID] {
				p.Lanes = append(p.Lanes, planLane{ID: rec.ID, Status: "registered"})
			}
		}
	}
	served := 0
	for _, x := range reg.Projects {
		if x.ID != id && ls.rts[x.ID] != nil {
			served++
			if p.Default && p.NextDefault == "" {
				p.NextDefault, p.NextDefaultName = x.ID, ls.rts[x.ID].r.cfg.Name
			}
		}
	}
	switch {
	case len(p.Lanes) > 0:
		p.Refused = fmt.Sprintf("%s has %d lane(s). Stop or close them first: removing never stops a lane.", p.Name, len(p.Lanes))
	case served == 0 && ls.rts[id] != nil:
		p.Refused = "it is the only project this panel serves: add another first"
	}
	return p, nil
}

// Remove serves POST /api/projects/{id}/remove.
func (ls *liveSet) Remove(ctx context.Context, id string, dryRun bool) (any, *lanes.LaneError) {
	if e := ls.onlyRefused(); e != nil {
		return nil, e
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	p, e := ls.removePlan(id)
	if e != nil || dryRun {
		return p, e
	}
	if p.Refused != "" {
		return nil, lerr(http.StatusConflict, "refused", "%s", p.Refused)
	}
	if _, err := install.RemoveProject(ls.o.Home, id, false); err != nil {
		return nil, lerr(http.StatusConflict, "refused", "%v", err)
	}
	fmt.Fprintf(ls.o.Out, "removed project %s from the page (nothing on disk was deleted)\n", id)
	_ = ls.reconcile()
	p.Removed = true
	return p, nil
}

// trustReport is what Trust config… shows: what `panel trust` prints.
type trustReport struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Hash    string   `json:"hash"`
	Runs    []string `json:"runs"`
	Trusted bool     `json:"trusted"`
	Prev    string   `json:"prev,omitempty"`
	// Reloaded: the file differed from the one the panel had loaded, so the project
	// was started again with the bytes just trusted.
	Reloaded bool `json:"reloaded,omitempty"`
}

// Trust serves POST /api/projects/{id}/trust. The report renders the config on disk
// (it runs nothing); trusting records exactly the bytes whose hash the page names.
func (ls *liveSet) Trust(ctx context.Context, id string, dryRun bool, hash string) (any, *lanes.LaneError) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	lr := ls.rts[id]
	if lr == nil {
		if f, ok := ls.failed[id]; ok {
			return nil, lerr(http.StatusConflict, "bad-config", "%s cannot load: %s", id, f.Error)
		}
		return nil, lerr(http.StatusNotFound, "no-project", "no such project: %s", id)
	}
	path := lr.entry.ConfigPath()
	cfg, raw, err := config.LoadConfigRaw(path)
	if err != nil {
		return nil, lerr(http.StatusConflict, "bad-config", "its config does not load: %v", err)
	}
	h := install.ConfigHash(raw)
	tv, _ := install.CheckTrust(ls.o.Home, lr.entry.Root, path, h, false)
	rep := trustReport{ID: id, Name: cfg.Name, Path: path, Hash: h, Runs: append([]string{}, cfg.RunList()...), Trusted: tv.Trusted, Prev: tv.Prev}
	if dryRun {
		return rep, nil
	}
	if hash != h {
		return nil, refusal(install.ErrConfigChanged)
	}
	if _, err := install.TrustExact(ls.o.Home, lr.entry.Root, path, hash); err != nil {
		return nil, refusal(err)
	}
	rep.Trusted = true
	fmt.Fprintf(ls.o.Out, "project %s: config %s sha256 %s trusted from the page\n", id, path, config.ShortHash(hash))
	if hash == install.ConfigHash(lr.r.raw) {
		if up := lr.r.markTrusted("trusted from the page"); up != nil {
			lr.r.hub.Update(up)
		}
		return rep, nil
	}
	// The file is not the one the panel loaded: serve the bytes just trusted.
	// First check that it loads, so a config that would not (its socket is another
	// project's, git fails) never stops the runtime that serves now.
	e := lr.entry
	if err := ls.canStart(e); err != nil {
		return nil, lerr(http.StatusConflict, "bad-config", "trusted, but it does not load as it is now, so the panel keeps serving the config it loaded: %v", err)
	}
	wasDefault := ls.m.defRT() == lr.r
	ls.restartStop(id)
	err = ls.startEntry(e, wasDefault)
	ls.sums.Reorder(ls.order)
	ls.settle()
	if err != nil {
		return nil, lerr(http.StatusConflict, "bad-config", "%v", err)
	}
	rep.Reloaded = true
	return rep, nil
}
