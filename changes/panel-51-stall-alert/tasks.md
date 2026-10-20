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
  | L1 | a hook (heard, live, ended stay so; `SessionEnd` is L7) | moved, but by a `Notification` that reports a stop | the session's own `Stop` replaces each field it carries, an absent one keeping what was kept (a); one with an `agent_id` is never kept (b) | a `SubagentStop`'s id joins the ring and ends its task | `blocked`'s | `TestStallIdleLaneWithOpenPR` (the `Notification`), `TestStallAbsentFieldsKeepTheKeptStop`, `TestStallAgentStopIsNotTheSessions`, `TestStallStopBeforeItsStop` |
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
  - a session's stall state (its clock, kept `Stop`, `SubagentStop` ids and ask time) changes only by the lifecycle's transitions (the Decision log's table, L1 to L13), across its end, a resume (a process told apart by `claude agents`' `pid` and `startedAt`), `forgetSessions`, failing and recovering polls, and a restart;
  - a stall that begins across a restart (D1 as amended 2026-10-20, option B, [STALL-1-S25]): within the stall time of the panel's start, a way that holds is approximate (`≈`, never notified) unless `stall.json` marked the clock it rests on stalled at the save and the restart restored it; in one place, where `approx` is set (`judgeStalls`' `hold`), with the restored marks recorded as `ApplyStallState` and `restoreKeep` restore them; a way held only approximately (on a reading not current, or inside that window) is saved stalled and approximate, keeping its clock's time, and only a way held exact is carried exact; the row names the restart as why it is approximate, unless a reading is not current; the notifier is unchanged (an approximate alert keeps a mark it holds and never sends), so the stall notifies once when the window ends;
  - `LaneView.Stall` (`since`, `waitedOn`, `openWork`, `approx`: D11) and the `stalled` alert (key `stalled:<lane path>`, severity `block`, `Since` the earliest origin among the ways that hold: D1, D6), every stall approximate while the `claude agents` source is not current;
  - the receipt and HEAD it reads are D10's (HEAD from the worktree list), the merged read's result and the saved state (D1's `stall.json`) arrive through `Apply` calls the runtime makes (group 3), and the model hands back the state to save.
  - Tested in `framework/internal/panel/state/stall_test.go` with a fixed clock, citing [STALL-1-S1], [STALL-1-S2], [STALL-1-S3], [STALL-1-S4], [STALL-1-S5], [STALL-1-S6], [STALL-1-S7], [STALL-1-S9], [STALL-1-S10], [STALL-1-S11], [STALL-1-S12], [STALL-1-S13], [STALL-1-S14], [STALL-1-S15], [STALL-1-S16], [STALL-1-S17], [STALL-1-S18], [STALL-1-S19], [STALL-1-S20], [STALL-1-S21], [STALL-1-S22], [STALL-1-S23], [STALL-1-S24], [STALL-1-S25], [STALL-2-S1], [STALL-2-S2], [STALL-2-S3], [STALL-2-S4], [STALL-2-S6], [STALL-2-S7], [STALL-2-S8], [STALL-2-S9], [STALL-2-S10], [STALL-2-S11], [STALL-2-S12], [STALL-2-S13], [STALL-2-S14] and [STALL-2-S15].
- [x] 2.4 `framework/internal/panel/state`: the Needs-you item of kind `stalled`, built once per lane after the session loop, as `mergeNeeds` (`state/lifecycle.go`) builds the `merged` rows; a waiting lane that stalls keeps its one waiting item, labelled stalled, with the quiet time and open work (D11). Tested in `state/stall_test.go`, citing [STALL-4-S2] and [STALL-4-S3].

## 3. The config and the reads without a page (D1, D5, D10)
- [ ] 3.1 `framework/internal/panel/config`: `alerts.stall_minutes` (default 30, 0 off) at config version 7, in `config.go`'s `AlertConfig` and `Thresholds` and in `fields.go`'s table; regenerate `docs/panel.schema.json` and the *Keys* table in `docs/panel.md` as `config/generated_test.go` requires. Tested in `framework/internal/panel/config/config_test.go` (the default, 0, a negative refused) and `config/version_test.go` (the key refused at version 6). Citing [STALL-1-S8].
- [ ] 3.2 `framework/internal/panel/readiness_source.go`: the receipt read runs whether or not a page is in view; `tasks.md` and the review threads stay page-only (D10). The merged read runs `mergedPR`'s `gh pr list --head <branch> --state merged` (`autoclose.go`) through a variant that returns its error, for each lane with a clean receipt for its HEAD and no open PR: only a merge found is cached, per branch and HEAD; "none" is read again when the branch leaves the open-PR list, which this source tracks itself from the view's `PRs`, and otherwise at most every 5 minutes; a failed read keeps the last result, and with none the receipt is unknown, never "none" (D10).
- [ ] 3.3 `framework/internal/panel/runtime.go`: save D1's state to `~/.clauductor/panel/<project hash>/stall.json` (the time of the save; each lane's and each live session's last movement and whether it was stalled; each waiting session's ask time; each live session's kept `Stop` allowlist, each task's ended mark, each one-shot's resolved fire time, and the `agent_id`s of its last 64 `SubagentStop`s; the same for a session kept aside after failing polls; 0600, written atomically as `notifier.json` is, at most once every 30 s while it has changed and once at a clean stop) and hand it to the model at start (D1). Build the path as `notifier.json`'s is, `filepath.Join(config.ProjectDir(home, root), "stall.json")`: no literal `.clauductor` path, and no new helper in `config/paths.go`; `framework/internal/brand` holds the name (the owner's architect's constraint, in OPS-101's row in `docs/roadmap.md`). Add its read site to `fileReadSites` in `framework/internal/panel/notranscript_test.go`, with a comment on what it reads.
- [ ] 3.4 Tested in `framework/internal/panel/stall_integration_test.go`, a runtime with no page in view, a fake `gh` and a receipt written for a lane's HEAD:
  - with the merged read's 5-minute wait shortened, `/api/state` shows the lane stalled while the fake `gh` lists no pull request for its branch, open or merged; still stalled when the next merged read fails; and not stalled once the fake `gh` has listed a pull request open for the branch and then lists it merged instead;
  - a second lane whose first merged read fails is not stalled until a read succeeds;
  - an attached project whose sidecar sets `stall_minutes` 0 shows no stall;
  - a second runtime started on the same home after the first stops, with a fake `gh` that answers only after the notifier's first ticks, shows the same stall with the same `since`, its notifier sends nothing, and `notifier.json` keeps the stall's mark throughout. Show this case failing first with the pending flag ignored, and record that in the Decision log;
  - the same restart with a fake `claude agents` that lists a session still waiting on a question, and with a session whose saved `Stop` listed a running subagent whose paired `SubagentStop` arrived before the restart, and which now reads `idle`: both lanes show their stalls with the `since` they had, and nothing is sent.
  - Citing [STALL-1-S5], [STALL-1-S8], [STALL-1-S13], [STALL-1-S19], [STALL-1-S20], [STALL-1-S21] and [STALL-3-S7].

## 4. The notification and the push (D6, D7)
- [ ] 4.1 `framework/internal/panel/state/notifier.go`: a `stalled` alert interrupts by the notifier's rules unchanged, but for one: while the view says stalls are pending (D1), `Process` keeps each `stalled:` mark it holds rather than deleting it as cleared. Each `Notice` carries its `stalled` alert apart, if it has one. `noticeLine` words a stall's line for the macOS notification. Tested in `framework/internal/panel/state/notifier_test.go`, citing [STALL-3-S2], [STALL-3-S3] and [STALL-3-S7].
- [ ] 4.2 `framework/internal/panel/push.go`:
  - read `~/.clauductor/panel/push.json` (as the next bullet says when), its path built as `filepath.Join(config.PanelDir(home), "push.json")`, with no literal `.clauductor` path and no new helper in `config/paths.go` (OPS-101's row, as in 3.3), and refuse it by D7's rules (the file's owner and mode, its folder's mode, an absolute `argv[0]`, one JSON object) with a warning bar naming the file and why;
  - for each notice that carries a stall, run its argv with a title and body built from the `stalled` alert's own fields (D7), never from `Notice.Body`, with no shell and a 10 s timeout; a failure counts in the notifier's failures with its error;
  - the machine (`machine.go`) reads the file once for every project, at start and from a machine source that checks its `pathSignature` every 10 s; each project's `pollNotify` (`runtime.go`) takes the machine's current command or refusal at each tick, runs the command, and adds a refusal to its view's `Warnings`; the machine's own notifier (the quota, the login locks) never pushes;
  - its read site goes in `fileReadSites` (`notranscript_test.go`), with a comment on what it reads.
  - Tested in `framework/internal/panel/push_test.go` with a stand-in program written by `internal/testbin` that records its arguments, a notice carrying a stall and a `waiting` alert among them; and a stall that begins across a restart pushes nothing while it is approximate and once when the panel has watched for the stall time. Citing [STALL-1-S25], [STALL-3-S1], [STALL-3-S4], [STALL-3-S5] and [STALL-3-S6].

## 5. The page and the docs (D11)
- [ ] 5.1 `framework/internal/panel/web/static/stall-view.js` (new), loaded in `web/index.html` before `panel.js` as `context-colour.js` is: from a lane's `stall` and its Needs-you item, the word "stalled", the row's tint class, and the Needs-you row's text (quiet time, open work, what it waited on), with Open terminal only where the lane has a terminal, and a waiting row labelled stalled. `panel.js` calls it from the lane's state cell, tab, tree node and Needs you; nothing else in `panel.js` changes (D11).
  - Tested in `framework/internal/panel/web/stall_view_test.go`, which loads `stall-view.js` under node through `vm`, as `web/context_colour_test.go` loads `context-colour.js`, over lanes from a `/api/state` fixture: a stalled lane with a terminal, one started by hand, and a waiting lane that stalled. Citing [STALL-4-S1].
  - And in `framework/internal/panel/testdata/browser/stalled-lane.cjs`, which `testdata/browser/run.sh` runs against a panel of its own, started after the shared one with its own HOME, project and `panel.json` at version 7 with `stall_minutes` 0.05 (3 s), so no other script sees a stalled lane. Its fake `claude agents` lists one `idle` session whose `cwd` is lane "second"'s worktree, and its fake `gh` lists pull request #7 open for that lane's branch, so the lane stalls by D1 as it stands, not by a weakened rule. Its `SHOTS` screenshots go to the group's reviewer (OPS-84 is not merged yet). Citing [STALL-4-S1].
- [ ] 5.2 `docs/panel.md`: the `stalled` row in *Alerts*; the stall rule, what it reads and what it does not see (a teammate's own background shell, whose end nothing reports: D9; and an MCP `monitor` task, open work by the allowlist, which stalls its lane if it never ends: unprobed, the Decision log of 2026-10-20) in a new *Stalled lanes* section under *Alerts*; `push.json` in *Notifications* and in *Security model*, with an ntfy and a Pushover example; `stall.json` beside `notifier.json`; the two decoded `Stop` fields in *How signals are read*; "stalled" in *The page*'s lane rows and Needs you.

- [ ] Slice: an owner away from the Mac gets one push on their phone when a lane on the panel at http://127.0.0.1:<port> has stalled with work open, and sees the lane marked stalled, with what it last waited on, when they open the page
