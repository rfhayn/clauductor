# Tasks: panel-45-ui-design

## Progress
- 2026-10-15 proposed, then revised the same day by the owner's steer (D2, D3, D11): one modern direction with mission control's terminal, drawn in three terminal models; nothing built yet
- 2026-10-15 group 1 done: `mockups/data.js` captured from the harness (by the orchestrator); `mockups/README.md` holds the inventory (135 functions, 11 groups) and the parked-row table with recommendations, the owner's column empty. Next: group 2, the three variants.
- 2026-10-16 group 1 (The data and the inventory) built and reviewed: converged in 2 round(s), peak medium (round 1), low (round 2)

## Decision log
- 2026-10-15, group 1: the harness capture (1.1) was made by the orchestrator, outside this builder's sandbox; the builder checked `mockups/data.js` for a token and temp paths (none) and ticked 1.1 on that. Alternative: the builder runs the harness; not taken, as the sandbox would not start a panel or tmux.
- 2026-10-15, group 1: inventory IDs are a short prefix per place (PG, SB, NY, BN, RAIL, WS, SP, DR, DLG, MN, FT, K) and a number, with a page-wide PG group for what lives nowhere in particular (tab title, favicon, disconnected bar, kept choices, the in-view signal). Alternative: one running number; rejected because a variant moving "SB-12" says where it came from.
- 2026-10-15, group 1: the inventory cites `panel.js` lines at `b1f56bc` (one number per function, the function or its first line), not ranges; PANEL-46 will move them, and the ID, not the line, is what the variants cite.
- 2026-10-15, group 1: the parked-row table adds a "Where its work stands" column (approved, built on a branch, merged groups), because PRD-ui decision 5 sends an approved proposal whose code moves back to the owner and the owner needs that state to decide. PANEL-31's groups 1-2 sit unmerged on `change/panel-31-build`; PANEL-30's group 1 merged in #80.
- 2026-10-15 (the owner): every variant reserves a place for a per-lane "top context contributors" figure, drawn with sample data marked as a sample, for PANEL-50 (queued in #131).
- 2026-10-15, group 1: PANEL-33 is recommended with a condition to observe (a login-lock alert after PANEL-28) rather than a phase, per AGENTS.md rule 1. The alert is a blocking one that raises an OS notification, so something reaches the owner when the condition holds. PANEL-44 is recommended folded into PANEL-31's re-proposal, as a row after PANEL-46.
- 2026-10-16, group 1 review round 1: PANEL-32 is recommended dropped as a scope cut, with no claim that it will come back (nothing would watch for that). Its review notes move into PANEL-31's re-proposal. Alternative: keep it parked until a version is seen not to clear, which nothing observes. PANEL-34 and OPS-64 are now rows after PANEL-46, not drops: D9 drops a row only when the redesign removes what it fixes, and neither one's problem is removed. PANEL-25 now goes back to the owner, because its task 1.5 changes `panel.js`.
- 2026-10-16, group 1 review round 2: the owner's PANEL-50 place (the entry above) is carried into tasks 2.1–2.3 as a bullet, so group 2's builder, reviewer and verify-change see it.

## 1. The data and the inventory
- [x] 1.1 Start the UX harness's throwaway panel (`framework/internal/panel/testdata/ux/up.sh`, its own `HOME`, port and tmux sockets, never the installed panel; D4).
  - Read `/api/state` and `/api/projects` once into `changes/panel-45-ui-design/mockups/data.js`, with the run's temp paths and token removed.
  - Capture one lane's terminal as plain text for the still image.
  - Tear the run down with `down.sh`.
  - (manual: a capture, not a behaviour; there is no scenario)
  - Done by the orchestrator on 2026-10-15. It verified the teardown: all six `ux-p45` tmux sockets are gone, the temp dir is removed, no harness process is left, and port 4745 is free.
- [x] 1.2 `changes/panel-45-ui-design/mockups/README.md`: the inventory of every function the page offers today (D6), read from `framework/internal/panel/web/static/panel.js` and `docs/panel.md`, grouped by where it lives (status bar, Needs-you rows, rail, workspace, side panel, dialogs, menus, keys). (manual: a document)
- [x] 1.3 In the same README, the parked-row table (D9): PANEL-25, 30, 31, 32, 33, 34 and 44, OPS-62 and OPS-64, each with what it fixes and a recommendation (into PANEL-46, a row after it, or dropped), left for the owner. (manual: a document)

## 2. Three terminal models of one direction
- [ ] 2.1 Variant 1, Stage: `mockups/1-stage.html`. The terminal is always the main pane, with the shared modern chrome around it (D2).
  - It draws the selected-lane screen with the project menu open, in light and in dark (D3).
  - It has the Pulse column with sample data (D11) and its rules panel.
  - It reserves a place for the selected lane's top context contributors (PANEL-50), drawn with sample data marked as a sample (the owner, 2026-10-15; Decision log).
  - (manual: a design, judged by eye)
- [ ] 2.2 Variant 2, Overview first: `mockups/2-overview.html`, with the same screens, modes, Pulse, contributors place and rules panel.
  - Lane tiles are the home view, and opening one brings its terminal forward.
  - A lane that needs you surfaces with its terminal ready.
  - (manual: a design, judged by eye)
- [ ] 2.3 Variant 3, Split: `mockups/3-split.html`, with the same screens, modes, Pulse, contributors place and rules panel.
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
