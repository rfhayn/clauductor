# panel-stall Specification

## Purpose
The panel tells the owner when work has stopped moving: a lane that waits on the owner, or has open work, and has gone a set time with no movement and nothing working, is marked stalled on the page, says what it last waited on, and interrupts the owner once, on the Mac and through a push command of the owner's. It judges every session on the machine the same way, whether or not the panel launched it.

## ADDED Requirements
### Requirement: A lane is stalled when a session of it has waited on the owner for the stall time since it asked; or when a session of it ended its turn on background work, or the lane has open work, and that session or lane is not working and has not moved for the stall time
THE SYSTEM SHALL mark a lane stalled WHEN a session of it waits on the owner and its ask is at least the project's `alerts.stall_minutes` (default 30, 0 off) old, timed from the ask alone (the `Since` Needs you shows for it), no hook resetting it, whatever its other sessions, its lane's lease and its own background work do; or WHEN a live session of it is not working, its own last `Stop` listed background work of a type that counts (a `shell`, `monitor` or `teammate` task is open work, a `subagent` or `workflow` task is working, and a task of any other type is neither), the lane neither holds nor waits for a queue's lease, the session has itself not moved for the stall time, and it has no wakeup of its own still due, whatever its other sessions do; or WHEN no live session of it is working, it neither holds nor waits for a queue's lease (a holder counting only while alive and not stale), the time since the lane's last movement is at least the stall time, and either it has a live session, open work (an open pull request for its branch, or a clean gate receipt for its HEAD with no pull request open or merged for its branch) and no live session with a wakeup still due, or the panel launched it and its claude exited or its tmux session is gone while it has an open pull request or such a receipt; a lane with no live session SHALL NOT otherwise be stalled. A session SHALL be working while it is busy on a current `claude agents` reading, or while its own last `Stop` listed a `subagent` task, running or pending, whose `id` no `SubagentStop` from it has carried as its `agent_id`, before or after that `Stop` arrived, or a `workflow` task, running or pending, that no later `Stop` from it has dropped, and the session has moved within 4 × the stall time; past that limit such a task SHALL be open work; a `SubagentStop` that pairs with no listed task SHALL end none. A session's own movement SHALL be a hook from it other than a `Notification` that reports a stop, a change in its `claude agents` reading, or its start: a listing, after the panel's first successful `claude agents` poll, of a session that was not live just before (a new id, or one that had ended or was absent from a successful poll, a session resumed under the same id included), but not of one dropped while the polls failed, which takes back its clock; the lane's movement SHALL be any of its sessions' movement, the lane taking or giving up a lease bound to it (its holder first seen dead or stale counting as giving it up), or its pane dying or coming back; movement SHALL clear the stall it resets, and the sessions that first poll lists SHALL NOT count as movement. Each lane's and each live session's last movement, each waiting session's ask time, and each live session's kept `Stop` fields and the `agent_id`s of its last 64 `SubagentStop`s SHALL be kept across a restart, for the sessions the first poll after it lists; a lane or session not stalled at the last save SHALL carry over the quiet time it had then and SHALL NOT count the time the panel was down; and a lane or session with no time known SHALL count from the panel's start. Each way of stalling SHALL be judged only once the sources it rests on have succeeded since the start, a source not succeeded within 10 minutes SHALL be named in a warning, and every stall SHALL be approximate while the `claude agents` source is not current.

#### Scenario: [STALL-1-S1] An idle lane with an open pull request stalls at the stall time
- **GIVEN** a project with `alerts.stall_minutes` 30, and a lane on branch `change/x` whose session `claude agents` reads `idle`, with pull request #152 open for `change/x`
- **AND** the lane's last movement was its `Stop` hook at 00:00
- **WHEN** the panel derives its view at 00:29 and again at 00:30
- **THEN** at 00:29 the lane is not stalled
- **AND** at 00:30 the lane's `stall` has `since` 00:00 and `openWork` naming "#152 open", and a `stalled` alert of severity `block` is active for it

