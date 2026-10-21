# Tasks: panel-51-stall-alert

## Progress
- 2026-10-18 proposed; nothing built yet. The build starts from `main` once approved, on the
  owner's Mac (session 5), beside PANEL-49's build.
- 2026-10-18 pre-review (propose step 2b), for the proposal PR's body (this replaces the
  earlier lines of 5a94976 and 52983b7):
  Review: converged by the owner's decision after 7 round(s), peak medium; round 7's M1 and M2
  (and L1 to L4) fixed without re-review at 0714273 · design c9d2de7c70b7 (reviewed through 7f5dce5 ·
  design d5257196b98e). The build's group-1 probes on the Mac settle the hook-ordering cases.
  - Rounds: 1 peak high (16 findings), 2 peak high (13), 3 peak high (8), 4 peak medium (8),
    5 peak medium (10), 6 peak medium (6), 7 peak medium (6). Rounds 1 to 6 were fixed in
    full; round 7's by the owner's decision of 2026-10-18, with no round 8. Peak never rose
    after round 3.
  - Round 3's proposed fixes: F1 name only paths the build changes (or unbacktick `/hook`,
    `/loop`); F2 append the PANEL-46 and PANEL-25 tasks in this proposal's PR and drop 5.3
    (`cg_build_tasks` skips a PR touching only `changes/`); F3 save each waiting session's ask
    time in `stall.json`; F4 save each live session's decoded `Stop` allowlist there, or read
    (d) as unknown after a restart; F5 reword S19 to a stalled lane whose re-read fails; F6 list
    the project-level sources, the hooks by the install check, and keep a mark while its way is
    unjudged; F7 the lease's release as movement, `stall.json` written at most every N seconds;
    F8 align the requirement title, the lease wording and the proposal's `panel.js` line.
- 2026-10-18 round 3 fixed on the owner's Mac (session 5): F1 the probe tasks name only this
  change's `tasks.md`; F2 task 5.3 dropped, PANEL-46's task 2.9 and PANEL-25's task 3.8 appended
  in this PR; F3 and F4 `stall.json` keeps each waiting session's ask time and each live
  session's kept `Stop` fields (STALL-1-S20, S21); F5 STALL-1-S19 reworded to a stalled lane
  whose re-read fails; F6 the sources listed; F7 `stall.json` written at most every 30 s; F8 the
  requirement title and the proposal's page line.
- 2026-10-18 review round 4: peak medium (2 medium, 6 low). Fixed: M1 a wakeup holds off only
  the open-work stall, never a waiting session's (STALL-2-S7); M2 a running background subagent
  or workflow is working until its `SubagentStop` (D3 (d), STALL-2-S8, P2 extended); L1 a dead
  holder first seen is the lease given up (S18); L2 `fileReadSites` in 3.3 and 4.2; L3 how 5.1's
  browser lane stalls; L4 the alert's `since` when both ways hold; L5 an outage is not quiet
  time; L6 PANEL-46 depends on PANEL-51 in the roadmap.
- 2026-10-18 review round 5 (at 52983b7): peak medium; applied in the next entry. Its findings
  and fixes:
  - F1: end a listed task only by a matching `SubagentStop` (`agent_id` equals the task's `id`,
    `signals/testdata/spike-hooks.log` line 5), with `stopSubagent`'s rules for
    `workflow-subagent`, never an empty-type stop (that log's events 9, 10 and 13); or drive it
    from the model's `Subagents` set, with `idleClearsAgents` held off while a listed task is
    open. Correct D3 (d)'s "not known to match", and D4's "the subagent's own work" (the list is
    the session's). P2 records the empty-type stops.
  - F2: judge (d) per session, as the waiting way is: its own busy flag, clock and wakeups; keep
    the lane clock and lane-wide wakeup for (a) to (c). Add a scenario: a main-checkout
    teammate's shell-in-flight stall while a sibling loops on a recurring cron.
  - F3 add the carry-over to STALL-1-S13; F4 a text case for a listed task that has ended; F5
    reword proposal.md's wakeup line to D4; F6 only a session id never seen since start is a
    start; F7 waiting- and idle-kind Notifications are not movement (or P1 records them); F8 a
    background Monitor in P1 or P2; F9 the stalled word overrides D6's "needs you" chip and
    outranks "lost", with the test's full path; F10 a machine source re-reads `push.json` on
    `pathSignature`, and how its warning reaches each project's page.
- 2026-10-18 round 5 fixed at the owner's request (session 5): F1 a listed `subagent` or
  `workflow` task ends only at the `SubagentStop` whose `agent_id` is its `id` (the task `id`
  joins D4's allowlist; empty-type internal stops end nothing; STALL-2-S8 adjusted); F2 a
  session's background-work stall runs on its own clock, held off only by its own wakeups and
  working (D1 now has three ways; STALL-1-S22); F3 the carry-over in STALL-1-S13; F4 the
  "finished and woke nothing" text; F5 the proposal's wakeup line; F6 only an id never seen is
  a start; F7 notifications that report a stop are not movement; F8 a background Monitor in P2;
  F9 PANEL-46 2.9's chip and rank; F10 the machine's `push.json` source.
- 2026-10-18 review round 6: peak medium (1 medium, 5 low). Fixed: M1 a lost or out-of-order
  `SubagentStop` cannot hold a session working for ever (the last 64 `agent_id`s are matched
  whatever the order, and an unpaired task is open work once the session has not moved for 4 ×
  the stall time; STALL-2-S9, S10); L1 a pending task counts as a running one; L2 task 3.4's
  restart case has its paired stop; L3 the seen session ids survive a restart; L4 a question is
  never held off by working; L5 this record, and STALL-1-S21 before S22.
- 2026-10-18 review round 7 (at 7f5dce5): peak medium. Fixed at the owner's request without
  re-review (M1 a question timed from the ask alone, STALL-1-S23; M2 a session not live just
  before is a start, a resumed pane is lane movement, STALL-1-S24; L1 the headlines; L2
  requirement 2's ring; L3 the ended mark on the kept task; L4 no seen-id set, only sessions
  kept aside during failing polls). Its findings and fixes:
  - M1: a background subagent's `SubagentStart`/`SubagentStop` arrive under the parent's
    session id, so an orchestrator asking a question while its own background workflow runs
    has its clock reset at each agent start or stop, and stalls only 30 minutes after the
    workflow's last agent. Fix: time the waiting way from the ask alone (`blocked`'s `Since`),
    or at least exclude `SubagentStart`, `SubagentStop` and agent-tool `PreToolUse`/
    `PostToolUse` from a waiting session's clock; add a scenario (waiting since 00:00, its
    subagent starting at 00:10 and stopping at 00:20, stalled at 00:30 with `since` 00:00).
  - M2: `claude --resume` and `-c` keep the session id (the panel's own Resume and Restore rely
    on it), so a session resumed beside an old open PR is not a start and its lane stalls at
    once; a panel-launched lane's pane coming back is not lane movement either. Fix: a start is
    any listing of a session that was not live just before (ended, or gone from a successful
    poll); keep "not a start" only for a session forgotten while the polls failed; a pane
    coming back after Resume is lane movement; add a scenario (ended at 00:00 beside PR #160,
    resumed under the same id at 03:00, not stalled at 03:01, stalled at 03:30 with `since`
    03:00).
  - L1 reword D1's title, the requirement title and the proposal so "not working" applies to
    the two ways that are not a question; L2 requirement 2 keeps the last 64 `agent_id`s, not
    only the one that ends a kept task; L3 mark a kept task ended on the task itself and save
    it, using the ring only for stops that arrive before their `Stop`; L4 drop a seen id after
    it has gone unlisted for a set age.
- 2026-10-18 group 1 (build-change builder): no task ticked. Finished: a static reading of the
  Mac's Claude Code binary (2.1.303, `~/.local/share/claude/versions/2.1.303`, `strings -n 8`),
  recorded in the Decision log. It already meets 1.4's stop condition (D4's one-shot rule) and
  shows teammates can be in-process (D9's stop condition, unconfirmed live). Left, for the owner:
  the live runs of 1.1 to 1.4 in a throwaway lane on the owner's login, with a throwaway panel
  built from this branch carrying an uncommitted debug log of the decoded `Stop` fields; and the
  owner's amendment of D4's one-shot rule. The builder could not start a live session: its
  sandbox denies writes to `~/.claude/projects` and `~/.claude/session-env`, and the tasks are
  `manual:` on the owner's login.
- 2026-10-18 design amended after group 1 (D4's one-shot rule, D3 (d)'s Monitor rule, the
  Decision log's entries below): STALL-2-S3 and requirement 2 reworded, STALL-2-S11 and S12
  added, tasks 1.2, 1.4, 2.3 and 3.3 follow. The approval is revoked: `design.md` changed, and
  the owner approves the amended design before the build resumes. Nothing is built yet.
- 2026-10-18 amendment review round 1: peak medium (3 medium, 5 low), all fixed: M1 and M2
  STALL-2-S11 gains a same-day lane and a lane restored from `stall.json`; M3 an overdue
  wakeup that does not fire within a minute stops the build (task 1.4, D4); L4 and L7 the
  `M H * * *` shape and "the first H:M at or after the minute the `Stop` arrived in"; L5 the
  Monitor assumption reworded to what the binary shows; L6 STALL-2-S12's Monitor runs in a
  teammate; L8 D4's Beat.
- 2026-10-18 amendment review round 2: converged, peak low (4 low), all applied as wording
  fixes with no round 3: STALL-2-S11's main-checkout cron `35 0 * * *` (within
  `ScheduleWakeup`'s 3600 s clamp); lane #154 listed `idle` by the first poll after its restart;
  D4's boundary in task 2.3 and the Refusals row; "before group 2 commits" in task 1.4.
  For the amendment's PR body:
  Review: converged in 2 round(s), peak low, at 032017b · design 1b4626a580ed
- 2026-10-19 merge-pr step 3 on #167, round 2 of the whole PR (reviewer, at 2bb98a2): peak
  medium, NOT converged. M1 (confirmed from round 1): D3 (d)'s "a task of any other type is open
  work" reads Claude Code's own housekeeping tasks (`dream`, `auto-mode scan`, `memory import`,
  which end silently with no wake) as open work that never ends: a false push 30 minutes after
  a turn that ended while a dream ran. Possibly also ambient `monitor_ws` tasks (unprobed). L2
  task 1.4's overdue case untested (settled live below); L3 STALL-2-S11, S12 and the teammate
  GIVEN at spec line 135 assume a teammate is a session of its own, which P3 refutes. M1's fix
  edits `design.md`, which voids the approval: the owner's, in the re-amendment P2 and P3 force.
- 2026-10-19 group 1's live probes (session 6, TRACK C): 1.1 and 1.4 ticked; **1.2 and 1.3
  meet their stop conditions, so the build stops for the owner before group 2** (Decision log;
  `docs/owner-queue.md`). Nothing is built.
- 2026-10-19 the owner answered by chat (session 6): A and A, and the allowlist (Decision log
  below). D3 (d) and D9 amended, STALL-2-S13 to S15 added, S11, S12 and STALL-1-S22 taken off
  the teammate-as-session assumption; re-approved at design 902e287ac2e2 (head 8764dc7).
- 2026-10-20 merge-pr step 3 on #167, round 1 of the whole PR after the re-approval (reviewer,
  at b53bde9): peak high (1 high, 2 medium, 7 low, 2 nits). Fixed in `spec.md`, `tasks.md`,
  `proposal.md` (outside its Risk line) and `docs/owner-queue.md`, so the approval stands: H1
  STALL-2-S13 rebuilt so it fails if a workflow were read as anything but working (a view at
  00:35 inside the quiet stretch, events in time order, a checkable AND, and a second lane with
  no later `Stop` stalled at 4 × the stall time); M1 tasks 3.3 and 4.2 carry OPS-101's path constraint; M2
  and L5 recorded in the Decision log; L1 S13 to S15 in 2.3's test list; L4 S14 drops `TeammateIdle` and gives
  `t1` a description; L6 the proposal names the allowlist and D9; L7 these entries; the nits.
  L2 and L3 are `design.md` wording, left as written (Decision log).
- 2026-10-20 round 2 (at 83d6acf): converged, peak low (4 low, 2 nits), all applied: "A-T8"
  replaced by OPS-101's row, which exists; STALL-2-S13's WHEN in strict time order;
  STALL-2-S14's title says open work, never working, and its THEN D4's teammate wording; the L5
  entry reworded to what P3 shows; the Decision log in date order.
