# Design: panel-25-idle-cost

## Where the behaviour lives today

- **The cadences** are `DefaultTicks()` in `framework/internal/panel/run.go`, defaulted field by
  field in `Ticks.withDefaults`. Each project's sources are the table in `newRuntime`
  (`framework/internal/panel/runtime.go`), each run by `runLoop`:
  - `worktrees`: fixed rate, `Ticks.Worktrees` (10 s); kicked by `kickWT` and by a watch that
    `stat`s git's worktree registry directory every `WorktreeWatch` (2 s, no spawn).
  - `prs`: fixed rate, `Ticks.PRs` (60 s), `gh pr list --json …` (the open pull requests), into
    `Model.ApplyPRs`. Not gated on a page.
  - `agents`: `pollAgents` returns `Ticks.agentsInterval(lastHook, now, hasLanes)`: 5 s while
    hooks flow (a hook in the last 30 s), 15 s with no lane and no hook for 5 min, else 2 s. The
    `--cwd` filter is cross-checked with two more `claude agents` calls at the first poll
    (`agentsFilterCheck.IsZero()`) and then every `AgentsFilter` (5 min). `hookSeen` kicks the
    loop only while `agentsQuietNow` (the 15 s wait).
  - `tmux`: `tmuxPoller.tick` returns `TmuxFast` (2 s) with lanes on the socket, `TmuxIdle` (10 s)
    without, and `TmuxFast` after any error (`list-panes` failing other than "no server", or the
    registry re-read failing). It re-reads the lane registry from disk every `RegistryLoad` (30 s),
    inside the poll.
  - `autoclose` (only with `lanes_auto_close`, and only while `r.lanes != nil && r.trusted()`):
    fixed rate, `Ticks.PRs`, `waitFirst`. It reads the open list from the hub (no spawn) and runs
    `gh pr list --head <branch> --state merged --json number` for a lane when it first sees it
    (`a.seen`), when its branch leaves the open list, and every 5 min while it asks
    (`autoclose.go`). It logs nothing on a close; it prints only when a notification fails.
- **A gap auto-close has today.** A pull request opened and merged between two reads of the open
  list is never in it, so it never "leaves" it. The lane's first-sight check ran before the pull
  request existed, and the lane is not asking, so it stays open until the panel restarts. The
  window is 60 s today.
- **A page in view**: the page posts `/api/seen` once a minute while visible and on
  `visibilitychange` (`panel.js`); `web.Server.MarkVisible` records it and, when no page was in
  view, runs `OnVisible`, which today kicks only the merged-PR read (`Runtime.pageInView`,
  `metrics_source.go`). `PageVisible` holds for 90 s. The dashboard's reads (`pollProcs`,
  `pollGit`, `pollReadiness`, `pollMerged`) return early when it does not.
- **Wakes that exist**: `lanes.Changed` kicks tmux, worktrees and agents on every lane action
  (`live.go`); an event from a cwd the worktree list does not know kicks the worktrees
  (`Machine.applyHook` → `kickWorktrees`); **Refresh** kicks every kickable source.
- **Where the panel starts processes.**
  - **The one Runner.** `Run` sets `o.Runner` once (`signals.ExecRunner` unless a test injects
    one). Every runtime's `r.run`, the machine's `Machine.run` (through `m.defRT().exec`, so in
    the default project's root) and every `LaneManager.Run` (`newLaneManager`: `ClosePlan`,
    `agentStatus`, `removeVerdict`, the first-prompt check, `git worktree add`) share it.
  - **Beside it**: tmux in `LaneManager.tmuxIn` (`exec.CommandContext`, or `LaneManager.Exec` in
    tests); `SendNotice`'s `osascript` (`runtime.go`); a terminal's `tmux attach` under a PTY
    (`web/terminal.go`); a queue RUN (`queues.go`); Terminal.app (`lanes.go`
    `OpenInTerminalApp`); the browser (`install/launchd.go`); and a lease holder's start time
    (`lease.ProcStart`, `/bin/ps`, cached per process by `ProcCache`).
- **PANEL-28 builds first** (this row's Deps; the order is PANEL-29, PANEL-28, then PANEL-30, 31
  and 25), so this change builds on it, not on the code above alone. As proposed
  (`changes/panel-28-login-refresh/design.md`, branch `change/panel-28-login-refresh`, under
  revision as this is written), PANEL-28:
  - wraps the same Runner with a `claude` gate: one machine-wide slot for every panel `claude`
    call, a stop policy past a timeout, and a reader of Claude Code's login lock;
  - has a poll that finds the slot taken (or the lock live) skip its turn and keep its value,
    while a lane action waits for the slot within its own timeout (its D3, D4);
  - takes `claude auth status` off its 10-minute timer: at start, when no recent reading is saved,
    and on **Refresh** (its D5).

  Where this design says "the gate", "the slot" or "a skipped poll", it means PANEL-28's as it
  merges. The builder reads its merged design first.
