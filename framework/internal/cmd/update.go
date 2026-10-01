package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var (
	forceUpdate  bool
	updateDryRun bool
)

func init() {
	updateCmd.Flags().BoolVar(&forceUpdate, "force", false, "Update even a repository that runs its own operating model")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "List what would change, with the settings.json diff, and change nothing")
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project's Clauductor skills, hooks, and templates",
	Long: `Compares every framework file the template ships (skills, hooks, checks,
workflows, modules, the model's scripts) and the agents against the project's
copies, at the paths its .claude/project.conf gives them. Shows changes and
applies them interactively. New files are applied automatically. Modified files
require manual review to prevent overwriting project-specific customizations.

.claude/settings.json is merged, not copied: the model's hooks, status line,
deny list and sandbox entries are brought up to date, the project's keys are
kept, and every conflict is reported. --dry-run shows the diff first.

.gitattributes gains the template's line-ending rules it lacks (LF for every
text file, so shell scripts run under WSL2 and Git for Windows), prepended so
the project's own lines still override them.

Docs the project owns are never written. Of them, the template's guidance docs
(the playbook, conventions, principles, the READMEs of changes/, specs/ and
scripts/ci/, ...) are named when they differ from the template's copy or are
missing, so new guidance is found; ` + "`clauductor diff`" + ` notes them too.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get working directory: %w", err)
		}

		if !forceUpdate {
			if err := refusePluginModel("update", targetDir); err != nil {
				return err
			}
		}

		// Verify this looks like a Clauductor project
		if _, err := os.Stat(filepath.Join(targetDir, ".claude", "skills")); os.IsNotExist(err) {
			return fmt.Errorf("no .claude/skills/ found — is this a Clauductor project? Run 'clauductor install' first")
		}

		// A repository running its own operating model is left alone (ownguard.go).
		if !forceUpdate && !ownedByClauductor(targetDir) {
			files, err := template.ListTemplateFiles()
			if err != nil {
				return err
			}
			tmplDir, err := template.TemplatePath()
			if err != nil {
				return err
			}
			overwrite, add, err := foreignModel(targetDir, tmplDir, files)
			if err != nil {
				return err
			}
			if len(overwrite)+len(add) > 0 {
				return refuseForeign("update", targetDir, overwrite, add)
			}
		}

		fmt.Printf("Checking for updates in %s\n\n", targetDir)
		printGuidanceDrift(os.Stdout, targetDir)

		// Find files that differ from template
		diffs, err := template.FindDiffs(targetDir)
		if err != nil {
			return fmt.Errorf("failed to compare files: %w", err)
		}
		settings, err := planProjectSettings(targetDir)
		if err != nil {
			return err
		}

		attrs, err := planGitattributes(targetDir)
		if err != nil {
			return err
		}

		if len(diffs) == 0 && !settings.Changed() && attrs == nil {
			fmt.Println("All skills, hooks, and templates are up to date.")
			if len(settings.Changes) > 0 {
				printSettingsPlan(os.Stdout, settings, false)
			}
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
				fmt.Printf("  %s (%s)\n", diffLabel(d), d.Status)
			}
			fmt.Println()
		}
		if len(hookDiffs) > 0 {
			fmt.Printf("Hooks (%d):\n", len(hookDiffs))
			for _, d := range hookDiffs {
				fmt.Printf("  %s (%s)\n", diffLabel(d), d.Status)
			}
			fmt.Println()
		}
		if len(otherDiffs) > 0 {
			fmt.Printf("Other (%d):\n", len(otherDiffs))
			for _, d := range otherDiffs {
				fmt.Printf("  %s (%s)\n", diffLabel(d), d.Status)
			}
			fmt.Println()
		}

		if settings.Changed() || len(settings.Changes) > 0 {
			printSettingsPlan(os.Stdout, settings, updateDryRun)
		}
		attrs.print(os.Stdout)
		if updateDryRun {
			fmt.Println("--dry-run: no changes made.")
			return nil
		}
		if err := attrs.apply(targetDir); err != nil {
			return err
		}
		if settings.Changed() {
			if err := writeSettings(targetDir, settings); err != nil {
				return err
			}
			fmt.Printf("Merged %s.\n\n", template.SettingsPath)
		}

		// Auto-apply new files
		if len(newFiles) > 0 {
			fmt.Printf("Applying %d new files automatically...\n", len(newFiles))
			for _, d := range newFiles {
				if err := template.ApplyUpdate(targetDir, d); err != nil {
					fmt.Printf("  Error adding %s: %v\n", d.Path, err)
				} else {
					fmt.Printf("  Added %s\n", diffLabel(d))
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
			destPath := filepath.Join(targetDir, d.Dest)

			fmt.Printf("  %s\n", diffLabel(d))
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

func diffLabel(d template.FileDiff) string {
	if d.Dest == "" || d.Dest == d.Path {
		return d.Path
	}
	return d.Path + " → " + d.Dest
}

// planProjectSettings is the settings.json merge for the project in targetDir.
func planProjectSettings(targetDir string) (*template.SettingsPlan, error) {
	tmplDir, err := template.TemplatePath()
	if err != nil {
		return nil, err
	}
	conf, err := template.LoadConf(targetDir, tmplDir)
	if err != nil {
		return nil, err
	}
	tmpl, err := os.ReadFile(filepath.Join(tmplDir, filepath.FromSlash(template.SettingsPath)))
	if err != nil {
		return nil, err
	}
	cur, readErr := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(template.SettingsPath)))
	sp, err := template.PlanSettings(cur, readErr == nil, tmpl, conf)
	if err != nil {
		return nil, fmt.Errorf("%s: %w (fix it, then run update again)", template.SettingsPath, err)
	}
	return sp, nil
}

// printGuidanceDrift names the template's guidance docs (the playbook, conventions, ...) that
// differ from the project's copies or that it lacks. update never writes them, since a project
// edits them, so without this notice a project installed earlier would never learn that, say, the
// playbook gained its machine-setup guide. A survey that cannot run says so rather than nothing.
func printGuidanceDrift(out io.Writer, targetDir string) {
	differs, missing, err := template.GuidanceDrift(targetDir)
	if err != nil {
		fmt.Fprintf(out, "Docs — could not compare the template's docs with this project's: %v\n\n", err)
		return
	}
	if len(differs)+len(missing) == 0 {
		return
	}
	fmt.Fprintln(out, "Docs — the template's guidance differs from this project's copies. update never overwrites")
	fmt.Fprintln(out, "them (the project edits them); compare with `clauductor diff`, and take what you want:")
	for _, p := range differs {
		fmt.Fprintf(out, "  ~ %s\n", p)
	}
	for _, p := range missing {
		fmt.Fprintf(out, "  - %s (missing here)\n", p)
	}
	fmt.Fprintln(out)
}

// gitattributesPlan is update's merge of the template's line-ending rules into the project's
// .gitattributes. Update copies only framework files, so without this a project installed before
// the template shipped the rules would never get them, and a collaborator on Windows would check
// every shell script out with CRLF. nil means the project already has every rule.
type gitattributesPlan struct {
	exists bool
	added  []string
	result []byte
}

func planGitattributes(targetDir string) (*gitattributesPlan, error) {
	tmplDir, err := template.TemplatePath()
	if err != nil {
		return nil, err
	}
	want, err := os.ReadFile(filepath.Join(tmplDir, template.GitattributesPath))
	if os.IsNotExist(err) {
		return nil, nil // a template from before the rules
	}
	if err != nil {
		return nil, err
	}
	have, readErr := os.ReadFile(filepath.Join(targetDir, template.GitattributesPath))
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, readErr
	}
	merged, added := template.MergeGitattributes(string(have), string(want))
	if len(added) == 0 {
		return nil, nil
	}
	return &gitattributesPlan{exists: readErr == nil, added: added, result: []byte(merged)}, nil
}

func (p *gitattributesPlan) print(out io.Writer) {
	if p == nil {
		return
	}
	if p.exists {
		fmt.Fprintf(out, "Line endings — %s gains %d rule(s), prepended so the project's own lines still win:\n", template.GitattributesPath, len(p.added))
	} else {
		fmt.Fprintf(out, "Line endings — %s will be created:\n", template.GitattributesPath)
	}
	for _, l := range p.added {
		fmt.Fprintf(out, "  + %s\n", l)
	}
	fmt.Fprintln(out)
}

func (p *gitattributesPlan) apply(targetDir string) error {
	if p == nil {
		return nil
	}
	if err := os.WriteFile(filepath.Join(targetDir, template.GitattributesPath), p.result, 0o644); err != nil {
		return err
	}
	fmt.Printf("Merged %s. Files already checked out keep their line endings until checked out again; .claude/checks/line-endings.sh names any that still hold a CR.\n\n", template.GitattributesPath)
	return nil
}

func writeSettings(targetDir string, sp *template.SettingsPlan) error {
	dst := filepath.Join(targetDir, filepath.FromSlash(template.SettingsPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, sp.Result, 0o644)
}
