---
name: new-adr
model: opus
effort: xhigh
description: "Promote a decision or a lesson with a mechanism into an Architecture Decision Record under docs/adr/ (ADR_DIR), or amend the ADR that owns it. An ADR is the owner's decision: draft it Proposed unless they have decided. TRIGGER when the user says 'write an ADR', 'promote this to an ADR', 'record this decision', or when a log-insight promotion check flags a trade-off."
argument-hint: <decision title>
---

# Write an ADR (promote a decision)

## Context: the state, computed
!`sh .claude/skills/new-adr/context.sh`

If the line above shows as literal text instead of output, run
`sh .claude/skills/new-adr/context.sh` yourself and read the result before step 1.

## Steps
0. **Decide: a new ADR, an amendment, or neither.** Read *What this tier holds* in
   `docs/adr/README.md`.
   - A new principle → a new ADR (steps 1–4).
   - A refinement of an existing ADR's principle → a dated `## Amendment — <today>: <what changed>`
     section inside that ADR, with its own source note and its own **Enforcement** line; then step
     5. If the amendment makes the ADR's title wrong, state the current decision in the index's
     Status column.
   - A further example that changes nothing → no ADR edit: retag the insights row
     `Instance of ADR-NNNN (check N)` (or the sentence form) and stop.
1. **Take the number the context block prints**: one past the highest on `origin/main` and in the
   open PRs, never from your branch. If it says CANNOT CHECK, run those commands yourself.
2. Copy `docs/adr/TEMPLATE.md` to `docs/adr/NNNN-<kebab-title>.md` and fill every section. The
   **Enforcement** section names the hook, check or step that keeps it true, or says "none".
3. Set **Status**: `Accepted` only if the owner has decided it (ADRs are theirs, AGENTS.md *Who
   decides*); otherwise `Proposed`, and put the decision in front of them.
4. Add a row to the index in `docs/adr/README.md`.
5. **Retag EVERY insights-log row the ADR or amendment cites**, not only the one that prompted
   it: `Promoted → ADR-NNNN` (or `Promoted → ADR-NNNN (<date> amendment)`). Grep for each cited
   topic and read each status cell back. A promotion that does not retag its sources keeps
   session-close's promotion trigger firing on work already done.
6. If it supersedes an older ADR, set that one's Status to `Superseded by ADR-NNNN`.
7. Run `sh .claude/checks/run.sh adr-numbering`.

## Rules
- Never renumber an ADR that is on `main`; supersede it. Renumbering your own unmerged ADR after a
  collision is the one exception.
- An ADR without an Enforcement mechanism is a wish: name one, or state its absence.