- **The browser at login**: `Run`'s `switch` opens under `o.Launchd` when
  `install.ShouldOpenAtLogin` allows (once per 5 min), and checks `!o.NoOpen` only for a run that is
  not under launchd. `renderPlist` writes `panel [--project …] [--config …] --port N --launchd`.
  `panelInstallCmd` has no `--no-open`.

## The shape of the change

1. **Three words, one predicate each.**
   - **Out of view**: no page has said it is in view in the last 90 s (`!pageVisible(now)`, as
     today). The page is the machine's, so this is one fact for every project (D1).
   - **Dormant**, per project: out of view, and the project has no lane (`Runtime.hasLanes`:
     registered, or on its socket). `git worktree list` and tmux back off when dormant.
   - **Idle**, per project: dormant, and no hook has reached the project for 5 min (today's quiet
     rule). `claude agents` backs off when idle, and the spawn count's idle minutes are these.
2. **The backed-off polls** (D2). One new tick, `Ticks.Dormant` (5 min), in `DefaultTicks` and
   `withDefaults`. It is the wait between timed polls; every wake in item 4 still polls at once.
   - `pollAgents` asks `agentsInterval(lastHook, now, lanes, inView)`, which returns `Dormant`
     where it returns `AgentsQuiet` today and no page is in view. `agentsQuietNow` is set for
     either wait, so a hook still kicks the loop at once. The first poll's cross-check still runs
     (the filter must be decided once, idle or not); after it, while idle, the cross-check waits,
     and the next one runs at the first poll after the project stops being idle. Otherwise every
     idle poll would be three calls.
   - **A poll PANEL-28's slot skips** (the slot taken, or the login lock live) spawned nothing and
     read nothing, so it never waits the dormant interval (D9). `pollAgents` recognises the gate's
     skip and returns `AgentsFast` (2 s): it retries every 2 s, spawning nothing while it is
     skipped, until a poll runs, and only that poll's interval may be `Dormant`. The kick that
     started it is spent, so the retry is what keeps a hook's answer seconds away; it is also what
     keeps a timed idle poll that was skipped from making the reading 10 minutes old. The
     cross-check's two calls go through the slot the same way: a skipped cross-check stays due.
   - `tmuxPoller.tick` returns `Dormant` where it returns `TmuxIdle` today and no page is in view,
     and also after an error while dormant (D7).
   - The `worktrees` source's poll returns `Dormant` when the project is dormant.
   - `runLoop`: a fixed-rate source that returns a wait longer than its tick waits that long, on
     a timer, before going back to its ticker. A kick or its watch still wakes it at once, and a
     tick that queued during the long wait does not cause a second poll.
3. **The pull requests** (D3).
   - The `prs` source (the open list, for the page) runs in view, every 60 s as today, and spawns
     nothing out of view. It is kicked when a page comes into view.
   - The `autoclose` source keeps its gate (`r.lanes != nil && r.trusted()`) and its filter of
     watched lanes (registered, on a branch, in its own worktree, `AutoCloseOf(type) ==
     on_merge`). New: when it has a watched lane, it reads the merged pull requests itself, at most
     every `Ticks.MergedLanes` (3 min, in `DefaultTicks` and `withDefaults`), in view or not: one
     `gh pr list --state merged --limit 20 --json number,headRefName,mergedAt`. A watched lane
     whose branch is in that list, under a number not yet checked for that lane, is due. With no
     watched lane, no call.
   - A due lane goes through today's path unchanged: `gh pr list --head <branch> --state merged`
     (now asking `--json number,mergedAt`), Close's own plan, then a close or an ask.
   - In view, the open-then-gone trigger stays as the fast path (within 60 s). The first-sight
     check and the 5-minute recheck while asking stay as they are.
4. **Waking.** `Runtime.pageInView` (run on `OnVisible`) kicks `agents`, `worktrees`, `tmux` and
   `prs` as well as the merged read. `hookSeen` and `lanes.Changed` are unchanged; they already
   wake what each needs.
