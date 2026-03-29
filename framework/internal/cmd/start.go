package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/hud"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Clauductor orchestration (tmux session + HUD)",
	Long: `Starts a tmux session with the Clauductor HUD in the first pane.
Worker sessions are added as additional panes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Verify tmux is available
		if _, err := execCommand("tmux", "-V").Output(); err != nil {
			return fmt.Errorf("tmux is required but not found — install with: brew install tmux")
		}

		targetDir, err := os.Getwd()
		if err != nil {
			return err
		}

		projectName := filepath.Base(targetDir)
		sessionName := fmt.Sprintf("clauductor-%s", projectName)

		// Check if session already exists
		if err := execCommand("tmux", "has-session", "-t", sessionName).Run(); err == nil {
			fmt.Printf("Clauductor session '%s' already running. Attaching...\n", sessionName)
			return runCommand("", "tmux", "attach-session", "-t", sessionName)
		}

		fmt.Printf("Starting Clauductor session '%s'...\n", sessionName)

		// Create tmux session with HUD
		// The HUD will be implemented in M5; for now, start with a status message
		if err := runCommand("", "tmux", "new-session", "-d", "-s", sessionName, "-c", targetDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}

		// Send the HUD command to the first pane (placeholder for M5)
		hudCmd := fmt.Sprintf("clauductor watch")
		runCommand("", "tmux", "send-keys", "-t", sessionName, hudCmd, "Enter")

		// Attach to the session
		return runCommand("", "tmux", "attach-session", "-t", sessionName)
	},
}

var hudDemoMode bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Launch the HUD (real-time dashboard)",
	Long:  `Displays the real-time orchestration dashboard. Reads from the SQLite state database. Use --demo for stub data.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var source hud.DataSource

		if hudDemoMode {
			source = hud.NewStubDataSource()
		} else {
			// Check if orchestration DB exists
			dbPath := filepath.Join("orchestration", "framework.db")
			if _, err := os.Stat(dbPath); err == nil {
				source = hud.NewSQLiteDataSource(dbPath)
			} else {
				// No DB — fall back to demo mode
				fmt.Println("No orchestration database found. Running in demo mode.")
				fmt.Println("Run 'clauductor init' or 'clauductor install' to set up orchestration.\n")
				source = hud.NewStubDataSource()
			}
		}

		return hud.Run(source)
	},
}

func init() {
	watchCmd.Flags().BoolVar(&hudDemoMode, "demo", false, "Run HUD with demo/stub data")
}
