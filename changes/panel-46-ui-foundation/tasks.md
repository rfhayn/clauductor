# Tasks: panel-46-ui-foundation

## Progress
- 2026-10-17 proposed; nothing built yet

## Decision log
- (none yet)

Each group's review reads the UX harness's screenshots of the views it changed (design D11):
the orchestrator runs `testdata/ux/shots.sh` outside the sandbox after the group's quick gate and
hands the PNG paths to the reviewer. A group whose shots could not be taken says so in Progress.

## 1. The modules (no visible change)
- [ ] 1.1 Before any move, run `shots.sh` on today's page (overview, a working lane, a lane that needs you, the project menu, New lane, Metrics, both modes of the default theme) and keep the PNGs as the baseline outside the repository; write `testdata/ux/shots.sh` for it (D11)
- [ ] 1.2 Move `panel.js` into `static/js/` modules by hand, one per area (D2): shared state in `js/store.js`, `render()` in the entry, no import called at a module's top level; `index.html` loads `/static/panel.js` as `type="module"` after the classic helpers, which stay as they are
- [ ] 1.3 Add `readScript(t)` (the entry, then `static/js/*.js` in name order) and switch the 24 `readWeb(t, "panel.js")` calls, `machine_account_test.go` and `state/blocked_test.go` to it; every assertion stays as it is
- [ ] 1.4 `TestModulesImportCleanly` in `web/modules_test.go` citing [PAGE-1-S1]: walks every import from the entry through the embedded files, checks `index.html`'s one module script and no inline script or import map, and fails on an imported binding called at a module's top level; run it first against a module with a bare specifier and one with a top-level call, and show both failing
- [ ] 1.5 `TestModulesNeedTheCookie` in `web/modules_test.go` citing [PAGE-1-S2]: a module with and without the cookie (200 `text/javascript` with `nosniff`; 401)
- [ ] 1.6 Run `shots.sh` again and compare with 1.1's baseline: no view differs, beyond the clock and ages. Record the comparison in Progress

## 2. Tokens, mode and the lane's look
- [ ] 2.1 Rewrite `TestPanelAvoidsTheTells` (`web/tells_test.go`) to ADR-0009's tells 1–9, with tell 10 left to groups 4 and 5. Run it against today's `panel.css` first and record the failures it names in the Decision log, citing [PAGE-3-S1]
- [ ] 2.2 `themes.css` holds ADR-0009's tokens for light and dark (and the terminal's Grey HMI `--term-*` and `--ansi-*`, D5), and nothing else. `panel.css` names only those tokens. `tuned.js`, `types.css`, the theme and type pickers and their tokens go (D4). Appearance shrinks to Mode and text size
- [ ] 2.3 Self-host JetBrains Mono: one variable Latin `woff2` and its OFL under `static/fonts/`, and the six type systems' faces removed (D5). `types_test.go` holds the one face, its licence and a byte budget. If the file cannot be fetched, stop for the owner (Refusals)
- [ ] 2.4 Rewrite `themes_test.go` for the one theme in two modes, citing [PAGE-3-S2]; add the no-literal-colour check citing [PAGE-3-S3]; rewrite `TestPanelCSSFadesNothingButDisabledControls` to ADR-0009's opacity rule citing [PAGE-3-S4]; remove `TestTunedMeetsTheFloorsForAnyInput` with `tuned.js` (D4)
- [ ] 2.5 `theme.js` keeps the mode and the text size and drops the theme, type and Tuned keys (D10), tested under node in `web/theme_mode_test.go` citing [PAGE-3-S5]
- [ ] 2.6 `js/states.js` maps each state to its look and word (D6); the lane table, the tabs and the Needs-you rows draw the dot and chip, tested under node in `web/states_test.go` citing [PAGE-2-S1], [PAGE-2-S2] and [PAGE-2-S3]
- [ ] 2.7 Shots of 1.1's views in both modes for the review

