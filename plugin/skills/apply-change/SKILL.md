---
name: apply-change
model: opus
effort: high
description: "Build an approved change by hand, task group by task group, with the builder and reviewer agents: the same loop as the build-change workflow, for when the Workflow tool is not available or one group needs doing alone. TRIGGER when the user says 'apply the change', 'build group N', 'implement <change> by hand', or build-change is unavailable."
argument-hint: <change-id> [group number]
---

# Apply a change, group by group

The paved road is the `build-change` workflow (`${CLAUDE_PLUGIN_ROOT}/workflows/build-change.js`), which runs
this loop for you and stops cleanly. Use this skill when the Workflow tool is not available, or to
do one group alone. The loop is the same, and so are its stops.

## Preconditions
- The change is approved: `changes/<id>/proposal.md` has its `**Approved:**` line, on `main`.
- You are on `change/<id>` in its OWN worktree, never the main checkout (hooks run from there), with
  a clean tree.

## For each task group with open tasks, in order
1. **Build.** Spawn the `clauductor:builder` agent: "Implement task group N (<title>) of change <id>
   (changes/<id>/). Only group N. Leave the work uncommitted." A `design-issue` or `blocked` reply
   is a STOP: notify the owner (`PushNotification`, one line) and stop.
2. **Register new and deleted files**, so the gate and the reviewer see them: for each path from
   `git ls-files --others --exclude-standard`, `git add -N -- <path>` (intent-to-add); for each
   from `git ls-files --deleted`, `git rm -q --cached -- <path>`.
3. **Gate.** `scripts/ci/gate.sh --quick`. Red: the builder fixes it at its source (at most two
   attempts); an environment fault or a gate still red is a STOP.
4. **Review.** Spawn the `clauductor:reviewer` agent with only the change id and the group number, never the
   builder's summary (its blind spots must differ). Findings medium or worse go back to the builder
   to fix at their source, or to dispute with a reason; then gate again and review again. **Stop**
   if peak severity RISES between rounds, or after 3 rounds without converging, and name the
   builder's disputes for the owner to rule on.
5. **Commit** the group once a round finds nothing medium or worse: check `git diff HEAD
   --name-status` for anything secret-looking, `git add -u`, and commit
   `<id>: task group N — <title>`, ending with the attribution trailer when
   `.claude/model-roles.json` enables it. Do not push yet.

## Then
- The full gate on the committed HEAD: `scripts/ci/gate.sh` (no flags). It writes the receipt.
- Push, open the PR, set the roadmap row to `⬜ in flight (#N)` in it, and land it with
  `merge-pr`, recording `Review: converged in <n> round(s), peak <sev>, at <sha>` per group.
