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
    **subagent** runs (fork mode's reviewer, the build's workflow). E1 covers a shell only.
  - **P3** a teammate (an agent team, as merge-train was): is it a session of its own in
    `claude agents` and in the hooks, and what does its turn's end with a background command
    look like? Does `TeammateIdle` fire, and in whose session?
  - **P4** a one-shot `ScheduleWakeup`: the `schedule` text it lists, and its time zone.

## Decisions

- **D1 A lane is stalled when it is not working, has not moved for the stall time, and either
  waits on the owner or has open work.**
  - **Working** (never stalled): a session of the lane is `busy` on a current `claude agents`
    reading, or the lane holds or waits for a queue's lease (the gate). E1 shows a background
    command keeps the reading `busy`, so a lane that waits correctly on its own command is
    working. A lease holder counts only while it is alive and not stale (`LeaseView.Alive`
    and not `Stale`): `lease.ReadQueue` keeps a dead holder as `Holder` until the next waiter
    reclaims it, and a gate killed with no waiter behind it would otherwise keep its lane
    working for ever. A waiter counts as listed, since only live waiters are listed.
  - **Movement** sets the stall clock back to zero:
    - a hook from any of the lane's sessions (`LastHookAt`);
    - a change in a session's `claude agents` reading (`BusySince`, `IdleSince`, `WaitingSince`);
    - a session starting: one that `claude agents` lists for the first time after the panel's
      first successful poll. A session started by hand sends none of the hooks the panel
      subscribes to before its first prompt (there is no `SessionStart` among them), so without
      this the owner opening `claude` beside an old pull request would be pushed a stall at once;
    - the lane taking or giving up a lease. The lease is held through the gate's whole run, so
      a gate is movement until it ends;
    - the lane's tmux pane dying, or its tmux session going, as the tmux poll first sees it.
  - **Stalled,** two ways:
    - **a session waits on the owner** (the one `blocked` predicate Needs you reads, so the two
      cannot disagree) and has itself not moved for the stall time: its own clock, from the
      ask's `Since` and its own hooks, whatever its lane's other sessions do. In the main
      checkout a busy sibling (another session, a teammate bound by its `cwd`) would otherwise
      hide the orchestrator's question for as long as the sibling works;
    - **the lane** is not working, `now − its last movement ≥ stall time`, and either it has a
      live session, open work (D3) and no live session with a wakeup still due (D4), or the
      panel launched it and its claude exited or its tmux session is gone, with open work
      (D3 (c)).
  - **A lane with no live session otherwise never stalls. (The owner decides.)** A worktree
    whose hand-started session has ended, with its pull request waiting for review, is the owner
    having closed it, not work that stopped by itself. That is the commonest false stall the
    rule could raise. A "live" session is one the state holds and has not ended (`SessionEnd`,
    gone from `claude agents`, or forgotten). **Beat:** every worktree with open work stalls,
    with or without a session. It would push for every PR the owner left open and closed the
    terminal on.
  - **The clock's origin, and a restart.** The model is not saved, so after a restart the panel
    has seen no movement yet:
    - each lane's last movement is saved in `~/.clauductor/panel/<project hash>/stall.json`
      (beside `notifier.json`), and read back at start. A lane with no saved movement starts its
      clock at the panel's start, never at the zero time;
    - the sessions the first successful `claude agents` poll after start lists are not movement:
      that poll sets their readings (an `IdleSince` it sets is not movement). A session first
      listed by a later poll is a session starting, which is;
    - **each way of stalling is judged from the sources it rests on,** once each has succeeded
      since start: a waiting session on `claude agents`; (a) on the pull requests; (b) on the
      receipt read and the merged read; (c) on tmux with (a) or (b); (d) on the hooks; and
      working on `claude agents` and, where the project declares queues, the queues. So a
      repository whose `gh pr list` never succeeds (no GitHub remote, `gh` logged out) still
      stalls on a frozen question. A source a way rests on that has not succeeded within 10
      minutes of start shows a warning bar naming it and what it leaves unjudged;
    - **the notifier keeps the stalls' marks while they are not yet judged.** Until every source
      the project has has succeeded once since start, or 10 minutes have passed, the view says
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
  - **The alert's `Since` is the last movement,** stable through the stretch. So the notifier's
    "once per stretch" holds, and the page's age is the quiet time.
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
      `PermissionRequest`, `Notification`, `SessionEnd`, `StopFailure`;
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
    - **(d) the last `Stop` listed background work** (D4), on any lane, the main checkout
      included. The turn ended expecting to be woken.
  - **A waiting session needs no open work:** a question or a permission prompt is work waiting
    on the owner, on any lane (D1).
  - **The main checkout** has no branch of its own, so only (d) and a waiting session apply
    there. That is where the orchestrator runs.
  - **Not open work:** a branch of its own with commits and no pull request, and no receipt for
    its HEAD. A parked branch is not stalled work, and the `stale` alert (`stale_days`) already
    covers a branch with no commit for days. A running build is working (D1) while its session
    is busy. When its turn has ended with the workflow in the background, it is (d).
  - **Beat:**
    - counting commits ahead of the base: every parked branch would stall;
    - (a) alone: a gate that passed with no PR opened after it, and a lane whose claude
      exited, would go unseen.