#### Scenario: [STALL-1-S2] A busy lane is never stalled
- **GIVEN** a lane with pull request #152 open whose session `claude agents` has read `busy` for two hours with no hook, its last `Stop` having listed one running shell
- **WHEN** the panel derives its view
- **THEN** the lane is not stalled, and no `stalled` alert is active

#### Scenario: [STALL-1-S3] A lane that holds or waits for the gate is working, and its clock starts when it lets go
- **GIVEN** a lane with pull request #152 open, its session idle, holding the gate's lease from 00:00 to 00:45
- **WHEN** the panel derives its view at 00:40, at 01:14 and at 01:15
- **THEN** it is not stalled at 00:40 or at 01:14
- **AND** at 01:15 it is stalled, with `since` 00:45

#### Scenario: [STALL-1-S4] An idle lane with no open work is not stalled
- **GIVEN** a lane on a branch with two commits ahead of the base, no pull request, no gate receipt, and its session idle for three hours
- **WHEN** the panel derives its view
- **THEN** the lane is not stalled
- **AND** only the `idle` alert is active for it, as before

#### Scenario: [STALL-1-S5] A gate that passed with nothing after it is open work, and a merge ends it
- **GIVEN** a lane, idle for 40 minutes, with no open pull request and a clean gate receipt for its HEAD
- **WHEN** the merged read finds no pull request merged from its branch, and later finds #153 merged from it
- **THEN** the lane is first stalled, with `openWork` naming the gate's pass for its HEAD and no pull request
- **AND** once #153 is read as merged it is not stalled

#### Scenario: [STALL-1-S6] A session on the main checkout that waits on a question stalls with no branch of its own
- **GIVEN** the project's main checkout, whose session `claude agents` reads `waiting` with `waitingFor` "input needed", after a `PermissionRequest` hook for `AskUserQuestion` at 00:00
- **WHEN** the panel derives its view at 00:30
- **THEN** the main checkout's lane is stalled, its `waitedOn` naming the question
- **AND** the `waiting` alert is active too, as before

#### Scenario: [STALL-1-S7] Movement clears a stall, and the next one is a new stretch
- **GIVEN** a lane stalled since 00:00
- **WHEN** a `UserPromptSubmit` hook arrives from its session at 01:00, its turn ends with a `Stop` at 01:05, and the panel derives its view at 01:20 and at 01:35
- **THEN** at 01:20 it is not stalled
- **AND** at 01:35 it is stalled with `since` 01:05

#### Scenario: [STALL-1-S8] The stall time is the project's, and 0 turns it off
- **GIVEN** two projects, one with `alerts.stall_minutes` 10 and an attached one whose sidecar sets it to 0, each with an idle lane quiet for 20 minutes and an open pull request
- **WHEN** the panel derives each project's view
- **THEN** the first project's lane is stalled
- **AND** the attached project's lane is not, and no `stalled` alert is active there

#### Scenario: [STALL-1-S9] A session the panel did not launch is judged by the same rule
- **GIVEN** a worktree on branch `fix/9-x` with pull request #160 open, where a claude session started by hand is bound by its `cwd`, reads `idle`, and has sent no hook for 30 minutes
- **WHEN** the panel derives its view
- **THEN** that worktree's lane is stalled, with `openWork` naming "#160 open"

#### Scenario: [STALL-1-S10] A lane whose claude exited with open work is stalled
- **GIVEN** a lane the panel launched, with pull request #152 open, whose pane died at 00:00 with exit status 1
- **WHEN** the panel derives its view at 00:30
- **THEN** the lane is stalled, its `openWork` naming "#152 open" and "claude exited", and its `waitedOn` naming exit status 1

#### Scenario: [STALL-1-S11] A stall from a reading that is not current is approximate
- **GIVEN** a lane with pull request #152 open, last read `idle` by a `claude agents` poll that has failed for the last 10 minutes, and no movement for 30 minutes
- **WHEN** the panel derives its view, and again after the polls have failed for 40 minutes and its session has been forgotten while a lane the panel launched in the same project has a dead pane and pull request #153 open
- **THEN** the first lane's `stall` has `approx` true, and its `stalled` alert is marked approximate
- **AND** after 40 minutes the first lane is not stalled, and the launched lane's stall is approximate

