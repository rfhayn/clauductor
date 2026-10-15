# Design: panel-45-ui-design

## Decisions

- **D1 What is built: static mockups of two screens, in three directions.**
  - **The screens:** the whole page with a lane selected (status bar, Needs-you rows, rail,
    workspace, side panel) and the project menu open. The terminal is a still image of real
    terminal output, not a live xterm.
  - **The form:** each direction is one self-contained HTML file with its CSS and its data inline,
    no network and no build. It opens from disk in any browser.
  - **Beat:** images of a design tool (nothing to click, and they drift from what HTML can do), and
    a working prototype on the real panel (that is PANEL-46's cost, paid before the choice it
    needs).
- **D2 Three directions, different in structure, not only in colour.** The current panel already
  has six themes: a seventh palette would not answer the owner. Each direction changes what is
  shown where, as well as how it looks.
  - **A. Calm control room.** The current philosophy, done well. Grey at rest stays.
    - The status bar keeps four figures (project, quota, lanes, needs you); the rest move behind
      a click.
    - The side panel's seven tabs become three, with more space, hierarchy and type contrast.
    - One theme in light and dark.
    - The smallest change from today.
  - **B. Modern developer tool** (in the manner of Linear, Vercel and Raycast). This direction
    lifts the ban on colour and shape at rest:
    - colour carries state at rest (working, waiting, idle, needs you), shown as chips;
    - rounded surfaces and soft shadows separate panes;
    - motion runs on state changes;
    - a ⌘K command palette replaces most buttons, and projects and lanes sit in a left sidebar.
  - **C. Mission control** (in the manner of the MAPS dashboard).
    - Dark. The terminal is the stage.
    - Lanes are colour-coded tiles with live indicators.
    - A Pulse column (routines, Today) is reserved for PANEL-37 and PANEL-38, so they arrive in a
      layout that expects them.
    - Colour and motion are decoration as well as state.
  - **Beat:** two directions (cheaper by a third, but a choice between two reads as A or B, and C is
    the one PRD-maps admired), and one refined current design (no choice at all).
- **D3 Each direction states its rules beside it.** A short panel on each mockup says:
  - colour at rest and on state;
  - motion;
  - density (figures in the status bar, tabs in the side panel);
  - type (which of today's six systems survive, if any);
  - themes kept;
  - what `TestPanelAvoidsTheTells` would hold.

  Those lines become the ADR when one is chosen. **Beat:** the owner inferring the rules from the
  picture, which leaves PANEL-46 to guess them.
- **D4 The data is real, but never the installed panel's.**
  - **Source:** the UX harness's throwaway panel (`framework/internal/panel/testdata/ux/up.sh`),
    which runs with its own `HOME`, port and tmux sockets, and its projects and lanes in varied
    states.
  - **Capture:** `/api/state` and `/api/projects` are read once into
    `changes/panel-45-ui-design/mockups/data.js`, with the run's temp paths and token removed.
  - **The installed panel is never read.** It serves the owner's live Standing Tee lanes.
  - **Beat:** data written by hand (it looks tidy, and real panels are not), and the installed
    panel's state (it holds real work, and reading it means its token).
- **D5 Eyes in the loop: screenshots for the reviewer, and the owner opens the files.**
  - Each mockup is screenshotted with Playwright and Chromium at 1440×900 and 1920×1080, in light
    and dark where the direction has both.
  - Playwright is installed in the session's scratchpad, never in the repository.
  - The reviewer reads the PNGs and judges legibility, contrast and whether every function from
    the inventory (D6) is still reachable.
  - If Playwright cannot be installed, the build says so and the owner's look is the only one.
  - **Beat:** review from the HTML alone, which is the gap PRD-ui named ("no eyes in the loop").
- **D6 What every direction must keep: an inventory.** Before any mockup is drawn, the build lists
  every function the page offers today, from `panel.js` and `docs/panel.md`:
  - each figure;
  - each lane action;
  - each dialog and menu;
  - each keyboard path.

  The list goes in `mockups/README.md`. A direction may move a function, or put it behind a click
  or ⌘K, but must say where it went; dropping one is a choice for the owner, named in D3's panel.
  **Beat:** drawing freely and checking afterwards, which loses the functions nobody looks at
  (the restore bar, the gate holder, budgets).
- **D7 The owner's choice is a stop in the build.** After group 2, the build stops. The owner
  opens the three files and picks one direction, or a named mix ("A's density with C's colour"),
  or sends one back. Group 3 then writes the choice down.
  **Beat:** the reviewer choosing, which is a design decision and the owner's (AGENTS.md, *Who
  decides*).
- **D8 The output is an ADR, and `docs/panel.md` changes in PANEL-46.**
  - **The ADR:** the next ADR number (`new-adr`) records the chosen direction's rules, the
    reasons, and the rules of "grey at rest" it replaces. It is Proposed until the owner accepts
    it in the same PR.
  - **`docs/panel.md` keeps today's rules** until PANEL-46 changes the code, the tests and the
    doc together. Otherwise the doc would describe a page the code does not draw, and the tells
    test would contradict the doc.
  - **Beat:** rewriting *The page* and *Appearance* now.
- **D9 The parked panel rows are decided here.** PANEL-25, 30, 31, 32, 33, 34 and 44, OPS-62 and
  OPS-64 were parked on 2026-10-14 for this review.
  - **The table:** the build gives each one a recommendation: into PANEL-46's scope, a row of its
    own after it, or dropped, because the chosen direction removes the thing it fixes.
  - **The owner decides each row**, and the roadmap applies those decisions in this change's PR.
  - **Approved proposals:** an approved proposal whose row is dropped is cancelled. One that is
    kept goes back to the owner if the redesign moved its code (PRD-ui decision 5).
  - **Beat:** carrying all nine into PANEL-46, which makes it too large to converge in review.
- **D10 Risk normal, budget $50.**
  - **Risk:** normal, not low, because the mockups' quality is the whole deliverable, and the low
    tier builds on a smaller model.
  - **Budget:** $50 holds the owner's limit (one lane, a bounded spend): about three mockups, an
    inventory, screenshots and one ADR. `apply-change` checks `change-cost.sh` before each group
    and stops when the budget is spent.
  - **Beat:** low risk with no budget.

## Refusals
| Situation | What happens instead |
|---|---|
| Playwright or Chromium cannot be installed | The build says so in the Progress log; review judges the HTML and the inventory, and the owner's look is the only visual check |
| The harness panel cannot start (a port in use, tmux missing) | The build retries on the next free port in 4700–4799. If it still cannot start, it stops and reports. It never falls back to the installed panel |
| The owner picks none of the three | Group 3 is not built. The owner names what to change, the build redraws the direction(s) named, and the stop repeats |
| The budget is spent before group 3 | The build stops for the owner: raise the budget or accept the mockups as they stand |
