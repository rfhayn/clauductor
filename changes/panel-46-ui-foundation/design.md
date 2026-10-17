# Design: panel-46-ui-foundation

What is built: the hybrid of ADR-0009, on today's data and today's server, with every function the
inventory lists (`changes/archive/2026-10-16-panel-45-ui-design/mockups/README.md`) where its
*Shared placement* and *Variant 4* sections put it (D16). ADR-0009 is the rule. Where this design
and ADR-0009 disagree, ADR-0009 wins, except on the three questions it left open, which D3 to D5
settle. Line numbers are `panel.js` at `f16c9e8`. They move once PANEL-48 and PANEL-49 merge
(D15).

## Decisions

- **D1 One change in eight groups, built over two sessions (recommended).**
  - **Re-measured (`propose` step 0c):**
    - Write surfaces: `framework/internal/panel/web` (the page, its tests, one route), one
      read-only method in `framework/internal/panel/lanes` (D7), two tests that read the page in
      `framework/internal/panel` and `framework/internal/panel/state`, the UX harness
      (`framework/internal/panel/testdata/ux`), `docs/panel.md`, `docs/ux-passes.md` and
      ADR-0009's *Open for PANEL-46* (D3 to D5).
    - It modifies no living requirement and adds one capability, `panel-page`.
    - Most of the work is in one package, but it is more than one screen of change. The groups
      are cut so each is one builder's and one reviewer's worth, and every group leaves the page
      working:
      1. the modules;
      2. tokens, motion and states;
      3. the sidebar and the top bar;
      4. the body, Pulse, the layout and the sheets;
      5. home;
      6. the stage;
      7. the palette;
      8. the docs and the final look.
  - **Beat:** two rows, one for the modules and tokens on today's layout, one for the layout. The
    first would ship ADR-0009's colours on a layout ADR-0009 replaced: a page neither design
    describes, with `docs/panel.md` true of neither. One PR keeps code, tests and doc changing
    together (PANEL-45 D8).

