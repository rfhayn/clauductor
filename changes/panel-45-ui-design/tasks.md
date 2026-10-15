# Tasks: panel-45-ui-design

## Progress
- 2026-10-15 proposed, then revised the same day by the owner's steer (D2, D3, D11): one modern direction with mission control's terminal, drawn in three terminal models; nothing built yet

## Decision log
- (none yet)

## 1. The data and the inventory
- [ ] 1.1 Start the UX harness's throwaway panel (`framework/internal/panel/testdata/ux/up.sh`, its own `HOME`, port and tmux sockets, never the installed panel; D4).
  - Read `/api/state` and `/api/projects` once into `changes/panel-45-ui-design/mockups/data.js`, with the run's temp paths and token removed.
  - Capture one lane's terminal as plain text for the still image.
  - Tear the run down with `down.sh`.
  - (manual: a capture, not a behaviour; there is no scenario)
- [ ] 1.2 `changes/panel-45-ui-design/mockups/README.md`: the inventory of every function the page offers today (D6), read from `framework/internal/panel/web/static/panel.js` and `docs/panel.md`, grouped by where it lives (status bar, Needs-you rows, rail, workspace, side panel, dialogs, menus, keys). (manual: a document)
- [ ] 1.3 In the same README, the parked-row table (D9): PANEL-25, 30, 31, 32, 33, 34 and 44, OPS-62 and OPS-64, each with what it fixes and a recommendation (into PANEL-46, a row after it, or dropped), left for the owner. (manual: a document)

## 2. Three terminal models of one direction
- [ ] 2.1 Variant 1, Stage: `mockups/1-stage.html`. The terminal is always the main pane, with the shared modern chrome around it (D2).
  - It draws the selected-lane screen with the project menu open, in light and in dark (D3).
  - It has the Pulse column with sample data (D11) and its rules panel.
  - (manual: a design, judged by eye)
- [ ] 2.2 Variant 2, Overview first: `mockups/2-overview.html`, with the same screens, modes, Pulse and rules panel.
  - Lane tiles are the home view, and opening one brings its terminal forward.
  - A lane that needs you surfaces with its terminal ready.
  - (manual: a design, judged by eye)
- [ ] 2.3 Variant 3, Split: `mockups/3-split.html`, with the same screens, modes, Pulse and rules panel.
  - The lane list and the terminal sit side by side.
  - The terminal widens while you type and narrows when its lane is idle; both states are drawn.
  - (manual: a design, judged by eye)
- [ ] 2.4 `mockups/index.html` links the three side by side.
  - Each variant is screenshotted at 1440×900 and 1920×1080, in light and in dark, with Playwright installed in the session's scratchpad (D5).
  - The PNGs go to `mockups/shots/`.
  - Each variant's README section says which inventory items it moved, hid or dropped (D6).
  - (manual: a capture)
- [ ] 2.5 The owner opens the three variants and chooses one, chooses a named mix, or sends one back (D7). The build stops here until they do. (manual: the owner's decision)

## 3. The choice, written down
- [ ] 3.1 The ADR (next number, through `new-adr`) records the chosen variant's rules and the parts of "grey at rest" they replace (D8).
  - The rules are D3's lines, made exact: colour tokens, motion, density, terminal behaviour, the one theme and type system, and what `TestPanelAvoidsTheTells` will hold.
  - It is Accepted on the owner's word.
  - (manual: a record)
- [ ] 3.2 Record the choice and the parked-row decisions (D9). (manual: records)
  - `docs/prds/active/PRD-ui.md` records the choice and links the ADR.
  - `docs/roadmap.md`'s PANEL-46 row names the chosen variant.
  - The owner's decision on each parked row from 1.3 is applied: queued, folded into PANEL-46, or cancelled with its reason. An approved proposal of a cancelled row is cancelled with it.

- [ ] Slice: an owner can open three rendered variants of the panel's new look at `changes/panel-45-ui-design/mockups/index.html` in a browser and choose one
