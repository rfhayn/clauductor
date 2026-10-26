# PANEL-57 — clauductor lane close: an agent closes a finished lane, never a working one

- **PR**: #250
- **Session**: S12, started 2026-10-24 22:29 ET
- **Review**: 13 rounds, peak high, cap 3 (per group 7, 2, 1 and 3; the owner raised group 1's cap to 6, then allowed a narrow 7th on Fable; group 4's two lows at its cap are #265's)
- **Gate**: CANNOT CHECK — no full gate completed before this commit; the final gate follows it
- **Cost**: $133.67 (usage-report.sh, this session: the proposal and the build, at the fragment's commit)
- **Session start**: 65187792 tokens, 227 min

## What happened

`clauductor lane close <id>` asks the running panel to close a lane through Close lane's own plan
and guards. With no flag it prints the plan and changes nothing; with `--yes` it closes, with the
plan's offers as consent. The panel serves it on `~/.clauductor/panel/cli.sock` (mode 0600, the one
route, named in `owner.json`), and an interrupted command leaves the close finished. One verdict,
`closeHeld`, decides whether a lane's work has ended: no gate lease held or waited for, then claude
exited or gone, or idle with its own session at rest (no subagent, workflow, shell, monitor,
teammate or wakeup in flight). Both the command and auto-close (PANEL-20) read it, and the close
re-reads tmux under the lane lock, so it decides nothing from a value read before the lock. The
command also needs the branch merged unless claude exited, and refuses the caller's own lane.
session-close's *Close the lanes* rule is its production caller, and the template allows
`Bash(clauductor lane close *)` and nothing wider. docs/panel.md lists every refusal word for word,
and `TestEveryCloseWordIsDocumented` holds that list to the reasons `closeheld.go` returns.

## Decisions

- D1–D7 as recommended (the owner, 2026-10-24), with proposal review round 3's four findings and
  merge-pr's two as build tasks rather than design edits, so the approval covered design.md as
  reviewed.
- Group 1's cap: the owner raised it to 6 after round 3, with wakeups added to D2's background
  work, then allowed a narrow 7th round on Fable after round 6.
- Round 1 of group 4 asked the skill to report the result's Kept lines; the builder kept the plan's
  Keeps, which design.md D5 names word for word. Changing it would void the approval, so it stands
  as D5 words it (tasks.md's Decision log); a design amendment is the owner's if wanted.
- Group 4 stopped at its cap with two lows in the docs test's own coverage (derived words, every
  `CloseHeld` write): #265 owns them.

## Surprises

- Each review round of group 1 found one more "unknown or earlier state read as current": a poll
  that straddles a `/clear`, a re-listed session that lost its last `Stop`, a lease source not yet
  read, a tmux value read before the lock. The fix that converged was one list of what the verdict
  will not judge without.
- `clauductor update` run with `!` has no stdin: it merged settings.json, then cancelled the skill's
  review. The owner copied the template's skill by hand (it matched byte for byte at HEAD).
- Three tests flaked under gate load and passed alone: #260 owns the two in claudecall,
  `TestGateStoppedCallHoldsTheSlotUntilItExits` and now `TestGateStopsATimedOutClaudeWithSIGTERM`;
  #234 owns the stall test that failed group 4's first quick gate.
- verify-change failed on a task naming `lane.go` bare; it matches a backticked path against the
  diff's full paths, so the task now names `framework/internal/cmd/lane.go`.
- The merge broke main. #250 merged 10 commits behind `origin/main`, on GitHub's CLEAN and a
  receipt for its own head; pr-merge-guard said so as an advisory, and rule 8 (journal and
  insights only) did not block. OPS-135 (#263) had landed a `gitIn` in `doctor_test.go` too, so
  `internal/cmd`'s tests stopped compiling on main (#276, fixed by #277). The branch's green
  full gate could not see it: it judged the branch's own tree.
- A builder replaced two lines of docs/panel.md with a scripted exact-match replace; the
  no-blind-source-rewrite hook did not block it. Both swaps were reviewed in round 2.

## Insights

| 2026-10-25 | Process | verdict/one-unknowns-list | A safety verdict built from several readings (a poll, a hook, a lease file, tmux) gets one new "an unknown or earlier state read as current" finding per review round when each reading's staleness is handled where it is read. Naming once what the verdict will not judge without (no reading yet, a reading older than an event, a value read before the lock) converged what seven rounds of local fixes did not. | PANEL-57 group 1: rounds 1–7 each found one; `closeHeld` in `state/closeheld.go` | Raw |
| 2026-10-25 | Process | review/baseline-ref | Save the tree each review round sees as a ref before the fix round (`read-tree HEAD`, `add -A` and `commit-tree` under a temporary `GIT_INDEX_FILE`, since `git stash create` fails beside intent-to-add entries), so the next round reviews an exact delta, not "what changed since, roughly". | `refs/clauductor/panel-57-g4-r1..r3`; `git diff` between them | Raw |
| 2026-10-25 | Process | merge/behind-is-untested | A code PR behind `origin/main` has gate evidence for a tree merging will not produce: GitHub's "no conflicts" is textual, and two branches that each add a same-named Go test helper merge clean and fail to compile. When main has moved under a code PR, merge it and gate again before the squash; a busy machine (several lanes merging an hour) makes this the common case, not the rare one. | #250 merged 10 behind; #276 (`gitIn` twice in `internal/cmd`); pr-merge-guard's behind line is advisory | Raw |
| 2026-10-25 | Testing | docs/words-from-the-authority | Docs that hand-list a verdict's reasons drift from the code. A test that parses the source and requires every returned reason in the docs holds them, but only if it walks the returns, not a naming convention, and derives the words of built reasons rather than copying them. | `TestEveryCloseWordIsDocumented`; adding `return "claude is compacting"` fails it; #265's two gaps | Raw |
