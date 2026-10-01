# UX passes: the panel's UI/UX harness

`framework/internal/panel/testdata/ux/` (UX-1) starts throwaway panels with realistic data and
drives them with Playwright. It takes screenshots over a matrix of viewports, appearances and
views, checks each shot against layout rules, and runs scripted end-to-end flows. Several runs
can go at once, because each one has its own run id, port, temp `HOME` and tmux sockets
(`ux-<runid>-<n>`). It never touches `~/.claude`, `~/.clauductor`, launchd or any other tmux
socket. It uses synthetic data only.

It is not part of `go test`: it needs node, Playwright with Chromium, tmux, git, curl and perl.

## Run it

```sh
export PLAYWRIGHT=/path/to/node_modules/playwright     # or NODE_PATH, so require("playwright") works
cd framework/internal/panel/testdata/ux
./run-parallel.sh                          # every shard at once; prints the report's path
./run-parallel.sh --shards flows-lanes --run me1 --ports 4750-4759 --out /tmp/ux-me
```

`run-parallel.sh` builds the checkout once (or uses `$CLAUDUCTOR_BIN`). It then runs each shard
on a panel of its own, and writes `<out>/report.md`, `<out>/findings.json`, and each shard's
`shots/`. Two people or agents running it at the same time need different `--run` ids and
`--ports` ranges. The default range is 4700–4799.

