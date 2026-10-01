package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var (
	diffJSON     bool
	diffPath     string
	diffExitCode bool
	diffAll      bool
)

// errDiverged makes `diff --exit-code` exit 1, as `git diff --exit-code` does, so a CI
// script can gate on convergence.
var errDiverged = errors.New("the repository differs from the template (diff --exit-code)")

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare this repository with the template, file by file and key by key",
	Long: `Compare the repository in the current directory with the Clauductor template,
changing nothing. It reads any repository, including one that runs an operating
model of its own (install and update refuse those; diff only reads).

  framework files   identical | differs | missing | extra (in a framework
                    directory, not in the template) | skipped (the project's
                    config says it runs its own, e.g. GATE_RUN)
  doc and config    present | missing (and what a merge would add to CLAUDE.md
                    and .gitignore)
  settings.json     the key-level merge install and update would do, with each
                    conflict and the diff of the merged file

Paths follow .claude/project.conf (GATE_RUN, GATE, GATE_STEPS, ROADMAP, JOURNAL,
INSIGHTS, OWNER_QUEUE, ADR_DIR, CHANGES_DIR, SPECS_DIR); the mapping is listed.

--json prints the whole report for a script. --exit-code exits 1 when a
framework file differs or is missing, or the settings merge would change the
file.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return err
		}
		rep, err := template.Survey(targetDir, strings.TrimPrefix(diffPath, "./"))
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if diffJSON {
			enc := json.NewEncoder(out)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rep); err != nil {
				return err
			}
		} else {
			printSurvey(out, rep, diffAll)
		}
		if diffExitCode && !rep.Converged() {
			return errDiverged
		}
		return nil
	},
}

func init() {
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Print the report as JSON")
	diffCmd.Flags().StringVar(&diffPath, "path", "", "Only files whose template or project path starts with this")
	diffCmd.Flags().BoolVar(&diffExitCode, "exit-code", false, "Exit 1 when the framework or settings.json differ from the template")
	diffCmd.Flags().BoolVar(&diffAll, "all", false, "List identical and present files too")
	rootCmd.AddCommand(diffCmd)
}

func printSurvey(out io.Writer, r *template.Report, all bool) {
	fmt.Fprintf(out, "clauductor diff: %s against %s\n", r.Repo, r.Template)
	marks := map[string]string{"identical": "=", "differs": "~", "missing": "-", "extra": "+", "skipped": "s", "present": "="}
	for _, tier := range []string{"framework", "doc", "config"} {
		var lines []string
		counts := map[string]int{}
		var order []string
		for _, f := range r.Files {
			if f.Tier != tier {
				continue
			}
			if counts[f.Status] == 0 {
				order = append(order, f.Status)
			}
			counts[f.Status]++
			if !all && (f.Status == "identical" || (f.Status == "present" && f.Note == "")) {
				continue
			}
			name := f.Path
			switch {
			case f.Path == "":
				name = f.Dest
			case f.Dest != "":
				name = fmt.Sprintf("%s → %s (%s)", f.Path, f.Dest, f.MappedBy)
			}
			l := fmt.Sprintf("  %s %-9s %s", marks[f.Status], f.Status, name)
			if f.Note != "" {
				l += " — " + f.Note
			}
			lines = append(lines, l)
		}
		if len(order) == 0 {
			continue
		}
		var parts []string
		for _, s := range order {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
		fmt.Fprintf(out, "\n%s: %s\n", tier, strings.Join(parts, ", "))
		for _, l := range lines {
			fmt.Fprintln(out, l)
		}
	}
	if s := r.Settings; s != nil {
		fmt.Fprintf(out, "\nsettings: %s %s", template.SettingsPath, s.Status)
		if s.Conflicts > 0 {
			fmt.Fprintf(out, ", %d conflict(s)", s.Conflicts)
		}
		fmt.Fprintln(out)
		if s.Error != "" {
			fmt.Fprintf(out, "  %s\n", s.Error)
		}
		for _, c := range s.Changes {
			fmt.Fprintf(out, "  %s\n", settingsChangeLine(c))
		}
		if s.Diff != "" {
			fmt.Fprint(out, indent(s.Diff, "  "))
		}
	}
	if len(r.Mapping) > 0 {
		fmt.Fprintln(out, "\nmapping (from .claude/project.conf):")
		for _, h := range r.Mapping {
			if h.Skip != "" {
				fmt.Fprintf(out, "  %s: not installed (%s)\n", h.Path, h.Skip)
			} else {
				fmt.Fprintf(out, "  %s → %s (%s)\n", h.Path, h.Dest, h.Key)
			}
		}
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(out, "\nWARNING: %s\n", w)
	}
	if r.Converged() {
		fmt.Fprintln(out, "\nThe framework matches the template.")
	}
}