#### Scenario: [STALL-1-S12] A lane whose hand-started session has ended is not stalled
- **GIVEN** a worktree on branch `fix/9-x` with pull request #160 open, whose claude session, started by hand and bound by its `cwd`, ended (`SessionEnd`) two hours ago
- **WHEN** the panel derives its view
- **THEN** that worktree's lane is not stalled, and no `stalled` alert is active for it

#### Scenario: [STALL-1-S13] A restart keeps a stall's quiet time and starts an unknown clock at the start
- **GIVEN** a lane stalled since 00:00 whose stall was notified, a second lane with an open pull request and no saved movement, a third lane with an open pull request quiet for 20 minutes at the last save at 01:00, and the panel restarted at 02:00
- **WHEN** the first `claude agents` reading after the start reads every session `idle`, and every source has succeeded once
- **THEN** the first lane is stalled with `since` 00:00
- **AND** the second lane is not stalled until 02:30, when it stalls with `since` 02:00
- **AND** the third lane is not stalled until 02:10, when it stalls with `since` 01:40

#### Scenario: [STALL-1-S14] A lease is bound to a lane by its tmux id or a unique worktree basename
- **GIVEN** an idle lane with tmux id `t:second`, pull request #7 open and no movement for an hour, and in turn a gate lease whose `lane` is `t:second`, one whose `lane` is the lane's worktree basename, unique in the project, and one whose `lane` is a basename two worktrees share
- **WHEN** the panel derives its view while each lease is held
- **THEN** with the first two the lane is working and not stalled
- **AND** with the third the lane is stalled, the lease being no lane's

#### Scenario: [STALL-1-S15] A session started by hand beside an old pull request is a start, not a stall
- **GIVEN** a panel running since 00:00 whose `claude agents` polls have succeeded, and a worktree with pull request #160 open whose last saved movement is two days old
- **WHEN** the owner starts `claude` there at 03:00, a poll first lists it `idle`, and the panel derives its view at 03:01 and at 03:30
- **THEN** at 03:01 the lane is not stalled
- **AND** at 03:30 it is stalled with `since` 03:00

#### Scenario: [STALL-1-S16] A source that never succeeds leaves the other ways of stalling judged
- **GIVEN** an attached project whose `gh pr list` fails at every poll, and its main checkout's session waiting on an AskUserQuestion since 00:00
- **WHEN** the panel derives its view at 00:30
- **THEN** the main checkout's lane is stalled
- **AND** a warning names the pull requests as not read, and that a pull request cannot make a lane stalled until they are

#### Scenario: [STALL-1-S17] A waiting session stalls while a sibling in its lane works
- **GIVEN** the main checkout with two sessions: one waiting on a question since 00:00, and one reading `busy` throughout and sending hooks every minute
- **WHEN** the panel derives its view at 00:30
- **THEN** the lane is stalled, with `since` 00:00 and `waitedOn` naming the question

#### Scenario: [STALL-1-S18] A stale lease holder does not keep its lane working
- **GIVEN** an idle lane with pull request #7 open, the gate's lease naming it as holder, first read with `alive` false at 00:10, no waiter, and no other movement since
- **WHEN** the panel derives its view at 00:39 and at 00:40
- **THEN** at 00:39 the lane is not stalled
- **AND** at 00:40 it is stalled, with `since` 00:10

#### Scenario: [STALL-1-S19] A failed merged read keeps the last result, and is never a "none"
- **GIVEN** a lane, idle for an hour, with no open pull request and a clean gate receipt for its HEAD, stalled because its merged read found no pull request merged from its branch
- **AND** a second lane in the same state whose first merged read has not yet succeeded
- **WHEN** the first lane's next merged read fails, and the second lane's first merged read fails
- **THEN** the first lane is still stalled, with the same `since`
- **AND** the second lane is not stalled, its receipt not counted as open work until a read succeeds

