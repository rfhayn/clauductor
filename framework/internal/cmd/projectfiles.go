package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/cobra"
)

// What install and update do beyond the framework tier: the template they read, the old model's
// leftovers, and the additive changes to project-owned files (OPS-14, the OPS-8 rehearsal fixes).

var (
	pruneOld      bool // --prune: remove the old model's replaced files without asking
	createMissing bool // update --create-missing: create the new project files without asking
)

func addTemplateFlag(c *cobra.Command) {
	c.Flags().StringVar(&template.OverrideDir, "template-dir", "", "Use this template (a template directory, or a clauductor checkout or release holding template/), even if its version is not this binary's")
}

// announceTemplate resolves the template and says which one this run copies from.
func announceTemplate(out io.Writer) (*template.Source, error) {
	s, err := template.Resolve()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "Template: %s\n", s)
	if s.Warning != "" {
		fmt.Fprintf(out, "\n  WARNING: %s\n", s.Warning)
	}
	fmt.Fprintln(out)
	return s, nil
}

// extras is what install and update would do to project-owned files and the old model's.
type extras struct {
	docNew        []placed           // doc-tier files the project lacks (update offers them)
	roles         []byte             // model-roles.json with the template's new keys added (nil: nothing to add)
	rolesAdded    []string           // the keys added
	panelProblems []string           // panel.json disagrees with the BRANCH_* keys
	agentsRows    []string           // AGENTS.md table rows the template has and the project lacks
	old           []template.OldFile // the old model's files the current model replaced
	oldStubs      []placed           // project skills that are the old model's unedited stubs: replaced
	conf          *template.Conf
	pm            *template.PathMap
}

func planExtras(targetDir, tmplDir string, files []string) (*extras, error) {
	conf, err := template.LoadConf(targetDir, tmplDir)
	if err != nil {
		return nil, err
	}
	pm, err := conf.Mapping(files)
	if err != nil {
		return nil, err
	}
	x := &extras{conf: conf, pm: pm}
	for _, rel := range files {
		if classifyFile(rel) != tierDoc || strings.HasPrefix(rel, ".claude/agents/") {
			continue // agents: update reviews them with the framework files
		}
		dest, key, skip := pm.Resolve(rel)
		if skip != "" {
			continue
		}
		pl := placed{rel: rel, dest: dest, key: key}
		if !fileExists(filepath.Join(targetDir, filepath.FromSlash(dest))) {
			x.docNew = append(x.docNew, pl)
		} else if template.OldModelUnchanged(targetDir, dest) {
			x.oldStubs = append(x.oldStubs, pl)
		}
	}
	read := func(dir, rel string) ([]byte, bool) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		return b, err == nil
	}
	if have, ok := read(targetDir, template.ModelRolesPath); ok {
		if want, ok := read(tmplDir, template.ModelRolesPath); ok {
			merged, added, err := template.MergeAddOnly(have, want)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", template.ModelRolesPath, err)
			}
			if len(added) > 0 {
				x.roles, x.rolesAdded = merged, added
			}
		}
	}
	if have, ok := read(targetDir, template.PanelPath); ok {
		x.panelProblems = template.PanelBranchProblems(have, conf)
	}
	if have, ok := read(targetDir, template.AgentsPath); ok {
		if want, ok := read(tmplDir, template.AgentsPath); ok {
			x.agentsRows = template.MissingAgentsRows(string(have), string(want))
		}
	}
	x.old = template.StaleOldModel(targetDir, files)
	return x, nil
}

