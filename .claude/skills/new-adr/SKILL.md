---
name: new-adr
model: opus
effort: xhigh
description: "Promote a decision about the system into an Architecture Decision Record under docs/adr/ (ADR_DIR), or a lesson about how we work into a practice record under docs/practices/ (PRACTICES_DIR, PRAC-NNNN); asks which first, or amends the record that owns it. Both are the owner's decisions: draft Proposed unless they have decided. TRIGGER when the user says 'write an ADR', 'promote this to an ADR', 'write a practice record', 'record this decision', or when a log-insight promotion check flags a trade-off or a lesson with a mechanism."
argument-hint: <decision title>
---

# Write an ADR or a practice record (promote a decision)

## Context: the state, computed
!`sh .claude/skills/new-adr/context.sh`

If the line above shows as literal text instead of output, run
`sh .claude/skills/new-adr/context.sh` yourself and read the result before step 1.

## Steps
0. **Ask the kind first: a decision (ADR) or a practice (PRAC)?** Say which, and why, in one line
   before writing anything; if it is not clear, ask the person. Nothing checks this judgment.
   - **A decision about the system** (its structure, dependencies, interfaces, how it is built) →
     an ADR in `ADR_DIR` (`docs/adr/`), headed `# ADR NNNN: <title>`, cited `ADR-NNNN`.
   - **How we work** (a lesson about testing, review, evidence or process with a mechanism, or a
     tooling decision behind one) → a practice record in `PRACTICES_DIR` (`docs/practices/`),
     headed `# PRAC NNNN: <title>`, cited `PRAC-NNNN`. If the directory is missing, create it from
     the template's `docs/practices/` (its README and TEMPLATE.md; `stridal update` offers them).

   Below, *the record* is that ADR or practice record, and `<DIR>` its directory.
0a. **Decide: a new record, an amendment, or neither.** Read *What this tier holds* in
   `docs/adr/README.md` (or *What this series holds* in the practices README).
   - A new principle → a new record (steps 1–4).
   - A refinement of an existing record's principle → a dated `## Amendment — <today>: <what
     changed>` section inside that record, with its own source note and its own **Enforcement**
     line; then step 5. If the amendment makes the title wrong, state the current decision in the
     index's Status column.
   - A further example that changes nothing → no record edit: retag the insights row
     `Instance of ADR-NNNN (check N)` or `Instance of PRAC-NNNN (check N)` (or the sentence form)
     and stop.
1. **Take the number the context block prints**: one past the highest on `origin/main` and in the
   open PRs, across BOTH directories (ADRs and practice records share one number space), never
   from your branch. If it says CANNOT CHECK, run those commands yourself.
2. Copy `<DIR>/TEMPLATE.md` to `<DIR>/NNNN-<kebab-title>.md` and fill every section. The
   **Enforcement** section names the hook, check or step that keeps it true, or says "none".
3. Set **Status**: `Accepted` only if the owner has decided it (ADRs and practice records are
   theirs, AGENTS.md *Who decides*); otherwise `Proposed`, and put the decision in front of them.
4. Add a row to the index in `<DIR>/README.md`.
5. **Retag EVERY insights-log row the record or amendment cites**, not only the one that prompted
   it: `Promoted → ADR-NNNN` or `Promoted → PRAC-NNNN` (or `… (<date> amendment)`). Grep for each
   cited topic and read each status cell back. A promotion that does not retag its sources keeps
   session-close's promotion trigger firing on work already done. This edits the insights log,
   so the record lands in its own PR or the close's (rule 8 holds either on top of `origin/main`),
   never inside a row's PR, which writes only its fragment (OPS-108).
6. If it supersedes an older record, set that one's Status to `Superseded by ADR-NNNN` (or
   `PRAC-NNNN`).
7. Run `sh .claude/checks/run.sh adr-numbering`.

## Rules
- Never renumber a record that is on `main`; supersede it. Renumbering your own unmerged record
  after a collision is the one exception. A record never moves between the two directories.
- A record without an Enforcement mechanism is a wish: name one, or state its absence.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/new-adr/`) add to this
skill. Follow them as part of the steps above:

!`sh .claude/extensions.sh fragments new-adr`
