package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project's Clauductor skills and templates",
	Long: `Compares the current project's skills and doc templates against the latest
framework version. Shows changes and applies them interactively. Does not
touch user code or doc content.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get working directory: %w", err)
		}

		// Verify this looks like a Clauductor project
		if _, err := os.Stat(filepath.Join(targetDir, ".claude", "skills")); os.IsNotExist(err) {
			return fmt.Errorf("no .claude/skills/ found — is this a Clauductor project? Run 'clauductor install' first")
		}

		fmt.Printf("Checking for updates in %s\n\n", targetDir)

		// Find files that differ from template
		diffs, err := template.FindDiffs(targetDir)
		if err != nil {
			return fmt.Errorf("failed to compare files: %w", err)
		}

		if len(diffs) == 0 {
			fmt.Println("All skills and templates are up to date.")
			return nil
		}

		fmt.Printf("Found %d files with updates available:\n\n", len(diffs))
		for _, d := range diffs {
			fmt.Printf("  %s (%s)\n", d.Path, d.Status)
		}

		fmt.Println("\nOptions: [a]pply all / [i]nteractive / [c]ancel")
		choice := readChoice()

		switch choice {
		case "a", "apply":
			for _, d := range diffs {
				if err := template.ApplyUpdate(targetDir, d); err != nil {
					fmt.Printf("  Error updating %s: %v\n", d.Path, err)
				} else {
					fmt.Printf("  Updated %s\n", d.Path)
				}
			}
		case "i", "interactive":
			for _, d := range diffs {
				fmt.Printf("\n  %s (%s)\n", d.Path, d.Status)
				fmt.Printf("  Apply? [y/n] ")
				if readChoice() == "y" {
					if err := template.ApplyUpdate(targetDir, d); err != nil {
						fmt.Printf("  Error: %v\n", err)
					} else {
						fmt.Printf("  Updated.\n")
					}
				} else {
					fmt.Printf("  Skipped.\n")
				}
			}
		default:
			fmt.Println("Cancelled.")
			return nil
		}

		fmt.Println("\nUpdate complete.")
		return nil
	},
}
