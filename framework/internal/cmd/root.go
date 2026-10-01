package cmd

import (
	"github.com/spf13/cobra"
)

var Version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "clauductor",
	Short: "An operating model for Claude Code, and the panel that runs it",
	Long: `Clauductor installs an operating model into a repository (skills, hooks, checks,
agents and docs), keeps it current, and runs the panel: one local page for the
lanes of every project.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
}
