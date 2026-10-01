# ADR 0006: A repository works without clauductor installed

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: template, panel, plugin
- **Source**: PRD-change-process D10 (OPS-12, #23); built in OPS-8

## Context
Standing Tee's designer works without clauductor, possibly on Windows. If a gate step, a hook or a
skill quietly called the binary or the panel, the repository would work for the owner and break
for every other contributor, and nothing on the owner's machine would show it.

## Decision
- Everything a contributor needs lives in the repo and runs in Claude Code alone: skills, hooks,
  checks, the gate and its queue lock (`lease.sh` when there is no binary), the merge guard and
  the records.
- The panel, the binary and the plugin are conveniences on top, never dependencies.
- Anything that talks to the panel treats a missing panel as a normal state: the status line's
  post, session-start's panel line (no longer reported as unhealthy), the economy file, the cards.

## Consequences
### Positive
- A second contributor needs only Claude Code, git, jq and the project's own toolchain.

### Negative / trade-offs
- Every check runs a second time inside `no-clauductor.sh` (about 45 s in the template), and every
  new panel feature needs a no-panel path.

## Enforcement
`template/.claude/checks/no-clauductor.sh`: with `clauductor` absent from PATH (its directory
mirrored minus the binary) and a HOME with no panel, it runs the gate in a throwaway repo, every
other process check, session-start's context script and the status line, and fails on an error or
a line asking for the binary or the panel; it scans `GATE_STEPS` for a bare `clauductor` call.
Falsified in fixtures (a gate step and a context script that call it) and by hand during OPS-8 (a
hard call in `steps.sh`, and in `statusline.sh`, each failed the check). Named in AGENTS.md's table
and the playbook. This repo's own gate runs it, and its plugin step uses `go run`, not the binary.

## Related
- ADR-0007; Standing Tee gets the same check at convergence (roadmap ST-3)
