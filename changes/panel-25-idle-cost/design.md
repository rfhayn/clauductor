# Design: panel-25-idle-cost

## Where the behaviour lives today

- **The cadences** are `DefaultTicks()` in `framework/internal/panel/run.go`. Each project's
  sources are the table in `newRuntime` (`framework/internal/panel/runtime.go`), each run by
  `runLoop`:
  - `worktrees`: fixed rate, `Ticks.Worktrees` (10 s); kicked by `kickWT` and by a watch that
    `stat`s git's worktree registry directory every `WorktreeWatch` (2 s, no spawn).
  - `prs`: fixed rate, `Ticks.PRs` (60 s), `gh pr list --json …`, into `Model.ApplyPRs`. Not gated
    on a page.
  - `agents`: `pollAgents` returns `Ticks.agentsInterval(lastHook, now, hasLanes)`: 5 s while
    hooks flow (a hook in the last 30 s), 15 s with no lane and no hook for 5 min, else 2 s. The
    `--cwd` filter is cross-checked with two more `claude agents` calls every `AgentsFilter`
    (5 min). `hookSeen` kicks the loop only while `agentsQuietNow` (the 15 s wait).
  - `tmux`: `tmuxPoller.tick` returns `TmuxFast` (2 s) with lanes on the socket, `TmuxIdle` (10 s)
    without. It re-reads the lane registry from disk every `RegistryLoad` (30 s), inside the poll.
    The tmux calls go through `exec.CommandContext` in `lanes.LaneManager.tmuxIn`, not the
    Runner (tests inject `LaneManager.Exec`).
  - `autoclose` (only with `lanes_auto_close`): fixed rate, `Ticks.PRs`, `waitFirst`. It reads
    the PR list from the hub (no spawn) and runs `gh pr list --head <branch> --state merged` for a
    lane when it first sees it, when its branch leaves the open list, and every 5 min while it
    asks (`autoclose.go`).
- **A page in view**: the page posts `/api/seen` once a minute while visible and on
  `visibilitychange` (`panel.js`); `web.Server.MarkVisible` records it and, when no page was in
  view, runs `OnVisible`, which today kicks only the merged-PR read (`Runtime.pageInView`,
  `metrics_source.go`). `PageVisible` holds for 90 s. The dashboard's reads (`pollProcs`,
  `pollGit`, `pollReadiness`, `pollMerged`) return early when it does not.
- **Wakes that exist**: `lanes.Changed` kicks tmux, worktrees and agents on every lane action
  (`live.go`); an event from a cwd the worktree list does not know kicks the worktrees
  (`Machine.applyHook` → `kickWorktrees`); **Refresh** kicks every kickable source.
- **The browser at login**: `Run`'s `switch` opens under `o.Launchd` when
  `install.ShouldOpenAtLogin` allows (once per 5 min), and checks `!o.NoOpen` only for a run that is
  not under launchd. `renderPlist` writes `panel [--project …] [--config …] --port N --launchd`.
  `panelInstallCmd` has no `--no-open`.

## The shape of the change

