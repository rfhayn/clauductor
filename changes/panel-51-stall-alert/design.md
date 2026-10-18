# Design: panel-51-stall-alert

The panel code cited is `main` at d9f2a98 (2026-10-18). The evidence below was taken on Claude Code
**2.1.302** (Linux, a throwaway cloud container) on 2026-10-18. The owner's Mac may run another
version, so group 1 verifies E1 to E3 there first. Session ids and the scratch path are shortened
to `<id>` and `$PROBE`.

## Evidence

- **E1 A turn that ends with a background command: what `claude agents` and the `Stop` hook say.**
  An interactive `claude --settings $PROBE/settings.json --model haiku` ran in tmux, on its own
  socket. Its settings held a `Stop`, `Notification`, `UserPromptSubmit` and `PermissionRequest`
  hook that each wrote its stdin to a file. Prompt: run `sleep 70; echo probe-done` with
  `run_in_background`, and end the turn at once. `claude agents --json` was polled every 4–5 s:
  ```
  $ claude agents --json | jq -c '.[]|select(.cwd==$p)|{status,waitingFor}'
  t=4  {"status":"busy","waitingFor":null}     ← the turn has ended ("started"); the shell runs
  …    {"status":"busy","waitingFor":null}     ← busy for as long as the shell runs
  t=64 {"status":"idle","waitingFor":null}     ← the shell ended, woke the session, and its next turn ended
  ```
  The `Stop` hook's input at the turn's end:
  ```
  {"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"started",
   "background_tasks":[{"id":"bp6g8gdi2","type":"shell","status":"running",
     "description":"Run delayed probe echo in background","command":"sleep 70; echo probe-done"}],
   "session_crons":[], …}
  ```
  An `idle_prompt` Notification fired about 60 s after the `Stop`, while the shell still ran and
  `claude agents` still said busy. The shell's end woke the session: a `UserPromptSubmit` whose
  prompt began `<task-notification>`, then a second `Stop` with `"background_tasks":[]` and
  `"session_crons":[]`.
  - **What it shows:** in a main session a background command keeps the session `busy`, and its
    end wakes the session. The 4:55 AM stall was in a teammate, where the end woke no one. What a
    teammate looks like to the panel is unknown (P3).
