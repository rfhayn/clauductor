# Tasks: panel-46-ui-foundation

## Progress
- 2026-10-17 proposed; nothing built yet

## Decision log
- (none yet)

Built through `/apply-change`, group by group (design D11). Tasks marked *(the session)* are the
orchestrating session's: it runs them outside the sandbox between the group's quick gate and its
reviewer, writes the paths of the shots' folder (and of `moved.sh`'s output file, both outside
the repository) into the group's Progress line, which
the reviewer reads with the change's files, and ticks them once review converges. The builder
leaves them. Each group's inventory IDs are design D16's.

## 1. The modules (no visible change)
- [ ] 1.1 *(the session)* Before any move, run `shots.sh` on today's page and keep the PNGs outside the repository as the baseline: the overview, a working lane, a lane that needs you, the project menu, New lane and Metrics, in both modes of the default theme, at 1440×900. Group 1 writes `testdata/ux/shots.sh` first (D11)
- [ ] 1.2 Rewrite the UX harness's `page.evaluate` calls that read page globals (`matrix.cjs:514,526`; `flows.cjs:97,396,469`) to drive the page through clicks, keys and the DOM (D11)
- [ ] 1.3 Move `panel.js` into `static/js/` modules by hand (D2):
  - shared state in `js/store.js`;
  - `render()` and the `init()` calls in the entry;
  - no imported call and no browser global at a module's top level;
  - the pure modules import only pure modules;
  - `index.html` loads `/static/panel.js` as `type="module"` after the classic helpers, which stay as they are.
- [ ] 1.4 `testdata/ux/moved.sh` (D2): the base's `panel.js` split into top-level statements (declarations, and every other top-level statement: listeners, timers, observers, `try` blocks that restore saved state, loops, assignments, calls and bare blocks); on the modules' side `import` lines and the `export ` keyword dropped and the `store.` prefix normalised; a declaration matched by name and any other statement by its text, at top level or inside an `init()`; printed are only a changed declaration, a base statement found nowhere or more than once, and what is new. Each one it prints is explained in the Decision log
- [ ] 1.5 The `/static/` handler sets `Content-Type: text/javascript; charset=utf-8` on `.js` itself (D2)
- [ ] 1.6 Add `readScript(t)` and switch every `readWeb(t, "panel.js")` call in `web` to it; give `machine_account_test.go` and `state/blocked_test.go` the same few lines; update `startdialog_test.go:95`, `project_config_test.go:109` and `terminal_test.go:128` as D13's table says. Every other assertion stays
- [ ] 1.7 `TestModulesImportCleanly` in `web/modules_test.go`, citing [PAGE-1-S1]:
  - it walks every import from the entry through the embedded files;
  - it checks `index.html`'s one module script, and that it has no inline script or import map;
  - it fails on a top-level imported call, a top-level browser global, or a pure module importing an impure one;
  - it passes a top-level function that calls an import when called.

  Run it against a module with a bare specifier, one with a top-level call and one reading `document` at load, and show each failing.
- [ ] 1.8 `TestModulesNeedTheCookie` in `web/modules_test.go`, citing [PAGE-1-S2]: a module with and without the cookie returns 200 with `text/javascript; charset=utf-8` and `nosniff`, and 401
- [ ] 1.9 *(the session)* Run `moved.sh` and `shots.sh` again and give the reviewer both. The shots match 1.1's baseline in every view, beyond the clock and ages. Record the comparison in Progress

## 2. Tokens, motion and the lane's look
- [ ] 2.1 Rewrite `TestPanelAvoidsTheTells` (`web/tells_test.go`) to ADR-0009's tells 1–9, with tell 10 left to groups 5 and 6, skipping only the selectors in `legacySelectors`: every selector a later group replaces (D17).
  - Copy today's `panel.css` to `web/testdata/panel-css-before.css`.
  - Assert the tells pass on the page's CSS and fail on that copy, naming each tell it breaks, citing [PAGE-3-S1].
- [ ] 2.2 `themes.css` holds ADR-0009's tokens for light and dark, and nothing else:
  - the project hues as tokens;
  - `--term-bg` and `--term-head` from ADR-0009;
  - Grey HMI's `--term-fg`, `--term-cursor`, `--term-selection` and `--ansi-*` (D5).

  `panel.css`, legacy selectors included, names only those tokens. `tuned.js`, `types.css`, the theme and type pickers and their tokens go (D4). Appearance shrinks to Mode and text size (MN-5)
- [ ] 2.3 The motion of ADR-0009 as keyframes and tokens (D17):
  - `ping` at 2 s and 1.4 s;
  - `breathe`, `breatheT` and `breatheS` at 2.8 s;
  - `enter` and `fade` at 160 ms;
  - `forward` at 260 ms;
  - the 200 ms chip crossfade;
  - all of it off under `prefers-reduced-motion`.

  The changed-figure flash stays (PG-7)