| Shard | What | Time on PANEL-18 (M-series Mac, all five at once) |
|---|---|---|
| `layout-views` | 6 viewports (1280×800, 1440×900, 1920×1080, 2000×900, 2560×1440, 900×800) × 23 views: 138 shots (UX-2: 27 views with Checks, Metrics and the quiet, ready and budget lanes, ~162 shots, ~210 s) | ~180 s |
| `layout-appearance` | every theme × light/dark, every type system, and the text sizes (85–175%) at 1280, 1920 and 900 px: 75 shots | ~115 s |
| `flows-lanes` | start (from New lane, from New lane here), type and Ctrl+], interrupt, restart, stop, close, remove, forget, kill tmux and Restore all | ~60 s |
| `flows-attachments` | PNG and JPEG dropped, image pasted, non-image refused, selection survives the pointer, ⌘-click URL, OSC 8 asks | ~35 s |
| `flows-projects` | switching projects keeps each one's selection; a project that cannot load | ~20 s |
| `layout-metrics` (UX-2) | once the signals are up (≤ 1 min: the spend ledger is written every minute): 13 PANEL-19/20 views (Needs you with approval/budget/stale, economy badge, Flow card, Budget bar over and amber, Port and Remote in the header, Checks not ready/ready/root, the stale lane's Alerts, ⋯ with Remote control and its idle and busy confirmations, Metrics from the Flow card) at 1280, 1440, 1920 and 900 px; the Metrics view's 4 tabs × 3 ranges × 2 scopes at 1440 and 900 px: ~100 shots | ~160 s |
| `flows-metrics` (UX-2) | Metrics tabs, ranges and scope (values from the command, the all-projects error, focus back on Escape); Beta's bad payload (the error names the path, the built-in spend still draws); the Flow card opens Metrics; economy on/off with hysteresis (quota 42 → 38 stays on → 36 off → 39 stays off → 41 on, `economy.json` 0600); readiness turns Not ready on a failed check and back; ports in the state, header, tmux environment and setup marker; a new lane's `.worktreeinclude` copy, setup marker and start note, and Close's teardown; Remote control asks, types only after Confirm into an idle lane, and offers no Confirm on a busy one; the approval, budget and stale rows and the Budget bars | ~75 s |
| `flows-lifecycle` (UX-2) | auto-resume: a `StopFailure` rate_limit in an idle lane with the 5-hour window resetting 20 s later is typed `continue` + Enter once, 30 s after the reset; auto-close: the ship PRs leave the open list merged at their tips, the clean lane closes as Close lane would (worktree, branch, registry, teardown), the dirty one asks "PR merged: close lane?" | ~120 s |

Each run takes about 35 s to set up (most of it waiting for the panel to re-read the stale
lane's backdated registry record). With all eight shards in parallel, the whole pass takes about
4 minutes (243 s on ux/integration: the slowest shard, `layout-views`). `UX_MATRIX_ARGS=--full` adds every theme × type × mode
combination (72 more shots).

The pieces also run alone:

```sh
./up.sh r1 4710 > /tmp/r1.json    # one JSON line: base, token, home, sockets, projects, lanes…
node matrix.cjs /tmp/r1.json --areas views --viewports 1440x900 --out /tmp/m
node flows.cjs /tmp/r1.json --flows attachments --only drop-png --out /tmp/f
./down.sh r1                      # --keep leaves the temp dir (panel.log, HOME) to read
```

The flows act: they start and close lanes and kill the run's tmux server. Give them a run of their
own. `matrix.cjs` only opens things and cancels them.

## What a run holds

`up.sh` creates three git repositories and registers them with `panel add` and `panel trust`:

- **Alpha** (the default project) has a bare `origin`. Its `main` is 2 commits behind, so the
  cards say they may be stale. It has pinned cards, a template with Up next and a queue, and these
  lanes: `working` (busy, with subagents starting and stopping), `waiting` (a permission prompt,
  so it is in Needs you), `idle` (the project root), `stream` (busy, printing a log line every
  0.4 s), `merged` (its PR is merged, so Close lane removes its branch) and `orphan` (its tmux
  session killed). There are also two worktrees with no lane: a clean detached one and a dirty
  one. A `budget` lane starts only if the build shows budgets (PANEL-19 does).
- **Alpha, for PANEL-19/20** (UX-2; config version 5):
  - `metrics.command` is the metrics package's fixture (`metrics/testdata/metrics.sh`), and the
    fake `gh` lists 6 merged PRs over 30 days for the built-in figures;
  - `changes/` (untracked, in `.git/info/exclude`): `add-group-card`'s proposal has no
    Approved line and is 3 days old (**approval_wait**); `budget`'s budget is $20 and the
    `budget` lane spends $48.25 (**budget**, a red Budget bar); `working`'s is $3.50 against
    $3.10 (an amber bar);
  - `quiet`: its last commit is 5 days old and its registry record is backdated 5 days, so it
    is **stale** (`stale_days` 3);
  - `ready`: a green, approved PR (#44) with resolved review threads, every task in its
    `tasks.md` ticked and a clean `ci-receipt` for HEAD in its git dir, so its **Checks** tab
    says Ready to merge; `working`'s PR has a pending check, a review required and one of two
    threads unresolved (the fake `gh api graphql`);
  - `quota_economy.five_hour_pct` 40 against the fake's 42%, so **economy** is on (the badge
    names the roles of `.claude/model-roles.json`); `economy.json` is written under the temp
    HOME;
  - `remote-control.json` in the temp HOME says `lanes`: every lane starts with
    `--remote-control` and its **⋯** has **Remote control**;
  - `ports` 39100 + 10 per lane (nothing listens there); `.worktreeinclude` names the
    gitignored `.env.local` (and the tracked `README.md`, which must never be copied);
    `worktree_setup` writes `.ux-setup` (gitignored) in the new worktree and a line to
    `$HOME/fake/setup.log`, `worktree_teardown` a line to `$HOME/fake/teardown.log`, each
    `<lane> <port>`;
  - `lanes_auto_close` `on_merge` for the lane type `ship` only (`build` and `fix` say `off`, so
    `merged` stays for Close lane's flow), and `quota_auto_resume` on.
  - With `UX_LIFECYCLE=1` (the `flows-lifecycle` shard only) it also starts `shipclean` and
    `shipdirty` (type `ship`, PRs #45 and #46 open; `shipdirty` has an untracked file) and
    `limited` (idle, for auto-resume).
- **Beta** has one lane and one card, and a metrics command that prints a payload breaking the
  contract (`METRICS_FIXTURE=bad`).
- **Gamma**'s `panel.json` is broken after it is added, so the project menu shows it as a project
  that cannot load.

A fake `claude` answers `--version` (2.1.284), `agents --json` (each live session, with its
role's status), `auth status --json` (Max) and interactive mode. In interactive mode it draws a
TUI-ish screen that asks for mouse modes 1000/1002/1003/1006 and bracketed paste, and prints a URL
and two OSC 8 links. It records the bytes typed into it in `$HOME/typed.log` (and
`$HOME/fake/typed/<lane>.log`), and it posts hooks and status lines (5-hour quota at 42%, 7-day
at 61%). On `/exit` or a hangup it sends `SessionEnd`. It records each start's argv in
`$HOME/fake/argv/<lane>`. A flow moves the quota by writing `$HOME/fake/five_hour_pct` (a number)
or `$HOME/fake/five_hour_resets` (unix seconds). A fake `gh` answers `pr list` with PRs and
checks, a merged PR for `change/merged` (and whatever branch `$HOME/fake/gh/alpha.merged` lists),
the merged-PR list the Metrics view reads, and `api graphql` review threads per PR number; it logs
every call to `$HOME/fake/gh.log`.

## The layout rules (every shot)

`lib.cjs` `layoutAudit`:

- **hscroll**: the page scrolls sideways.
- **clip**: an element's own text overflows a box that hides it without an ellipsis.
- **overlap**: two controls intersect (only the parts that are visible, and not a menu or dialog
  over the page).
- **covered**: something that is not a control sits over the centre of a control.
- **offscreen**: a control runs past the window's side.
- **selected-hidden**: a selected tab is scrolled out of its strip.
- **xterm-fill** / **xterm-fit**: the terminal does not fill its host within 2 px (one cell of
  slack), or its cols/rows differ from what a fit gives. Both must hold for 1.5 s, so a refit
  that is still in progress does not count.
- **console**: page errors and console errors.
- **focus-ring**: checked once per viewport and per theme × mode. Tabbing through the first stops,
  each focused control must draw an outline or a box-shadow.

Flows run the same rules on the state they leave behind: more lanes, result bars and restore
bars. Each finding records its selector, viewport, theme, mode, type, size and screenshot. The
report groups repeats.

## Findings on PANEL-18 (bb63d02)

The harness surfaced these. None is fixed here.

1. **The footer hides behind the terminal below 1180 px** (the automated rule `covered`, seen in
   every 900×800 shot). Once the side panel moves under the terminal, `#wsbody` (938 px tall)
   overflows `#shell` (352 px). The footer (`#obs`: hook counters, **All counters**) is laid out
   at y≈561, in the middle of the terminal, and the terminal paints over it. A faint band shows
   through the terminal, and **All counters** cannot be clicked.
2. **The footer clips its counters at 900 px and 175%** (`clip`). "notifications 0" runs 59 px
   past the footer, with no ellipsis and no wrap.
3. **A session can be bound to the project root for good** (seen through the API, and reproduced
   with `UX_FAKE_EARLY=1 ./up.sh …`). If a lane's claude shows in `claude agents` (or sends a
   hook) before the panel's worktree list has the lane's new worktree, `bindLane` matches the
   record's path to the enclosing worktree (the root). `sess` binds a session once, so every
   such lane's session, status, context and subagents then show on the root lane, and the lanes
   themselves show "none". The worktree list's kick is throttled to 2 s, and the agents poll is
   kicked right after a start. A real claude that registers within about 2 s of its lane
   starting (likely when several lanes start together) could hit this. The fake waits 4 s by
   default, as claude takes seconds to start.
4. **Without a `SessionEnd`, a stopped or closed lane lingers** (seen before the fake sent
   `SessionEnd`). A lane stopped or closed through the panel went on showing as a "≈" row, with
   no ⋯, and as a tab, while status posts or hooks from its session kept coming in the seconds
   after it ended. Real claude sends `SessionEnd` on `/exit`, but a crash or `SIGKILL` does not.
   A closed lane whose worktree is gone showed as a row too.
5. **Copy**: the orphan message says "RESUME … FORGET … CLOSE LANE" in capitals, but its buttons
   read Resume, Forget and Close lane.
6. **The tab strip at 175%** (from reading the screenshots): at 1280 px the last tab is cut
   mid-word ("worki") with no fade or overflow hint. The selected tab stays in view (the
   `selected-hidden` rule holds).

All 19 flows passed on PANEL-18. These all work: starting lanes, typing and Ctrl+], Interrupt,
Restart, Stop, Close, Remove, Forget, Restore all, images dropped and pasted (typed as a
bracketed paste with no Enter, the file 0600 and outside every worktree), the non-image refusal,
the selection surviving the pointer, ⌘-click and OSC 8 links, the per-project selection, and the
Gamma row. Every view had no console errors, no page-level horizontal scroll, no overlapping
controls and no focus ring missing.

## Findings on PANEL-19/20 (ux/integration, UX-2)

None is fixed here. Two rules are new: `unclickable` (a flow could not click a control a person
would need; it then presses it as the keyboard would, so the flow goes on) and
`feature-missing` (a signal the views wait for never showed).

1. **Teardown runs without `CLAUDUCTOR_PORT`** (flows `worktreeinclude-setup-teardown` and
   `auto-close-on-merge`). docs/panel.md says setup and teardown run with `CLAUDUCTOR_LANE` and
   `CLAUDUCTOR_PORT` set, but `teardown.log` reads `ux-incl ` with no port. `Close`
   (`lanes/close.go`) stops and forgets the lane (its registry record, which holds the port)
   before `runHook` asks `LanePort(id)`, so the port is 0 and `hookArgv` leaves it out. Setup
   gets it (`.ux-setup` reads `<lane> <port>`).
2. **Close lane's confirmation can sit under the footer at 1440×900** (`covered`,
   `unclickable`; flow `close`, view `close-confirm`). With the Needs-you rows, the Alerts and
   the restore bar up, and the confirmation's new teardown note, **Confirm close** and
   **Cancel** fall at y≈863–889, under the fixed footer (`#obs`): a person cannot click them.
   It is the PANEL-18 footer finding again, now at a common laptop size. Seen in two of three
   full runs (it depends on which bars are up when the confirmation opens).
3. **The tab strip does not follow the selection** (`selected-hidden`). Picking **Remote
   control** (or any item) in a lane's **⋯** selects that lane, but its tab stays scrolled out
   of the strip (`working` shows 0 of 121 px at 1280, 1440 and 900 px); the same after Stop,
   Forget, Close or Restore all moves the selection to another lane (`budget`).
4. **⋯ under the footer at 900 px** (`covered`): the Lanes table's **⋯** of the lowest rows
   (`idle`, `merged`) are under `#obs`, so Remote control cannot be reached by pointer there
   (the PANEL-18 footer finding, new controls).
5. **Copy**: the auto-close ask reads "PR merged: close lane?: its pull request #77 merged; …"
   (a "?:" pair) in Needs you.
6. **Metrics, All projects** (from the screenshots): By model mixes the project command's
   names (`opus`, `sonnet`) with the panel's display names from Beta (`Opus 4.1`), so one model
   shows as two rows.

Everything else in the PANEL-19/20 flows passed: the Metrics view (values per range from the
command, the all-projects error, focus back on Escape), Beta's bad payload (the error names
`windows.30d.flow.change_fail_rate.value`, the built-in spend still draws), the Flow card,
economy with its hysteresis (on at 40, still on at 38, off at 36 with the documented reason,
still off at 39, on at 41; `economy.json` 0600 under the temp HOME), merge readiness (Ready to
merge, Not ready on a failed check, back again; working's pending check, required review,
unresolved thread and missing receipt; nothing to merge on the root), ports (unique, in the
header, the tmux environment and the setup marker), `.worktreeinclude` (the ignored file copied,
"copied 1 file(s)" in the start note), Remote control (`--remote-control` before `-n`; asks
first, types `/remote-control` + Enter only after Confirm into an idle lane; no Confirm on a
busy one), the approval, budget and stale rows and the Budget bars, auto-resume (`continue` +
Enter once, 30 s after the reset, not before) and auto-close (the clean merged lane closed with
its worktree, branch and record; the dirty one asks and keeps both).

Harness bugs fixed in UX-2: `up.sh` read `curl | grep -q` under `pipefail`, so the build's
budgets always read as missing (grep's early exit is curl's SIGPIPE); a standalone `up.sh`
built with HOME already the temp HOME, so go's read-only module cache landed there and
`down.sh` could not remove the run (it now builds first, and `down.sh` makes the tree writable
and fails loudly if anything is left).

## Limitations

- The fake claude is a shell script, not claude's TUI. Layout findings inside the terminal (how
  claude redraws on a resize) are out of scope; the rules check the terminal's box and grid only.
- The Metrics view and budgets are feature-detected: a build without them (PANEL-18) skips
  those views.
- The review threads are read at most every 2 minutes per PR, so no flow changes them; the
  readiness flow turns the verdict with a check instead.
- The fake does not send `UserPromptSubmit` for typed text, so after auto-resume types
  `continue` the lane's rate-limit alert stays (a real claude's prompt clears it).
- The dirty lane's auto-close ask is not followed to a close: the panel looks again only every
  5 minutes while it asks. The side panel's tabs are skipped where the layout folds the side panel (1280 px).
- The rules are heuristics. `clip` skips ellipsis truncation, and `overlap` and `covered` skip
  floating layers, so a real problem inside an open menu or dialog needs the screenshot.
- Drag-and-drop and paste are synthetic DOM events (`DataTransfer`, `ClipboardEvent`), not OS
  drags. ⌘-click popups are captured, and the linked sites are answered by a stub so nothing
  leaves the machine.
- Only Chromium. Screenshots are the viewport, not the full page (the page may scroll down).
- The flows share one run in order: `restore-all` kills the server, so it runs last.
