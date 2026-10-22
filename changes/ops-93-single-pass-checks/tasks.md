# Tasks: ops-93-single-pass-checks

## Progress
- 2026-10-18 proposed; nothing built yet. No spec delta (`.openspec.yaml`): the tests are the
  checks' own fixtures and the plugin test named below.
- 2026-10-18 pre-review (propose step 2b), for the proposal PR's body:
  Review: converged in 3 round(s), peak low, at 9e98b38 · design 78e8b98101ab
  - Rounds: 1 peak medium (2 medium, 4 low, fixed), 2 peak medium (2 medium, 4 low, fixed in
    9e98b38), 3 peak low. Round 3's two lows are left open on purpose for the builder: L1 edit
    `template/AGENTS.md` before the plugin rebuild; L2 `modules.sh`'s whole-set fixture inherits
    the markers.
- 2026-10-18 approved by the owner (design 78e8b98101ab).
- 2026-10-19 group 1: 1.1 done (baseline below). 1.2 to 1.6 not started: the builder's Edit of
  `template/.claude/checks/lib.sh` was refused as a sensitive file (a permission prompt no one can
  answer inside the workflow). Every group-1 file is under `.claude/`, so the group needs a run
  that may edit `template/.claude/checks/` (and, for 1.6, `.claude/` via `clauductor update`).
  Nothing else is edited. Notes for the resumed run, not yet decisions: the plugin builder keeps
  comment lines verbatim (`rewrite.go` `shell`), so comment markers around run.sh's wrap let the
  D7 fixture derive "run.sh with the wrap removed" with `sed`; `modules.sh:68,89,92` start
  whole-set fixture runs and need the markers cleared (pre-review L2); a quick way to see the
  fixtures fail first is `CHECKS_SET=whole sh template/.claude/checks/no-clauductor.sh` (part 2
  fails fast, the rest runs).
- 2026-10-21 group 1 done, uncommitted, for review: 1.1 was done on 2026-10-19 (the baseline
  under the Decision log); 1.2 to 1.6 done this run, in an interactive session that could edit
  `template/.claude/`. Group 2 not started. Pre-review L1 (`template/AGENTS.md` before the plugin
  rebuild) belongs to task 2.4; L2 is done (Decision log).
