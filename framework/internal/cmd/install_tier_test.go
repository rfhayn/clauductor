package cmd

import "testing"

// An install brings the operating model's own code up to date and never overwrites a value the
// project owns: its config, its roles, its panel config, its gate steps, its rules and records,
// and the skills it configures.
func TestClassifyFile(t *testing.T) {
	for path, want := range map[string]fileTier{
		".claude/skills/merge-pr/SKILL.md":                       tierFramework,
		".claude/skills/merge-pr/review-lane.sh":                 tierFramework,
		".claude/hooks/pr-merge-guard.sh":                        tierFramework,
		".claude/hooks/lib/ci-receipt.sh":                        tierFramework,
		".claude/checks/run.sh":                                  tierFramework,
		".claude/lib/conf.sh":                                    tierFramework,
		".claude/workflows/build-change.js":                      tierFramework,
		".claude/modules/openspec/README.md":                     tierFramework,
		".claude/settings.json":                                  tierSettings,
		".claude/compound.sh":                                    tierFramework,
		".claude/statusline.sh":                                  tierFramework,
		".claude/roadmap-queue.sh":                               tierFramework,
		".claude/scenario-trace.sh":                              tierFramework,
		".claude/change-approval.sh":                             tierFramework,
		".claude/change-cost.sh":                                 tierFramework,
		".claude/verify-change.sh":                               tierFramework,
		".claude/metrics.sh":                                     tierFramework,
		".claude/usage-report.sh":                                tierFramework,
		".claude/health/flow.sh":                                 tierDoc,
		".claude/examples/changes/add-greeting-name/proposal.md": tierFramework,
		"scripts/ci/run-local.sh":                                tierFramework,
		"scripts/ci/lease.sh":                                    tierFramework,
		".claude/skills/release-prep/SKILL.md":                   tierDoc,
		".claude/skills/architecture-audit/SKILL.md":             tierDoc,
		".claude/agents/builder.md":                              tierDoc,
		".claude/project.conf":                                   tierDoc,
		".claude/model-roles.json":                               tierDoc,
		".claude/health/worktrees.sh":                            tierDoc,
		".clauductor/panel.json":                                 tierDoc,
		"scripts/ci/steps.sh":                                    tierDoc,
		"AGENTS.md":                                              tierDoc,
		"docs/roadmap.md":                                        tierDoc,
		".github/dependabot.yml":                                 tierDoc,
		"CLAUDE.md":                                              tierConfig,
		".gitignore":                                             tierConfig,
	} {
		if got := classifyFile(path); got != want {
			t.Errorf("classifyFile(%q) = %s, want %s", path, tierLabel(got), tierLabel(want))
		}
	}
}
