# Insights log

Raw, non-obvious technical observations captured **during** the work: platform quirks, gotchas,
trade-offs, debugging wins, guards that failed silently. Newest at the top. This is the intake
tier; durable decisions get **promoted** from here (`/log-insight` runs the check).

## Promotion rules
Routing follows *What this tier holds* in [`adr/README.md`](adr/README.md).
- **A decision with trade-offs, or a lesson with a mechanism** → a new ADR, or a dated amendment
  inside the ADR that already owns the principle. Mark **every** row the ADR cites
  `Promoted → ADR-NNNN`, not only the one that prompted it.
- **Already covered by an existing check** → `Instance of ADR-NNNN (check N)`, or the sentence
  form. Nothing is written in the ADR.
- **3+ rows of one shape** → a promotion candidate. Topic tags are typed by hand, so a real group
  can be spread across several tags.
- **A recurring gotcha or convention** → **not `AGENTS.md`**, which changes only to add a
  mechanism or delete something. Amend the owning ADR, add a mechanism, or keep the row as
  `Technique — no mechanism`.

## Status vocabulary

| Status | Means |
|---|---|
| `Raw` | Genuinely un-triaged. Not "we know about it". |
| `Promoted → ADR-NNNN` | The ADR (or its dated amendment) states it now. |
| `Instance of ADR-NNNN (check N)` / `Instance of ADR-NNNN ("<covering sentence>")` | Triaged; an existing ADR or AGENTS.md rule already covers it. Find the sentence first. |
| `Folded → <file>` | Its content moved into a named doc. |
| `Deferred → owned by <change>` | Work remains, and a named, open change owns it (AGENTS.md rule 1). Never an endpoint or a phase. |
| `Decided` | A decision was taken; say whose and where it is recorded. |
| `Fixed → <where>` / `Shipped → <where>` | Closed by named code, checked by existence (`grep`, `ls`, `git log`), not by re-reading the row. |
| `Closed — <what closed it>` / `Partly closed — <what remains>` | Resolved otherwise; say what remains, and who owns it. |
| `Superseded → <what>` | A later row or decision replaced it. |
| `Technique — no mechanism` | Triaged, retained for the pattern, and nobody owes anything. Not a place to park work. |
| `Archived` | No longer relevant. |

Statuses may be emphasised (`**Decided 2026-01-02** — …`): anything that reads this column strips
formatting before matching the vocabulary.

## Log

| Date | Area | Topic | Observation | How to verify | Status |
|------|------|-------|-------------|---------------|--------|