- **D4 What it last waited on: two fields of `Stop` and `SubagentStop`, decoded to an
  allowlist.**
  - **`background_tasks`:** `type`, `status` and `description` of each task, clipped to one line
    of 120 runes. `command`, `server`, `tool`, `name` and `agent_type` are never decoded. A
    shell command line can hold a secret, as `tool_input` can, which `HookEvent` keeps raw and
    of which only `AgentCall` decodes two fields (`subagent_type`, `description`).
  - **`session_crons`:** `schedule` and `recurring` of each. `prompt` is never decoded.
  - **Kept on the session from its `Stop` alone,** until its next `Stop`, with the turn's last
    line, which the panel already decodes (`last_assistant_message`, clipped to 120 runes). A
    `SubagentStop` carries the same fields (E2) for the subagent's own work, and this change
    keeps nothing from it: a background subagent's last message is not the lane's "last said".
    P2 records what a turn that ends with a subagent in flight lists in its `Stop`.
  - **A live session that scheduled its own wakeup holds the stall off while the wakeup is
    due.** A session that has ended, or whose pane died, wakes nothing, so its crons count for
    nothing.
    - A recurring cron, or one whose fields are not all numbers: it will wake the session, so
      the lane is not stalled while that `Stop`'s list stands.
    - A one-shot wakeup (`recurring` false, minute, hour, day and month all numbers, read in the
      panel's local time: P4): the stall is held off until 5 minutes after its fire time. The
      wakeup only holds it off: after that D1 applies as it stands, and `since` is still the
      last movement. A wakeup that fired is movement (its prompt is a `UserPromptSubmit`).
  - **The text, by case:**
    - waiting: the ask, as Needs you words it today ("input needed", "wants to use Bash");
    - turn ended with work in flight: "its turn ended waiting on 1 shell: Run checks (running)";
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
      for a case none of the three stalls had.

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
    - **Read** at start and when the file's signature changes (the same poll as the config's).
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
  - **PANEL-25** (idle cost, queued after PANEL-46) must keep these reads while a lane has open
    work. Its approved proposal returns to the owner before its build anyway (its roadmap row).
    Task 5.3 opens the `ops/` pull request that adds this to its `tasks.md` (D11 says why not in
    this change's own PR).
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
    dot and ring, a chip that says "stalled" in words, and a place in Needs you. Task 5.3 opens
    an `ops/` pull request that appends a task saying so to
    `changes/panel-46-ui-foundation/tasks.md`, so PANEL-46's build cannot merge without it
    (`pr-merge-guard` rule 9). Its `design.md` is not touched, so its approval stands.
    - **Not in this change's own PR:** rule 3 of `pr-merge-guard.sh` collects every
      `changes/<id>/` a PR's diff touches, and rule 9 then requires each of them fully ticked.
      PANEL-46's and PANEL-25's open tasks would block PANEL-51's build from merging. Rule 9
      runs only on change branches, so an `ops/` PR carries the two appended tasks.
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
| A session scheduled its own wakeup | Not stalled while a recurring cron is listed, or until 5 minutes past a one-shot's fire time (D4) |
| A branch with commits and no PR or receipt | Not open work. The `stale` alert covers it after `stale_days` (D3) |
| A session the panel did not launch has ended, its PR still open | Its lane has no live session and is not stalled (D1) |
| The panel restarts while a lane is stalled | The saved last movement keeps the stall's `Since`, and nothing is sent again (D1) |
| A lease whose lane names no lane of the project, or two | It is no lane's, and working rests on the `claude agents` reading (D1) |
| The project declares no queue | No lease is read; a gate keeps its session busy, which is working (D1, E1) |
| A lease's holder is dead or stale, with no waiter to reclaim it | It is not working; the lane can stall (D1) |
| A source never succeeds (`gh pr list` with no GitHub remote) | The ways of stalling that rest on it are not judged, a warning bar names it after 10 minutes, and the others still are: a frozen question still stalls (D1) |
| The merged read fails | The last result stands; with none, the receipt is not counted as open work (D10) |
| The owner starts `claude` by hand in a worktree with an old open PR | Its first listing is a session starting, which is movement: no stall for the stall time (D1) |
| A session waits on the owner while another session of its lane works | The waiting session stalls on its own clock (D1) |
| `alerts.stall_minutes` is 0 | No lane is ever stalled. The waiting and idle alerts work as today |
| `alerts.notify` is false | Stalls show on the page, and nothing is sent, the push included |
| `push.json` is missing | The macOS notification only, and no warning: a push is optional (D7) |
| `push.json` is writable by others, not the user's, not one JSON object, or names a relative program | It never runs, and a warning bar says why (D7) |
| The push command fails or times out | Counted in the notifier's failures, its error shown. The macOS notification still goes (D7) |
| A teammate is not a session of its own (P3) | The build stops for the owner before group 2 commits (D9) |
| The build's cost reaches $80 | The owner is told, and the build goes on, once OPS-85 has merged (D12) |
