package cmd

import (
	"github.com/spf13/cobra"
)

var Version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "clauductor",
	Short: "Multi-worker orchestration framework for Claude Code",
	Long: `Clauductor coordinates multiple Claude Code sessions (human or AI)
working simultaneously on the same codebase. It provides file locking,
a real-time HUD, session management, and orchestration logging.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(deregisterCmd)
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(unlockCmd)
	rootCmd.AddCommand(eventCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(heartbeatCmd)
	rootCmd.AddCommand(milestoneCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(checkLockCmd)
	rootCmd.AddCommand(contextCmd)
}
