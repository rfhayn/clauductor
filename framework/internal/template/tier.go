package template

import "strings"

// Tier is how install, update and diff treat a template file. It lives here, not in the
// command, so FindDiffs derives what `update` refreshes from the same classification `install`
// uses: a hand list of paths drifted and left five framework scripts un-updated (B3).
type Tier int

const (
	TierFramework Tier = iota // the model's own code: always installed, refreshed by update
	TierDoc                   // project-owned config, agents and docs: created only if missing
	TierConfig                // CLAUDE.md, .gitignore, .gitattributes: merged line-wise
	TierSettings              // .claude/settings.json: merged key by key (MergeSettings)
)

// SettingsPath is the one settings file the template ships.
const SettingsPath = ".claude/settings.json"

// FrameworkScripts are the operating model's own scripts outside .claude/: a project runs them
// but does not edit them, so an install brings them up to date. Every script directly under
// .claude/ (statusline.sh, roadmap-queue.sh, metrics.sh, ...) is the model's too, by rule rather
// than by list (isModelScript): a list here missed each new one (B3, and again metrics.sh and
// usage-report.sh), where the plugin's packager, placing by the same rule, did not.
var FrameworkScripts = map[string]bool{
	"scripts/ci/run-local.sh": true,
	"scripts/ci/gate.sh":      true,
	"scripts/ci/lease.sh":     true,
	// The model's gate steps as a library: a project's own runner calls them too (P1.6).
	"scripts/ci/lib/steps.sh": true,
}

// isModelScript: a shell script directly under .claude/.
func isModelScript(rel string) bool {
	rest, ok := strings.CutPrefix(rel, ".claude/")
	return ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".sh")
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
	// The lease conformance kit: the suite IS the protocol, so a project runs it as shipped and
	// update refreshes it (P1.12).
	"scripts/ci/lease-conformance/",
}

// LocalDir is the project's own extension layer (.claude/local/README.md): guard rules, context
// sections, health lines, checks, skill fragments and conflict rows that install and update never
// write over. It is the doc tier BY RULE, tested first, so no framework entry (a FrameworkDirs
// prefix, the model-script rule) can ever claim a file under it (P1.2).
const LocalDir = ".claude/local/"

// ProjectSkills are template skills a project configures (CONFIGURE FIRST stubs): created when
// missing, never overwritten, like docs.
var ProjectSkills = []string{".claude/skills/architecture-audit/", ".claude/skills/release-prep/"}

// Classify determines how a template file is handled.
func Classify(relPath string) Tier {
	if strings.HasPrefix(relPath, LocalDir) {
		return TierDoc
	}
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
	if FrameworkScripts[relPath] || isModelScript(relPath) {
		return TierFramework
	}
	if relPath == "CLAUDE.md" || relPath == ".gitignore" || relPath == GitattributesPath {
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