- **D2 Native ES modules under `static/js/`, entered by `static/panel.js` (recommended).**
  - **The load:** `index.html` loads `/static/panel.js` as `<script type="module">`, after the
    classic helpers. Its imports are relative (`./js/lanes.js`), because the CSP
    (`script-src 'self'`, `server.go:303`) refuses an inline import map. `go:embed index.html
    static vendor` already embeds a new folder.
  - **The content type:** the `/static/` handler sets `Content-Type: text/javascript;
    charset=utf-8` on every `.js` itself. Left to `http.FileServerFS`, Go's `mime` package lets a
    system table override it (macOS's `/etc/apache2/mime.types` maps `js` to
    `application/javascript`), so the type would depend on the machine. A module needs a
    JavaScript type under `nosniff`. The cookie rides a same-origin module fetch, so
    `requireAuth` is unchanged.
  - **The modules:** one per area of today's file, named for the section comments it already has.
    Among them: `conn`, `time`, `graphics`, `lanes`, `needs`, `status` (the top bar), `sidebar`,
    `home`, `stage`, `boxes`, `terminals`, `dialogs`, `actions` and `palette` (D9), `metrics`,
    `activity`, `projects`, `pulse`, and the pure ones below.
  - **Shared state:** what the page holds today in top-level `let`s (`S`, `selKey`, `PID`, `P`,
    `conn`, …) moves into one exported object in `js/store.js`. An ES import binding cannot be
    assigned by its importer, so a setter per variable would multiply call sites.
  - **Two rules for every module, so imports and node both work:**
    1. A module calls an imported binding only inside a function, never at its own top level, so
       a cycle between areas cannot run a function before its module has loaded. Defining a
       function at top level that calls an import (`const f = () => g()`) is allowed.
    2. A module touches no browser global (`document`, `window`, `location`, `localStorage`,
       `navigator`, `matchMedia`, `ResizeObserver`) at its top level. What today runs at load
       (`STYLE_NONCE`, `PID` from `location.search`, the listeners, `new ResizeObserver`) moves
       into each module's `init()`. The entry calls them in order, then connects.
  - **Pure modules decide; other modules draw.** The decisions node tests live in their own files,
    which draw nothing and import only one another:
    - `states.js` (D6);
    - `home-groups.js` (`homeGroups`, D8);
    - `layout.js`;
    - `box-views.js` (`contextView` and the PR and Run rows);
    - `chips.js`;
    - `stage-route.js` (what stays opened, Needs you's order);
    - `pulse-sections.js` (`SECTIONS`);
    - `commands.js` (D9).

    The modules that draw (`home`, `boxes`, `pulse`, `stage`, `palette`, …) import them along
    with `graphics` (`el()`) and `store`. `TestModulesImportCleanly` holds both rules, and that a
    pure module imports only pure modules (PAGE-1-S1).
  - **The classic helpers stay classic:** `theme.js`, `xterm-style.js`, `term-links.js`,
    `start-plan.js`, `project-config.js`, and PANEL-48's context-colour file (its task 4.2, D15)
    load before the module, as globals. Their node harnesses run each file in a `vm` context with
    a `window` (`project_config_test.go:14`). Converting them buys the owner nothing, and those
    harnesses would have to be rewritten.
  - **The tests that read `panel.js` as text:**
    - every `readWeb(t, "panel.js")` call in `web` (23 at `f16c9e8`, more once PANEL-48 and
      PANEL-49 merge) switches to `readScript(t)`: the entry, then every file under
      `static/js/` in name order;
    - `machine_account_test.go:198` (package `panel`) and `state/blocked_test.go:308` (package
      `state`) get the same few lines of their own, since a test helper does not cross packages;
    - a test that counts occurrences (`server_test.go:390`, one `enterTerm`) still counts across
      the whole page.
  - **By hand, and how review sees it:**
    - `no-blind-source-rewrite.sh` refuses a scripted move of tracked source, so the builder
      moves code with Edit. Group 1 is the costliest per line for that reason (D14).
    - The reviewer is not asked to read 4,151 moved lines. Group 1 commits
      `testdata/ux/moved.sh`, which compares by declaration, not by position:
      1. it splits the base's `panel.js` and the modules into top-level declarations (a
         `function`, `const`, `let` or `class` and its body);
      2. it drops `import` lines and only the `export ` keyword, and normalises the `store.`
         prefix;
      3. it matches declarations by name, whatever their file or order;
      4. it prints only what differs: a declaration whose text changed, one that is new (each
         `init()`, the store's fields), and one that is gone.
    - The session runs it and records its output with the group's Progress line, beside
      `git diff --color-moved` as the view of order. A declaration it prints that the Decision
      log does not explain is a finding.
  - **Beat:**
    - a bundler or npm, which PRD-ui decision 2 rules out;
    - one module per `render*()` function: about 25 files, with cycles everywhere;
    - converting the helpers too.

- **D3 No component helper: the page keeps `el()` and patching in place (recommended).**
  - **What it keeps:** the page builds the DOM with `el()`, keyed nodes and `patch` (PANEL-6).
    Tests hold that it re-renders on no timer, assigns no `innerHTML`, and patches its live
    regions in place (`server_test.go:403-409`, `projectmenu_test.go:91`).
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
  - **The terminal's colours:** `--term-bg` and `--term-head` are ADR-0009's values in each mode.
    The foreground, the cursor, the selection and the ANSI palette (`--term-fg`,
    `--term-cursor`, `--term-selection`, `--ansi-*`) are today's default theme's (Grey HMI's),
    which ADR-0009 leaves the terminal. The 7:1 floor is checked on ADR-0009's background.
  - **Held by:** `types_test.go` shrinks to the one face, its licence and its byte budget.
  - **Beat:** taking the face from what is installed, as the mockup did. The cell would differ
    between a Mac with JetBrains Mono and one without.

- **D6 Each lane maps to one of ADR-0009's five looks, by one function (recommended).**
  The mapping lives in one pure module, `js/states.js`, that every view reads. The rows are
  checked in this order, and the first that matches wins:

  | The lane (today's fields; `laneState`, `panel.js:416`) | Look | Chip |
  |---|---|---|
  | its terminal is in the restore list (`t.restorable`, `S.restorable`) | lost: dashed ring | "lost" |
  | status `waiting` (a blocking ask: a permission, an elicitation, an agent's input), or a `block`-severity Needs-you item names it (a template's first prompt not taken) | needs you: red, filled | "needs you" |
  | status `busy` | working: green, filled | "working" |
  | `dead`, `orphaned` (not restorable), `stale` | idle: hollow grey ring | "exited", "orphaned", "no hooks" |
  | idle, with a finished turn in `S.done` (*Your move*) | waiting: amber, filled | "your move" |
  | idle with no finished turn; `none` (starting, no session) | idle: hollow grey ring | "idle", "starting", "no session" |

  - **Where they sort:** in the sidebar's lanes: needs you, then exited, orphaned and no hooks (as
    today, `STATE_ORDER`), then working, waiting and idle. Their alert, an amber mark in the
    sidebar, carries the problem. On home they are the first of the compact idle tiles (D8).
  - **A `warn` Needs-you item** (`quota_auto_resume_stale`) is a Needs-you card with an amber
    edge. It changes no lane's look: red is the blocking ask's.
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
  - **Where it cuts:** Claude Code draws its prompt as a frame at the pane's foot: a rule, the
    prompt line(s), a rule, then a status line or two.
    - First the blank rows below the content go. A pane that is not full yet (a freshly started
      lane, drawn inline) ends in them.
    - A rule is a line made only of `─` (U+2500) and spaces. A box's bottom (`╰───╯`) holds
      corners, so it is not a rule.
    - The cut is the frame's **upper** rule:
      - with two or more rules in the last 40 non-blank-trimmed lines, it is the second rule from
        the bottom, so a long typed prompt or a completion list between the two rules stays out;
      - with one rule, it is that rule;
      - with none, nothing is cut.
    - The preview is the last four non-blank lines above the cut, with no prompt, no typed text,
      no status line and no cursor, as the mockup draws it (`4-hybrid.html:1008`).
    - Group 5 records a capture of a real Claude Code pane in `lanes/testdata/` as the
      scenario's fixture (PAGE-4-S6), so the cut is held to the real frame, not only the fake's.
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
    4. waiting tiles, compact;
    5. idle tiles, compact: exited, orphaned and no hooks first, then idle;
    6. the lost lane, with Restore all (D9);
    7. the New lane tile.

    Above them, home's heading counts the lanes by state ("3 working, 1 needs you, … 1 lost"),
    which replaces the Lanes figure (SB-10) on home. `homeGroups(lanes)` is pure
    (`home-groups.js`); `home.js` draws.
  - **The terminal's life:** today a visited running lane keeps its xterm and tmux client
    (`panel.js:2579-2586`). That stays.
    - The spotlight reuses the lane's xterm if it has one, refitted to the spotlight's frame.
    - Going home leaves an opened lane's xterm attached and hidden, so coming back does not
      re-attach.
    - A refit resizes the lane's tmux window to the page, as any attach does today (*Window
      size: the latest client wins*). The spotlight is where the owner is looking, so it wins.
  - **A lane opened** (`js/stage.js`): the filmstrip of every live lane (←/→ move in it, K-4),
    the terminal as the stage's first track with its foot line (WS-27), then one row of three
    boxes.
  - **Where the side panel's tabs go** (`SIDE_TABS`, `panel.js:1147`):
    - Agents → **Run**;
    - Figures → behind a click on Run;
    - Git and Checks → **Pull request**;
    - Gate → the top bar's Gate figure (SP-9);
    - Alerts → the sidebar's alerts, filtered to the lane;
    - Activity → the Activity sheet, filtered to the lane.
  - **The Context box draws PANEL-48's measures through PANEL-48's own function.** PANEL-48 task
    4.2 puts the warning-colour and tick decision in a classic static file, tested in
    `web/context_colour_test.go` and citing LANECTX-2-S4. `box-views.js`'s `contextView(lane,
    state, decide)` calls that function, which the entry passes in from its global. It holds no
    threshold or tick of its own, so the decision has one implementation and LANECTX-2-S4's test
    stays on live code. Its node test runs PANEL-48's file in a `vm` context with a `window`, as
    the existing harnesses do, and passes the function in as `decide`.
  - **The terminal's header:** name, state chip, branch, the ↵ to type / Typing into … hint
    (WS-14), then the chips. The chips give way whole, from the end: uptime, the port, Remote,
    then thinking. The model and effort stay. `headerChips(lane, width)` is pure.
  - **The folds:**
    - ⌘J folds the boxes and ⌘. folds Pulse;
    - Pulse folds by itself below 1440 px wide, and the boxes below 800 px high;
    - Context lists four contributors, not five, below 960 px high.

    `layout(width, height, folded, sidebarW)` is pure. It replaces today's 85-column rule
    (`TERM_COLS`, `panel.js:3434`) and the side panel's drag edge (SP-16, dropped by the owner
    in variant 4).
  - **The sidebar's edge stays draggable.** RAIL-14 and K-10 stay: variant 4 drops only SP-16's
    edge, and only the owner drops a function. The drag, or ←/→ and Home/End on the focused
    edge, sets a width kept per browser, as the rail's is today. A double-click or Escape resets
    it to ADR-0009's 252 px, or 268 px from 1700 px, and Enter hides the sidebar.
  - **A lane that needs you while another is open** is not opened for you: its filmstrip tile
    breathes red, and it leads Needs you (ADR-0009). The route and the Needs-you order are pure
    functions (PAGE-5-S3).
  - **Beat:** keeping the tabbed side panel beside the terminal. ADR-0009 sets no tabs and puts
    the boxes beneath it.

- **D9 The ⌘K palette is built here, over one command registry (recommended).**
  - **The registry:** `js/commands.js` (pure) lists each command: its id, its words, its key and
    when it applies (a lane in tmux, a lost lane, a project). The palette, the ⋯ menus and the
    buttons all dispatch through `runCommand(id)` in `js/actions.js`. `actions.js` is the only
    module that sends a lane action's request, so one request has one call site.
  - **What it holds:** every function the shared placement puts behind ⌘K, among them:
    - the lane actions (Restart…, Stop lane…, Close lane…, Remote control…, Interrupt), and the
      lost lane's Resume, Forget and Close lane (WS-25);
    - **Restore all** (BN-4). The server restores lost lanes only together (`POST
      …/lanes/restore-all`, `routes.go:298`), so the sidebar's and the lost tile's Restore is
      Restore all. It keeps BN-4's quota-guard override, its result line and **Dismiss**. A
      single lost lane comes back through its own Resume;
    - **Worktrees**, a sheet of the tree, with New lane here and Remove;
    - **Refresh** (SB-19, ⌘K only);
    - hide the sidebar (RAIL-13, ⌘\);
    - Activity, Metrics and Help;
    - each mode, and the text sizes;
    - New lane, with *Up next* first;
    - the project switch.
  - **Keys (K-1 to K-13 kept, and the new ones):**
    - On a Mac, ⌘K, ⌘J, ⌘. and ⌘\ work everywhere, the terminal included, since xterm takes no
      ⌘ key.
    - Off a Mac they are Ctrl+Alt+K, Ctrl+Alt+J, Ctrl+Alt+. and Ctrl+Alt+\, as K-5's text-size
      keys are. That leaves the terminal its Ctrl+K and Ctrl+J, and leaves the browser its own.
    - Every Ctrl+Alt handler, K-5's included, ignores an event with
      `getModifierState("AltGraph")`. On Windows AltGr reports Ctrl and Alt together. Today
      AltGr+ß on a German layout (a `\`, code `Minus`) shrinks the page instead of typing
      (`panel.js:2355`), and four more keys would copy that.
    - Esc leaves the stage only outside the terminal and fields, where `?` works too. In the
      terminal, Esc is claude's.
  - **Beat:** a row of its own for the palette, keeping today's buttons meanwhile. The hybrid
    places Worktrees, Refresh, text size and the lane menu's items behind ⌘K, so without it they
    have no home on the new page.

- **D10 A stored appearance migrates to a mode (recommended).**
  - **What `theme.js` keeps:** the mode (System, Light, Dark) and the text size, under their
    current `localStorage` keys. It ignores the theme, type and Tuned keys, and removes them on
    the first write.
  - **What the browser sees:** a page with a stored Glass Cockpit dark opens in Dark, with no
    error and no flash (`theme.js` still runs before the first paint).
  - **Beat:** resetting every stored choice to System, which loses the one choice that still
    exists.

- **D11 Eyes in the loop: screenshots in each UI group's review, run by the session through
  `/apply-change`, and OPS-80 for later rows (recommended).**
  - **The harness:**
    - group 1 adds `testdata/ux/shots.sh`: `up.sh`, `matrix.cjs` over the views a group names, at
      the sizes it names in light and dark, then `down.sh`;
    - each group moves `matrix.cjs`'s views to the page it leaves;
    - the `appearance` area becomes the two modes;
    - its layout audit (horizontal scroll, clipped text, overlaps, the terminal's fit) stays.
  - **No page globals:** today the harness reads `panel.js`'s globals, which modules remove. In
    group 1 its `page.evaluate` calls drive the page instead, through clicks, keys and the DOM:
    - `matrix.cjs:514,526` call `rail.w`, `saveRail`, `applyRail`, `setRailW`, `railMax`; these
      become a drag or keys on the edge;
    - `flows.cjs:97` reads `S`, `:396` `terms[selTerm]` and `:469` `P`; these become what the DOM
      shows.

    No test seam ships in the page.
  - **How this change is built:** with `/apply-change`, group by group, not the build-change
    workflow. `build-change.js` has no step between its quick gate and its reviewer (lines
    332-385), and its builder leaves a review task unticked (`ORCHESTRATOR_TASKS`, line 329).
    Under `/apply-change` the session itself goes between them:
    1. it runs the quick gate;
    2. it runs `shots.sh` (and, in group 1, `moved.sh`), outside the sandbox, since they need a
       loopback port, tmux and Chromium, as PANEL-42's 5.3 did;
    3. it writes the shots' folder (and `moved.sh`'s output) into the group's Progress line in
       `tasks.md`, which the reviewer reads with the change's files, so the reviewer is still
       spawned with only the change id and the group (`/apply-change` step 4);
    4. once review converges, it ticks the group's shots task.

    Each such task is marked *(the session)* in `tasks.md`. The session carries eight groups'
    orchestration this way, which D14's budget counts.
  - **For later UI rows:** the mechanism is a new row, **OPS-80**, which this proposal queues. It
    gives `build-change` a project review step that runs `shots.sh` when a group's diff touches
    `framework/internal/panel/web`, and hands the reviewer the shots. Until OPS-80 merges, each
    UI row is built through `/apply-change` with the same task: **Nothing. You.**
  - **The harness is not a gate step.** PRD-ui left that open. A run takes minutes and needs
    Chromium on every gate.
  - **Beat:**
    - build-change with no screenshots, which leaves ADR-0009's *Enforcement* item with no
      executor;
    - a gate step;
    - building OPS-80's step here, which widens this change into the template and the plugin.

- **D12 Later rows plug into named places, and none is drawn before its row (recommended).**

  | Row | Its place |
  |---|---|
  | PANEL-50 | the Context box's contributors list: `js/contributors.js` exports `drawContributors(box, lane)`, which draws nothing until PANEL-50 fills it; the box reserves no height for it |
  | PANEL-37 | a routines section in Pulse: `js/pulse.js` draws the sections in `pulse-sections.js`'s `SECTIONS` list, which holds Flow then Pinned here (ADR-0009's order: routines, Today, the next 14 days, Flow, the pinned cards); PANEL-37 adds routines at the top |
  | PANEL-38 | Today and the next 14 days: two more `SECTIONS` entries, after routines |
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
  - **Node runs the pure modules (D2):** from Go, as `start-plan.js` is run today:
    - a temp folder with `package.json` `{"type":"module"}` and a copy of `static/js/`;
    - `globalThis` holds no `document` or `window`, so a module that breaks D2's second rule
      fails to import.
    - These tests take milliseconds, so they run under `-short`, in the quick gate. They skip
      only without node.
  - **The quick gate** is `go test -short` (`scripts/ci/steps.sh:105`). The real-tmux tail test
    (task 5.1) needs tmux and skips under `-short`, as `copymode_test.go` does. The group
    runs it without `-short`, outside the sandbox, and its Progress line records the result.
  - **Behaviour only a browser shows** (a breathe, a crossfade, focus) is checked in the UX
    harness's flows and escaped `(manual: …)` in `tasks.md`. Its logic (the route, the order)
    is tested under node.
  - **The existing tests, and what each becomes:**

    | Test | What it becomes |
    |---|---|
    | `tells_test.go` `TestPanelAvoidsTheTells` | ADR-0009's ten tells. It also runs over a fixture copy of today's `panel.css` (`web/testdata/panel-css-before.css`) and asserts that copy fails (PAGE-3-S1) |
    | `themes_test.go` | One theme in two modes and ADR-0009's floors. `TestPanelCSSFadesNothingButDisabledControls` follows ADR-0009's opacity rule |
    | `types_test.go` | The one terminal face, its licence and its byte budget |
    | `tells_test.go` `TestTunedMeetsTheFloorsForAnyInput` | Removed with `tuned.js` (D4) |
    | `tells_test.go` `TestLayoutFitsTheWindow` | The new selectors, the same rules: the page never scrolls, the terminal is laid over its frame, a long name yields its width |
    | `projectmenu_test.go:43-66` (`.projbtn` takes `--r-ctl` and `--bar-text-2`; no radius on `.proj*`) | The menu button takes ADR-0009's 8 px control radius and its tokens; the radius ban gives way to tell 3's scale |
    | `startdialog_test.go:95`, `project_config_test.go:109` (the literal `<script src="/static/panel.js"></script>`) | The module tag. The order they hold (the helpers before the entry) stays |
    | `terminal_test.go:128` (the static files served) | Today's list less `tuned.js`, `types.css` and Overpass, plus `js/store.js` and the terminal face |
    | `server_test.go:407` (the live regions patched in place: `needs`, `lanelist`, `tree`, `tabs`, `side` …) | The new page's regions, so "patch in place" holds for every region the page has |
    | `help_test.go` `TestHelpListsTheRealShortcuts` | The keys D9 names |
    | Every other `web` test that reads the page | Its assertion unchanged, read through `readScript` (D2) |

    A test not in this table that has to change is a finding, unless the Decision log says
    what it held and what holds it now.
  - **Beat:** a JavaScript test runner, which needs npm.

- **D14 Risk normal; budget $120, soft (recommended).**
  - **Risk:** normal. It runs the opus/high builder and reviewer.
  - **The budget, from the actuals:**
    - PANEL-42 cost $70.87 over five groups, about $14 a group;
    - PANEL-45's archive records about $31.74, while the owner, briefing this proposal on
      2026-10-17, counted it at $48.22. Its own line says the tracker cannot see agent transcripts
      (OPS-74), so the higher figure is the safer one;
    - PANEL-48 is heading to about $90, by the same briefing.

    Eight groups at about $14 is about $112. Group 1's moves by hand, the screenshot reviews and
    the session's own orchestration of eight groups under `/apply-change` (D11) add to that. So
    $120 is the top of the $100–120 the owner said to expect in that briefing, with two
    sessions.
  - **Soft:** reaching it is reported to the owner and the build goes on, as PANEL-48's is.
  - **Beat:**
    - high risk: the xhigh builder on eight groups would take the cost past the owner's range,
      while a regression here is caught by the existing tests and the screenshots;
    - a hard budget, which would stop a half-moved page mid-group.

- **D15 Built after PANEL-48 and PANEL-49 merge (recommended).**
  - **Why:**
    - PANEL-48 changes the context bar's tick and the warning colour (`panel.js:1118`, `:1401`)
      and adds the classic file D8 calls.
    - PANEL-49 changes the quota note.
    - Both are approved, and both come first in the queue.
  - **What it does:** this change moves their code into the modules and keeps their behaviour and
    their tests (D8, PAGE-5-S4). The build branch is cut from `main` after both are merged, and
    the row's Deps name them.
  - **Beat:** building in parallel and merging over them, which would re-apply their `panel.js`
    hunks by hand into files that did not exist when they were written.

- **D16 Every inventory function has a group, and the last group walks the whole list
  (recommended).**
  The inventory's 135 functions go where *Shared placement* and *Variant 4* put them. These are
  the groups that place them. Each group's tasks name its IDs, and task 8.4 walks all 135 against
  the page.

  | Inventory | Group | Where |
  |---|---|---|
  | PG-1 to PG-11 (PG-12 dropped by the owner) | 1, 2, 4 | kept as they are; PG-4 and PG-5's Live and the connection bar in 3; PG-7 by the motion rules in 2 |
  | SB-2 | 3 | the badge on a project's row in the menu, and *Other projects* in the sidebar (the mockup's section, `4-hybrid.html:582`) |
  | SB-10 | 5 | out of the strip on home and in a lane: home's heading counts the lanes by state, and the filmstrip is the count |
  | SB-1 to SB-22 (less SB-2, SB-10) | 3 | the top bar's figures, as Variant 1's table places them (which variant 4 inherits through Overview first); SB-13 to SB-15 behind its ⋯; SB-22's rule (Gate, then Lanes, then Economy give way into the ⋯ before it would wrap); SB-16 New lane; SB-17, SB-18 in 4; SB-19 Refresh in 7 (⌘K only) |
  | NY-1 to NY-7 | 3 | the sidebar's Needs you and alerts |
  | BN-1 to BN-5 | 3 | BN-1, BN-3 notices under the top bar; BN-2 in the project menu; BN-4 the lost lane in the sidebar; BN-5 a toast at the bottom left with **Dismiss** |
  | RAIL-1, 3, 5, 7 to 15 | 3, 6, 7 | RAIL-5 the sidebar's lane rows; RAIL-8 the ⋯; RAIL-12 the fade; RAIL-13 ⌘\ (7); RAIL-14 the sidebar's edge (D8); RAIL-7 Context's cache line (6); RAIL-9 to 11 Worktrees (7) |
  | RAIL-4 | 4 | the two-hour timeline at the top of the Activity sheet |
  | RAIL-2, RAIL-6, MN-6 | — | dropped by the owner (variant 4: the column picker, sorting) |
  | WS-1 to WS-28, SP-17 | 5, 6 | the filmstrip (WS-1, WS-3); the header (WS-4 to WS-8, WS-14, WS-20, WS-21); the ⋯ (WS-22 to WS-26, the lost lane's in 3); the foot line (WS-27); the boxes (WS-9 to WS-11); WS-12 becomes ⌘J; WS-13 to WS-19 the terminal, kept; WS-2 New lane (3); WS-28 and SP-17 the terminal's empty state (6) |
  | SP-1 to SP-15 | 4, 6 | the boxes (6); Pulse's Pinned and Flow (4); SP-9 the Gate figure (3); SP-10 the sidebar's alerts; SP-11 Activity (4) |
  | SP-16 | — | dropped by the owner (variant 4: the boxes fold instead) |
  | DR-1, DR-2 | 4 | Activity and Metrics as sheets |
  | DLG-1 to DLG-5 | 7 | the dialogs, restyled, opened from ⌘K and their buttons |
  | MN-1 to MN-5, MN-7 | 3, 7 | the project menu (3); Mode and text size (MN-5) in 2 and 7; MN-7 by the filmstrip (6); Theme, Tuned and Type dropped by the owner |
  | FT-1, FT-2 | 3 | behind a click on Live |
  | K-1 to K-13 | 5, 6, 7 | K-4 in the filmstrip (6); K-10 on the sidebar's edge (3); K-11 Metrics' tabs only; the rest kept |

  - **Beat:** relying on each group to remember. That is how a variant loses the functions
    nobody looks at (PANEL-45 D6), and round 1 of this proposal's review found ten such gaps.

- **D17 The tells hold from group 2, over the selectors no later group replaces (recommended).**
  - **The tells:** group 2 rewrites the tells test and applies tells 1 to 9 to every selector of
    `panel.css` that is not legacy.
  - **Legacy is everything a later group replaces:** groups 3 to 7 replace or restyle these, so
    restyling them in group 2 would pay for the same CSS twice:
    - the status bar (group 3);
    - the Needs-you rows and the banners (3);
    - the rail (3);
    - the drawers (4);
    - the tab strip, the workspace and the side panel (6);
    - the menus and every dialog (7).
  - **How the list works:**
    - Their selectors go in one named list in `tells_test.go`, `legacySelectors`, which tells 1
      to 9 skip.
    - Each group deletes or restyles its share and removes it from the list.
    - Task 7.7 asserts the list is empty and stays so.
  - **What holds the legacy rules meanwhile:** the tokens and the floors hold over every selector
    from group 2 on, the legacy ones included. A legacy rule names only tokens.
  - **Group 2 is tokens, motion, the face, the mode and the states.** It restyles no layout.
  - **Motion and colour are tokens and keyframes in group 2:**
    - the pings (2 s, 1.4 s), the breathe (2.8 s), `enter`, `fade`, `forward`;
    - `prefers-reduced-motion`;
    - the project hues as tokens.

    Groups 5 and 6 use them: the tile's 1 px hover rise, the chip's 200 ms crossfade, and the
    lane moving to its state's place.
  - **Beat:** restyling the old layout to pass every tell and then replacing it, which pays for
    the same CSS twice and makes group 2 a whole-page restyle too large for one review round.

## Refusals
| Situation | What happens instead |
|---|---|
| Playwright or Chromium cannot run where the session is | The group's Progress line says so. Review judges the diff and the pure-module tests. The owner's look at the end is the only visual check, and the group is not called converged on visuals |
| The JetBrains Mono file cannot be fetched (network policy) | The build stops for the owner before group 2 commits. The stack falls back to the installed mono only by the owner's word, recorded in the Decision log, since it reverses D5 |
| `tmux capture-pane` fails or the lane has gone | The tail endpoint answers 404 or 502 with a reason. The tile says "no preview", and nothing is retried faster than the 3 s poll |
| node is not installed | The pure-module tests skip. CI's Ubuntu runner has node, so the PR's run is where they run |
| A function in the inventory has no home on the new page | The group stops and names it. Only the owner drops a function (PANEL-45 D6) |
| A tell cannot hold on the mockup's own CSS | ADR-0009 wins. The CSS changes, never the tell, unless the owner amends the ADR |
| `moved.sh` shows a change that is not a move and that the Decision log does not explain | The group's review treats it as a finding |
| The budget is reached | Soft: reported to the owner, and the build goes on |
| PANEL-48 or PANEL-49 has not merged when this row comes up | The build does not start. The session reports the dependency |
