# PRAC 0010: Close the class, not the instance, when review keeps finding the next one

- **Status**: Accepted (by the owner, 2026-10-19)
- **Date**: 2026-10-18
- **Tags**: review, process
- **Source**: insights-log rows *shell parser never converges* and *unbounded input space*
  (2026-10-17), *close the class* (2026-10-18); #146, #150, #157, #159

## Context
Four reviews in two days kept finding one more instance of the same class of finding until
someone stopped them:
- **#146 (OPS-66), 5 rounds.** Each round found a new merge-guard bypass in one more shell form
  (redirections mid-command, a `(` from process substitution or a zsh glob qualifier, then a
  regression from narrowing the fail-closed rule). Failing closed on a whole class converged
  better than listing forms. What is left needs a different design, which OPS-86 owns.
- **#150 (OPS-72), 5 rounds.** Each found a narrower medium in the shape of `GATE`; round 3's own
  fix made a new one.
- **#157 (PANEL-51's proposal), 7 rounds** (peak high to round 3, then medium). The owner
  ended it: "fix M1 and M2, no round 8".
- **#159 (PANEL-49), group 3, 4 rounds.** Each round found one more route past the transcript
  reader's decode guard: a read through `os/exec` or an `fs.FS`, a decoder in another file of the
  package, a reference to a decoder already listed for another body, a method declared elsewhere
  on the reader's type. The owner skipped round 5. The rounds that closed something for good
  replaced a list of what is refused with a list of what is allowed: `readerImports` (in place of
  a deny list of packages), `signalsDecodes` (every decode site in the package) and
  `readerReferences` (every outside name the reader uses).

Each of these problems is bounded by construction: the code under review decides what the input
can be. A review that refutes one instance at a time is then searching a space the fix never
bounded, so every round finds the next instance, and each round costs a reviewer, a fix and,
often, a gate.

## Practice
- **When a review finding is a second instance of a class an earlier round already found, the fix
  closes the class.** Allow only what is listed (an allowlist, a type that cannot hold the field,
  one named path) instead of refusing one more listed form. The fix's commit or Decision-log entry
  names the class and the rejected instance-by-instance fix.
- **If the class cannot be closed within the change's scope, say so by round 3 and offer the owner
  the narrowed scope**, with the known limits written down (as PANEL-49's *known limit* and #146's
  hook header do), rather than running round 4.
- **A reviewer who finds an instance of a class says so**: "one more X; the class is Y", so the
  fixer can tell an instance from a new problem.

## Consequences
### Positive
- Reviews of a bounded problem converge in fewer rounds, and the owner is asked once, early,
  instead of stopping a loop late.
- An allowlist fails closed on the next case nobody thought of; a deny list fails open on it.

### Negative / trade-offs
- An allowlist is stricter: a legitimate new use has to be listed, and its test fails until it is.
- Closing a class can be a larger change than the instance, and may need the owner's approval of
  a design change mid-build.

## Enforcement
Partial. The reviewer's prompt (`.claude/agents/reviewer.md`, and the template's: OPS-97, the
mechanism the owner chose on accepting this record, 2026-10-19) asks it to name each finding's
class in its summary and say whether an earlier round found one of that class. `merge-pr` step 3
and `build-change` (`maxRounds`) cap the rounds. The prompt is an eval trigger, so a change to it
needs a passing eval receipt (`pr-merge-guard.sh` rule 13).

Known limits, so this is not read as full enforcement:
- **The reviewer sees only part of the earlier rounds.** `build-change` passes it the findings the
  builder DISPUTES (`REVIEW_DISPUTED`), not every earlier finding; beyond those it has the Decision
  log in `tasks.md` and the commit messages of committed groups. A second instance of a class
  whose first was fixed without a note in either reads as "first seen".
- **No code acts on the class.** It is prose in the finding's summary: the review schema has no
  field for it, and neither the fixer's prompt nor the round cap acts on it mechanically. The one
  code that reads it, `build-change`'s `findingKey`, drops the class wherever the summary marks it
  (`class:`, `the class is`, `first seen`), so a finding's identity across rounds is what it says,
  not its class: two findings of one class in one file stay two, and a recurrence whose class
  wording changes ("first seen" to "one more") is still one, as long as its own words do not change.
  Closing
  the class, and sending an unclosable one to the owner by round 3, are still the fixer's and the
  session's judgment.

## Related
- OPS-86 (the merge path as one allowlisted command: the same practice applied to the merge guard)
- AGENTS.md rule 4, *A guard must iterate the AUTHORITY, not the list it was handed*
- `changes/archive/2026-10-18-panel-49-lane-measures/tasks.md`, Decision log, group 3
