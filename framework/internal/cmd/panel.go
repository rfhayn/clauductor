package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/clauductor/clauductor/internal/panel"
	"github.com/spf13/cobra"
)

var (
	panelProject   string
	panelConfig    string
	panelPort      int
	panelNoOpen    bool
	panelUninstall bool
	panelLaunchd   bool
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
			changed, err := panel.UninstallHooks(home)
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintf(out, "Removed panel hooks from %s (pre-panel backup: settings.json.clauductor-panel.bak).\n", panel.SettingsPath(home))
			} else {
				fmt.Fprintf(out, "No panel hooks in %s.\n", panel.SettingsPath(home))
			}
			return nil
		}
		project := panelProject
		if project == "" {
			top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
			if err != nil {
				return fmt.Errorf("not inside a git repository; pass --project <path>")
			}
			project = strings.TrimSpace(string(top))
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return panel.Run(ctx, panel.Options{
			Project:    project,
			ConfigPath: panelConfig,
			Port:       panelPort,
			NoOpen:     panelNoOpen,
			Home:       home,
			Out:        out,
			Launchd:    panelLaunchd,
		})
	},
}

func init() {
	panelCmd.Flags().StringVar(&panelProject, "project", "", "project root (default: git toplevel of the current directory)")
	panelCmd.Flags().StringVar(&panelConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelCmd.Flags().IntVar(&panelPort, "port", 4393, "loopback port; the panel refuses to start if it is taken")
	panelCmd.Flags().BoolVar(&panelNoOpen, "no-open", false, "print the URL instead of opening a browser")
	panelCmd.Flags().BoolVar(&panelUninstall, "uninstall-hooks", false, "remove the panel's hooks from ~/.claude/settings.json and exit")
	// Set only by the login agent's plist: persistent token, 30-day cookie, one browser open per login.
	panelCmd.Flags().BoolVar(&panelLaunchd, "launchd", false, "run as the launchd login agent")
	_ = panelCmd.Flags().MarkHidden("launchd")

	panelInstallCmd.Flags().StringVar(&installProject, "project", "", "project root the agent serves (required)")
	panelInstallCmd.Flags().StringVar(&installConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelInstallCmd.Flags().IntVar(&installPort, "port", 4393, "loopback port")
	panelInstallCmd.Flags().BoolVar(&installApp, "app", false, "also create ~/Applications/Clauductor Panel.app, which runs `clauductor panel open`")
	_ = panelInstallCmd.MarkFlagRequired("project")
	panelOpenCmd.Flags().IntVar(&openPort, "port", 0, "port (default: the running panel's marker file, else 4393)")
	panelCmd.AddCommand(panelInstallCmd, panelUninstallCmd, panelOpenCmd)
	rootCmd.AddCommand(panelCmd)
}

var (
	installProject string
	installConfig  string
	installPort    int
	installApp     bool
	openPort       int
)

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
		return panel.Install(panel.InstallOptions{Home: home, Project: installProject, Config: installConfig,
			Port: installPort, App: installApp, Out: cmd.OutOrStdout()})
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
		return panel.Uninstall(home, cmd.OutOrStdout(), nil)
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
			if b, err := os.ReadFile(panel.MarkerPath(home)); err == nil {
				if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
					port = p
				}
			}
		}
		url, err := panel.OpenURL(context.Background(), home, port)
		if err != nil {
			return err
		}
		panel.OpenBrowser(url)
		return nil
	},
}
