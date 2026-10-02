# Tasks: panel-25-idle-cost

## Progress
- 2026-10-02 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The panel counts what it spawns
- [ ] 1.1 A counting Runner wraps each runtime's `run` and the machine's (`Machine.run`), and `LaneManager` reports each tmux call through a `Spawned` callback set in `newLaneManager` (and on the `Exec` path tests use); counts are kept by command (`claude agents`, `git worktree`, `gh pr`, `tmux list-panes`, …) over a rolling 10 minutes on the panel's clock and published in each project's `Obs` (`spawnsPerMin`, `spawnsBy`) by `pollObs`, tested in `framework/internal/panel/spawns_test.go` (a fake runner and `pclock.NewFake`, as `dashboard_cost_test.go`) citing [IDLE-4-S1]
- [ ] 1.2 Reading `/api/state` does not mark a page in view (only `POST /api/seen` does): asserted in `framework/internal/panel/web/server_test.go` citing [IDLE-4-S1]
- [ ] 1.3 Under `--launchd`, a machine source writes "spawns in the last hour: N (claude agents a, git worktree b, tmux list-panes c, gh pr d, other e)" once an hour to the panel's output, tested in `spawns_test.go` with the fake clock citing [IDLE-4-S2]
- [ ] 1.4 The observability footer shows the rate and the top commands, next to the `claude agents` row (`panel.js`, the observability footer), wiring tested in `framework/internal/panel/web/spawns_footer_test.go` (the `readWeb` string test `rowactions_test.go` uses) citing [IDLE-4-S1]

## 2. A dormant project backs off, and wakes at once
- [ ] 2.1 `Ticks.Dormant` (5 min) in `DefaultTicks` and `withDefaults`; `agentsInterval` takes whether a page is in view and returns `Dormant` where it returns `AgentsQuiet` today and none is; `agentsQuietNow` is set for either wait; while dormant the `--cwd` cross-check waits. Tested in `framework/internal/panel/cost_test.go` (extend `TestAgentsCadence` and `TestAgentsFilterAndBackoff`) citing [IDLE-1-S1] and [IDLE-1-S3]
- [ ] 2.2 `tmuxPoller` returns `Dormant` where it returns `TmuxIdle` today and no page is in view, tested in `cost_test.go` with `tmuxPollerFor` and `runTicks` (as `TestTmuxIdleSocketSpawnsOnlyListPanesEvery10s`) citing [IDLE-1-S1] and [IDLE-1-S3]
- [ ] 2.3 `runLoop`: a fixed-rate source that returns a wait longer than its tick waits that long on a timer, a kick or its watch still waking it, and a tick queued meanwhile causes no second poll; the `worktrees` poll returns `Dormant` when the project is dormant. Tested in `framework/internal/panel/sources_test.go` with `pclock.NewFake` and `BlockUntil` (as `TestHookEndsTheQuietAgentsInterval`), and [IDLE-2-S4] against a temp git repo in `framework/internal/panel/idle_test.go`
- [ ] 2.4 A hook kicks a 5-minute `claude agents` wait at once, tested in `cost_test.go` (extend `TestHookEndsTheQuietAgentsInterval` to the dormant wait) citing [IDLE-2-S1]
- [ ] 2.5 `Runtime.pageInView` kicks `agents`, `worktrees`, `tmux` and `prs` as well as the merged read; a lane action's existing kick (`lanes.Changed`) ends dormancy. Tested in `idle_test.go` (a Runtime with its kick channels and a counting runner) citing [IDLE-2-S2] and [IDLE-2-S3]
- [ ] 2.6 Two runtimes on one fake clock and one `web.Server`, a lane in the first only: the second backs off, the first does not, tested in `idle_test.go` citing [IDLE-1-S2]

## 3. GitHub only when something reads it; auto-close preserved
- [ ] 3.1 The watched-lane filter `pollAutoClose` applies today (registered, on a branch, in its own worktree, `AutoCloseOf(type) == on_merge`) moves into one function both it and the `prs` poll call, unchanged, tested in `framework/internal/panel/idle_test.go` with lanes in and out of the filter
- [ ] 3.2 `Ticks.PRsOutOfView` (3 min); the `prs` poll runs every `PRs` in view, every `PRsOutOfView` out of view only with a watched lane, and spawns nothing otherwise. Tested in `idle_test.go` with a counting runner and the fake clock citing [IDLE-3-S1] and [IDLE-3-S3]
- [ ] 3.3 Auto-close with no page in view: a lane whose branch leaves the open list is closed, or asks, after the out-of-view poll, tested in `framework/internal/panel/autoclose_integration_test.go` (a throwaway socket and a fake `gh`, as `TestAutoCloseOnMerge`, with `PRsOutOfView` shortened) citing [IDLE-3-S2]

## 4. The login agent without a browser tab
- [ ] 4.1 `panel install --no-open` → `InstallOptions.NoOpen` → `plistSpec.NoOpen` → `--no-open` after `--launchd`; `install` prints which it installed. Tested in `framework/internal/panel/install/launchd_test.go` (as `TestPlistContent` and `TestInstallAndUninstallWithATempHome`) citing [IDLE-5-S1] and [IDLE-5-S2], and the flag's default in `framework/internal/cmd/cmd_test.go`'s `TestPanelFlagDefaults`
- [ ] 4.2 In `Run`, `NoOpen` wins under `--launchd`: no open and no `browser-opened`, tested in `framework/internal/panel/run_test.go` (as `TestLaunchdRunKeepsTheTokenOutOfTheLog`, recording `OpenBrowser`) citing [IDLE-5-S1]

## 5. Docs
- [ ] 5.1 `docs/panel.md`: the sources table, *What the panel reads, and when*, *Close a lane when its PR merges*, *The lane registry*, the `claude agents` cadence under *How signals are read*, and *The launchd agent* (`--no-open`); and the spawn count in the observability footer's description

- [ ] Slice: an owner can leave the panel running at login with no page open, no browser tab and no lane, and read in its log that it spawns at most 3 processes a minute
