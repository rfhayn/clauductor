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

var (
	forceUpdate  bool
	updateDryRun bool
)

func init() {
	updateCmd.Flags().BoolVar(&forceUpdate, "force", false, "Update even a repository that runs its own operating model")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "List what would change, with the settings.json diff, and change nothing")
	updateCmd.Flags().BoolVar(&pruneOld, "prune", false, "Remove the old model's files that the current model replaced, without asking (never a project-owned or unknown file)")
	updateCmd.Flags().BoolVar(&createMissing, "create-missing", false, "Create the project files the template added that this project lacks, without asking (never overwrites)")
	addTemplateFlag(updateCmd)
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
kept, every conflict is reported, and a hook registration that runs a script
that does not exist (or one of the old model's hooks) is dropped.

Project-owned files are never overwritten, but update now offers what the
template added to them:
  new project files (health lines, evals, docs)  → offered, created only if
                                                    missing (--create-missing:
                                                    without asking)
  .claude/model-roles.json                       → the template's new keys added;
                                                    no existing value changes
  AGENTS.md                                      → the template's new table rows
                                                    listed as a suggestion, not applied
  .clauductor/panel.json                         → checked against the BRANCH_* keys

The old model's files that the current one replaced are listed and offered for
removal (--prune removes them without asking). --dry-run shows everything first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
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

		fmt.Fprintf(out, "Checking for updates in %s\n", targetDir)
		src, err := announceTemplate(out)
		if err != nil {
			return err
		}
		tmplPath := src.Dir
		files, err := template.ListTemplateFiles()
		if err != nil {
			return err
		}

		// A repository running its own operating model is left alone (ownguard.go).
		announceOldModel(out, targetDir)
		if !forceUpdate && !ownedByClauductor(targetDir) {
			overwrite, add, err := foreignModel(targetDir, tmplPath, files)
			if err != nil {
				return err
			}
			if len(overwrite)+len(add) > 0 {
				return refuseForeign("update", targetDir, overwrite, add)
			}
		}

		// Find files that differ from template
		diffs, err := template.FindDiffs(targetDir)
		if err != nil {
			return fmt.Errorf("failed to compare files: %w", err)
		}
		settings, err := planProjectSettings(targetDir)
		if err != nil {
			return err
		}
		x, err := planExtras(targetDir, tmplPath, files)
		if err != nil {
			return err
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
		for _, grp := range []struct {
			title string
			list  []template.FileDiff
		}{{"Skills", skillDiffs}, {"Hooks", hookDiffs}, {"Other", otherDiffs}} {
			if len(grp.list) == 0 {
				continue
			}
			fmt.Fprintf(out, "%s (%d):\n", grp.title, len(grp.list))
			for _, d := range grp.list {
				fmt.Fprintf(out, "  %s (%s)\n", diffLabel(d), d.Status)
			}
			fmt.Fprintln(out)
		}
		if len(diffs) == 0 {
			fmt.Fprintln(out, "All skills, hooks, and templates are up to date.")
			fmt.Fprintln(out)
		}

		if settings.Changed() || len(settings.Changes) > 0 {
			printSettingsPlan(out, settings, updateDryRun)
		}
		x.print(out, true)
		if updateDryRun {
			fmt.Fprintln(out, "--dry-run: no changes made.")
			return nil
		}
		if settings.Changed() {
			if err := writeSettings(targetDir, settings); err != nil {
				return err
			}
			fmt.Fprintf(out, "Merged %s.\n\n", template.SettingsPath)
		}
		if err := x.applyAdditive(out, targetDir); err != nil {
			return err
		}
		if len(x.docNew) > 0 {
			if createMissing || confirm(fmt.Sprintf("Create the %d new project file(s) listed above (none exists yet)?", len(x.docNew))) {
				for _, f := range x.docNew {
					if err := writeDocFile(targetDir, tmplPath, f, x.conf); err != nil {
						fmt.Fprintf(out, "  Error adding %s: %v\n", f.dest, err)
					} else {
						fmt.Fprintf(out, "  Added %s\n", f.label())
					}
				}
			} else {
				fmt.Fprintln(out, "  Skipped the new project files (--create-missing adds them without asking).")
			}
			fmt.Fprintln(out)
		}
		if err := x.offerPrune(out, targetDir); err != nil {
			return err
		}

		// Auto-apply new files
		if len(newFiles) > 0 {
			fmt.Fprintf(out, "Applying %d new files automatically...\n", len(newFiles))
			for _, d := range newFiles {
				if err := template.ApplyUpdate(targetDir, d); err != nil {
					fmt.Fprintf(out, "  Error adding %s: %v\n", d.Path, err)
				} else {
					fmt.Fprintf(out, "  Added %s\n", diffLabel(d))
				}
			}
			fmt.Fprintln(out)
		}

		// Interactive review for modified files
		if len(modifiedFiles) == 0 {
			fmt.Fprintln(out, "Update complete.")
			return nil
		}

		fmt.Fprintf(out, "%d modified files need review (may contain project customizations):\n\n", len(modifiedFiles))

		for _, d := range modifiedFiles {
			srcPath := filepath.Join(tmplPath, d.Path)
			destPath := filepath.Join(targetDir, d.Dest)

			fmt.Fprintf(out, "  %s\n", diffLabel(d))
			fmt.Fprintf(out, "  [y] overwrite with template  [d] show diff  [s] skip  [c] cancel remaining\n  > ")

			for {
				choice := readChoice()
				switch choice {
				case "y":
					if err := template.ApplyUpdate(targetDir, d); err != nil {
						fmt.Fprintf(out, "  Error: %v\n", err)
					} else {
						fmt.Fprintf(out, "  Updated.\n\n")
					}
					goto next
				case "d":
					showDiff(destPath, srcPath)
					fmt.Fprintf(out, "  [y] overwrite with template  [s] skip  [c] cancel remaining\n  > ")
					continue
				case "s":
					fmt.Fprintf(out, "  Skipped.\n\n")
					goto next
				case "c":
					fmt.Fprintln(out, "  Cancelled remaining.")
					return nil
				default:
					fmt.Fprintf(out, "  [y/d/s/c] > ")
					continue
				}
			}
		next:
		}

		fmt.Fprintln(out, "Update complete.")
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
	sp, err := template.PlanSettings(cur, readErr == nil, tmpl, conf, template.TemplateHookPrune(targetDir, tmplDir))
	if err != nil {
		return nil, fmt.Errorf("%s: %w (fix it, then run update again)", template.SettingsPath, err)
	}
	return sp, nil
}

func writeSettings(targetDir string, sp *template.SettingsPlan) error {
	dst := filepath.Join(targetDir, filepath.FromSlash(template.SettingsPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, sp.Result, 0o644)
}
