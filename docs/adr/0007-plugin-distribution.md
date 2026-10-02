# ADR 0007: The model also ships as a Claude Code plugin, generated from template/

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: plugin, template, distribution
- **Source**: OPS-11 (#24); docs/plugin.md

## Context
`clauductor install` needs the Go binary. A contributor who only has Claude Code (Standing Tee's
designer) could not get the model's skills and hooks without it. Claude Code plugins distribute
skills, agents and hooks through a marketplace, read-only and versioned.

## Decision
- `plugin/` is built from `template/` by `clauductor plugin build` (`scripts/build-plugin.sh`) and
  committed; `.claude-plugin/marketplace.json` lists it. `template/` stays the one source.
- Framework files live in the plugin cache, paths rewritten to `${CLAUDE_PLUGIN_ROOT}`; project
  files are scaffolded once by `/clauductor:init`, which never overwrites.
- One way per repository: the binary install and the plugin each detect the other's marker and
  refuse (ADR-0004); the plugin's hooks stand aside in a repository it did not set up.

## Consequences
### Positive
- The model reaches Claude Code users without the binary; updates arrive through `/plugin update`.

### Negative / trade-offs
- Every template edit must be followed by a rebuild, or the plugin ships stale.
- Skill names differ (`/clauductor:session-start` vs `/session-start`), and the rewriter must place
  every new template file (a file it cannot place fails the build).

## Enforcement
`TestCommittedPluginIsCurrent` (CI) fails when `plugin/` differs from a fresh build; this repo's
gate runs `clauductor plugin check` from source (`scripts/ci/steps.sh`); the plugin tests run the
checks in a scaffolded repository on macOS and Linux.

## Related
- ADR-0004, ADR-0006
