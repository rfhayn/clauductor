# PRAC 0014: Converge review by deleting mechanism, not adding it

- **Status**: Proposed
- **Date**: 2026-10-30
- **Tags**: review, build-change, builder
- **Source**: Standing Tee's PRAC-0017 (*Size a change so that one review round converges*), its
  2026-08-30 amendment (score a round by peak severity) and its 2026-10-09 amendment (delete the
  mechanism); Standing Tee insight `review/delete-the-mechanism-when-fixes-breed-bugs`
  (2026-10-08, from its PR #601); roadmap row ST-12

## Context
Standing Tee's PR #601 ran review rounds 5, 6 and 7, and each one found exactly one medium. All
three were in one drawing optimization, an edge skip, and each was a regression of the previous
round's fix: file order, then a path through a milestone, then a path through a done milestone.
Deleting the optimization cost a few extra grey edges, and round 8 came back clean. Counting
findings could not tell this apart from a chain of test gaps: both read 1, 1, 1. What told them
apart was where the findings sat (the same few lines) and what produced them (the last fix).

Earlier, on Standing Tee's #70, rounds 2 to 4 each found defects introduced by the previous
round's fixes. The fix rate matched the defect rate, so the loop did not end; it was stopped.
PRAC-0017 first read that as "watch the regression column"; its 2026-08-30 amendment replaced
that with what this record keeps, measured on a later PR whose findings fell 15 → 7 → 4 while the
share of regressions rose: a finding count says nothing on its own, and a round is read by its
peak severity and how far its defects reach.

Here, `build-change.js` already scores a round by peak severity (it stops when severity rises or
stops falling), and `builder.md` already said to prefer deleting over hardening, but only when a
fix needs a NEW mechanism. Two gaps remained. Each review-fix builder is a fresh agent that saw only
the current round's findings, so it could not see that a finding was the last fix's regression in
the same mechanism. And the loop dropped the builder's `notes`, so a deletion's cost never reached
the report the PR is written from.

## Practice
1. **A round is judged by its peak severity and how far its defects reach, not by its finding
   count.** A falling count at a flat peak is not convergence; six findings in round 1 of a
   well-sized change is a healthy review.
2. **A fix that adds state or logic beyond what its finding needs is a stop, not a fix.** The
   builder returns `design-issue` naming what it needs; the design is the owner's. A finding marked
   with a `source:` is still fixed at that source, and a new mechanism a finding does need is named
   in the builder's `notes`.
3. **When a finding sits in the same small mechanism as the previous round's and is a regression
   of its fix, the next fix asks first whether the mechanism can be deleted.** If what it buys is
   an optimization or a nicety, delete it and state the cost in the PR body. Harden it only when
   deleting it would lose a requirement. A mechanism that `design.md` or a spec scenario names is
   the owner's: the builder proposes its deletion by returning `design-issue`, and deletes directly
   only what the build or an earlier fix added. This governs what a fix is, not how many rounds there are:
   the cap (`.claude/review-cap.sh`) is unchanged.

## Consequences
### Positive
- A loop that keeps regressing in one mechanism converges in a round, as #601's did, instead of
  running to the cap.
- A fix cannot grow the design quietly: unnamed state reaches the owner as a design issue.
- A deletion's cost is written down in the change's Decision log, which survives a stop and a
  resume, and from there reaches the PR body.

### Negative / trade-offs
- Deleting a nicety the build added loses it; the owner learns that from the PR body, after the
  fact. A designed one waits for the owner, which costs a stop.
- "Same small mechanism" is the builder's judgment from two rounds' finding lists. A builder can
  misjudge it either way, and nothing checks that it asked.
- More design-issue stops: a fix that a builder would once have patched in now stops the run.

## Enforcement
- **`template/.claude/workflows/build-change.js`**: every review-fix prompt carries the stop and
  delete-first instruction (`DELETE_FIRST`, which also sends a designed mechanism's deletion to
  `design-issue` and a deletion's cost to the Decision log), from round 2 the previous round's
  findings, and each fix's `notes` in the group's report (`fixNotes`); the workflow's success
  steps put the deletions in the PR body. **`checks/build-change.sh`** asserts these in its driver.
  The review prompt and call (rule 13's triggers) are unchanged, so no eval receipt is needed.
- **A limit, and how it is covered.** #601's own shape (one medium, then one medium that regresses
  its fix) stops build-change's stuck-loop breaker at round 2 (*not falling*), before any fixer
  sees both rounds. So the stuck stop's notify, which reaches the owner, opens with the delete
  option, and its full reason lists both rounds' findings; the playbook's *stuck* row says the
  same. The deletion is then the owner's decision, not the builder's.
- Every review-fix loop is reached: build-change (above); **`builder.md`**, *Converge by removing,
  not adding*; **`/apply-change`** step 4, which hands the builder the previous round's findings;
  and **`merge-pr`** step 3's hand-run loop, the one #601 ran. The last three are instructions:
  nothing checks them.
- Peak-severity scoring is `build-change.js`'s `gradeRound` and its stuck-loop stops, as before.
- The builder's judgment that a finding is a regression in the same mechanism, and the PR body
  stating the cost: **Nothing. You.**

## Related
- PRAC-0010 (close the class): its opposite move for a finding that recurs across the change, not
  in one mechanism.
- Standing Tee PRAC-0017 and its amendments; ST-13 (parking on a classifier stop), the other
  Standing Tee review practice of S18.