## 3. The shell: sidebar, top bar, body and Pulse
- [ ] 3.1 `index.html` and `js/sidebar.js`: the project menu at the top, Needs you cards and the alerts (CLAUDECALLS-5's words and **What to do** kept), the lane rows (dot, name, one figure; ⋯ on hover and when selected), the lost lane with Restore, Activity and Metrics, and at the foot Live, with every counter behind a click (FT-1, FT-2), and Mode. ⌘\ hides the sidebar. The rail's lane table, its column picker and sorting, and the worktree tree go from the page (the tree returns as 6.2's sheet)
- [ ] 3.2 `js/status.js`: the top bar's figures as `mockups/README.md`'s *Shared placement* lists them, with New lane its one primary button and the Gate figure opening the queue (SP-9). The banners and the connection bar go under it
- [ ] 3.3 The body routes between home and a lane (`?p=` and the selected lane kept per project, as today). Until groups 4 and 5, home is today's overview and a lane is today's workspace
- [ ] 3.4 `js/pulse.js` draws `SECTIONS`, Pinned then Flow (D12), tested under node in `web/pulse_test.go`, and the page searched for PANEL-45's sample rows, citing [PAGE-7-S1]
- [ ] 3.5 `js/layout.js`, `layout(width, height, folded)`: the sidebar's and Pulse's widths (252/268 px and 304/340 px from 1700 px), Pulse's fold below 1440 px and ⌘., the boxes' below 800 px high and ⌘J, four contributors below 960 px high. Tested under node in `web/layout_test.go` citing [PAGE-5-S2]. The 85-column rule, the rail's and side panel's drag edges and their stored widths go
- [ ] 3.6 Activity and Metrics open as sheets, with every tab, range and scope Metrics has today
- [ ] 3.7 Rewrite `TestLayoutFitsTheWindow` for the shell's selectors, keeping its rules: the page never scrolls, and the terminal is laid over its frame
- [ ] 3.8 Move `matrix.cjs`'s views to the shell; shots for the review

## 4. Home
- [ ] 4.1 `LaneManager.Tail(ctx, id, n)` in `lanes` (`capture-pane -p -J`, no `-e`; the four lines above the lowest box-drawing rule, else the last four with words), tested in `lanes/tail_test.go` against a real tmux on a private socket (skipped under `-short`), citing [PAGE-4-S3] and [PAGE-4-S4]
- [ ] 4.2 `GET /api/p/{project}/lanes/{id}/tail`, behind the cookie and the project, 404 for a lane the project's state does not hold (D7), tested in `web/tail_test.go` citing [PAGE-4-S5]
- [ ] 4.3 `js/home.js`, `homeGroups(lanes)`, tested under node in `web/home_test.go` citing [PAGE-4-S1] and [PAGE-4-S2]
- [ ] 4.4 Draw home: the spotlight with the one xterm and the ask in view; working tiles with the preview polled every 3 s only while home is shown and the page is visible, one request per lane in flight, "no preview" on a failure; compact waiting and idle tiles; the lost lane with Restore; the New lane tile. A click, or ↵ on a focused tile, opens the lane
- [ ] 4.5 Tell 10's home half in `TestPanelAvoidsTheTells`: home creates a terminal only for the spotlight, and a tile's preview is drawn with `textContent` from the tail
- [ ] 4.6 Home in `matrix.cjs` (one, two and no lanes asking); shots for the review

## 5. The stage
- [ ] 5.1 `js/stage.js`: the filmstrip of every live lane (scrolling, with a fade and no "N more" count), the terminal as the stage's first track, and one row of three boxes. It comes forward from 94% in 260 ms after a click, ↵ or ⌘K, and draws in its final state when loaded from its address. Esc, All lanes in the crumb, or a filmstrip tile leave it. The tab strip, its "N more" menu and the side panel go
- [ ] 5.2 [PAGE-5-S1] in the UX harness's flows (`flows.cjs`): a tile opens its lane, Escape returns home (manual: a browser's focus and key events, which no gate runs)
- [ ] 5.3 `js/boxes.js`: Context (the bar, the auto-compact tick, %, cache hit and cold, and `drawContributors` from `js/contributors.js`, which draws nothing, D12), Pull request (the readiness verdict, a row per check, the diff), Run (cost against budget amber from 80% and red past 100%, the agents now and finished, the 45 s strip, time; the figures and the Gantt behind a click). `contextBar(lane, threshold)` is tested under node in `web/boxes_test.go`, and the modules searched for a literal 85 or 95, citing [PAGE-5-S4]
- [ ] 5.4 The terminal's header: name, state chip, branch, model and effort, thinking, port, Remote, uptime, Interrupt, Attach and ⋯. `headerChips(lane, width)` is tested under node in `web/chips_test.go` citing [PAGE-5-S5]
- [ ] 5.5 A lane that asks while another is open: its filmstrip tile breathes, and it leads Needs you without being opened. [PAGE-5-S3] is checked in the UX harness's flows with the fake's `waiting` lane (manual: the breathe ring is an animation in a browser)
- [ ] 5.6 Tell 10's stage half: the terminal is the stage's first track, and no rule sets its width to 0
- [ ] 5.7 The stage in `matrix.cjs` (working, needs you, a long name, boxes folded, Pulse folded); shots for the review

## 6. The palette and the rest of the inventory
- [ ] 6.1 `js/palette.js`: a registry of commands, each with its words, key and the function its button or ⋯ item calls; ⌘K opens the palette (160 ms enter, scrim fade, caret in the accent), and the ⋯ menus are built from the same entries. Tested under node in `web/palette_test.go` citing [PAGE-6-S1] and [PAGE-6-S3]
- [ ] 6.2 Worktrees as a sheet from ⌘K: the tree, with New lane here and Remove (today's requests and plans)
- [ ] 6.3 The dialogs (New lane, Close lane, Remove, Help, Add a project…, Remove from panel…, Trust config…) restyled to the tokens; their requests, words and focus return unchanged (NEWLANE, ATTACH-4 and ATTACH-5 stay green)
- [ ] 6.4 Help lists the keys the page has now (⌘K, ⌘J, ⌘., ⌘\, Esc and today's), held by `TestHelpListsTheRealShortcuts`
- [ ] 6.5 `web/dropped_test.go` searches the page's files for the theme and type pickers, Tuned, the column picker, sorting and the side panel's edge, citing [PAGE-6-S2]
- [ ] 6.6 The palette, Worktrees and each dialog in `matrix.cjs`; shots for the review

## 7. The docs and the final look
- [ ] 7.1 `docs/panel.md`: *The page* and *Appearance* rewritten to the page as drawn (ADR-0009's layout, colour, motion, mode and type), every "grey at rest" line ADR-0009 lists replaced, and *What checks them* naming the tests as they now stand
- [ ] 7.2 ADR-0009's *Open for PANEL-46*: each question answered as the owner approved it (D3, D4, D5), and *Enforcement* naming the tests and `shots.sh`
- [ ] 7.3 `docs/ux-passes.md`: the areas, views and modes the harness now covers, and `shots.sh`
- [ ] 7.4 The full matrix at 1440×900 and 1920×1080 in both modes, with `findings.json` free of layout findings. The final review compares the shots with `mockups/shots/4-hybrid-*` and lists every difference with its reason
- [ ] 7.5 The roadmap's PANEL-46 row names the open places later rows fill (D12), and OPS-80 stays queued with its design reference

- [ ] Slice: an owner can open the panel and work from the hybrid's home: see every lane's state in colour and words, answer the lane that needs them in the spotlight, open any lane onto its stage with Context, Pull request and Run beneath its terminal, and reach every function through ⌘K, in light or dark