#### Scenario: [STALL-1-S20] A restart keeps a waiting session's stall
- **GIVEN** the main checkout's session waiting on a question since 00:00, its lane stalled and notified at 00:30, and the panel restarted at 01:00
- **WHEN** the first `claude agents` reading after the start lists the session still `waiting` with `waitingFor` "input needed", and every source has succeeded once
- **THEN** the lane is stalled with `since` 00:00
- **AND** no notification and no push go out for it

#### Scenario: [STALL-1-S21] A restart keeps what a stalled turn ended waiting on
- **GIVEN** the main checkout's session reading `idle`, whose last `Stop` listed one running subagent with `id` `a1` described "Review group 2", and whose `SubagentStop` with `agent_id` `a1` arrived at 00:00 with nothing after it, its lane stalled since 00:00 on that background work, and the panel restarted at 01:00
- **WHEN** the first `claude agents` reading after the start lists the session `idle`, and every source has succeeded once
- **THEN** the lane is stalled with `since` 00:00, its `waitedOn` naming 1 subagent, "Review group 2", which finished and woke nothing
- **AND** no notification and no push go out for it

#### Scenario: [STALL-1-S22] A session's background-work stall is judged on its own clock, whatever a sibling does
- **GIVEN** the main checkout with two sessions: a second session, started by hand, reading `idle` whose last `Stop`, at 00:00, listed one running shell described "Run checks", with no hook from it since; and the orchestrator reading `busy` throughout, sending hooks every minute, whose last `Stop` listed a recurring cron
- **WHEN** the panel derives its view at 00:29 and at 00:30
- **THEN** at 00:29 the lane is not stalled
- **AND** at 00:30 it is stalled with `since` 00:00, its `waitedOn` naming the second session's shell, "Run checks"

#### Scenario: [STALL-1-S23] A question is timed from the ask, whatever the session's own background agents do
- **GIVEN** the main checkout's orchestrator, whose background workflow is running, asking an AskUserQuestion at 00:00 (`PermissionRequest` for `AskUserQuestion`, `claude agents` reading `waiting`)
- **WHEN** the workflow's agents send `SubagentStart` at 00:10 and 00:20 and `SubagentStop` at 00:15 and 00:25 under the orchestrator's session id, and the panel derives its view at 00:29 and at 00:30
- **THEN** at 00:29 the lane is not stalled
- **AND** at 00:30 it is stalled with `since` 00:00, its `waitedOn` naming the question

#### Scenario: [STALL-1-S24] A session resumed beside an old pull request is a start, not a stall
- **GIVEN** a panel running since 00:00 whose `claude agents` polls have succeeded, a worktree with pull request #160 open whose hand-started session ended (`SessionEnd`) at 01:00, and a lane the panel launched with pull request #161 open whose claude exited at 01:00
- **WHEN** at 03:00 the owner runs `claude --resume` with the first session's id in the worktree, and the panel's Resume brings the launched lane's pane back, and the panel derives its view at 03:01 and at 03:30
- **THEN** at 03:01 neither lane is stalled
- **AND** at 03:30 both are stalled with `since` 03:00, given no movement since

### Requirement: A stall says what the lane last waited on, from the `Stop` hook's in-flight work, decoded to an allowlist
WHEN a `Stop` hook carries `background_tasks` or `session_crons` THE SYSTEM SHALL keep each task's `id`, `type`, `status` and `description`, each cron's `schedule` and `recurring`, and the turn's last line, on the session until its next `Stop`, across a restart included, SHALL keep nothing from a `SubagentStop` but its `agent_id` (the last 64, to pair with a task a `Stop` lists before or after it), SHALL NOT show a task's `id`, and SHALL NOT decode a task's `command`, `server`, `tool`, `name` or `agent_type`, or a cron's `prompt`, from either; SHALL read a task listed as a `shell`, a background `Monitor` among them, as a shell, which is open work; a live session with a recurring cron listed, or a one-shot wakeup not yet 5 minutes past its fire time (for a one-shot `M H * * *`, minute and hour numbers and day, month and weekday `*`, the first local H:M at or after the minute the `Stop` that listed it arrived in, that day or the next, resolved once and kept across a restart), SHALL hold off its own background-work stall and its lane's open-work stall, but not another session's stall, and not its own while it waits on the owner; and the stall's `since` SHALL stay the last movement.

