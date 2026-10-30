# PRAC 0013: Batch reviewed lanes into one integration branch, one gate per batch

- **Status**: Accepted (by the owner, 2026-10-29)
- **Date**: 2026-10-29
- **Tags**: gate, review, lanes, process
- **Source**: S16 (2026-10-29): batch 1 (#331: OPS-154, PANEL-47, OPS-129, OPS-161) and batch 2
  (#339: OPS-156, OPS-160, OPS-159, OPS-167, OPS-170, OPS-166, OPS-168, OPS-91); OPS-155's
  merge refused by GitHub after #316 and #324 moved `main`; insights-log rows of 2026-10-29
  (*batch gates catch repo-wide checks*, *merge origin/main right before gating*)

## Context
A full gate takes about 33–56 minutes on the owner's Mac (S16's passing runs in
`gate-runs.log`), and two full gates on one machine never overlap (`run-local.sh`'s lease). With
a dozen reviewed lanes ready in one session, a gate per PR queues them for most of a day, and each
merge moves `main` under the next PR's receipt.

In S16 the owner had twelve rows merged in two batches instead: each lane was reviewed on its own
PR as before, then its branch was merged into one integration branch, which took one gate and
landed as one squash. The batch gates were not a formality. They caught what each lane's scoped
tests had passed: batch 1's race scope failed on PANEL-47's test; batch 2's `approval-guarded`
check failed on `quota-gate.sh`, its panel clock check on OPS-159's drain, and its integration
review found a medium in OPS-167's asserts. Each was fixed in the batch.

Two things went wrong around the batches and shaped the steps below. OPS-155's PR passed the
guard and was refused by GitHub for a conflict once #316 and #324 had merged, so its receipt was
for a head that could no longer merge. And the fold records each row's merge from the PR its
fragment names, which for a batched row is the lane PR that was closed, not the batch that merged
(S16's close set twelve Status cells by hand).

## Practice
1. **Review stays per row.** Every lane opens its own PR and is reviewed to convergence on it,
   against its own cap, exactly as before. A batch never replaces or merges a row's review.
2. **Batches are planned with the owner at session start** (from the session plan's rows), and
   adjusted as the session goes: a row that becomes ready joins the next batch, a row that is not
   ready waits for a later one.
3. **A batch is frozen once its gate starts.** Nothing joins it after; a late row waits for the
   next batch. A fix the gate asks for is the one exception (step 8).
4. **Merge order:** the lane branches merge into the integration branch `--no-ff`, in the planned
   order, a row that changes a rule other rows are judged by (a guard, a check, the gate) first.
5. **Regenerate `plugin/`** (`scripts/build-plugin.sh`) after the merges, since every lane's
   `plugin/` is built from its own template.
6. **Merge `origin/main` right before the gate**, every time, so the receipt covers a head GitHub
   will merge.
7. **Keep out a row something waits on alone**: a row another row or the owner is blocked on
   lands on its own, so a failure elsewhere in the batch does not hold it.
8. **A failed batch gate is fixed in the batch**, one commit per reconciled row (its id in the
   subject), and the gate runs again. A row whose fix is not small leaves the batch for a later
   one.
9. **Each lane runs a pre-PR pass first**: `go test -short ./...` and both check sets
   (`sh .claude/checks/run.sh`, `sh template/.claude/checks/run.sh`), so the batch gate is not
   the first run of a repo-wide check.
10. **One squash per batch**, titled by its first row's id and naming every row; each lane PR is
    closed with a comment pointing to the batch PR.

## Consequences
### Positive
- One gate per batch instead of one per row: S16 landed twelve rows on six batch gate runs, three
  of them failures that were real catches, where a gate per row would have taken twelve or more.
- The repo-wide checks run over the rows together, so interactions between rows (a rule one row
  adds and another row breaks) are caught before `main`, not after.
- `main` moves once per batch, so fewer receipts go stale under a moving base.

### Negative / trade-offs
- A failure anywhere in the batch holds every row in it until it is fixed or the row leaves.
- The squash commit names several rows, so `git log` on `main` attributes a batch's changes to its
  first row's id; the lane PRs and the batch PR's body hold the per-row history.
- The fold reads each row's merge from its fragment's PR, so until OPS-176 writes the batch PR
  there, a batched row's Status is corrected by hand at the close.
- The steps are by hand, so a step can be skipped without anything noticing.

## Enforcement
**`scripts/ci/batch.sh`** (OPS-176) runs steps 4–6 and the fragments: `merge` (the `--no-ff`
merges in the order given, then `BATCH_REGENERATE`, which rebuilds `plugin/` here), `sync` (the
`origin/main` merge before each gate) and `record` (the batch PR into each row's fragment, or
under **Also closes**, so the fold records the PR that merged); **`checks/batch.sh`** holds it,
and `docs/playbook.md` (*Batching reviewed lanes*) names the order. Step 9's pre-PR pass is
`scripts/ci/gate.sh -C <worktree> --quick`. Planning, the order, the freeze, step 7 and the
pre-PR pass being run: **Nothing. You.** The mechanism's Rule 4 row is the template's AGENTS.md
only: this repo's is at its byte ceiling, so this section is where its row would point. The guard rules that hold any PR (`pr-merge-guard.sh`: gate evidence for the exact
head, rule 8 for the records) hold a batch PR too, but none of them knows it is a batch.

## Related
- PRAC-0010 (close the class): the batch gate is where a class one lane's tests miss shows up.
- OPS-176 (the batch script and the pre-PR pass), OPS-175 (parallel checks: a shorter gate).
- #331, #339: S16's two batches.
