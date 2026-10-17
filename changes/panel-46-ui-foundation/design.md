# Design: panel-46-ui-foundation

What is built: the hybrid of ADR-0009, on today's data and today's server, with every function the
inventory lists (`changes/archive/2026-10-16-panel-45-ui-design/mockups/README.md`) where its
*Shared placement* and *Variant 4* sections put it. ADR-0009 is the rule. Where this design and
ADR-0009 disagree, ADR-0009 wins, except on the three questions it left open, which D3 to D5
settle. Line numbers are `panel.js` at `f16c9e8`. They move once PANEL-48 and PANEL-49 merge
(D15).

## Decisions

- **D1 One change in seven groups, built over two sessions (recommended).**
  - **Re-measured (`propose` step 0c):**
    - Write surfaces: `framework/internal/panel/web` (the page, its tests, one route), one
      read-only method in `framework/internal/panel/lanes` (D7), the UX harness
      (`framework/internal/panel/testdata/ux`), `docs/panel.md`, `docs/ux-passes.md` and
      ADR-0009's *Open for PANEL-46* (D3 to D5).
    - It modifies no living requirement and adds one capability, `panel-page`.
    - Most of the work is in one package, but it is more than one screen of change. The groups
      are cut so each is one builder's and one reviewer's worth, and every group leaves the page
      working:
      1. modules;
      2. tokens and states;
      3. the shell;
      4. home;
      5. the stage;
      6. the palette;
      7. the docs and the final look.
  - **Beat:** two rows, one for the modules and tokens on today's layout, one for the layout. The
    first would ship ADR-0009's colours on a layout ADR-0009 replaced: a page neither design
    describes, with `docs/panel.md` true of neither. One PR keeps code, tests and doc changing
    together (PANEL-45 D8).

- **D2 Native ES modules under `static/js/`, entered by `static/panel.js` (recommended).**
  - **The load:** `index.html` loads `/static/panel.js` as `<script type="module">`. Its imports
    are relative (`./js/lanes.js`), because the CSP (`script-src 'self'`, `server.go:303`)
    refuses an inline import map. `go:embed index.html static vendor` already embeds a new
    folder. `http.FileServerFS` serves `.js` as `text/javascript`, which a module needs under
    `nosniff`. The cookie rides a same-origin module fetch, so `requireAuth` is unchanged.
  - **The modules:** one per area of today's file, named for the section comments it already has.
    Among them: `conn`, `time`, `graphics`, `lanes` (and `states`, D6), `needs`, `status` (the
    top bar), `sidebar`, `home`, `stage`, `boxes`, `terminals`, `dialogs`, `palette`, `metrics`,
    `activity`, `projects`, `layout`, `pulse`.
  - **Shared state:** what the page holds today in top-level `let`s (`S`, `selKey`, `PID`, `P`,
    `conn`, …) moves into one exported object in `js/store.js`. An ES import binding cannot be
    assigned by its importer, so a setter per variable would multiply call sites.
  - **Import cycles:** `render()` lives in the entry. A module may call an import only inside a
    function, never at its own top level, so cycles between areas cannot run a function before
    its module has loaded. `TestModulesImportCleanly` holds this (PAGE-1-S1).
  - **The helpers stay classic scripts:** `theme.js`, `xterm-style.js`, `term-links.js`,
    `start-plan.js` and `project-config.js` load before the module, as globals, as today. Their
    node harnesses run each file in a `vm` context with a `window` (`project_config_test.go:14`).
    Converting them buys the owner nothing, and those harnesses would have to be rewritten.
  - **The tests that read `panel.js`:**
    - 24 calls in `web` and two outside it (`machine_account_test.go:198`,
      `state/blocked_test.go:308`) read the script as text.
    - They switch to `readScript(t)`: the entry, then every file under `static/js/` in name
      order. A test that counts occurrences (`server_test.go:390`, one `enterTerm`) still counts
      across the whole page.
  - **By hand:** `no-blind-source-rewrite.sh` refuses a scripted move of tracked source, so the
    builder moves code with Edit. Group 1 is the costliest per line for that reason (D14).
  - **Beat:**
    - a bundler or npm, which PRD-ui decision 2 rules out;
    - one module per `render*()` function: about 25 files, with cycles everywhere;
    - converting the helpers too.