- **E2 The fields' schema, from the 2.1.302 binary.** `strings -n 8 /opt/claude-code/bin/claude`
  holds the hook input schema. `Stop` and `SubagentStop` both carry:
  - `background_tasks`: "In-flight background work (running/pending + backgrounded) registered in
    this session. Lets hooks distinguish "session is done" from "session is paused waiting for
    background work to wake it". Empty array when nothing is in flight." Each task has `id`, `type`
    ("'shell', 'subagent', 'monitor', 'workflow'"), `status`, `description`, and `command` (shell
    only), `agent_type` (subagent only), `server`, `tool` and `name` (workflow), all optional
    but the first four.
  - `session_crons`: "Session-scoped cron tasks (CronCreate, ScheduleWakeup, /loop) that will
    wake this session later. Empty array when none are scheduled." Each has `id`, `schedule` (a
    cron expression), `recurring` ("False for one-shot wakeups whose cron field encodes a single
    fire time") and `prompt`.
  - Both are `.optional()`: an older version sends neither.
  - The same listing shows a `TeammateIdle` hook event (`teammate_name`, `team_name`), which the
    panel does not subscribe to (P3).
- **E3 A question to the owner (AskUserQuestion), in the same session.**
  ```
  t=4…32 {"status":"waiting","waitingFor":"input needed"}
  PermissionRequest {"tool_name":"AskUserQuestion"}
  Notification      {"notification_type":"permission_prompt","message":"Claude needs your permission"}
  ```
  The binary maps `requires_action` to `waiting`, and the tool `AskUserQuestion` (or a `dialog:`
  tool) to `input needed`, any other to `permission prompt`. So the 11:51 PM freeze was a
  `waiting` session to the panel, and today's waiting alert fires on it after `waiting_seconds`
  (120 s): a macOS banner only.
- **What the probes did not reach (assumptions group 1 verifies before anything is built):**
  - **P1** E1 and E3 on the owner's Mac, on its Claude Code version, through the panel's own
    async `Stop` hook. The command hook posts its stdin unchanged (`docs/panel.md`, *Hooks*), so
    the fields should arrive, but nothing has shown they do.
  - **P2** what `claude agents` reads for a session whose turn ended while a background
    **subagent** runs (fork mode's reviewer, the build's workflow), and which `SubagentStop`
    hooks the session sends until it ends, and whether the one at its end carries the task's
    `id` as its `agent_id` (D3 (d) pairs them; the cloud spike's recorded payloads show it for a
    subagent). Likewise a background workflow and a background `Monitor`, which the Mac's 2.1.303
    binary lists as a `shell` (D3 (d)). E1 covers a shell only.
  - **P3** a teammate (an agent team, as merge-train was): is it a session of its own in
    `claude agents` and in the hooks, and what does its turn's end with a background command
    look like? Does `TeammateIdle` fire, and in whose session?
  - **P4** a one-shot `ScheduleWakeup`: the `schedule` text it lists, and its time zone. Group
    1's static reading of the Mac's 2.1.303 binary (`tasks.md`, Decision log) shows it armed as
    `M H * * *` from the local clock and listed `recurring: false`, which D4 now reads. The live
    run confirms that, and records what a wakeup that comes due before its turn ends does.

## Decisions

- **D1 A lane is stalled when one of its sessions has waited on the owner for the stall time
  since it asked; or when one of its sessions ended its turn on background work, or the lane
  has open work, and that session or lane is not working and has not moved for the stall
  time.**
  - **Three clocks.** A question is timed from the ask alone. A session's own clock is reset by
    its own movement; the lane's clock by any movement in the lane. The background work a
    session's own turn ended on is judged on that session's clock alone, so a busy or looping
    sibling in the same checkout cannot hide it. A way that belongs to the lane (its branch's
    pull request or receipt, its claude exiting) is judged on the lane's clock.
  - **Working** (never stalled):
    - **a session** is working while it is `busy` on a current `claude agents` reading, or while
      its own last `Stop` listed a `subagent` or `workflow` task (running or pending) that has
      not yet ended, within D3 (d)'s limit. E1 shows a background command keeps the reading
      `busy`. Working never holds off a session's question (the first way below);
    - **the lane** is working while any of its live sessions is, or while it holds or waits for
      a queue's lease (the gate). A lease is the lane's, not one session's, so it holds off
      every way of the lane but a question: a gate run in a teammate's background shell would
      otherwise read as that teammate's stall while the gate still runs. A lease holder counts
      only while it is alive and not stale (`LeaseView.Alive` and not `Stale`):
      `lease.ReadQueue` keeps a dead holder as `Holder` until the next waiter reclaims it, and a
      gate killed with no waiter behind it would otherwise keep its lane working for ever. A
      waiter counts as listed, since only live waiters are listed.
  - **Movement** sets a clock back to zero. A session's own movement:
    - a hook from it (`LastHookAt`), but for a `Notification` whose kind reports a stop
      (those `signals.ClassifyNotification` marks `Waiting`, `Done` or `SetsIdle`:
      `permission_prompt`, `idle_prompt`, the elicitation dialogs, `agent_needs_input`,
      `agent_completed`). Those report the stop; they are not movement, and E1 and E3 show them
      arriving after the `Stop` or the ask. A `SubagentStop` is movement, so the
      end of a background subagent is;
    - a change in its `claude agents` reading (`BusySince`, `IdleSince`, `WaitingSince`);
    - its start: `claude agents` listing, after the panel's first successful poll, a session
      that was not live just before: a new id, or one that had ended (`SessionEnd`, or absent
      from a successful poll). `claude --resume` and `-c` keep the session id (the panel's own
      Resume and Restore run `claude --resume <id>`), so a resumed session is a start like a new
      one. A session started by hand sends none of the hooks the panel subscribes to before its
      first prompt (there is no `SessionStart` among them), so without this the owner opening or
      resuming `claude` beside an old pull request would be pushed a stall at once. The one
      exception: a session `forgetSessions` dropped while the `claude agents` polls failed is
      kept aside with its clock, and when the polls recover and list it again it is not a
      start and takes that clock back;
  - The lane's movement is any of its sessions' movement, and:
    - the lane taking or giving up a lease. The lease is held through the gate's whole run, so
      a gate is movement until it ends. The first queue poll that sees the lane's holder dead or
      stale counts as giving the lease up, at that poll's time;
    - the lane's tmux pane dying, or its tmux session going, or its pane coming back (a Resume
      or Restore), as the tmux poll first sees it.
  - **Stalled,** three ways:
    - **a session waits on the owner** (the one `blocked` predicate Needs you reads, so the two
      cannot disagree) and its ask is at least the stall time old. The ask's time is `blocked`'s
      `Since` (the `PermissionRequest`, the `permission_prompt` or elicitation `Notification`,
      or the `claude agents` reading of `waiting`, whichever it reports), and nothing else
      resets it: an answer ends the waiting itself, and a new ask sets a new `Since`. A
      background subagent's or workflow's hooks arrive under the parent's session id
      (`SubagentStart`, `SubagentStop`, the agent tools' `PreToolUse`), so a clock reset by the
      session's own hooks would hold off an orchestrator's question for as long as its own
      background build runs. Nothing holds this way off: not the lane's other sessions, its
      lease, nor the session's own working. In the main checkout a busy sibling (another
      session, a teammate bound by its `cwd`) would otherwise hide the orchestrator's question
      for as long as the sibling works;
    - **a session's turn ended on background work** (D3 (d)): the session is live and not
      working, the lane holds and waits for no lease, the session has not itself moved for the
      stall time, and it has no wakeup of its own still due (D4). Its siblings' busy readings,
      hooks and wakeups do not count: the orchestrator's stall-watcher loop in the main checkout
      would otherwise hide a teammate's turn that ended on a shell that woke no one;
    - **the lane** is not working, `now − the lane's last movement ≥ stall time`, and either it
      has a live session, open work (D3 (a) or (b)) and no live session with a wakeup still due
      (D4), or the panel launched it and its claude exited or its tmux session is gone, with
      open work (D3 (c)).
  - **A lane with no live session otherwise never stalls. (The owner decides.)** A worktree
    whose hand-started session has ended, with its pull request waiting for review, is the owner
    having closed it, not work that stopped by itself. That is the commonest false stall the
    rule could raise. A "live" session is one the state holds and has not ended (`SessionEnd`,
    gone from `claude agents`, or forgotten). **Beat:** every worktree with open work stalls,
    with or without a session. It would push for every PR the owner left open and closed the
    terminal on.
  - **The clock's origin, and a restart.** The model is not saved, so after a restart the panel
    has seen no movement yet:
    - **`~/.clauductor/panel/<project hash>/stall.json`** (beside `notifier.json`, 0600) keeps
      what a stall's clock and its text rest on, and is read back at start:
      - each lane's last movement, and the time of the save. A lane stalled at the save keeps
        its `since`. A lane not stalled at the save carries over the quiet time it had then, and
        the time the panel was down does not count: its clock starts at the panel's start minus
        that quiet time. Hooks sent while the panel was down reached no one, so counting the
        outage would push a lane that kept working. A lane with no saved movement starts its
        clock at the panel's start, never at the zero time;
      - each live session's own last movement, and each waiting session's ask time (its clock's
        origin), by session id, carried over as a lane's is. The first poll after a restart sets
        `WaitingSince` to its own time, so without it a stall notified before the restart would
        clear, and come back 30 minutes later with a new `since`, notified again;
      - each live session's kept `Stop` fields (D4's allowlist, clipped, task ids included, and
        each one-shot's resolved fire time) and the `agent_id`s of its last 64 `SubagentStop`s
        (D3 (d)), by session id.
        They live only in the model, so without them a session stalled on (d) would read as
        having no background work after a restart, and its stall would vanish unseen;
      - the same for each session kept aside after `forgetSessions` dropped it during failing
        polls (D1's one exception to a start), until a successful poll lists it or does not;
      - a session's entries are used only when the first poll after start lists that session (a
        waiting one still waiting), and dropped otherwise, so the file holds no session that has
        gone;
      - it is written atomically, at most once every 30 s while it has changed and once at a
        clean stop: a busy lane moves on every hook. A crash loses at most 30 s of movement,
        which moves a clock back by as much, never forward;
    - the sessions the first successful `claude agents` poll after start lists are not movement:
      that poll sets their readings (an `IdleSince` it sets is not movement). A session a later
      poll lists that the first did not was not live just before, so it is a session starting,
      which is;
    - **each way of stalling is judged from the sources it rests on,** once each has succeeded
      since start. The project's sources are `claude agents`; the pull requests (`gh pr list`);
      the receipt read; the merged read (succeeded when no lane needs one); tmux, where the
      project has lanes the panel launched; and the queues, where it declares any. A waiting
      session rests on `claude agents`; (a) on the pull requests; (b) on the receipt read and the
      merged read; (c) on tmux with (a) or (b); (d) on the session's kept `Stop`, received since
      start or restored from `stall.json` (a session with neither has (d) unknown, which is not
      open work); and working on `claude agents` and the queues. So a repository whose `gh pr
      list` never succeeds (no GitHub remote, `gh` logged out) still stalls on a frozen question.
      A source a way rests on that has not succeeded within 10 minutes of start shows a warning
      bar naming it and what it leaves unjudged;
    - **the notifier keeps the stalls' marks while they are not yet judged.** Until every one of
      those sources has succeeded once since start, or 10 minutes have passed, the view says
      stalls are pending, and `Notifier.Process` keeps each `stalled:` mark it holds instead of
      deleting it as cleared. Once judged, a stall resting on the saved movement has the `Since`
      it was notified for, so a restart sends nothing again (STALL-3-S7). Without that, the
      notify source's first 2 s tick, before `gh pr list` or the 30 s receipt read have answered,
      would find the stall missing, drop its mark, and send it again.
    - **Beat:** keying the notifier on the lane and its open work instead of `Since`. A stall
      that cleared and came back with the same pull request would never notify again.
  - **Which lane holds a lease.** `LeaseView.Lane` is free text: `$CLAUDUCTOR_LANE` (the tmux
    lane id the panel gives its lanes) or `basename "$PWD"` (`scripts/ci/run-local.sh`). A lease
    is the lane's when its `Lane` equals the lane's tmux id (`LaneView.Terminal`), or the
    basename of the lane's worktree path when no other worktree of the project has that
    basename. Any other lease is no lane's. A project that declares no queue has no lease read
    (`runtime.go` polls queues only then), so working rests on the `claude agents` reading
    alone. That still holds a gate run in the foreground or the background, because both keep
    the session busy (E1).
  - **The alert's `Since` is the last movement,** stable through the stretch: for a question the
    ask's time, for a session's background work its own clock's origin, for the lane's way the
    lane's last movement, and when several
    ways hold the lane stalled, the earliest of them. So the notifier's "once per stretch" holds (it sends
    again whenever `Since` changes), and the page's age is the quiet time.
  - **A stall clears on movement.** The next stall is a new stretch, and notifies again.
  - **Beat:**
    - a timer per signal (since the last `Stop`, since the receipt was written). Several clocks
      disagree, and each needs its own reset rule;
    - counting a busy session that sends no hook for a long time. A 45-minute foreground gate
      sends none (the panel hears `PreToolUse` only for agent tools), so it would read as a
      stall. A hung command is OPS-90's (its timeouts).

