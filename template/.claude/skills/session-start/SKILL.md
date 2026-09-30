---
name: session-start
model: opus
effort: low
description: "Orient at the start of a working session: the owner queue, the panel, git and PR state, the change queue, the latest journal entry, insights, ADRs and health lines, all computed; then set the status-line focus and run hands-off to session-close. TRIGGER at the start of a session or when the user says 'session start', 'where were we', 'get oriented', 'catch up'."
---

# Session start: orient, then run the lanes

## Context: the state, computed
!`sh .claude/skills/session-start/context.sh`

If the line above shows as literal text instead of output, this harness does not pre-execute it:
run `sh .claude/skills/session-start/context.sh` yourself and read the result before step 1.

## Steps
1. **Summarize in 3–5 lines**: what the last session did (the journal), what is in flight (open
   PRs, lane worktrees, uncommitted changes), and the next step (the queue's NEXT row, the
   journal's "What's next"). **Then list every owner-queue item**, one line each with what it needs
   first. `CANNOT CHECK` there is not an empty queue.
2. **Say every line that is not healthy**: each health line that is not `OK` (a `CANNOT CHECK` is
   not a pass), a queue `ERROR` (the queue is then UNKNOWN: stop treating it as known), the main
   checkout not on `main`, the panel down. One sentence each, even when the session is about
   something else.
3. **Other people's work.** Name every `★` PR in one line. An `OVERLAP` line means their open PR
   changes a file your branch changes: read their diff before touching that file, and do not build
   on anything it removes. Never merge another person's PR.
4. **Set the focus**: `sh .claude/status-write.sh "[<branch>] <short focus>"`. The panel shows it
   on the lane's card.
5. **Flag what needs attention before new work**: uncommitted changes, a `Raw` insight that should
   be promoted, a `Proposed` ADR awaiting the owner, a "What's next" that is now stale, two changes
   proposed at once.

## Rules
- **From here the session runs hands-off to `session-close`**: build, review, merge through
  `merge-pr`, close. It interrupts the owner only through `PushNotification`, when an owner
  decision or a block stops it (`session-close`, *Hands-off*).
- **The main checkout stays on `main`.** Hooks run from it; a feature branch there makes
  `worktree-hook-drift.sh` refuse every worktree agent.
- **Run as an orchestrator of lanes** (`docs/playbook.md`, *Lanes*): one build lane at a time in its
  OWN worktree (a panel lane, or `git worktree add .claude/worktrees/<lane> -b change/<id>
  origin/main` then `EnterWorktree` there before starting `/build-change`, and stay there until it
  returns: a workflow agent takes the session's cwd when it starts); at most one proposal ahead;
  `fix/` and `ops/` lanes in their own worktrees when they share no files. Take summaries from
  lanes rather than reading their files. When a lane frees up, offer the next queue row.
- **Research-only work goes to the `researcher` agent**, not a fork (a fork inherits every tool).
- Read, don't assume: the repo's records are the source of truth for "where we were". Keep the
  orientation short; this is a launchpad, not a report.
