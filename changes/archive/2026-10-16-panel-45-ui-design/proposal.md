**Approved:** 2026-10-15 by Rich · design 7f5df5a2d884
**Roadmap row:** PANEL-45
**Risk:** normal
**Budget:** $50

## Why
The owner judged the panel's UI to be Stridal's weakest part, in both function and look
(`docs/prds/active/PRD-ui.md`, decided 2026-10-14). The page is dense:
- the status bar carries about ten figure groups;
- the side panel has seven tabs;
- six themes and six type systems split the design effort;
- `docs/panel.md`'s "grey at rest" rule and `TestPanelAvoidsTheTells` forbid most of what makes an
  interface feel finished.

No one has chosen the look as a whole; it grew feature by feature (PANEL-11 to PANEL-29).
Before any code is rewritten (PANEL-46), the owner should see real alternatives and pick one.

Its turn: the owner asked for PANEL-45 next after PANEL-42, ahead of PANEL-43 (2026-10-14). The
five changes already proposed are parked, waiting or cancelled by that day's rescope (OPS-71).

## What changes
The owner can open three rendered variants of the panel's new look side by side in a browser, and
choose one.
- **The direction is the owner's** (2026-10-15): a modern developer tool, with mission control's
  terminal as a first-class pane.
- **The variants differ** in how the terminal comes and goes: always the stage, overview first, or
  split.
- **Each variant** is drawn from the same real panel data and states its rules.

The choice becomes an ADR that PANEL-46 builds and tests against. The parked panel rows are each taken into
the redesign or dropped, by the owner's decision.

Nothing in the shipped panel changes in this change: no Go, no `panel.js`, no CSS, no test.

## What the existing specs already guarantee
No living spec covers the page's look (`specs/` holds panel-attach, panel-claude-calls and
panel-new-lane, none of them about appearance). The rules live in `docs/panel.md` (*The page*,
*Appearance*) and are held by `web/tells_test.go` and `themes_test.go`. This change adds and
modifies no requirement (`.openspec.yaml`: `skip_specs: true`). PANEL-46 rewrites the rules and
their tests to the chosen variant.

## Out of scope
- Rewriting `panel.js`, the CSS, `docs/panel.md`'s rules or `TestPanelAvoidsTheTells`: PANEL-46.
- The routines view and Today: PANEL-37 and PANEL-38, which build on PANEL-46.
- The terminal (xterm) and its tmux bridge.

## How we'll know
- **Signal:** the owner has chosen a variant and the ADR records it as Accepted, and PANEL-46's
  proposal cites that ADR's rules rather than inventing its own.
- **Check after:** 21 days