5. **The spawn count** (D5, D6).
   - **Where.** A counter wraps the Runner once in `Run`, before any runtime, the machine or a
     lane manager receives it, so each start through the Runner is seen once. It goes **inside**
     PANEL-28's gate, which is already there when this builds: the project tag (in the context),
     then the gate, then the counter, then `signals.ExecRunner`. A call the gate skips, or holds
     waiting for the slot, reaches the counter only when it starts, so it is never counted as a
     spawn. `LaneManager.tmuxIn` reports each tmux call through a `Spawned` callback (on the
     `Exec` path too); tmux is not `claude`, so the gate does not see it.
   - **Whose.** Each runtime runs its commands with the context tagged with its project, and so
     does its lane manager (the tmux callback is set per project in `newLaneManager`).
     `Machine.run` tags its calls "machine", and no longer charges them to the default project.
   - **What it holds.** Per project (and "machine"): one-minute buckets for the last 60 minutes,
     each with the minute's spawns by command (`claude agents`, `git worktree`, `gh pr`, `tmux
     list-panes`, …), whether the project was idle at each start, and the seconds it was idle. The
     idle seconds accrue on the `obs` source's tick (1 s, no spawn).
   - **Who reads it.** `pollObs` publishes the last 10 minutes into the project's `Obs`
     (`spawnsPerMin`, `spawnsIdlePerMin`, `spawnsBy`): the footer shows it beside the `claude
     agents` row, and `/api/state` returns it without marking a page in view. Under `--launchd` a
     machine source writes, once an hour, one line per project and one for the machine:
     `spawns, last hour, clauductor: idle <I> min, <N> spawns (<N/I> a minute); busy <B> min, <M>
     spawns; claude agents <a>, tmux list-panes <b>, git worktree <c>, gh pr <d>, other <e>`, and
     `spawns, last hour, machine: <n> (claude --version <v>, other <o>)` (`auth status` shows
     under "other" when **Refresh** or a start runs it).
   - **What it leaves out**, named in the docs: the starts beside the Runner other than tmux (a
     notification, a terminal attach, a queue RUN, Terminal.app, the browser, a lease holder's
     start time). Each follows a person's action or a held lease, not a timer.
6. **The auto-close log line** (D8). Each close and each ask writes one line to the panel's
   output: `auto-close: lane fix-x: PR #12 merged 14:02:11, closed 14:04:30 (2m19s)`, or `… asked
   14:04:30: claude is working`. The merge time is the `mergedAt` the `--head` check now asks for.
7. **`panel install --no-open`** (D4). `InstallOptions.NoOpen` → `plistSpec.NoOpen` →
   `--no-open` after `--launchd` in `ProgramArguments`. In `Run`, `o.NoOpen` wins under launchd
   too: no tab, and `browser-opened` is not written. `install` prints which it installed: "At
   login: opens the page once per login (--no-open to stop)" or "At login: opens no browser tab
   (clauductor panel open opens it)".
8. **Docs**: `docs/panel.md`'s sources table, *What the panel reads, and when*, *Close a lane
   when its PR merges*, *The lane registry*, the `claude agents` cadence under *How signals are
   read*, *The launchd agent*, and the observability footer (the count, and what it leaves out).

### The cadences

`git worktree list` and tmux follow "dormant", `claude agents` "idle", the open pull requests "out
of view", and auto-close's reads whether the project has a watched lane.

| Poll | In view, or the project has a lane | Idle today | Idle after this change | Woken at once by |
|---|---|---|---|---|
| `claude agents` | unchanged: 2 s; 5 s while hooks flow; 15 s with no lane and no hook for 5 min | 15 s, plus the cross-check every 5 min | **5 min**; the cross-check at the first poll only | a hook; a lane action; a page coming into view; Refresh |
| `git worktree list` | unchanged: 10 s | 10 s | **5 min** | a worktree added or removed (the 2 s `stat` watch); an event from an unknown cwd; a lane action; a page coming into view; Refresh |
| tmux `list-panes` (the registry re-read rides on it) | unchanged: 2 s with lanes (and after an error), 10 s without | 10 s; 2 s after an error | **5 min**, after an error too | a lane action; a page coming into view; Refresh |
| `gh pr list` (open) | in view: unchanged, 60 s | 60 s | **none** while out of view | a page coming into view; Refresh |
| `gh pr list --state merged --limit 20` (auto-close) | new: every 3 min with a watched lane, in view or not | — | none (no lane, so none watched) | — |
| `gh pr list --head <b> --state merged` | unchanged: first sight of a lane, its branch leaving the open list, every 5 min while it asks; new: its branch in the merged list | as today | none (no lane) | — |

The arithmetic, two projects with the template's `panel.json`, idle: three polls at 0.2 a minute
each is 0.6 per project, plus about 0.4 from the project's own interval commands: 1.0 per
project, against a target of 1.2. The machine's timed calls are `claude --version` alone, once
PANEL-28 has taken `auth status` off its 10-minute timer: 0.1, against 0.2. About 2.1 a minute
for both, against 3. A poll PANEL-28's slot skips spawns nothing, so it lowers these figures,
never raises them.

## Refusals

| Situation | What happens instead |
|---|---|
| A hook arrives while `claude agents` waits its 5 min | It polls at once (`hookSeen`, as for the 15 s wait today), and the next interval counts the hook: no longer idle for 5 min |
| A lane starts in a dormant project | `lanes.Changed` polls tmux, worktrees and agents at once; with a lane the project is not dormant |
| A lane in project A, none in project B, no page | A keeps today's cadence; B is dormant. A lane is per project, the page is the machine's (D1) |
| Out of view, a lane on a branch with `lanes_auto_close` off | No `gh` call: nothing reads the answer until a page comes back |
| A pull request opened and merged between two reads | Auto-close's merged-list read sees it within 3 min, in view or not (D3); today it is missed |
| More than 20 pull requests merge in the project between two merged-list reads | A lane whose PR fell off the list is missed by that read. In view, the open-then-gone trigger still catches a PR it saw open. Stated in the docs |
| A merged-list or `--head` read fails | Nothing is due from it; the next read tries again. Auto-close never closes on a failed read |
| A page comes into view | Every backed-off source polls at once; the page shows them current within a second or two |
| tmux errors while the project is dormant | The next poll is in 5 min, not 2 s (D7); the page's "Cannot read" banner refreshes when a page comes into view |
| The tmux server is up with no lane while dormant | `list-panes` every 5 min; `show-environment` with it at most then (it runs only on a poll) |
| PANEL-28's gate skips a poll, or holds a lane action waiting for the slot | Not counted until it starts (D6) |
| A hook-kicked `claude agents` poll finds PANEL-28's slot taken, or the login lock live | Skipped, and retried every 2 s, spawning nothing, until it runs; the dormant 5 min applies only after a poll that ran (D9) |
| A timed idle `claude agents` poll is skipped | The same: retried every 2 s, not 5 min later, so the reading never ages past one dormant interval plus the skip |
| `/api/state` read with the token, no page open | Answers, the spawn count included; it does not mark a page in view (only `POST /api/seen` does) |
| `panel install` without `--no-open` | Today's behaviour, and it says so in its output. A reinstall rewrites the plist, so the choice is made at each install (D4) |
| `panel --launchd --no-open` crash-restarted by KeepAlive | Opens nothing, writes no `browser-opened` |

## Decisions (awaiting the owner)

**D1. What makes a project dormant.**
- **Recommended:** no lane in that project, and no page in view anywhere. A lane is the
  project's; the page is the machine's (one `/api/seen` for all projects).
- **Alternative:** in view only for the project the page shows, so a page open on project A lets
  project B back off.
- **Why:** the page's project menu summarises every project from its hub, so B's figures are on
  screen too. The page does not tell the server which project it shows when it says it is in
  view, and adding that buys little: an open page is the owner at the computer.

**D2. The dormant cadence.**
- **Recommended:** 5 minutes between timed polls for `claude agents` (idle), `git worktree list`
  and tmux (dormant).
- **Alternatives:** 1 minute (about 7 spawns a minute for two projects; gentler); or no timed poll
  at all, only the wakes.
- **Why:** everything that matters already wakes the panel at once (a hook, a lane action, a
  worktree added, a page in view), so the timed poll only bounds what nothing announces: a branch
  switched in the main checkout, a session whose hooks do not reach the panel, a tmux session
  started by hand on the socket. Five minutes matches the hook-quiet window and auto-close's
  recheck. Polling never would leave those to the next page view.

**D3. How auto-close learns of a merge.**
- **Recommended:** the open list is the page's and stops out of view. Auto-close reads the merged
  pull requests itself: one `gh pr list --state merged --limit 20` per project every 3 minutes,
  only while the project has a watched lane, in view or not. A watched lane whose branch is in it
  goes through today's `--head` check and Close's plan.
- **Alternatives:**
  - keep the open list out of view for a project with a watched lane, every 3 min, and add a
    per-lane `gh pr list --head <branch> --state merged` for each watched lane whose branch was
    never seen open (one call per such lane per read);
  - keep the open list out of view every 3 min, and state the gap: a PR opened and merged between
    two reads leaves its lane open until a restart;
  - the same at 60 s.
- **Why:** the open list cannot see a pull request that opened and merged between two reads, at
  any cadence; a merged list can. One call covers every lane, however many have no PR yet, and it
  closes the gap that exists at 60 s today. Auto-close is off by default (`config/fields.go`), so
  most projects make no GitHub call out of view. A merged lane closes within about 4 minutes
  instead of 1 when the open list missed it, which costs nothing when nobody is looking.

**D4. Where `--no-open` lives.**
- **Recommended:** in the plist's `ProgramArguments`, written by `panel install --no-open`; each
  install states which it installed.
- **Alternative:** a machine setting (like `remote-control.json`) that a reinstall without the
  flag keeps.
- **Why:** it is the flag `clauductor panel` already has, visible in the one file that says what
  the agent runs, and `install` rewrites that file anyway. Only `panel install` writes the plist
  (`install.sh` does not), so a reinstall is always a person at a terminal who sees the line.

**D5. How the spawn rate is measured.**
- **Recommended:** the panel counts what it spawns, per project and by command, with each
  project's idle minutes beside its idle spawns: the last 10 minutes in `obs` (the footer and
  `/api/state`), and under the login agent one line per project an hour.
- **Alternatives:**
  - one machine total an hour (simpler, but the check cannot tell idle hours from busy ones, and
    the owner keeps lanes running);
  - no code; measure from outside (sample `ps`, or `execsnoop` with root).
- **Why:** the outcome check in 7 days needs an idle rate it can read without root and without
  opening the page, which ends the very state it measures. `ps` sampling misses processes that
  live 50 ms. The per-project busy figures also tell the next row which cadence costs the most.

**D6. Where the count sits, and what it covers.**
- **Recommended:** one counter around the shared Runner, applied once in `Run`, plus tmux's own
  starts; calls tagged by project (the machine's as "machine"). It sits innermost, inside
  PANEL-28's `claude` gate (which builds first), so a call the gate skips or holds waiting is not
  a spawn, and only processes actually started are counted. The starts beside the
  Runner that follow a person's action (a notification, a terminal attach, a queue RUN,
  Terminal.app, the browser, a lease holder's start time) are left out, and the docs say so.
- **Alternatives:**
  - count at every start site, eight of them, for a literal "every process";
  - count outside the gate, so a skipped call counts as an attempt (that measures the panel's
    intent, not what the machine paid).
- **Why:** the Runner and tmux are every start that runs on a timer, which is what idling costs.
  Wrapping the Runner per runtime and again in the machine would count `claude --version` and
  `auth status` twice and charge them to the default project; wrapping it once, with a tag, sees
  each start once and knows whose it is.

**D7. A tmux error while dormant.**
- **Recommended:** wait the dormant 5 minutes, as after a good poll.
- **Alternative:** keep today's 2 s retry after an error, dormant or not.
- **Why:** with no lane and no page there is nothing to recover quickly for, and a broken tmux
  would otherwise spawn 30 times a minute indefinitely. A lane start or a page coming into view
  polls at once, and the error is shown when a page is there to show it.

**D8. A log line per auto-close.**
- **Recommended:** each close and each ask writes one line with the pull request's merge time and
  the close or ask time; the `--head` check asks for `mergedAt` to get it.
- **Alternative:** no line; the close stays visible only in the lane's Activity and a
  notification.
- **Why:** the Activity goes with the lane and a notification is not kept, so neither can show in
  a week how long merged lanes stayed open. The line is the evidence for the auto-close half of
  the Signal, where a project has auto-close on.

**D9. A `claude agents` poll that PANEL-28's slot skips.**
- **Recommended:** retry every 2 s (`AgentsFast`), spawning nothing while skipped, until a poll
  runs; only a poll that ran may set the dormant 5-minute wait. This holds whatever woke the poll:
  a hook, a page coming into view, or the timer.
- **Alternatives:**
  - keep the interval the cadence gives, so a skipped idle poll waits the full 5 minutes again;
  - retry only a kicked poll, and let a timed one wait.
- **Why:** a hook-kicked poll is how a waiting session outside a lane gets its notification
  (*Current or stale*: only a current reading confirms "waiting"). Its kick is spent when it is
  skipped, so without the retry the answer could be up to 5 minutes late, or never come if the
  hook was the session's last. A skipped poll costs nothing, so retrying every 2 s adds no spawn;
  the slot is held for seconds, so the retry runs within seconds (within about a minute while
  PANEL-28 holds polls back for a live login lock, as it proposes). A timed poll is treated the same
  so an idle reading never ages past one dormant interval plus the skip.
