package template

import "strings"

// Tier is how install, update and diff treat a template file. It lives here, not in the
// command, so FindDiffs derives what `update` refreshes from the same classification `install`
// uses: a hand list of paths drifted and left five framework scripts un-updated (B3).
type Tier int

const (
	TierFramework Tier = iota // the model's own code: always installed, refreshed by update
	TierDoc                   // project-owned config, agents and docs: created only if missing
	TierConfig                // CLAUDE.md, .gitignore: merged line-wise
	TierSettings              // .claude/settings.json: merged key by key (MergeSettings)
)

// SettingsPath is the one settings file the template ships.
const SettingsPath = ".claude/settings.json"

// FrameworkScripts are the operating model's own scripts outside the framework directories: a
// project runs them but does not edit them, so an install brings them up to date.
var FrameworkScripts = map[string]bool{
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

// FrameworkDirs are the template directories whose every file is the model's code. A file a
// project adds under one of them is reported by `clauductor diff` as extra.
var FrameworkDirs = []string{
	".claude/skills/",
	".claude/hooks/",
	".claude/checks/",
	".claude/lib/",
	".claude/workflows/",
	".claude/modules/",
	".claude/examples/",
}

// ProjectSkills are template skills a project configures (CONFIGURE FIRST stubs): created when
// missing, never overwritten, like docs.
var ProjectSkills = []string{".claude/skills/architecture-audit/", ".claude/skills/release-prep/"}

// Classify determines how a template file is handled.
func Classify(relPath string) Tier {
	for _, p := range ProjectSkills {
		if strings.HasPrefix(relPath, p) {
			return TierDoc
		}
	}
	if relPath == SettingsPath {
		return TierSettings
	}
	// The project's own values live elsewhere (.claude/project.conf, model-roles.json,
	// .clauductor/panel.json, scripts/ci/steps.sh, AGENTS.md, docs/), which fall through to the
	// doc tier and are never overwritten.
	for _, d := range FrameworkDirs {
		if strings.HasPrefix(relPath, d) {
			return TierFramework
		}
	}
	if FrameworkScripts[relPath] {
		return TierFramework
	}
	if relPath == "CLAUDE.md" || relPath == ".gitignore" {
		return TierConfig
	}
	// Agents, docs, README and the rest: create only if missing.
	return TierDoc
}

// TierLabel names a tier for output.
func TierLabel(t Tier) string {
	switch t {
	case TierFramework:
		return "framework"
	case TierDoc:
		return "doc"
	case TierConfig:
		return "config"
	case TierSettings:
		return "settings"
	default:
		return "unknown"
	}
}
