package panel

import (
	"context"
	"fmt"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// loadedProject is a registered project whose config and worktrees were read.
type loadedProject struct {
	entry   config.ProjectEntry
	cfg     *config.Config
	raw     []byte
	cfgPath string
	wts     []signals.Worktree
}

// loadProjects reads every registered project the panel is to serve (PANEL-16). A
// project that cannot load (its config, its worktree list, a socket another
// project has) is an item in the menu with the reason, and the others serve; only
// the project named on the command line is fatal, as it always was, and so is
// having none left.
func loadProjects(ctx context.Context, o Options, reg *config.Projects, primary string) ([]loadedProject, []web.ProjectSummary, error) {
	var out []loadedProject
	var failed []web.ProjectSummary
	sockets := map[string]string{}
	var firstErr error
	for _, e := range reg.Projects {
		if o.Only && e.ID != primary {
			continue
		}
		p, err := loadProject(ctx, o, e, e.ID == primary)
		if err == nil {
			if other, taken := sockets[p.cfg.Socket()]; taken {
				err = fmt.Errorf("tmux socket %q is %s's already; give this project its own tmux_socket", p.cfg.Socket(), other)
			}
		}
		if err != nil {
			if e.ID == primary {
				return nil, nil, err
			}
			fmt.Fprintf(o.Out, "project %s not loaded: %v\n", e.ID, err)
			failed = append(failed, web.ProjectSummary{ID: e.ID, Name: e.ID, Error: signals.Clip(err.Error(), 200), Default: e.ID == reg.Default})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sockets[p.cfg.Socket()] = e.ID
		out = append(out, p)
	}
	if len(out) == 0 {
		if firstErr == nil {
			firstErr = fmt.Errorf("no project to serve")
		}
		return nil, nil, firstErr
	}
	return out, failed, nil
}

// loadProject reads one project: its config, with the tmux socket it resolves to,
// and its worktree list, the event filter's authority.
func loadProject(ctx context.Context, o Options, e config.ProjectEntry, primary bool) (loadedProject, error) {
	cfgPath := e.ConfigPath()
	cfg, raw, err := config.LoadConfigRaw(cfgPath)
	if err != nil {
		return loadedProject{}, err
	}
	cfg.TmuxSocket = e.Socket(cfg)
	if primary && o.TmuxSocket != "" {
		if !config.SocketNameRe.MatchString(o.TmuxSocket) {
			return loadedProject{}, fmt.Errorf("tmux socket %q must match %s", o.TmuxSocket, config.SocketNameRe)
		}
		cfg.TmuxSocket = o.TmuxSocket
	}
	// Without the worktree list every event of the project would be dropped, so a
	// failure here is an error rather than a quiet empty project.
	wts, err := signals.ReadWorktrees(ctx, o.Runner, e.Root)
	if err != nil {
		return loadedProject{}, fmt.Errorf("%s: %w", e.Root, err)
	}
	return loadedProject{entry: e, cfg: cfg, raw: raw, cfgPath: cfgPath, wts: wts}, nil
}
