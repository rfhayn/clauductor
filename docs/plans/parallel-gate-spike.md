# The parallel-gate spike: results (OPS-105)

**Status:** results, measure only. Nothing was built, and nothing is built on these results until
the owner decides Q28 (slots before launch, B, or in v0.1.x, C) along with #168's fix and the
integration rule.
**Question:** can two full gates of this repo run at once on the owner's Mac without changing each
other's verdict, and at what pair slowdown *s*?
**Criteria:** G1–G5, as the owner approved them on 2026-10-21 (OPS-105's row in `docs/roadmap.md`).
**Raw data:** `~/spike/` on the owner's Mac, kept until this note merges: `single-N` and
`pair-N-{a,b}` `.out` and `.log` files, each clone's `.git/clauductor/gate-runs.log` (each run's
result and each step's seconds) and `mem.log` (one sample every 5 s).

## Verdict

**Two concurrent gates do not keep their verdicts today: G2 fails.** 7 of the 10 gates run in
pairs failed, and each failure was in a test that passes when its gate runs alone. The machine
itself kept up. The slowdown where it could be measured was about 1.11 (G1), memory stayed under
its ceilings (G4), and no step came near a timeout (G5). The failures come from load, not from one
clone reaching into the other (G3, with the gaps that section states). So parallel slots, under B
or C, need those tests hardened first. **OPS-122** owns that, and the rerun that would show it
worked.

| | Criterion | Result |
|---|---|---|
| G1 | Slowdown: *s* ≤ 1.5 in at least 4 of 5 pairs, median *s* ≤ 1.5; throughput at least 1.33× serial | **Pass where measured, but short of its sample.** *s* = 1.11 and 1.12 in the 2 pairs that ran full length together. Throughput was 1.79–1.80× serial. The other 3 pairs measure no *s* |
| G2 | Verdicts: zero disagreements with the singles; every run writes a complete receipt for the right SHA | **Fail.** 7 of 10 pair runs failed where the singles passed 3 of 3. Receipts were correct |
| G3 | Isolation: no shared lease, log, receipt, scratch, tmux socket or port; trees unchanged; no leftovers | **Pass on the evidence there is.** It was not instrumented for tmux sockets, ports or a before-and-after snapshot of each git directory |
| G4 | Memory: combined peak ≤ 12 GB, swap growth ≤ 1 GB, pressure never critical | **Pass.** Peak total RSS was 11.0 million KB (10.5 GiB), swap did not grow, and pressure peaked at warn |
| G5 | Timeouts: no step hits its timeout; the race step ends within 20 minutes in every pair | **Pass.** The longest race step took 471 s (7.9 minutes), and no step ended on a timeout |

## The run

- **When:** 2026-10-22, from 8:38 to 11:00 AM ET (`run.out`: `done Thu Oct 22 11:00:27 EDT 2026`).
  It ran in the morning rather than overnight, from a plain Terminal, with the panel off.
- **What:** two full clones, `~/spike/a` and `~/spike/b`, both at `f9a5f396`, the head of `main`
  after #201. `~/spike/run.sh` ran three single full gates in `a`, then five pairs, each starting
  `a` and `b` together and waiting for both.
- **Machine:** the owner's Mac (16 GB). Before the spike, swap held 4270.50M of 5120M, and the
  total RSS of all processes was about 9.5 million KB.
- **Sampler:** `mem.log` recorded the date, the memory pressure level, free memory, `vm.swapusage`
  and total RSS every 5 s. The level lines start at 8:40:29 AM ET, so the first two minutes of
  single 1 have no level reading. The sampler had stopped by 11:32 AM ET.

## The runs

Times are each run's total in seconds, from `gate-runs.log`. Every run's queue time was 0 s, so
neither clone ever waited on a lease.

| Run | a | b | Ran together for the whole run? |
|---|---|---|---|
| single 1 | pass, 1027 | — | — |
| single 2 | pass, 1018 | — | — |
| single 3 | pass, 1011 | — | — |
| pair 1 | pass, 1131 | **FAIL** (race), 1133 | yes |
| pair 2 | **FAIL** (race), 1136 | **FAIL** (race), 1136 | yes |
| pair 3 | **FAIL** (race), 1058 | **FAIL** (short), 315 | no: `b` ended at about 5 minutes |
| pair 4 | **FAIL** (short), 305 | pass, 1038 | no: `a` ended at about 5 minutes |
| pair 5 | pass, 1055 | **FAIL** (short), 316 | no: `b` ended at about 5 minutes |

The median single took **1018 s**, close to the 1048 s measured on #198's head the day before.

### Each step, single against pair

The median seconds of each long step: the three singles, then the four runs of pairs 1 and 2,
which overlapped for their whole length. Every other step took 3 s or less.

| Step | Singles | Full-length pairs | Ratio |
|---|---|---|---|
| process checks | 245 | 268 | 1.09 |
| test (short) | 28 | 32.5 | 1.16 |
| template checks | 334 | 362 | 1.08 |
| test (race) | 410 | 465 | 1.13 |

## G1, slowdown

*s* is a pair's median gate time over the median single (1018 s).

- **Pair 1:** median 1132 s, so *s* = **1.11**. **Pair 2:** median 1136 s, so *s* = **1.12**.
- **Pairs 3 to 5 do not measure *s*.** In each, one gate failed in its short step at about 5
  minutes, and its partner then ran alone for its remaining 12 minutes or so. The partner's time
  (1058, 1038 and 1055 s, so 1.02 to 1.04 of the single) is mostly a single's time. Their pair
  medians (about 0.67) only show that one gate stopped early.
- **Throughput:** pairs 1 and 2 finished two gates in 1133 and 1136 s of wall clock, against
  2036 s for two singles run one after the other. That is **1.80× and 1.79× serial's work**,
  well over the 1.33 the criterion asks.

G1 asks for *s* ≤ 1.5 in 4 of 5 pairs. Every *s* measured meets it by a wide margin, but only 2
pairs measured one, so the sample the criterion names was not reached. A rerun after OPS-122
completes it.

## G2, verdicts

**Fail.** The singles passed 3 of 3, and the pairs failed 7 of 10 on the same SHA. Each failing
test passed in all three singles:

| Test | Step | Pair runs failed | What it said |
|---|---|---|---|
| `TestStallPushUntrustedTitle` (`internal/panel`) | short (3), race (1) | 3b, 4a, 5b; 3a | `push_test.go:763: the push's arguments [""]` (or `["clauductor panel · a"]`), want the title and the body |
| `TestLeaseConformanceCatchesEveryMutant` (`internal/panel/lease`) | race | 1b, 2a, 2b | `conformance_test.go:297: the suite passed a lease.sh with …`: 3, 5 and 4 mutants missed |
| `TestDaemonLeftByTheGateDoesNotHoldTheLease/shell` | race | 1b, 2a, 2b | `lease_shell_test.go:497: took 4.15 s` (5.94 s, 6.84 s): the daemon's fd held the lease |
| `TestShellLeaseInteroperatesWithLockRun` | race | 2a, 2b | `lease_shell_test.go:124: timed out after 10s waiting for three iterations of the shell's wait loop (CLAUDUCTOR_TEST_SLOW=1)` |
| `TestTemplateLaneGetsItsFirstPromptOnceWhenReady` (`internal/panel`) | race | 2a | `orchestration_test.go:195: the restart re-notified` |

What the code shows about each:

- **The stall push test reads a record that is still being written.** Its stand-in
  (`pushStandIn`, `push_test.go:35`) runs `printf '%s\n' "$@" > runs/run.$$`. The redirection
  creates the file before `printf` writes to it, and the test's wait ends as soon as the file
  exists. Under load it read the file empty (`[""]`) or with the title line only. This is a test
  bug that load exposes, not a panel bug, and not the other clone.
- **The lease tests run against fixed wall-clock bounds.** One is the 4 s bound at
  `lease_shell_test.go:496`. Another is the conformance driver's `CONFORMANCE_WAIT=2` and
  `CONFORMANCE_TIMEOUT=12`, which nothing scales. `testwait`'s own waits run at
  `CLAUDUCTOR_TEST_SLOW=1` in the gate. A mutant reads as "passed" when its failing case runs out
  of time before it can say `not ok`. #92 is open on the same `DaemonLeft` test. #169 fixed a
  zombie cause that made it fail, and this is its other bound, under load.
- **The lane prompt test** re-notified once across a restart. Its cause was not read here.

Every failure is in the race step or the short step, the two steps whose time grew most in a pair
(1.13 and 1.16 against 1.08 and 1.09). That fits load as the cause.

**Receipts were right.** Each passing run reported its receipt in its own clone
(`receipt: …/ci-receipt (clean)`), every run log line names `f9a5f396`, and `a`'s last receipt
reads `f9a5f396… full clean all`. Each failing run removed its clone's earlier pass ("a stale pass must not
outlive a fail"), which is the intended behaviour. Each clone's `gate-runs.log` holds only its own
runs: 8 in `a` and 5 in `b`.

## G3, isolation

**Pass on what was observed.**

- **Lease:** each clone has its own `<git dir>/clauductor/gate.lock`, and every run's queue time
  was 0 s, so neither clone held the other's lease.
- **Run log and receipt:** each clone's `gate-runs.log` lists only its own runs, and each receipt
  names its own clone's path.
- **Trees:** after the spike, both clones are at `f9a5f396`, and `git status --porcelain` is
  empty in both.
- **Errors:** no `address already in use`, `bind:` or lock error appears in any `.out` or `.log`.
- **Leftovers:** no process with either clone's path, and no `gate.sh`, `run-local.sh` or
  `go test` binary, was running at about 11:40 AM ET (`ps -axo`).
- **Not measured:** the script took no snapshot of each git directory before and after (only HEAD
  and status afterwards), and nothing watched tmux sockets, ports or scratch directories during
  the run. The test failures above trace to timing within one clone, and none shows a path or a
  name from the other clone.

## G4, memory

**Pass.**

- **Peak total RSS:** 11,003,680 KB (11.0 million KB, 10.5 GiB), at 9:57:55 AM ET, in pair 2.
  That is under the 12 GB ceiling. The singles peaked at 9.93 million KB, against a baseline of
  about 9.5 million KB before the spike. Total RSS sums every process's resident pages, shared
  ones included, so it overstates the real use.
- **Swap:** never above its starting 4270.50M during the spike. It ended at 4070.50M, and its
  lowest was 4070.50M. Growth was 0, against a limit of 1 GB.
- **Pressure:** never critical (level 4). Of the samples with a level reading, the spike window
  holds 1640 at normal (level 1) and 28 at warn (level 2). The warn samples cluster in three
  stretches: 10:00:16–10:00:55 AM ET, when both of pair 2's race steps were starting (building
  the race binaries); 10:24:55–10:25:10, at the end of pair 3; and 10:28:16–10:30:27, in pair 4.
  Eleven more warn samples fall after the spike, between 11:00 and 11:32 AM ET.