- **D2 What the panel reads to tell waiting from working: what it reads today, for every
  session on the machine, plus two `Stop` fields. No pane text, no transcript, no new hook.**
  - **A lane the panel launched:** its session, bound by `--session-id`:
    - `claude agents`: busy, idle or waiting, and `waitingFor`;
    - the hooks it already subscribes to: `Stop` (now with D4's fields), `UserPromptSubmit`,
      `PermissionRequest`, `Notification`, `SessionEnd`, `StopFailure`, and `SubagentStop`
      (paired with a kept task for D3 (d));
    - its tmux pane: dead (exited) or its session gone (orphaned), as the state already
      holds it (`ApplyTmux`);
    - the queue leases (`pollQueues`).
  - **A session it did not launch** (started by hand, in this repository or an attached one):
    the same reads but the pane. The hooks are installed in the user's `~/.claude/settings.json`,
    so every session on the machine posts them, and `claude agents` lists every session. Such a
    session is bound by its `cwd` at first sight, as today, and judged by the same rule. Its open
    work is its worktree's (D3).
    - A repository whose own settings turn hooks off (`disableAllHooks`) still has its
      `claude agents` reading: busy, idle and waiting are known, and what it waited on is "not
      reported".
    - Once `claude agents` stops listing it, the session has ended, and its lane, with no live
      session, is not stalled (D1).
  - **Beat:**
    - **the pane's text** (`tmux capture-pane`: "1 shell still running"). It is a parser of a
      TUI that changes with every release, and it exists only for lanes the panel launched;
    - **the transcript** (its last record, or its mtime). PANEL-49 reads transcripts for
      attached projects alone, under an invariant this change would widen. The hooks already
      carry what is needed, for every project;
    - **a hook of the operating model's** (D8).

- **D3 What counts as open work. (The owner decides.)**
  - **Recommended:**
    - **(a) an open pull request** whose head is the lane's branch, a draft included (`gh pr
      list`, as today);
    - **(b) a clean gate receipt for the lane's HEAD with no pull request after it, open or
      merged:** the receipt in the worktree's git dir, HEAD from `git worktree list`, and one
      merged read for the branch (D10). That is a gate that passed and nothing after it;
    - **(c) its claude exited or its tmux session is gone** (a lane the panel launched, with
      (a) or (b) true). That is work stopped with no session left to stall, and the one case
      where a lane with no live session stalls (D1);
    - **(d) the session's own last `Stop` listed background work** (D4), on any lane, the main
      checkout included. The turn ended expecting to be woken. It is that session's open work,
      judged on its own clock (D1), not the lane's.
      - **A listed `subagent` or `workflow` task is working, not yet open work,** whether its
        status is `running` or `pending` (the binary lists in-flight work in both), until a
        `SubagentStop` from the session carries an `agent_id` equal to that task's `id`. The
        recorded payloads show the two match: in `signals/testdata/spike-hooks.log` the
        `SubagentStop` of agent `a49d5b2bd0502ba13` lists a `subagent` task with that same `id`.
        They pair rather than count, because the same log shows sessions sending
        `SubagentStop`s with an empty `agent_type` for internal agents that never started (its
        events 9, 10 and 13, right after a `Stop`): a count would take one of those for the
        reviewer's end. A stop that pairs with no listed task changes nothing in (d), as
        `stopSubagent` (`state/model.go`) already retires nothing for an empty type. The kept
        tasks come from the `Stop`, not from the model's running-subagent set, which
        `idleClearsAgents` empties after 10 s idle.
      - **Order does not matter.** Each async hook posts on its own, so a `SubagentStop` can
        reach the panel before the `Stop` that still lists its task (spike line 5 shows a
        stopping subagent listed `running`). A paired stop marks the kept task ended on the task
        itself, saved with it. For a stop that arrives before its `Stop`, the session keeps the
        `agent_id`s of the last 64 `SubagentStop`s it sent, and a task a new `Stop` lists whose
        `id` is among them is marked ended as the `Stop` is kept. Ids are unique, so the order
        does not matter.
      - **A limit on a stop never heard.** A hook can be dropped, and one sent while the panel
        was down is lost. So an unpaired task holds its session working only while the session
        has moved within 4 × the stall time (2 hours by default; a background workflow's own
        agents send hooks in the session as they start and stop, which is movement). Past that,
        the task is open work, the stall's `since` is the session's last movement, and its text
        says the task's end was never heard. A task restored from `stall.json` is held to the
        same limit, from the session's restored clock. **Beat:** no limit, which would leave a
        teammate whose subagent's stop was lost working for ever, unseen.
      - The panel hears a subagent's own tool calls not at all (its `PreToolUse` and
        `PostToolUse` matchers are the agent tools alone, `signals.HookMatchers`), so without
        this a background reviewer working quietly for 30 minutes would be pushed as stalled
        while it works, and the operating model sends gates, merges and builds to background
        agents by design. The paired `SubagentStop` is the session's movement (D1), so a stall
        follows only when nothing wakes the session after its last subagent ends.
      - **P2 verifies the pairing on the owner's Mac** for a background subagent and a background
        workflow: the end of each sends a `SubagentStop` whose `agent_id` is the listed task's
        `id`. A workflow's agents stop under ids of their own (`workflow-subagent`, which
        `stopSubagent` names), so whether the workflow task's own end carries its `id` is not
        shown yet. If either does not pair, the build stops for the owner before group 2 commits,
        with P2's evidence.
      - **A `shell` task, a background `Monitor` among them, or a task of any other type,** is
        open work. A shell keeps a main session busy while it runs (E1), so it matters where
        that does not hold (a teammate: P3).
      - **A background `Monitor` is read as a shell.** The 2.1.303 binary builds each listed
        task from its registry entry's `type` alone: `id`, a label for the type, `status`,
        `description`, and `command` for a `local_bash` task. The `Monitor` tool registers a
        `local_bash` task of kind `monitor`, which is labelled `shell`, and its kind is not
        sent. Only an MCP monitor (`monitor_mcp`, `monitor_ws`) arrives as `monitor`. Nothing D4
        keeps tells the two apart: the `description` is the model's own words, with no marker
        added, and the `command` is never decoded (D4). So a Monitor is worded as one
        ("its turn ended waiting on 1 shell: Watch the gate's log (running)").
        - **P2 decides on the Mac.** If a session reads `busy` while its background `Monitor`
          runs, as it does for a shell, the Monitor is working through that reading and nothing
          changes. If it reads `idle`, a session waiting correctly on a Monitor would stall at
          the stall time, and the build stops for the owner before group 2 commits, with P2's
          evidence: telling a Monitor from a shell needs a field D4 does not keep.
        - **Beat:** a `monitor` task treated as working if a Monitor reads `idle` (this design
          before group 1): it reaches only MCP monitors, never the `Monitor` tool. And a marker
          read from `description` or `command`: the binary adds none, and a command can hold a
          secret.
  - **A waiting session needs no open work:** a question or a permission prompt is work waiting
    on the owner, on any lane (D1).
  - **The main checkout** has no branch of its own, so only (d) and a waiting session apply
    there. That is where the orchestrator runs.
  - **Not open work:** a branch of its own with commits and no pull request, and no receipt for
    its HEAD. A parked branch is not stalled work, and the `stale` alert (`stale_days`) already
    covers a branch with no commit for days. A running build is working (D1) while its session
    is busy, and while the workflow its turn ended on has not ended (d). After that, quiet, it
    is open work (d).
  - **Beat:**
    - counting commits ahead of the base: every parked branch would stall;
    - (a) alone: a gate that passed with no PR opened after it, and a lane whose claude
      exited, would go unseen.

- **D4 What it last waited on: two fields of `Stop` and `SubagentStop`, decoded to an
  allowlist.**
  - **`background_tasks`:** `id`, `type`, `status` and `description` of each task, the
    description clipped to one line of 120 runes. The `id` is an opaque handle, kept only to pair
    a task with its `SubagentStop` (D3 (d)) and never shown. `command`, `server`, `tool`, `name`
    and `agent_type` are never decoded. A
    shell command line can hold a secret, as `tool_input` can, which `HookEvent` keeps raw and
    of which only `AgentCall` decodes two fields (`subagent_type`, `description`).
  - **`session_crons`:** `schedule` and `recurring` of each. `prompt` is never decoded.
  - **Kept on the session from its `Stop` alone,** until its next `Stop`, with the turn's last
    line, which the panel already decodes (`last_assistant_message`, clipped to 120 runes), and
    saved in `stall.json` so a restart keeps them (D1). That file is the machine's, 0600, like
    `notifier.json`, and holds the allowlist alone. A
    `SubagentStop` carries the same fields (E2), listing the session's whole registry, the
    stopping subagent itself still `running` (spike line 5), and this change keeps nothing from
    it but its `agent_id`, to pair with a kept task (D3 (d)): a background subagent's last
    message is not the lane's "last said". P2 records what a turn that ends with a subagent in
    flight lists in its `Stop`.
  - **A live session's own wakeup holds off that session's background-work stall, and the
    lane's open-work stall, while it is due.** It does not hold off a sibling session's stall
    (D1): the orchestrator's stall-watcher loop would otherwise hide every other session in the
    main checkout. A session that has ended, or whose pane died, wakes nothing, so its crons
    count for nothing. **A session waiting on the owner still stalls:** a cron does not answer a
    question or a permission prompt, and the orchestrator that arms a stall-watcher loop is the
    session most likely to freeze on one.
    - A recurring cron, or a one-shot of neither shape below: it will wake the session, so the
      stall is held off while that `Stop`'s list stands.
    - A one-shot wakeup (`recurring` false) whose minute, hour, day and month are all numbers,
      read in the panel's local time: the stall is held off until 5 minutes after its fire
      time.
    - **A one-shot `M H * * *`: minute and hour numbers; day, month and weekday `*`** (as
      `ScheduleWakeup` arms it: P4). **(The owner decided, 2026-10-18.)** It is read as firing
      at the next matching minute and hour after the `Stop` that listed it, in the panel's
      local time: the first H:M at or after the minute the `Stop` arrived in, that day or the
      next. The stall is held off until 5 minutes after then.
      - The fire time is resolved once, from the time the panel received that `Stop`, and kept
        with the cron (saved in `stall.json`), so a restart does not move it.
      - `ScheduleWakeup` clamps its delay to 60–3600 s, so a wakeup armed late in the turn
        fires within the hour after the `Stop`. One that came due before its turn ended (a turn
        longer than its delay) reads as a day away: if it did not fire, its session's stall and
        its lane's open-work stall would be held off silently for up to about 24 hours. So P4
        (task 1.4) runs one, and if an overdue wakeup does not fire within a minute of its turn
        ending, the build stops for the owner before group 2 commits.
      - **Beat:** the next match after the cron's creation. `session_crons` carries no creation
        time (`id`, `schedule`, `recurring`, `prompt` alone). And "held off while that `Stop`'s
        list stands", which this shape fell to before group 1: a session whose wakeup never
        fired would be held off until a next `Stop` that may never come.
    - Either way a one-shot only holds the stall off: after that D1 applies as it stands, and
      `since` is still the last movement. A wakeup that fired is movement (its prompt is a
      `UserPromptSubmit`).
  - **The text, by case:**
    - waiting: the ask, as Needs you words it today ("input needed", "wants to use Bash");
    - turn ended with work in flight: "its turn ended waiting on 1 shell: Run checks (running)";
    - a listed subagent or workflow that has since ended (its paired `SubagentStop`): "its turn
      ended waiting on 1 subagent: Review group 2, which finished and woke nothing";
    - a listed subagent or workflow past D3 (d)'s limit, unpaired: "its turn ended waiting on 1
      subagent: Review group 2, whose end was never heard";
    - turn ended with nothing in flight: "its turn ended with nothing in flight to wake it",
      then "last said: …";
    - fields absent (an older Claude Code): "what it waited on is not reported by this Claude
      Code version". The stall is still judged;
    - exited: "its claude exited (status N)".
  - **Beat:**
    - decoding `command` and `prompt` for a fuller message. They can hold secrets, and the
      description says what the command is for;
    - ignoring crons. A session that waits correctly, with `ScheduleWakeup`, would be marked
      stalled while it does the right thing;
    - parsing every cron expression for its next fire time. That is a five-field cron parser
      for a case none of the three stalls had; D4 reads the two one-shot shapes alone.

- **D5 The stall time: 30 minutes by default, per project. (The owner decides both.)**
  - **The key:** `alerts.stall_minutes`, a number, 0 turns stalls off. Config version 7
    (`config/fields.go`: the version gate, `docs/panel.schema.json` and *Keys* follow it). An
    attached project sets it in its sidecar.
  - **Why 30:**
    - it is the `idle_minutes` default, the owner's sense of "a while";
    - each stall would have been seen within 30 minutes, not after 3.5 to 5 hours;
    - the cases that are quiet and correct for longer are working (a gate holds the lease, a
      background command keeps the session busy: E1) or have a wakeup (D4).
  - **What a short time costs:** a lane that has opened its pull request and waits for the
    owner's review is quiet and has open work, so it stalls after 30 minutes. That is one
    notification for something that does wait on the owner, and it is a stall in the row's
    sense.
  - **Beat:**
    - **15 minutes:** more of those, during a working day, for 15 minutes saved overnight;
    - **60 minutes:** an hour lost each time;
    - **one machine-wide setting:** the other thresholds are per project, and an attached
      repository's rhythm can differ.

- **D6 When and how often it interrupts: once per stretch, through the notifier that exists.
  (The owner decides.)**
  - **The alert:** `stalled`, key `stalled:<lane path>`, severity `block`, `Since` the last
    movement (D1). `block` puts it in Needs you and in the notifier's interrupts.
  - **The notifier's rules, unchanged:**
    - once per stretch: still stalled at the next poll, nothing is sent; stalled again after
      movement, it is sent again;
    - grouped with the lane's other new alerts, at most one notification per lane per
      `min_interval_seconds`;
    - suppressed while the lane's terminal has keyboard focus in a visible page;
    - an approximate alert never interrupts. Every stall is approximate while the `claude
      agents` source is not current (its last good poll older than two intervals), whether or
      not a session is left: past 30 minutes of failing polls `forgetSessions` drops the
      sessions, and a lane the panel launched could otherwise stall on (c)'s terms. So a failing
      poll cannot raise a false stall;
    - a restart does not send again for a stall still active (`notifier.json`).
  - **A waiting session** keeps today's waiting alert (a macOS banner after `waiting_seconds`)
    and, if still unanswered at the stall time, also stalls, which pushes (D7). That is the
    banner at the desk first, then the phone.
  - **No reminder** while a lane stays stalled: the row asks for one notification, and the
    page keeps showing it.
  - **Beat:**
    - a reminder every few hours. The owner is told once. A reminder at 3 AM for a stall
      notified at midnight adds noise, not news;
    - one notification per project, listing every stalled lane. The notifier groups per lane,
      and Needs you lists the rest.

- **D7 How it pushes: the macOS notification, plus a push command of the owner's, for stalls
  only. (The owner decides.)**
  - **Today's channel** (`osascript`) stays for every interrupting alert. It reaches the Mac's
    screen only, and none of the three stalls had anyone at it.
  - **A push command, the machine's:** `~/.clauductor/panel/push.json`, `{"argv": ["/abs/path",
    "arg", …]}`. The panel runs it with two more arguments, the title and the body, with no
    shell, under a 10 s timeout, for each notice that carries a `stalled` alert. Examples in
    `docs/panel.md`: ntfy, and `curl` to Pushover.
    - **The title** is the macOS notification's: `<project> · <lane>`, the project's `name` only
      while its config is trusted (else "clauductor panel"), as `pollNotify` titles it today.
    - **Refused** (a warning bar says why, and it never runs): a file not owned by the user, or
      writable by group or others, or in a folder that is; an `argv[0]` that is not an absolute
      path; anything but one JSON object with a non-empty `argv` of strings.
    - **Read** by the machine, once for every project: at start, and by a machine source that
      checks the file's `pathSignature` every 10 s and re-reads it when that changes. Each
      project's `pollNotify` takes the machine's current command, or its refusal, at each tick;
      a refusal is added to that project's view `Warnings`, so the warning bar shows on every
      project's page.
    - **A failed push** counts in the notifier's failures and shows its error, as a failed
      `osascript` does today. The macOS notification still goes.
    - **What the body carries:** how long the lane has been quiet, the open work (a PR's number) and the kind of wait ("a shell in flight", "nothing in flight",
      "a question"). Never the last line or a task's description: those are the model's words,
      and the push goes to a service the owner chose outside this machine.
    - **The push's title and body are built from the `stalled` alert's own fields,** never from
      `Notice.Body`. The notifier joins a lane's new alerts into one body, so a stall sent with a
      `waiting` alert would otherwise carry that alert's text (an elicitation's message, "wants
      to use …") to the outside service.
  - **Stalls only:** a permission prompt at 120 s stays on the Mac. It reaches the phone if it is
    still unanswered at the stall time, as a stall.
  - **Beat:**
    - **the macOS notification alone:** no change for an owner who is away, which is when every
      stall happened;
    - **waking the orchestrator to call `PushNotification`** (PANEL-40's way). The party that
      is stalled may be the orchestrator, and typing into its pane would answer its open
      question;
    - **a key in `panel.json`:** a project's config is committed (this repository is public),
      and a push topic is personal;
    - **every interrupting alert to the phone:** each permission prompt would buzz the phone
      while the owner sits at the Mac;
    - **a row of its own for the push** (the split D12 names): smaller here, but the stall
      would be marked on a page nobody is looking at until that row lands.

- **D8 OPS-90's hooks post nothing to the panel. (The owner decides.)**
  - The panel's own hook already receives every `Stop` with `background_tasks` and
    `session_crons` (E1, E2), for every session on the machine.
  - OPS-90 can read the same fields: `SubagentStop` carries `background_tasks` too (E2), which
    is what its row's probe asks for. This change records that evidence. It does not build
    OPS-90's hook.
  - **Beat:** an OPS-90 hook that posts "the turn ended, waiting on X" to the panel. It would
    make the template call the panel (`checks/no-clauductor.sh` holds that the model works
    without it), it would miss an attached repository that does not run the model, and it would
    send what the panel already has.

- **D9 Teammates: probe first (P3), and stop for the owner if they are invisible.**
  - **If a teammate is a `claude` session of its own** in `claude agents` and the hooks (a pane
    in tmux mode), it is bound by its `cwd` and judged by D1 like any session. Nothing more is
    built.
  - **If it is in-process,** the panel cannot see its turn from what it reads, and the build
    stops for the owner before group 2 commits, with P3's evidence. Recording `TeammateIdle`
    would be a design change: what it means, and whose lane it marks.
  - **Beat:** subscribing to `TeammateIdle` now. Whose session it fires in, and when, is
    unprobed, and the panel would act on a guess.

- **D10 What runs with no page in view.** Today the receipt read (`pollReadiness`) and the git
  poll run only while a page is in view. A stall matters most when nobody is looking.
  - **The receipt read** runs every 30 s whatever the page: one file read per lane worktree,
    its git dir cached. `tasks.md` and the review threads stay page-only.
  - **HEAD comes from `git worktree list`,** which is always polled, not from the page-only git
    poll.
  - **The merged read** for (b): the same `gh pr list --head <branch> --state merged` that
    `(*Runtime).mergedPR` in `autoclose.go` runs, through a variant that returns its error
    (`mergedPR` returns `(0, false)` for an error and for "none" alike), for each lane with a
    clean receipt for HEAD and no open PR.
    - Only a merge found is cached, per branch and HEAD. "None" is read again when the branch
      leaves the open-PR list, and otherwise at most every 5 minutes. A squash merge leaves the
      branch's HEAD as it was, so a cached "none" from before the PR was opened would mark every
      merged lane stalled.
    - The readiness source tracks that transition itself, from the view's `PRs` at each poll:
      `pollAutoClose`'s own tracking exists only when auto-close is on, and only for tmux lanes.
    - A read that fails keeps the last result. With no result yet, (b) is unknown, which is not
      open work. An error never reads as "none".
  - **`gh pr list` and the queues** already poll with no page.
  - **PANEL-25** (idle cost, queued after PANEL-46) must keep these reads while stalls are on and
    a lane has a live session or open work: its task 3.1 and IDLE-3-S1 stop `gh pr list` with no
    page in view. This proposal's PR appends task 3.8 to `changes/panel-25-idle-cost/tasks.md`
    saying so, so PANEL-25's build cannot merge without it (`pr-merge-guard` rule 9). Its
    `design.md` is not touched, so its approval stands; its proposal returns to the owner before
    its build anyway (its roadmap row), which is where IDLE-3-S1 takes the exception.
  - **Beat:** reading only while a page is in view. Nobody would be told when nobody looks.

- **D11 Today's page, kept small, and what PANEL-46 carries.**
  - **On today's page:**
    - the lane's state cell, its tab and its tree node read **stalled** (the word and the
      square), tinted red as a failed or stale lane is today;
    - one Needs-you row per stalled lane: its name, "quiet for 34m", the open work and what it
      last waited on, with **Open terminal** where the lane has a terminal (a lane started by
      hand has none, and its row only selects it). It is built once per lane after the session loop,
      as the `merged` and `restored` rows are (`mergeNeeds`, `state/lifecycle.go`), so a lane
      with no session (D3 (c)) gets one and a lane with two sessions does not get two;
    - **a waiting lane that stalls keeps one row:** its waiting row is labelled "Stalled" and
      gains the quiet time and open work, rather than a second row for the same ask;
    - the `stalled` alert in the lane's Alerts tab;
    - **the words and rows are drawn by a small module, `web/static/stall-view.js`,** loaded
      before `panel.js` as `context-colour.js` is, and tested under node through `vm` as
      `web/context_colour_test.go` tests that module. `panel.js` only calls it from the lane
      row, the tab, the tree node and Needs you; nothing else in `panel.js` changes. **Beat:**
      drawing in `panel.js` itself. No test runs `panel.js` (it needs a DOM, xterm and an
      event stream), so the scenario would be cited by a text grep that drives nothing.
  - **The contract, in `/api/state`:**
    - `LaneView.stall`: `since`, `waitedOn` (D4's text), `openWork` (a list of short strings:
      "#152 open", "gate passed for 1a2b3c4d5e, no PR", "claude exited"), and `approx`;
    - a `NeedView` of kind `stalled`;
    - the `stalled` alert.
  - **What PANEL-46 must carry:** under ADR-0009 a stalled lane reads as *needs you*: the red
    dot and ring, a chip that says "stalled" in words (in place of PANEL-46 D6's "needs you"),
    and a place in Needs you; a stalled lane outranks D6's "lost". This proposal's
    PR appends task 2.9 saying so to `changes/panel-46-ui-foundation/tasks.md`, so PANEL-46's
    build cannot merge without it (`pr-merge-guard` rule 9). Its `design.md` is not touched, so
    its approval stands.
    - **In the proposal's PR, not the build's:** rule 3 of `pr-merge-guard.sh` collects every
      `changes/<id>/` a PR's diff touches, and rule 9 then requires each of them fully ticked
      when the PR is a build. A build PR carrying the two appended tasks would be held to
      PANEL-46's and PANEL-25's open tasks. The proposal's PR touches only change records, which
      `cg_build_tasks` (`.claude/hooks/lib/change-guard.sh`) reads as a proposal, so it merges
      with every task open. The owner approves the two appended tasks with this design.
    - **Beat:** an `ops/` pull request the build opens. No agent of the build opens pull
      requests, so the task would have no executor.
  - **Beat:** a new colour or a new state beside ADR-0009's five. A stall is the owner's move,
    which is what "needs you" means.

- **D12 Five groups. Risk normal. Budget $80, soft. (The owner decides.)**
  - **Why normal risk:**
    - the change runs a command the owner names (the push), under D7's rules;
    - a false stall costs a notification, and a missed one costs what the three stalls cost;
    - it changes no lane's launch and writes nothing in a repository.
  - **The budget:**
    - PANEL-49 is budgeted at $60 for four groups with no probes. PANEL-48 cost $280.47 against
      $100, most of it on stalled review rounds;
    - this change has five groups, one of them probes on the owner's Mac (E1 to E3 again, P2 to
      P4), and one browser test;
    - **soft:** reaching $80 is reported to the owner and the build goes on, once OPS-85 has
      merged (session 5 runs OPS-85 first). Before it merges, `build-change` stops on any
      budget.
  - **The split it could take:** the push (group 4) as a row of its own, built after. D7 says why
    not.
  - **Beat:** a hard stop at $80. A stall alert half-built is no alert.

## Refusals
| Situation | What happens instead |
|---|---|
| A session is busy, with no hook, for hours (a hung foreground command) | It reads as working and is never stalled (D1). OPS-90's timeouts bound such a command |
| A lane holds or waits for the gate's lease | Working, however long. The stall clock starts when it gives the lease up (D1) |
| A `claude agents` reading is not current (the poll fails or is old) | A stall judged from it is approximate: shown, marked `≈`, never notified (D6) |
| The `Stop` input has no `background_tasks` (an older Claude Code) | The stall is still judged. What it waited on reads "not reported by this Claude Code version" (D4) |
| A repository whose settings turn hooks off | Judged from `claude agents` alone; what it waited on is "not reported" (D2) |
| A session scheduled its own wakeup | That session's background-work stall and its lane's open-work stall are held off while a recurring cron is listed, or until 5 minutes past a one-shot's fire time (for `ScheduleWakeup`'s `M H * * *`, the fire time is the first local H:M at or after the minute the panel received its `Stop`, that day or the next); a sibling session's stall is not, and a session waiting on the owner still stalls (D4) |
| A turn ended on a background `Monitor` | Listed as a `shell`, it is read as one: open work, worded "1 shell". If P2 finds a session reads `idle` while its Monitor runs, the build stops for the owner (D3 (d)) |
| A turn ended with a background subagent or workflow running | Working until the `SubagentStop` whose `agent_id` is the task's `id`, in whatever order the two hooks arrive; an internal stop with an empty type ends nothing; quiet for the stall time after the paired stop, it stalls (D3 (d)) |
| The paired `SubagentStop` was dropped, or sent while the panel was down | The task holds its session working only while the session has moved within 4 × the stall time; then it is open work, "whose end was never heard" (D3 (d)) |
| A teammate's turn ends on a shell that woke no one while the orchestrator in the same checkout works or loops | The teammate stalls on its own clock (D1) |
| A branch with commits and no PR or receipt | Not open work. The `stale` alert covers it after `stale_days` (D3) |
| A session the panel did not launch has ended, its PR still open | Its lane has no live session and is not stalled (D1) |
| The panel restarts while a lane is stalled | `stall.json`'s last movement, ask time and kept `Stop` fields keep the stall, its `Since` and its text, and nothing is sent again (D1) |
| A lease whose lane names no lane of the project, or two | It is no lane's, and working rests on the `claude agents` reading (D1) |
| The project declares no queue | No lease is read; a gate keeps its session busy, which is working (D1, E1) |
| A lease's holder is dead or stale, with no waiter to reclaim it | It is not working; the lane can stall (D1) |
| A source never succeeds (`gh pr list` with no GitHub remote) | The ways of stalling that rest on it are not judged, a warning bar names it after 10 minutes, and the others still are: a frozen question still stalls (D1) |
| The merged read fails | The last result stands; with none, the receipt is not counted as open work (D10) |
| The owner starts `claude` by hand in a worktree with an old open PR | Its first listing is a session starting, which is movement: no stall for the stall time (D1) |
| A session waits on the owner while another session of its lane works, or while its own background agents start and stop | It stalls the stall time after its ask (D1) |
| The owner resumes a session (`claude --resume`, `-c`, the panel's Resume) beside an old open PR | Its listing is a start, and a resumed pane is lane movement: no stall for the stall time (D1) |
| `alerts.stall_minutes` is 0 | No lane is ever stalled. The waiting and idle alerts work as today |
| `alerts.notify` is false | Stalls show on the page, and nothing is sent, the push included |
| `push.json` is missing | The macOS notification only, and no warning: a push is optional (D7) |
| `push.json` is writable by others, not the user's, not one JSON object, or names a relative program | It never runs, and a warning bar says why (D7) |
| The push command fails or times out | Counted in the notifier's failures, its error shown. The macOS notification still goes (D7) |
| A teammate is not a session of its own (P3) | The build stops for the owner before group 2 commits (D9) |
| The build's cost reaches $80 | The owner is told, and the build goes on, once OPS-85 has merged (D12) |