- 2026-10-21 group 1 review round 1 (peak medium): M1, M2, L1, L2 and L3 fixed (Decision log).
  For group 2 (review L4, task 2.4's playbook line): `scripts/ci/lib/steps.sh:75-77`'s
  `model_step_no_clauductor` runs `checks/run.sh no-clauductor`, a named run, so no-clauductor.sh
  runs alone and re-runs the set: a gate that calls it AND `model_step_checks` pays the double pass
  again. The playbook line should say a gate calls one of them, not both (the template's own
  `scripts/ci/steps.sh` calls only `run.sh`).
- 2026-10-21 group 1 review round 2 (peak medium): stopped as a design issue, with nothing edited.
  The medium is that round 1's background child ignores INT, so a Ctrl-C never reaches the running
  check. Its proposed fix, run.sh `exec`ing into the environment with the inner run.sh removing it
  on EXIT, reverses D1 ("It runs a child, not exec") and task 1.3 ("run itself as a child … let
  the EXIT trap remove the scratch"). That is the owner's decision, and an edit to design.md voids
  the approval (design 78e8b98101ab). Left, for after the decision:
  - the medium;
  - the low "the suffix from the real state, with a fixture": how it is built depends on the choice;
  - the low "one setup helper for the two fixture projects";
  - marker-driven, not sleep-driven, signal fixtures.
  The tree is still round 1's (quick gate passed on tree cdf713782), plus this note.
- 2026-10-21 the owner chose A: D1 and task 1.3 amended to `exec`, with the inner run removing
  the environment on EXIT; task 1.3 reopened. Approval re-recorded on the amended design.
- 2026-10-21 group 1 round 2's findings done, uncommitted, for review: 1.3 rebuilt to `exec` and
  ticked; the medium and the three lows (Decision log). Plugin rebuilt, `.claude/` updated from a
  binary built in this worktree (`model-roles.json` restored). A re-run on a loaded machine (load
  average 5 to 12), not a timing: `run.sh no-clauductor` (template) passes, 479 s; the whole template
  set passes, 408 s, 44 checks, and this repo's, 409 s, 47 checks, each ending `(in …, without
  clauductor)`; `TestPluginChecksPassInScaffoldedProject` (413 s) and `TestCommittedPluginIsCurrent`
  pass with `-timeout 25m`. 1.6's figures remain the measurement. Group 2 not started.
- 2026-10-21 group 1 review round 3's findings done, uncommitted (Decision log); plugin rebuilt,
  `.claude/` updated, `model-roles.json` restored. Re-run, not a timing (load average 5 to 7):
  `run.sh no-clauductor` (template) passes in the foreground, 483 s (44 ok), and launched with `&`
  from a non-interactive sh, 497 s (42 ok: the INT fixture's three lines replaced by its SKIPPED
  line); the whole template set passes, 390 s, 44 checks, and this repo's, 394 s, 47 checks, both
  `(in …, without clauductor)`. After the SKIPPED-line follow-up (plugin rebuilt, `.claude/`
  updated, `model-roles.json` restored): `run.sh no-clauductor` passes in the foreground, 462 s
  (45 ok), and launched with `&`, 483 s (43 ok, the SKIPPED line printed under PASS). The whole sets
  were not re-run after this follow-up.
- 2026-10-21 group 1 review round 4's two lows done (Decision log); plugin rebuilt, `.claude/`
  updated, `model-roles.json` restored. `run.sh no-clauductor` (template) passes, 358 s (46 ok).
  The whole sets were not re-run after it.
- 2026-10-21 group 1 (The single pass) built and reviewed: converged in 4 round(s), peak low
  (rounds 1 to 3 medium; round 4, approved by the owner, low). Quick gate passed on tree d9577b6ad.

## Decision log
- 2026-10-19, group 1, task 1.1 baseline (worktree at bbc23074, Apple M5, 10 cores, before any
  edit; one set at a time, nothing else of this build running): `sh template/.claude/checks/run.sh`
  698 s, 44 checks; `run.sh no-clauductor` alone 354 s (51%). `sh .claude/checks/run.sh` 766 s,
  47 checks; `run.sh no-clauductor` alone 358 s (47%). The share was measured as a separate named
  run, not from inside the whole run (run.sh prints no per-check time), rather than adding timing
  to run.sh for a one-off; it agrees with the amendment's "half the set".
- 2026-10-21, group 1, task 1.2, the helper's names: `bare_env` keeps no-clauductor.sh's logic
  and its `np`, `H` and `bare` as they were, but its loop temporaries take `_b` names (`_bp`,
  `_bf`, `_bx`, `_bi`) and its directory is `$(scratch)/bare`, exposed as `BARE_DIR`, rather than
  the bare `i`, `f`, `p`, `x` it had: in a library every check sources, a helper must not
  overwrite a caller's `f` or `p`. CHECKS_BARE's value is `BARE_DIR`, and the environment's HOME is
  `$BARE_DIR/home`, which is what the assertion compares `$HOME` with.
- 2026-10-21, group 1, task 1.2, the assertion's shape: `bare_assert` returns 0 (inside), 1 and
  the reason (outside the environment under a whole-set run, or a marker the real state
  contradicts), or 2 (run alone or the double pass: re-run). The alternative was a function for
  the three-part state check alone, with the three-way choice written in no-clauductor.sh; but
  then fixture 2's check would have had to copy that choice, and would test the copy. One function
  holds all of D3, and part 2 and the fixture both call it.
- 2026-10-21, group 1, tasks 1.4 and 1.5, clearing the markers: a new `lib.sh` helper,
  `unmarked CMD` (`env CHECKS_BARE= CHECKS_SET= CHECKS_DOUBLE_PASS= CMD`), and every test of a
  marker reads empty as unset. The alternative, `unset` in a subshell at each call site, repeats
  the three names wherever a whole-set run starts. D3's re-run sets all three explicitly too:
  `CHECKS_SET=` and `CHECKS_BARE=$BARE_DIR` as D3 says, and `CHECKS_DOUBLE_PASS=` (which D3 does
  not name), because a check inside a wrapped run never sees `CHECKS_DOUBLE_PASS=1`, and D1 says a
  re-run sets or unsets all three.
- 2026-10-21, group 1, task 1.3, the wrap's markers: run.sh's wrap sits between the comments
  `# wrap: start` and `# wrap: end`, inside the `$# -eq 0` branch and after `CHECKS_SET=whole` is
  exported, so fixture 2 removes exactly the wrap with `sed` and its run.sh still marks a whole-set
  run. The plugin builder keeps comment lines verbatim, so the same `sed` works on the plugin's
  copy. no-clauductor.sh also checks the markers exist, so a renamed marker fails rather than
  leaving fixture 2 testing an unchanged run.sh. The summary line of a run inside the environment
  says so: `checks: all N passed (in ROOT, without clauductor)` (a green result names its subject).
- 2026-10-21, group 1, task 1.5, the fixtures' PATH: each fixture run has the stub `clauductor`
  in front of no-clauductor.sh's own stripped PATH (`np`), not in front of the caller's PATH. The
  stub is then the only `clauductor` the fixture can find, whether no-clauductor.sh runs inside the
  single pass, alone, or in the double pass, and the fixture's wrap mirrors one small directory
  rather than the caller's real `bin`. Two fixtures beyond D7's three: "with the wrap removed,
  the clauductor-calling check passes" (so fixture 1 fails for want of `clauductor`, not for a
  broken fixture) and "a stale `CHECKS_BARE` fails the assertion" (D3's *Beat*).
- 2026-10-21, group 1, pre-review L2, `modules.sh`: its three whole-set fixture runs go through
  `unmarked`, so each wraps itself as a real run does (and needs the fixture's own `lib.sh`, which
  it already copies). The alternative, `CHECKS_DOUBLE_PASS=1`, keeps them unwrapped and cheaper,
  but tests a path production no longer takes; under the single pass PATH is already stripped, so
  the fixture's wrap mirrors nothing.
- 2026-10-21, group 1, task 1.5, each fixture failing first. Each revert was made in a copy of
  `template/` (one copy per revert, the tree untouched), and that copy's no-clauductor.sh run with
  `CHECKS_SET=whole`; the fixture lines it printed:
  - **the wrap** (run.sh's `if … bare_env … exit` removed): `FAIL falsified: a check that calls
    clauductor fails a whole-set run.sh (the single pass) (exit 0, want 1)`, with `FAIL ...and
    passes under the real run.sh` and `FAIL the gap: a check that fails only with clauductor on PATH
    passes the single pass (exit 1, want 0)`;
  - **the outside-the-environment case** (`bare_assert`'s `CHECKS_SET=whole` branch made `if
    false`): `FAIL falsified: a whole-set run.sh with the wrap removed fails the environment
    assertion (exit 0, want 1)`;
  - **the real-state check** (`bare_assert` returning 0 whenever `CHECKS_BARE` is set): `FAIL
    falsified: a stale CHECKS_BARE fails the assertion, which reads the real state (exit 0, want 1)`;
  - **the `CHECKS_DOUBLE_PASS=1` switch** (run.sh wrapping regardless of it): `FAIL ...and fails
    under CHECKS_DOUBLE_PASS=1, the double pass (exit 0, want 1)`.
  With the code as written, every fixture line passes (`ok`), alone and inside both real sets.
- 2026-10-21, group 1, task 1.6, `clauductor update`: run from the worktree with
  `CLAUDUCTOR_FRAMEWORK` unset, so it reads this checkout's template (with it set, the binary read
  the main checkout's `template/`, which has none of this change). It also merges the template's
  `roles.reviewer.eval.baseline` into `.claude/model-roles.json` and reformats that file: drift
  already on `main`, not this change's, and an eval key (rule 13's ground), so that file was
  restored after each update and left for its own change.
- 2026-10-21, group 1, task 1.6, the totals after the change (same Mac, Apple M5, 10 cores; one
  run at a time, but the owner's other lanes were live: load average 3 to 5 throughout, where the
  baseline had nothing else running). Single pass: `sh template/.claude/checks/run.sh` **347 s**
  (baseline 698 s: −351 s, −50%), 44 checks; `sh .claude/checks/run.sh` **380 s** (baseline 766 s:
  −386 s, −50%), 47 checks, `local:roadmap-tracker` and `local:team-queue` passing inside the
  environment. Each saving is close to the baseline's `no-clauductor` share (354 s and 358 s), so
  the Mac agrees with the amendment's "half the set" (201.9 s of 400.25 s on Linux): no
  contradiction to stop on. Both sets end `checks: all N passed (in …, without clauductor)`.
  Double pass: `CHECKS_DOUBLE_PASS=1` on the template set 764 s, on this repo's 845 s, both
  passing; 9 to 10% over the baseline, which this load explains (the new fixtures add about 2 s;
  the one `PATH` directory holding `clauductor`, `~/.local/bin`, has 7 entries to mirror).
  `go test -count=1 -run 'TestPluginChecksPassInScaffoldedProject|TestCommittedPluginIsCurrent'
  ./internal/plugin/ ./internal/cmd/` passes with `-timeout 25m` (the plugin test 402 s); with go
  test's default 10 minutes, inside the sandbox, the plugin test timed out at 600 s. The gate
  already runs it with `-timeout 25m` (`scripts/ci/steps.sh:121-123`), as CI does.
- 2026-10-21, group 1, review round 1:
  - **M1, the stale-marker fixtures:** the one stale-`CHECKS_BARE` fixture ran with the stub on
    PATH, so it failed at the PATH test and never showed the HOME or `~/.clauductor` test failing.
    Now each part of the real state has its own fixture: a stale marker with the stub (PATH test),
    without the stub and with a HOME of its own that holds no `~/.clauductor` (HOME test, a CI
    runner's case), and `CHECKS_BARE` whose HOME is the environment's but holds a `~/.clauductor`
    (that test), plus a control (the same HOME without one passes). Each also checks the reason
    printed. Failing first, each clause deleted in a copy: HOME test gone → `FAIL falsified: a stale
    CHECKS_BARE with no clauductor and no ~/.clauductor fails the assertion (exit 0, want 1)` and
    `FAIL the run did not say 'not the environment's'`; `~/.clauductor` test gone → `FAIL falsified:
    CHECKS_BARE with its own HOME holding a ~/.clauductor fails the assertion (exit 0, want 1)` and
    `FAIL the run did not say '.clauductor exists'`. A first try ran the HOME fixture with the
    caller's HOME: on this Mac that HOME has a `~/.clauductor`, so with the HOME test deleted the
    run still failed (on the other test) and only the reason line caught it. Giving the fixture a
    HOME of its own makes the exit code fail too, on any machine.
  - **M2, a TERM to the wrapping run.sh:** the child now runs in the background, and the parent
    `wait`s for it, with an INT/TERM trap that sends the child TERM, waits for it, removes the
    scratch and exits 1. To make `$!` the child run.sh itself and not a subshell around it, `lib.sh`
    gains `bare_exec` (`exec env …`), and `bare` becomes `(bare_exec "$@")`, so the overrides are still
    written once. Two consequences, both stated in run.sh's header:
    - a command started in the background with no job control ignores INT, and a shell cannot undo
      that (`trap - INT` in the subshell does not; tried on macOS sh and dash). So every check now
      runs with INT ignored, and the parent stops the child with TERM for an INT too. No check
      sends INT (grepped). The INT half of lib.sh's scratch trap, and of people.sh's widened copy,
      now never fires, while their TERM half still does. The rejected alternative was `set -m` (a process group to
      signal whole). It is off when there is no terminal (dash), and a background group reading the
      terminal stops on SIGTTIN.
    - the check running when the TERM lands is not signalled and runs to its end, as the current
      check did when an unwrapped run.sh was killed. A KILL to the parent still orphans the child
      (no trap can catch it). A watchdog in the child (stop when the parent is gone) would be a new
      mechanism, and is not added.
    The new fixture runs a fixture set of two checks (the first sleeps 3 s), sends TERM once the
    first has started, and checks three things: the run exits non-zero before that check
    finishes, the second check never starts, and the fixture's TMPDIR is empty afterwards. Failing
    first, with the child back in the foreground: `FAIL a TERM did not stop the whole-set run.sh
    promptly (exit 1, first check finished first: 1)` and `FAIL the set ran on after a TERM: the
    next check started`. It passes under macOS sh and under dash as `sh`. no-clauductor.sh's
    fixtures went from about 6 s to about 11 s.
  - **L1:** the summary's ", without clauductor" now needs a whole-set run as well as
    `CHECKS_BARE`. The test is this run's own `$# -eq 0`, not `CHECKS_SET`, which a named run inside
    the set inherits.
  - **L2:** `fx CHECKS_DOUBLE_PASS=1` through `bare_assert` with the real run.sh expects 0. Failing
    first, the exemption deleted: `FAIL ...and passes under CHECKS_DOUBLE_PASS=1, where
    no-clauductor.sh re-runs the set itself (exit 1, want 0)`.
  - **L3:** run.sh's header now says only what HOME carries is lost, and that `GH_TOKEN`,
    `SSH_AUTH_SOCK`, `GIT_CONFIG_GLOBAL` and `XDG_CONFIG_HOME` pass through.
  - After the fixes, with the plugin rebuilt and `.claude/` updated (`model-roles.json` restored
    again): both single passes still pass, the template set in 418 s and this repo's in 445 s
    (load average 3 to 6). The plugin layout's tests pass too: `TestPluginChecksPassInScaffoldedProject`
    375 s, plus `TestCommittedPluginIsCurrent`. 1.6's timings above remain the change's
    measurement. These re-runs show the fixes pass on a loaded machine; they are not a second
    timing.
- 2026-10-21, group 1, task 1.3 as amended (D1: `exec`), the inner run's cleanup: the outer run
  passes its scratch (which holds the environment) in `CHECKS_BARE_RM`; the inner run sources
  `lib.sh` and takes that directory as its own `_scratch`, so lib.sh's existing traps (EXIT, INT,
  TERM) remove it. It takes it only when `CHECKS_BARE` is `$CHECKS_BARE_RM/bare`, and unsets
  `CHECKS_BARE_RM` before any check runs, so a nested whole-set run (a fixture with an explicit or
  stale `CHECKS_BARE`) can never remove its parent's environment. Rejected: deriving the directory
  from `CHECKS_BARE`'s parent (a fixture's `CHECKS_BARE=$d/stale` would then remove no-clauductor.sh's
  own scratch `$d`), and traps of run.sh's own (a second copy of lib.sh's, when run.sh sources lib.sh
  anyway for `bare_assert`). A signal ends the run with `exit 1`, as it ends every check, rather than
  re-raising it. Round 1's background child, `wait` and INT/TERM trap are gone. `lib.sh`'s
  `bare`/`bare_exec` split stays: run.sh's `exec` is `bare_exec` in its own shell. Left as stated, not
  guarded: a signal in the instant between the `exec` and the inner run's sourcing lib.sh leaves the
  environment in TMPDIR; a TERM sent to run.sh's pid alone (not its group) takes effect when the
  running check ends, since a shell runs a trap once its foreground command returns (run.sh's
  header says so).
