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
    (`autoclose.go`). `mergedPR` asks for up to 5 merged PRs on the branch and keeps only the
    highest number. It logs nothing on a close; it prints only when a notification fails.
  - **The project's own commands** run on `panel.json`'s refresh rules, page or not. With the
    template's `panel.json` several call GitHub: the metrics command every 600 s
    (`metrics_source.go` `metricsSources`, ungated; `.claude/metrics.sh` runs `gh api graphql
    --paginate` and `gh pr list --limit 200`), and the fix suggestions every 300 s
    (`.claude/panel-suggest.sh fix`: `gh issue list`). The metrics command's only reader is the
    Metrics view.
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

  As of PANEL-28's `17617ae` (agreed with its proposer), the gate is a plain Runner wrapper that
  starts nothing itself: it runs the inner Runner under its own context and keeps its slot until
  the inner call returns, and `signals.ExecRunner` is the only starter (the stop signals for
  `claude` argv live there). A poll that finds the slot taken waits up to 2 s, then returns a retry
  in 1 s through its own next-wait, applying no update; while a login lock is live it returns a
  retry in 2 s (its D3, D4). `checkFilter` records its check only after a cross-check that ran.

  Where this design says "the gate", "the slot" or "a skipped poll", it means PANEL-28's as it
  merges. The builder reads its merged design and code first.
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
   - **A poll PANEL-28's gate skips** (the slot still taken after its 2 s wait, or the login lock
     live) spawned nothing and read nothing. PANEL-28 already returns its own short retry through
     the poll's next-wait (1 s for the slot, 2 s for a live lock). This change adds one rule (D9):
     the dormant wait never replaces that retry. `pollAgents` computes `Dormant` only after a poll
     that ran. The kick that started a skipped poll is spent, so the retry is what keeps a hook's
     answer seconds away, and what keeps a skipped timed idle poll from leaving the reading 10
     minutes old. A skipped cross-check stays due (PANEL-28 records the check only after one ran).
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
     on_merge`). New: when it has a watched lane, it reads the recently merged pull requests
     itself (the **merged-list read**), at most every `Ticks.MergedLanes` (3 min, in
     `DefaultTicks` and `withDefaults`), in view or not, bounded by merge time:
     `gh pr list --state merged --search "merged:>=<since>" --limit 100 --json
     number,headRefName,mergedAt`.
     - **Its own argv builder**, `signals.MergedSinceArgv(since time.Time)`, beside
       `signals.MergedArgv`, which stays as it is: that one takes a date (`2006-01-02`), a limit of
       300 and the Metrics view's fields, all pinned by `TestParseMergedPRsRealShape`, and a date
       would make a 10-minute margin meaningless. `ParseMergedPRs` parses both outputs (a row
       without `mergedAt` is skipped, as today).
     - **`<since>` is RFC 3339 in UTC with a `Z`** (`2026-10-02T14:02:11Z`, `time.RFC3339` of the
       UTC time), never a `+` offset, which a search query would read as a space.
     - **Why the bound.** Without `--search`, `gh pr list --state merged --limit N` returns the N
       most recently *created* merged PRs, not the most recently merged. With many newer PRs
       merged meanwhile, a lane whose PR was opened long ago would be missing.
     - **`<since>`** is the start of the last read that succeeded, minus 10 minutes (the margin
       covers GitHub's search index lagging a merge, and clock skew). The first read uses the
       panel's start minus 10 minutes: a merge before that is the first-sight check's to find. A
       failed read leaves `<since>` where it was, so the next read covers the gap. A merge the
       index has not caught up with is found by a later read, so every "within N minutes" for
       auto-close means plus any search-index lag.
     - **The limit, truthfully.** The read returns at most 100 PRs merged since `<since>`. When it
       returns exactly 100 the list may be cut, so every watched lane not checked since `<since>`
       gets the `--head` check instead (one call per such lane, that once).
     - **Due.** A watched lane whose branch is in the list under a PR number not yet checked for
       that lane. With no watched lane, no call.
   - **"Checked" is a set.** `mergedPR` (the **`--head` check**) now returns every merged PR it
     lists for the branch (`--json number,mergedAt`, up to 5), and each lane records every number
     it has checked, at first sight too, not only the highest. So "a number not yet checked" is
     well defined: a PR merged before the lane was first seen is never due again.
   - A due lane goes through today's path unchanged: the `--head` check, Close's own plan, then a
     close or an ask.
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
     spawn. Since PANEL-28's `17617ae` the gate starts nothing itself and `ExecRunner` is the only
     starter, so the counter between them counts `claude` calls too, once each, when the inner
     call starts the process. It counts on entry to the inner Runner, not on return: the gate can
     hand its caller a timeout while the process still runs, and that process is still one spawn.
     `LaneManager.tmuxIn` reports each tmux call through a `Spawned` callback (on the `Exec` path
     too); tmux is not `claude`, so the gate does not see it.
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
6. **The auto-close log line** (D8). A close writes one line to the panel's output: `auto-close:
   lane fix-x: PR #12 merged 14:02:11, closed 14:04:30 (2m19s)`. An ask writes `… asked 14:04:30:
   claude is working` the first time for that lane and PR, and again only when its reasons
   change; a 5-minute recheck that finds the same reasons writes nothing. A close after an ask
   writes the close line. The merge time is the `mergedAt` the `--head` check now asks for.
7. **The metrics command waits for a page** (D10), for both forms of `metrics.refresh`
   (`interval:<s>` and `watch:<path>`, `config.ParseRefresh`). Two flags on the metrics store,
   beside `mergedDue`, and no other state:
   - **`metricsForce`**: `refreshAll` sets it, as it sets `mergedDue` today (it otherwise only
     kicks sources, so the poll could not tell **Refresh** from any other kick). Every poll
     clears it first, with `Swap(false)` at entry, as `pollMerged` does `mergedDue`.
   - **`metricsDue`** starts true (never run). A run clears it.
   - **A poll** (whatever woke it: a tick of the interval, a change of the watched file, a trust
     kick, a page's return, **Refresh**):
     - with a page in view, or with `metricsForce` taken at entry: runs the command, as today.
       So **Refresh** always runs it, even in the rare case it arrives with no page in view;
     - otherwise: runs nothing and sets `metricsDue`. It then reads the page's visibility once
       more, and runs after all if a page came into view meanwhile, so a return that raced the
       poll is not missed.
   - **A page coming into view** (`pageInView`) kicks the source only if `metricsDue` or
     `metricsForce` is set. So the return runs it once when anything was missed (an interval
     passed, the watched file changed, it never ran), and sends no kick at all otherwise.
   - Its run at start therefore waits for the first page.

   Cards and suggest commands keep their rules.
8. **`panel install --no-open`** (D4). `InstallOptions.NoOpen` → `plistSpec.NoOpen` →
   `--no-open` after `--launchd` in `ProgramArguments`. In `Run`, `o.NoOpen` wins under launchd
   too: no tab, and `browser-opened` is not written. `install` prints which it installed: "At
   login: opens the page once per login (--no-open to stop)" or "At login: opens no browser tab
   (clauductor panel open opens it)".
9. **Docs**: `docs/panel.md`'s sources table, *What the panel reads, and when*, *Close a lane
   when its PR merges*, *The lane registry*, the `claude agents` cadence under *How signals are
   read*, *Metrics* and the `metrics.refresh` key (D10), *The launchd agent*, and the
   observability footer (the count, and what it leaves out).

### The cadences

`git worktree list` and tmux follow "dormant", `claude agents` "idle", the open pull requests "out
of view", and auto-close's reads whether the project has a watched lane.

| Poll | In view, or the project has a lane | Idle today | Idle after this change | Woken at once by |
|---|---|---|---|---|
| `claude agents` | unchanged: 2 s; 5 s while hooks flow; 15 s with no lane and no hook for 5 min | 15 s, plus the cross-check every 5 min | **5 min**; the cross-check at the first poll only | a hook; a lane action; a page coming into view; Refresh |
| `git worktree list` | unchanged: 10 s | 10 s | **5 min** | a worktree added or removed (the 2 s `stat` watch); an event from an unknown cwd; a lane action; a page coming into view; Refresh |
| tmux `list-panes` (the registry re-read rides on it) | unchanged: 2 s with lanes (and after an error), 10 s without | 10 s; 2 s after an error | **5 min**, after an error too | a lane action; a page coming into view; Refresh |
| `gh pr list` (open) | in view: unchanged, 60 s | 60 s | **none** while out of view | a page coming into view; Refresh |
| merged-list read, `gh pr list --state merged --search "merged:>=<since>" --limit 100` (auto-close) | new: every 3 min with a watched lane, in view or not | — | none (no lane, so none watched) | — |
| `--head` check, `gh pr list --head <b> --state merged` | unchanged: first sight of a lane, its branch leaving the open list, every 5 min while it asks; new: its branch in the merged list under an unchecked number, or a merged-list read that came back full | as today | none (no lane) | — |
| the project's metrics command (D10) | in view: unchanged, its `metrics.refresh` | its `metrics.refresh` (600 s in the template) | **none** while out of view | a page coming into view, when its interval passed; Refresh |

The arithmetic, two projects with the template's `panel.json`, idle:
- **The panel's polls:** three at 0.2 a minute each, 0.6 per project.
- **The project's own interval commands:** the fix suggestions every 300 s (0.2), the health card
  every 600 s (0.1) and the metrics command every 600 s (0.1): 0.4, or 0.3 with D10.
- **Per project:** 1.0, or 0.9 with D10, against a target of 1.2.
- **The machine:** `claude --version` alone on a timer, once PANEL-28 has taken `auth status` off
  its 10-minute timer: 0.1, against 0.2.
- **Both projects and the machine:** about 2.1 a minute (1.9 with D10), against 3.

A poll PANEL-28's gate skips spawns nothing, so skips lower these figures, never raise them.

## Refusals

| Situation | What happens instead |
|---|---|
| A hook arrives while `claude agents` waits its 5 min | It polls at once (`hookSeen`, as for the 15 s wait today), and the next interval counts the hook: no longer idle for 5 min |
| A lane starts in a dormant project | `lanes.Changed` polls tmux, worktrees and agents at once; with a lane the project is not dormant |
| A lane in project A, none in project B, no page | A keeps today's cadence; B is dormant. A lane is per project, the page is the machine's (D1) |
| Out of view, a lane on a branch with `lanes_auto_close` off | No `gh` call from the panel's own polls: nothing reads the answer until a page comes back. The project's own commands still run on their rules (D10 for the metrics command) |
| A pull request opened and merged between two reads | Auto-close's merged-list read sees it within 3 min, in view or not (D3); today it is missed |
| The lane's PR was opened weeks ago, and over a hundred PRs created after it merged before the last read | Found: the read is bounded by merge time, not by creation, so the older-merged PRs are outside it and every PR merged since `<since>` is in it, up to 100 |
| The merged-list read returns its full 100 | The list may be cut, so every watched lane not checked since `<since>` gets the `--head` check, once |
| A merge GitHub's search has not indexed yet when the read runs | `<since>` trails the last good read by 10 min, so the next read covers it |
| A merged-list or `--head` read fails | Nothing is due from it, and `<since>` stays put, so the next read covers the gap. Auto-close never closes on a failed read |
| The first-sight check finds two merged PRs on a reused branch | Both numbers are recorded as checked; neither makes the lane due again |
| A lane asks, and each 5-minute recheck finds the same reasons | One log line at the first ask, none at the rechecks; another only when the reasons change, and one at the close |
| No page in view and the metrics command's interval passes | It does not run (D10), and it is marked due. The first page view runs it at once; the Metrics view shows its last figures until it returns |
| No page in view and the file a `watch:` metrics rule names changes | The same: marked due, run at once when a page comes back. The watch's kick is not lost |
| A page comes back and nothing was missed | `pageInView` sends the metrics source no kick, so nothing runs for the return; its rule runs it next |
| **Refresh** while the metrics command ran a minute ago | It runs (`metricsForce`), as today, with a page in view or not |
| A tick of the interval and a page's return arrive together | The tick's poll runs it (a page is in view) and clears `metricsDue`; if the return's kick is still buffered, that poll runs it again as today's in-view kick would. At most one extra run, never a lost one |
| A page comes into view | Every backed-off source polls at once; the page shows them current within a second or two |
| tmux errors while the project is dormant | The next poll is in 5 min, not 2 s (D7); the page's "Cannot read" banner refreshes when a page comes into view |
| The tmux server is up with no lane while dormant | `list-panes` every 5 min; `show-environment` with it at most then (it runs only on a poll) |
| PANEL-28's gate skips a poll, or holds a lane action waiting for the slot | Not counted until it starts (D6) |
| A hook-kicked `claude agents` poll finds PANEL-28's slot taken, or the login lock live | PANEL-28's retry applies (a wait of up to 2 s, then a retry in 1 s for the slot, 2 s for a live lock), spawning nothing; the dormant 5 min applies only after a poll that ran (D9) |
| A timed idle `claude agents` poll is skipped | The same retry, not 5 min later, so the reading never ages past one dormant interval plus the skip |
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
  pull requests itself: one `gh pr list --state merged --search "merged:>=<since>" --limit 100`
  per project every 3 minutes, only while the project has a watched lane, in view or not.
  `<since>` is the last good read's start minus 10 minutes, in RFC 3339 UTC, so the read is
  bounded by merge time to the second, through a new argv builder beside `signals.MergedArgv`
  (which takes a date). A watched lane whose branch is in it under a number it has
  not checked goes through today's `--head` check and Close's plan. A read that comes back full
  sends every watched lane not checked since `<since>` to the `--head` check instead.
- **Alternatives:**
  - keep the open list out of view for a project with a watched lane, every 3 min, and add a
    per-lane `gh pr list --head <branch> --state merged` for each watched lane whose branch was
    never seen open (one call per such lane per read);
  - keep the open list out of view every 3 min, and state the gap: a PR opened and merged between
    two reads leaves its lane open until a restart;
  - the same at 60 s;
  - the merged list without `--search`, `--limit 20`: simpler, but gh orders it by creation, so a
    merge train hides an old PR's merge.
- **Why:** the open list cannot see a pull request that opened and merged between two reads, at
  any cadence; a merged list can, provided it is bounded by merge time. One call covers every
  lane, however many have no PR yet, and it closes the gap that exists at 60 s today. Auto-close
  is off by default (`config/fields.go`), so for most projects the panel's own polls make no
  GitHub call out of view. A merged lane closes within about 4 minutes (plus any search-index
  lag) instead of 1 when the open
  list missed it, which costs nothing when nobody is looking.

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
  a spawn, and only processes actually started are counted. Because PANEL-28's gate (as of
  `17617ae`) starts nothing itself and `ExecRunner` is the only starter, the counter sees every
  `claude` call too, once, at the moment the inner call starts it. Nothing in `17617ae` conflicts:
  its gate keys on the argv and reads no tag, its stop signals live in `ExecRunner` beneath the
  counter, and its proposer has adopted this order. The starts beside the Runner that follow a
  person's action (a notification, a terminal attach, a queue RUN,
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

**D9. A `claude agents` poll that PANEL-28's gate skips.**
- **Recommended:** reuse PANEL-28's retry as it stands (a wait of up to 2 s for the slot, then a
  retry in 1 s for the slot or 2 s for a live lock, through the poll's next-wait), and add one
  rule: the dormant 5-minute wait is set only by a poll that ran, never after a skip. This holds
  whatever woke the poll: a hook, a page coming into view, or the timer.
- **Alternatives:**
  - a retry of this change's own (every 2 s), beside PANEL-28's: two retry rules for one skip;
  - let the cadence's interval win, so a skipped idle poll waits the full 5 minutes again;
  - keep the short retry only for a kicked poll, and let a timed one wait.
- **Why:** a hook-kicked poll is how a waiting session outside a lane gets its notification
  (*Current or stale*: only a current reading confirms "waiting"). Its kick is spent when it is
  skipped, so if the dormant wait won, the answer could be up to 5 minutes late, or never come if
  the hook was the session's last. PANEL-28's retry already does the right thing and spawns
  nothing while skipped; this change must only not override it. The slot is held for about
  0.1 s a call, so the retry runs within 2 s (longer only while a login refresh holds its lock).
  A timed poll is treated the same, so an idle reading never ages past one dormant interval plus
  the skip.

**D10. The project's metrics command while no page is in view.**
- **Recommended:** run it only while a page is in view, on its `metrics.refresh` rule, interval
  or watch, and at once when a page comes into view if anything was missed meanwhile (an interval
  passed, the watched file changed, it never ran); **Refresh** runs it as today. The mechanism is
  two flags: a poll with no page in view marks it due instead of running, and a page's return
  kicks it only when it is due or forced; every poll with a page in view runs as today. Cards and
  suggest commands keep their rules.
- **Alternatives:**
  - a third flag marking a poll woken by the page's return, which then runs only if due: it can
    run twice (a tick's poll takes the flag, then the buffered kick runs as an in-view poll) and
    lose a watched change (a coalesced kick finds the flag and skips after the watch has moved
    on), so it was dropped;
  - for a `watch:` rule, decide on the page's return by the watched file's mtime against the last
    run's time, instead of a due flag (it also survives a restart, which a never-run command
    covers anyway);
  - apply it to `interval:` rules only, and leave a `watch:` rule running page or not;
  - leave it on its rule, page or not (the template's runs every 600 s and calls `gh api graphql
    --paginate` and `gh pr list --limit 200` each time);
  - also hold the interval cards and suggest commands while no page is in view (the fix
    suggestions' `gh issue list` every 300 s, the health card every 600 s).
- **Why:** the metrics command's only reader is the Metrics view, and its contract is a reader's
  (stdout is the metrics JSON), so holding it changes nothing anyone sees: the first page view
  runs it. It is the costliest of the project's commands, a paginated search on GitHub. Cards
  and suggest commands are also read only by the page, but a card's command is any script the
  project names, with no contract that it only reads, so when it runs is the project's choice in
  `panel.json`, not the panel's to change. Holding them is the
  alternative if the owner prefers the lower figure: about 0.6 a minute per idle project
  instead of 0.9.
