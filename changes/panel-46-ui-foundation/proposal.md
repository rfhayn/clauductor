**Status:** awaiting approval
**Roadmap row:** PANEL-46
**Risk:** normal
**Budget:** $120

## Why
The owner chose the panel's new look on 2026-10-16: variant 4, the hybrid
(`changes/archive/2026-10-16-panel-45-ui-design/mockups/4-hybrid.html`). Its rules are
[ADR-0009](../../docs/adr/0009-the-panel-carries-state-in-colour-at-rest.md), accepted that day.
Nothing enforces them yet. Today's code, tests and `docs/panel.md` still draw and hold "grey at
rest", the rule ADR-0009 replaces:
- `panel.js` is one 4,151-line file;
- six themes and six type systems, each in light and dark;
- `TestPanelAvoidsTheTells` fails any rounding, shadow, motion or badge.

Pulse (PANEL-37, PANEL-38) and the per-lane contributors view (PANEL-50) are queued to be built on
the redesigned page, so this row comes first. Its turn comes after PANEL-48 and PANEL-49. Both are
approved and both change `panel.js`, so this change builds on what they merge (design D15).

## What changes
The owner opens the panel and sees the hybrid, drawn from the live state, in light or dark:
- **Home:** a left sidebar (the project menu, Needs you, alerts, the lanes, Activity and
  Metrics, then Live and Mode), a top bar of figures, and a grid of lane tiles grouped by state:
  - the lane that needs you is a spotlight, with its terminal ready;
  - a working lane is a tile with its last four lines;
  - waiting and idle lanes are compact tiles;
  - then the lost lane, with Restore, and a New lane tile.
- **A lane opened:** a filmstrip of the live lanes, the terminal as the stage, and Context, Pull
  request and Run in three boxes beneath it, with Pulse (Flow and the pinned cards) at the right.
- **⌘K:** a palette that reaches every function the buttons it replaces did.
- **Colour:** a lane's state shows at rest, as a coloured dot and a chip that says it in words.
- **Appearance:** one theme in two modes (System, Light, Dark) and one type system. The six
  themes, the Tuned sliders and the six type systems go, as ADR-0009 decided.

Underneath:
- `panel.js` is split into native ES modules with no build step;
- the design tokens are ADR-0009's;
- `TestPanelAvoidsTheTells` holds ADR-0009's ten tells, shown failing on today's CSS first;
- `docs/panel.md`'s *The page* and *Appearance* describe the page as drawn;
- every UI group of this change is reviewed with the UX harness's screenshots, so it is built
  through `/apply-change`, where the session takes them between the gate and the reviewer
  (design D11).

Later rows plug into named places (design D12): PANEL-50 the Context box's contributors, PANEL-37
and PANEL-38 Pulse's sections, PANEL-31 the version field beside Live, PANEL-30 the Worktrees
sheet, PANEL-25 the counters behind Live. PANEL-34 needs only Close's plan, which stays.

## What the existing specs already guarantee
No living spec covers the page's look or layout. `specs/` holds `panel-attach`,
`panel-claude-calls` and `panel-new-lane`. The rules sit in `docs/panel.md` and the web package's
tests. This change adds the capability `panel-page` (ADDED only).

It keeps, and does not modify:
- **ATTACH-4 and ATTACH-5** (the project menu's config line, the Remove from panel… note):
  `project-config.js` is unchanged and its words are drawn in the restyled menu.
- **NEWLANE-1 to NEWLANE-5** (the New lane dialog, its start plan, untrusted templates):
  `start-plan.js` and the dialog's requests are unchanged; the dialog is restyled and opened from
  ⌘K too.
- **LANECTX-2-S4 and LANECTX-2-S5** (PANEL-48, approved, not yet living): the context figure's
  warning colour follows `alerts.context_pct`, and the auto-compact tick is placed per lane. The
  Context box draws both from the lane's fields, never a constant (PAGE-5-S4).
- **CLAUDECALLS-5** (a stuck, stranded or future-dated lock is shown, never removed): its alerts
  move from the rows under the status bar to the sidebar's alerts, with the same words and
  **What to do**. The other CLAUDECALLS requirements are server side and untouched. The only
  server change is one new read-only endpoint (design D7).

## Out of scope
- The per-lane top contributors (PANEL-50), routines (PANEL-37), Today (PANEL-38): their places
  are named and left undrawn (design D12).
- The quiet version field (PANEL-31), Remove worktree's plain words (PANEL-30), the idle panel's
  cost (PANEL-25) and a session closing its own lane (PANEL-34): rows after this one.
- **+ Add a project…** → Attach (PANEL-43): the menu item stays as today.
- The terminal (xterm), its tmux bridge and OPS-62's lost key (PRD-ui, *Out of scope*).
- A reviewer that runs the screenshots for every later UI row: OPS-80, queued by this proposal
  (design D11).

## How we'll know
- **Signal:** two weeks after merge, the owner works from the hybrid's home. No function is
  reported missing that the inventory placed (`mockups/README.md`). A UX harness run on `main`
  finds no layout finding at 1440×900 or 1920×1080, in either mode. No later UI row has had to
  move a token or a tell to land.
- **Check after:** 14 days