#### Scenario: [STALL-2-S1] A turn that ended with a shell in flight says so
- **GIVEN** a lane with pull request #152 open, whose last `Stop` carried `background_tasks` `[{"type":"shell","status":"running","description":"Run checks","command":"sh .claude/checks/run.sh"}]`
- **WHEN** the lane stalls
- **THEN** its `waitedOn` reads that its turn ended waiting on 1 shell, "Run checks", running
- **AND** no part of the view, the feed or `/api/state` contains "sh .claude/checks/run.sh"

#### Scenario: [STALL-2-S2] A turn that ended with nothing in flight says so, with its last line
- **GIVEN** a lane with pull request #152 open, whose last `Stop` carried `"background_tasks":[]`, `"session_crons":[]` and `last_assistant_message` "Waiting for merge-train's reply."
- **WHEN** the lane stalls
- **THEN** its `waitedOn` reads that its turn ended with nothing in flight to wake it, and that it last said "Waiting for merge-train's reply."

#### Scenario: [STALL-2-S3] A scheduled wakeup holds the stall off until it is due
- **GIVEN** a lane with pull request #152 open, quiet since 00:00, whose live session's last `Stop` listed a one-shot cron `0 1 18 10 *` (`recurring` false) firing at 01:00 local time on 2026-10-18, and another lane with pull request #153 open, quiet as long, whose live session's last `Stop` listed a recurring cron
- **WHEN** the panel derives its view at 01:04 and at 01:05
- **THEN** at 01:04 neither lane is stalled
- **AND** at 01:05 the first lane is stalled with `since` 00:00, and the second is not

#### Scenario: [STALL-2-S4] A Claude Code that sends no in-flight fields still stalls
- **GIVEN** a lane with pull request #152 open, whose last `Stop` has neither `background_tasks` nor `session_crons`, quiet for 30 minutes
- **WHEN** the panel derives its view
- **THEN** the lane is stalled, and its `waitedOn` says what it waited on is not reported by this Claude Code version

#### Scenario: [STALL-2-S5] The decoded hook types hold only the allowlist
- **GIVEN** the Go types the panel decodes a hook's `background_tasks` and `session_crons` into
- **WHEN** they are walked by reflection
- **THEN** they declare `id`, `type`, `status`, `description`, `schedule` and `recurring`, and no `command`, `prompt`, `server`, `tool`, `name` or `agent_type`

#### Scenario: [STALL-2-S6] A `SubagentStop` changes nothing the lane last waited on
- **GIVEN** a lane whose last `Stop` carried `"background_tasks":[]` and `last_assistant_message` "Done for now."
- **WHEN** a `SubagentStop` arrives from the same session carrying one running shell and `last_assistant_message` "reviewer finished"
- **THEN** the session's kept tasks are still empty, and its last line is still "Done for now."

#### Scenario: [STALL-2-S7] A scheduled wakeup does not hold off a session waiting on the owner
- **GIVEN** the main checkout's session, whose last `Stop` listed a recurring cron, waiting on an AskUserQuestion since 00:00
- **WHEN** the panel derives its view at 00:30
- **THEN** the main checkout's lane is stalled, with `since` 00:00 and `waitedOn` naming the question

#### Scenario: [STALL-2-S8] A background subagent at work holds the stall off until it ends
- **GIVEN** a lane with pull request #152 open, whose session reads `idle` and whose last `Stop`, at 00:00, listed one running subagent with `id` `a1`
- **WHEN** a `SubagentStop` with an empty `agent_type` and `agent_id` `a9` arrives from the session at 00:01, the panel derives its view at 01:00, a `SubagentStop` with `agent_id` `a1` arrives from it at 01:10, and the panel derives its view at 01:39 and at 01:40
- **THEN** at 01:00 and at 01:39 the lane is not stalled
- **AND** at 01:40 it is stalled with `since` 01:10

