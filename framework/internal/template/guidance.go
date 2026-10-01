package template

import (
	"os"
	"path/filepath"
)

// Doc-tier files are created only if missing and never overwritten, because a project edits them.
// So a project installed before a doc changed (the playbook's machine setup, say) never sees the
// new text unless something tells it. The guidance docs are the doc-tier files whose content is
// the model's advice rather than the project's own record or configuration; `update` names the
// ones that differ, and `diff` notes each, so the new content is discoverable without being forced.

// guidanceDocs are the template's guidance docs: prose a project reads and rarely edits, so a
// difference most likely means the template moved on. An ALLOW list, not a deny list: most
// doc-tier files hold the project's own content (AGENTS.md rows, the ADR index, eval cases, health
// scripts, dependabot.yml, records, config), differ from the template by design, and would make the
// notice fire on every update forever, which teaches everyone to ignore it.
// TestGuidanceDocsAreTemplateDocs holds each entry to a doc-tier template file.
var guidanceDocs = map[string]bool{
	"docs/playbook.md":         true,
	"docs/conventions.md":      true,
	"docs/principles.md":       true,
	"docs/adr/TEMPLATE.md":     true,
	"changes/README.md":        true,
	"specs/README.md":          true,
	"scripts/ci/README.md":     true,
	".claude/health/README.md": true,
}

const guidanceNote = "differs from the template's copy (update never overwrites it: compare and take what you want)"

// IsGuidance says whether rel (a template path) is one of the template's guidance docs.
func IsGuidance(rel string) bool { return guidanceDocs[rel] }

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