func (x *extras) print(out io.Writer, showDocNew bool) {
	if showDocNew && len(x.docNew) > 0 {
		fmt.Fprintf(out, "  NEW PROJECT FILES — the template added them; created only if missing, never overwritten (%d):\n", len(x.docNew))
		for _, f := range x.docNew {
			fmt.Fprintf(out, "    + %s\n", f.label())
		}
		fmt.Fprintln(out)
	}
	if len(x.oldStubs) > 0 {
		fmt.Fprintf(out, "  OLD-MODEL STUBS — unedited, replaced with the template's (%d):\n", len(x.oldStubs))
		for _, f := range x.oldStubs {
			fmt.Fprintf(out, "    ~ %s\n", f.label())
		}
		fmt.Fprintln(out)
	}
	if len(x.rolesAdded) > 0 {
		fmt.Fprintf(out, "  MODEL ROLES — %s gains the template's new keys; no existing value changes (%d):\n", template.ModelRolesPath, len(x.rolesAdded))
		for _, k := range x.rolesAdded {
			fmt.Fprintf(out, "    + %s\n", k)
		}
		fmt.Fprintln(out)
	}
	if len(x.panelProblems) > 0 {
		fmt.Fprintf(out, "  PANEL — %s disagrees with the branch keys in .claude/project.conf (edit it; it is the project's):\n", template.PanelPath)
		for _, p := range x.panelProblems {
			fmt.Fprintf(out, "    ! %s\n", p)
		}
		fmt.Fprintln(out)
	}
	if len(x.agentsRows) > 0 {
		fmt.Fprintf(out, "  AGENTS.md — rows the template's \"What executes each rule\" table has and this one lacks.\n")
		fmt.Fprintf(out, "  Not applied (the file is the project's, and holds to a byte budget): add the ones that apply.\n")
		for _, r := range x.agentsRows {
			fmt.Fprintf(out, "    + %s\n", r)
		}
		fmt.Fprintln(out)
	}
	if len(x.old) > 0 {
		fmt.Fprintf(out, "  OLD MODEL — files of clauductor's old model that the current model replaced (%d):\n", len(x.old))
		for _, f := range x.old {
			note := "edited since the old template shipped it"
			if f.Unchanged {
				note = "unedited"
			}
			fmt.Fprintf(out, "    - %s (%s)\n", f.Path, note)
		}
		fmt.Fprintln(out, "  They are offered for removal (--prune removes them without asking). Nothing else is touched.")
		fmt.Fprintln(out)
	}
}

// applyAdditive writes the model-roles merge and the replaced stubs.
func (x *extras) applyAdditive(out io.Writer, targetDir string) error {
	if x.roles != nil {
		if err := os.WriteFile(filepath.Join(targetDir, filepath.FromSlash(template.ModelRolesPath)), x.roles, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "  Merged %s (+%d keys).\n", template.ModelRolesPath, len(x.rolesAdded))
	}
	for _, f := range x.oldStubs {
		if err := template.CopyFile(targetDir, f.rel, f.dest); err != nil {
			return err
		}
		fmt.Fprintf(out, "  Replaced the old model's stub %s.\n", f.dest)
	}
	return nil
}

// offerPrune removes the old model's replaced files with --prune, or after a yes.
func (x *extras) offerPrune(out io.Writer, targetDir string) error {
	if len(x.old) == 0 {
		return nil
	}
	if !pruneOld && !confirm(fmt.Sprintf("Remove the old model's %d replaced file(s) listed above?", len(x.old))) {
		fmt.Fprintln(out, "  Kept them. `clauductor diff` lists them as extra; `--prune` removes them.")
		return nil
	}
	removed, err := template.PruneOldModel(targetDir, x.old)
	for _, r := range removed {
		fmt.Fprintf(out, "  Removed %s\n", r)
	}
	return err
}

// writeDocFile creates a project-owned file from the template: panel.json rendered with the
// project's branch keys, anything else copied.
func writeDocFile(targetDir, tmplDir string, f placed, conf *template.Conf) error {
	if f.rel != template.PanelPath {
		return template.CopyFile(targetDir, f.rel, f.dest)
	}
	tmpl, err := os.ReadFile(filepath.Join(tmplDir, filepath.FromSlash(f.rel)))
	if err != nil {
		return err
	}
	out, err := template.RenderPanel(tmpl, conf)
	if err != nil {
		return err
	}
	dst := filepath.Join(targetDir, filepath.FromSlash(f.dest))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, out, 0o644)
}
