package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A repository can run its own operating model — skills, hooks and scripts with the same names as
// the template's, grown in place. `install` overwrites the template's framework tier and `update`
// adds whatever is missing, so run in such a repo either one would replace or scatter files the
// project owns. Both therefore act only on a repository clauductor put its model into (the marker,
// or an old-model install), and otherwise refuse unless --force, naming what they would touch.

// installMarker records that clauductor installed the operating model here, so later installs and
// updates may bring its files up to date.
const installMarker = ".claude/clauductor-template"

const markerNote = "The operating model in this repository was installed by `clauductor install` or `init`.\n" +
	"`clauductor install` and `update` may bring its framework files up to date. Delete this file to\n" +
	"make them refuse (unless --force), as they do in a repository that runs its own model.\n"

// ownedByClauductor says whether clauductor installed the model here: the marker, or the old
// model's orchestration config (installs made before the marker existed).
func ownedByClauductor(targetDir string) bool {
	return fileExists(filepath.Join(targetDir, installMarker)) ||
		fileExists(filepath.Join(targetDir, "orchestration", "config.json"))
}

// writeInstallMarker marks targetDir as clauductor's to update.
func writeInstallMarker(targetDir string) error {
	p := filepath.Join(targetDir, installMarker)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(markerNote), 0o644)
}

// ownModelSigns are the files that show a repository runs an operating model of its own.
var ownModelSigns = []string{"AGENTS.md", ".claude/skills", ".claude/hooks", ".claude/settings.json"}

// foreignModel lists what an install into targetDir, not clauductor's, would change in a model the
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
	for _, rel := range files {
		dst := filepath.Join(targetDir, rel)
		if !fileExists(dst) {
			if own && classifyFile(rel) == tierFramework {
				add = append(add, rel)
			}
			continue
		}
		if classifyFile(rel) != tierFramework {
			continue // docs are kept and CLAUDE.md is merged: never replaced
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
			overwrite = append(overwrite, rel)
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
			fmt.Fprintf(&b, "    ~ %s\n", f)
		}
	}
	if len(add) > 0 {
		fmt.Fprintf(&b, "\nIt would ADD %d framework file(s) beside the project's own:\n", len(add))
		for _, f := range add {
			fmt.Fprintf(&b, "    + %s\n", f)
		}
	}
	b.WriteString("\nThe panel needs none of this: `clauductor panel` works in any repository as it is.\n" +
		"To borrow one piece, copy it from the template by hand. To replace the project's model\n" +
		"with the template's anyway, run again with --force (commit first: it overwrites).")
	return fmt.Errorf("%s", b.String())
}

// pluginMarker is what the clauductor plugin's /clauductor:init writes: the repository runs the
// model from the plugin (docs/plugin.md).
const pluginMarker = ".claude/clauductor-plugin"

// refusePluginModel refuses install and update in a repository that runs the model from the
// plugin. Installing the framework files too would register every hook twice and give each
// skill two copies that drift apart.
func refusePluginModel(verb, targetDir string) error {
	if !fileExists(filepath.Join(targetDir, pluginMarker)) {
		return nil
	}
	return fmt.Errorf("%s refused: %s runs the operating model from the clauductor Claude Code plugin (%s).\n"+
		"Run one or the other, not both (docs/plugin.md, Running both). To switch this repository to\n"+
		"`clauductor install`, uninstall or disable the plugin here and delete %s, then run again;\n"+
		"--force installs anyway", verb, targetDir, pluginMarker, pluginMarker)
}
