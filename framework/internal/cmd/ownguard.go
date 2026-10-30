package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rfhayn/stridal/internal/brand"
	"github.com/rfhayn/stridal/internal/template"
)

// A repository can run its own operating model — skills, hooks and scripts with the same names as
// the template's, grown in place. `install` overwrites the template's framework tier and `update`
// adds whatever is missing, so run in such a repo either one would replace or scatter files the
// project owns. Both therefore act only on a repository stridal put its model into (the marker,
// or an old-model install), and otherwise refuse unless --force, naming what they would touch.

// installMarker records that stridal installed the operating model here, so later installs and
// updates may bring its files up to date.
var installMarker = brand.InstallMarker(brand.Name)

// markerIn is marker when targetDir holds it, or "" when it does not.
func markerIn(targetDir, marker string) string {
	if fileExists(filepath.Join(targetDir, marker)) {
		return marker
	}
	return ""
}

// hasInstallMarker says whether targetDir holds the install marker.
func hasInstallMarker(targetDir string) bool { return markerIn(targetDir, installMarker) != "" }

const markerNote = "The operating model in this repository was installed by `stridal install` or `init`.\n" +
	"`stridal install` and `update` may bring its framework files up to date. Delete this file to\n" +
	"make them refuse (unless --force), as they do in a repository that runs its own model.\n"

// ownedByStridal says whether stridal installed the model here: the marker, or signs of
// stridal's old model (installs made before the marker existed). The signs are read from
// tracked files (template.OldModelSigns): the old model's runtime state, orchestration/config.json,
// is gitignored, so a fresh clone of an old-model repository looked foreign (OPS-8 rehearsal).
func ownedByStridal(targetDir string) bool {
	return hasInstallMarker(targetDir) || len(template.OldModelSigns(targetDir)) > 0
}

// announceOldModel says why a repository without the marker counts as stridal's.
func announceOldModel(out io.Writer, targetDir string) {
	if hasInstallMarker(targetDir) {
		return
	}
	signs := template.OldModelSigns(targetDir)
	if len(signs) == 0 {
		return
	}
	fmt.Fprintln(out, "This repository runs stridal's old operating model, so it is stridal's to update:")
	for _, s := range signs {
		fmt.Fprintf(out, "    · %s\n", s)
	}
	fmt.Fprintln(out)
}

// writeInstallMarker marks targetDir as stridal's to update.
func writeInstallMarker(targetDir string) error {
	p := filepath.Join(targetDir, installMarker)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(markerNote), 0o644)
}

// ownModelSigns are the files that show a repository runs an operating model of its own.
var ownModelSigns = []string{"AGENTS.md", ".claude/skills", ".claude/hooks", ".claude/settings.json"}

// foreignModel lists what an install into targetDir, not stridal's, would change in a model the
// project already has: each framework-tier file that exists with other content ("overwrite"), and,
// when any sign of an own model is present, each template file that would be added beside it
// ("add"). Empty means the install cannot clobber anything the project owns.
func foreignModel(targetDir, tmplDir string, files []string) (overwrite, add []string, err error) {
	own := false
	for _, s := range ownModelSigns {
		if fileExists(filepath.Join(targetDir, s)) {
			own = true
			break
		}
	}
	// Compare where the project keeps each file (its project.conf may move the gate); an
	// unreadable mapping falls back to the template's paths, which only over-reports.
	var pm *template.PathMap
	if conf, err := template.LoadConf(targetDir, tmplDir); err == nil {
		pm, _ = conf.Mapping(files)
	}
	for _, rel := range files {
		t := classifyFile(rel)
		if t != tierFramework && t != tierSettings {
			continue // docs are kept and CLAUDE.md is merged: never replaced
		}
		dest, _, skip := pm.Resolve(rel)
		if skip != "" {
			continue
		}
		dst := filepath.Join(targetDir, filepath.FromSlash(dest))
		if !fileExists(dst) {
			if own {
				add = append(add, dest)
			}
			continue
		}
		have, err := os.ReadFile(dst)
		if err != nil {
			return nil, nil, err
		}
		want, err := os.ReadFile(filepath.Join(tmplDir, rel))
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(have, want) {
			overwrite = append(overwrite, dest)
		}
	}
	sort.Strings(overwrite)
	sort.Strings(add)
	return overwrite, add, nil
}

// refuseForeign is the error install and update return in a repository that runs its own model.
func refuseForeign(verb, targetDir string, overwrite, add []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s refused: %s runs an operating model of its own (no %s).\n", verb, targetDir, installMarker)
	if len(overwrite) > 0 {
		fmt.Fprintf(&b, "\nIt would OVERWRITE %d file(s) the project has changed:\n", len(overwrite))
		for _, f := range overwrite {
			if f == template.SettingsPath {
				fmt.Fprintf(&b, "    ~ %s (merged: the model's hooks, status line and entries updated, the project's keys kept)\n", f)
				continue
			}
			fmt.Fprintf(&b, "    ~ %s\n", f)
		}
	}
	if len(add) > 0 {
		fmt.Fprintf(&b, "\nIt would ADD %d framework file(s) beside the project's own:\n", len(add))
		for _, f := range add {
			fmt.Fprintf(&b, "    + %s\n", f)
		}
	}
	if files, err := template.ListTemplateFiles(); err == nil {
		for _, w := range template.SkillCollisions(targetDir, files) {
			fmt.Fprintf(&b, "\nWARNING: %s\n", w)
		}
	}
	b.WriteString("\nTo see every difference, file by file and key by key: stridal diff.\n")
	b.WriteString("\nThe panel needs none of this: `stridal panel` works in any repository as it is.\n" +
		"To borrow one piece, copy it from the template by hand. To replace the project's model\n" +
		"with the template's anyway, run again with --force (commit first: it overwrites).")
	return fmt.Errorf("%s", b.String())
}

// pluginMarker is what the stridal plugin's /stridal:init writes: the repository runs the
// model from the plugin (docs/plugin.md).
var pluginMarker = brand.PluginMarker(brand.Name)

// refusePluginModel refuses install and update in a repository that runs the model from the
// plugin. Installing the framework files too would register every hook twice and give each
// skill two copies that drift apart.
func refusePluginModel(verb, targetDir string) error {
	marker := markerIn(targetDir, pluginMarker)
	if marker == "" {
		return nil
	}
	return fmt.Errorf("%s refused: %s runs the operating model from the stridal Claude Code plugin (%s).\n"+
		"Run one or the other, not both (docs/plugin.md, Running both). To switch this repository to\n"+
		"`stridal install`, uninstall or disable the plugin here and delete %s, then run again;\n"+
		"--force installs anyway", verb, targetDir, marker, marker)
}
