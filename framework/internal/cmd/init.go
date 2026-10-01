package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Create a new project with Clauductor framework",
	Long: `Initialize a new project directory with the Clauductor operating model: skills,
hooks, checks, agents, docs and settings. If path doesn't exist, it will be created.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		// Check if directory already has content
		if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(targetDir)
			if len(entries) > 0 {
				return fmt.Errorf("%s is not empty — use 'clauductor install' to add framework to an existing project", targetDir)
			}
		}

		// Create directory if needed
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("could not create directory: %w", err)
		}

		fmt.Printf("Initializing new Clauductor project in %s\n", targetDir)

		// Copy template files
		if err := template.CopyTemplate(targetDir); err != nil {
			return fmt.Errorf("failed to copy template: %w", err)
		}
		if err := writeInstallMarker(targetDir); err != nil {
			return fmt.Errorf("could not mark the install: %w", err)
		}

		// Initialize git if not already a repo
		if _, err := os.Stat(filepath.Join(targetDir, ".git")); os.IsNotExist(err) {
			fmt.Println("  Initializing git repository...")
			if err := runCommand(targetDir, "git", "init"); err != nil {
				fmt.Printf("  Warning: could not init git: %v\n", err)
			}
		}

		fmt.Println("\nDone! Next steps:")
		fmt.Printf("  cd %s\n", targetDir)
		fmt.Println("  claude")
		fmt.Println("  /session-start")
		return nil
	},
}

func runCommand(dir string, name string, args ...string) error {
	cmd := execCommand(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
