---
name: archive-change
model: opus
effort: medium
description: "Archive a change whose PR has merged: promote its spec deltas into the living specs, record its actual cost, queue the check of its outcome, move changes/<id>/ to changes/archive/YYYY-MM-DD-<id>/, and mark its roadmap row merged. TRIGGER when the user says 'archive the change', after merge-pr lands a change PR, or when session-close finds a finished change."
argument-hint: <change-id>
---

# Archive a finished change

Archiving is what makes the spec tier true: a requirement joins `specs/` when the change that
built it has merged, never before. Skipping it leaves the specs describing a system one change
behind, and the next proposal reads them as current.

## Preconditions (check each by existence, not by memory)
- The change's PR is merged: `gh pr list --state merged --search "<id>"` names it.
- **Every task in `changes/<id>/tasks.md` is `[x]`** (the Slice line aside). An open task is either
  done elsewhere (say where, and tick it with that note) or owned by a named change or row
  (AGENTS.md rule 1); otherwise the change is not finished. `pr-merge-guard` rule 11 refuses an
  archive with an open task.
- **The change has a spec delta, or says it has none**: `changes/<id>/specs/*/spec.md`, or
  `skip_specs: true` in `changes/<id>/.openspec.yaml`. Rule 11 refuses neither.
  (`openspec archive -y` archives both anyway, with a warning: do not rely on it.)

## Steps
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh ${CLAUDE_PLUGIN_ROOT}/project-config.sh prefixes`

1. On an ops branch (`<BRANCH_OPS><name>`), or inside the session-close PR, for each `changes/<id>/specs/<capability>/spec.md`:
   - **ADDED** requirements are appended to `specs/<capability>/spec.md` (create the file, with the
     delta's `## Purpose`, if the capability is new: never a placeholder).
   - **MODIFIED** requirements replace the requirement of the same name, whole. The delta already
     copies every current scenario header (`checks/changes.sh` held it to that), so nothing is lost.
   - **REMOVED** requirements are deleted; their scenario IDs are retired, never reused, and the
     reason goes in the journal.
   With `PROPOSALS=openspec` (CLI 1.13 or later), `openspec archive <id> -y` does this step and the
   move in step 4; the preconditions and steps 2–3 are still yours.
2. **Record the actual cost** under `## Progress` in `changes/<id>/tasks.md`:
   `clauductor-model change-cost.sh <id>` prints it, from this machine's transcripts at list price.
   Write `- <date> archived: actual cost $X of budget $N` (or `(no budget)`), or, when it says
   CANNOT CHECK, `- <date> archived: actual cost unknown (<its reason>)`. Rule 11 requires the line.
3. **Queue the outcome check.** If `proposal.md` has `## How we'll know`, add a row to the roadmap's
   `## Outcome checks` table, dated to when it says to look (`Check on:`, or the merge date plus
   `Check after:`):
   `| o.<n> | \`ops/check-outcome-<id>\` — check the outcome of <id> (due YYYY-MM-DD) | <the signal> | — | ⬜ queued |`
   `roadmap-queue.sh --text` lists it as DUE from that date, so session-start raises it. Rule 11
   requires the row.
4. `git mv changes/<id> changes/archive/<YYYY-MM-DD>-<id>` (today's date).
5. Optionally, compound now: the insights this change produced are freshest at its archive
   (`clauductor-model compound.sh`, then session-close step 3b's three choices). Otherwise session-close
   does it.
6. Set the roadmap row to `✅ merged (#N)` if it is not already, then
   `clauductor-model roadmap-queue.sh --check` and `clauductor-model checks/run.sh changes scenarios`: the
   promoted scenarios are now living, so the trace holds their tests to them from here on.
7. Land it through `merge-pr` (or with the session close).

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/archive-change/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments archive-change`
