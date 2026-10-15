# Tasks: panel-45-ui-design

## Progress
- 2026-10-15 proposed; nothing built yet

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

## 2. Three directions
- [ ] 2.1 Direction A, calm control room: `mockups/a-control-room.html`, with the selected-lane screen and the project menu, and its rules panel (D2, D3). (manual: a design, judged by eye)
- [ ] 2.2 Direction B, modern developer tool: `mockups/b-dev-tool.html`, the same screens and its rules panel. (manual: a design, judged by eye)
- [ ] 2.3 Direction C, mission control: `mockups/c-mission-control.html`, the same screens and its rules panel, with the Pulse column reserved for PANEL-37 and PANEL-38. (manual: a design, judged by eye)
- [ ] 2.4 `mockups/index.html` links the three side by side.
  - Screenshots of each direction at 1440×900 and 1920×1080, light and dark where it has both, written with Playwright installed in the session's scratchpad (D5).
  - The PNGs go to `mockups/shots/`.
  - Each direction's README section says which inventory items it moved, hid or dropped (D6).
  - (manual: a capture)
- [ ] 2.5 The owner opens the three directions and chooses one, a named mix, or sends one back (D7). The build stops here until they do. (manual: the owner's decision)

## 3. The choice, written down
- [ ] 3.1 The ADR (next number, through `new-adr`) records the chosen direction's rules (D3's lines, made exact: colour tokens, motion, density, type, themes kept, what `TestPanelAvoidsTheTells` will hold) and the parts of "grey at rest" it replaces; Accepted on the owner's word (D8). (manual: a record)
- [ ] 3.2 `docs/prds/active/PRD-ui.md` records the choice and links the ADR; `docs/roadmap.md`'s PANEL-46 row names the chosen direction, and the owner's decision on each parked row from 1.3 is applied (queued, folded into PANEL-46, or cancelled with its reason; an approved proposal of a cancelled row is cancelled with it) (D9). (manual: records)

- [ ] Slice: an owner can open three rendered directions for the panel's look at `changes/panel-45-ui-design/mockups/index.html` in a browser and choose one