- **Lowest free memory:** 31%.

## G5, timeouts

**Pass.** No step ended on a timeout: every failure was a test's own assertion, and the step
exited 1. The race step runs under `go test -timeout 25m` (`scripts/ci/steps.sh:227`), and its
longest run in a pair took **471 s**, 39% of the 20-minute bound and 31% of the limit. The short
step's longest took 35 s.

## What it means for Q28 and the next steps

- **The machine can take two gates.** At about 1.11× the time, a pair does about 1.8× the work of
  one gate, with memory to spare: two queued gates would finish in about 1135 s instead of 2036 s.
- **The tests cannot take it yet.** The 7 failures came from two kinds of test fault: a read
  before a write completes, and a wall-clock bound that load can outrun. Until those are fixed, a
  gate in a slot would fail most of the time with nothing wrong in its change. **OPS-122**
  hardens those tests and the rest of their class, then reruns this script. Its done condition is
  G2 at zero disagreements, which also completes G1's sample.
- **Q28 (B or C)** is the owner's, at its decision point, on this note. Under either option the
  slots wait for OPS-122. Under B, OPS-122's size (not estimated here) adds to the sessions before
  REL-1.
- **#168** (a SIGKILLed lock-run costs the next waiter 30 s) was not exercised: no run was killed.
- **To rerun:** the owner queue's OPS-105 item has the script (steps 1 to 5), and it should also
  log `git -C <clone> rev-parse HEAD` and a snapshot of each git directory before and after, plus
  `lsof -i` and the tmux socket list during each pair, to close G3's gaps.
