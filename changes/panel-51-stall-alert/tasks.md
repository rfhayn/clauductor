# Tasks: panel-51-stall-alert

## Progress
- 2026-10-18 proposed; nothing built yet. The build starts from `main` once approved, on the
  owner's Mac (session 5), beside PANEL-49's build.
- 2026-10-18 pre-review (propose step 2b), for the proposal PR's body:
  Review: NOT converged after 3 round(s), peak high, open: F1 verify-change paths in tasks
  1.1/1.4/5.3 (high), F2 task 5.3 has no executor, F3 a waiting stall re-sent after a restart,
  F4 background-work stall lost on a restart, F5 STALL-1-S19 guards the wrong read (medium),
  F6 pending sources, F7 stall.json write rate, F8 wording (low) at 5a94976 · design c4683f6c5f18
  - Rounds: 1 peak high (16 findings), 2 peak high (13), 3 peak high (8). Rounds 1 and 2 were
    fixed in full. Round 3 was not, by the three-round limit.
  - Round 3's proposed fixes: F1 name only paths the build changes (or unbacktick `/hook`,
    `/loop`); F2 append the PANEL-46 and PANEL-25 tasks in this proposal's PR and drop 5.3
    (`cg_build_tasks` skips a PR touching only `changes/`); F3 save each waiting session's ask
    time in `stall.json`; F4 save each live session's decoded `Stop` allowlist there, or read
    (d) as unknown after a restart; F5 reword S19 to a stalled lane whose re-read fails; F6 list
    the project-level sources, the hooks by the install check, and keep a mark while its way is
    unjudged; F7 the lease's release as movement, `stall.json` written at most every N seconds;
    F8 align the requirement title, the lease wording and the proposal's `panel.js` line.

## Decision log