#### Scenario: [STALL-2-S9] A `SubagentStop` that arrives before its `Stop` still ends the task
- **GIVEN** a lane with pull request #152 open, whose session reads `idle`, and a `SubagentStop` with `agent_id` `a1` that arrives from the session at 00:00
- **WHEN** the session's `Stop` arrives at 00:00 too, after it, listing one running subagent with `id` `a1`, and the panel derives its view at 00:30
- **THEN** the lane is stalled with `since` 00:00, its `waitedOn` saying the subagent finished and woke nothing

#### Scenario: [STALL-2-S10] A task whose end is never heard stops holding the stall off
- **GIVEN** `alerts.stall_minutes` 30, and a lane with pull request #152 open whose session reads `idle` and whose last `Stop`, at 00:00, listed one running subagent with `id` `a1` described "Review group 2", with no hook from the session since
- **AND** a second such lane, restored from a `stall.json` saved at 01:00 by a panel restarted at once, the session's last movement 00:00 and `a1` unpaired
- **WHEN** the panel derives its view at 01:59 and at 02:00
- **THEN** at 01:59 neither lane is stalled
- **AND** at 02:00 both are stalled with `since` 00:00, each `waitedOn` saying the end of "Review group 2" was never heard

#### Scenario: [STALL-2-S11] A `ScheduleWakeup` one-shot holds its own session's stall off until 5 minutes past its next fire time
- **GIVEN** `alerts.stall_minutes` 30, times in the panel's local time, and the main checkout with a second session, started by hand, reading `idle` whose last `Stop`, received at 23:40, listed one running shell described "Run checks" and one cron `35 0 * * *` (`recurring` false), with no hook from it since
- **AND** a lane with pull request #152 open whose live session reads `idle` and whose last `Stop`, received at 23:30, listed no task and one cron `20 0 * * *` (`recurring` false), quiet since then
- **AND** a lane with pull request #153 open whose live session reads `idle` and whose last `Stop`, received at 00:05 after midnight, listed no task and one cron `40 0 * * *` (`recurring` false), quiet since then
- **AND** a lane with pull request #154 open, restored from a `stall.json` saved at 00:22 after midnight by a panel restarted at once, whose session the first `claude agents` poll after the restart lists, reading `idle`, and whose kept `Stop`, received at 23:30, listed no task and one cron `20 0 * * *` (`recurring` false), the session's and the lane's last movement 23:30, every source having succeeded once since the restart
- **WHEN** the panel derives its view after midnight at 00:24, 00:25, 00:35, 00:39, 00:40, 00:44 and 00:45
- **THEN** at 00:24 none of the four lanes is stalled
- **AND** at 00:25 the lanes of #152 and #154 are stalled, each with `since` 23:30
- **AND** at 00:35 and at 00:44 the lane of #153 is not stalled, and at 00:45 it is, with `since` 00:05
- **AND** at 00:39 the main checkout's lane is not stalled, and at 00:40 it is, with `since` 23:40 and its `waitedOn` naming the second session's shell, "Run checks"

#### Scenario: [STALL-2-S12] A background `Monitor`, listed as a shell, is read as one
- **GIVEN** `alerts.stall_minutes` 30, and the main checkout with a session reading `idle` whose last `Stop`, at 00:00, carried `background_tasks` `[{"id":"b7","type":"shell","status":"running","description":"Watch the gate's log","command":"tail -f gate.log | grep --line-buffered FAIL"}]`, as Claude Code lists a running `Monitor`, with no hook from the session since (P2 found a session reads `busy` while its `Monitor` runs; this tests the listing alone)
- **WHEN** the panel derives its view at 00:29 and at 00:30
- **THEN** at 00:29 the main checkout's lane is not stalled
- **AND** at 00:30 it is stalled with `since` 00:00, its `waitedOn` reading that the session's turn ended waiting on 1 shell, "Watch the gate's log", running
- **AND** no part of the view, the feed or `/api/state` contains "tail -f gate.log"