- **D3 No component helper: the page keeps `el()` and patching in place (recommended).**
  - **What it keeps:** the page builds the DOM with `el()`, keyed nodes and `patch` (PANEL-6).
    Tests hold that it re-renders on no timer, assigns no `innerHTML`, and patches its live
    regions in place (`server_test.go:405-409`, `projectmenu_test.go:92`).
  - **What it costs:** the hybrid adds no interaction that `el()` cannot draw. The mockup was
    drawn without a helper.
  - **Beat:** vendoring Preact with htm, as PRD-ui decision 2 allows. Every render function would
    be rewritten into components. A virtual DOM re-renders a region instead of patching it, which
    those tests forbid. It would add a dependency to keep current, for about twice the build's
    cost.

- **D4 The accent is fixed at indigo; Tuned goes (recommended).**
  - **What goes:** `tuned.js`, the sliders and `TestTunedMeetsTheFloorsForAnyInput` go with the
    six themes.
  - **What holds the accent:** ADR-0009's floors in both modes (3:1 for focus and the accent, the
    accent ink at 4.5:1).
  - **Beat:** one tunable accent hue held to the same floors. It keeps a generator and its
    property test, plus a picker in ⌘K, for a choice the owner has not asked to make. The accent
    marks selection and data series only, never a state, so its hue carries no meaning to tune.

