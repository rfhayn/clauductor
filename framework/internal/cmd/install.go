package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

var (
	dryRun       bool
	forceInstall bool // install over a model the project runs itself (ownguard.go)
)

// The tiers are the template package's (template.Classify), so install, update and diff agree on
// what is the framework's.
type fileTier = template.Tier

const (
	tierFramework = template.TierFramework
	tierDoc       = template.TierDoc
	tierConfig    = template.TierConfig
	tierSettings  = template.TierSettings
)

func classifyFile(relPath string) fileTier { return template.Classify(relPath) }

func tierLabel(t fileTier) string { return template.TierLabel(t) }

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Add Clauductor framework to an existing project",
	Long: `Install Clauductor skills, docs, and orchestration infrastructure into
an existing project repository.

File handling by tier:
  FRAMEWORK (skills, hooks, checks, workflows, the
    operating model's scripts)                     → always installed
  SETTINGS (.claude/settings.json)                 → merged key by key: the model's
    hooks, status line, deny list and sandbox entries are brought up to date; the
    project's model, effort, env, skill overrides, plugins, and its own permissions
    and hooks are kept; every disagreement is reported
  DOC TEMPLATES (agents, AGENTS.md, project.conf,
    model-roles.json, panel.json, docs, steps.sh)  → created only if missing
  CONFIG (CLAUDE.md, .gitignore, .gitattributes)    → merged with existing (the
    .gitattributes line-ending rules are prepended, so the project's own win)

Paths follow .claude/project.conf: the gate scripts go where GATE_RUN, GATE and
GATE_STEPS say (a GATE_RUN with another file name is the project's own runner,
and the template's is left out), and the roadmap, journal, insights, owner
queue, ADRs, changes and specs where their keys say.

A repository that runs an operating model of its own (skills, hooks or
AGENTS.md that clauductor did not install) is refused, with the files an
install would overwrite or add, unless --force. One that runs clauductor's
old model (recognised from tracked files: its skills, hooks and settings) is
clauductor's, and is installed over; the old model's files the current one
replaced are listed and offered for removal (--prune removes them without
asking; a project-owned or unknown file is never touched), and settings.json
loses the hook registrations that run a script that will not exist.

The template comes from --template-dir, CLAUDUCTOR_FRAMEWORK, next to the
binary (a release ships its template), or the checkout the binary was built
from, in that order; its version must match the binary's. The first line
printed says which.

Use --dry-run to preview changes, including the settings.json diff, without
modifying anything. ` + "`clauductor diff`" + ` compares without the guard.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get working directory: %w", err)
		}

		// Verify this looks like a project
		if _, err := os.Stat(filepath.Join(targetDir, ".git")); os.IsNotExist(err) {
			fmt.Fprintln(out, "Warning: this directory is not a git repository.")
			if !dryRun && !confirm("Continue anyway?") {
				return fmt.Errorf("aborted")
			}
		}

		fmt.Fprintf(out, "Installing Clauductor framework into %s\n", targetDir)
		src, err := announceTemplate(out)
		if err != nil {
			return err
		}
		tmplDir := src.Dir

		allFiles, err := template.ListTemplateFiles()
		if err != nil {
			return fmt.Errorf("failed to list template files: %w", err)
		}

		if !forceInstall {
			if err := refusePluginModel("install", targetDir); err != nil {
				return err
			}
		}

		// A repository running its own operating model is left alone (ownguard.go).
		announceOldModel(out, targetDir)
		if !forceInstall && !ownedByClauductor(targetDir) {
			overwrite, add, err := foreignModel(targetDir, tmplDir, allFiles)
			if err != nil {
				return err
			}
			if len(overwrite)+len(add) > 0 {
				refusal := refuseForeign("install", targetDir, overwrite, add)
				if dryRun {
					fmt.Fprintln(out, refusal)
					fmt.Fprintln(out, "\n--dry-run: no changes made.")
					return nil
				}
				return refusal
			}
		}

		plan, err := planInstall(targetDir, tmplDir, allFiles)
		if err != nil {
			return fmt.Errorf("install refused: %w", err)
		}
		x, err := planExtras(targetDir, tmplDir, allFiles)
		if err != nil {
			return fmt.Errorf("install refused: %w", err)
		}
		x.oldStubs = nil // planInstall already replaces them (its FRAMEWORK list)
		plan.print(out, dryRun)
		x.print(out, false)
		for _, w := range template.SkillCollisions(targetDir, allFiles) {
			fmt.Fprintf(out, "  WARNING: %s\n\n", w)
		}

		if dryRun {
			fmt.Fprintln(out, "--dry-run: no changes made.")
			return nil
		}
		if err := plan.apply(targetDir); err != nil {
			return err
		}
		if err := x.applyAdditive(out, targetDir); err != nil {
			return err
		}
		if err := x.offerPrune(out, targetDir); err != nil {
			return err
		}

		fmt.Fprintln(out, "\nDone! Clauductor framework installed.")
		printPrereqs(out, tmplDir)
		fmt.Fprintln(out, "Next steps:")
		fmt.Fprintln(out, "  claude")
		fmt.Fprintln(out, "  /session-start")
		return nil
	},
}

func init() {
	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes (and the settings.json diff) without modifying anything")
	installCmd.Flags().BoolVar(&forceInstall, "force", false, "Install even over an operating model the repository runs itself (overwrites its framework files)")
	installCmd.Flags().BoolVar(&pruneOld, "prune", false, "Remove the old model's files that the current model replaced, without asking (never a project-owned or unknown file)")
	addTemplateFlag(installCmd)
}

// placed is one template file and where it goes.
type placed struct {
	rel, dest, key string
}

func (p placed) label() string {
	if p.dest == p.rel {
		return p.rel
	}
	return fmt.Sprintf("%s → %s (%s)", p.rel, p.dest, p.key)
}

// installPlan is everything an install would do, worked out before anything is written.
type installPlan struct {
	create    []placed          // missing in the project: written
	update    []placed          // framework files that differ: overwritten
	unchanged int               // framework files already identical
	keep      []placed          // doc files that exist: left alone
	merge     []placed          // CLAUDE.md, .gitignore, .gitattributes: merged
	skipped   map[string]string // template path → why the project's config leaves it out
	settings  *template.SettingsPlan
	tmplDir   string
	conf      *template.Conf
}

func planInstall(targetDir, tmplDir string, files []string) (*installPlan, error) {
	conf, err := template.LoadConf(targetDir, tmplDir)
	if err != nil {
		return nil, err
	}
	pm, err := conf.Mapping(files)
	if err != nil {
		return nil, err
	}
	p := &installPlan{skipped: map[string]string{}, tmplDir: tmplDir, conf: conf}
	for _, rel := range files {
		tier := classifyFile(rel)
		if tier == tierSettings {
			continue
		}
		dest, key, skip := pm.Resolve(rel)
		if skip != "" {
			p.skipped[rel] = skip
			continue
		}
		pl := placed{rel: rel, dest: dest, key: key}
		destPath := filepath.Join(targetDir, filepath.FromSlash(dest))
		if !fileExists(destPath) {
			p.create = append(p.create, pl)
			continue
		}
		switch tier {
		case tierFramework:
			if template.FilesEqual(filepath.Join(tmplDir, filepath.FromSlash(rel)), destPath) {
				p.unchanged++
			} else {
				p.update = append(p.update, pl)
			}
		case tierDoc:
			if template.OldModelUnchanged(targetDir, dest) {
				p.update = append(p.update, pl) // the old model's unedited stub (release-prep, ...)
			} else {
				p.keep = append(p.keep, pl)
			}
		case tierConfig:
			p.merge = append(p.merge, pl)
		}
	}
	tmplSettings, err := os.ReadFile(filepath.Join(tmplDir, filepath.FromSlash(template.SettingsPath)))
	if err != nil {
		return nil, err
	}
	cur, readErr := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(template.SettingsPath)))
	p.settings, err = template.PlanSettings(cur, readErr == nil, tmplSettings, conf, template.TemplateHookPrune(targetDir, tmplDir))
	if err != nil {
		return nil, fmt.Errorf("%s: %w (fix it, or move it aside and install again)", template.SettingsPath, err)
	}
	return p, nil
}

func (p *installPlan) print(out io.Writer, showDiff bool) {
	section := func(title string, items []placed, mark string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(out, "  %s (%d files):\n", title, len(items))
		for _, it := range items {
			fmt.Fprintf(out, "    %s %s\n", mark, it.label())
		}
		fmt.Fprintln(out)
	}
	section("NEW — will be created", p.create, "+")
	section("FRAMEWORK — will be updated", p.update, "~")
	if p.unchanged > 0 {
		fmt.Fprintf(out, "  FRAMEWORK — %d files already up to date\n\n", p.unchanged)
	}
	section("DOCS — keeping existing project content", p.keep, "=")
	section("CONFIG — will merge with existing", p.merge, "m")
	if len(p.skipped) > 0 {
		fmt.Fprintf(out, "  SKIPPED (%d files) — the project's config says it runs its own:\n", len(p.skipped))
		for _, rel := range sortedKeys(p.skipped) {
			fmt.Fprintf(out, "    - %s: %s\n", rel, p.skipped[rel])
		}
		fmt.Fprintln(out)
	}
	printSettingsPlan(out, p.settings, showDiff)
}

// printSettingsPlan reports what happens to settings.json: created, merged (with every change and
// conflict, and the diff when asked), or already current.
func printSettingsPlan(out io.Writer, sp *template.SettingsPlan, showDiff bool) {
	switch {
	case !sp.Exists:
		fmt.Fprintf(out, "  SETTINGS — %s will be created\n\n", template.SettingsPath)
		return
	case !sp.Changed():
		fmt.Fprintf(out, "  SETTINGS — %s is up to date", template.SettingsPath)
	default:
		fmt.Fprintf(out, "  SETTINGS — %s will be merged (the model's keys updated, the project's kept)", template.SettingsPath)
	}
	if n := sp.Conflicts(); n > 0 {
		fmt.Fprintf(out, "; %d conflict(s) reported", n)
	}
	fmt.Fprintln(out, ":")
	for _, c := range sp.Changes {
		fmt.Fprintf(out, "    %s\n", settingsChangeLine(c))
	}
	if showDiff && sp.Changed() {
		fmt.Fprintln(out)
		fmt.Fprint(out, indent(template.UnifiedDiff(string(sp.Current), string(sp.Result), "project/"+template.SettingsPath, "merged/"+template.SettingsPath), "    "))
	}
	fmt.Fprintln(out)
}

func settingsChangeLine(c template.SettingsChange) string {
	var s string
	switch c.Action {
	case "add":
		s = fmt.Sprintf("+ %s: %s", c.Path, c.Want)
	case "remove":
		s = fmt.Sprintf("- %s: %s", c.Path, c.Have)
	case "replace":
		s = fmt.Sprintf("! %s: the project's %s replaced by the template's %s", c.Path, c.Have, c.Want)
	case "keep":
		s = fmt.Sprintf("= %s: the project's %s kept (the template has %s)", c.Path, c.Have, c.Want)
	default:
		s = c.Action + " " + c.Path
	}
	if c.Note != "" {
		s += " — " + c.Note
	}
	return s
}

func indent(s, pre string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	return pre + strings.Join(lines, "\n"+pre) + "\n"
}

func (p *installPlan) apply(targetDir string) error {
	for _, list := range [][]placed{p.create, p.update} {
		for _, f := range list {
			if err := writeDocFile(targetDir, p.tmplDir, f, p.conf); err != nil {
				return fmt.Errorf("failed to install %s: %w", f.dest, err)
			}
		}
	}
	if p.settings.Changed() || !p.settings.Exists {
		dst := filepath.Join(targetDir, filepath.FromSlash(template.SettingsPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, p.settings.Result, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", template.SettingsPath, err)
		}
	}
	if err := writeInstallMarker(targetDir); err != nil {
		return fmt.Errorf("could not mark the install: %w", err)
	}
	for _, f := range p.merge {
		if err := mergeConfigFile(targetDir, f.rel); err != nil {
			fmt.Printf("  Warning: could not merge %s: %v\n", f.rel, err)
		}
	}
	// The old model kept its runtime state in orchestration/; a repository that still has one
	// keeps it out of git. The current model has none, so a fresh install adds no such line.
	if fileExists(filepath.Join(targetDir, "orchestration")) {
		if err := ensureGitignore(targetDir, "orchestration/"); err != nil {
			fmt.Printf("  Warning: could not update .gitignore: %v\n", err)
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stdin is ONE reader for every prompt: a fresh bufio.Reader per prompt buffers past the first
// line, so piped answers ("y\ny\n") after the first were silently swallowed.
var stdin = bufio.NewReader(os.Stdin)

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	answer, _ := stdin.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// readChoice reads one answer from stdin. At end of input it returns "c" (cancel), never "":
// update re-prompts on an empty answer, so a closed stdin (a script, CI, an agent) looped forever
// printing the prompt (OPS-8 rehearsal: 690 MB of "[y/d/s/c] >" in two minutes).
func readChoice() string { return readChoiceFrom(stdin) }

func readChoiceFrom(r *bufio.Reader) string {
	answer, err := r.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" && err != nil {
		fmt.Println("\n  (no more input: cancelling the remaining reviews)")
		return "c"
	}
	return answer
}

// mergeConfigFile merges a config file (CLAUDE.md, .gitignore, .gitattributes) into the
// project's copy.
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
		// The template's entries (lane worktrees, per-machine settings, ...), each added once.
		for _, entry := range template.GitignoreEntries(string(srcContent)) {
			if err := ensureGitignore(targetDir, entry); err != nil {
				return err
			}
		}
		return nil

	case template.GitattributesPath:
		merged, added := template.MergeGitattributes(existing, string(srcContent))
		if len(added) == 0 {
			return nil
		}
		return os.WriteFile(destPath, []byte(merged), 0644)

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
	if !strings.Contains(content, "# Clauductor\n") {
		f.WriteString("\n# Clauductor\n")
	}
	f.WriteString(entry + "\n")
	return nil
}