## 1. Probes on the owner's Mac (design E1 to E3, P1 to P4)
- [ ] 1.1 P1: on the Mac's Claude Code version, with the panel running, repeat E1 and E3 in a throwaway lane: a background `sleep` with the turn ended at once, then an AskUserQuestion. Record what `claude agents --json` reads at each step, and whether the `Stop` body the panel's `/hook` receives carries `background_tasks` and `session_crons` (a debug log of the decoded fields, never committed). Record the result in the Decision log. If the fields do not arrive through the async command hook, stop for the owner: D2 and D4 rest on them. (manual: a live Claude Code session on the owner's login, which no test can start)
- [ ] 1.2 P2: in the same lane, a turn that ends while a background subagent runs (an Agent call with `run_in_background`). Record what `claude agents` reads until the subagent ends, and what the `Stop` lists. If it reads `idle`, D3 (d) carries the case, and the Decision log says so. (manual: a live session)
- [ ] 1.3 P3: a teammate (an agent team in the lane, one teammate told to start a background `sleep` and end its turn). Record whether it is a session of its own in `claude agents` and in the hooks, and whether `TeammateIdle` fires and in whose session. If it is not a session of its own, stop for the owner before group 2 commits (D9). (manual: a live agent team)
- [ ] 1.4 P4: a one-shot `ScheduleWakeup` in a `/loop`. Record the `schedule` text its `Stop` lists, and whether the fire time is local time. If it is not five fields with the minute, hour, day and month as numbers, D4's one-shot rule is amended with the owner. (manual: a live session)

## 2. The rule (D1, D3, D4)
- [ ] 2.1 `framework/internal/panel/signals/hooks.go`: `HookEvent` declares `background_tasks` and `session_crons`, each into a declared type that holds D4's allowlist alone (`type`, `status`, `description`; `schedule`, `recurring`), absent kept apart from empty. Tested in `framework/internal/panel/signals/hooks_stop_test.go` over E1's two `Stop` bodies, an older body with neither field, and a body whose task carries `command`, `server`, `tool`, `name` and `agent_type`. Citing [STALL-2-S5].
- [ ] 2.2 `framework/internal/panel/notranscript_test.go`: its decoded-types check also walks the new types by reflection and fails on any field D4 forbids. Show it failing first on an added `command` field, and record that in the Decision log. Citing [STALL-2-S5].
- [ ] 2.3 `framework/internal/panel/state/stall.go`: per lane, from what the model holds:
  - working (D1), with a lease bound to the lane by its tmux id or a worktree basename unique in the project, a holder counting only while alive and not stale, and no lease read where the project declares no queue;
  - the last movement (D1: hooks, reading changes, a session first listed after the first successful poll, a bound lease taken or given up, the pane dying), the sessions of that first poll not counting, and its origin (the saved movement, else the panel's start);
  - each way of stalling judged only from the sources it rests on, once each has succeeded since start; the view's "stalls pending" flag until every source has succeeded once or 10 minutes have passed; and a warning naming a source not succeeded by then and what it leaves unjudged (D1);
  - a waiting session's stall on its own clock, whatever its lane's other sessions do (D1);
  - the live-session rule (D1): a lane with no live session stalls only on D3 (c);
  - open work (D3 (a) to (d)), and the wakeups still due, counted for live sessions only (D4);
  - the session keeps its last `Stop`'s tasks, crons and last line, clipped, until its next `Stop`, and nothing from a `SubagentStop` (D4);
  - `LaneView.Stall` (`since`, `waitedOn`, `openWork`, `approx`: D11) and the `stalled` alert (key `stalled:<lane path>`, severity `block`, `Since` the last movement: D6), every stall approximate while the `claude agents` source is not current;
  - the receipt and HEAD it reads are D10's (HEAD from the worktree list), the merged read's result and the saved movements arrive through `Apply` calls the runtime makes (group 3), and the model hands back the movements to save.
  - Tested in `framework/internal/panel/state/stall_test.go` with a fixed clock, citing [STALL-1-S1], [STALL-1-S2], [STALL-1-S3], [STALL-1-S4], [STALL-1-S5], [STALL-1-S6], [STALL-1-S7], [STALL-1-S9], [STALL-1-S10], [STALL-1-S11], [STALL-1-S12], [STALL-1-S13], [STALL-1-S14], [STALL-1-S15], [STALL-1-S16], [STALL-1-S17], [STALL-1-S18], [STALL-1-S19], [STALL-2-S1], [STALL-2-S2], [STALL-2-S3], [STALL-2-S4] and [STALL-2-S6].
- [ ] 2.4 `framework/internal/panel/state`: the Needs-you item of kind `stalled`, built once per lane after the session loop, as `mergeNeeds` (`state/lifecycle.go`) builds the `merged` rows; a waiting lane that stalls keeps its one waiting item, labelled stalled, with the quiet time and open work (D11). Tested in `state/stall_test.go`, citing [STALL-4-S2] and [STALL-4-S3].

## 3. The config and the reads without a page (D1, D5, D10)
- [ ] 3.1 `framework/internal/panel/config`: `alerts.stall_minutes` (default 30, 0 off) at config version 7, in `config.go`'s `AlertConfig` and `Thresholds` and in `fields.go`'s table; regenerate `docs/panel.schema.json` and the *Keys* table in `docs/panel.md` as `config/generated_test.go` requires. Tested in `framework/internal/panel/config/config_test.go` (the default, 0, a negative refused) and `config/version_test.go` (the key refused at version 6). Citing [STALL-1-S8].
- [ ] 3.2 `framework/internal/panel/readiness_source.go`: the receipt read runs whether or not a page is in view; `tasks.md` and the review threads stay page-only (D10). The merged read runs `mergedPR`'s `gh pr list --head <branch> --state merged` (`autoclose.go`) through a variant that returns its error, for each lane with a clean receipt for its HEAD and no open PR: only a merge found is cached, per branch and HEAD; "none" is read again when the branch leaves the open-PR list, which this source tracks itself from the view's `PRs`, and otherwise at most every 5 minutes; a failed read keeps the last result, and with none the receipt is unknown, never "none" (D10).
- [ ] 3.3 `framework/internal/panel/runtime.go`: save each lane's last movement to `~/.clauductor/panel/<project hash>/stall.json` (0600, written atomically when it changes, as `notifier.json` is) and hand it to the model at start (D1).
- [ ] 3.4 Tested in `framework/internal/panel/stall_integration_test.go`, a runtime with no page in view, a fake `gh` and a receipt written for a lane's HEAD:
  - `/api/state` shows the lane stalled, then not stalled once the fake `gh` reports the branch merged after its PR left the open list, and still not stalled when the next merged read fails;
  - an attached project whose sidecar sets `stall_minutes` 0 shows no stall;
  - a second runtime started on the same home after the first stops, with a fake `gh` that answers only after the notifier's first ticks, shows the same stall with the same `since`, its notifier sends nothing, and `notifier.json` keeps the stall's mark throughout. Show this case failing first with the pending flag ignored, and record that in the Decision log.
  - Citing [STALL-1-S5], [STALL-1-S8], [STALL-1-S13], [STALL-1-S19] and [STALL-3-S7].

## 4. The notification and the push (D6, D7)
- [ ] 4.1 `framework/internal/panel/state/notifier.go`: a `stalled` alert interrupts by the notifier's rules unchanged, but for one: while the view says stalls are pending (D1), `Process` keeps each `stalled:` mark it holds rather than deleting it as cleared. Each `Notice` carries its `stalled` alert apart, if it has one. `noticeLine` words a stall's line for the macOS notification. Tested in `framework/internal/panel/state/notifier_test.go`, citing [STALL-3-S2], [STALL-3-S3] and [STALL-3-S7].
- [ ] 4.2 `framework/internal/panel/push.go`:
  - read `~/.clauductor/panel/push.json` at start and when its signature changes (`pathSignature`), and refuse it by D7's rules (the file's owner and mode, its folder's mode, an absolute `argv[0]`, one JSON object) with a warning bar naming the file and why;
  - for each notice that carries a stall, run its argv with a title and body built from the `stalled` alert's own fields (D7), never from `Notice.Body`, with no shell and a 10 s timeout; a failure counts in the notifier's failures with its error;
  - the machine (`machine.go`) reads the file once for every project, and each project's `pollNotify` (`runtime.go`) runs it; the machine's own notifier (the quota, the login locks) never pushes.
  - Tested in `framework/internal/panel/push_test.go` with a stand-in program written by `internal/testbin` that records its arguments, a notice carrying a stall and a `waiting` alert among them. Citing [STALL-3-S1], [STALL-3-S4], [STALL-3-S5] and [STALL-3-S6].

## 5. The page, the docs and what PANEL-46 and PANEL-25 carry (D10, D11)
- [ ] 5.1 `framework/internal/panel/web/static/stall-view.js` (new), loaded in `web/index.html` before `panel.js` as `context-colour.js` is: from a lane's `stall` and its Needs-you item, the word "stalled", the row's tint class, and the Needs-you row's text (quiet time, open work, what it waited on), with Open terminal only where the lane has a terminal, and a waiting row labelled stalled. `panel.js` calls it from the lane's state cell, tab, tree node and Needs you; nothing else in `panel.js` changes (D11).
  - Tested in `framework/internal/panel/web/stall_view_test.go`, which loads `stall-view.js` under node through `vm`, as `web/context_colour_test.go` loads `context-colour.js`, over lanes from a `/api/state` fixture: a stalled lane with a terminal, one started by hand, and a waiting lane that stalled. Citing [STALL-4-S1].
  - And in `framework/internal/panel/testdata/browser/stalled-lane.cjs`, which `testdata/browser/run.sh` runs against a panel of its own, started after the shared one with its own HOME, project and `panel.json` at version 7 with `stall_minutes` 0.05 (3 s), so no other script sees a stalled lane. Its `SHOTS` screenshots go to the group's reviewer (OPS-84 is not merged yet). Citing [STALL-4-S1].
- [ ] 5.2 `docs/panel.md`: the `stalled` row in *Alerts*; the stall rule, what it reads and what it does not see in a new *Stalled lanes* section under *Alerts*; `push.json` in *Notifications* and in *Security model*, with an ntfy and a Pushover example; `stall.json` beside `notifier.json`; the two decoded `Stop` fields in *How signals are read*; "stalled" in *The page*'s lane rows and Needs you.
- [ ] 5.3 Open an `ops/` pull request, apart from this change's build PR (D11: rule 9 would otherwise hold this build to PANEL-46's and PANEL-25's open tasks), that appends a task to `changes/panel-46-ui-foundation/tasks.md`'s group 2 (the lane's look) carrying D11's contract (a stalled lane reads as *needs you*: the red dot and ring, a "stalled" chip, its place in Needs you, drawn from `stall-view.js` or its successor; tested by its tells and page tests), and one to `changes/panel-25-idle-cost/tasks.md`'s group 3 (the GitHub reads) keeping the receipt read, the stall's merged read (with its cache) and `gh pr list` running with no page in view while a lane has open work (D10). Neither `design.md` is touched. Name the PR in the Decision log; it merges through `merge-pr` like any ops PR.

- [ ] Slice: an owner away from the Mac gets one push on their phone when a lane on the panel at http://127.0.0.1:<port> has stalled with work open, and sees the lane marked stalled, with what it last waited on, when they open the page
