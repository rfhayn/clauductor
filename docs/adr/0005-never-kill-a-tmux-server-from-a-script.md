# ADR 0005: No script kills a tmux server, or a claude or clauductor process

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: process, panel, template
- **Source**: the 2026-09-30 incident (both of the owner's live Standing Tee lanes died at once)

## Context
On 2026-09-30 every lane of the owner's installed panel died at once. The cause was Standing Tee's
`.claude/machine-quiet.sh`: its step 1 killed orphaned processes (parent pid 1) whose command line
named `.claude/worktrees/`. The panel's daemonized tmux server matches exactly: it keeps the
`new-session -c <worktree>` argv it was started with. A cleanup script that pattern-matches
command lines will eventually match the one process every lane lives in.

## Decision
- No script in the model kills a tmux server or client, a `claude` process or a `clauductor`
  process, whatever its argv or parent says. machine-quiet kills only orphans of a KNOWN leak shape
  (`MQ_ORPHAN_SHAPES`), never "anything under `.claude/worktrees/`".
- Agents and tests use throwaway tmux sockets (`tmux -L <tmp>`), a temp HOME and `:0` ports, and
  never touch the owner's sockets.

## Consequences
### Positive
- A session-close can no longer take down live lanes.

### Negative / trade-offs
- A genuinely orphaned tmux server is left running; the panel's restore handles a lost server,
  and the owner can end it by hand.

## Enforcement
`template/.claude/machine-quiet.sh` excludes tmux, claude and clauductor by argv[0] whatever the
shapes list; `checks/machine-quiet.sh` starts an orphaned tmux-shaped process whose argv names a
worktree and fails if a dry run would kill it (falsified beside a hook-shaped orphan that IS
listed). The panel's tests fail if they leave a tmux server behind (`TestMain`). AGENTS.md
essentials name the rule for sessions; nothing checks an ad-hoc command an agent types.

## Related
- ADR-0003