- [ ] 2.4 Self-host JetBrains Mono: one variable Latin `woff2` and its OFL under `static/fonts/`, and the six type systems' faces removed (D5). `types_test.go` holds the one face, its licence and a byte budget. If the file cannot be fetched, stop for the owner (Refusals)
- [ ] 2.5 Rewrite `themes_test.go` for the one theme in two modes, the terminal's foreground on ADR-0009's background included, citing [PAGE-3-S2]. Add the no-literal-colour check, citing [PAGE-3-S3]. Rewrite `TestPanelCSSFadesNothingButDisabledControls` to ADR-0009's opacity rule, citing [PAGE-3-S4]. Remove `TestTunedMeetsTheFloorsForAnyInput` with `tuned.js` (D4)
- [ ] 2.6 `theme.js` keeps the mode and the text size and drops the theme, type and Tuned keys (D10). Tested under node in `web/theme_mode_test.go`, citing [PAGE-3-S5]
- [ ] 2.7 `js/states.js` maps each lane to its look and word in D6's order. Today's lane table, tabs and Needs-you rows draw the dot and chip. Tested under node in `web/states_test.go`, citing [PAGE-2-S1], [PAGE-2-S2], [PAGE-2-S3] and [PAGE-2-S4]
- [ ] 2.8 *(the session)* Shots of 1.1's views in both modes for the review

