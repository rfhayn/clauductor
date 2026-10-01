package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// AddOptions registers a project with the machine's panel (PANEL-16).
type AddOptions struct {
	Home    string
	Project string // any path inside the project's main worktree
	Config  string // "" for <root>/.clauductor/panel.json
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

// AddProject registers a project: one git common dir, named by its main worktree. It
// loads and validates the config, but never trusts it: trusting is `panel trust`.
func AddProject(ctx context.Context, o AddOptions) (AddResult, error) {
	if o.Run == nil {
		o.Run = signals.ExecRunner
	}
	root := signals.ResolvePath(o.Project)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return AddResult{}, fmt.Errorf("%s is not a directory", o.Project)
	}
	wts, err := signals.ReadWorktrees(ctx, o.Run, root)
	if err != nil {
		return AddResult{}, fmt.Errorf("%s: %w", root, err)
	}
	if len(wts) == 0 {
		return AddResult{}, fmt.Errorf("%s: git lists no worktree", root)
	}
	// Two entries for one repository would both claim its worktrees and its lanes:
	// a project is its main worktree, and a linked one (or a symlink to either)
	// names the same project.
	if main := wts[0].Path; main != root {
		if i := signals.MatchWorktree(wts, root); i > 0 && o.Strict {
			return AddResult{}, fmt.Errorf("%s is a linked worktree of %s; add that instead (a project is its main worktree)", root, main)
		}
		root = main
	}
	reg, err := config.LoadProjects(o.Home)
	if err != nil {
		return AddResult{}, err
	}
	cfgPath := ""
	if o.Config != "" {
		cfgPath = signals.ResolvePath(o.Config)
	}
	if e := reg.Find(root); e != nil {
		res := AddResult{Entry: *e}
		changed := false
		if cfgPath != "" && e.Config != cfgPath {
			e.Config, changed = cfgPath, true
		}
		if o.MakeDefault && reg.Default != e.ID {
			reg.Default, changed = e.ID, true
		}
		cfg, err := config.LoadConfig(e.ConfigPath())
		if err != nil {
			return AddResult{}, err
		}
		res.Entry, res.Cfg = *e, cfg
		if changed {
			return res, config.SaveProjects(o.Home, reg)
		}
		return res, nil
	}
	e := config.ProjectEntry{Root: root, Config: cfgPath, Added: o.Now.Unix()}
	cfg, err := config.LoadConfig(e.ConfigPath())
	if err != nil {
		return AddResult{}, err
	}
	e.ID = o.ID
	if e.ID == "" {
		e.ID = reg.Slug(cfg.Name)
	}
	if !config.ProjectIDRe.MatchString(e.ID) {
		return AddResult{}, fmt.Errorf("project id %q must match %s", e.ID, config.ProjectIDRe)
	}
	if reg.Find(e.ID) != nil {
		return AddResult{}, fmt.Errorf("project id %q is taken; pass --id", e.ID)
	}
	e.TmuxSocket = reg.NewSocket(e.ID)
	// Two projects on one tmux server would each see the other's lanes as strays,
	// and a lane name used in both would refuse to start in the second.
	sock := e.Socket(cfg)
	for _, other := range reg.Projects {
		oc, _ := config.LoadConfig(other.ConfigPath())
		if other.Socket(oc) == sock {
			return AddResult{}, fmt.Errorf("tmux socket %q is %s's already; give this project its own tmux_socket in %s", sock, other.ID, e.ConfigPath())
		}
	}
	reg.Projects = append(reg.Projects, e)
	if reg.Default == "" || o.MakeDefault {
		reg.Default = e.ID
	}
	if err := config.SaveProjects(o.Home, reg); err != nil {
		return AddResult{}, err
	}
	return AddResult{Entry: e, Cfg: cfg, Added: true}, nil
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
		return config.ProjectEntry{}, fmt.Errorf("no registered project %q (`clauductor panel list` names them)", idOrPath)
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
		fmt.Fprintln(w, "No projects registered. `clauductor panel add --project <path>` adds one; a bare `clauductor panel` in a repository adds that one.")
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
		fmt.Fprintf(w, "%s %-16s %s\n    root %s\n    socket %s · %s · %d lane(s)\n", mark, e.ID, name, e.Root, e.Socket(cfg), trust, laneCount(home, e.Root))
	}
	fmt.Fprintln(w, "* the default: legacy routes and a page with no ?p= open on it.")
	return nil
}

// RestartHint is how to make a running login agent read projects.json again.
func RestartHint() string {
	return fmt.Sprintf("A running panel reads projects.json at start: restart it (launchctl kickstart -k gui/%d/%s, or Ctrl-C and start it again).", os.Getuid(), LaunchdLabel)
}