1. **Two words, one predicate each.**
   - **Out of view**: no page has said it is in view in the last 90 s (`!pageVisible(now)`, as
     today). The page is the machine's, so this is one fact for every project (D1).
   - **Dormant**, per project: out of view, and the project has no lane (`Runtime.hasLanes`:
     registered, or on its socket). `claude agents` is dormant only when, in addition, no hook has
     reached the project for 5 min (today's quiet rule).
2. **The backed-off polls** (D2). One new tick, `Ticks.Dormant` (5 min):
   - `pollAgents` asks `agentsInterval(lastHook, now, lanes, inView)`, which returns `Dormant`
     where it returns `AgentsQuiet` today and no page is in view. `agentsQuietNow` is set for
     either wait, so a hook still kicks the loop at once. While dormant the `--cwd` cross-check
     waits: the filter stands as last checked, and the next check runs at the first poll after
     dormancy ends (otherwise every dormant poll would be three calls).
   - `tmuxPoller.tick` returns `Dormant` where it returns `TmuxIdle` today and no page is in view.
   - The `worktrees` source's poll returns `Dormant` when the project is dormant.
   - `runLoop`: a fixed-rate source that returns a wait longer than its tick waits that long, on
     a timer, before going back to its ticker. A kick or its watch still wakes it at once, and a
     tick that queued during the long wait does not cause a second poll.
3. **The pull requests** (D3). A new tick, `Ticks.PRsOutOfView` (3 min). The `prs` poll:
   - in view: runs and returns 0 (every 60 s, as today);
   - out of view, with a lane auto-close watches (the filter `pollAutoClose` applies today:
     registered, on a branch, in its own worktree, `AutoCloseOf(type) == on_merge`, factored into
     one function both call): runs, and returns `PRsOutOfView`;
   - out of view with no such lane: spawns nothing, returns `PRsOutOfView`, and checks again then.
   `pollAutoClose` is unchanged: it still compares the open list it reads from the hub with the
   last, and asks `gh` about a merge before it closes anything.
4. **Waking.** `Runtime.pageInView` (run on `OnVisible`) kicks `agents`, `worktrees`, `tmux` and
   `prs` as well as the merged read. `hookSeen` and `lanes.Changed` are unchanged; they already
   wake what each needs.
5. **The spawn count** (D5). Each runtime's Runner is wrapped by a counter, and so is the
   machine's (`Machine.run`); `LaneManager` counts its tmux calls through a `Spawned` callback set
   in `newLaneManager`. Each counts by command (`claude agents`, `git worktree`, `gh pr`, `tmux
   list-panes`, …) over a rolling 10 minutes, into the project's `Obs` (`spawnsPerMin`,
   `spawnsBy`): the footer shows it, and so does `/api/state`, which does not mark a page in view.
   Under `--launchd` a machine source writes one line an hour to the log: `spawns in the last
   hour: N (claude agents a, git worktree b, tmux list-panes c, gh pr d, other e)`.
6. **`panel install --no-open`** (D4). `InstallOptions.NoOpen` → `plistSpec.NoOpen` →
   `--no-open` after `--launchd` in `ProgramArguments`. In `Run`, `o.NoOpen` wins under launchd
   too: no tab, and `browser-opened` is not written. `install` prints which it installed: "At
   login: opens the page once per login (--no-open to stop)" or "At login: opens no browser tab
   (clauductor panel open opens it)".
7. **Docs**: `docs/panel.md`'s sources table, *What the panel reads, and when*, *Close a lane
   when its PR merges*, *The lane registry*, the `claude agents` cadence under *How signals are
   read*, and *The launchd agent*.

### The cadences

Dormant applies to the first three rows; the pull requests follow "out of view" alone.

| Poll | In view, or the project has a lane | Idle today | Idle after this change | Woken at once by |
|---|---|---|---|---|
| `claude agents` | unchanged: 2 s; 5 s while hooks flow; 15 s with no lane and no hook for 5 min | 15 s, plus the cross-check every 5 min | **5 min** (dormant and no hook for 5 min), no cross-check | a hook; a lane action; a page coming into view; Refresh |
| `git worktree list` | unchanged: 10 s | 10 s | **5 min** (dormant) | a worktree added or removed (the 2 s `stat` watch); an event from an unknown cwd; a lane action; a page coming into view; Refresh |
| tmux `list-panes` (the registry re-read rides on it) | unchanged: 2 s with lanes, 10 s without | 10 s | **5 min** (dormant) | a lane action; a page coming into view; Refresh |
| `gh pr list` | in view: unchanged, 60 s | 60 s | out of view: **none**, or **3 min** for a project with a lane auto-close watches | a page coming into view; Refresh |
| `gh pr list --head <b> --state merged` | unchanged | as today | unchanged: first sight of a lane, its branch leaving the open list, every 5 min while it asks | — |

