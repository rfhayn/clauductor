package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/lanes"
	"github.com/rfhayn/stridal/internal/panel/signals"
)

// AddOptions registers a project with the machine's panel (PANEL-16).
type AddOptions struct {
	Home    string
	Project string // any path inside the project's main worktree
	Config  string // "" for <root>/.stridal/panel.json
	ID      string // "" for the slug of the config's name
	Run     signals.Runner
	Now     time.Time
	// Strict refuses a linked worktree (`panel add`); otherwise it registers the
	// main worktree the linked one belongs to (a bare `panel` run inside it).
	Strict bool
	// MakeDefault makes the project the default: the one legacy routes and a bare
	// page open on.
	MakeDefault bool
}

// AddResult is what AddProject did.
type AddResult struct {
	Entry config.ProjectEntry
	Cfg   *config.Config
	Added bool // false: it was registered already
}

// AddPlan is what CheckAdd found AddProject would do: the registry as it would be saved, and the
// entry registered or to register. It holds no file's content but the registry's, in memory.
type AddPlan struct {
	reg     *config.Projects
	entry   config.ProjectEntry
	cfg     *config.Config
	added   bool // a new entry; false: registered already
	changed bool // the registry differs from the file (a new entry, config or default)
}

// CheckAdd runs every check AddProject makes before it saves, and writes nothing: it returns
// AddProject's own refusal, or the plan AddProject then saves. AddProject calls it, so a caller
// that asks "would this be refused" (stridal doctor) and the command itself cannot read the
// rule two ways.
func CheckAdd(ctx context.Context, o AddOptions) (AddPlan, error) {
	if o.Run == nil {
		o.Run = signals.ExecRunner
	}
	root := signals.ResolvePath(o.Project)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return AddPlan{}, fmt.Errorf("%s is not a directory", o.Project)
	}
	wts, err := signals.ReadWorktrees(ctx, o.Run, root)
	if err != nil {
		return AddPlan{}, fmt.Errorf("%s: %w", root, err)
	}
	if len(wts) == 0 {
		return AddPlan{}, fmt.Errorf("%s: git lists no worktree", root)
	}
	// Two entries for one repository would both claim its worktrees and its lanes:
	// a project is its main worktree, and a linked one (or a symlink to either)
	// names the same project.
	if main := wts[0].Path; main != root {
		if i := signals.MatchWorktree(wts, root); i > 0 && o.Strict {
			return AddPlan{}, fmt.Errorf("%s is a linked worktree of %s; add that instead (a project is its main worktree)", root, main)
		}
		root = main
	}
	reg, err := config.LoadProjects(o.Home)
	if err != nil {
		return AddPlan{}, err
	}
	cfgPath := ""
	if o.Config != "" {
		cfgPath = signals.ResolvePath(o.Config)
	}
	if e := reg.Find(root); e != nil {
		changed := false
		if cfgPath != "" && e.Config != cfgPath {
			e.Config, changed = cfgPath, true
		}
		if o.MakeDefault && reg.Default != e.ID {
			reg.Default, changed = e.ID, true
		}
		cfg, err := config.LoadConfig(e.ConfigPath())
		if err != nil {
			return AddPlan{}, err
		}
		return AddPlan{reg: reg, entry: *e, cfg: cfg, changed: changed}, nil
	}
	// One repository, one entry, by git's common dir too (PANEL-22): a second path to
	// it that resolves elsewhere would claim the same worktrees and lanes.
	if common, err := commonDir(ctx, o.Run, root); err == nil {
		if id := registeredAs(ctx, o.Run, reg, root, common); id != "" {
			return AddPlan{}, fmt.Errorf("%s is the repository registered already as %q", root, id)
		}
	}
	e := config.ProjectEntry{Root: root, Config: cfgPath, Added: o.Now.Unix()}
	cfg, err := config.LoadConfig(e.ConfigPath())
	if err != nil {
		return AddPlan{}, err
	}
	e.ID = o.ID
	if e.ID == "" {
		e.ID = reg.Slug(cfg.Name)
	}
	if !config.ProjectIDRe.MatchString(e.ID) {
		return AddPlan{}, fmt.Errorf("project id %q must match %s", e.ID, config.ProjectIDRe)
	}
	if reg.Find(e.ID) != nil {
		return AddPlan{}, fmt.Errorf("project id %q is taken; pass --id", e.ID)
	}
	e.TmuxSocket = reg.NewSocket(e.ID)
	// Two projects on one tmux server would each see the other's lanes as strays,
	// and a lane name used in both would refuse to start in the second.
	sock := e.Socket(cfg)
	if other := socketOwner(reg, sock, ""); other != "" {
		return AddPlan{}, fmt.Errorf("tmux socket %q is %s's already; give this project its own tmux_socket in %s", sock, other, e.ConfigPath())
	}
	reg.Projects = append(reg.Projects, e)
	if reg.Default == "" || o.MakeDefault {
		reg.Default = e.ID
	}
	return AddPlan{reg: reg, entry: e, cfg: cfg, added: true, changed: true}, nil
}

