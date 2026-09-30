---
name: archive-change
model: opus
effort: medium
description: "Archive a change whose PR has merged: promote its spec deltas into the living specs, move changes/<id>/ to changes/archive/YYYY-MM-DD-<id>/, and mark its roadmap row merged. TRIGGER when the user says 'archive the change', after merge-pr lands a change PR, or when session-close finds a finished change."
argument-hint: <change-id>
---

# Archive a finished change

Archiving is what makes the spec tier true: a requirement joins `specs/` when the change that
built it has merged, never before. Skipping it leaves the specs describing a system one change
behind, and the next proposal reads them as current.

## Preconditions (check each by existence, not by memory)
- The change's PR is merged: `gh pr list --state merged --search "<id>"` names it.
- Every task in `changes/<id>/tasks.md` is `[x]`. An open task is either done elsewhere (say where)
  or owned by a named change or row (AGENTS.md rule 1); otherwise the change is not finished.

## Steps
1. On an `ops/` branch (or inside the session-close PR), for each `changes/<id>/specs/<capability>/spec.md`:
   - **ADDED** requirements are appended to `specs/<capability>/spec.md` (create the file, with a
     real `## Purpose`, if the capability is new: never a placeholder).
   - **MODIFIED** requirements replace the requirement of the same name, whole.
   - **REMOVED** requirements are deleted; the reason goes in the journal.
   With `PROPOSALS=openspec`, `openspec archive <id>` does this step and the next.
2. `git mv changes/<id> changes/archive/<YYYY-MM-DD>-<id>` (today's date).
3. Set the roadmap row to `✅ merged (#N)` if it is not already, then
   `clauductor-model roadmap-queue.sh --check` and `clauductor-model checks/run.sh changes`.
4. Land it through `merge-pr` (or with the session close).
