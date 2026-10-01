# ADR 0003: One panel serves every repository on the machine

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: panel
- **Source**: PANEL-16 (#18); PRD-change-process, OPS-8 "The panel"

## Context
The panel began as one process per project (`clauductor panel` in a repo, one port, one tmux
socket). The owner runs Standing Tee and clauductor side by side, and quota, cost and notifications
are per person, not per repo. Two panels would split the quota picture, contend for the launchd
agent and the default port, and double the notifications.

## Decision
- One panel process per user, registered with a project menu; each project keeps its own
  `.clauductor/panel.json`, its own tmux socket and its own trust decision.
- A repository is added explicitly (`panel trust`, `panel add`); nothing is discovered and run.
- Clauductor's own repo is the second project, the first real multi-repo use (roadmap OPS-15).

## Consequences
### Positive
- One quota and cost view, one place for notifications, one launchd agent.

### Negative / trade-offs
- A fault in the panel affects every project's lanes at once; lane state must survive a panel
  restart (the registry restores lanes).
- Per-project config must not leak between projects (tmux sockets, lane templates, cards).

## Enforcement
The panel's tests (`framework/internal/panel`, run by CI with `-race` on macOS and Ubuntu) once
PANEL-16 merges. Until then nothing enforces it: the decision is ahead of the code.

## Related
- ADR-0005 (the panel's tmux server), ADR-0006 (nothing needs the panel)
