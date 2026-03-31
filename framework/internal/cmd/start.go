package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/hud"
	"github.com/spf13/cobra"
)

// teamConfig holds orchestration/config.json settings.
type teamConfig struct {
	AutoClaude bool `json:"auto_claude"`
}

func loadTeamConfig(targetDir string) teamConfig {
	cfg := teamConfig{AutoClaude: true}
	data, err := os.ReadFile(filepath.Join(targetDir, "orchestration", "config.json"))
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	return cfg
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Clauductor team workspace (HUD + supervisor)",
	Long: `Creates a tmux session with:
  Window 0: HUD (clauductor watch)
  Window 1: Supervisor (claude with /supervisor)

Workers are spawned on-demand by the supervisor via /spawn, preventing idle
claude sessions from burning context and API credits.
Auto-claude behavior for the supervisor is controlled by config.json "auto_claude" field.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := execCommand("tmux", "-V").Output(); err != nil {
			return fmt.Errorf("tmux is required but not found — install with: brew install tmux")
		}

		targetDir, err := os.Getwd()
		if err != nil {
			return err
		}

		_ = loadTeamConfig(targetDir) // config read for future use by /spawn

		projectName := filepath.Base(targetDir)
		sessionName := fmt.Sprintf("clauductor-%s", projectName)

		// Attach if session already exists
		if err := execCommand("tmux", "has-session", "-t", sessionName).Run(); err == nil {
			fmt.Printf("Session '%s' already running. Attaching...\n", sessionName)
			return runCommand("", "tmux", "attach-session", "-t", sessionName)
		}

		fmt.Printf("Starting team workspace '%s'...\n", sessionName)
		fmt.Printf("  HUD + supervisor (workers spawn on demand)\n\n")

		// Window 0: HUD
		if err := runCommand("", "tmux", "new-session", "-d", "-s", sessionName, "-c", targetDir, "-n", "hud"); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		runCommand("", "tmux", "send-keys", "-t", fmt.Sprintf("%s:hud", sessionName), "clauductor watch", "Enter")

		// Window 1: Supervisor
		runCommand("", "tmux", "new-window", "-t", sessionName, "-c", targetDir, "-n", "supervisor")
		runCommand("", "tmux", "send-keys", "-t", fmt.Sprintf("%s:supervisor", sessionName), "claude '/supervisor'", "Enter")

		// Select the supervisor window before attaching
		runCommand("", "tmux", "select-window", "-t", fmt.Sprintf("%s:supervisor", sessionName))

		return runCommand("", "tmux", "attach-session", "-t", sessionName)
	},
}

var hudDemoMode bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Launch the HUD (real-time dashboard)",
	Long:  `Displays the real-time orchestration dashboard. Reads from the SQLite state database. Use --demo for stub data.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hud.Version = Version
		var source hud.DataSource

		if hudDemoMode {
			source = hud.NewStubDataSource()
		} else {
			dbPath := filepath.Join("orchestration", "framework.db")
			if _, err := os.Stat(dbPath); err == nil {
				sqlSource, err := hud.NewSQLiteDataSource(dbPath)
				if err != nil {
					return fmt.Errorf("opening orchestration database: %w", err)
				}
				defer sqlSource.Close()
				source = sqlSource
			} else {
				fmt.Println("No orchestration database found. Running in demo mode.")
				fmt.Println("Run 'clauductor init' or 'clauductor install' to set up orchestration.")
				source = hud.NewStubDataSource()
			}
		}

		return hud.Run(source)
	},
}

func init() {
	watchCmd.Flags().BoolVar(&hudDemoMode, "demo", false, "Run HUD with demo/stub data")
}
