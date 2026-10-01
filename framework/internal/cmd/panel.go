package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/clauductor/clauductor/internal/panel"
	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/spf13/cobra"
)

var (
	panelProject   string
	panelConfig    string
	panelPort      int
	panelNoOpen    bool
	panelUninstall bool
	panelLaunchd   bool
	panelTrust     bool
	panelOnly      bool
)

// panelCmd is standalone by design: unlike the rest of the CLI it never opens the
// SQLite state, and needs no `clauductor install` in the project.
var panelCmd = &cobra.Command{
	Use:   "panel",
	Short: "Serve a local, read-only web dashboard of the Claude sessions in a project",
	Long: `Serve a loopback-only web dashboard of every Claude Code session working in one
project's git worktrees: status, context %, running subagents, notifications,
quota, open PRs and project cards. It reads only Claude Code's own signals
(HTTP hooks, the status line, 'claude agents --json'), git and gh.

On start it installs tagged HTTP hooks into ~/.claude/settings.json (idempotent;
other hooks are untouched). --uninstall-hooks removes them. See docs/panel.md.`,
	Args: cobra.NoArgs,
	// A refused start (port taken, no config) is not a usage mistake; print the reason once.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if panelUninstall {
			changed, err := install.UninstallHooks(home)
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintf(out, "Removed panel hooks from %s (pre-panel backup: settings.json.clauductor-panel.bak).\n", install.SettingsPath(home))
			} else {
				fmt.Fprintf(out, "No panel hooks in %s.\n", install.SettingsPath(home))
			}
			// A running panel re-checks its hooks every 30 s and puts them back.
			if other := install.RunningPanel(context.Background(), home, os.Getpid(), lease.LiveProc); other != nil {
				fmt.Fprintf(out, "A panel is running (pid %d); it reinstalls its hooks within 30 s. Stop it first to keep them out.\n", other.PID)
			}
			return nil
		}
		// One panel serves every registered project (PANEL-16). A bare `panel` inside
		// a repository registers that one (if it is not) and opens on it; outside
		// one, or as the login agent with no --project, it serves the registry.
		project := panelProject
		if project == "" && !panelLaunchd {
			if top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
				project = strings.TrimSpace(string(top))
			} else if reg, lerr := config.LoadProjects(home); lerr != nil || len(reg.Projects) == 0 {
				return fmt.Errorf("not inside a git repository, and no project is registered; pass --project <path>")
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return panel.Run(ctx, panel.Options{
			Project:     project,
			ConfigPath:  panelConfig,
			Only:        panelOnly,
			Port:        panelPort,
			NoOpen:      panelNoOpen,
			Home:        home,
			Out:         out,
			Launchd:     panelLaunchd,
			TrustConfig: panelTrust,
		})
	},
}

