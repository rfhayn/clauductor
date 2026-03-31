package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project's Clauductor skills, hooks, and templates",
	Long: `Compares the current project's skills, hooks, and doc templates against the
latest framework version. Shows changes and applies them interactively.
New files are applied automatically. Modified files require manual review
to prevent overwriting project-specific customizations.`,
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
			fmt.Println("All skills, hooks, and templates are up to date.")
			return nil
		}

		// Separate new files from modified files
		var newFiles, modifiedFiles []template.FileDiff
		for _, d := range diffs {
			if d.Status == "new" {
				newFiles = append(newFiles, d)
			} else {
				modifiedFiles = append(modifiedFiles, d)
			}
		}

		// Categorize for display
		var hookDiffs, skillDiffs, otherDiffs []template.FileDiff
		for _, d := range diffs {
			switch {
			case strings.HasPrefix(d.Path, ".claude/hooks/"):
				hookDiffs = append(hookDiffs, d)
			case strings.HasPrefix(d.Path, ".claude/skills/"):
				skillDiffs = append(skillDiffs, d)
			default:
				otherDiffs = append(otherDiffs, d)
			}
		}

		if len(skillDiffs) > 0 {
			fmt.Printf("Skills (%d):\n", len(skillDiffs))
			for _, d := range skillDiffs {
				fmt.Printf("  %s (%s)\n", d.Path, d.Status)
			}
			fmt.Println()
		}
		if len(hookDiffs) > 0 {
			fmt.Printf("Hooks (%d):\n", len(hookDiffs))
			for _, d := range hookDiffs {
				fmt.Printf("  %s (%s)\n", d.Path, d.Status)
			}
			fmt.Println()
		}
		if len(otherDiffs) > 0 {
			fmt.Printf("Other (%d):\n", len(otherDiffs))
			for _, d := range otherDiffs {
				fmt.Printf("  %s (%s)\n", d.Path, d.Status)
			}
			fmt.Println()
		}

		// Auto-apply new files
		if len(newFiles) > 0 {
			fmt.Printf("Applying %d new files automatically...\n", len(newFiles))
			for _, d := range newFiles {
				if err := template.ApplyUpdate(targetDir, d); err != nil {
					fmt.Printf("  Error adding %s: %v\n", d.Path, err)
				} else {
					fmt.Printf("  Added %s\n", d.Path)
				}
			}
			fmt.Println()
		}

		// Interactive review for modified files
		if len(modifiedFiles) == 0 {
			fmt.Println("Update complete.")
			return nil
		}

		tmplPath, err := template.TemplatePath()
		if err != nil {
			return err
		}

		fmt.Printf("%d modified files need review (may contain project customizations):\n\n", len(modifiedFiles))

		for _, d := range modifiedFiles {
			srcPath := filepath.Join(tmplPath, d.Path)
			destPath := filepath.Join(targetDir, d.Path)

			fmt.Printf("  %s\n", d.Path)
			fmt.Printf("  [y] overwrite with template  [d] show diff  [s] skip  [c] cancel remaining\n  > ")

			for {
				choice := readChoice()
				switch choice {
				case "y":
					if err := template.ApplyUpdate(targetDir, d); err != nil {
						fmt.Printf("  Error: %v\n", err)
					} else {
						fmt.Printf("  Updated.\n\n")
					}
					goto next
				case "d":
					showDiff(destPath, srcPath)
					fmt.Printf("  [y] overwrite with template  [s] skip  [c] cancel remaining\n  > ")
					continue
				case "s":
					fmt.Printf("  Skipped.\n\n")
					goto next
				case "c":
					fmt.Println("  Cancelled remaining.")
					return nil
				default:
					fmt.Printf("  [y/d/s/c] > ")
					continue
				}
			}
		next:
		}

		fmt.Println("Update complete.")
		return nil
	},
}

// showDiff runs diff between two files and prints the output.
func showDiff(projectFile, templateFile string) {
	cmd := exec.Command("diff", "-u", "--label", "project", projectFile, "--label", "template", templateFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run() // exit code 1 means files differ, which is expected
	fmt.Println()
}
