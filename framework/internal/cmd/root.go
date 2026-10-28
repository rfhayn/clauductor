package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var Version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "clauductor",
	Short: "An operating model for Claude Code, and the panel that runs it",
	Long: `Clauductor installs an operating model into a repository (skills, hooks, checks,
agents and docs), keeps it current, and runs the panel: one local page for the
lanes of every project.`,
	// Main prints a command's error; cobra printing it too showed every error twice (#251).
	SilenceErrors: true,
	// Set once flags and arguments have passed, so only a mistyped command line prints usage,
	// never a command's own failure. A subcommand's PersistentPreRun would replace this one, so it
	// must silence usage too: TestUsageIsSilencedOnceTheArgumentsPass refuses one that does not.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { cmd.SilenceUsage = true },
}

func Execute() error {
	return rootCmd.Execute()
}

// errReported marks an error whose command has already told the user, in its own output, why it
// exits 1 (doctor's last line is its count). Main exits 1 for it and prints nothing more.
var errReported = errors.New("reported")

// Main runs the root command as the binary does and returns the process's exit status: a command's
// error is printed to stderr as "Error: ...", unless the command reported it itself.
func Main(stderr io.Writer) int {
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return 0
	}
	if !errors.Is(err, errReported) {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		// SilenceErrors also silences cobra's pointer to help after an unknown command. Only the
		// root reports one (cobra's Find): a subcommand's cobra.NoArgs uses the same words for an
		// extra argument, and cobra printed its usage, not the pointer (#269).
		if cmd == rootCmd && strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", cmd.CommandPath())
		}
	}
	return 1
}

func init() {
	// The template's version marker must match this binary's (template.Resolve).
	template.BinaryVersion = Version
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
}