The arithmetic, two projects with the template's `panel.json`, dormant: three polls at 0.2 a
minute each is 0.6 per project, plus about 0.4 from the project's own interval commands, so 2 for
both; the machine's `claude --version` and `auth status` add 0.2. About 2.2 a minute, against a
target of 3.

## Refusals

| Situation | What happens instead |
|---|---|
| A hook arrives while `claude agents` waits its 5 min | It polls at once (`hookSeen`, as for the 15 s wait today), and the next interval counts the hook: no longer dormant for 5 min |
| A lane starts in a dormant project | `lanes.Changed` polls tmux, worktrees and agents at once; with a lane the project is not dormant |
| A lane in project A, none in project B, no page | A keeps today's cadence; B is dormant. A lane is per project, the page is the machine's (D1) |
| Out of view, a lane on a branch with `lanes_auto_close` off | No `gh pr list`: nothing reads it until a page comes back |
| `gh pr list` fails while out of view | As today: the last list is kept, marked failed; auto-close sees no branch leave the list, so it closes nothing on a failed read |
| The PR list is minutes old when auto-close reads it | It only triggers a look: a close still needs `gh pr list --head <branch> --state merged` and Close's own plan, as today |
| A page comes into view | Every backed-off source polls at once; the page shows them current within a second or two, not 5 minutes later |
| The tmux server is up with no lane while dormant | `list-panes` every 5 min; `show-environment` with it at most then (it runs only on a poll) |
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
- **Recommended:** 5 minutes for `claude agents`, `git worktree list` and tmux.
- **Alternatives:** 1 minute (about 7 spawns a minute for two projects; gentler); or no timed poll
  at all, only the wakes.
- **Why:** everything that matters already wakes the panel at once (a hook, a lane action, a
  worktree added, a page in view), so the timed poll only bounds what nothing announces: a branch
  switched in the main checkout, a session whose hooks do not reach the panel, a tmux session
  started by hand on the socket. Five minutes matches the hook-quiet window and auto-close's
  recheck. Polling never would leave those to the next page view.

**D3. How auto-close learns of a merge with no page in view.**
- **Recommended:** keep the one `gh pr list` per project, but out of view only for a project with
  a lane auto-close watches, every 3 minutes. No such lane, no GitHub call.
- **Alternatives:**
  - per lane, `gh pr list --head <branch> --state merged` on a timer (one call per lane, not per
    project);
  - only on a hook that shows `gh pr merge` in a lane (misses a merge on github.com or from another
    machine);
  - the same gate at 60 s.
- **Why:** one call covers every lane, and `autoclose.go`'s open-then-gone trigger and its
  `--state merged` check stay exactly as tested. A merged lane closing within about 4 minutes
  rather than 1 costs nothing when nobody is looking, and auto-close is opt-in, so most projects
  make no GitHub call at all while idle.

**D4. Where `--no-open` lives.**
- **Recommended:** in the plist's `ProgramArguments`, written by `panel install --no-open`; each
  install states which it installed.
- **Alternative:** a machine setting (like `remote-control.json`) that a reinstall without the
  flag keeps.
- **Why:** it is the flag `clauductor panel` already has, visible in the one file that says what
  the agent runs, and `install` rewrites that file anyway. Only `panel install` writes the plist
  (`install.sh` does not), so a reinstall is always a person at a terminal who sees the line.

**D5. How the spawn rate is measured.**
- **Recommended:** the panel counts what it spawns, by command: a rolling 10 minutes in each
  project's `obs` (the footer and `/api/state`), and under the login agent one log line an hour.
- **Alternative:** no code; measure from outside (sample `ps`, or `execsnoop` with root).
- **Why:** the outcome check in 7 days needs a number it can read without root and without
  opening the page (which ends the very state it measures). `ps` sampling misses processes that
  live 50 ms. The count also tells the next row which cadence costs the most.