- **D5 The terminal face is self-hosted JetBrains Mono (recommended).**
  - **The faces:** one variable Latin `woff2` under `static/fonts/`, with its SIL OFL licence. The
    stack is JetBrains Mono, then SF Mono, then the system mono (ADR-0009). The six type systems'
    faces go, about 600 KB of the binary. The UI and data faces are the system sans and mono, as
    ADR-0009 sets.
  - **Why it matters:** xterm measures its cell from the face (`panel.js:2194`). A self-hosted
    face makes a lane's column count the same on every machine, and a lane's tmux window is sized
    from it.
  - **The ANSI palette:** the terminal keeps today's default theme's (Grey HMI's) `--term-*` and
    `--ansi-*` values, dark in both modes (ADR-0009, *The tokens*).
  - **Held by:** `types_test.go` shrinks to the one face, its licence and its byte budget.
  - **Beat:** taking the face from what is installed, as the mockup did. The cell would differ
    between a Mac with JetBrains Mono and one without.

- **D6 Each lane state the server sends maps to one of ADR-0009's five looks (recommended).**
  The mapping lives in one pure module, `js/states.js`, that every view reads.

  | Today (`laneState`, `panel.js:416`) | Look | Chip |
  |---|---|---|
  | `waiting` (a blocking ask: a permission, an elicitation, an agent's input; `docs/panel.md`, *How signals are read*) | needs you: red, filled | "needs you" |
  | `idle` with a finished turn in `S.done` (*Your move*) | waiting: amber, filled | "your move" |
  | `busy` | working: green, filled | "working" |
  | `idle` with no finished turn; `none` (starting, no session) | idle: hollow grey ring | "idle", "starting", "no session" |
  | `dead`, `orphaned`, `stale` | idle: hollow grey ring | "exited", "orphaned", "no hooks" |
  | a lane in the restore list (`S.orphan`, BN-4) | lost: dashed ring | "lost" |

  - **Where they sort:** an exited, orphaned or no-hooks lane sorts straight after needs you, as today
    (`STATE_ORDER`). Its alert, an amber mark in the sidebar, carries the problem.
  - **A Needs-you item that is not a lane's ask** (a `warn`, such as `quota_auto_resume_stale`)
    is a Needs-you card with an amber edge. It changes no lane's look: red is the blocking ask's.
  - **Beat:** drawing exited, orphaned and no hooks in red as failures. ADR-0009 keeps red at
    rest for needs you, and a red that is not an ask would send the owner to a lane with nothing
    to answer.

- **D7 A working tile's preview comes from a read-only tail endpoint (recommended).**
  - **The route:** `GET /api/p/{project}/lanes/{id}/tail`, behind `requireAuth` and
    `withProject`. It is registered under `/api/p/{project}` only, not the legacy `/api`
    (`routes.go:24`, TODO PANEL-19).
    - It answers 404 for a lane the project's state does not hold.
    - It calls one new method, `LaneManager.Tail(ctx, id, 4)`:
      `tmux capture-pane -p -J -t =<id>:`, with no `-e`, so it carries no escape sequences.
  - **The four lines:** the last four lines with words in them, above the pane's lowest line made
    only of box-drawing rules. That rule is Claude Code's prompt frame, so the preview shows no
    prompt and no cursor, as the mockup draws it (`4-hybrid.html:1008`). With no rule, it is the
    pane's last four lines with words in them.
  - **How it is drawn:** as text (`textContent`), never HTML.
  - **When the page asks:** every 3 s, only while home is shown and the page is visible, only for
    working lanes, one request per lane in flight. Opening a lane stops the polling.
  - **When it fails:** the tile says "no preview" and never shows stale lines as current.
  - **Beat:**
    - a terminal WebSocket per tile: each attaches a tmux client, and a client resizes the lane's
      window (*Window size: the latest client wins*);
    - the server polling every pane into `/api/state`: it costs the idle panel, against PANEL-25,
      while nobody looks.

- **D8 Home and the stage, laid out as ADR-0009 sets (recommended).**
  - **Home** (`js/home.js`), in order:
    1. the lane whose ask is oldest is the spotlight, the only xterm on home;
    2. any other lane that needs you leads the tiles with its red ring;
    3. working tiles, each with its preview (D7);
    4. waiting tiles, then idle tiles, both compact;
    5. the lost lane, with Restore;
    6. the New lane tile.

    `homeGroups(lanes)` is a pure function.
  - **A lane opened** (`js/stage.js`): the filmstrip of every live lane, the terminal as the
    stage's first track, then one row of three boxes.
  - **Where the side panel's tabs go** (`SIDE_TABS`, `panel.js:1147`):
    - Agents → **Run**;
    - Figures → behind a click on Run;
    - Git and Checks → **Pull request**;
    - Gate → the top bar's Gate figure (SP-9);
    - Alerts → the sidebar's alerts, filtered to the lane;
    - Activity → the Activity sheet, filtered to the lane.
  - **The Context box draws PANEL-48's measures, never a constant:** the lane's context
    percentage, its auto-compact tick (per lane, LANECTX-2-S5), and the warning colour from
    `alerts.context_pct` (LANECTX-2-S4). `contextBar(lane, threshold)` is pure. No module
    carries the literal `85` or `95` for context.
  - **The terminal's header** gives its chips way whole, from the end: uptime, the port, Remote,
    then thinking. The model and effort stay. `headerChips(lane, width)` is pure.
  - **The folds:** ⌘J folds the boxes and ⌘. folds Pulse. Pulse folds by itself below 1440 px
    wide and the boxes below 800 px high, and Context lists four contributors, not five, below
    960 px high. `layout(width, height, folded)` is pure. This replaces today's
    85-column rule (`TERM_COLS`, `panel.js:3434`) and the rail and side panel's drag edges.
  - **A lane that needs you while another is open** is not opened for you: its filmstrip tile
    breathes red, and it leads Needs you (ADR-0009).
  - **Beat:** keeping the tabbed side panel beside the terminal. ADR-0009 sets no tabs and puts
    the boxes beneath it.

- **D9 The ⌘K palette is built here, over a registry of commands (recommended).**
  - **The registry:** `js/palette.js` holds one entry per command: its words, its key and the
    function its button calls. The palette, the ⋯ menus and the buttons run the same function,
    so one request has one call site.
  - **What it holds:** every inventory function the shared placement puts behind ⌘K:
    - the lane actions (Restart…, Stop lane…, Close lane…, Remote control…, Interrupt);
    - **Worktrees**, a sheet of the tree, with New lane here and Remove;
    - Activity, Metrics and Help;
    - Mode and text size;
    - New lane, with *Up next* first;
    - the project switch.
  - **Beat:** a row of its own for the palette, keeping today's buttons meanwhile. The hybrid
    places Worktrees, text size and the lane menu's items behind ⌘K, so without it they have no
    home on the new page.

- **D10 A stored appearance migrates to a mode (recommended).**
  - **What `theme.js` keeps:** the mode (System, Light, Dark) and the text size, under their
    current `localStorage` keys. It ignores the theme, type and Tuned keys, and removes them on
    the first write.
  - **What the browser sees:** a page with a stored Glass Cockpit dark opens in Dark, with no
    error and no flash (`theme.js` still runs before the first paint).
  - **Beat:** resetting every stored choice to System, which loses the one choice that still
    exists.

- **D11 Eyes in the loop: screenshots in each UI group's review here, and OPS-80 for later rows
  (recommended).**
  - **The harness:**
    - group 1 adds `testdata/ux/shots.sh`: `up.sh`, `matrix.cjs` over the views a group names,
      at 1440×900 and 1920×1080 in light and dark, then `down.sh`;
    - each group moves `matrix.cjs`'s views to the page it leaves;
    - the `appearance` area becomes the two modes;
    - its layout audit (horizontal scroll, clipped text, overlaps, the terminal's fit) stays.
  - **Who runs it:** the orchestrator, outside the sandbox (it needs a loopback port, tmux and
    Chromium), as PANEL-42's 5.3 did. The PNG paths go to the group's reviewer.
  - **For later UI rows:** the mechanism is a new row, **OPS-80**, which this proposal queues. It
    gives `build-change` a project review step that runs `shots.sh` when a group's diff touches
    `framework/internal/panel/web`, and hands the reviewer the shots. Until OPS-80 merges, each
    UI row's `tasks.md` names its shots task: **Nothing. You.**
  - **The harness is not a gate step.** PRD-ui left that open. A run takes minutes and needs
    Chromium on every gate.
  - **Beat:**
    - a gate step;
    - building OPS-80's step here, which widens this change into the template and the plugin.

- **D12 Later rows plug into named places, and none is drawn before its row (recommended).**

  | Row | Its place |
  |---|---|
  | PANEL-50 | the Context box's contributors list: `js/contributors.js` exports `drawContributors(box, lane)`, which draws nothing until PANEL-50 fills it; the box reserves no height for it |
  | PANEL-37 | a routines section in Pulse: `js/pulse.js` draws the sections in its `SECTIONS` list (Pinned and Flow here); PANEL-37 adds one |
  | PANEL-38 | Today and the next 14 days: two more `SECTIONS` entries |
  | PANEL-31 | the version field at the sidebar's foot, beside Live; until then BN-3's notice shows under the top bar, restyled |
  | PANEL-30 | the Worktrees sheet's Remove (D9), whose request is today's |
  | PANEL-25 | the counters behind a click on Live (FT-1, FT-2) and `POST /api/seen`, unchanged |
  | PANEL-34 | Close's plan dialog, unchanged |

  - **No sample data ships:** nothing marked as a sample in PANEL-45's mockups appears on the
    page.
  - **Beat:** empty placeholders or the mockups' samples, which show the owner something that
    does not work.

- **D13 What tests the page (recommended).**
  - **Go tests read the files:** the tells, the floors, the module graph and the registry
    (`TEST_GLOBS="*_test.go"`).
  - **Node runs the pure modules:** `states`, `home`, `layout`, `boxes`, `chips` and the palette
    registry run from Go, as `start-plan.js` is run today:
    - a temp folder with `package.json` `{"type":"module"}` and a copy of `static/js/`;
    - skipped under `-short` or without node.
  - **Behaviour in a browser** (an open, a fold, a breathe) is checked in the UX harness's flows
    and escaped `(manual: …)` in `tasks.md`, because no gate runs a browser.
  - **The existing tests:**
    - `tells_test.go`, `themes_test.go`, `types_test.go` and `TestLayoutFitsTheWindow` are
      rewritten to ADR-0009;
    - `TestPanelAvoidsTheTells` is first run against today's `panel.css` and shown failing;
    - `TestPageEntersTheTerminalOnlyOnPurpose`'s list of live regions (`server_test.go:407`,
      today's `needs`, `lanelist`, `tree`, `tabs`, `side` …) becomes the new page's regions, so
      "patch in place" still holds for every region the page has;
    - every other web test keeps its assertion and reads through `readScript` (D2).
  - **Beat:** a JavaScript test runner, which needs npm.

- **D14 Risk normal; budget $120, soft (recommended).**
  - **Risk:** normal. It runs the opus/high builder and reviewer.
  - **The budget:** from the actuals: PANEL-42 cost $70.87 over five groups, about $14 a group;
    PANEL-45 cost $48.22; PANEL-48 is heading to about $90.
    - Seven groups at $14 is about $100.
    - Group 1's moves by hand and the screenshot reviews add to it.
    - So $120: the top of the owner's range.
  - **Soft:** reaching it is reported to the owner and the build goes on, as PANEL-48's is.
  - **Beat:**
    - high risk: the xhigh builder on seven groups would take the cost past the owner's range,
      while a regression here is caught by the existing tests and the screenshots;
    - a hard budget, which would stop a half-moved page mid-group.

- **D15 Built after PANEL-48 and PANEL-49 merge (recommended).**
  - **Why:**
    - PANEL-48 changes the context bar's tick and the warning colour (`panel.js:1118`, `:1401`).
    - PANEL-49 changes the quota note.
    - Both are approved, and both come first in the queue.
  - **What it does:** this change moves their code into the modules and keeps their behaviour
    (D8, PAGE-5-S4). The build branch is cut from `main` after both are merged, and the row's
    Deps name them.
  - **Beat:** building in parallel and merging over them, which would re-apply their `panel.js`
    hunks by hand into files that did not exist when they were written.

## Refusals
| Situation | What happens instead |
|---|---|
| Playwright or Chromium cannot run where the orchestrator is | The group's Progress line says so. Review judges the diff and the pure-module tests. The owner's look at the end is the only visual check, and the group is not called converged on visuals |
| The JetBrains Mono file cannot be fetched (network policy) | The build stops for the owner before group 2 commits. The stack falls back to the installed mono only by the owner's word, recorded in the Decision log, since it reverses D5 |
| `tmux capture-pane` fails or the lane has gone | The tail endpoint answers 404 or 502 with a reason. The tile says "no preview", and nothing is retried faster than the 3 s poll |
| node is not installed | The pure-module tests skip, as today's node harnesses do. CI's Ubuntu runner has node, so the PR's run is where they run |
| A function in the inventory has no home on the new page | The group stops and names it. Only the owner drops a function (PANEL-45 D6) |
| A tell cannot hold on the mockup's own CSS | ADR-0009 wins. The CSS changes, never the tell, unless the owner amends the ADR |
| The budget is reached | Soft: reported to the owner, and the build goes on |
| PANEL-48 or PANEL-49 has not merged when this row comes up | The build does not start. The session reports the dependency |
