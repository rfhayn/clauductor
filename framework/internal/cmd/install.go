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

var (
	dryRun       bool
	forceInstall bool // install over a model the project runs itself (ownguard.go)
)

// File tiers for install behavior
type fileTier int

const (
	tierFramework fileTier = iota // Skills, agents, settings, statusline → always install
	tierDoc                       // project-owned config, agents and docs → create only if missing
	tierConfig                    // CLAUDE.md, .gitignore → merge
)

// frameworkScripts are the operating model's own scripts outside the framework directories: a
// project runs them but does not edit them, so an install brings them up to date.
var frameworkScripts = map[string]bool{
	".claude/statusline.sh":      true,
	".claude/status-write.sh":    true,
	".claude/owner-queue.sh":     true,
	".claude/roadmap-queue.sh":   true,
	".claude/panel-suggest.sh":   true,
	".claude/machine-quiet.sh":   true,
	".claude/scenario-trace.sh":  true,
	".claude/change-approval.sh": true,
	".claude/change-cost.sh":     true,
	".claude/verify-change.sh":   true,
	".claude/compound.sh":        true,
	"scripts/ci/run-local.sh":    true,
	"scripts/ci/gate.sh":         true,
	"scripts/ci/lease.sh":        true,
}

// projectSkills are template skills a project configures (CONFIGURE FIRST stubs): created when
// missing, never overwritten, like docs.
var projectSkills = []string{".claude/skills/architecture-audit/", ".claude/skills/release-prep/"}

// classifyFile determines how to handle a template file during install.
func classifyFile(relPath string) fileTier {
	for _, p := range projectSkills {
		if strings.HasPrefix(relPath, p) {
			return tierDoc
		}
	}
	// Framework files — always install/overwrite. The project's own values live elsewhere
	// (.claude/project.conf, model-roles.json, .clauductor/panel.json, scripts/ci/steps.sh,
	// AGENTS.md, docs/), which fall through to the doc tier and are never overwritten.
	if strings.HasPrefix(relPath, ".claude/skills/") ||
		strings.HasPrefix(relPath, ".claude/hooks/") ||
		strings.HasPrefix(relPath, ".claude/checks/") ||
		strings.HasPrefix(relPath, ".claude/lib/") ||
		strings.HasPrefix(relPath, ".claude/workflows/") ||
		strings.HasPrefix(relPath, ".claude/modules/") ||
		strings.HasPrefix(relPath, ".claude/examples/") ||
		relPath == ".claude/settings.json" ||
		frameworkScripts[relPath] {
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
  FRAMEWORK (skills, hooks, checks, workflows, the
    operating model's scripts, settings)           → always installed
  DOC TEMPLATES (agents, AGENTS.md, project.conf,
    model-roles.json, panel.json, docs, steps.sh)  → created only if missing
  CONFIG (CLAUDE.md, .gitignore)                    → merged with existing

A repository that runs an operating model of its own (skills, hooks or
AGENTS.md that clauductor did not install) is refused, with the files an
install would overwrite or add, unless --force.

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

		// A repository running its own operating model is left alone (ownguard.go).
		if !forceInstall && !ownedByClauductor(targetDir) {
			tmplDir, err := template.TemplatePath()
			if err != nil {
				return err
			}
			overwrite, add, err := foreignModel(targetDir, tmplDir, allFiles)
			if err != nil {
				return err
			}
			if len(overwrite)+len(add) > 0 {
				refusal := refuseForeign("install", targetDir, overwrite, add)
				if dryRun {
					fmt.Println(refusal)
					fmt.Println("\n--dry-run: no changes made.")
					return nil
				}
				return refusal
			}
		}

		var frameworkFiles []string // Always install
		var docSkipped []string     // Skip if exists
		var configFiles []string    // Merge
		var newFiles []string       // Don't exist yet, install regardless of tier

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

		if err := writeInstallMarker(targetDir); err != nil {
			return fmt.Errorf("could not mark the install: %w", err)
		}

		// Handle config file merges
		for _, f := range configFiles {
			if err := mergeConfigFile(targetDir, f); err != nil {
				fmt.Printf("  Warning: could not merge %s: %v\n", f, err)
			}
		}

		// Initialize orchestration directory and config
		if err := initOrchestration(targetDir); err != nil {
			return fmt.Errorf("failed to init orchestration: %w", err)
		}
		if err := ensureOrchestrationConfig(targetDir); err != nil {
			fmt.Printf("  Warning: could not create orchestration config: %v\n", err)
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
	installCmd.Flags().BoolVar(&forceInstall, "force", false, "Install even over an operating model the repository runs itself (overwrites its files)")
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
		// Ensure the runtime state and the lane worktrees stay out of git.
		for _, entry := range []string{"orchestration/", ".claude/worktrees/"} {
			if err := ensureGitignore(targetDir, entry); err != nil {
				return err
			}
		}
		return nil

	case "CLAUDE.md":
		// The operating model's rules live in AGENTS.md, which CLAUDE.md imports. An existing
		// CLAUDE.md keeps its content and gains the import line.
		for _, line := range strings.Split(existing, "\n") {
			if strings.TrimSpace(line) == "@AGENTS.md" {
				return nil
			}
		}
		f, err := os.OpenFile(destPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		sep := "\n"
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			sep = "\n\n"
		}
		_, err = f.WriteString(sep + "@AGENTS.md\n")
		return err

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
