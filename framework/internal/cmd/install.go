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

// File tiers for install behavior
type fileTier int

const (
	tierFramework fileTier = iota // Skills, agents, settings, statusline → always install
	tierDoc                       // current-story, journal, etc. → create only if missing
	tierConfig                    // CLAUDE.md, .gitignore → merge
)

// classifyFile determines how to handle a template file during install.
func classifyFile(relPath string) fileTier {
	// Framework files — always install/overwrite
	if strings.HasPrefix(relPath, ".claude/skills/") ||
		relPath == ".claude/settings.json" ||
		relPath == ".claude/statusline.sh" {
		return tierFramework
	}

	// Agent files — create only if missing (preserves project-specific agents)
	if strings.HasPrefix(relPath, ".claude/agents/") {
		return tierDoc
	}

	// Config files — merge
	if relPath == "CLAUDE.md" || relPath == ".gitignore" {
		return tierConfig
	}

	// Everything else (docs, README) — create only if missing
	return tierDoc
}

func tierLabel(t fileTier) string {
	switch t {
	case tierFramework:
		return "framework"
	case tierDoc:
		return "doc"
	case tierConfig:
		return "config"
	default:
		return "unknown"
	}
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Add Clauductor framework to an existing project",
	Long: `Install Clauductor skills, docs, and orchestration infrastructure into
an existing project repository.

File handling by tier:
  FRAMEWORK (skills, agents, settings, statusline) → always installed
  DOC TEMPLATES (current-story, journal, etc.)      → created only if missing
  CONFIG (CLAUDE.md, .gitignore)                    → merged with existing

Use --dry-run to preview changes without modifying anything.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get working directory: %w", err)
		}

		// Verify this looks like a project
		if _, err := os.Stat(filepath.Join(targetDir, ".git")); os.IsNotExist(err) {
			fmt.Println("Warning: this directory is not a git repository.")
			if !dryRun && !confirm("Continue anyway?") {
				return fmt.Errorf("aborted")
			}
		}

		fmt.Printf("Installing Clauductor framework into %s\n\n", targetDir)

		// Get all template files and classify them
		allFiles, err := template.ListTemplateFiles()
		if err != nil {
			return fmt.Errorf("failed to list template files: %w", err)
		}

		var frameworkFiles []string  // Always install
		var docSkipped []string      // Skip if exists
		var configFiles []string     // Merge
		var newFiles []string        // Don't exist yet, install regardless of tier

		for _, relPath := range allFiles {
			destPath := filepath.Join(targetDir, relPath)
			exists := fileExists(destPath)
			tier := classifyFile(relPath)

			if !exists {
				newFiles = append(newFiles, relPath)
				continue
			}

			switch tier {
			case tierFramework:
				frameworkFiles = append(frameworkFiles, relPath)
			case tierDoc:
				docSkipped = append(docSkipped, relPath)
			case tierConfig:
				configFiles = append(configFiles, relPath)
			}
		}

		// Report plan
		if len(newFiles) > 0 {
			fmt.Printf("  NEW (%d files) — will be created:\n", len(newFiles))
			for _, f := range newFiles {
				fmt.Printf("    + %s\n", f)
			}
			fmt.Println()
		}

		if len(frameworkFiles) > 0 {
			fmt.Printf("  FRAMEWORK (%d files) — will be updated:\n", len(frameworkFiles))
			for _, f := range frameworkFiles {
				fmt.Printf("    ~ %s\n", f)
			}
			fmt.Println()
		}

		if len(docSkipped) > 0 {
			fmt.Printf("  DOCS (%d files) — keeping existing project content:\n", len(docSkipped))
			for _, f := range docSkipped {
				fmt.Printf("    = %s\n", f)
			}
			fmt.Println()
		}

		if len(configFiles) > 0 {
			fmt.Printf("  CONFIG (%d files) — will merge with existing:\n", len(configFiles))
			for _, f := range configFiles {
				fmt.Printf("    m %s\n", f)
			}
			fmt.Println()
		}

		if dryRun {
			fmt.Println("--dry-run: no changes made.")
			return nil
		}

		// Build skip list: doc files that already exist
		skipFiles := make(map[string]bool)
		for _, f := range docSkipped {
			skipFiles[f] = true
		}
		// Also skip config files — we handle those separately
		for _, f := range configFiles {
			skipFiles[f] = true
		}

		// Copy template (skipping docs that exist and config files)
		if err := template.CopyTemplateWithSkips(targetDir, skipFiles); err != nil {
			return fmt.Errorf("failed to copy template: %w", err)
		}

		// Handle config file merges
		for _, f := range configFiles {
			if err := mergeConfigFile(targetDir, f); err != nil {
				fmt.Printf("  Warning: could not merge %s: %v\n", f, err)
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

// mergeConfigFile handles merging a config file (CLAUDE.md, .gitignore).
// For now, appends Clauductor-specific sections if they don't already exist.
func mergeConfigFile(targetDir, relPath string) error {
	tmplPath, err := template.TemplatePath()
	if err != nil {
		return err
	}

	destPath := filepath.Join(targetDir, relPath)
	srcPath := filepath.Join(tmplPath, relPath)

	destContent, err := os.ReadFile(destPath)
	if err != nil {
		return err
	}

	srcContent, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	existing := string(destContent)

	switch relPath {
	case ".gitignore":
		// Just ensure orchestration/ is in gitignore
		if !strings.Contains(existing, "orchestration/") {
			return ensureGitignore(targetDir, "orchestration/")
		}
		return nil

	case "CLAUDE.md":
		// If the existing CLAUDE.md doesn't reference Clauductor, append a section
		if !strings.Contains(existing, "Clauductor") && !strings.Contains(existing, "clauductor") {
			marker := "\n\n## Clauductor Framework\n\n"
			marker += "This project uses [Clauductor](https://github.com/rfhayn/clauductor) for orchestration.\n\n"
			marker += "See `template/CLAUDE.md` in the framework repo for the full reference, or run `clauductor update` to sync skills.\n"

			f, err := os.OpenFile(destPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString(marker)
			return err
		}
		return nil

	default:
		// Unknown config file — just use the template version
		return os.WriteFile(destPath, srcContent, 0644)
	}
}

func ensureGitignore(targetDir, entry string) error {
	gitignorePath := filepath.Join(targetDir, ".gitignore")

	content := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		content = string(data)
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
	}

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
