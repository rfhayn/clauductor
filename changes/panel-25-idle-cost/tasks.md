# Tasks: panel-25-idle-cost

## Progress
- 2026-10-02 proposed; nothing built yet
- 2026-10-02 revised after review: auto-close reads merged PRs itself (D3), the count sits once around the shared Runner (D6), per-project idle hours (D5), tmux errors while dormant (D7), the auto-close log line (D8)

## Decision log
- (none yet)

## 1. The panel counts what it spawns
- [ ] 1.1 A counter wraps `o.Runner` once in `Run`, before any runtime, the machine or a lane manager receives it (D6); `LaneManager` reports each tmux call through a `Spawned` callback set per project in `newLaneManager` (on the `Exec` path too). Calls carry their project in the context (each runtime's `run`, each lane manager's `Run`); `Machine.run` tags "machine". Tested in `framework/internal/panel/spawns_test.go` (a fake runner and `pclock.NewFake`, as `dashboard_cost_test.go`) citing [IDLE-4-S3]
- [ ] 1.2 The counter keeps one-minute buckets for the last 60 minutes per project and for the machine: spawns by command, idle or not at each start, and idle seconds accrued on the `obs` tick. `pollObs` publishes the last 10 minutes as `spawnsPerMin`, `spawnsIdlePerMin` and `spawnsBy`. Tested in `spawns_test.go` with the fake clock
- [ ] 1.3 `/api/state` returns those `obs` fields and does not mark a page in view: a test starts a panel with a counting runner (`startPanelWith`, as `multiproject_integration_test.go`), reads `/api/state` with `get`, asserts `spawnsBy`, `spawnsPerMin` and `spawnsIdlePerMin`, and asserts no page counts as in view afterwards (the Metrics view's `gh pr list --state merged --search …`, which `OnVisible` kicks, does not run, and the open-list `gh pr list` stays off); in `framework/internal/panel/spawns_integration_test.go` citing [IDLE-4-S1]
- [ ] 1.4 Under `--launchd`, a machine source writes once an hour one line per project ("spawns, last hour, <project>: idle I min, N spawns (R a minute); busy …; by command") and one for the machine, tested in `spawns_test.go` with the fake clock and two projects, one idle all hour, citing [IDLE-4-S2]
- [ ] 1.5 The observability footer shows the rate, the idle rate and the top commands beside the `claude agents` row (`panel.js`), wiring tested in `framework/internal/panel/web/spawns_footer_test.go` (the `readWeb` string test `rowactions_test.go` uses)

## 2. A dormant project backs off, and wakes at once
- [ ] 2.1 `Ticks.Dormant` (5 min) in `DefaultTicks` and `withDefaults`; `agentsInterval` takes whether a page is in view and returns `Dormant` where it returns `AgentsQuiet` today and none is; `agentsQuietNow` is set for either wait. Interval table tested in `framework/internal/panel/cost_test.go` (extend `TestAgentsCadence`)
- [ ] 2.2 While idle, the `--cwd` cross-check runs only at the first poll and then waits until the project stops being idle. Tested by running `pollAgents` through `runLoop` with a counting runner over 30 minutes of `pclock.NewFake`, counting `claude agents` polls and asserting no unfiltered call after the first poll, in `cost_test.go` (extend `TestAgentsFilterAndBackoff`) citing [IDLE-1-S1]; the same loop for one minute with a page marked in view (`srv.MarkVisible`, as `dashboard_cost_test.go`) citing [IDLE-1-S3]
- [ ] 2.3 `tmuxPoller` returns `Dormant` where it returns `TmuxIdle` today and no page is in view, and after an error (a failing `list-panes`, a failing registry re-read) while dormant (D7). Tested in `cost_test.go` with `tmuxPollerFor` and `runTicks` (as `TestTmuxIdleSocketSpawnsOnlyListPanesEvery10s`) citing [IDLE-1-S1] and [IDLE-1-S3], and with `fakeTmux` answering an error citing [IDLE-1-S4]
- [ ] 2.4 `runLoop`: a fixed-rate source that returns a wait longer than its tick waits that long on a timer, a kick or its watch still waking it, and a tick queued meanwhile causes no second poll. Tested in `framework/internal/panel/sources_test.go` with `pclock.NewFake` and `BlockUntil` (as `TestHookEndsTheQuietAgentsInterval`)
- [ ] 2.5 The `worktrees` poll returns `Dormant` when the project is dormant. Tested by running the `worktrees` source through `runLoop` with a counting runner: 30 fake minutes dormant (at most 7 `git worktree list`) citing [IDLE-1-S1], one minute in view (6) citing [IDLE-1-S3], and a `git worktree add` in a temp repo read within the watch tick citing [IDLE-2-S4], in `framework/internal/panel/idle_test.go`
- [ ] 2.6 A hook kicks a 5-minute `claude agents` wait at once, tested in `cost_test.go` (extend `TestHookEndsTheQuietAgentsInterval` to the dormant wait) citing [IDLE-2-S1]
- [ ] 2.7 `Runtime.pageInView` kicks `agents`, `worktrees`, `tmux` and `prs` as well as the merged read; a lane action's existing kick (`lanes.Changed`) ends dormancy. Tested in `idle_test.go` (a Runtime with its kick channels and a counting runner) citing [IDLE-2-S2] and [IDLE-2-S3]
- [ ] 2.8 Two runtimes on one fake clock and one `web.Server`, a lane in the first only: the second backs off, the first does not, tested in `idle_test.go` citing [IDLE-1-S2]

## 3. GitHub only when something reads it; auto-close catches every merge
- [ ] 3.1 The `prs` source (the open list) runs every `PRs` in view and spawns nothing out of view. Tested in `idle_test.go` with a counting runner and the fake clock citing [IDLE-3-S1] and [IDLE-3-S3]
- [ ] 3.2 `Ticks.MergedLanes` (3 min) in `DefaultTicks` and `withDefaults`. `pollAutoClose` keeps its `r.lanes != nil && r.trusted()` gate and its watched-lane filter; with a watched lane it runs `gh pr list --state merged --limit 20 --json number,headRefName,mergedAt` at most every `MergedLanes`, in view or not, and makes due a watched lane whose branch is in it under a number not yet checked for that lane; with none it runs nothing. A failed read makes nothing due. Tested in `idle_test.go` with lanes in and out of the filter, an untrusted config and a failing read
- [ ] 3.3 Auto-close with no page in view, and a pull request opened and merged between two open-list reads in view: each lane is closed, or asks, after the merged-list read. Tested in `framework/internal/panel/autoclose_integration_test.go` (a throwaway socket and a fake `gh`, as `TestAutoCloseOnMerge`, with `MergedLanes` shortened) citing [IDLE-3-S2] and [IDLE-3-S4]
- [ ] 3.4 The `--head` check asks `--json number,mergedAt`; each close and each ask writes "auto-close: lane <id>: PR #n merged <time>, closed <time> (<delay>)" (or "asked <time>: <why>") to the panel's output (D8), asserted on the output in `autoclose_integration_test.go` citing [IDLE-3-S5]

## 4. The login agent without a browser tab
- [ ] 4.1 `panel install --no-open` → `InstallOptions.NoOpen` → `plistSpec.NoOpen` → `--no-open` after `--launchd`; `install` prints which it installed. Tested in `framework/internal/panel/install/launchd_test.go` (as `TestPlistContent` and `TestInstallAndUninstallWithATempHome`) citing [IDLE-5-S1] and [IDLE-5-S2], and the flag's default in `framework/internal/cmd/cmd_test.go`'s `TestPanelFlagDefaults`
- [ ] 4.2 In `Run`, `NoOpen` wins under `--launchd`: no open and no `browser-opened`, tested in `framework/internal/panel/run_test.go` (as `TestLaunchdRunKeepsTheTokenOutOfTheLog`, recording `OpenBrowser`) citing [IDLE-5-S1]

## 5. Docs
- [ ] 5.1 `docs/panel.md`: the sources table, *What the panel reads, and when*, *Close a lane when its PR merges* (the merged-list read, its 20-PR limit, the log line), *The lane registry*, the `claude agents` cadence under *How signals are read*, *The launchd agent* (`--no-open`), and the observability footer (the spawn count, the hourly lines, and the starts it leaves out)

- [ ] Slice: an owner can leave the panel running at login with no page open, no browser tab and no lane, and read in its log that each idle project spawns at most 1.2 processes a minute
