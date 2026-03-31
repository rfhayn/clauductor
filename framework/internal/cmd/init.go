package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Create a new project with Clauductor framework",
	Long: `Initialize a new project directory with Clauductor skills, docs, and
orchestration infrastructure. If path doesn't exist, it will be created.`,
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

		// Initialize orchestration directory and config
		if err := initOrchestration(targetDir); err != nil {
			return fmt.Errorf("failed to init orchestration: %w", err)
		}
		if err := ensureOrchestrationConfig(targetDir); err != nil {
			fmt.Printf("  Warning: could not create orchestration config: %v\n", err)
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

func initOrchestration(targetDir string) error {
	orchDir := filepath.Join(targetDir, "orchestration")
	if err := os.MkdirAll(filepath.Join(orchDir, "prompts"), 0755); err != nil {
		return err
	}

	fmt.Println("  Created orchestration/ directory")

	dbPath := filepath.Join(orchDir, "framework.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	db.Close()
	fmt.Println("  Initialized orchestration database")
	return nil
}

// ensureOrchestrationConfig creates orchestration/config.json if it doesn't exist.
func ensureOrchestrationConfig(targetDir string) error {
	configPath := filepath.Join(targetDir, "orchestration", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return nil // already exists
	}
	defaultConfig := []byte("{\n  \"default_workers\": 3,\n  \"auto_claude\": true\n}\n")
	if err := os.WriteFile(configPath, defaultConfig, 0644); err != nil {
		return err
	}
	fmt.Println("  Created orchestration/config.json")
	return nil
}

func runCommand(dir string, name string, args ...string) error {
	cmd := execCommand(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
