# PRD: a panel that is good to use and good to look at

> **Superseded by the direction shift of 2026-10-27** (ADR-0011, `docs/plans/direction-shift.md`). The local face
> becomes a terminal UI, and PANEL-46 and the browser rows below are cancelled. PRD-tui (PANEL-60) replaces this
> PRD and carries ADR-0009's colour rules. The text below is kept as it was decided.

**Status:** decided 2026-10-14 by the owner (plan approved in session). Rows PANEL-45 and PANEL-46 are queued in
`docs/roadmap.md` (Phase 2, *UI and Pulse*), ahead of OPS-42, PANEL-37 and PANEL-38, which build on them. Each row
is still proposed just in time and approved on its own `design.md`. This PRD gives direction; it does not approve a
design.

## Why

The owner judged the panel's UI to be the weakest part of Clauductor, in both function and look. The rescope of
2026-10-14 (`PRD-maps.md`, *Rescope*) moved the investment there. Exploration found four causes:

- **One file.** `framework/internal/panel/web/static/panel.js` is 4,149 lines. It builds the DOM by hand with an
  `el()` helper and about 25 `render*()` functions, with no modules and no reactive state. A change anywhere risks
  everything.
- **Rules that forbid polish.** `docs/panel.md` (*The page*, *Appearance*) sets the page to "grey at rest, colour only
  when something is abnormal". `web/tells_test.go` (`TestPanelAvoidsTheTells`) fails any rounding, shadow, motion,
  gradient or badge. The austerity was chosen, and it is now the owner's to choose again.
- **Breadth.** Six themes in light and dark and six type systems: every visual change must pass contrast tests in
  twelve combinations.
- **No eyes in the loop.** The Playwright UX harness (`framework/internal/panel/testdata/ux/`, a screenshot matrix and scripted flows, generated per run) runs by hand. Review
  reads CSS as text, so nothing judges rendered pixels.

## Decisions (the owner, 2026-10-14)

1. **Redesign from mockups.** PANEL-45 builds two or three rendered directions. The owner picks one, and an ADR
   records its rules, replacing "grey at rest". `TestPanelAvoidsTheTells` is then rewritten to hold the new rules,
   not deleted: the look stays a tested contract.
2. **Native ES modules, no build step.** PANEL-46 splits `panel.js` into modules the browser loads directly, still
   served by `go:embed`. A small component helper (for example Preact with htm) may be vendored as xterm is, only if
   PANEL-45's ADR chooses it. No npm, no bundler in the gate.
3. **Pulse is the first feature in the new look.** PANEL-37 (routines) and PANEL-38 (Today) build on PANEL-46.
4. **One lane at a time, on a limited budget.** Each row's proposal sets a `Budget:` that `build-change` holds
   (`.claude/change-cost.sh`).
5. **The parked panel rows are reviewed in PANEL-45.** Each of PANEL-25, 30, 31, 32, 33, 34 and 44, OPS-62 and
   OPS-64 is taken into the redesign or dropped. An approved proposal whose code moves under it goes back to the
   owner first.

## The choice (the owner, 2026-10-16)

PANEL-45 drew one direction (a modern developer tool with mission control's terminal) in three terminal models, and
the owner chose a mix of two: **variant 4, the hybrid** (`changes/archive/2026-10-16-panel-45-ui-design/mockups/4-hybrid.html`), with
Overview first's home of lane tiles and a lane opened as Stage lays it out, Context, Pull request and Run beneath the
terminal. Its rules, and the lines of "grey at rest" they replace, are
[ADR-0009](../../adr/0009-the-panel-carries-state-in-colour-at-rest.md). The nine parked rows went as PANEL-45
recommended (`mockups/README.md`, *The parked rows*); `docs/roadmap.md` carries each one.

## Out of scope

- The Go server and its endpoints, beyond what a view needs.
- The terminal (xterm) and its tmux bridge.
- The Map view (cancelled 2026-10-14).

## Open

- **Which direction.** Decided: the hybrid (*The choice*, ADR-0009).
- **How many themes and type systems survive.** Decided: one theme in two modes, and one type system (ADR-0009).
- **Whether to vendor a component helper, whether the accent stays tunable, and whether the terminal face is
  self-hosted.** Left open by ADR-0009 for PANEL-46's design.
- **Whether the UX harness becomes a gate step.** It costs minutes per run, so PANEL-46's design decides.