#### Scenario: [STALL-2-S13] A background workflow holds the stall off until a later `Stop` no longer lists it
- **GIVEN** `alerts.stall_minutes` 30, and two lanes, with pull requests #152 and #153 open, each with one session reading `idle` whose last `Stop`, at 00:00, listed one running workflow: `w1` described "Build group 2" in #152's, `w2` described "Build group 3" in #153's
- **WHEN**, in this order: each workflow's one agent sends `SubagentStart` at 00:01 under its lane's session, with `agent_type` `workflow-subagent` and `agent_id` `a7` for `w1`, `a8` for `w2`; the panel derives its view at 00:35; each agent sends its `SubagentStop` at 00:40, with the same `agent_type` and `agent_id`; the panel derives its view at 00:40; #152's session sends a `Stop` at 00:41 that lists no task, and #153's sends nothing more; and the panel derives its view at 01:10, 01:11, 02:39 and 02:40
- **THEN** at 00:35 neither lane is stalled: each workflow holds off a 34-minute quiet stretch
- **AND** at 00:40 each session still keeps its workflow task, not marked ended: a `workflow-subagent` stop ends nothing
- **AND** at 01:10 #152's lane is not stalled, and at 01:11 it is, with `since` 00:41
- **AND** at 02:39 #153's lane is not stalled, and at 02:40 it is, with `since` 00:40, its `waitedOn` saying the end of "Build group 3" was never heard

#### Scenario: [STALL-2-S14] A teammate is part of its lead's session: its hooks move the lead, and its listed task is open work, never working
- **GIVEN** `alerts.stall_minutes` 30, and the main checkout with one session, the lead, reading `idle`, whose last `Stop`, at 00:00, listed one task `{"id":"t1","type":"teammate","status":"running","description":"Probe the stall rule"}` and nothing else
- **WHEN** a `SubagentStop` with `agent_type` `probe-mate` and `agent_id` `aprobe-mate-1` arrives under the lead's `session_id` at 00:10, with no hook after (a `TeammateIdle` is not among the hooks the panel subscribes to); and the panel derives its view at 00:39 and at 00:40
- **THEN** at 00:39 the main checkout's lane is not stalled
- **AND** at 00:40 it is stalled with `since` 00:10, its `waitedOn` reading that the lead's turn ended waiting on 1 teammate, "Probe the stall rule", not that it finished: the teammate's hook was the lead's movement, and its `running` task was open work that held nothing off

#### Scenario: [STALL-2-S15] Only the allowlisted task types count; Claude Code's own housekeeping is not open work
- **GIVEN** `alerts.stall_minutes` 30, and the main checkout with one session reading `idle` whose last `Stop`, at 00:00, listed `{"id":"d1","type":"dream","status":"running"}` and `{"id":"m1","type":"memory import","status":"running"}` and no other task, with no hook from it since and no pull request or receipt for its lane
- **WHEN** the panel derives its view at 01:00
- **THEN** the main checkout's lane is not stalled
- **AND** with a third task `{"id":"s1","type":"shell","status":"running","description":"Run checks"}` in the same `Stop`, it is stalled at 00:30 with `since` 00:00, its `waitedOn` naming the shell alone