// AddProject registers a project: one git common dir, named by its main worktree. It
// loads and validates the config, but never trusts it: trusting is `panel trust`.
func AddProject(ctx context.Context, o AddOptions) (AddResult, error) {
	plan, err := CheckAdd(ctx, o)
	if err != nil {
		return AddResult{}, err
	}
	res := AddResult{Entry: plan.entry, Cfg: plan.cfg, Added: plan.added}
	if plan.changed {
		return res, config.SaveProjects(o.Home, plan.reg)
	}
	return res, nil
}

// ConfigPathOf is the config a --project/--config pair names (PANEL-42, design.md D7):
// --config when given; else the registry's config for a project it holds, which may
// sit outside the repository (a sidecar); else <root>/.stridal/panel.json. The
// registry is the machine's choice, so a repository's own file never displaces it.
func ConfigPathOf(home, project, cfg string) (string, error) {
	if cfg != "" {
		return cfg, nil
	}
	root := signals.ResolvePath(project)
	reg, err := config.LoadProjects(home)
	if err != nil {
		return "", err
	}
	if e := reg.Find(root); e != nil {
		return e.ConfigPath(), nil
	}
	return config.OwnConfigPath(root), nil
}

// ConfigOutside reports whether a project's config resolves outside its repository
// (design.md D5, D7): a sidecar, or any --config elsewhere. Both are resolved through
// symlinks as far as they exist.
func ConfigOutside(e config.ProjectEntry) bool {
	return configOutside(e.Root, e.ConfigPath())
}

// configOutside is ConfigOutside for a root and a config path: what trust decides on,
// since its callers hold paths, not an entry (design.md D5). Both sides go through
// one resolver: signals.ResolvePath gives up on a root that does not exist, and would
// leave its symlinked ancestor (/var for /private/var) unresolved beside the config's.
func configOutside(root, cfgPath string) bool {
	return !config.Within(config.ResolveExisting(cfgPath), config.ResolveExisting(root))
}

// OutsideConfig is the entry's config path when it is outside its repository, else
// "": what the project menu and the remove confirmation name (design.md D6, D7).
func OutsideConfig(e config.ProjectEntry) string {
	if ConfigOutside(e) {
		return e.ConfigPath()
	}
	return ""
}

// ShellWord is s as one POSIX shell word, for a command line a person copies and runs:
// bare when it holds only characters no shell reads, else single-quoted (shq). The
// page's copy is project-config.js's shellWord.
func ShellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return shq(s)
}

