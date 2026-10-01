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
1. On an `ops/` branch (or inside the session-close PR), promote the deltas with
   **`sh .claude/archive-change.sh <id>`**: it prints the plan and exits 1 on any `STOP`. First list
   every decision this change reversed (what `design.md` records as rejected or superseded, and
   anything a review round changed; the plan prints the design lines that say so) and pass a
   distinctive phrase of each OLD wording as `--superseded '<phrase>'`. A hit in a delta is a STOP:
   fix it where the finding pointed, in the delta, before any text reaches `specs/`. When the plan
   is clean, re-run with `--apply`. What it does, so you can read the plan:
   - **HELD**: a capability whose delta directory holds `NOT-SYNCED.md` is not promoted. The file
     says why (a design with no production caller) and when to sync; honour it.
   - **RENAME** keeps the body under the new heading; **REMOVE** deletes (the scenario IDs are
     retired, never reused, and the reason goes in the journal); **ADD** appends, creating a new
     capability with the delta's `## Purpose` (never a placeholder).
   - **REPLACE / MERGE**: a MODIFIED requirement replaces the living one, unless its delta has
     FEWER scenarios: then the living scenarios it did not restate are kept (MERGE), unless
     `design.md` names each one dropped. A **NOTE** names living scenario headers a replace drops.
   - After `--apply` no requirement's scenario count may fall; it says FAIL if one did.
   With `PROPOSALS=openspec`, `openspec archive <id> -y --skip-specs` can do the move in step 4
   after this; do not let the CLI promote (it replaces a shorter MODIFIED whole, and syncs a
   held-back capability).
2. **Record the actual cost** under `## Progress` in `changes/<id>/tasks.md`:
   `sh .claude/change-cost.sh <id>` prints it, from this machine's transcripts at list price.
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
   (`sh .claude/compound.sh`, then session-close step 3b's three choices). Otherwise session-close
   does it.
6. Set the roadmap row to `✅ merged (#N)` if it is not already, then
   `sh .claude/roadmap-queue.sh --check` and `sh .claude/checks/run.sh changes scenarios`: the
   promoted scenarios are now living, so the trace holds their tests to them from here on.
7. Land it through `merge-pr` (or with the session close).

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/archive-change/`) add to this
skill. Follow them as part of the steps above:

!`sh .claude/extensions.sh fragments archive-change`