- 2026-10-21, group 1, round 2's low, the summary from the real state: run.sh's ", without
  clauductor" now needs a whole-set run (`$# -eq 0`), `CHECKS_BARE` and `bare_assert` returning 0,
  the same function no-clauductor.sh's part 2 uses. Fixture: a check that passes anywhere, under a
  stale `CHECKS_BARE` with the stub on PATH, passes but its summary lacks the suffix; the wrapped run's
  summary has it. Failing first, the suffix from the marker alone: `FAIL a stale CHECKS_BARE earned
  the summary 'without clauductor': checks: all 1 passed (in …/single, without clauductor)` (round 1's
  run.sh fails it the same way).
- 2026-10-21, group 1, round 2's low, one setup helper: `fx_project DIR` (a repo with this
  directory's run.sh and lib.sh and conf.sh) builds both the single-pass fixture project and each
  signal fixture's.
- 2026-10-21, group 1, the signal fixtures (round 2's medium and low), how INT is delivered: the run
  under test runs in no-clauductor.sh's FOREGROUND, so it keeps INT; the sender runs in the
  background (a background command ignores INT, but can still send one). The sender waits for the
  check's marker (`a.started`, holding the check's pid; at most 20 × 1 s), then signals run.sh's pid
  and the check's pid in one `kill`, standing in for a terminal, which signals its whole foreground
  process group: no sh can make a group without job control (`set -m` is off without a tty in dash).
  The check traps INT and TERM around `sleep 10 & wait` (a `wait` returns at once on a trapped
  signal) and writes `a.stopped`. After the run returns: exit non-zero, `a.stopped` present and
  `a.done` absent (the signal reached the check and stopped it), `b.started` absent (the next check
  never started), the fixture's TMPDIR empty (the environment removed). If no-clauductor.sh itself
  was started with INT ignored, no run it starts can catch INT: a probe (`sh -c 'trap … INT; kill
  -INT $$'`) then fails the INT fixture as `cannot check INT`, rather than letting it pass or fail
  for the wrong reason; shown by running no-clauductor.sh in the background of `sh` (the TERM
  fixture still passes there). The places that run the checks keep INT: the gate's steps run in the
  foreground, go test's `exec`, CI, and a background command of the Bash tool (probed: it caught
  INT). Failing first, each in a copy of `template/`, run with `CHECKS_SET=whole`:
  - round 1's run.sh (the background child): `FAIL INT did not stop the running check and the run
    (exit 1, check stopped: no, finished: no)` (its TERM half passes: round 1 handled TERM);
  - a foreground child (the approved text before the amendment, `bare … sh "$dir/run.sh"; exit $?`):
    `FAIL the set ran on after INT: the next check started` and `FAIL the set ran on after TERM: the
    next check started`;
  - the inner run not taking the environment as its scratch: `FAIL INT left checks.3aaQ8q behind`
    and `FAIL TERM left checks.TE87EE behind`.
  With the code as written both pass, under macOS sh (bash 3.2) and under dash as `sh`;
  no-clauductor.sh's fixtures take about 5 s.
- 2026-10-21, group 1, review round 3 (peak medium). Each fix, and each new fixture failing first in a
  copy of `template/` run with `CHECKS_SET=whole` (the copy's other fixture lines all `ok`):
  - **M1, a launch with INT ignored** (`run.sh &` from a script, a wrapper with no job control): the
    INT fixture no longer fails there. When the probe finds INT ignored, it prints `ok   SKIPPED the
    INT fixture: this check was started with INT ignored (in the background, with no job control),
    so no run it starts can receive INT; the TERM fixture runs`, and the TERM fixture runs. This is
    the reviewer's ruling within D7, and it departs from lib.sh's "a check that cannot run what it
    checks must FAIL": the state it cannot test is the launcher's, not run.sh's, and every other
    part of the check still runs. Shown: `sh -c 'CHECKS_SET=whole sh …/no-clauductor.sh & wait'`
    prints that line and `ok   TERM to a whole-set run.sh reaches its running check …`; started the
    same way, round 1's run.sh still fails the TERM fixture (`FAIL TERM did not stop the running check
    and the run (exit 1, want 143; …)`). In the foreground the INT fixture runs and fails first as
    listed below.
  - **L2, part 2's `1` never falsified:** the three-way choice is lib.sh's `bare_report` (0 reports
    ok, 1 reports fail with the reason, 2 reports nothing and the caller re-runs). Part 2 and the
    fixture check both call it. Failing first, `bare_report` reporting a 1 as `ok`: `FAIL falsified:
    a whole-set run.sh with the wrap removed fails the environment assertion (exit 0, want 1)`, and
    the same `(exit 0, want 1)` for each of the three stale-`CHECKS_BARE` fixtures.
  - **L3, a reused pid:** the sender signals only if the check's marker exists (so the run is alive),
    and is killed when the run returns.
  - **L4, re-raise:** the inner run's INT and TERM traps remove the environment, reset the trap and
    `kill` the run with the same signal, so it exits 130 or 143 and a caller sees the signal. The
    signal fixtures now want exactly that status. Failing first, with lib.sh's `exit 1` traps
    instead: `FAIL INT did not stop the running check and the run (exit 1, want 130; check stopped:
    yes, finished: no)`, and the same for TERM with 143. The no-clauductor.sh around the fixture is
    not stopped by it (bash 3.2 and dash).
  - **L5, `CONTEXT_OFFLINE`:** run.sh sources lib.sh through `use_lib`, which puts the caller's
    `CONTEXT_OFFLINE` back (unset when it was unset), so a check run by its own `#!` line sees what it
    would unwrapped. New fixture: a check that does not source lib.sh fails if it sees `1` under a
    caller's empty value. Failing first, `use_lib` not restoring it: `FAIL a whole-set run.sh passes
    the caller's CONTEXT_OFFLINE on, not lib.sh's 1 (exit 1, want 0)`.
  - **L6, the globals:** `np` and `H` are now `BARE_PATH` and `BARE_HOME`, beside `BARE_DIR`, rather
    than `_b` names: no-clauductor.sh reads the stripped PATH for its fixtures, so it is a name a
    caller uses, not a private temporary; no check uses a `BARE_` name of its own (grepped).
  - **L7, the summary losing its subject:** noted, not fixed. If a check created `~/.clauductor` in
    the environment's HOME, the run's last line would drop ", without clauductor" while still
    passing. Hypothetical: no check writes there, and no-clauductor.sh's part 2 asserts the same
    state within the run.
  - **M1 follow-up, the skip seen through run.sh:** inside a run.sh run the SKIPPED line was folded
    into `PASS no-clauductor (42)`. run.sh now prints a passing check's `^ok   SKIPPED` lines,
    indented, under its PASS line. That is the existing convention (`artifacts.sh`, `build-change.sh`,
    `health.sh`, `openspec.sh` and the openspec module print `ok "SKIPPED…"`), so their skips show
    too. New fixture: a passing check that prints one `SKIPPED` line must show it on the line after
    `PASS c (2)`. Failing first, the `printf … | grep '^ok   SKIPPED'` line removed: `FAIL run.sh
    swallowed a passing check's SKIPPED line: PASS c (2)`. Launched with `&` from a non-interactive
    sh, `run.sh no-clauductor` now prints `PASS no-clauductor (43)` and, under it, `    ok   SKIPPED
    the INT fixture: …`.
  - **Review round 4, L1, `CONTEXT_OFFLINE` truly unset:** a second fixture case runs the fixture
    run.sh through `sh -c 'unset CONTEXT_OFFLINE; exec "$@"'` (POSIX, where `env -u` is not), and its
    check fails unless `${CONTEXT_OFFLINE+x}` is empty. Failing first, run.sh's `unset
    CONTEXT_OFFLINE` in `use_lib` replaced by `:`: `FAIL ...and a caller with no CONTEXT_OFFLINE
    passes none on (exit 1, want 0)`; the empty-value case still passes there, which is why it
    alone did not cover the branch. Passes under macOS sh and dash.
  - **Review round 4, L2:** part 2's failure excerpt (`grep -E '^(FAIL|    )'`) now drops run.sh's
    indented `    ok   SKIPPED` lines, which would otherwise take the place of a failure's reason
    in its eight lines. Shown on a sample output: before, `    ok   SKIPPED: node is not installed`
    led the excerpt; after, only `FAIL b` and `    FAIL broke`.
  - The earlier fail-first copies, re-run on the new code, still fail: round 1's run.sh fails INT
    (`check stopped: no`), TERM (`exit 1, want 143`), the summary and `CONTEXT_OFFLINE`; a foreground
    child fails both signals (`the set ran on after INT` / `after TERM`); no scratch adoption leaves
    the environment (`FAIL INT left checks.1e9ap1 behind`, the same for TERM); the suffix from the
    marker alone earns it under a stale marker.

## 1. The single pass (D1, D2, D3, D4, D6, D7)
- [x] 1.1 Before any edit, time a whole-set `sh template/.claude/checks/run.sh` and `sh .claude/checks/run.sh` on the owner's Mac, and record both totals and `no-clauductor`'s share in the Decision log as the baseline. (manual: a one-off measurement on the owner's machine, not shipped code)
- [x] 1.2 `template/.claude/checks/lib.sh`: move `no-clauductor.sh:20-39` unchanged into one helper, which builds the `PATH` mirror, the fresh `HOME` with its `.gitconfig`, and `bare` (D2). Add D3's assertion as a function: `command -v clauductor` fails, `$HOME` is the environment's `HOME`, and `$HOME/.clauductor` is absent.
- [x] 1.3 `template/.claude/checks/run.sh`: for a whole-set run with no `CHECKS_DOUBLE_PASS=1`, source `lib.sh` after `ROOT` is set, build the environment, and `exec` itself under it with `CHECKS_BARE` set, telling the inner run which directory to remove; the inner run removes it on `EXIT` (D1, amended 2026-10-21). Export `CHECKS_SET=whole` for every whole-set run. A whole-set run that finds `CHECKS_BARE` set runs the checks directly: that is the inner run's stop condition. Named runs and `CHECKS_DOUBLE_PASS=1` do not wrap (D1, D4). Update the header comment to say so.
- [x] 1.4 `template/.claude/checks/no-clauductor.sh`: use the helper. Part 2 asserts under `CHECKS_BARE`, fails under `CHECKS_SET=whole` with no environment, and re-runs the set otherwise (D3). The re-run clears `CHECKS_SET` and sets `CHECKS_BARE` to its own environment. It runs `$dir/run.sh` (D6). Rewrite the header's step 2 to say all this.
- [x] 1.5 The fixtures in `template/.claude/checks/no-clauductor.sh` (D7), each shown failing first, by reverting the code it covers and recording the failure in the Decision log. Each fixture run clears `CHECKS_BARE`, `CHECKS_SET` and `CHECKS_DOUBLE_PASS`, then sets only what its case needs, so it behaves the same inside the gate's single pass and inside the weekly double pass:
  - a fixture check that calls `clauductor` fails a whole-set run of the fixture `run.sh`, with a stub `clauductor` on the outer `PATH`;
  - a fixture `run.sh` with the wrap removed fails D3's assertion;
  - a fixture check that fails only with `clauductor` on `PATH` passes the single pass and fails under `CHECKS_DOUBLE_PASS=1`.
- [x] 1.6 Bring the copies along and measure:
  - run `scripts/build-plugin.sh`, then `framework/clauductor update` from the repo root, with a binary built from this checkout;
  - `sh .claude/checks/run.sh` and `sh template/.claude/checks/run.sh` pass. Time both on the Mac, and record the totals against 1.1's baseline in the Decision log. If they contradict the amendment's figures, stop and ask the owner (design, *What rests on sources outside the repo*);
  - `CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh` and `CHECKS_DOUBLE_PASS=1 sh template/.claude/checks/run.sh` pass. This proves the rollback on the real sets, and is what the weekly job runs (D4, D7);
  - `cd framework && go test -count=1 -run 'TestPluginChecksPassInScaffoldedProject|TestCommittedPluginIsCurrent' ./internal/plugin/ ./internal/cmd/` passes. This is the plugin layout's wrapped whole-set run (D6).

## 2. The weekly double pass and the record (D5, D8, D9)
- [ ] 2.1 `.github/workflows/checks-double-pass.yml` (D5):
  - a weekly `schedule:` (the cadence the owner chose in O3) and `workflow_dispatch`, on Ubuntu, with `permissions: contents: read` and actions pinned to `test.yml`'s SHAs;
  - copy `test.yml`'s setup steps (Go, tmux and the OpenSpec CLI), and set `OPENSPEC_REQUIRED=1` and `NODE_REQUIRED=1` as its process-checks step does;
  - build `clauductor` from `framework/` onto `PATH`;
  - run `CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh` and `CHECKS_DOUBLE_PASS=1 sh template/.claude/checks/run.sh`;
  - a header comment naming its reader, `.claude/health/scheduled-workflows.sh`.
- [ ] 2.2 `.claude/local/checks/double-pass.sh` (D5): it fails when the workflow is missing, has no `schedule:`, does not set `CHECKS_DOUBLE_PASS=1`, does not put a built `clauductor` on `PATH` before the run, or does not run both sets. It is falsified on fixture copies of the workflow, one per missing piece, each shown failing first. `sh .claude/checks/run.sh local:double-pass` passes.
- [ ] 2.3 Insert design.md's amendment block verbatim into `docs/adr/0006-a-repository-works-without-clauductor.md`, between `## Enforcement` and `## Related` (D9), with the heading date the owner chose in O1. Prove it word for word: extract the fenced block from design.md and from the ADR's amendment section, and `cmp` the two (they differ only where the owner's answers to O1, O2 or O6 changed a word). Run `sh .claude/checks/run.sh adr-numbering`.
- [ ] 2.4 `template/docs/playbook.md` and `docs/playbook.md`: change the no-clauductor bullet to say the checks run once inside that environment, and name `CHECKS_DOUBLE_PASS=1` as the switch back to the double pass (D4, O5). Then rebuild the plugin and run `clauductor update` again. In `template/AGENTS.md` and this repo's `AGENTS.md`, replace the second cell of the row "The repo works without clauductor" with D9's words for that copy. `sh .claude/checks/run.sh agents-md-budget` and `sh template/.claude/checks/run.sh agents-md-budget` pass.
- [ ] 2.5 The release-notes item (D8), in `docs/roadmap.md`: if REL-1's row is still open, append the item to its Change cell; if REL-1 has merged, add a queued Phase 4 row whose Change cell says the next release's `CHANGELOG.md` section carries the item, in *Changed*. Then `sh .claude/roadmap-queue.sh --check` passes. If `CHANGELOG.md` is on `main` and a version section is being prepared, add the item there too, never under `[Unreleased]`.
- [ ] 2.6 The full gate passes (`GATE`). The Mac's before and after totals stay in the Decision log, which the build PR carries.

- [ ] Slice: a contributor runs `sh .claude/checks/run.sh` and every check runs once, inside the no-clauductor environment, in about half the time; `CHECKS_DOUBLE_PASS=1` brings back the double pass, which runs weekly on `main`