func init() {
	panelCmd.Flags().StringVar(&panelProject, "project", "", "a project to serve and open on, registered if it is not (default: git toplevel of the current directory)")
	panelCmd.Flags().BoolVar(&panelOnly, "only", false, "serve --project alone, not every registered project")
	panelCmd.Flags().StringVar(&panelConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelCmd.Flags().IntVar(&panelPort, "port", 4393, "loopback port; the panel refuses to start if it is taken")
	panelCmd.Flags().BoolVar(&panelNoOpen, "no-open", false, "print the URL instead of opening a browser")
	panelCmd.Flags().BoolVar(&panelUninstall, "uninstall-hooks", false, "remove the panel's hooks from ~/.claude/settings.json and exit")
	// Set only by the login agent's plist: persistent token, 30-day cookie, one browser open per login.
	panelCmd.Flags().BoolVar(&panelLaunchd, "launchd", false, "run as the launchd login agent")
	_ = panelCmd.Flags().MarkHidden("launchd")
	panelCmd.Flags().BoolVar(&panelTrust, "trust-config", false, "trust panel.json as it is now, even if it changed since it was last trusted")
	panelTrustCmd.Flags().StringVar(&trustProject, "project", "", "project root (default: git toplevel of the current directory)")
	panelTrustCmd.Flags().StringVar(&trustConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")

	panelInstallCmd.Flags().StringVar(&installProject, "project", "", "add this project (trusting its config) and make it the default; without it the registry must hold one")
	panelInstallCmd.Flags().StringVar(&installConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelInstallCmd.Flags().IntVar(&installPort, "port", 4393, "loopback port")
	panelInstallCmd.Flags().BoolVar(&installApp, "app", false, "also create ~/Applications/Clauductor Panel.app, which runs `clauductor panel open`")
	panelOpenCmd.Flags().IntVar(&openPort, "port", 0, "port (default: the running panel's marker file, else 4393)")
	panelOpenCmd.Flags().StringVar(&openProject, "project", "", "open on this project (an id or a path)")
	panelInitCmd.Flags().StringVar(&initProject, "project", "", "project root (default: git toplevel of the current directory)")
	panelAddCmd.Flags().StringVar(&addProject, "project", "", "project root (default: git toplevel of the current directory)")
	panelAddCmd.Flags().StringVar(&addConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelAddCmd.Flags().StringVar(&addID, "id", "", "the project's id in routes and the page (default: from its config's name)")
	panelAddCmd.Flags().BoolVar(&addDefault, "default", false, "make it the default project")
	panelRemoveCmd.Flags().BoolVar(&removeForce, "force", false, "remove it even while lanes are registered (they keep running; their registry is kept)")
	panelCmd.AddCommand(panelInitCmd, panelInstallCmd, panelUninstallCmd, panelOpenCmd, panelRotateCmd, panelTrustCmd,
		panelAddCmd, panelRemoveCmd, panelListCmd)
	rootCmd.AddCommand(panelCmd)
}

var (
	installProject string
	installConfig  string
	installPort    int
	installApp     bool
	openPort       int
	openProject    string

	addProject  string
	addConfig   string
	addID       string
	addDefault  bool
	removeForce bool
)

var panelAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a project with the panel (one panel serves every registered project)",
	Long: `Register a repository in ~/.clauductor/panel/projects.json, so the panel serves it
beside the others. It loads and checks the project's panel.json and gives the
project an id (from its name, or --id) and a tmux socket of its own. It does not
trust the config: run 'clauductor panel trust' after reviewing it. A project is
its main worktree; a linked worktree is refused. A running panel reads
projects.json at start, so restart it to serve the project.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		project := addProject
		if project == "" {
			top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
			if err != nil {
				return fmt.Errorf("not inside a git repository; pass --project <path>")
			}
			project = strings.TrimSpace(string(top))
		}
		res, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: project, Config: addConfig,
			ID: addID, Run: signals.ExecRunner, Now: clock.System.Now(), Strict: true, MakeDefault: addDefault})
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if !res.Added {
			fmt.Fprintf(out, "%s is registered already, as %q.\n", res.Entry.Root, res.Entry.ID)
		} else {
			fmt.Fprintf(out, "Registered %s as %q (%s), on tmux socket %s.\n", res.Entry.Root, res.Entry.ID, res.Cfg.Name, res.Entry.Socket(res.Cfg))
		}
		_, raw, err := config.LoadConfigRaw(res.Entry.ConfigPath())
		if err == nil && !install.TrustedNow(home, res.Entry.Root, res.Entry.ConfigPath(), install.ConfigHash(raw)) {
			fmt.Fprintf(out, "Its config %s is not trusted: its cards, queue commands and templates stay off until you review it and run `clauductor panel trust --project %s`.\n",
				res.Entry.ConfigPath(), res.Entry.Root)
		}
		if other := install.RunningPanel(context.Background(), home, os.Getpid(), lease.LiveProc); other != nil {
			fmt.Fprintln(out, install.RestartHint())
		}
		return nil
	},
}

var panelRemoveCmd = &cobra.Command{
	Use:   "remove <id|path>",
	Short: "Unregister a project (its lanes keep running; nothing is deleted)",
	Long: `Remove a project from ~/.clauductor/panel/projects.json. It never stops a lane and
keeps the project's lane registry, so adding it again brings its lanes back. While
lanes are registered it refuses unless --force. A running panel reads projects.json
at start, so restart it.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		e, err := install.RemoveProject(home, args[0], removeForce)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Removed %q (%s) from %s. Its lanes, worktrees and lane registry are untouched.\n", e.ID, e.Root, config.ProjectsPath(home))
		if other := install.RunningPanel(context.Background(), home, os.Getpid(), lease.LiveProc); other != nil {
			fmt.Fprintln(out, install.RestartHint())
		}
		return nil
	},
}

var panelListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List the registered projects: id, name, root, socket, trust and lanes",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return install.ListProjects(cmd.OutOrStdout(), home)
	},
}

var panelInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Run the panel at login as a launchd agent, with no terminal",
	Long: `Copy this binary to ~/.clauductor/panel/bin, write
~/Library/LaunchAgents/com.clauductor.panel.plist (start at login, restart after a
crash, logs in ~/.clauductor/panel/logs) and load it. The token persists in
~/.clauductor/panel/token so 'clauductor panel open' can open the page. --app also
creates ~/Applications/Clauductor Panel.app for the Dock and Spotlight.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if installProject != "" {
			// Installing is choosing this project: register it as the default, and
			// record its config as trusted, so the agent does not start it in the
			// untrusted mode; say what that trusts.
			res, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: installProject,
				Config: installConfig, Run: signals.ExecRunner, Now: clock.System.Now(), MakeDefault: true})
			if err != nil {
				return err
			}
			h, runs, err := install.TrustConfigReport(home, res.Entry.Root, res.Entry.Config)
			if err != nil {
				return err
			}
			install.PrintTrusted(cmd.OutOrStdout(), res.Entry.ConfigPath(), h, runs)
		}
		// The agent serves projects.json (PANEL-16): the plist names no project.
		return install.Install(install.InstallOptions{Home: home, Port: installPort, App: installApp, Out: cmd.OutOrStdout(), Clock: clock.System})
	},
}

var panelUninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Stop and remove the panel's launchd agent (lanes keep running)",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return install.Uninstall(home, cmd.OutOrStdout(), nil)
	},
}

var panelOpenCmd = &cobra.Command{
	Use:           "open",
	Short:         "Open the installed panel in the browser",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		port := openPort
		if port == 0 {
			port = 4393
			if b, err := os.ReadFile(install.MarkerPath(home)); err == nil {
				if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
					port = p
				}
			}
		}
		url, err := install.OpenURL(context.Background(), home, port)
		if err != nil {
			return err
		}
		if openProject != "" {
			// The page opens on that project (?p=, kept through the token's redirect).
			reg, err := config.LoadProjects(home)
			if err != nil {
				return err
			}
			e := reg.Find(openProject)
			if e == nil {
				e = reg.Find(signals.ResolvePath(openProject))
			}
			if e == nil {
				return fmt.Errorf("no registered project %q (`clauductor panel list` names them)", openProject)
			}
			url += "&p=" + e.ID
		}
		install.OpenBrowser(url)
		return nil
	},
}

var panelRotateCmd = &cobra.Command{
	Use:   "rotate-token",
	Short: "Replace the installed panel's token; old cookies and open terminals stop working",
	Long: `Write a new ~/.clauductor/panel/token. The running login agent picks it up within
2 s: every cookie issued for the old token gets 401, and every open terminal and
event stream is closed. Then 'clauductor panel open' opens the page with the new one.
'clauductor panel install' also rotates the token.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if _, err := install.RotateToken(home); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Rotated %s. The running panel follows within 2 s; `clauductor panel open` opens it with the new token.\n", install.TokenPath(home))
		return nil
	},
}

var (
	trustProject string
	trustConfig  string
	initProject  string
)

var panelInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a starter .clauductor/panel.json for this project",
	Long: `Write <project>/.clauductor/panel.json from what the repository already says: its
name, its default branch (the base new lanes start from), where its worktrees
live, the branch prefixes it uses, and a gate script it defines (a package.json
script or Makefile target named gate, ci, check, verify or test) as a queue. It
writes no card, and a queue's command runs only when you press RUN. It refuses to
overwrite an existing file. The file names its JSON Schema, so an editor
validates it. See docs/panel.md, "Configuration reference".`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := initProject
		if dir == "" {
			dir = "."
		}
		res, err := install.InitConfig(context.Background(), signals.ExecRunner, dir)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Wrote %s:\n\n%s\n", res.Path, res.Body)
		for _, n := range res.Notes {
			fmt.Fprintf(out, "  %s\n", n)
		}
		fmt.Fprintf(out, "\nJSON has no comments, so the reasons are here. It declares \"version\": %d and \"$schema\", so an editor\n"+
			"validates it. Review it, then run `clauductor panel trust`: until then the panel runs none of its commands.\n"+
			"Then `clauductor panel add` registers it with the panel, beside your other projects.\n", config.LatestVersion)
		return nil
	},
}

var panelTrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust panel.json as it is now, so its cards, queue commands and templates run",
	Long: `panel.json names commands the panel runs and prompts it types into lanes. The
panel runs them only for the exact bytes you trusted: a config it has never seen,
or one that changed (a pull, say), keeps its cards, queue commands and templates
off until you review it and trust it with this command. A running panel notices
within 5 s.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		project := trustProject
		if project == "" {
			top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
			if err != nil {
				return fmt.Errorf("not inside a git repository; pass --project <path>")
			}
			project = strings.TrimSpace(string(top))
		}
		h, runs, err := install.TrustConfigReport(home, project, trustConfig)
		if err != nil {
			return err
		}
		install.PrintTrusted(cmd.OutOrStdout(), configPathOf(project, trustConfig), h, runs)
		return nil
	},
}

// configPathOf is the config a --project/--config pair names.
func configPathOf(project, cfg string) string {
	if cfg != "" {
		return cfg
	}
	return filepath.Join(project, config.DefaultConfigRel)
}