- 2026-10-20 group 2 (build-change builder): 2.1 to 2.4 built and ticked, uncommitted for the
  group's review. `signals.HookEvent` decodes the two fields (`BackgroundTask`, `SessionCron`);
  `state/stall.go` holds the rule, its `View`/`LaneView` fields, the Needs-you row and the
  `stalled` alert; `state/stall_test.go` cites every scenario 2.3 and 2.4 list (39 tests), each
  rule shown failing under a mutation (wakeups, the 4 × limit, leases, the pairing, the ring,
  the allowlist, the ask's clock, a sibling's hold, the restore, a start). For group 3: the
  runtime calls `SetStallTime` (or 3.1 moves the time into `Thresholds`), `ApplyMergedReads`,
  `ApplyStallState` before the first `claude agents` poll, and saves `StallState(now)`. For group
  4: `View.StallsPending` is the pending flag; the `stalled` alert's `Text` carries the waited-on
  text (a task's description included), so the push needs the open work and the kind of wait from
  fields the alert does not carry yet.
- 2026-10-20 group 2's review (apply-change, session 6 TRACK D, after build-change stopped on a
  quick gate killed at the tool's 20-minute ceiling): **NOT converged after 3 rounds, peak medium
  each round, so the build stops; group 2 stays uncommitted.** Round 1: 3 medium, 4 low, 1 nit;
  round 2: 3 medium, 2 low; both fixed (Decision log). Round 3, open: M1 a session resumed
  before any poll misses it stays `ended` (`model.go` clears it only when the poll had dropped
  it): clear it, as a start, when the PID changes too; M2 `forgetSessions` sets aside an ended
  session, so a resume after failing polls takes its old clock back: set aside only a live one;
  L1 the restored `SubagentStop` ids merged into the session's ring are untested. Each round
  found new session-lifecycle cases, none recurring. Resume with apply-change at group 2's fixes.
- 2026-10-20 group 2 resumed (apply-change, session 6 TRACK D2): the owner's rules (a) and (b)
  added to 2.3 test-first, and a session's stall lifecycle (L1 to L13, Decision log) written as
  one rule and built in one place, with round 3's M1, M2 and L1 as its instances. Review resumes
  at round 4, at most three more rounds. Round 4: peak medium (1, the lifecycle's class), fixed
  through the rule as L13. Round 5: peak medium (2 medium, restart carry and source gating; 5
  low), quick gate PASS on its tree (b73d9a52a); fixed, L3 left with a reason. Round 6: peak
  medium (1 medium, 1 low, both missing tests, no defect in the rule), quick gate PASS on its tree
  (323db2211). **NOT converged after the three further rounds the owner allowed (4, 5, 6, each
  peak medium, never rising), so the build stops again; group 2 stays uncommitted.** Round 6's
  tests are written and pass (Decision log), unreviewed. Resume: one review round of group 2; if it
  finds nothing medium or worse, commit and go on to group 3.
- 2026-10-20 group 2, review round 7: peak high (H1 a hollow test of STALL-1-S18; M1 accepted as
  designed, owned by #185; L1 a missing test of the pane's `Created`); fixed in the tests alone
  (Decision log) after the round's quick gate PASS on its tree (1fbd27c9b). `stall.go` unchanged.
- 2026-10-20 group 2, review round 8 (session 6 TRACK D3): peak medium, quick gate PASS on its
  tree (5b1a5f4c8). Round 7's tests each fail under their mutations. **Not clean, so the build
  stops for the owner (the lane's rule: one round after 7); group 2 stays uncommitted, unfixed.**
  - M1, a defect in the rule, of the restart class: `stall.json` saves no lease state (`leased`,
    `deadSeen`) and the restart's first queue read is a baseline, so a lease given up while the
    panel was down is never movement, and the gate's whole run before the save carries over as
    quiet time. Reproduced: PR #7 open, session idle, gate held alive from 00:05, saved at 00:45
    not stalled; restart at 01:00 with the queue free reads stalled at once, `since` 00:20, a new
    push (without the restart: stalled at 01:20, `since` 00:50). Against D1 (`design.md`
    122-123, 160-163). The reviewer's fixes: save the bound lanes' lease state and compare the
    first queue read against it, or save a lane working by its lease with `Moved` the save time.
    A neighbour that is D1 as written (the owner's): a session busy at the save and idle at the
    first poll stalls at once too (D1's "an `IdleSince` it sets is not movement"), close to #185.
  - L1, wording: `TestStallRestartFirstPollPinsD1`'s name and comment, and round 7's M1 entry
    below, attribute the first tmux poll's baseline to D1; it is this Decision log's reading.
- 2026-10-20 the owner amended D1 and D6 (option B, re-approved at 00d4f27306b2): a stall that
  begins across a restart is approximate until the panel has watched for one stall time
  (STALL-1-S25). Built in group 2 (session 6 TRACK D4) test-first, as one rule; it settles round
  8's M1 and #185, and round 8's L1 is fixed (Decision log). Group 2's review resumes at round 9.
- 2026-10-20 group 2, review round 9: peak medium (2 medium in amendment B's code, 2 low), quick
  gate PASS on its tree (6a4e65de0); M1, M2 and L2 fixed test-first, L1 left with a reason
  (Decision log). Round 10 next, at most rounds 10 and 11 more.
- 2026-10-20 group 2, review round 10: peak medium (1 medium, 3 low), quick gate PASS on its tree
  (fb0d2fbcb). The lows fixed test-first; M1 disputed as D4 as written, for the owner (#186).
  Round 11 next, the last of three.
- 2026-10-20 group 2, review round 11: converged, peak low (3 low), quick gate PASS on its tree
  (d2fe8be05); the three applied as wording only (two comments and the Decision log), with #187
  for the lenient decode.
- 2026-10-20 group 2 (The rule (D1, D3, D4)) built and reviewed: converged in 11 round(s), peak high (round 7; rounds 9 to 11 of TRACK D4: medium, medium, low)
- 2026-10-20 **the build stops before group 3 on the lane's budget** (session 6 TRACK D4): at most
  $180 in total for PANEL-51, $137.05 after group 2's commit (`change-cost.sh`), and group 3
  (four tasks, its integration test among them) estimated at $30 to $45 with one or two review
  rounds, so it would likely pass the ceiling; groups 4 and 5 cannot fit. Resume with
  apply-change at group 3 on this branch, carrying round 9's note for 3.2 (read every worktree's
  receipt, `QuietWorktrees` included) and the OPS-101 path constraint in 3.3 and 4.2. Open with
  the owner: #186 (an interrupted turn, D4; round 10's M1), #187 (a lenient decode), and #185,
  settled by amendment B, to close when PANEL-51 merges.
- 2026-10-20 the owner amended D4 for #186 (option 1, re-approved at 53fc8456ab6b) and raised the
  lane's ceiling to $310 for PANEL-51. Built into group 2 (session 6 TRACK D5) test-first
  (Decision log); one review round over it before its commit, then group 3.
- 2026-10-20 #186 in group 2 built and reviewed: converged in 2 round(s), peak medium (round 1:
  M3, L4 and nit 5 fixed, M1 and M2 disputed for the owner, on #186; round 2: low).
- 2026-10-20 group 3 (builder): 3.1 to 3.4 built and ticked, uncommitted for the group's review;
  `go test -short ./...`, `gofmt` and `go vet` clean but for one test, below. **Stopped for the
  owner on one artifact conflict (design-issue):** config version 7 (D5) makes `panel init` and
  `panel attach` write `"version": 7` (they write `config.LatestVersion`), while the living
  requirement LANECTX-3 (`specs/panel-lane-context/spec.md` lines 105 and 110) says they "declare
  config version 6", so `install/attach_test.go`'s LANECTX-3-S1 check (`raw.Version != 6`) fails.
  Left failing on purpose, unchanged. The likely fix, the owner's: a MODIFIED delta of LANECTX-3
  and S1 in this change ("declare the latest config version") and the check reading
  `config.LatestVersion`. Group 3 also built 4.1's first clause (the notifier keeps `stalled:` marks
  while stalls are pending), which 3.4's restart case tests (Decision log). For group 4: 4.1's
  `Notifier.StallsPending` exists and is set by the notify source; its notifier_test.go cases,
  the Notice's stalled alert and `noticeLine` are still 4.1's.
- 2026-10-20 **the build stops in group 3 for the owner** (session 6 TRACK D5, apply-change: a
  `design-issue` reply is a stop). D5 does not say what `panel init` and `panel attach` write,
  and the choice changes another capability's living requirement: (A) they write the latest
  version, 7, with a MODIFIED delta of LANECTX-3 and its S1 in this change; or (B) they keep
  writing 6, LANECTX-3 standing as written, and a project raises its own version to set
  `stall_minutes`. Group 3 is uncommitted, with no quick gate and no review round run on it; a
  copy is the local ref `refs/clauductor/p51-d5-group3`. $146.63 spent before group 3's builder.
  Resume with apply-change at group 3's review once the owner has chosen.
- 2026-10-20 the owner chose option A (D5, re-approved at 1e365ec5ebf7) and accepted #186's M1
  and M2 as built. Session 6 TRACK D6: the MODIFIED delta of LANECTX-3 and its S1 ("declare the
  latest config version"), copied word for word with S2 unchanged; `TestAttachedLanesLaunchWithThePolicy`
  checks `config.LatestVersion`, shown failing with `init.go` writing a literal 6 ("declares
  version 6 ... want 7"). Group 3's review next.
- 2026-10-20 group 3's review (session 6 TRACK D6, apply-change): **NOT converged after 3 rounds,
  peak medium each round (never rising), so the build stops; group 3 stays uncommitted.** A copy
  is the local ref `refs/clauductor/p51-d6-group3`, with this entry. Round 1, quick gate PASS on its
  tree (bc34abf0c): 1 medium, 8 low; M1 (a stale `stall.json` restored when stalls are turned
  back on) and L1 to L7 fixed (Decision log), L8 (the template's `panel.json` at version 6) left
  for the owner, #189. Round 2, quick gate PASS (5b1c138f4): 1 medium, 3 low; M1 (a save before
  the sources answer drops a restored stalled mark: the restart class) and L2 to L4 fixed. Round
  3, quick gate PASS (686aa91a4): 1 medium, 2 low, open: M1 D10's merged read counts an older
  merge on a reused branch as HEAD's, so D3 (b) never holds there (a missed stall, never a false
  push; round 1 called it D10 as written), the owner's, #188, with L1 (a failed merged read with
  no result retried every poll, lane after lane) and L2 (task 3.4 still cites STALL-1-S13, now
  cited only by group 2's test). $179.72 spent at the stop. Resume: the owner rules on #188 (and
  #189); then one review round of group 3; if it finds nothing medium or worse, commit and go on
  to group 4.
- 2026-10-20 group 3 (builder, session 6 after the owner's #188 and #189): #188, #189 and round
  3's L1 and L2 built test-first (Decision log); 3.1 to 3.4 stay ticked, uncommitted for group 3's
  next review round. `gofmt`, `go vet`, `go test -short ./...` (run with `ANTHROPIC_API_KEY`
  unset: the builder's environment set it, and 14 lane-start tests refuse to start a lane while
  it is set) and both check suites pass. Next:
  one review round of group 3; if nothing medium or worse, commit and go on to group 4.
- 2026-10-20 group 3 review round 1 (after #188 and #189): peak medium, 1 medium 2 low, quick
  gate PASS on tree 8264c8678; fixed test-first (Decision log): M1 (a pull request closed
  unmerged re-pushed its lane's stall; a "none" from before a pull request opened stood after it
  merged) by a "none" counting for nothing once its branch has left the open-PR list and the
  notifier keeping a lane's stalled mark while its merged read is due and unanswered; L1 a test
  that `tasks.md` and the review threads stay page-only with stalls on; L2 `docs/panel.md`'s
  reads table and its two no-page lines. 3.1 to 3.4 stay ticked, uncommitted. Next: the next
  review round of group 3.
- 2026-10-20 group 3 review round 2 (after #188 and #189): peak medium, 1 medium 1 low 1 nit,
  quick gate PASS on tree b7d8ea862; fixed test-first (Decision log): M1 (the open-PR fetch had
  no `--limit`, so gh's 30 rows read as every open pull request) by `--limit 1000`; L1 (the
  merged read stamped with the poll's start, so an update landing meanwhile voided its "none"
  for a wait) by stamping when it answers; the nit by computing `mergedUnanswered` once per
  snapshot. 3.1 to 3.4 stay ticked, uncommitted. Next: the caller's decision on group 3 (the
  last review round allowed).
- 2026-10-20 group 3 review round 3 (after #188 and #189): converged, peak low (3 low, 1 nit),
  quick gate PASS on tree 811c29d83; L3 applied as wording (the `Ticks.StallSave` comment), L1
  to the owner as #190, L2 and the nit left with a reason (Decision log).
- 2026-10-20 group 3 (The config and the reads without a page) built and reviewed: converged in 3 round(s), peak medium (rounds 1 and 2 medium, round 3 low, after #188 and #189; session 6 TRACK D7)
- 2026-10-20 group 4 (builder): 4.1 and 4.2 built test-first and ticked, uncommitted for
  review. The stall's kind of wait is a field of its own (`Wait`, with `OpenWork`) on the
  `stalled` alert; each `Notice` carries its stalled alert as `Stall`; `push.go` reads
  `push.json` (machine source `push`, every 10 s) and `pollNotify` runs it per stalled notice.
  `gofmt`, `go vet`, `go test -short ./...` (with `ANTHROPIC_API_KEY` unset) and both check
  suites pass. Next: group 4's review; then group 5 (the page and the docs).
- 2026-10-21 group 4 (The notification and the push) built and reviewed: converged in 1 round(s), peak low (8 low; quick gate PASS on its tree at a7e8d03 + the group's diff, 2026-10-21T04:22Z; session 6 TRACK D8). L1 left with a reason, L2 to L8 applied in group 5's diff so its review reads them (Decision log).

## Decision log
- 2026-10-18, group 1: read the 2.1.303 binary statically before any live probe, rather than
  starting a live `claude` from the builder (sandbox-denied, and `manual:` on the owner's
  login). The reading is evidence about the code, not a probe: it does not tick 1.1 to 1.4.
  What it shows:
  - P1: `Stop` and `SubagentStop` inputs are built with `background_tasks` (the session's task
    registry) and `session_crons` spread in, the schema unchanged from E2. Not yet shown: that
    they reach the panel through its async command hook.
  - P2: a task is listed while `running` or `pending` and not foreground (`isBackgrounded` not
    false). A subagent task's `id` is its `agentId` (the registry entry is made with the agent
    id), so the pairing D3 (d) needs holds for a subagent in code; a workflow's is not shown.
    The `type` label map is `local_agent` subagent, `local_workflow` workflow, `local_bash`
    shell, `monitor_mcp`/`monitor_ws` monitor, `mcp_task` "MCP task", `in_process_teammate`
    teammate, `remote_agent` "cloud session", plus dream, auto-mode scan and memory import. **A
    background `Monitor` tool is a `local_bash` task of kind `monitor`, so it is listed as type
    `shell`:** D4's allowlist cannot tell it from a shell, and D3 (d)'s "a `monitor` task is
    working" fallback would reach only MCP monitors. If the live P2 finds a running Monitor reads
    `idle`, that is an owner decision (D3 (d), D4).
  - P3: the registry has an `in_process_teammate` type, listed as `teammate` in a lead's
    `background_tasks`, beside a tmux pane mode (`SwarmPaneError`). Teammates can be in-process,
    D9's stop condition; which mode merge-train's team ran in needs the live probe.
  - P4: `ScheduleWakeup` arms a one-shot with cron `` `${d.getMinutes()} ${d.getHours()} * * *` ``
    (local time, day and month `*`) and no `recurring` field, which `session_crons` lists as
    `recurring: false`; the fire time is the next match after the cron's creation
    (`oneShot: true`). It is not five fields with day and month as numbers, so by D4 as written
    it falls to "fields not all numbers: held off while that `Stop`'s list stands". Task 1.4
    sends this to the owner: D4's one-shot rule is amended with them (for example, a one-shot
    whose day and month are `*` fires at the next minute and hour after the `Stop` that listed
    it, in local time; a `ScheduleWakeup` delay is clamped to 60 s to 3600 s).
- 2026-10-18, the owner decided D4's amendment in session 5: "read a one-shot whose day and
  month are `*` as firing at the next matching minute and hour after the `Stop` that listed it,
  in local time". D4 now reads that shape so, resolving the fire time once from the time the
  panel received the `Stop`; the full-number rule stays for a one-shot that carries a day and a
  month. `session_crons` carries no creation time (the binary's builder maps each cron to `id`,
  `schedule`, `recurring` and `prompt` alone), so the next match after the cron's creation was
  not an option. A wakeup due before its turn ended would read as a day away: task 1.4 records
  what one does.
- 2026-10-18, D3 (d)'s Monitor rule, from the 2.1.303 strings
  (`strings -n 8`, the builder's saved copy): the `background_tasks` builder makes each entry
  `{id, type: <label of the registry type>, status, description}` and adds `command` for
  `local_bash`, `agent_type` for `local_agent`, `server`/`tool` for `monitor_mcp` and
  `mcp_task`, `name` for `local_workflow`. The label map is keyed on the registry type alone
  (`local_bash` to `shell`), and the `Monitor` tool registers its task with `kind: "monitor"`
  on a `local_bash` entry, so a Monitor arrives as `type: "shell"` and its kind is not sent.
  Claude Code's own views do tell the two apart, on that `kind` (its "1 shell, 1 monitor"
  summary, "Background monitor" reminders, "Monitor details"), but no hook field carries it.
  So the design reads a Monitor as a shell (open work), drops the old "a `monitor` task is
  working if a Monitor reads idle" fallback, which could reach only MCP monitors, and P2
  decides: a Monitor that leaves its session `busy` changes nothing; one that reads `idle`
  stops the build for the owner. Assumption, unproven until P2: a running Monitor keeps a main
  session `busy` as a shell does (E1). The binary does not settle it: monitors and shells are
  handled apart in print mode's wind-down (`holdForArmedMonitors`, `holdForBackgroundShells`),
  and `claude agents`' busy comes from `isLoading` or `delegatedActive`, so which of a shell or
  a Monitor holds `isLoading` interactively is unknown until P2. For P2: `delegatedActive`
  counts only `local_agent`, `remote_agent`, `in_process_teammate` and `local_workflow` tasks,
  so a running background subagent or workflow should read `busy` through it.
- 2026-10-18, D9 left as it stands: the `in_process_teammate` registry type (P3 above) is its
  stop condition, unconfirmed until the live P3 shows which mode a team runs in. If P3 confirms
  in-process teammates, the build stops for the owner before group 2 commits, as D9 says.
- 2026-10-19, group 1's live probes, on the owner's Mac (Claude Code 2.1.303, local time EDT).
  Each run: an interactive `claude --model haiku` in tmux on a socket of its own (`-L probe51`),
  started with `env -u ANTHROPIC_API_KEY` and `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, its
  `--settings` holding exactly the hooks a throwaway panel built from this branch installed under
  a temporary HOME (plus `TeammateIdle`), with `--setting-sources project,local`. The panel
  carried an uncommitted debug log in `Machine.ingest` decoding D4's allowlist alone (never
  `command` or `prompt`); `claude agents --json` was polled every 3 s. Times are the panel's
  receipt of each hook. Three runs: A 11:24 to 11:42 (from 202acc8; its log was read again for
  this entry, which trusts it over that run's notes); B 16:17 to 16:22 (P1 and a subagent, its
  `/status` not recorded); C 16:33 to 16:48 (from 2bb98a2, `/status`: "Login method: Claude Max
  account"). C repeated the two probes whose stop conditions A met, and the overdue wakeup A did
  not reach.
  - **P1 (1.1): the fields arrive through the async command hook. Ticked.** E1 (A, repeated in
    B): a background `sleep 70` with the turn ended at once. B's `Stop` 16:17:28 listed
    `{id bhw834yhk, type shell, status running}` and `session_crons` `[]`; `claude agents` read
    `busy` from 16:17:25; an `idle_prompt` Notification came 60 s after the `Stop`, still `busy`;
    the shell's end woke the session (`UserPromptSubmit` 16:18:36), and the next `Stop` (16:18:37)
    carried `[]` and `[]`, `idle` by 16:18:39. E3: an AskUserQuestion read `waiting`, `waitingFor`
    "input needed", from 16:19:07 to its answer (`idle` 16:19:35), with `PermissionRequest`
    (`tool_name` AskUserQuestion) 16:19:05 and a `permission_prompt` Notification 6 s later. Both
    fields are on every `Stop` and `SubagentStop` and on no other event. D2 and D4's stop
    condition is not met.
  - **Empty-type `SubagentStop`s are frequent.** One to two seconds after nearly every `Stop`
    comes a `SubagentStop` with an `agent_id` and no `agent_type` (an internal agent; the prompt
    suggestion's, by its timing), carrying the session's `background_tasks`. None ever matched a
    listed task's `id`: D3 (d)'s "an unpaired stop ends nothing" is what holds them off.
  - **P2 (1.2), a background subagent: the pairing holds** (A and B). B: `Stop` 16:19:44 listed
    `{id abc4cb1b3fc456a35, type subagent, status running}`; the `SubagentStop` with that
    `agent_id` (`agent_type` general-purpose) arrived 16:19:48.539, the wake 7 ms later, then a
    `Stop` with `[]`. A: the same for a99b57c1bf8b599da, `busy` on all 19 polls of its run.
  - **P2 (1.2), a background workflow: NO `SubagentStop` carries the workflow task's `id`.
    Task 1.2's stop condition is met, in A and again in C.** C: `Stop` 16:37:07.791 listed
    `{id w0pu2v4gc, type workflow, status running}`. The workflow's one agent sent
    `SubagentStart` 16:37:05.496 and `SubagentStop` 16:37:47.616, both with `agent_id`
    a1a7d243b22c6eeb9 and `agent_type` workflow-subagent; the wake followed (`UserPromptSubmit`
    16:37:50.209, `Stop` 16:37:51.960 with `[]`). `w0pu2v4gc` is in no hook's `agent_id`.
    `claude agents` read `busy` on every poll from 16:37:03 to 16:37:50, `idle` from 16:37:53.
    A: the same, for
    `wvhvmjkd7` and agent a19d6155ed5db2d98. **D3 (d)'s pairing cannot end a workflow task: the
    build stops for the owner before group 2 commits.** What C shows instead: the workflow's end
    wakes its session, so the wake's `Stop` no longer lists it.
  - **P2 (1.2), a background `Monitor`: listed as a shell, keeps the session busy** (A). `Stop`
    11:31:33 listed `{id b8c6mol2r, type shell, status running, description "probe51
    monitor"}`; `claude agents` read `busy` on all 14 polls while it ran; its one output line
    woke the session at 11:32:12. Neither Monitor stop condition is met: the design's reading (a
    Monitor is open work, as a shell) stands.
  - **P3 (1.3), a teammate: in-process, not a session of its own. D9's stop condition is met, in
    A and again in C.** C, default teammate mode inside tmux: the lead spawned `probe-mate`
    in-process (one pane, no second process); `claude agents` listed the lead alone. The
    teammate's hooks arrived under the LEAD's `session_id`, `agent_id`
    aprobe-mate-7532800a6ad753fe, `agent_type` probe-mate (`SubagentStart` 16:38:13.500, its turn's
    end as a `SubagentStop`); `TeammateIdle` fired 16:38:19.846 in the lead's session
    (`teammate_name` probe-mate, `team_name` session-3ec14d00). The lead's `Stop` listed `{id
    t4xa1miqt, type teammate, status running}` and the teammate's background shell `{id
    b5pw25228, type shell, status running}`. The shell ended about 16:38:55 and **woke no one**:
    `claude agents` read `idle` from 16:38:56 and the only hook after was the lead's
    `idle_prompt` (16:39:22). The teammate task still read `running` on the lead's `Stop`s at
    16:48, ten minutes after `TeammateIdle`: "running" on a teammate task does not mean working.
    **The build stops for the owner before group 2 commits (D9).** Tmux pane mode
    (`--teammate-mode tmux`) was not probed.
  - **P4 (1.4), `ScheduleWakeup`: `M H * * *`, local time, rounded up; an overdue one fires as
    its turn ends. Ticked.** A: a /loop with `delaySeconds` 120 listed `{schedule "37 11 * * *",
    recurring false}` on a `Stop` at 11:34:53 (+120 s, rounded up to the next whole minute, local)
    and fired at 11:37:00.313. A's overdue try had its `Stop` (11:41:57) in the fire minute
    itself, which D4 resolves to the same day, so it did not test the risk; C did: armed with
    `delaySeconds` 60 about 16:44:54, listed as `46 16 * * *` (`recurring` false) on the turn's
    `Stop`, which came at 16:48:16.685, in a minute two past the fire time, where D4 resolves
    the H:M to the next day. The wakeup fired 23 ms later (`UserPromptSubmit` 16:48:16.708), and
    the next `Stop` (16:48:19.446) listed no crons, so the day-away reading is replaced within
    seconds. None of 1.4's stop conditions is met.
  - No `Stop` in the three runs (about 150 hooks) listed a task of a type other than `shell`,
    `subagent`, `workflow` or `teammate`.
- 2026-10-19, the owner's answers to the two stop conditions and #167's round-2 finding (by
  chat, session 6): (1) a workflow ends when a later `Stop` drops it (A); (2) a teammate is part
  of its lead's session (A); (3) only `shell`, `monitor` and `teammate` are open work and
  `subagent` and `workflow` working, any other type neither. Written into D3 (d) and D9.
- 2026-10-20, from #167's merge review (rounds 1 and 2 after the re-approval):
  - **An MCP `monitor` task that never ends (M2): accepted, not probed.** The owner chose the
    allowlist with the question in front of them (the owner-queue item named "perhaps ambient
    `monitor_ws`, unprobed"), keeping `monitor` as open work. No probe run saw a `monitor`
    task. If one stays listed with no end, its lane stalls after each turn: a false push, but
    a loud one, since its `waitedOn` names the monitor. `docs/panel.md` says so (task 5.2).
    Reading it as neither is a `design.md` change, for the owner, if a push ever shows it.
  - **Whether a working teammate holds its lead `busy` (L5): unprobed.** P3 read the lead
    `busy` while its teammate's shell ran, after `TeammateIdle`, so with the teammate itself
    idle; and `idle` from 16:38:56 while the teammate task still read `running`, so
    `delegatedActive` does not count a listed teammate task by its status. If a lead reads
    `idle` while its teammate works, a teammate working longer than the stall time with no hook
    pushes a false stall on its lead, naming the teammate.
  - **L2 and L3, `design.md` wording, left as written.** D9's "a teammate's own background
    shell is not seen" (P3 saw it listed on the lead's `Stop`; only its end goes unreported)
    and D3's "after that, quiet, it is open work (d)" for a finished build (true now only past
    4 × the stall time). Editing `design.md` voids the approval, the rules each states are
    unaffected (D3 (d) and task 2.3 hold the workflow rule; task 5.2 words the teammate's shell
    for the docs), and the archive keeps the design as approved.
- 2026-10-20, group 2, what `design.md` leaves open, as built:
  - **2.2 failed first:** with `Command string \`json:"command"\`` added to
    `signals.BackgroundTask`, `TestStopFieldTypesHoldOnlyTheAllowlist` failed ("decodes
    \"command\", outside the allowlist"; "declares \"command\", which D4 never decodes") and
    `TestStopFieldsDecodeToTheAllowlist` failed (the decoded tasks held "hunter2"); removed, both pass.
  - **Absent kept apart from empty** by a nil slice against a non-nil empty one (what
    `encoding/json` gives for a missing field or `null`, and for `[]`), not a pointer to a slice:
    the decode types stay plain, and the test pins both cases.
  - **The stall time** is `state.DefaultStallTime` (30 min) set by `Model.SetStallTime`, until
    3.1 adds `alerts.stall_minutes`; `alerts_test.go`'s all-thresholds-0 case calls
    `SetStallTime(0)` for now. Rather than adding the `Thresholds` field here, which is 3.1's (config
    version 7, the schema and the *Keys* table move with it).
  - **A one-shot with day and month as numbers** resolves to that date in the year nearest the
    `Stop` (the year before, the same or the next), so a wakeup armed on 31 December for 1 January
    is not read as a year back, nor an overdue one as a year ahead. Rather than "the next match",
    which would hold an overdue wakeup off for a year. An impossible date (30 February) falls to
    "neither shape".
  - **The first successful queue poll and tmux poll are baselines,** as the first `claude agents`
    poll is: they record the leases and panes and are not movement. Otherwise a restart's first
    sight of a dead lease holder, or of a dead pane, would restart a lane's clock and notify its
    stall again. STALL-1-S18 is tested with the holder read alive at 00:05 and dead at 00:10.
  - **Live** is listed by `claude agents` (the last listing, kept while the polls fail) and not
    ended. (Superseded by the lifecycle below: live is the session's own lifecycle state, set by
    a listing and cleared by an end, and the hook status is no longer touched.)
  - **The owner's rules (a) and (b), 2026-10-20, each shown failing first on the code as it
    stood** (`go test -run 'TestStallAbsentFieldsKeepTheKeptStop|TestStallAgentStopIsNotTheSessions'`):
    - (a) `TestStallAbsentFieldsKeepTheKeptStop` failed with "00:10: kept {reported:false tasks:[]
      crons:[] last:still building}, want w1 open and a1 ended": a `Stop` with neither field wiped
      the kept workflow, the ended subagent and the cron. Now each field the `Stop` does not carry
      keeps what was kept for it, so an absent `background_tasks` ends no workflow and changes no
      kept task. **Read wider than the owner's words:** an absent `session_crons` keeps the kept
      crons too, since task 2.1 keeps absent apart from empty for both fields; the last line and
      the `Stop`'s time still follow the session's own `Stop`. `reported` stays true once a `Stop`
      of the session has carried a field.
    - (b) `TestStallAgentStopIsNotTheSessions` failed with "00:10: kept {tasks:[] crons:[]
      last:teammate done}, want the lead's own Stop of 00:00 unchanged": a teammate's `Stop` under
      the lead's id replaced the lead's. Now a `Stop` with an `agent_id` is not kept at all; it is
      still movement (D9: a teammate's hooks are the lead's movement), and the rest of
      `ApplyHook`'s `Stop` handling, outside the stall rule, is unchanged. A guard: P3 saw a
      teammate's turn end as a `SubagentStop`, and whether a teammate ever sends a `Stop` is
      unprobed.
  - **Sources:** the receipt read has answered once `ApplyLaneExtras` is called; the merged read
    has answered when every lane needing one (a clean receipt for HEAD, no open pull request) has
    a read for its branch and HEAD. `MergedRead` and `ApplyMergedReads` (by lane path) carry the
    runtime's last result; the runtime keeps it through a failed read (3.2).
  - **A lease binds** to a lane by the id of any running tmux lane in its worktree (not only the
    first, which is `LaneView.Terminal`), or by a basename no other worktree shares.
  - **(c)** holds when every lane the panel launched in the worktree has its pane dead or its tmux
    session gone; its open work reads "claude exited" or "tmux session gone".
  - **Text:** `waitedOn` is the question's if one stalled the lane, else the first (d) session's
    turn, else the lane's latest-stopped live session's (or the exit); a session with no `Stop`
    since the start reads "what it waited on is not known: no turn of it has ended since the panel
    started". (d) adds "turn ended on 1 shell" to `openWork`. A stall is approximate when every way
    that holds is (always so while `claude agents` is not current).
  - **`stall.json`'s shape** (`StallState`) keeps every worktree's lane clock, its default origin
    included, so a lane's quiet time since a start carries across the next restart; and re-saves a
    restored session no poll has listed yet, so a save between the start and the first poll loses
    nothing.
- 2026-10-20, group 2, review round 1:
  - **A restored `Stop` is paired with the merged ring** of `SubagentStop` ids as the poll restores
    it (`pairStops`, shared with `keepStop`), so a subagent whose stop arrived after the restart
    but before the first poll is ended, not held working for 4 × the stall time and then reported
    as never heard. Rather than leaving restored tasks as saved, which the review showed loses that
    stop.
  - **A `shell` or `monitor` task is open work whatever its status**, as `teammate` is; the status
    is shown in the text. Rather than gating it on running or pending as `subagent` and `workflow`
    are: D3 (d) names the three open work with no status condition, and P2 shows Claude Code lists
    a task only while it is in flight, so another status would be a change worth seeing, not
    dropping.
  - **The lease holds (d) off; it does not move the session's clock.** Once the lease is given up,
    a session whose turn ended on background work and has not itself moved for the stall time
    stalls at once, `since` its own last movement (D1: the lease is the lane's movement, and (d) is
    judged on the session's clock alone). Tested so (`TestStallLeaseHoldsOffTurnWork`), rather
    than the review's "stalled at 01:15 since 00:45", which would need the release to move the
    session's clock: a change to D1.
- 2026-10-20, group 2, review round 2:
  - (The ring half of this entry is superseded by the lifecycle's L7: the ids are kept.)
    **A session's kept `Stop` and its `SubagentStop` ids are dropped as it ends** (`SessionEnd`,
    and `session.end`: gone from a successful poll, or its lane retired), not at the start that
    lists it again. Nothing reads them while the session is not live (D4: an ended session wakes
    nothing), so dropping at the end changes nothing there, and a resumed session is then a start
    like a new one (D1), with no pre-exit cron holding its lane off and no dead shell or subagent
    as its work. Rather than clearing at the start, as the review proposed: a resumed process can
    send its own `UserPromptSubmit` and `Stop` before the poll that lists it again, and a clear at
    the start would drop that new `Stop`. The cost: D4's "kept until its next `Stop`" now ends at
    the session's end, and the session's own clock is kept. The bug shows only when the resume
    comes within `forgetSessionAge` (30 min) of the exit, since a session ended longer ago is
    forgotten whole; `TestStallResumeDropsPreExitStop` resumes at 20 minutes for that reason, and
    S24's own test keeps its 03:00 times.
  - **Whether tmux is a source is read from the registry `ApplyTmux` is given, on a failed poll
    too** (`stallModel.launched`), not from `laneRecords`, which a failed poll never sets. A
    failed registry reload passes no records, and leaves the last answer standing.
- 2026-10-20, group 2, **a session's stall lifecycle (PRAC-0010: the class, not the instance).**
  Rounds 1 to 3 each found one more case of one class: how a session's identity and its kept
  state behave across its end, a resume, `forgetSessions`, polls that fail and recover, and a
  restart. Round 3's open three were instances: M1 a session resumed before any poll missed it
  stayed `ended`; M2 an ended session forgotten while the polls failed was set aside and took its
  old clock back; L1 the restored ring's merge was untested. Rather than three more one-offs, the
  state changes only by the transitions below, in one section of `state/stall.go` ("a session's
  lifecycle"), each a named function the model calls once: `stallHook` (L1, from `ApplyHook`),
  `stallListed` (L2 to L8, from `ApplyAgents`), `stallSession.end` (L7, from `SessionEnd` and
  `session.end`), `stallForgotten` (L9, L10, from `forgetSessions`), `stallPolled` (L11),
  `StallState`/`ApplyStallState` (L12). **A process is `claude agents`' `pid` and `startedAt`:**
  `claude --resume` keeps the session id, so the id alone cannot tell a resumed session from
  the process that exited; the process is saved with a session set aside or in `stall.json`.
  States: *heard* (by hooks, never listed), *live*, *ended* (in the model); *set aside*, *saved*
  (out of it, in `stallModel`); *gone* (nothing held).

  | Row | Transition | Clock | Kept `Stop` | `SubagentStop` ids | Ask time | Test |
  |---|---|---|---|---|---|---|
  | L1 | a hook (heard, live, ended stay so; `SessionEnd` is L7) | moved, but by a `Notification` that reports a stop | the session's own `Stop` replaces each field it carries, an absent one keeping what was kept (a); one with an `agent_id` is never kept (b); its own `UserPromptSubmit` marks the turn unreported until its next `Stop` (#186), the mark kept, saved and merged with the `Stop` | a `SubagentStop`'s id joins the ring and ends its task | `blocked`'s | `TestStallIdleLaneWithOpenPR` (the `Notification`), `TestStallAbsentFieldsKeepTheKeptStop`, `TestStallAgentStopIsNotTheSessions`, `TestStallStopBeforeItsStop`, `TestStallUnreportedTurnLeavesWorkUnknown`, `TestStallUnreportedTurnAcrossTheLifecycle` |
  | L2 | heard or gone → live: the first successful poll after the start lists it | not moved: the panel's start, or its hooks' | kept | kept | the poll's | `TestStallLifeFirstPollIsBaseline` |
  | L3 | saved → live: that poll lists it, the same process | restored (carried) | restored, paired with the ids heard since | merged with the ids heard since | restored while still waiting | `TestStallRestartKeepsTurn`, `TestStallRestoredStopPairsEarlyStop`, `TestStallLifeRestoredRingPairs` (round 3's L1) |
  | L4 | heard, ended or gone → live: a later poll lists it | a start: moved | what was heard since its end | kept | the poll's | `TestStallHandStartIsMovement`, `TestStallResumeIsMovement` |
  | L5 | set aside → live: a successful poll lists it, the same process | restored | restored, paired | merged | restored while still waiting | `TestStallAsideTakesItsClockBack` |
  | L6 | live → live: a poll whose reading changed | moved | kept | kept | a new wait sets it | `TestStallLifeReadingChangeMoves` |
  | L7 | live or heard → ended: `SessionEnd`, gone from a successful poll, its lane retired | kept, not read | dropped: what it listed died with the process, and with it any entry set aside or saved for the id | kept: ids are unique, so they pair nothing of a later process | dropped | `TestStallResumeDropsPreExitStop`, `TestStallEndedSessionNotStalled`, `TestStallLifeEndWhileAside` |
  | L8 | ended (L7) then L4: a poll lists another process under the id; the ended process, still listed under its own, stays ended | a start: moved | dropped, but a `Stop` heard since the new process started | kept | the poll's | `TestStallLifeResumeBeforeAPollMisses` (round 3's M1), `TestStallLifeProcessChanged` |
  | L9 | live → set aside: `forgetSessions` while the polls fail | kept | kept | kept | kept | `TestStallAsideTakesItsClockBack` |
  | L10 | heard or ended → gone: `forgetSessions`, the polls failing or not | dropped | dropped | dropped | dropped | `TestStallLifeEndedIsNotSetAside` (round 3's M2) |
  | L11 | saved or set aside → gone: the next successful poll does not list it, or lists another process (then L2 or L4) | dropped | dropped | dropped | dropped | `TestStallLifeUnlistedKeepIsDropped`, `TestStallLifeRestartOtherProcess`, `TestStallLifeAsideOtherProcess` |
  | L12 | live, set aside, saved → saved: a restart through `stall.json` | carried: stalled keeps its time, any other its quiet time | saved | saved | saved | `TestStallRestartCarriesClocks`, `TestStallRestartKeepsQuestion` |
  | L13 | a record heard while its id is set aside or saved: it continues that entry | the newer | the record's if it has one, else the entry's | the entry's, then the record's | the entry's | `TestStallLifeHeardContinuesTheAside`, `TestStallLifeHeardContinuesTheSaved` |

  L13's one merge (`stallKeep.merged`) is what a restore (L3, L5), a save (L12, `continued`) and a
  forget (L10) of such a record all use, so the three cannot keep different things.

  - **Shown failing first** on the code as it stood: `TestStallLifeResumeBeforeAPollMisses`
    ("01:05: the resumed session is not live"), `TestStallLifeEndedIsNotSetAside` ("an ended
    session was set aside", then stalled at 00:40 since 00:05), `TestStallLifeProcessChanged`
    ("the old process's Stop is kept"), `TestStallLifeAsideOtherProcess` and
    `TestStallLifeRestartOtherProcess` (the old process's clock and shell restored). The three
    that passed there each failed under a mutation of the rule they guard: L2 with the first
    poll counted as movement, L3 with the restored ring not merged, L11 with `stallPolled` a
    no-op. `TestStallLifeReadingChangeMoves` (L6) was added after, for a row no test isolated.
  - **What changed from rounds 1 to 3 as built:** live is the lifecycle's own state, so no hook
    can bring an ended session back, and `ApplyAgents` no longer clears an `ended` hook status;
    the ring outlives an end (L7); only a live session is set aside (L9, round 3's M2); another
    process is an end and a start (L8, round 3's M1, and the same for a session set aside or
    saved). `stall.json` gains each session's `pid` and `startedAt`, which D1's list does not
    name: it is what "a session the first poll lists" needs, to mean the same session and not a
    resumed one.
- 2026-10-20, group 2, review round 4 (at the lifecycle above): peak medium, 1 finding, **of the
  lifecycle's class, so fixed through the rule.** M1: a record heard while its id was set aside
  or saved did not continue that entry. Forgotten while the polls still failed, it left the
  older entry to be restored over what it had heard (a shell ended at 00:40 still the lane's
  work at 01:16, `since` 00:00), and a save before the first poll after a restart wrote the
  restored entry rather than the `Stop` heard since. The rule gains L13 (a heard record continues
  the entry, through one merge for restore, save and forget) and L7 now drops the entry as the
  session ends or its lane is retired (`stallDropKept`). `TestStallLifeHeardContinuesTheAside`,
  `TestStallLifeHeardContinuesTheSaved` and `TestStallLifeEndWhileAside` each failed first on the
  code as it stood ("since 20:00, want 00:40"; "saved … Tasks:[{b1 shell running}], want the Stop
  heard at 00:06"; "the entry set aside outlived the session's end").
- 2026-10-20, group 2, review round 5: peak medium (2 medium, 5 low). Neither medium is the
  lifecycle's class; L2 and L3 are. Each failed first on the code as it stood, but L3 and L5:
  - **M1, restart carry: a clock is marked stalled at the save only by the way that rests on
    it.** `StallLaneMark.Stalled` is the lane's own way (D3 (a) to (c)), `StallSessionMark.
    Stalled` the session's (d), and the new `AskStalled` its question; before, a lane stalled
    by its session's question kept its own last movement as an absolute time, so the outage
    counted and a lane answered while the panel was down stalled at once with a new `since`
    (`TestStallRestartCarriesOnlyTheWaysClock`).
  - **M2, source gating: D3 (b) rests on the pull requests too** ("no pull request after it, open
    or merged"), so `needsMerged` waits for `gh pr list` (`TestStallGatePassedWaitsForPRs`).
  - **L1** the stalled label adds no second "≈" to a waiting row that already carries one
    (`TestStallApproxQuestionLabelledOnce`).
  - **L2, the lifecycle: rule (a) on L13's path.** The rule is now one function, `overlay` (a
    `Stop` laid over the one kept, field by field), which both `keepStop` and L13's merge use;
    a kept `Stop` records which fields a `Stop` of the session carried (`hasTasks`, `hasCrons`,
    saved as such in `stall.json`, in place of `reported`) (`TestStallLifeMergeKeepsAbsentFields`).
  - **L3, the lifecycle, left as built:** a `claude agents` entry with neither `pid` nor
    `startedAt` matches any process, so a resume after a heard `SessionEnd` would stay ended. The
    real output carries both (`signals/testdata/agents-real.json`); with neither, no reading can
    tell a resume from the exiting process, and reading every such listing as a start would
    bring back each ended session that lingers in the list.
  - **L4** the merged read counts as answered once it has answered for every lane that needed
    it (`mergedOnce`), so a lane that needs it later is not named in the warning
    (`TestStallMergedWarningOnlyBeforeItAnswers`).
  - **L5** `readiness` now calls `ownBranch` and `openPR` rather than repeating them.
- 2026-10-20, group 2, review round 6: peak medium (1 medium, 1 low), **no defect in the rule**;
  both of the class "a test that would pass without the code it guards". M1: no test applied a
  dead lease holder or a dead pane twice, or a restart whose first queue read or tmux poll already
  sees one, so three mutations (every dead-holder poll moving, every tmux poll moving, either
  first-poll baseline dropped) passed the suite. L1: "a pane coming back is lane movement" was
  tested only beside a session's start. Tests written after the round, **not yet reviewed**:
  `TestStallDeadLeaseHolder` and `TestStallClaudeExited` repeat the dead reading;
  `TestStallRestartFirstReadsAreBaselines` and `TestStallPaneComingBackMoves` are new. Each
  passes on the rule as round 6 reviewed it, and each failed under its mutation (the dead holder
  seen again moving the lane; the first queue read moving it; every tmux poll moving it; a pane
  coming back not moving it). `stall.go` is unchanged since round 6 and the quick gate's pass on
  its tree (323db2211).
- 2026-10-20, group 2, review round 7: peak high (1 high, 1 medium accepted as designed, 1 low).
  `stall.go` is unchanged (sha1 fce4b2a5 before and after the mutations); the fixes are tests,
  through `stall_test.go`'s shared helpers (`gateFree`, and `gateRun(lane, nonce, alive)` behind
  `gateHeld`). Each mutation was made in `stall.go` with its original copied to /tmp, run with
  `go test -short ./internal/panel/state/`, and copied back.
  - **H1, a hollow S18 test.** `TestStallDeadLeaseHolder` read the holder alive at 00:05, so at
    00:10 the release loop moved the lane on its own. Now 00:05 is `gateFree()` (the baseline,
    no lease) and 00:10 the holder's first sight, dead or stale (S18's GIVEN), with the same
    reading repeated at 00:20 and 00:30, for both halves (one table). New case: another gate
    run's dead holder (nonce `run-2` in place of `run-1`) at 00:20 is movement at its first
    sight. Mutations: `if !m.stall.deadSeen[k] {` → `if _ = k; false {` fails all three cases
    ("the holder dead at 00:39: /repo/w/x is stalled", "stalled since Oct 18 00:00, want Oct 18
    00:10", the same for the stale half, and "want Oct 18 00:20" for the second run);
    `dead[p] = p` (the holder's identity collapsed to its lane) fails the second-run case
    ("another run's dead holder at 00:49: /repo/w/x is stalled", "stalled since Oct 18 00:10,
    want Oct 18 00:20").
  - **L1, the pane's `Created`.** `TestStallPaneComingBackMoves` is a table: Resume brings the
    pane back with `Created: 1` (`respawn-pane`, `lanes.go`, keeps session_created), and a tmux
    session killed and recreated between two polls reads alive(1) then alive(2). Mutation
    `panes[rec.ID] = "alive"` fails the recreated case ("its tmux session killed and recreated
    between two polls at 00:49: /repo/w/x is stalled", "stalled since Oct 18 00:00, want Oct 18
    00:20").
  - **M1, accepted as designed, not changed in code; owned by issue #185** (open; settled by
    amendment B, below). A lane stalled at the save keeps its old `since` when its claude was
    resumed while the panel was down: D1 reads the sessions of the first successful `claude
    agents` poll as no movement, and this Decision log (not D1) reads the first tmux poll as a
    baseline (group 2's entry "The first successful queue poll and tmux poll are baselines").
    The fix the reviewer proposed (a process started after the saved state's save is a start at
    its `startedAt`) edits D1, the owner's decision. `TestStallRestartFirstPollPinsD1` (cites no
    scenario; renamed `TestStallRestartResumedWhileDownKeepsSince` and cited to STALL-1-S25 by
    amendment B) pins it: x stalled at 00:45 since 00:00
    on a dead pane with #152 open, a restart at 01:00 through `StallState`/`ApplyStallState`, the
    first tmux poll seeing the pane alive and the first agents poll listing sx under a process
    started 00:55; at 01:00 x is stalled since 00:00. Dropping the first tmux poll's baseline
    (`m.stall.tmuxRead &&` removed) fails it ("01:00, the restart's first polls: /repo/w/x is not
    stalled"), so a change to D1 shows there.
  - The shared `clock` helper now prints UTC: the view's `since` came back from `time.UnixMilli`
    in the machine's zone, so a failure read "since Oct 17 20:00, want Oct 18 00:10" for one
    instant 4 hours off. The times quoted above are as it prints them now.
- 2026-10-20, group 2, **amendment B** (the owner, option B; design re-approved at
  00d4f27306b2, head 913ddb4): a stall that begins across a restart is approximate until the
  panel has watched for one stall time (D1, D6, STALL-1-S25). Built as one rule in one place,
  `judgeStalls`' `hold`: within the stall time of the panel's start, a way that holds is
  approximate unless the clock it rests on was marked stalled at the save and restored
  (`stallModel.carried`, keyed by `wayKey`: a lane's way as `ApplyStallState` restores its mark,
  a session's turn and ask as `restoreKeep` restores them for the same process). No reading is
  added to `stall.json`.
  - **What it settles.** Round 8's M1 (the lease given up while the panel was down) and the
    busy-at-save case (a session busy at the save and idle at the first poll) are its instances:
    each now reads stalled and `≈` at once, and notifies once at the start plus the stall time,
    with the `since` its carried clock gives (`TestStallRestartBeginsApprox`, lanes x and z).
    #185 (a session resumed while the panel was down, its lane stalled at the save) is the
    amendment's other half, a lane stalled at the save keeping its `since` and its mark: the
    round-7 pin test is revised to B as `TestStallRestartResumedWhileDownKeepsSince`, citing
    STALL-1-S25, and now also checks the stall is exact and nothing is sent again. Round 8's L1
    is fixed in round 7's M1 entry above (the first tmux poll's baseline is this Decision log's,
    not D1's) and in the renamed test's comment.
  - **Read so:** "with every source the stall rests on succeeded" is D1's gating as it stands (a
    way is judged only once its sources have succeeded since the start), so the window ends at
    the start plus the stall time, as STALL-1-S25's 01:30 requires with every source answering by
    01:01; rather than timing the window from each source's first answer, which would hold that
    stall approximate to 01:31. A source that first answers late takes its baseline late, which
    D1's 10-minute warning names. A carried mark also needs no clearing on movement: a clock that
    moves after the start cannot reach the stall time inside the window.
  - (Superseded by round 9's M1, below: such a way is saved stalled and approximate.) **A way
    still in its window is not saved as stalled** (`hold` reports it, and `StallState` marks
    only what it reports), so a second restart inside the window does not make it exact; its
    clock carries its quiet time instead, and the next start's window applies again.
  - **The notifier is unchanged:** `Process` keeps an approximate alert's mark and never sends
    it, so a stall marked at the save keeps its mark through the window, and one that begins
    across the restart sends once when its alert turns exact.
  - **Shown failing first:** `TestStallRestartBeginsApprox` on the code before B ("01:01: x's
    stall began across the restart, yet it is not approximate", x's notice sent at 01:01 and
    none at 01:30). Each part then failed under its mutation (`stall.go` copied aside and back,
    sha1 6b47dcc2 restored): the lane's restored mark not recorded ("y, stalled at the save, is
    approximate"; the revised pin test "x, stalled at the save, is approximate"); the turn's
    (`TestStallRestartKeepsTurn`, which now checks `Approx`); the ask's
    (`TestStallRestartKeepsQuestion`, likewise); `hold` reporting every way exact ("saved at 01:29:
    … want only y marked stalled"); the window timed from the first `claude agents` answer (five
    restart tests fail).
- 2026-10-20, group 2, review round 9 (at amendment B as built above; quick gate PASS on its
  tree, 6a4e65de0): peak medium (2 medium, 2 low), both mediums in amendment B's code, neither
  the lifecycle's class. Fixed:
  - **M1, a stall approximate at the save came back exact.** `hold` reported a way exact
    whatever its reading, so a stall held while `claude agents` was not current was saved
    `Stalled`, restored as carried, and sent at the first poll after the start: never exact,
    so the notifier held no mark for it. Now the save keeps two facts apart, per way: *held*
    (`Stalled`, `AskStalled`: the clock keeps its time) and *held only approximately* (new
    `Approx` on the lane's mark, `Approx` and `AskApprox` on a session's: on a reading not
    current, or inside a restart's window); only a way held exact is carried exact. A way held
    approximately keeps its clock's time, so a stall notified before its reading went stale
    keeps its `since`, and its kept mark sends nothing again at the window's end (rather than
    the simpler "not saved as stalled", which would move its `since` and send it a second
    time). These flags are about the stall, not readings: a restart compares nothing against
    them, so amendment B's "no baselines in `stall.json`" stands. A way inside a restart's
    window is now saved stalled and approximate, keeping its `since` through a second restart
    (`TestStallRestartBeginsApprox`'s 01:35 restart).
  - **M2, the row's reason.** A stall approximate only by the restart window read "≈ stale:
    claude agents hasn't answered lately" on a healthy poll. `laneStall.stale` records whether a
    way held on a reading not current; the row says "≈ since the panel's restart: not notified
    until it has watched for the stall time" otherwise.
  - **L2:** task 4.2 cites STALL-1-S25 (its push half).
  - **L1, left as built:** one ill-typed value in `background_tasks` or `session_crons` fails
    the whole hook's decode. E2's schema and every probed body type the fields as `HookEvent`
    does (P1 to P4, about 150 hooks), and a lenient second pass is code with no observed input
    to test it against. (Round 11's L3 corrected the reason first given here, "the hook
    endpoint's parse error is the loud signal": a parse error only counts in
    `Machine.malformed`, Observe's `MalformedDrops`, with no warning bar, so a schema change
    would drop every `Stop` silently. Deferred to issue #187, a lenient decode.)
  - **For group 3 (task 3.2), from the round's notes:** the receipt read covers `v.Lanes` only
    (`readiness_source.go`), so a launched lane whose tmux session is gone, shown among
    `QuietWorktrees`, has no receipt read and D3 (c) with (b) cannot hold for it. 3.2 reads
    every worktree's receipt, as D10 says.
  - **Shown failing first** on round 9's code: `TestStallRestartApproxAtSaveStaysApprox` ("lane,
    never sent, 01:00: approximate at the save, yet exact inside the window", its notice sent at
    01:00; both cases' rows labelled without the restart) and `TestStallRestartBeginsApprox`'s
    new label check ("Stalled (≈ stale: claude agents hasn't answered lately)"). Then each guard
    under its mutation (`stall.go` copied aside and back, sha1 3ca5490c restored): the lane's
    carry ignoring `Approx` (the lane case, and the 01:35 second restart); the turn's ignoring
    `approx` and the ask's ignoring `askApprox` (their cases); the stale reason never given
    (`TestStallApproxWhenNotCurrent`'s new label check).
- 2026-10-20, group 2, review round 10 (quick gate PASS on its tree, fb0d2fbcb): peak medium (1
  medium, 3 low); round 9's fixes held.
  - **M1, disputed: D4 as written, the owner's decision; owned by issue #186** (open). A turn
    that ends with no `Stop` (an Esc interrupt, which Claude Code documents as sending no
    `Stop`, unprobed here; or a `StopFailure`) leaves the turn before's kept `Stop` in force, so
    a shell it listed, ended since, stalls the session 30 minutes later with a false push naming
    it. The code keeps a session's last `Stop` "until its next `Stop`" as D4 says, and D3 (d)
    judges "its own last `Stop`"; making (d) unjudged after a `UserPromptSubmit` with no `Stop`
    since is a reading of D3 (d) that edits what D4 keeps, so it is not made here. #186 gives
    the owner three options (unjudged after an unreported turn end, accept, or probe first).
  - **L1** building the save changed the entries it continued: `merged` laid a record over a
    stored entry's own `Stop` and marked its tasks ended in place (`overlay` returns the base, or
    shares its lists). `merged` now works on a copy (`keptStop.clone`)
    (`TestStallSaveChangesNothing`).
  - **L2** a task id was clipped and the `SubagentStop` id it pairs with was not, so an id past
    120 runes never paired. Both are now bounded by the same clip, the restored ring included
    (`TestStallLongIDsPair`).
  - **L3** an approximate question that only a hook reports (the poll current, reading idle)
    was labelled "≈ stale: claude agents hasn't answered lately". The row now takes its reason
    from the way that made the stall approximate (`laneStall.why`: `blocked`'s own `Why` for a
    question, the stale reading for any way, empty for the restart window alone), through one
    function, `stallLabel` (`TestStallLabelSaysWhy`).
  - **Shown failing first** on round 10's code, with `stallLabel` extracted unchanged first:
    "the restored entry … ended:true … was changed by building the save"; "00:10: the
    subagent's stop arrived, yet its session is still working"; "labelled \"Stalled (≈ stale:
    claude agents hasn't answered lately)\", want the question's own reason".
- 2026-10-20, group 2, review round 11 (quick gate PASS on its tree, d2fe8be05): converged, peak
  low (3 low), round 10's M1 left with #186 and not raised again. Applied as wording, so the code
  committed is the code reviewed: L1 `TestStallRestartBeginsApprox`'s comment (a stall in its
  window is saved stalled and approximate, round 9's M1); L2 `stallListed`'s comment narrowed (a
  session heard only by hooks whose `SessionEnd` came before any poll listed it is live again at
  that listing: one movement, which its `SessionEnd` had already given, leaning to no stall); L3
  round 9's L1 reason corrected above, and the lenient decode deferred to #187.
- 2026-10-20, group 2, **#186 built** (the owner's option 1, D4 amended and re-approved at
  53fc8456ab6b; session 6 TRACK D5), test-first in the one place (d) reads the kept `Stop`
  (`judgeStalls`' turn loop), with the mark a field of the session's lifecycle:
  - **The mark** is `stallSession.unreported`: set by the session's own `UserPromptSubmit`
    (L1), cleared by its own next `Stop` (`keepStop`) alone (an end drops the kept `Stop`, so (d)
    is not judged without one; clearing there too could clear a resumed process's own prompt);
    kept aside, saved (`StallSessionMark.Unreported`) and restored with it (L3, L5, L9, L12), and
    in L13's one merge the record's mark wins when it heard a `Stop` or a prompt. Rather than
    clearing the kept tasks, which would drop the wakeups D4 keeps in force, or a mark on
    `keptStop`, which a record that heard a prompt and no `Stop` (L13) could not carry. **Read
    wider than D4's words:** `stall.json` gains the mark, which D1's list does not name, as it
    gained `pid` and `startedAt`; without it a restart inside an unreported turn would bring the
    false push back.
  - **The lane's text** (D3 (a) to (c)) for a session whose turn went unreported reads "what it
    waited on is not known: its last turn's Stop was not heard (interrupted, failed, or sent
    while the panel was down)", not the kept `Stop`'s work, so no stall names a shell the turn
    before listed.
  - **Left as it was:** a `subagent` or `workflow` task the kept `Stop` lists still holds the
    session working (D1), which leans to no stall; D4's amendment takes only (d) off the kept
    `Stop`.
  - **A `UserPromptSubmit` with an `agent_id` marks nothing**, as rule (b) keeps no such `Stop`:
    a teammate's prompt under its lead's id would otherwise silence the lead's (d). A guard:
    whether a teammate sends one is unprobed.
  - **Shown failing first** (with only `StallSessionMark.Unreported` declared, so the tests
    built): `TestStallUnreportedTurnLeavesWorkUnknown` ("00:35, the turn after the Stop
    unreported: /repo is stalled", "the view names the shell the turn before listed") and
    `TestStallUnreportedTurnAcrossTheLifecycle` (both restart cases stalled at 00:45, the save
    without the mark, the lane's `waitedOn` naming the shell). Then each guard under its mutation
    (`stall.go` copied aside and back, sha1 49bc4c46 restored): `keepStop` not clearing the mark
    ("02:01, the next Stop listing the shell: /repo is not stalled"); L13's merge ignoring a
    prompt with no `Stop` (the after-the-restart case stalled); the save dropping the mark (the
    before-the-save case stalled); the `agent_id` guard removed ("00:35, a teammate's prompt:
    /repo is not stalled").
- 2026-10-20, group 2, #186's review round 1 (quick gate PASS on its tree, 3c6727611): peak
  medium (3 medium, 1 low, 1 nit).
  - **M3, fixed:** L13's merge clearing a set-aside or saved mark when the record heard a `Stop`
    was untested. Two subtests (a `Stop` heard while set aside; one heard after a restart, before
    the first poll), each failing under the reviewer's mutation (`if st.unreported {
    k.unreported = true }`: "01:10, the Stop heard while set aside: /repo is not stalled";
    "00:41, the Stop heard after the restart: /repo is not stalled"), sha1 49bc4c46 restored:
    run before L4's and nit 5's edits below, which touch no line the subtests drive.
  - **L4, fixed:** the text read "ended with no Stop", false when the `Stop` was sent while the
    panel was down after a save that kept the mark; now "its last turn's Stop was not heard".
  - **Nit 5, fixed:** the clear at `end` is gone (above).
  - **M1, disputed, for the owner:** a `workflow` (or `subagent`) task the kept `Stop` lists
    still holds the session working while its turn is unreported, delaying the lane's open-work
    stall up to D3 (d)'s 4 × limit when a workflow's wake turn ends in a `StopFailure`. Kept:
    working is D1's (design lines 89-93), and the amended requirement adds "with no
    `UserPromptSubmit` from it since" to the (d) clause alone, leaving the working clause as it
    was; dropping it would push a false stall on a lane whose workflow still runs after an Esc.
    Noted on #186.
  - **M2, disputed, for the owner:** the hooks are async, so a `UserPromptSubmit` that reached
    the panel after its own turn's `Stop` would leave (d) unknown until the next `Stop`. No
    reading tells that from a real new turn: P4's overdue wakeup sent its `UserPromptSubmit`
    23 ms after a `Stop`, so a time window would misread it. Each hook's curl starts as it fires,
    and a turn holds at least one model call, so the reorder needs one curl delayed past a whole
    turn. Noted on #186.
- 2026-10-20, group 2, #186's review round 2 (quick gate PASS on its tree, c2005e231):
  converged, peak low (1 low, 1 nit); round 1's fixes held and both disputes' reasons stood.
  L1 applied as a test alone, so `stall.go` is the code reviewed (sha1 25313a71): a turn that
  ends in a `StopFailure` keeps (d) unknown, failing under a mutation that clears the mark on
  `StopFailure` ("00:35, after a StopFailure: /repo is stalled"); the nit is round 1's hash note
  above.
- 2026-10-20, group 3 (builder):
  - **3.1, the stall time comes from the config when the model is built**
    (`newStallModel(cfg.AlertThresholds().Stall)`), and `SetStallTime` is gone (its one caller was
    `alerts_test.go`, which now sets `"stall_minutes":0`). Rather than the runtime calling a setter:
    the model's config is never replaced in place (a config change restarts the runtime), so the
    model reads it as it reads every other threshold. `Thresholds` gains `Stall` and
    `StallMinutes`; `docs/panel.md`'s hand-written versions table gains row 7 and "outside 1–7"
    with the generated *Keys* row, since D5 says the version gate follows.
  - **3.1, LANECTX-3 conflicts with version 7** (Progress, the design-issue): left for the owner,
    the failing check unchanged.
  - **3.2, the receipt read walks the model's worktree list** (every worktree with a branch, but
    the main checkout and a bare one), `QuietWorktrees` included (round 9's note); tasks.md and the
    review threads only for the page's lanes, while a page is in view. With no page, a worktree's
    page-only fields are carried from the model's last extras, as they stood when the whole source
    paused with no page; rather than dropping them, which would change what the page shows the
    moment it opens.
  - **3.2, which lanes are due a merged read is the model's rule** (`MergedReadsDue`, sharing
    `needsMerged`'s rule through `mergedDue`), judged with the receipts just read so the receipt and
    its merged read land in one update; rather than the runtime re-deriving "clean receipt, no open
    PR", a second copy of the rule.
  - **3.2, what D10 does not settle about retries:** a lane with no result for its branch and HEAD
    is read at each poll (every 30 s) until one answers, so a first failure leaves (b) unknown for
    one tick, not 5 minutes; a failed read after a result waits `MergedNone` as "none" does, but a
    branch that left the open list stays marked (`again`) until a read answers, so a failed read at
    the merge does not leave the old "none" standing for 5 minutes. A branch name `gh` is not asked
    about is an error, never "none" (`mergedPRRead`). The wait and the save interval are new
    `Ticks` fields (`MergedNone` 5 min, `StallSave` 30 s) so tests shorten them; not the existing
    `Ticks.Merged`, which is the metrics' merged-PR read.
  - **3.3, a save at each tick while the bytes differ, the save time included.** A lane's carried
    quiet time is `savedAt − moved`, so skipping a save when only `savedAt` changed would let a
    crash after a long quiet stretch understate it by that whole stretch: a clock moved forward,
    against D1's "never forward". Rather than comparing without `savedAt` (fewer writes, wrong after
    a crash). The cost is one small atomic write per project every 30 s while it has any worktree.
  - **3.3, nothing is saved before the first worktree list is read** (`StallState` keeps only the
    listed lanes, so a panel stopped in its first second would drop every restored lane clock), and
    neither read nor save runs while `stall_minutes` is 0. The clean stop's save is a goroutine
    counted with the runtime's sources, on the runtime's context, so `halt` and `stopAll` wait for
    it and a runtime started again on the same project reads it.
  - **3.4 needs 4.1's first clause, built here:** `Notifier.StallsPending`, set from the view by the
    notify source before each `Process`, keeps each `stalled:` mark while stalls are pending (D1).
    3.4's restart case cannot pass without it, and "failing first with the pending flag ignored"
    presumes it. A field, not a `Process` parameter, so `machine.go` and the existing tests are
    unchanged. 4.1 keeps its tests in `notifier_test.go`, the Notice's stalled alert and `noticeLine`.
  - **3.4 shown failing first** with the flag ignored (`false &&` in `Process`'s delete, sha1
    2f7e75c7 restored): `TestStallRestartSendsNothingAgain` failed with "notify tick 1 with gh held:
    notifier.json's mark for …/w/a is 0 (held false), want 1792521122194".
    `TestStallRestartKeepsTheQuestionAndTheTurn` passes with the flag ignored, as it should: both of
    its stalls rest on `claude agents` and the kept `Stop` alone, so they are judged before `gh`
    answers and hold their own marks; it fails without the `stall.json` load ("waiting for the
    second panel shows both stalls"). Other mutations, each restored by its sha1: the receipt read
    page-only again (the S5/S19 and restart tests time out unstalled); the open-list trigger removed
    (`TestMergedReadCache`: "the branch left the open list: … PR:0 after 4 calls, want #153 read at
    once"); the attached sidecar at 0.03 (`TestStallTimeIsTheProjects`: "shows a stall").
  - **3.4's STALL-1-S8** runs both projects in one panel, each with an idle lane and an open pull
    request, the stall time 1.8 s (`stall_minutes` 0.03) rather than the scenario's 10 minutes;
    STALL-1-S5 and S19's lanes likewise. The pure rule's timings stay in `state/stall_test.go`.
- 2026-10-20, group 3, review round 1 (the builder's fixes; each new test shown failing on the code
  as it stood, then passing):
  - **M1, a stale `stall.json` when stalls are turned back on: a runtime that starts with stalls
    off removes it** (`newRuntime`, `runtime.go`), rather than keeping saving while they are off.
    Why: D1 keeps `stall.json` for a restart of the stall rule, and D5's 0 turns the rule off; with
    no file, turning stalls on is a start D1 already settles (a lane with no saved movement counts
    from the panel's start, and amendment B holds a stall that begins across it approximate for a
    stall time). Saving while off would run the rule's writer with the rule off, and every clock it
    saved would rest on readings the panel judged under no rule; deleting the state is less
    mechanism than keeping it fresh. Cost: turned on again, a lane quiet all week waits one stall
    time to stall. `TestStallsTurnedOffForgetTheSavedClocks` (`stall_integration_test.go`: stalled
    and notified, a panel with stalls off, a panel with them on, the notifier's interval short as
    days between would make it) failed on the code as it stood: "stall.json kept by a panel with
    stalls off", "lane a is stalled since 1792524669421 (approx false), want a clock from this
    start, after 1792524669421" and, with its interval short, "the stall was sent for since
    1792524847651, want after 1792524847651": the push the finding described.
  - **L1**: LANECTX-3's requirement line now reads "a config that declares a version before 6 SHALL
    NOT use the policy's keys"; nothing else in that delta changed.
  - **L2**: the two restart tests start their first panel with `stopSaveTicks` (`StallSave` an
    hour), and `restartHeld` checks there is no `stall.json` before the stop and one after it. With
    the clean stop's save removed from `Runtime.start` both failed ("no stall.json after a clean
    stop"); restored, both pass.
  - **L3 (fixed)**: `TestStallTimeIsTheProjects` seeds a `stall.json` in the attached project's
    directory and checks it is gone at the end: with the sidecar's 0 read as off, M1 removes it;
    read as unset (30 minutes, stalls on) the runtime keeps and rewrites it. With the sidecar's
    `stall_minutes` taken out it failed ("keeps stall.json … its 0 was not read as off").
  - **L4: with stalls off, no page means no readiness reads, and no lane is ever due a merged
    read.** The receipt read with no page serves the stall rule alone (its only other reader is
    the page's merge-readiness checklist, `state/readiness.go`, which reads it again once a page
    is in view), so `pollReadiness` returns at once with no page and stalls off, as it did before
    PANEL-51; and `MergedReadsDue` lists nothing with the stall time 0, since the merged read's
    one reader is the rule (`ApplyMergedReads` into `m.stall.merged`), page or no page.
    `TestStallsOffReadNothingWithNoPage` failed on the code as it stood ("3 merged reads and 1 git
    dir reads, want none"); `TestStallsOffDueNoMergedRead` (`state/stall_test.go`) failed with the
    `MergedReadsDue` guard disabled ("stalls off: 1 lanes due a merged read, want 0").
  - **L5**: `saveStall` takes `stallMu` before it reads the model, so a tick's save that read
    before the clean stop's cannot write after it. No test: the interleaving needs a hook inside
    `hub.Read`, and the change is the lock's position alone.
  - **L6**: the readiness source's comment in `newRuntime` says what it reads with no page (D10).
  - **L7 (fixed)**: `gitDirOf` stats the worktree first and spawns no `git rev-parse` for a
    directory that is gone, and `pruneGitDirs` drops a worktree no longer listed from the cache
    each poll. `TestGitDirOfSkipsAGoneWorktree` (`readiness_merged_test.go`) failed with the stat
    disabled ("git dir … after 1 spawns, want none") and with the prune disabled ("keeps its cached
    git dir"). A worktree whose directory exists but whose rev-parse fails still spawns at each
    poll: rarer, and caching a failure would need a retry interval of its own.
  - **L8**: not changed, as the fix prompt says (the template's `panel.json`, the owner's).
- 2026-10-20, group 3, review round 2 (the builder's fixes; each new test shown failing on the code
  as it stood, then passing):
  - **M1: a save made before a way's sources have answered writes the mark the restart restored
    for that way, while its clock has not moved** (`Model.StallState`, `state/stall.go`), rather
    than skipping the save while sources are pending. Why: D1 judges each way from its own sources
    and says a repository whose `gh pr list` never succeeds still stalls on a frozen question, so
    holding every save until all sources answer would never save such a project's question and
    turn clocks; and `View.StallsPending` ends after 10 minutes, so it would leave the defect for a
    longer `gh` outage. The model keeps each stalled way's restored mark (`stall.restored`, by
    `wayKey`: a lane's in `ApplyStallState`, a session's turn in `restoreKeep` unless the record
    heard since moved its clock); `StallState` writes it, exact or approximate as restored, for a
    lane while `laneWayJudged` is false (working's sources, the pull requests, the receipts, its
    merged read if it needs one, tmux where launched) and for a turn while `workJudged` is false
    (`claude agents`, the queues; `judgeStalls` now reads the same method). A clock moved since the
    restart ended that stall, so it is saved not stalled. Sessions checked for the same defect:
    before the first poll lists a session it is saved from `stall.saved`, its marks as restored
    (unchanged); its ask rests on `claude agents` alone, judged once a poll lists it; its kept
    `Stop` and ids are saved from the session whatever the sources. Not changed, and not the save
    path's: a session set aside after 30 minutes of failing `claude agents` polls is saved with no
    stalled mark, but the live panel already drops its stall then (group 2's set-aside rule).
    `TestStallSecondRestartBeforeGhAnswersSendsNothingAgain` (`stall_integration_test.go`: a stall
    notified, a restart with `gh` held, that panel stopped with `gh` still held after two tick
    saves, a third panel, the notifier's interval short) failed on the code as it stood: "after the
    second restart lane a is stalled since 1792527374170, want 1792527374064" and "the second
    restart sent the stall again". `TestStallSaveBeforeItsSourcesKeepsRestoredMarks`
    (`state/stall_test.go`) failed: "lane x saved before the pull requests answered:
    {… Stalled:false …}", "session sy saved before the queues answered: stalled false", and the
    third start's stalls "since Oct 18 00:59, want Oct 18 00:00", approximate; with the clock
    check removed its lane w failed ("lane w, moved after the restart: {… Stalled:true …}").
  - **L2**: `stallSaved` and its comparison are gone (the state carries `SavedAt`, so it always
    differed); the comments on the stall source and `saveStall` say each call writes.
  - **L3: a branch outside `config.BranchRe` is due no merged read** (`mergedDue`, so
    `MergedReadsDue`, `needsMerged` and the merged source's warning), rather than a reworded warning.
    Why: `mergedPRRead` refuses it before `gh` runs, so no result can ever come; leaving it due kept
    a source "unanswered" for good. Its receipt stays unknown, never open work (D10).
    `TestStallBranchGhIsNotAskedAboutIsDueNoMergedRead` failed on the code as it stood ("merged
    reads due for [… change/x+y …]" and the warning "the merged read has not answered for x+y").
  - **L4**: `TestStallRestartSendsNothingAgain` no longer cites STALL-1-S13, which
    `TestStallRestartCarriesClocks` covers in full, with `StallState`'s `SavedAt` (it saves through
    the model), rather than adding a timing-bound second lane to the integration test.
- 2026-10-20, group 3, the owner's decisions and review round 3's lows (each new test shown
  failing on the code as it stood, then passing):
  - **#188 (D10 as amended: a merge counts only when its head commit is the lane's HEAD).**
    `mergedPRRead` takes the lane's HEAD, asks `gh pr list --head <branch> --state merged --json
    number,headRefOid --limit 20` and returns the number of the merged pull request whose
    `headRefOid` is HEAD, 0 for none; `readMerged` passes `wt.Head`. Reused, not rewritten: the
    rule and the 20 rows are `lanes/close.go`'s `branchVerdict` (`mergedBranch`'s read), and the
    one `gh` call and its parse are now `mergedPRList`, shared with auto-close's `mergedPR`.
    **Chosen over** changing auto-close too: `mergedPR` keeps its `number --limit 5` read and
    "the newest merge" answer, since D10 rules on D3 (b) alone, and auto-close's Close plan
    already refuses a merge not at the tip (`branchVerdict`). `TestStallReusedBranchCountsNoOlderMerge`
    (`stall_integration_test.go`: the fake `gh` answers `"<number>@<head commit>"` with its
    `headRefOid`; an old merge #77 at the branch's first commit, an empty commit after it, a
    clean receipt for the new HEAD, no PR open) failed on the code as it stood: "timed out after
    10s waiting for lane a, its branch's old merge not HEAD's, stalled on its gate's pass with no
    pull request". `TestStallWithNoPageReadsTheReceiptAndTheMergedRead`'s merge of #153 now
    carries the lane's HEAD; `TestMergedReadCache` asks for the new argv and gains the reused
    branch's older merge (read as "none" for HEAD) and a list holding both. No other test's fake
    answers the readiness read (`autoclose_integration_test.go` answers auto-close's unchanged argv).
  - **#189 (D5 as amended: the template's `panel.json` declares version 7).** Raised
    `template/.clauductor/panel.json` to 7 and regenerated `plugin/scaffold/.clauductor/panel.json`
    (`scripts/build-plugin.sh`). No test or check pinned the preset's version (grepped
    `framework` tests, `.claude/checks`, `template/.claude/checks`; `TestTemplatePanelConfigLoads`
    and `TestScaffoldPanelConfigLoads` only parsed it), so both now require `config.LatestVersion`
    rather than a literal: the next version raise fails them until the preset follows. Shown
    failing at 6: "template/.clauductor/panel.json declares version 6, want the latest, 7: raise
    it and rebuild the plugin (scripts/build-plugin.sh)" and "scaffold panel.json declares
    version 6, want the latest, 7". This repository's own `.clauductor/panel.json` is its own
    config and is not changed.
  - **Round 3, L1: a failed merged read with no result waits `Ticks.MergedNone` (5 minutes), as
    "none" does.** `mergedEntry` records the branch and HEAD the last read was tried for, and a
    lane whose last try was for its branch and HEAD is not read again inside the wait, answered
    or not; a merge found is still kept for good, and a new branch or HEAD is read at once. An
    error still never reads as "none", and with no result (b) stays unknown. **Kept:** with a
    "none" kept from before the branch left the open-PR list, a failed read leaves `again` set,
    so it is tried at each poll until one answers: that "none" may now be a merge, and standing it
    would be a false stall (a false push), which costs more than the reads of the one lane in that
    transition. With no result for the branch and HEAD, a failure clears `again`, so the wait
    applies. `TestMergedReadFailureWaitsLaneAfterLane` (`readiness_merged_test.go`: two lanes,
    every read failing, a poll every 30 s) failed on the code as it stood: "10 polls inside the
    wait, every read failing: map[change/a:10 change/b:10] merged reads, want 1 per lane".
    `TestMergedReadCache`'s first steps now expect no second try inside the wait.
  - **Round 3, L2:** task 3.4 no longer cites [STALL-1-S13]; 2.3 keeps it, and
    `state/stall_test.go` cites it (`scenario-trace.sh --change panel-51-stall-alert --now`:
    CITED there; every group-3 scenario CITED, the MISSING ones groups 4 and 5's).
- 2026-10-20, group 3, review round 1 after #188 and #189 (each new test shown failing on the
  code as it stood, then passing):
  - **M1: two rules, both at the source, both within D1 and D10 as written.** The window is
    the one between `gh pr list` dropping a branch (60 s source) and the readiness source's next
    merged read (30 s): in it the model judged (b) on what it had.
    - **A "none" counts for nothing once its branch has left the open-PR list** (D10: "'None' is
      read again when the branch leaves the open-PR list"; with no result, (b) is unknown).
      `MergedRead` carries `At`, its poll's time; the model records when each branch leaves the
      list (`stallModel.leftOpen`, set by `ApplyPRs` from the last good list and this one,
      dropped when the branch is listed again or no worktree has it), and `mergedKnown` refuses
      a "none" whose `At` is not after it. A merge found still counts for good. A time, not a
      deletion on the transition, because the runtime hands the model its cached "none" again
      at every readiness poll, including one whose view was read before the pull request
      closed. This closes the mirror case (a lane pushed as "gate passed, no PR" for a merged
      pull request).
    - **The notifier keeps a lane's `stalled:` mark while its merged read is due and
      unanswered** (D1: "the notifier keeps the stalls' marks while they are not yet judged";
      D6: once per stretch). `View.StallsUnjudged` (the notifier's, `json:"-"`) lists those lanes
      (`mergedUnanswered`, which the merged source's warning now shares), `Notifier.Process`
      keeps their marks as it keeps every stall's while `StallsPending`, and the runtime calls
      the new `Notifier.ProcessView`, which takes both flags from the view, so no caller sets one
      and forgets the other. The page shows the lane not stalled meanwhile ((b) unknown is not
      open work, D10); a kept mark only stops a second notice for the same `since`, and a lane
      that moved meanwhile stalls with a new one, which is sent.
    - **Chosen over** the reviewer's fix (1), keeping the lane's last verdict in the model: the
      page would show a stall on a (b) D10 calls unknown, and the model would need a memory of
      verdicts, where the notifier already keeps marks for the start's case. **Over** fix (2), a
      merged read while the pull request is open: D10 reads "for each lane with a clean receipt
      for HEAD and no open PR", and a kick from the PR source only narrows the window.
    - **Left, with the reason:** a pull request opened and closed between two readiness polls is
      never seen by the readiness source's own tracking (D10 puts it there), so its "none" is
      read again only after the 5-minute wait. The model, which saw it leave, counts the old
      "none" for nothing and keeps the mark meanwhile: a stall up to 5 minutes late, never a
      false or second push.
    - `TestStallClosedPRUnmergedSendsNothingAgain` (`state/stall_test.go`, the reviewer's
      timeline, the notifier at the config's default interval) failed: "01:40:02, the merged
      read unanswered: the notifier's mark is 0 (held false), want 00:00's" and "01:40:32: the
      stall since 00:00 sent again". `TestStallMergedPRStaleNoneIsNoResult` failed: "Oct 18
      01:20, the merged read due again: /repo/w/x is stalled: {… OpenWork:[gate passed for
      1a2b3c4d5e, no PR] …}" and "a merged pull request's lane pushed". End to end
      (`stall_integration_test.go`, the notify source at its default 2 s tick and the default
      `min_interval_seconds`, the merged read held): `TestStallClosedPRSendsNothingAgain` failed
      "notify tick 1 with the merged read unanswered: notifier.json's mark is 0 (held false),
      want 1792542486804"; `TestStallMergedPRNotPushedOnAnOldNone` failed "notify tick 2, #152
      merged and its merged read unanswered: lane a stalled {… OpenWork:[gate passed for
      996e1e15b2, no PR] …}". The `At` field was added before the run (it changes nothing
      alone). Each rule is load-bearing: with the notifier's keep disabled the two closed-PR
      tests fail again, and with the stale-"none" rule disabled the two merged ones.
  - **L1:** `TestStallNoPageLeavesTasksAndThreadsPageOnly` (a no-page panel, stalls on, a lane
    with #152 open and a `tasks.md`; four readiness polls, then `POST /api/seen`). `tasks.md` is
    a direct file read, so its read is counted where it lands, the lane's readiness `Tasks` row;
    the review threads by the fake `gh`'s `gh api graphql` calls. Passes as the code stands;
    with `visible &&` deleted from `readiness_source.go` it failed: "with no page in view: 1
    review-thread reads (gh api graphql), want 0" and "with no page in view: tasks.md read ("1 of
    2 ticked in changes/a/tasks.md"), want it unread".
  - **L2:** `docs/panel.md`: the reads table gains the receipt read and the merged read, each
    with its cadence and the stalls-on condition; *What it costs* and *Merge readiness* say the
    two run with no page while `alerts.stall_minutes` is above 0, `tasks.md` and the threads
    page-only.
- 2026-10-20, group 3, review round 2 after #188 and #189 (each new test shown failing on the
  code as it stood, then passing):
  - **M1: the open-PR fetch asks for `--limit 1000`** (`runtime.go`, the const `openPRLimit`).
    gh lists 30 rows unless asked for more, and the stall rule reads the list as every open pull
    request (D3 (a) and (b), `notePRsLeft`, the merged read's due list and `readMerged`'s own
    open tracking). 1000 is past any real project's open count. **Chosen over** treating a list
    at exactly the limit as cut short (an error to `ApplyPRs`, so the `prs` source is
    unanswered and its ways unjudged, as D1 says): a few lines, but it would also turn the
    page's pull-request list into an error for such a project, a change outside the stall rule;
    a project with 1000 open pull requests is out of reach and stated so in the const's comment.
    No other `gh pr list` is read as complete: the merged reads (`autoclose.go`'s
    `mergedPRList`, `lanes/close.go`) ask per branch with their own `--limit`, and the metrics'
    merged list (`signals/merged.go`) already has `MergedLimit`. The fake `gh` in
    `stall_integration_test.go` now lists as gh does (newest first, `--limit` rows, 30 unless
    given); `run_test.go`'s exact-match fake and `testdata/browser/run.sh` (run by hand, not
    by `go test`) take the new argument list. `TestStallSeesAnOpenPRPastGhsDefaultLimit` (31
    open pull requests, lane a's #1 the oldest) failed: "timed out after 10s waiting for lane a
    stalled on #1 open, the oldest of 31 open pull requests".
  - **L1: the merged read's `At` is the runtime clock after the read answers**
    (`readiness_source.go`), not the poll's `now`, taken before the hub's view; the wait
    (`e.at`) keeps the poll's `now`. The hub and the runtime share one clock (`live.go`), so
    `At` and `leftOpen` are on the same clock. `TestMergedReadStampsWhenItAnswered`
    (`readiness_merged_test.go`, a fake clock the read advances by 2 s) failed: "a "none"
    answered at t0+2s, its branch leaving the open list at t0+1s: At 2026-10-20 00:00:00
    +0000 UTC, want after 2026-10-20 00:00:01 +0000 UTC". `TestMergedReadCache` gains a fake
    clock (a successful read now reads it).
  - **Nit:** `applyStalls` computes `mergedUnanswered` once and hands it to
    `stallSourcesMissing`, so the warning and `StallsUnjudged` read the same lanes. No new
    test: a refactor with no behaviour to fail on; the existing stall tests pass.
- 2026-10-20, group 3, review round 3 (converged, peak low), what was done with each:
  - **L1, a design gap, to the owner as #190:** a clean receipt for a HEAD already on `main` (a
    worktree reset to `origin/main` and gated) is open work by D3 (b), since no merged pull
    request names that HEAD, so such a lane can be pushed with nothing to merge. Telling it
    apart is a rule D3 (b) and D10 do not state; left open when PANEL-51 merges.
  - **L2, left with a reason:** the open-PR fetch's `--limit 1000` asks for each row's checks, so
    a repository with hundreds of open pull requests could pass the fetch's 30 s timeout, where
    gh's 30 rows used to answer. Kept: 30 rows silently gave the stall rule a false "gate
    passed, no PR" past the 30th pull request (round 2's M1), a failed fetch is a warning bar
    rather than a wrong answer, and no project the panel serves has near that many open.
  - **L3, applied as wording:** `Ticks.StallSave`'s comment (`run.go`) said `stall.json` is
    saved only while it changes; each save writes, since the state carries its save time.
  - **Nit, left:** `Notifier.StallsPending` and `StallsUnjudged` stay set after `ProcessView`
    sets them. A project's runtime calls `ProcessView` alone, which sets both at every tick; the
    machine's notifier (`machine.go`) calls `Process` on an instance of its own whose flags
    nothing sets, and a test sets what it needs.
- 2026-10-20, group 4 (builder), what D6 and D7 do not settle:
  - **The kind of wait is a field of its own on the stall** (`laneStall.wait`, judged with
    `waitedOn` in `judgeStalls`), carried on the `stalled` alert as `Wait` beside `OpenWork`,
    both `json:"-"`: the page reads the lane's `stall`, so `/api/state` is unchanged. Its words
    are fixed: "a question" for any ask (D7's word; a permission prompt included), "nothing in
    flight", "a shell in flight" (by task type and count: "2 shells in flight"), "a subagent
    that ended and woke nothing", "a subagent whose end was not heard", "claude exited" or "tmux
    session gone", and "what it waited on is not known" or "... is not reported" where D4's text
    says so. `taskNote` gains `how` for it. **Chosen over** parsing `waitedOn` (it holds the
    description) and over adding it to `StallView` (no reader). `OpenWork` holds no model words
    ("#152 open", "gate passed for <short sha>, no PR", "turn ended on 1 shell", "claude exited").
  - **The push's body** (`state.PushText`): "stalled, quiet for 30m · a shell in flight · #152
    open, turn ended on 1 shell", clipped to 240 runes, the title to 80 as the macOS
    notification's. The title is built from the alert's `Name` and the notifier's project (the
    config's `name` only while trusted, as `pollNotify` titles the notice).
  - **The macOS line** (`noticeLine`): "stalled, quiet for 30m: <what it waited on>; open work:
    …". It stays on the Mac, so it keeps D4's words, as a waiting alert's line does.
  - **A push failure counts in `NotifyFailed` and `NotifyError`; a push that succeeds does not
    count in `NotifySent`,** which stays the count of notifications. A push runs only where the
    macOS notification would (`alerts.notify` off sends neither), after it, whatever became of it.
  - **Refusal details D7 leaves open:** an unknown key beside `argv` is allowed (a note to self
    in the file); a file that is not regular (a FIFO would block the read) is refused; through a
    symbolic link, both the link's folder and the file's own are checked; the owner is compared
    with `os.Getuid()`, handed to `readPush` so a test stands in for another user's file. The
    warning reads "Push command <path> refused, so stalls are not pushed: <why>." under the key
    `push_refused`; it is set on each project's model by its `pollNotify` when the refusal
    changes.
  - **The push source's signature adds the file's and its folder's mode and owner to
    `pathSignature`:** a `chmod` changes neither size nor mtime, so a file made safe (or unsafe)
    would otherwise not be read again until its next edit. Every 10 s (`Ticks.Push`), a stat.
  - **`runPush` sets `WaitDelay` 1 s:** a program killed at the timeout can leave a child
    holding its output open, and `CombinedOutput` would wait for it. The timeout is a parameter
    (`pushNotice` passes the 10 s const) so a test runs it at 200 ms with no shared variable for
    parallel panels to race on. The program inherits the panel's environment, as any command
    the owner configures; its error carries what it printed, never its argv (it can hold a
    token).
  - **The machine's own notifier never pushes:** `pollQuotaAlert` sends notices only, and no
    machine alert is a stall. No test of its own; the code has no path there.
  - **Each test of a control, failing first** (the control removed, then restored):
    - 4.1, the stalled marks kept while pending (`n.StallsPending ||` removed):
      `TestNotifierKeepsStalledMarksWhilePending`: "pending tick 0: the stall's mark is 0 (held
      false), want 1790604000000". The notice's stall (not set): `TestNotifierCarriesTheStallApart`:
      "the notice's stall: <nil>, want the stalled alert". Focus (`if focused[g]` off):
      `TestNotifierFocusedStallIsSeen`: "focused: [{... Keys:[stalled:/repo/w/a] ...}], want no
      notice". Approximate (`if a.Approx` off): `TestNotifierApproxStallNeverInterrupts`: "an
      approximate stall at +30m0s: [...], want no notice".
    - The push built from the alert's `Text` (appended in `PushText`):
      `TestStalledAlertCarriesThePushFields`: "a shell: the push ... carries "Run checks"", and
      the same for "merge-train", "Review group 2" and the question's "Bash".
    - The body from `Notice.Body` (in `pushNotice`): `TestPushRidesOnTheStallAlone`: "the push's
      body "waiting on you for 30m: wants to use Bash; stalled, …": want the stall's own fields,
      not the waiting alert's text"; `TestStallPushEndToEnd`: "the push's body ... carries "Run
      checks"".
    - The refusal rules (each check made false): `TestReadPushRefusesByD7`: "writable by group:
      argv ["/usr/bin/true"], want none to run", likewise writable by others, in a folder
      writable by group, not the user's, a relative program (`["ntfy" "publish" "topic"]`), two
      objects; "not a JSON object: refused "... its "argv" is not a non-empty list of strings.",
      want it to name ... and "not one JSON object""; `TestStallPushRefusedEndToEnd`: "a
      push.json writable by group ran: [["Stalls · a" "stalled, quiet for 1s · …"]]".
    - No push while approximate (`if a.Approx` off): `TestStallAcrossRestartPushesOnceWatched`:
      "an approximate stall: 1 notifications and 1 pushes, want none".
    - The timeout (`WithCancel` for `WithTimeout`): `TestRunPushFailsAndTimesOut`: "a push that
      never answers took 5.086698041s, want it stopped at its timeout".
    - A failure counted (the count skipped): `TestStallPushFailureCounted`: "timed out after 10s
      waiting for lane a stalled, notified and its push failed".
- 2026-10-21, group 4, review round 1 (converged, peak low), what was done with each:
  - **L1, left with a reason:** the push runs synchronously in the project's notify source, so
    several stalls at once against a hung endpoint delay the later notices by up to about 11 s
    each. Kept: the macOS send it follows is synchronous too, the notices are one lane's each
    and rare, and a goroutine per push would let a push outlive the notice that built it and
    race the next tick's for the same lane.
  - **L2 to L8, applied in group 5's diff** (so group 5's review reads them): L2 a task's `how`
    follows its status, not only its type; L3 an exited claude named once in the push body; L4
    `push.json`'s re-read signature covers its symlink target's folder; L5 tests for a
    `push.json` symlinked into a group-writable folder and for a non-regular file; L6 a
    restart test with `push.json` present asserts no push (STALL-3-S7); L7 a failed push's
    output clipped to its last line in the log; L8 `TestStallAcrossRestartPushesOnceWatched`'s
    window scaled by `testwait`.

## 1. Probes on the owner's Mac (design E1 to E3, P1 to P4)
- [x] 1.1 P1: on the Mac's Claude Code version, with the panel running, repeat E1 and E3 in a throwaway lane: a background `sleep` with the turn ended at once, then an AskUserQuestion. Record what `claude agents --json` reads at each step, and whether the `Stop` body the panel's hook endpoint receives carries `background_tasks` and `session_crons` (a debug log of the decoded fields, never committed). Record the result in this change's Decision log (`changes/panel-51-stall-alert/tasks.md`), as every probe of this group does. If the fields do not arrive through the async command hook, stop for the owner: D2 and D4 rest on them. (manual: a live Claude Code session on the owner's login, which no test can start)
- [x] 1.2 P2 (done 2026-10-19: the workflow's stop condition was met, and the owner amended D3 (d) the same day, option A: a workflow ends when a later `Stop` drops it): in the same lane, a turn that ends while a background subagent runs (an Agent call with `run_in_background`). Record what `claude agents` reads until the subagent ends, what the `Stop` lists (each task's `id` and `status`, `pending` included), and every `SubagentStop` the session sends until then, with its `agent_id` and `agent_type`, the empty-type ones included; then the same for a background workflow, and, for a background `Monitor`, what its `Stop` lists (its `type`, expected `shell`) and what `claude agents` reads while it runs. If a background subagent's or workflow's end sends no `SubagentStop` whose `agent_id` is the listed task's `id`, stop for the owner before group 2 commits: D3 (d)'s pairing rests on it. If a session reads `idle` while its `Monitor` runs, or the Monitor is listed as anything but a `shell`, stop for the owner before group 2 commits (D3 (d)). (manual: a live session)
- [x] 1.3 P3 (done 2026-10-19: the stop condition was met, and the owner amended D9 the same day, option A: a teammate is part of its lead's session): a teammate (an agent team in the lane, one teammate told to start a background `sleep` and end its turn). Record whether it is a session of its own in `claude agents` and in the hooks, and whether `TeammateIdle` fires and in whose session. If it is not a session of its own, stop for the owner before group 2 commits (D9). (manual: a live agent team)
- [x] 1.4 P4: a one-shot `ScheduleWakeup` in a /loop. Record the `schedule` text its `Stop` lists, and whether the fire time is local time, in the Decision log (`changes/panel-51-stall-alert/tasks.md`); then one armed with a 60 s delay in a turn that runs on past it, and whether it fires as that turn ends. If that overdue wakeup does not fire within a minute of its turn ending, stop for the owner before group 2 commits: D4 would read it as a day away, a silent hold of up to about 24 hours. If its `schedule` is neither `M H * * *` nor five fields with the minute, hour, day and month as numbers, or its fire time is not local, stop for the owner before group 2 commits: D4's one-shot rule (amended with the owner on 2026-10-18 for `M H * * *`) rests on it. (manual: a live session)

## 2. The rule (D1, D3, D4)
- [x] 2.1 `framework/internal/panel/signals/hooks.go`: `HookEvent` declares `background_tasks` and `session_crons`, each into a declared type that holds D4's allowlist alone (`id`, `type`, `status`, `description`; `schedule`, `recurring`), absent kept apart from empty. Tested in `framework/internal/panel/signals/hooks_stop_test.go` over E1's two `Stop` bodies, an older body with neither field, and a body whose task carries `command`, `server`, `tool`, `name` and `agent_type`. Citing [STALL-2-S5].
- [x] 2.2 `framework/internal/panel/notranscript_test.go`: its decoded-types check also walks the new types by reflection and fails on any field D4 forbids. Show it failing first on an added `command` field, and record that in the Decision log. Citing [STALL-2-S5].
- [x] 2.3 `framework/internal/panel/state/stall.go`: per lane and per session, from what the model holds:
  - working (D1): a session while busy, or while a `subagent` task (running or pending) its own last `Stop` listed has not been marked ended by the `SubagentStop` whose `agent_id` is the task's `id` (marked on the task when the stop arrives, or when the `Stop` is kept if the stop is among the last 64 the session sent before it), or while its own last `Stop` lists a `workflow` task (running or pending; a later `Stop` that drops it ends it, and its agents' `workflow-subagent` stops end nothing: D3 (d) as amended 2026-10-19, [STALL-2-S13]), and in either case the session has moved within 4 × the stall time (past that, open work "whose end was never heard"; an unpaired stop, such as an empty-type one, ends nothing: D3 (d)); a session's question is never held off by its working; the lane while any live session is, or while it holds or waits for a lease bound to it by its tmux id or a worktree basename unique in the project, a holder counting only while alive and not stale (first seen dead or stale: the lease given up, which is movement), no lease read where the project declares no queue;
  - three clocks (D1): a question's, from the ask alone (`blocked`'s `Since`), which no hook resets; each session's own last movement (its hooks but a `Notification` that reports a stop, its reading changes, its start: a listing of a session not live just before, a resumed id included, but not one dropped by `forgetSessions` while the polls failed, which is kept aside and takes its clock back); and the lane's (any session's movement, a bound lease taken or given up, the pane dying or coming back); the sessions of the first successful poll not counting; each origin the saved movement carried over, else the panel's start;
  - each way of stalling judged only from the sources it rests on, once each has succeeded since start (D1's list of the project's sources; (d) from the session's kept `Stop`, received or restored); the view's "stalls pending" flag until every one of those sources has succeeded once or 10 minutes have passed; and a warning naming a source not succeeded by then and what it leaves unjudged (D1);
  - a waiting session's stall the stall time after its ask, whatever its lane's other sessions, its lease or its own background agents do, its ask time the saved one when the first poll after start lists it still waiting (D1);
  - a session's background-work stall (D3 (d)) on its own clock, held off only by its own working and its own wakeups and by the lane's lease, never by a sibling's (D1, D4);
  - the live-session rule (D1): a lane with no live session stalls only on D3 (c);
  - the lane's open work (D3 (a) to (c)), held off by any live session's wakeup still due (D4);
  - a wakeup's due time (D4): a recurring cron, or a one-shot of neither shape, while its `Stop`'s list stands; a one-shot with minute, hour, day and month as numbers, 5 minutes past that local time; a one-shot `M H * * *`, 5 minutes past the first local H:M at or after the minute the panel received its `Stop`, that day or the next, resolved once as the `Stop` is kept and kept with the cron;
  - an allowlist of task types (D3 (d) as amended 2026-10-19, [STALL-2-S15]): `shell` (a background `Monitor` among them), `monitor` and `teammate` are open work, `subagent` and `workflow` working, and a task of any other type (`dream`, `auto-mode scan`, `memory import`, "MCP task" and "cloud session", the labels the hook sends for the registry's `mcp_task` and `remote_agent`, or one not seen) is neither; a `teammate` task is never working, whatever its status, and a teammate's hooks, which arrive under its lead's `session_id`, are the lead's movement (D3 (d), D9 as amended 2026-10-19, [STALL-2-S14]);
  - the session keeps its last `Stop`'s tasks (with their ids), crons (with each one-shot's resolved fire time) and last line, clipped, until its next `Stop`, and the `agent_id`s of its last 64 `SubagentStop`s, restored at start for a session the first poll lists, and nothing else from a `SubagentStop` (D4);
  - the owner's two rules of 2026-10-20 (Decision log): (a) a `Stop` whose `background_tasks` field is absent (an older Claude Code) ends no workflow and changes no kept task, absent not being empty (2.1), and likewise an absent `session_crons` changes no kept cron; (b) a `Stop` that carries an `agent_id` is never the session's own `Stop`: it is never kept, so it ends no workflow, replaces no kept task or cron and changes no last line, and is still the session's movement (a teammate's hooks arrive under its lead's `session_id`, D9; a guard, since whether a teammate sends a `Stop` is unprobed);
  - a turn that ends unreported (D4 as amended 2026-10-20, #186 option 1, [STALL-2-S16]): once a session sends `UserPromptSubmit` with no `Stop` since, its D3 (d) is unknown, not open work, until its next `Stop`, and the lane's text names none of the kept `Stop`'s work; its other ways are still judged, the kept `Stop` still holds its wakeups and is still saved, with the mark; a `UserPromptSubmit` carrying an `agent_id` (a teammate's, under its lead's id: D9) marks nothing, as rule (b) keeps no such `Stop`;
  - a session's stall state (its clock, kept `Stop`, `SubagentStop` ids and ask time) changes only by the lifecycle's transitions (the Decision log's table, L1 to L13), across its end, a resume (a process told apart by `claude agents`' `pid` and `startedAt`), `forgetSessions`, failing and recovering polls, and a restart;
  - a stall that begins across a restart (D1 as amended 2026-10-20, option B, [STALL-1-S25]): within the stall time of the panel's start, a way that holds is approximate (`≈`, never notified) unless `stall.json` marked the clock it rests on stalled at the save and the restart restored it; in one place, where `approx` is set (`judgeStalls`' `hold`), with the restored marks recorded as `ApplyStallState` and `restoreKeep` restore them; a way held only approximately (on a reading not current, or inside that window) is saved stalled and approximate, keeping its clock's time, and only a way held exact is carried exact; the row names the restart as why it is approximate, unless a reading is not current; the notifier is unchanged (an approximate alert keeps a mark it holds and never sends), so the stall notifies once when the window ends;
  - `LaneView.Stall` (`since`, `waitedOn`, `openWork`, `approx`: D11) and the `stalled` alert (key `stalled:<lane path>`, severity `block`, `Since` the earliest origin among the ways that hold: D1, D6), every stall approximate while the `claude agents` source is not current;
  - the receipt and HEAD it reads are D10's (HEAD from the worktree list), the merged read's result and the saved state (D1's `stall.json`) arrive through `Apply` calls the runtime makes (group 3), and the model hands back the state to save.
  - Tested in `framework/internal/panel/state/stall_test.go` with a fixed clock, citing [STALL-1-S1], [STALL-1-S2], [STALL-1-S3], [STALL-1-S4], [STALL-1-S5], [STALL-1-S6], [STALL-1-S7], [STALL-1-S9], [STALL-1-S10], [STALL-1-S11], [STALL-1-S12], [STALL-1-S13], [STALL-1-S14], [STALL-1-S15], [STALL-1-S16], [STALL-1-S17], [STALL-1-S18], [STALL-1-S19], [STALL-1-S20], [STALL-1-S21], [STALL-1-S22], [STALL-1-S23], [STALL-1-S24], [STALL-1-S25], [STALL-2-S1], [STALL-2-S2], [STALL-2-S3], [STALL-2-S4], [STALL-2-S6], [STALL-2-S7], [STALL-2-S8], [STALL-2-S9], [STALL-2-S10], [STALL-2-S11], [STALL-2-S12], [STALL-2-S13], [STALL-2-S14], [STALL-2-S15] and [STALL-2-S16].
- [x] 2.4 `framework/internal/panel/state`: the Needs-you item of kind `stalled`, built once per lane after the session loop, as `mergeNeeds` (`state/lifecycle.go`) builds the `merged` rows; a waiting lane that stalls keeps its one waiting item, labelled stalled, with the quiet time and open work (D11). Tested in `state/stall_test.go`, citing [STALL-4-S2] and [STALL-4-S3].

## 3. The config and the reads without a page (D1, D5, D10)
- [x] 3.1 `framework/internal/panel/config`: `alerts.stall_minutes` (default 30, 0 off) at config version 7, in `config.go`'s `AlertConfig` and `Thresholds` and in `fields.go`'s table; regenerate `docs/panel.schema.json` and the *Keys* table in `docs/panel.md` as `config/generated_test.go` requires. Tested in `framework/internal/panel/config/config_test.go` (the default, 0, a negative refused) and `config/version_test.go` (the key refused at version 6). Citing [STALL-1-S8]. `panel init` and `panel attach` write the latest version (D5, the owner's option A of 2026-10-20): the MODIFIED delta of LANECTX-3 in `changes/panel-51-stall-alert/specs/panel-lane-context/spec.md`, and `framework/internal/panel/install/attach_test.go` checks `config.LatestVersion`, citing [LANECTX-3-S1].
- [x] 3.2 `framework/internal/panel/readiness_source.go`: the receipt read runs whether or not a page is in view; `tasks.md` and the review threads stay page-only (D10). The merged read runs `mergedPR`'s `gh pr list --head <branch> --state merged` (`autoclose.go`) through a variant that returns its error, for each lane with a clean receipt for its HEAD and no open PR, asking for `number,headRefOid` and counting a merge only when its `headRefOid` is the lane's HEAD, as `lanes/close.go`'s `mergedBranch` does (D10, the owner's #188): only a merge found is cached, per branch and HEAD; "none" is read again when the branch leaves the open-PR list, which this source tracks itself from the view's `PRs`, and otherwise at most every 5 minutes; a failed read keeps the last result, and with none the receipt is unknown, never "none", and the read waits the same 5 minutes (D10; review round 3, L1).
- [x] 3.3 `framework/internal/panel/runtime.go`: save D1's state to `~/.clauductor/panel/<project hash>/stall.json` (the time of the save; each lane's and each live session's last movement and whether it was stalled; each waiting session's ask time; each live session's kept `Stop` allowlist, each task's ended mark, each one-shot's resolved fire time, and the `agent_id`s of its last 64 `SubagentStop`s; the same for a session kept aside after failing polls; 0600, written atomically as `notifier.json` is, at most once every 30 s while it has changed and once at a clean stop) and hand it to the model at start (D1). Build the path as `notifier.json`'s is, `filepath.Join(config.ProjectDir(home, root), "stall.json")`: no literal `.clauductor` path, and no new helper in `config/paths.go`; `framework/internal/brand` holds the name (the owner's architect's constraint, in OPS-101's row in `docs/roadmap.md`). Add its read site to `fileReadSites` in `framework/internal/panel/notranscript_test.go`, with a comment on what it reads.
- [x] 3.4 Tested in `framework/internal/panel/stall_integration_test.go`, a runtime with no page in view, a fake `gh` and a receipt written for a lane's HEAD:
  - with the merged read's 5-minute wait shortened, `/api/state` shows the lane stalled while the fake `gh` lists no pull request for its branch, open or merged; still stalled when the next merged read fails; and not stalled once the fake `gh` has listed a pull request open for the branch and then lists it merged instead;
  - a second lane whose first merged read fails is not stalled until a read succeeds;
  - a branch reused after a squash merge, whose old merged pull request's `headRefOid` is an older commit, with a new HEAD, a clean receipt for it and no pull request open, is stalled (#188);
  - an attached project whose sidecar sets `stall_minutes` 0 shows no stall;
  - a second runtime started on the same home after the first stops, with a fake `gh` that answers only after the notifier's first ticks, shows the same stall with the same `since`, its notifier sends nothing, and `notifier.json` keeps the stall's mark throughout. Show this case failing first with the pending flag ignored, and record that in the Decision log;
  - the same restart with a fake `claude agents` that lists a session still waiting on a question, and with a session whose saved `Stop` listed a running subagent whose paired `SubagentStop` arrived before the restart, and which now reads `idle`: both lanes show their stalls with the `since` they had, and nothing is sent.
  - Citing [STALL-1-S5], [STALL-1-S8], [STALL-1-S19], [STALL-1-S20], [STALL-1-S21] and [STALL-3-S7].

## 4. The notification and the push (D6, D7)
- [x] 4.1 `framework/internal/panel/state/notifier.go`: a `stalled` alert interrupts by the notifier's rules unchanged, but for one: while the view says stalls are pending (D1), `Process` keeps each `stalled:` mark it holds rather than deleting it as cleared. Each `Notice` carries its `stalled` alert apart, if it has one. `noticeLine` words a stall's line for the macOS notification. Tested in `framework/internal/panel/state/notifier_test.go`, citing [STALL-3-S2], [STALL-3-S3] and [STALL-3-S7].
- [x] 4.2 `framework/internal/panel/push.go`:
  - read `~/.clauductor/panel/push.json` (as the next bullet says when), its path built as `filepath.Join(config.PanelDir(home), "push.json")`, with no literal `.clauductor` path and no new helper in `config/paths.go` (OPS-101's row, as in 3.3), and refuse it by D7's rules (the file's owner and mode, its folder's mode, an absolute `argv[0]`, one JSON object) with a warning bar naming the file and why;
  - for each notice that carries a stall, run its argv with a title and body built from the `stalled` alert's own fields (D7), never from `Notice.Body`, with no shell and a 10 s timeout; a failure counts in the notifier's failures with its error;
  - the machine (`machine.go`) reads the file once for every project, at start and from a machine source that checks its `pathSignature` every 10 s; each project's `pollNotify` (`runtime.go`) takes the machine's current command or refusal at each tick, runs the command, and adds a refusal to its view's `Warnings`; the machine's own notifier (the quota, the login locks) never pushes;
  - its read site goes in `fileReadSites` (`notranscript_test.go`), with a comment on what it reads.
  - Tested in `framework/internal/panel/push_test.go` with a stand-in program written by `internal/testbin` that records its arguments, a notice carrying a stall and a `waiting` alert among them; and a stall that begins across a restart pushes nothing while it is approximate and once when the panel has watched for the stall time. Citing [STALL-1-S25], [STALL-3-S1], [STALL-3-S4], [STALL-3-S5] and [STALL-3-S6].

## 5. The page and the docs (D11)
- [ ] 5.1 `framework/internal/panel/web/static/stall-view.js` (new), loaded in `web/index.html` before `panel.js` as `context-colour.js` is: from a lane's `stall` and its Needs-you item, the word "stalled", the row's tint class, and the Needs-you row's text (quiet time, open work, what it waited on), with Open terminal only where the lane has a terminal, and a waiting row labelled stalled. `panel.js` calls it from the lane's state cell, tab, tree node and Needs you; nothing else in `panel.js` changes (D11).
  - Tested in `framework/internal/panel/web/stall_view_test.go`, which loads `stall-view.js` under node through `vm`, as `web/context_colour_test.go` loads `context-colour.js`, over lanes from a `/api/state` fixture: a stalled lane with a terminal, one started by hand, and a waiting lane that stalled. Citing [STALL-4-S1].
  - And in `framework/internal/panel/testdata/browser/stalled-lane.cjs`, which `testdata/browser/run.sh` runs against a panel of its own, started after the shared one with its own HOME, project and `panel.json` at version 7 with `stall_minutes` 0.05 (3 s), so no other script sees a stalled lane. Its fake `claude agents` lists one `idle` session whose `cwd` is lane "second"'s worktree, and its fake `gh` lists pull request #7 open for that lane's branch, so the lane stalls by D1 as it stands, not by a weakened rule. Its `SHOTS` screenshots go to the group's reviewer (OPS-84 is not merged yet). Citing [STALL-4-S1].
- [ ] 5.2 `docs/panel.md`: the `stalled` row in *Alerts*; the stall rule, what it reads and what it does not see (a teammate's own background shell, whose end nothing reports: D9; and an MCP `monitor` task, open work by the allowlist, which stalls its lane if it never ends: unprobed, the Decision log of 2026-10-20; and a turn that ends with no `Stop`, interrupted with Esc or failed, whose background work is unknown until the session's next `Stop`: #186) in a new *Stalled lanes* section under *Alerts*; `push.json` in *Notifications* and in *Security model*, with an ntfy and a Pushover example; `stall.json` beside `notifier.json`; the two decoded `Stop` fields in *How signals are read*; "stalled" in *The page*'s lane rows and Needs you.

- [ ] Slice: an owner away from the Mac gets one push on their phone when a lane on the panel at http://127.0.0.1:<port> has stalled with work open, and sees the lane marked stalled, with what it last waited on, when they open the page