// RemoveProject unregisters a project. It never stops a lane and keeps the lane
// registry, so the project's lanes come back if it is added again; while lanes are
// registered it refuses unless force.
func RemoveProject(home, idOrPath string, force bool) (config.ProjectEntry, error) {
	reg, err := config.LoadProjects(home)
	if err != nil {
		return config.ProjectEntry{}, err
	}
	e := reg.Find(idOrPath)
	if e == nil {
		e = reg.Find(signals.ResolvePath(idOrPath))
	}
	if e == nil {
		return config.ProjectEntry{}, fmt.Errorf("no registered project %q (`stridal panel list` names them)", idOrPath)
	}
	gone := *e
	if n := laneCount(home, gone.Root); n > 0 && !force {
		return gone, fmt.Errorf("%s has %d registered lane(s), which keep running in tmux; stop them first, or pass --force to remove it anyway (its lanes are kept, not stopped)", gone.ID, n)
	}
	kept := reg.Projects[:0]
	for _, p := range reg.Projects {
		if p.ID != gone.ID {
			kept = append(kept, p)
		}
	}
	reg.Projects = kept
	if reg.Default == gone.ID {
		reg.Default = ""
		if len(kept) > 0 {
			reg.Default = kept[0].ID
		}
	}
	// Catch-up is keyed by root (OPS-118 D2): a repository registered again at this
	// root starts with it off. Cleared first, so a failure leaves the project
	// registered with catch-up as it was, never a root listed that nothing serves.
	if err := config.SetCatchUp(home, gone.Root, false); err != nil {
		return gone, fmt.Errorf("%s stays registered: its catch-up could not be cleared: %v", gone.ID, err)
	}
	return gone, config.SaveProjects(home, reg)
}

// laneCount is how many lanes a project's registry lists (0 when it has none).
func laneCount(home, root string) int {
	r, err := lanes.OpenRegistry(home, root)
	if err != nil {
		return 0
	}
	return len(r.List())
}

// ListProjects prints every registered project: id, name, root, socket, trust and
// lanes. A project whose config does not load says why.
func ListProjects(w io.Writer, home string) error {
	reg, err := config.LoadProjects(home)
	if err != nil {
		return err
	}
	if len(reg.Projects) == 0 {
		fmt.Fprintln(w, "No projects registered. `stridal panel add --project <path>` adds one; a bare `stridal panel` in a repository adds that one.")
		return nil
	}
	for _, e := range reg.Projects {
		mark := " "
		if e.ID == reg.Default {
			mark = "*"
		}
		name, trust := "", "untrusted"
		cfg, raw, cerr := config.LoadConfigRaw(e.ConfigPath())
		if cerr != nil {
			name, trust = "(config does not load: "+signals.Clip(cerr.Error(), 80)+")", "-"
		} else {
			name = cfg.Name
			if TrustedNow(home, e.Root, e.ConfigPath(), ConfigHash(raw)) {
				trust = "trusted"
			}
		}
		fmt.Fprintf(w, "%s %-16s %s\n    root %s\n    config %s\n", mark, e.ID, name, e.Root, configLine(e))
		fmt.Fprintf(w, "    socket %s · %s · %d lane(s)\n", e.Socket(cfg), trust, laneCount(home, e.Root))
	}
	fmt.Fprintln(w, "* the default: legacy routes and a page with no ?p= open on it.")
	fmt.Fprintln(w, "Remote control: "+remoteLine(home))
	return nil
}

// configLine is `panel list`'s config: its path, marked when it is outside the
// repository, and, when the repository has a .stridal/panel.json of its own as
// well, that the panel does not read that one (design.md D7).
func configLine(e config.ProjectEntry) string {
	if !ConfigOutside(e) {
		return e.ConfigPath()
	}
	s := e.ConfigPath() + " (outside the repository)"
	own := config.OwnConfigPath(e.Root)
	if _, err := os.Stat(own); err == nil {
		s += "\n    " + own + " is not read: the registry names the config above"
	}
	return s
}

// remoteLine says where Remote Control is on (PANEL-19).
func remoteLine(home string) string {
	switch RemoteControlSummary(home) {
	case RemoteAll:
		return "all (every Claude session: remoteControlAtStartup is true in " + SettingsPath(home) + ")"
	case RemoteLanes:
		return "lanes (the panel's lanes start with claude --remote-control)"
	}
	return "off"
}

// RestartHint is how to make a running login agent read projects.json again.
func RestartHint() string {
	return fmt.Sprintf("A running panel reads projects.json at start: restart it (launchctl kickstart -k gui/%d/%s, or Ctrl-C and start it again).", os.Getuid(), LaunchdLabel)
}
