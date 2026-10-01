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
| `layout-views` | 6 viewports (1280×800, 1440×900, 1920×1080, 2000×900, 2560×1440, 900×800) × 23 views: 138 shots (UX-2: 27 views with Checks, Metrics and the quiet, ready and budget lanes, ~162 shots, ~210 s; PANEL-22: 8 more, a project's ⋯ actions, Add a project… empty, refused, with the init preview and with the trust report, Remove from panel… refused and allowed, Trust config…, ~228 shots, ~280 s) | ~180 s |
| `layout-appearance` | every theme × light/dark, every type system, and the text sizes (85–175%) at 1280, 1920 and 900 px: 75 shots (PANEL-22: the menu open and the trust report in every theme × mode, the menu with every type, the menu and the init preview at the smallest size and from 150%: ~123 shots) | ~115 s |
| `flows-lanes` | start (from New lane, from New lane here), type and Ctrl+], interrupt, restart, stop, close, remove, forget, kill tmux and Restore all | ~60 s |
| `flows-attachments` | PNG and JPEG dropped, image pasted, non-image refused, selection survives the pointer, ⌘-click URL, OSC 8 asks | ~35 s |
| `flows-projects` | switching projects keeps each one's selection; a project that cannot load; PANEL-22: Remove from panel… refused on Alpha (its lanes listed, a lane's link shows it), Trust config… on Zeta, Delta added live (init preview, Create this config, its own socket written in, Trust and add, served at once, then removed: back on Alpha, its config kept), `panel add`/`panel remove` of Epsilon taken live | ~30 s |
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

`up.sh` creates three git repositories and registers them with `panel add` and `panel trust`
(and, since PANEL-22, three more for the project menu, below):

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
- **PANEL-22**, for Add a project…: **Delta** has no `panel.json` and is not registered (the init
  preview; `flows-projects` adds it live and removes it, writing socket `ux-<runid>-4` into the
  config it creates before trusting it); **Epsilon** has a config with a card, a template and a
  queue and is not registered (its trust report; `flows-projects` adds and removes it with the
  CLI); **Zeta** is registered and untrusted, with no lane (Trust config…, and a project that can
  be removed). `up.json` names Delta and Epsilon under `candidates`.

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
- **vscroll** / **footer-offscreen** / **footer-covered** (PANEL-21): the page scrolls down, or
  the footer is not on screen whole, or something paints over it. The page is the window at
  every size.
- **truncated-untitled** (PANEL-21): a footer counter is truncated and its tooltip does not hold
  its text.
- **rowact-cut** / **control-cut** (PANEL-21): a lane's ⋯ in the rail, or a button in the bars
  over the lanes (Needs you's Open terminal), is cut sideways. **railActs** also scrolls
  the rail to every ⋯ in the Lanes table and the Worktrees tree, at the rail's narrowest (160 px),
  its default and its widest, in every viewport, and checks each is whole and clickable
  (`rowact-cut`, `rowact-covered`; a shot `rail-<width>.png` of each).
- **tab-overflow** (PANEL-21): tabs are out of sight and no "N more" says how many, or it says a
  different number.
- **confirm-hidden** (PANEL-21): an inline confirmation under the terminal (Stop, Close lane,
  Forget, Resume, Remote control, a link) is not whole on screen, or a button of it is covered.
- **copy** (PANEL-21): text composed from parts doubles its punctuation ("close lane?: its").
- **flowcard** (PANEL-21): a Flow card row (side panel) does not show its label whole, or
  something in the card runs past the side panel. `layout-metrics` shoots Beta's card, whose
  spend has under a week kept (its ledger starts with the run), at 100% and 150%
  (`flow-card-short`, `flow-card-short-150`), and Beta's Metrics Cost tab
  (`metrics-short-spend`); each fails if the span line is not on the page.

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
   **Fixed in PANEL-21**: a binding made before the inputs caught up moves to the lane's own
   worktree once the list has it (from the registry record, or from the session's first `cwd`),
   and a lane action re-reads the worktrees without the throttle. With `UX_FAKE_EARLY=1` every
   lane now shows its own session (`docs/panel.md`, *How signals are read*, Binding).
4. **Without a `SessionEnd`, a stopped or closed lane lingers** (seen before the fake sent
   `SessionEnd`). A lane stopped or closed through the panel went on showing as a "≈" row, with
   no ⋯, and as a tab, while status posts or hooks from its session kept coming in the seconds
   after it ended. Real claude sends `SessionEnd` on `/exit`, but a crash or `SIGKILL` does not.
   A closed lane whose worktree is gone showed as a row too.
   **Fixed in PANEL-21**: once a running lane is gone from the tmux poll (stopped, closed,
   forgotten, killed, its pane dead), its session ends and whatever else arrives from it is set
   aside (`observe.droppedEnded`); it is never bound again by its `cwd`.
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

## Fixed in PANEL-21 (layout and copy)

PANEL-18 findings 1, 2, 5 and 6 and PANEL-19/20 findings 2 to 5 are fixed, each with a rule
above or a flow, and two Go tests (`TestLayoutFitsTheWindow`, `TestCopyNamesButtonsAsTheyRead`):

- **The page is the window.** `body` no longer scrolls at any width; Needs you and the banners
  sit in `#topbars`, which gives way to the lanes and scrolls; the shell keeps
  `min(16rem, 40vh)`. The terminal is laid over its frame (`position: absolute`), so the frame
  shrinks and refits it (sized by its content, it never shrank below its last fit). Below 1180 px
  the side panel takes at most 40% under the terminal and scrolls; below 900 px the rail does
  the same over the workspace. What still does not fit scrolls in `#wsbody`, and an inline
  confirmation scrolls itself into view once per change of its words.
- **A Needs-you or alert row's ask yields** its width and truncates, its text in its tooltip
  (at 1280 px and 175% a long ask pushed Open terminal out of the window).
- **The footer wraps**, and a counter too long for a line truncates with an ellipsis; each
  counter's tooltip, and the footer's, hold the full text.
- **"N more"** after the tab strip counts the tabs out of sight and opens a menu of them (a
  menu button: Down, Up, Home, End, Enter, Escape). The selected tab is scrolled into the strip
  when the selection changes, a tab changes width, or the strip resizes (flow
  `tab-overflow-keys`).
- **The Lanes table's ⋯ column sticks** to the rail's edge, and the Lane column yields and
  truncates (a cell's `max-width` is no limit in an auto table; `max-width: 0` with `width: 100%`
  is). The rail's panes never run wider than the rail. `up.sh` adds the lane
  `a-rather-long-lane-name-for-layout`.
- **Copy**: the orphan message names Resume, Forget and Close lane as the buttons read (and the
  exited message Restart and Stop lane); a Needs-you label that asks ("PR merged: close lane?")
  ends its sentence, and the reason is the next one.
- **The Flow card's rows are a fixed grid** (UX pass 2's `clip` at 1440×900, 110%: Beta's
  "$0.20 (since 2026-09-30, 1 day)" pushed the Spend label out and the sparklines 22 px past the
  side panel). The label keeps its width, the value truncates, the spark cell is 3rem and clips,
  and a span goes on a smaller line under the label and value; the Metrics view's value cell
  puts it on a line under the amount too (rule `flowcard`). The Economy field reads **on**, not
  "economy" again (`TestFieldValueDoesNotRepeatItsLabel`).
- A terminal shown again after its frame changed while hidden is refitted once it settles (it
  kept its old grid: `xterm-fit` at 2560 px after the rail widths changed).

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
