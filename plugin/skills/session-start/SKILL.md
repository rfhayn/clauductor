---
name: session-start
model: opus
effort: low
description: "Orient at the start of a working session: the owner queue, the panel, git and PR state, the change queue, the latest journal entry, insights, ADRs and health lines, all computed; then set the status-line focus and run hands-off to session-close. TRIGGER at the start of a session or when the user says 'session start', 'where were we', 'get oriented', 'catch up'."
---

# Session start: orient, then run the lanes

## Context: the state, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/session-start/context.sh`
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh ${CLAUDE_PLUGIN_ROOT}/project-config.sh prefixes`

If the line above shows as literal text instead of output, this harness does not pre-execute it:
run `clauductor-model skills/session-start/context.sh` yourself and read the result before step 1.

## Steps
1. **Summarize in 3–5 lines**: what the last session did (the journal), what is in flight (open
   PRs, lane worktrees, uncommitted changes), and the next step (the queue's NEXT row, the
   journal's "What's next"). **Then list every owner-queue item**, one line each with what it needs
   first. `CANNOT CHECK` there is not an empty queue. Then repeat the `Health: flow` line as one
   **Flow** line (cycle time, merges per week, change-fail rate, spend per week, work in flight).
2. **Say every line that is not healthy**: each health line that is not `OK` (a `CANNOT CHECK` is
   not a pass), a queue `ERROR` (the queue is then UNKNOWN: stop treating it as known), the main
   checkout not on `main`. One sentence each, even when the session is about something else. A
   panel that is not running is a normal state, not an unhealthy line (the repository works
   without clauductor: `checks/no-clauductor.sh`).
3. **Other people's work.** Name every `★` PR in one line. An `OVERLAP` line means their open PR
   changes a file your branch changes: read their diff before touching that file, and do not build
   on anything it removes. Never merge another person's PR.
4. **Set the focus**: `clauductor-model status-write.sh "[<branch>] <short focus>"`. The panel shows it
   on the lane's card.
   **A row's session** (one roadmap row, in its own worktree on the row's branch) then marks its
   start: `clauductor-model skills/session-start/marker.sh [<row id> [S<n>]]`. With no arguments it finds
   the row whose branch this is and its session in the plan (on a branch no row names, it says so
   and does nothing), which is how the panel's row templates run it. It opens the row's PR as a draft whose body starts
   `session-start: S<n> <YYYY-MM-DD HH:MM ET> row <id>` and the session-start cost, which the row's
   fragment copies at close (`session-close`, *Two forms*). It never blocks: a failure is one
   `marker: WARNING` line to say, and the session goes on.
5. **Flag what needs attention before new work**: uncommitted changes, a `Raw` insight that should
   be promoted, a `Proposed` ADR or practice record awaiting the owner, a "What's next" that is
   now stale, two changes proposed at once.

## Rules
- **From here the session runs hands-off to `session-close`**: build, review, merge through
  `merge-pr`, close. It interrupts the owner only through `PushNotification`, when an owner
  decision or a block stops it (`session-close`, *Hands-off*). While a lane or anything else
  runs, a question for the owner goes to the owner queue or a push, never a blocking
  `AskUserQuestion`; and no agent ends its turn on a command it started that is still running.
  `${CLAUDE_PLUGIN_ROOT}/hooks/no-silent-wait.sh` refuses both for THIS session's agents, teammates and
  background commands; a lane that is a session of its own (a panel lane) it cannot see.
- **The main checkout stays on `main`.** Hooks run from it; a feature branch there makes
  `worktree-hook-drift.sh` refuse every worktree agent.
- **Run as an orchestrator of lanes** (`docs/playbook.md`, *Lanes*): one build lane at a time in its
  OWN worktree (a panel lane, or `git worktree add .claude/worktrees/<lane> -b <BRANCH_CHANGE><id>
  origin/main` then `EnterWorktree` there before starting `/build-change`, and stay there until it
  returns: a workflow agent takes the session's cwd when it starts); at most one proposal ahead;
  fix and ops lanes (`BRANCH_FIX`, `BRANCH_OPS`) in their own worktrees when they share no files. Take summaries from
  lanes rather than reading their files. When a lane frees up, offer the next queue row.
- **Price a request before you relay it to a running lane.** Before passing an owner request to a
  lane that is already working, tell the owner in two or three lines: which lane, its rough cost
  (tokens or time, extra review rounds), what it displaces or delays, and whether it lands in a
  control (AGENTS.md's *What executes* column). Offer a cheaper option when there is one; the
  owner chooses. Pricing is not declining. A skill step: nothing checks it.
- **Research-only work goes to the `clauductor:researcher` agent**, not a fork (a fork inherits every tool).
- Read, don't assume: the repo's records are the source of truth for "where we were". Keep the
  orientation short; this is a launchpad, not a report.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/session-start/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments session-start`
