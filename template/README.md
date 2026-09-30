# Project Name

> Set up with [Clauductor](https://github.com/rfhayn/clauductor)'s operating model for Claude
> Code: hands-off sessions, one lane per git worktree on the local panel, a roadmap queue,
> approved proposals, per-task-group review, and guards that execute the rules.

Start with **`docs/playbook.md`**: it explains the whole model, lane by lane.

## First run

```bash
claude
/start-project      # fills .claude/project.conf, the gate steps and AGENTS.md's essentials
/session-start      # every session
```

Then `clauductor panel` for the local panel (see the clauductor repo's `docs/panel.md`).

## Where things are

| Path | What |
|---|---|
| `AGENTS.md` (`CLAUDE.md` imports it) | The rules every agent loads, and what executes each |
| `.claude/project.conf` | Every project-specific name and path the scripts read |
| `.claude/model-roles.json` | The one place a model or effort is chosen; the commit trailer |
| `.claude/skills/`, `.claude/agents/`, `.claude/hooks/` | The operating model |
| `.claude/checks/run.sh` | Plain-shell checks that hold the model to what AGENTS.md says |
| `docs/roadmap.md` | Phases and the change queue |
| `changes/<id>/` | A proposed or in-flight change: proposal, design, tasks |
| `specs/` | What the system does, per capability |
| `docs/adr/` | Decisions |
| `docs/development-journal.md`, `docs/insights-log.md` | The session record and raw lessons |
| `docs/owner-queue.md` | What needs the owner at the computer |
