# ADR 0004: install and update refuse a repository that runs a model clauductor did not install

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: cli, template
- **Source**: OPS-1 (#19, the "OPS-6" guard); OPS-11 (#24, the plugin marker); OPS-8 rehearsal

## Context
`clauductor install` overwrites the template's framework tier (skills, hooks, checks, settings)
and `update` adds what is missing. Run in a repository that grew its own operating model
(Standing Tee has skills and hooks with the same names), either would replace or scatter files the
project owns. The plugin and the binary install the same hooks, so running both would judge every
Bash call twice.

## Decision
- Install and update act only where clauductor put the model: the `.claude/clauductor-template`
  marker, or the old model's `orchestration/config.json`.
- Elsewhere, when any sign of an own model exists (`AGENTS.md`, `.claude/skills`, `.claude/hooks`,
  `.claude/settings.json`), they refuse and list every file they would OVERWRITE and ADD. `--force`
  proceeds; the refusal says to commit first.
- A repository with the plugin's marker (`.claude/clauductor-plugin`) is refused unless `--force`;
  `/clauductor:init` refuses a binary-installed repository even with `--force`.

## Consequences
### Positive
- A project's own model is never clobbered by a casual `install`.

### Negative / trade-offs
- OPS-8 found the old-model sign is runtime state (`orchestration/` is gitignored), so a fresh
  clone of an old-model repo reads as foreign; and `--force` leaves old-model files the template
  no longer has (16 skills, 2 agents, 2 hooks here) without naming them. Owned by OPS-14.

## Enforcement
`framework/internal/cmd/ownguard.go`, tested by `ownguard_test.go` (CI); the plugin side by
`framework/internal/plugin` tests.

## Related
- ADR-0007 (plugin distribution); `docs/plugin.md`, "Running both"
