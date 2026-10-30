# PRD: the terminal UI, the panel's only local face

**Status:** awaiting the owner's approval (PANEL-60, S18). Not approved. It supersedes `PRD-ui.md`, which already
marks itself superseded by the direction shift of 2026-10-27. The rows it sets are PANEL-61 to PANEL-64 in
`docs/roadmap.md` (Phase 4, *The terminal UI*), with PANEL-50, PANEL-56 and PANEL-25 feeding them. Each row is still proposed just in
time and approved on its own `design.md`. This PRD gives direction; it does not approve a design.

## Why

The owner decided on 2026-10-27 that the terminal is the local face (ADR-0011, decision 2;
`docs/plans/direction-shift.md`). The panel's Go engine stays; its face becomes a full-screen terminal UI in the same
binary. A browser tab is one more place to look, and the session list belongs where the work happens. The terminal
UI also runs on macOS, Linux, WSL and over SSH from one binary, which the browser page never did without a tunnel.

What the engine already has, and the terminal UI only needs to draw (`framework/internal/panel/state`):
- each session's hook state (`busy`, `waiting`, `idle`, `ended`) and its lane (`state/model.go`);
- each session's estimated cost and context, from the status line's stdin or, in an attached project, its
  transcript (`docs/panel.md`, *The status line*, *Transcripts: an attached project's lanes*);
- the "needs you" items (`NeedView`), alerts, stalled lanes, the gate lease and merge readiness;
- running and finished subagents per session (`state/nesting.go`, at most 20 finished per session).

The four states the screens show are derived, not stored: needs you from a `NeedView` or a blocking ask, running
from `busy`, idle from `idle` or `waiting` with nothing asked. **Finished unread** has no engine state today: it needs
a read marker (a session that stopped since the owner last selected it), which PANEL-61's design settles.

## What it takes from the Ghostty Dock article

Andrew Baker's Ghostty Dock (andrewbaker.ninja, 2026-10-21; `docs/plans/direction-shift.md`, *What prompted it*)
answers three questions: *which session needs me, what is it costing, and is it about to fall over?* The terminal UI
takes these, and only these:

1. **Four facts per session:** its state (needs you, running, finished unread, idle), how long it has been in that
   state, its context size and its cost so far.
2. **A polite raise:** a session that needs the owner comes forward only after 10 seconds without input, and by
   default only once nothing else is running.
3. **A jump key** to the next session that needs the owner.
4. **Spending now:** dollars per hour over the last ten minutes, and "× your average" session.
5. **Context by part:** messages, tool results, tools and memory files, against the point where compaction starts.
6. **Subagents made visible:** what each was asked, its model, tokens, cost and last words.

It does not take the forked terminal, Burst's billing routing, gateway-level compaction or first-editor file
ownership (the direction shift lists why).

## Screens and keys

Three screens, one at a time, full-screen. Keys are single letters or arrows, shown in a footer line; no key needs a
modifier, so the UI works the same over SSH and in any terminal.

**Sessions** (the home screen, PANEL-61). One row per session across every project the panel serves: the project,
the lane's name and branch, the state as a mark and a word, time in state, context (a bar and a percentage) and
cost. A header line carries spending now ($/hr over ten minutes, and "× your average"), the 7-day quota and the
gate lease's holder. Needs-you rows sort first, then running, finished unread, idle.

| Key | Does |
|---|---|
| `↑` `↓` / `j` `k` | move the selection |
| `↵` | open the selected session's detail |
| `n` | jump to the next session that needs the owner |
| `f` | cycle the state filter (all, needs you, running, finished unread, idle) |
| `a` | attach to the selected lane's terminal (tmux), returning here on detach |
| `?` | the key list, and the docs link (the README's until the site exists) |
| `q` | quit; the engine keeps running |

**Session detail** (PANEL-61, with PANEL-50's contributors). The four facts in full; context by part against the
compaction point; the top context contributors (PANEL-50); the subagents, running then finished, each with its ask,
model, tokens, cost and last words; and, for a lane, its merge readiness. `Esc` returns to Sessions.

**Lanes** (PANEL-62). Up next (what `.claude/panel-suggest.sh` offers), the live lanes with merge readiness, and the
gate lease with its waiters. Keys: `s` start the selected suggestion as a lane, `x` close the selected lane (with the
same confirmation the browser's Close lane asks for, `docs/panel.md`, *Close lane*), `Esc` back.

**Product summary** (PANEL-63, after PROD-4). Read-only: ideas, their rows, estimated and actual cost. Its screens
proper are claude.ai artifacts (ADR-0011, decision 4).

The raise: when a session enters needs you, the UI waits until the owner has pressed no key for 10 seconds and, by
default, until no other session is running; it then moves the selection to that session and marks its row. It
never opens the session or attaches to its terminal by itself, as ADR-0009 already holds for the browser ("it is not
opened for you").

## Colour: ADR-0009's rules, carried over

ADR-0009 holds the browser's look; its rules carry over where a character grid can draw them (ADR-0011, *Negative*):
- **State is shown at rest, never by colour alone:** working green (filled dot), waiting amber (filled), needs you
  red (filled), idle a hollow grey ring, lost a dashed ring, each with its state in words.
- **Needs you adds red** to its row's mark, its age and the needs-you count; nowhere else.
- **Otherwise green, amber and red are verdicts only:** amber from context 85%, a budget 80% and quota 75%; red past
  a budget's 100% and from quota 90%; a check's pass, pending or fail.
- **One accent, indigo,** marks selection and focus and data series that are not states (context by part); it never
  marks a state.
- **The tokens are ADR-0009's values,** in one Go table the UI reads, never literals in a view. A terminal limited to
  256 colours takes the nearest; one with no colour (`NO_COLOR`) still reads, because every state has its word.

What does not carry: shadows, elevation, motion and the type system. A terminal has none of them to choose.

## The library

**Bubble Tea and Lip Gloss** (Charm), compiled into the `stridal` binary: users install nothing extra (ADR-0011,
decision 2). They are new dependencies of `framework/go.mod`, so they meet the supply-chain rules
(`.github/dependabot.yml`'s cooldown, `checks/supply-chain.sh`). The UI reads the engine's state in-process or from
the running panel's local API; PANEL-61's design chooses, since one panel runs per machine (`docs/panel.md`, *One
panel per machine*).

## The browser panel: frozen, then removed

- **Frozen now:** fixes only, no new features, until PANEL-64 (ADR-0011, decision 2). What executes it: **Nothing.
  You.** No check refuses a change to `framework/internal/panel/web` (ADR-0011, *Enforcement*).
- **The criterion for removing it:** the terminal UI covers what the owner uses. Concretely, PANEL-64 may be proposed
  once PANEL-61 and PANEL-62 have merged and the owner has run a working session from the terminal UI alone, without
  opening the browser page for anything it lacks. PANEL-64 then removes the web page, the vendored xterm and their
  tests, and cuts `docs/panel.md` to the engine and the terminal UI.

## Out of scope

- The engine: its hooks, sources and endpoints, beyond what a screen needs.
- The desktop app (PANEL-65, after v0.1.0, its own PRD).
- The browser rows the direction shift cancelled (PANEL-37, 38, 43, 30, 31, 39; OPS-62, 84, 94), except where a
  screen above takes their idea.

## Open questions for the owner

1. **The removal criterion.** Is "one working session from the terminal UI alone" the bar, or is it a list of
   functions (lane start, close, merge readiness, the lease, alerts, notifications)?
2. **The raise's default.** Ghostty Dock waits until nothing else is running; with several lanes always running
   that may mean never. Keep it as the default, or raise after the 10 seconds alone?
3. **Attach.** Does `a` attach to the lane's tmux session in place of the UI (as above), or is the terminal never
   embedded and the owner opens the lane in a terminal tab (Standing Tee's way, ST-15)?
4. **Which browser views return.** Of the cancelled rows, Pulse's routines and Today (PANEL-37, PANEL-38) are left
   out here. Confirm they stay out.
5. **The colour tokens.** One theme in the terminal's own dark or light, or follow the terminal's background?
