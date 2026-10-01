package template

import (
	"os"
	"path/filepath"
	"strings"
)

// Doc-tier files are created only if missing and never overwritten, because a project edits them.
// So a project installed before a doc changed (the playbook's machine setup, say) never sees the
// new text unless something tells it. The guidance docs are the doc-tier files whose content is
// the model's advice rather than the project's own record or configuration; `update` names the
// ones that differ, and `diff` notes each, so the new content is discoverable without being forced.

// projectContent are doc-tier files whose content is the project's by design: records, its
// configuration, its README. They always differ from the template, so naming them says nothing.
var projectContent = map[string]bool{
	"README.md":                   true,
	".claude/project.conf":        true,
	".claude/model-roles.json":    true,
	".clauductor/panel.json":      true,
	"scripts/ci/steps.sh":         true,
	"docs/roadmap.md":             true,
	"docs/development-journal.md": true,
	"docs/insights-log.md":        true,
	"docs/owner-queue.md":         true,
}

const guidanceNote = "differs from the template's copy (update never overwrites it: compare and take what you want)"

// IsGuidance says whether rel is a doc-tier template file that carries the model's guidance.
// Agents are left out: update already offers them file by file.
func IsGuidance(rel string) bool {
	if Classify(rel) != TierDoc || projectContent[rel] || strings.HasPrefix(rel, ".claude/agents/") {
		return false
	}
	for _, p := range ProjectSkills {
		if strings.HasPrefix(rel, p) {
			return false // CONFIGURE FIRST stubs the project fills in
		}
	}
	return true
}

// GuidanceDrift lists the guidance docs, at the project's paths, that differ from the template's
// copy and those the project lacks.
func GuidanceDrift(targetDir string) (differs, missing []string, err error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, nil, err
	}
	files, err := listDir(tmplPath)
	if err != nil {
		return nil, nil, err
	}
	conf, err := LoadConf(targetDir, tmplPath)
	if err != nil {
		return nil, nil, err
	}
	pm, err := conf.Mapping(files)
	if err != nil {
		return nil, nil, err
	}
	for _, rel := range files {
		if !IsGuidance(rel) {
			continue
		}
		dest, _, skip := pm.Resolve(rel)
		if skip != "" {
			continue
		}
		dst := filepath.Join(targetDir, filepath.FromSlash(dest))
		if _, err := os.Stat(dst); err != nil {
			missing = append(missing, dest)
		} else if !FilesEqual(filepath.Join(tmplPath, filepath.FromSlash(rel)), dst) {
			differs = append(differs, dest)
		}
	}
	return differs, missing, nil
}
