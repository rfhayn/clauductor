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
	panelTrust     bool
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
			// A running panel re-checks its hooks every 30 s and puts them back.
			if other := panel.RunningPanel(context.Background(), home, os.Getpid(), panel.LiveProc); other != nil {
				fmt.Fprintf(out, "A panel is running (pid %d); it reinstalls its hooks within 30 s. Stop it first to keep them out.\n", other.PID)
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
			Project:     project,
			ConfigPath:  panelConfig,
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
	panelCmd.Flags().StringVar(&panelProject, "project", "", "project root (default: git toplevel of the current directory)")
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

	panelInstallCmd.Flags().StringVar(&installProject, "project", "", "project root the agent serves (required)")
	panelInstallCmd.Flags().StringVar(&installConfig, "config", "", "panel config file (default: <project>/.clauductor/panel.json)")
	panelInstallCmd.Flags().IntVar(&installPort, "port", 4393, "loopback port")
	panelInstallCmd.Flags().BoolVar(&installApp, "app", false, "also create ~/Applications/Clauductor Panel.app, which runs `clauductor panel open`")
	_ = panelInstallCmd.MarkFlagRequired("project")
	panelOpenCmd.Flags().IntVar(&openPort, "port", 0, "port (default: the running panel's marker file, else 4393)")
	panelCmd.AddCommand(panelInstallCmd, panelUninstallCmd, panelOpenCmd, panelRotateCmd, panelTrustCmd)
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
		// Installing is choosing this config: record it as trusted, so the agent does
		// not start in the untrusted mode.
		if _, err := panel.TrustConfig(home, installProject, installConfig); err != nil {
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
		if _, err := panel.RotateToken(home); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Rotated %s. The running panel follows within 2 s; `clauductor panel open` opens it with the new token.\n", panel.TokenPath(home))
		return nil
	},
}

var (
	trustProject string
	trustConfig  string
)

var panelTrustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust panel.json as it is now, so its cards, queue commands and templates run",
	Long: `panel.json names commands the panel runs and prompts it types into lanes. The
panel records its SHA-256 on first use; when the file changes (a pull, say), a
running or starting panel keeps its cards, queue commands and templates off until
you trust the new version with this command. A running panel notices within 5 s.`,
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
		h, err := panel.TrustConfig(home, project, trustConfig)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Trusted panel config (sha256 %s).\n", h[:12])
		return nil
	},
}
