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

## 1. Probes on the owner's Mac (design E1 to E3, P1 to P4)
- [x] 1.1 P1: on the Mac's Claude Code version, with the panel running, repeat E1 and E3 in a throwaway lane: a background `sleep` with the turn ended at once, then an AskUserQuestion. Record what `claude agents --json` reads at each step, and whether the `Stop` body the panel's hook endpoint receives carries `background_tasks` and `session_crons` (a debug log of the decoded fields, never committed). Record the result in this change's Decision log (`changes/panel-51-stall-alert/tasks.md`), as every probe of this group does. If the fields do not arrive through the async command hook, stop for the owner: D2 and D4 rest on them. (manual: a live Claude Code session on the owner's login, which no test can start)
- [x] 1.2 P2 (done 2026-10-19: the workflow's stop condition was met, and the owner amended D3 (d) the same day, option A: a workflow ends when a later `Stop` drops it): in the same lane, a turn that ends while a background subagent runs (an Agent call with `run_in_background`). Record what `claude agents` reads until the subagent ends, what the `Stop` lists (each task's `id` and `status`, `pending` included), and every `SubagentStop` the session sends until then, with its `agent_id` and `agent_type`, the empty-type ones included; then the same for a background workflow, and, for a background `Monitor`, what its `Stop` lists (its `type`, expected `shell`) and what `claude agents` reads while it runs. If a background subagent's or workflow's end sends no `SubagentStop` whose `agent_id` is the listed task's `id`, stop for the owner before group 2 commits: D3 (d)'s pairing rests on it. If a session reads `idle` while its `Monitor` runs, or the Monitor is listed as anything but a `shell`, stop for the owner before group 2 commits (D3 (d)). (manual: a live session)
- [x] 1.3 P3 (done 2026-10-19: the stop condition was met, and the owner amended D9 the same day, option A: a teammate is part of its lead's session): a teammate (an agent team in the lane, one teammate told to start a background `sleep` and end its turn). Record whether it is a session of its own in `claude agents` and in the hooks, and whether `TeammateIdle` fires and in whose session. If it is not a session of its own, stop for the owner before group 2 commits (D9). (manual: a live agent team)
- [x] 1.4 P4: a one-shot `ScheduleWakeup` in a /loop. Record the `schedule` text its `Stop` lists, and whether the fire time is local time, in the Decision log (`changes/panel-51-stall-alert/tasks.md`); then one armed with a 60 s delay in a turn that runs on past it, and whether it fires as that turn ends. If that overdue wakeup does not fire within a minute of its turn ending, stop for the owner before group 2 commits: D4 would read it as a day away, a silent hold of up to about 24 hours. If its `schedule` is neither `M H * * *` nor five fields with the minute, hour, day and month as numbers, or its fire time is not local, stop for the owner before group 2 commits: D4's one-shot rule (amended with the owner on 2026-10-18 for `M H * * *`) rests on it. (manual: a live session)

## 2. The rule (D1, D3, D4)
- [ ] 2.1 `framework/internal/panel/signals/hooks.go`: `HookEvent` declares `background_tasks` and `session_crons`, each into a declared type that holds D4's allowlist alone (`id`, `type`, `status`, `description`; `schedule`, `recurring`), absent kept apart from empty. Tested in `framework/internal/panel/signals/hooks_stop_test.go` over E1's two `Stop` bodies, an older body with neither field, and a body whose task carries `command`, `server`, `tool`, `name` and `agent_type`. Citing [STALL-2-S5].
- [ ] 2.2 `framework/internal/panel/notranscript_test.go`: its decoded-types check also walks the new types by reflection and fails on any field D4 forbids. Show it failing first on an added `command` field, and record that in the Decision log. Citing [STALL-2-S5].
- [ ] 2.3 `framework/internal/panel/state/stall.go`: per lane and per session, from what the model holds:
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
  - `LaneView.Stall` (`since`, `waitedOn`, `openWork`, `approx`: D11) and the `stalled` alert (key `stalled:<lane path>`, severity `block`, `Since` the earliest origin among the ways that hold: D1, D6), every stall approximate while the `claude agents` source is not current;
  - the receipt and HEAD it reads are D10's (HEAD from the worktree list), the merged read's result and the saved state (D1's `stall.json`) arrive through `Apply` calls the runtime makes (group 3), and the model hands back the state to save.
  - Tested in `framework/internal/panel/state/stall_test.go` with a fixed clock, citing [STALL-1-S1], [STALL-1-S2], [STALL-1-S3], [STALL-1-S4], [STALL-1-S5], [STALL-1-S6], [STALL-1-S7], [STALL-1-S9], [STALL-1-S10], [STALL-1-S11], [STALL-1-S12], [STALL-1-S13], [STALL-1-S14], [STALL-1-S15], [STALL-1-S16], [STALL-1-S17], [STALL-1-S18], [STALL-1-S19], [STALL-1-S20], [STALL-1-S21], [STALL-1-S22], [STALL-1-S23], [STALL-1-S24], [STALL-2-S1], [STALL-2-S2], [STALL-2-S3], [STALL-2-S4], [STALL-2-S6], [STALL-2-S7], [STALL-2-S8], [STALL-2-S9], [STALL-2-S10], [STALL-2-S11], [STALL-2-S12], [STALL-2-S13], [STALL-2-S14] and [STALL-2-S15].
- [ ] 2.4 `framework/internal/panel/state`: the Needs-you item of kind `stalled`, built once per lane after the session loop, as `mergeNeeds` (`state/lifecycle.go`) builds the `merged` rows; a waiting lane that stalls keeps its one waiting item, labelled stalled, with the quiet time and open work (D11). Tested in `state/stall_test.go`, citing [STALL-4-S2] and [STALL-4-S3].

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
  - Tested in `framework/internal/panel/push_test.go` with a stand-in program written by `internal/testbin` that records its arguments, a notice carrying a stall and a `waiting` alert among them. Citing [STALL-3-S1], [STALL-3-S4], [STALL-3-S5] and [STALL-3-S6].

## 5. The page and the docs (D11)
- [ ] 5.1 `framework/internal/panel/web/static/stall-view.js` (new), loaded in `web/index.html` before `panel.js` as `context-colour.js` is: from a lane's `stall` and its Needs-you item, the word "stalled", the row's tint class, and the Needs-you row's text (quiet time, open work, what it waited on), with Open terminal only where the lane has a terminal, and a waiting row labelled stalled. `panel.js` calls it from the lane's state cell, tab, tree node and Needs you; nothing else in `panel.js` changes (D11).
  - Tested in `framework/internal/panel/web/stall_view_test.go`, which loads `stall-view.js` under node through `vm`, as `web/context_colour_test.go` loads `context-colour.js`, over lanes from a `/api/state` fixture: a stalled lane with a terminal, one started by hand, and a waiting lane that stalled. Citing [STALL-4-S1].
  - And in `framework/internal/panel/testdata/browser/stalled-lane.cjs`, which `testdata/browser/run.sh` runs against a panel of its own, started after the shared one with its own HOME, project and `panel.json` at version 7 with `stall_minutes` 0.05 (3 s), so no other script sees a stalled lane. Its fake `claude agents` lists one `idle` session whose `cwd` is lane "second"'s worktree, and its fake `gh` lists pull request #7 open for that lane's branch, so the lane stalls by D1 as it stands, not by a weakened rule. Its `SHOTS` screenshots go to the group's reviewer (OPS-84 is not merged yet). Citing [STALL-4-S1].
- [ ] 5.2 `docs/panel.md`: the `stalled` row in *Alerts*; the stall rule, what it reads and what it does not see (a teammate's own background shell, whose end nothing reports: D9; and an MCP `monitor` task, open work by the allowlist, which stalls its lane if it never ends: unprobed, the Decision log of 2026-10-20) in a new *Stalled lanes* section under *Alerts*; `push.json` in *Notifications* and in *Security model*, with an ntfy and a Pushover example; `stall.json` beside `notifier.json`; the two decoded `Stop` fields in *How signals are read*; "stalled" in *The page*'s lane rows and Needs you.

- [ ] Slice: an owner away from the Mac gets one push on their phone when a lane on the panel at http://127.0.0.1:<port> has stalled with work open, and sees the lane marked stalled, with what it last waited on, when they open the page
