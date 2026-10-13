---
name: apply-change
model: opus
effort: high
description: "Build an approved change by hand, task group by task group, with the builder and reviewer agents: the same loop as the build-change workflow, for when the Workflow tool is not available or one group needs doing alone. TRIGGER when the user says 'apply the change', 'build group N', 'implement <change> by hand', or build-change is unavailable."
argument-hint: <change-id> [group number] [reportTo <session>]
---

# Apply a change, group by group

The paved road is the `build-change` workflow (`.claude/workflows/build-change.js`), which runs
this loop for you and stops cleanly. Use this skill when the Workflow tool is not available, or to
do one group alone. The loop is the same, and so are its stops.

## Who hears each result
- **Wait for every agent you spawn, without handing back.** A spawn may come back as launched in
  the background (fork mode, the default, always does). After a spawn, do any other work this
  group has. If there is none, end your turn with no final report and without SubagentHandback:
  Claude Code keeps you running while an agent you spawned is still working, and delivers its
  result as your next turn. A hand-back finishes you, and the result then goes elsewhere (OPS-25).
  Never poll, sleep or re-spawn: a second builder would edit the same uncommitted tree as the first.
  If the Agent tool offers `run_in_background` (fork mode off: `-p`, the Agent SDK), pass
  `false`: there, a launcher that ends its turn has ended.
- **Report to the session that asked.** When the request names one (`reportTo <session>`, the
  name `SendMessage` addresses it by), send it every outcome with `SendMessage`: at a stop, its one
  line, in place of the owner's `PushNotification` (the session that asked notifies the owner).
  Run `merge-pr` with the same `reportTo`: it sends the merged outcome, or its own stop.
  If SendMessage is not in your tool list, put the outcome in your final hand-back if you are a
  subagent, else send it with `PushNotification`.
  With no reportTo, notify the owner yourself at each stop.

## Preconditions
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh .claude/project-config.sh prefixes`
- The change is approved: `changes/<id>/proposal.md` has its `**Approved:**` line, on `main`.
- You are on `<BRANCH_CHANGE><id>` in its OWN worktree, never the main checkout (hooks run from there), with
  a clean tree.
- **The models follow the proposal's `**Risk:**` tier**: a role with a variant for that tier in
  `.claude/model-roles.json` (`roles.<role>.tiers`) runs on it; pass its `model` and `effort` when
  you spawn the agent. In economy mode (`~/.clauductor/panel/economy.json` says `"economy": true`)
  the roles in `.economy.roles` drop a tier; the reviewer never does.
- **The budget**: if `proposal.md` has `**Budget:** $N`, check `sh .claude/change-cost.sh <id>`
  before each group; OVER BUDGET is a stop for the owner (raise it or cut scope).

## For each task group with open tasks, in order
1. **Build.** Spawn the `builder` agent: "Implement task group N (<title>) of change <id>
   (changes/<id>/). Only group N. Record in tasks.md's Decision log each decision design.md does
   not settle. Leave the work uncommitted." A `design-issue` or `blocked` reply
   is a STOP: one line to `reportTo`, or with none to the owner (`PushNotification`), and stop.
2. **Register new and deleted files**, so the gate and the reviewer see them: for each path from
   `git ls-files --others --exclude-standard`, `git add -N -- <path>` (intent-to-add); for each
   from `git ls-files --deleted`, `git rm -q --cached -- <path>`.
3. **Gate.** `scripts/ci/gate.sh --quick` (the project's `GATE` with its `GATE_QUICK_FLAGS`, from
   `.claude/project.conf`, when they differ). Red: the builder fixes it at its source (at most two
   attempts); an environment fault or a gate still red is a STOP. If the tool moves the gate to
   the background, call `scripts/ci/gate-wait.sh` in the foreground (timeout 600000): exit 3
   means call it again; any other exit is the answer (0 PASS, 1 FAIL; 4 or 2: no result, run the
   gate again).
4. **Review.** Spawn the `reviewer` agent with only the change id and the group number, never the
   builder's summary (its blind spots must differ). Findings medium or worse go back to the builder
   to fix at their source, or to dispute with a reason; then gate again and review again. **Stop**
   if peak severity RISES between rounds, or after 3 rounds without converging, and name the
   builder's disputes for the owner to rule on.
5. **Commit** the group once a round finds nothing medium or worse: add one line under
   `## Progress` in `tasks.md` (`- <date> group N (<title>) built and reviewed: converged in <n>
   round(s), peak <sev>`), check `git diff HEAD --name-status` for anything secret-looking,
   `git add -u`, and commit `<id>: task group N — <title>`, ending with the provenance trailers
   (`Change:`, `Agent-Role: builder`, `Model:`, `Session:`) and the attribution trailer when
   `.claude/model-roles.json` enables them. Do not push yet.

## Then
- The full gate on the committed HEAD: `scripts/ci/gate.sh` (no flags). It writes the receipt.
  If the tool moves the gate to the background, call `scripts/ci/gate-wait.sh` in the foreground
  (timeout 600000): exit 3 means call it again; any other exit is the answer (0 PASS, 1 FAIL; 4 or
  2: no result, run the gate again).
- `/verify-change <id>`: every task ticked, every scenario cited, the diff matching `tasks.md`.
- Push, open the PR, set the roadmap row to `⬜ in flight (#N)` in it, and land it with
  `merge-pr` (passing on any `reportTo`), recording `Review: converged in <n> round(s), peak
  <sev>, at <sha>` per group.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/apply-change/`) add to this
skill. Follow them as part of the steps above:

!`sh .claude/extensions.sh fragments apply-change`