## 3. The sidebar and the top bar
- [ ] 3.1 `index.html` and `js/sidebar.js`, from top to foot:
  - the project menu (MN-1 to MN-4, BN-2, SB-2's badges), whose button takes the 8 px control radius (`projectmenu_test.go`, D13);
  - *Other projects*, the projects that need you elsewhere (SB-2);
  - Needs you cards and the alerts (NY-1 to NY-7; CLAUDECALLS-5's words and **What to do** kept; SP-10 filtered to the lane when one is selected);
  - the lane rows (RAIL-1, RAIL-3, RAIL-5: dot, name, one figure; RAIL-8's ⋯ on hover and when selected, its menu restyled; RAIL-12's fade);
  - the lost lane with Restore all, keeping BN-4's quota-guard override, its result line and **Dismiss**, and its ⋯ with Resume, Forget and Close lane (BN-4, WS-25; D9);
  - Activity and Metrics (SB-17, SB-18);
  - at the foot: Live, with every counter behind a click (PG-4, PG-5, FT-1, FT-2), and Mode.

  The sidebar's edge drags, or moves with ←/→ and Home/End when focused, keeps its width per browser, resets on a double-click or Escape, and hides the sidebar on Enter (RAIL-14, K-10, D8). The rail's lane table, its column picker and its sorting go, and the Needs-you rows, the rail, the project menu and the lane row's ⋯ menu leave `legacySelectors`. The worktree tree returns as 7.2's sheet
- [ ] 3.2 `js/status.js`, the top bar:
  - the figures as Variant 1's table places them, which variant 4 inherits (SB-1 to SB-22, less SB-2 and SB-10), with SB-13 to SB-15 behind its ⋯ and SB-22's give-way rule;
  - New lane as its one primary button (SB-16, WS-2);
  - the Gate figure opening the queue (SP-9);
  - the banners (BN-1, BN-3) and the connection bar under it;
  - BN-5's toast at the bottom left, with **Dismiss**.

  The status bar and the banners leave `legacySelectors`
- [ ] 3.3 *(the session)* Move `matrix.cjs`'s views to the sidebar and top bar; shots at 1440×900 and 1920×1080 in both modes for the review

## 4. The body, Pulse, the layout and the sheets
- [ ] 4.1 The body routes between home and a lane, with `?p=` and the selected lane kept per project as today (PG-6, PG-9). Until groups 5 and 6, home is today's overview and a lane is today's workspace
- [ ] 4.2 `js/pulse.js` draws `pulse-sections.js`'s `SECTIONS` in order, Flow then Pinned (SP-12 to SP-15, D12). The order is tested under node in `web/pulse_test.go`, with the page's files searched for PANEL-45's sample rows, citing [PAGE-7-S1]
- [ ] 4.3 `js/layout.js`, `layout(width, height, folded, sidebarW)` (PG-11, D8):
  - the sidebar's and Pulse's widths: 252 px and 304 px, then 268 px and 340 px from 1700 px wide, or the sidebar's dragged width;
  - Pulse's fold below 1440 px wide, and by its key;
  - the boxes' fold below 800 px high, and by their key;
  - four contributors below 960 px high.

  Tested under node in `web/layout_test.go`, citing [PAGE-5-S2]. The 85-column rule and the side panel's drag edge go, with their stored widths and their CSS
- [ ] 4.4 Activity (RAIL-4's two-hour timeline at its top; SP-11 filtered to the lane) and Metrics (every tab, range and scope it has today) open as sheets (DR-1, DR-2). The drawers leave `legacySelectors`
- [ ] 4.5 Rewrite `TestLayoutFitsTheWindow` for the new selectors, keeping its rules (D13)
- [ ] 4.6 *(the session)* Shots of the body and the sheets at 1280×800, 1440×900, 1920×1080 and the narrow 900×800, in both modes, for the review

## 5. Home
- [ ] 5.1 `LaneManager.Tail(ctx, id, n)` in `lanes` (D7):
  - `capture-pane -p -J`, with no `-e`;
  - the trailing blank rows dropped, then the four non-blank lines above the frame's upper rule: among the last 40 lines, the second rule from the bottom only when the first non-blank line after it starts with `>` or `❯`; otherwise the lowest rule; with no rule, no cut.

  Tested in `lanes/tail_test.go` against a real tmux on a private socket, citing [PAGE-4-S3], [PAGE-4-S4] and [PAGE-4-S7]. The test skips under `-short`; the group runs it without `-short`, outside the sandbox, and records the result in Progress
- [ ] 5.2 Record a capture of a real Claude Code lane's pane (working, with text typed at its prompt) in `lanes/testdata/`. Test the cut on it in `lanes/tail_test.go` (pure, no tmux), citing [PAGE-4-S6]
- [ ] 5.3 `GET /api/p/{project}/lanes/{id}/tail`, behind the cookie and the project, answering 404 for a lane the project's state does not hold (D7). Tested in `web/tail_test.go`, citing [PAGE-4-S5]
- [ ] 5.4 `js/home-groups.js`, `homeGroups(lanes)` in D8's order, which `js/home.js` draws. Tested under node in `web/home_test.go`, citing [PAGE-4-S1] and [PAGE-4-S2]
- [ ] 5.5 Draw home:
  - the spotlight with the one xterm (the lane's own, refitted, D8) and the ask in view;
  - working tiles with the preview, polled every 3 s only while home is shown and the page is visible, one request per lane in flight, and "no preview" on a failure;
  - compact waiting and idle tiles;
  - the lost lane with Restore all;
  - the New lane tile;
  - above them, the heading that counts the lanes by state (SB-10).

  A click, or ↵ on a focused tile, opens the lane. A tile rises 1 px on hover
- [ ] 5.6 Tell 10's home half in `TestPanelAvoidsTheTells`: home creates a terminal only for the spotlight, and a tile's preview is drawn with `textContent` from the tail. The terminal's empty state (WS-28, SP-17) stays in `legacySelectors`, since a lane started outside the panel and a blocked start still show it on the stage; it leaves with 6.1 (D17)
- [ ] 5.7 *(the session)* Home in `matrix.cjs` (one, two and no lanes asking), with shots at 1440×900 and 1920×1080 in both modes for the review

## 6. The stage
- [ ] 6.1 `js/stage.js`:
  - the filmstrip of every live lane (WS-1, WS-3; it scrolls, with a fade and no "N more" count; ←/→ move in it, K-4);
  - the terminal as the stage's first track, with its foot line (WS-27) and its empty state (WS-28, SP-17);
  - one row of three boxes.

  It comes forward from 94% in 260 ms after a click, ↵ or ⌘K, and draws in its final state when loaded from its address. Esc outside the terminal, All lanes in the crumb, or a filmstrip tile leave it. The tab strip, its "N more" menu (MN-7) and the side panel go, with their CSS and their `legacySelectors` entries
- [ ] 6.2 [PAGE-5-S1] in the UX harness's flows (`flows.cjs`): a tile opens its lane, and Escape returns home (manual: a browser's focus and key events, which no gate runs)
- [ ] 6.3 `js/boxes.js` (WS-9 to WS-11, SP-2 to SP-8, RAIL-7):
  - **Context:** the bar, the tick, the %, cache hit and cold, and `drawContributors` from `js/contributors.js`, which draws nothing (D12);
  - **Pull request:** the readiness verdict, a row per check, the diff;
  - **Run:** cost against budget, amber from 80% and red past 100%; the agents now and finished, and the 45 s strip; the time. The figures and the Gantt are behind a click.

  `box-views.js`'s `contextView(lane, state, decide)` calls PANEL-48's context-colour function (D8), and `boxes.js` draws. It is tested under node in `web/boxes_test.go`, with PANEL-48's file run in a `vm` context with a `window` and its function passed in as `decide`, citing [PAGE-5-S4]
- [ ] 6.4 The terminal's header (WS-4 to WS-8, WS-14, WS-20 to WS-26): name, state chip, branch, the ↵ to type hint, model and effort, thinking, port, Remote, uptime, Interrupt, Attach and ⋯. `headerChips(lane, width)` is tested under node in `web/chips_test.go`, citing [PAGE-5-S5]
- [ ] 6.5 A lane that asks while another is open: `js/stage-route.js`'s route keeps the open lane, and the lane that asks leads Needs you with its filmstrip tile marked to breathe. Tested under node in `web/stage_test.go`, citing [PAGE-5-S3]. The breathe itself is checked in the UX harness's flows with the fake's `waiting` lane (manual: an animation in a browser)
- [ ] 6.6 State changes in motion: a chip crossfades in 200 ms, and a lane moves to its state's place in every list (PG-7, D17)
- [ ] 6.7 Tell 10's stage half: the terminal is the stage's first track, and no rule sets its width to 0
- [ ] 6.8 The tab strip, the workspace, the terminal's empty state and the side panel leave `legacySelectors` (D17)
- [ ] 6.9 *(the session)* The stage in `matrix.cjs` (working, needs you, a long name, boxes folded, Pulse folded), with shots at 1366×900, 1440×900, 1920×780 and 1920×1080 in both modes for the review

## 7. The palette and the rest of the inventory
- [ ] 7.1 `js/commands.js` and `js/actions.js` (D9):
  - every command's id, words, key and when it applies;
  - `runCommand(id)`, the one module that sends a lane action's request;
  - the ⋯ menus, the buttons and the palette dispatch by id.

  ⌘K opens the palette, with a 160 ms enter, the scrim's fade and the caret in the accent. Every Ctrl+Alt handler, K-5's text-size keys included, ignores an event with AltGraph held (D9). Tested under node in `web/palette_test.go`, citing [PAGE-6-S1], [PAGE-6-S3] and [PAGE-6-S4]. A Go test in the same file checks that no module but `actions.js` sends a lane action's request
- [ ] 7.2 Worktrees as a sheet from ⌘K (RAIL-9 to RAIL-11): the tree, with New lane here and Remove, using today's requests and plans
- [ ] 7.3 Refresh (SB-19) and hide the sidebar (RAIL-13) in the registry
- [ ] 7.4 The dialogs (DLG-1 to DLG-5: New lane, Close lane, Remove, Help, Add a project…, Remove from panel…, Trust config…) are restyled to the tokens. Their requests, words and focus return are unchanged, so NEWLANE, ATTACH-4 and ATTACH-5 stay green. The menus and the dialogs leave `legacySelectors`
- [ ] 7.5 Help lists the keys the page has now (D9's, and K-1 to K-13), held by `TestHelpListsTheRealShortcuts`
- [ ] 7.6 `web/dropped_test.go` searches the page's files for the theme and type pickers, Tuned, the column picker, sorting and the side panel's edge, citing [PAGE-6-S2]
- [ ] 7.7 `legacySelectors` is empty, and `TestPanelAvoidsTheTells` asserts that it stays so (D17)
- [ ] 7.8 *(the session)* The palette, Worktrees and each dialog in `matrix.cjs`, with shots at 1440×900 in both modes for the review

## 8. The docs and the final look
- [ ] 8.1 Rewrite `docs/panel.md`'s *The page* and *Appearance* to the page as drawn: ADR-0009's layout, colour, motion, mode and type, and D9's keys. Replace every "grey at rest" line ADR-0009 lists. *What checks them* names the tests as they now stand
- [ ] 8.2 ADR-0009's *Open for PANEL-46*: answer each question as the owner approved it (D3, D4, D5). *Enforcement* names the tests, `shots.sh` and OPS-84
- [ ] 8.3 `docs/ux-passes.md`: the areas, views, sizes and modes the harness now covers, `shots.sh` and `moved.sh`
- [ ] 8.4 Walk all 135 inventory functions against the page, and write where each is (or the owner's drop) in `docs/ux-passes.md`. An unplaced one stops the group (Refusals)
- [ ] 8.5 *(the session)* The final matrix:
  - views, at 1280×800, 1366×900, 1440×900, 1920×780, 1920×1080 and 900×800;
  - the `sizes` area (85–175%);
  - both modes.

  `findings.json` holds no layout finding. The final review compares the shots with `mockups/shots/4-hybrid-*` and lists every difference with its reason

- [ ] Slice: an owner can open the panel and work from the hybrid's home: see every lane's state in colour and words, answer the lane that needs them in the spotlight, open any lane onto its stage with Context, Pull request and Run beneath its terminal, and reach every function through ⌘K, in light or dark