### Requirement: A stall interrupts once per stretch, on the Mac and through the owner's push command
WHEN a lane's `stalled` alert first becomes active and is not approximate THE SYSTEM SHALL send it through the notifier's rules (once per stretch, grouped per lane, at most once per lane per `min_interval_seconds`, never while the lane's terminal has focus in a visible page, and not again after a restart while it is still active) as a macOS notification and, WHERE `~/.clauductor/panel/push.json` holds a valid push command, by running that command's argv with the title and body as two more arguments, with no shell; THE SYSTEM SHALL NOT run a push command whose file is not the user's, is writable by group or others or sits in a folder that is, or names a program by a relative path, SHALL NOT push any alert but `stalled`, and SHALL build the push's title and body from the `stalled` alert's own fields alone.

#### Scenario: [STALL-3-S1] A new stall sends one notification and one push
- **GIVEN** a valid `push.json` whose `argv` is a stand-in program that records its arguments, and a lane that stalls at 00:30
- **WHEN** the notifier runs at 00:30, 00:31 and 00:45
- **THEN** one macOS notification and one push go out at 00:30, and none after
- **AND** the push's last two arguments are the title, naming the project and the lane, and a body naming the quiet time, "#152" and the kind of wait, and holding no task description and no last line
- **AND** when the same notice also carries a `waiting` alert whose text is "wants to use Bash", the push's body does not contain it

#### Scenario: [STALL-3-S2] A focused terminal suppresses the stall's notification and push
- **GIVEN** a lane that stalls while its terminal has keyboard focus in a visible page
- **WHEN** the notifier runs
- **THEN** no notification and no push go out, and the stall counts as seen

#### Scenario: [STALL-3-S3] An approximate stall never interrupts
- **GIVEN** a lane whose `stalled` alert is approximate
- **WHEN** the notifier runs
- **THEN** no notification and no push go out

#### Scenario: [STALL-3-S4] A push file that is unsafe or malformed never runs
- **GIVEN** in turn a `push.json` writable by group, one in a folder writable by group, one whose `argv[0]` is `ntfy`, and one that is not a JSON object
- **WHEN** a lane stalls
- **THEN** the push program is never run, the macOS notification goes, and a warning names the file and why it was refused
- **AND** with no `push.json` at all, the macOS notification goes and no warning shows

#### Scenario: [STALL-3-S5] A failed push is counted and does not stop the notification
- **GIVEN** a valid `push.json` whose program exits 1 with "no route"
- **WHEN** a lane stalls
- **THEN** the macOS notification goes, the notifier's failed count rises by one, and its error names "no route"

#### Scenario: [STALL-3-S6] A waiting alert is not pushed, and a stall after it is
- **GIVEN** a valid `push.json`, `waiting_seconds` 120 and `stall_minutes` 30, and a session that waits on a permission prompt from 00:00
- **WHEN** the notifier runs at 00:02 and at 00:30
- **THEN** at 00:02 a macOS notification goes for the waiting alert and no push
- **AND** at 00:30 the lane's stall goes out as a notification and a push

#### Scenario: [STALL-3-S7] A restart does not notify a stall still active
- **GIVEN** a stall resting on a gate receipt that was notified, and the panel restarted with the lane still stalled from the same saved last movement
- **AND** `gh pr list` and the merged read answer only after the notifier's first ticks have run
- **WHEN** the notifier runs before those reads answer, and again after
- **THEN** no notification and no push go out, and the stall's mark is still held in `notifier.json` throughout

### Requirement: Today's page shows a stalled lane, and `/api/state` carries it
THE SYSTEM SHALL show a stalled lane as "stalled", in words and a red-tinted row, in its lane table row, its terminal tab and its worktree tree node, and as one Needs-you row per lane naming the lane, its quiet time, its open work and what it last waited on, with Open terminal where the lane has a terminal, a waiting lane's own row being labelled stalled rather than joined by a second; `/api/state` SHALL carry the lane's `stall` (`since`, `waitedOn`, `openWork`, `approx`), a Needs-you item of kind `stalled`, and the `stalled` alert.

#### Scenario: [STALL-4-S1] The page marks a stalled lane and lists it in Needs you
- **GIVEN** a panel serving a project with a lane "second" that is stalled with pull request #7 open
- **WHEN** the page loads
- **THEN** the lane's row, its tab and its tree node read "stalled", and its row is tinted as a blocking row
- **AND** a Needs-you row names "second", its quiet time, "#7 open" and what it last waited on, and its Open terminal selects the lane

#### Scenario: [STALL-4-S2] `/api/state` carries the stall for the next page
- **GIVEN** the same stalled lane
- **WHEN** `/api/state` is read
- **THEN** the lane's `stall` has `since`, `waitedOn`, `openWork` and `approx`
- **AND** `needsYou` holds an item of kind `stalled` for it, and `alerts` a `stalled` alert of severity `block`

#### Scenario: [STALL-4-S3] One Needs-you row per stalled lane
- **GIVEN** a stalled lane with two live sessions, a stalled lane the panel launched whose claude exited, and a lane whose session waits on a permission prompt past the stall time
- **WHEN** `/api/state` is read
- **THEN** `needsYou` holds exactly one item for each of the first two lanes, of kind `stalled`
- **AND** exactly one item for the third, its waiting item, labelled stalled
