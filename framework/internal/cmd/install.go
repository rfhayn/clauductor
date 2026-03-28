package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var dryRun bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Add Clauductor framework to an existing project",
	Long: `Install Clauductor skills, docs, and orchestration infrastructure into
an existing project repository. Non-destructive — prompts before overwriting
any existing files. Use --dry-run to preview changes without modifying anything.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get working directory: %w", err)
		}

		// Verify this looks like a project (has .git or at least some files)
		if _, err := os.Stat(filepath.Join(targetDir, ".git")); os.IsNotExist(err) {
			fmt.Println("Warning: this directory is not a git repository.")
			if !confirm("Continue anyway?") {
				return fmt.Errorf("aborted")
			}
		}

		fmt.Printf("Installing Clauductor framework into %s\n\n", targetDir)

		// Copy template with conflict resolution
		conflicts, err := template.FindConflicts(targetDir)
		if err != nil {
			return fmt.Errorf("failed to check for conflicts: %w", err)
		}

		if len(conflicts) > 0 {
			fmt.Printf("Found %d existing files that conflict with the framework:\n\n", len(conflicts))
			for _, c := range conflicts {
				fmt.Printf("  %s\n", c)
			}

			if dryRun {
				fmt.Println("\n--dry-run: no changes made.")
				return nil
			}

			fmt.Println("\nRecommendation: use the framework version for full functionality.")
			fmt.Println("Options for each conflict: [u]se framework / [k]eep existing (default) / [s]kip\n")

			skipFiles := make(map[string]bool)
			for _, c := range conflicts {
				fmt.Printf("  %s [u/K/s]: ", c)
				choice := readChoice()
				switch choice {
				case "u", "use":
					fmt.Printf("    Using framework version of %s\n", c)
				default:
					// Default to keeping existing file (safe default)
					skipFiles[c] = true
					fmt.Printf("    Keeping existing %s\n", c)
				}
			}
			fmt.Println()

			if err := template.CopyTemplateWithSkips(targetDir, skipFiles); err != nil {
				return fmt.Errorf("failed to copy template: %w", err)
			}
		} else {
			if dryRun {
				fmt.Println("No conflicts found. --dry-run: no changes made.")
				return nil
			}

			if err := template.CopyTemplate(targetDir); err != nil {
				return fmt.Errorf("failed to copy template: %w", err)
			}
		}

		// Initialize orchestration directory
		if err := initOrchestration(targetDir); err != nil {
			return fmt.Errorf("failed to init orchestration: %w", err)
		}

		// Ensure orchestration/ is in .gitignore
		if err := ensureGitignore(targetDir, "orchestration/"); err != nil {
			fmt.Printf("  Warning: could not update .gitignore: %v\n", err)
		}

		fmt.Println("\nDone! Clauductor framework installed.")
		fmt.Println("Next steps:")
		fmt.Println("  claude")
		fmt.Println("  /session-start")
		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying anything")
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func readChoice() string {
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(answer))
}

func ensureGitignore(targetDir, entry string) error {
	gitignorePath := filepath.Join(targetDir, ".gitignore")

	// Read existing .gitignore
	content := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		content = string(data)
		// Check if entry already exists
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == entry {
				return nil // Already there
			}
		}
	}

	// Append entry
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if content != "" && !strings.HasSuffix(content, "\n") {
		f.WriteString("\n")
	}
	f.WriteString("\n# Clauductor runtime state\n")
	f.WriteString(entry + "\n")
	return nil
}
