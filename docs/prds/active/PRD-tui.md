# PRD: the terminal UI, the panel's only local face

**Status:** Approved by the owner, 2026-10-30 (PANEL-60, S18), with the decisions below. It supersedes `PRD-ui.md`, which already
marks itself superseded by the direction shift of 2026-10-27 and names PRD-tui as its replacement, so this PRD leaves
PRD-ui's text as it is. The rows it sets are PANEL-61 to PANEL-64 and PANEL-66 in
`docs/roadmap.md` (Phase 4, *The terminal UI*), with PANEL-50 and PANEL-56 feeding them and PANEL-25 following PANEL-61. Each row is still proposed just in
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
- running and finished subagents per session, each with its type, start, end, parent and description
  (`SubagentView`, `state/model.go`; at most 20 finished per session, `maxFinished`, `state/model.go:99-100`);
- cost per hour over the last hour (`docs/panel.md`, *The page*, est. $) and a quota burn rate over 30 minutes
  (`state/trends.go`).

What the engine does **not** have yet, and which row builds it:

| Missing | Built by |
|---|---|
| a subagent's model, tokens, cost and last words (`SubagentView` carries none of them) | PANEL-66, a new engine row ahead of PANEL-61's detail screen; its proposal first checks whether the transcripts carry per-subagent tokens and cost (decision 6) |
| context by part (messages, tool results, tools, memory files) against the compaction point | PANEL-50, widened: the same transcript reading as its top contributors (decision 6) |
| $/hr over the last ten minutes (the engine's is over the last hour) and "× your average" session | PANEL-61's design defines both windows |
| finished unread: a session that stopped since the owner last selected it | PANEL-61's design defines the read marker |
| a session's colour carried into its terminal (the panel keeps tmux's status bar off today, `docs/panel.md`, *The page*) | PANEL-62, which opens lanes (decision 3) |

The roadmap rows carry this: PANEL-61's names the jump key and the read marker, PANEL-62's a session's colour in its
terminal, PANEL-50's context by part, and PANEL-66 the subagent detail. Neither PANEL-50 nor PANEL-66 blocks the
Sessions screen.

**The states.** The screens and ADR-0009 each name states; this table is the one mapping, from the engine up:

| Engine (hook state and needs) | Screen state (the Dock's word) | ADR-0009's mark |
|---|---|---|
| a `NeedView`, or a `waiting` ask that blocks | needs you | red, filled |
| `busy` | running | green, filled ("working") |
| `waiting` with nothing asked | waiting, filtered and sorted with idle | amber, filled |
| `idle` or `ended`, stopped since last selected | finished unread | grey ring, in bold until read |
| `idle` or `ended`, read | idle | grey ring |
| a lane whose tmux session is gone | lost | dashed ring |

The filter and sort use the screen states; the marks are ADR-0009's, except finished unread's bold, which ADR-0009 has
no state for and this PRD adds.

## What it takes from the Ghostty Dock article

Andrew Baker's Ghostty Dock (andrewbaker.ninja, 2026-10-21; `docs/plans/direction-shift.md`, *What prompted it*)
answers three questions: *which session needs me, what is it costing, and is it about to fall over?* The terminal UI
takes these, and only these:

1. **Four facts per session:** its state (needs you, running, finished unread, idle), how long it has been in that
   state, its context size and its cost so far.
2. **A polite raise:** a session that needs the owner comes forward only after 10 seconds without input. The Dock's
   "only once nothing else is running" is an option here, off by default (decision 2).
3. **A jump key** to the next session that needs the owner.
4. **Spending now:** dollars per hour over the last ten minutes, and "× your average" session.
5. **Context by part:** messages, tool results, tools and memory files, against the point where compaction starts.
6. **Subagents made visible:** what each was asked, its model, tokens, cost and last words.
7. **A session's colour follows it into its terminal** (PANEL-62, decision 3).

It does not take the forked terminal, Burst's billing routing, gateway-level compaction or first-editor file
ownership (the direction shift lists why).

## Screens and keys

Four screens, one at a time, full-screen. Keys are single letters or arrows, shown in a footer line; no key needs a
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
| `a` | attach to the selected lane's tmux session in place of the UI, returning here on detach (PANEL-62, decision 3) |
| `?` | the key list, and the docs link (the README's until the site exists) |
| `q` | quit; the engine keeps running |

**Session detail** (PANEL-61, with PANEL-50's contributors and context by part, and PANEL-66's subagent detail). The four facts in full; context by part against the
compaction point; the top context contributors (PANEL-50); the subagents, running then finished, each with its ask,
model, tokens, cost and last words; and, for a lane, its merge readiness. `Esc` returns to Sessions.

**Lanes** (PANEL-62). Up next (what `.claude/panel-suggest.sh` offers), the live lanes with merge readiness, and the
gate lease with its waiters. Keys: `s` start the selected suggestion as a lane, `x` close the selected lane (with the
same confirmation the browser's Close lane asks for, `docs/panel.md`, *Close lane*), `Esc` back.

**Product summary** (PANEL-63, after PROD-4). Read-only: ideas, their rows, estimated and actual cost. Its screens
proper are claude.ai artifacts (ADR-0011, decision 4).

The raise: when a session enters needs you, the UI waits until the owner has pressed no key for 10 seconds; it then
moves the selection to that session and marks its row. Waiting also until no other session is running is an option,
off by default. It never opens the session or attaches to its terminal by itself. The 10-second wait and the option
are the Dock's; ADR-0009 has neither. What ADR-0009 does hold for the browser carries over: a lane that
needs you "is not opened for you".

## Colour: ADR-0009's rules, carried over

ADR-0009 holds the browser's look; its rules carry over where a character grid can draw them (ADR-0011, *Negative*):
- **State is shown at rest, never by colour alone:** working green (filled dot), waiting amber (filled), needs you
  red (filled), idle a hollow grey ring, lost a dashed ring, each with its state in words.
- **Needs you adds red** to its row's mark, its age, the row's tint and the needs-you count; nowhere else.
- **Otherwise green, amber and red are verdicts only:** amber from context 85%, a budget 80% and quota 75%; red past
  a budget's 100% and from quota 90%; a check's pass, pending or fail.
- **One accent, indigo,** marks selection and focus and data series that are not states (context by part); it never
  marks a state.
- **Colour follows the terminal's background** (decision 5): ADR-0009's tokens in light and dark, the mode detected
  at startup, falling back to dark.
- **The tokens are ADR-0009's values,** in one Go table the UI reads, never literals in a view. A terminal limited to
  256 colours takes the nearest; one with no colour (`NO_COLOR`) still reads, because every state has its word.
  What executes "never literals in a view": PANEL-61 adds a test that fails a colour literal outside the token table,
  as `TestPanelAvoidsTheTells` holds the browser's CSS; until it merges, **Nothing. You.**

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
- **The criterion for removing it** (decision 1): as soon as possible. PANEL-64 is proposed once PANEL-61 and PANEL-62
  have merged and between them cover lane start, close, merge readiness, the gate lease, alerts and needs-you; no
  separate trial session gates it. The session plan puts it in S26–S28, right after PANEL-62. PANEL-64 then removes the web page, the vendored xterm and their
  tests, and cuts `docs/panel.md` to the engine and the terminal UI.

## Out of scope

- The engine: its hooks, sources and endpoints, beyond what a screen needs.
- The desktop app (PANEL-65, after v0.1.0, its own PRD).
- The browser rows the direction shift cancelled (PANEL-37, 38, 43, 30, 31, 39; OPS-62, 84, 94), except where a
  screen above takes their idea.

## Decisions (the owner, 2026-10-30)

1. **Removal: as soon as possible.** PANEL-64 is proposed once PANEL-61 and PANEL-62 have merged and between them
   cover lane start, close, merge readiness, the gate lease, alerts and needs-you. No separate trial session gates it.
   The session plan moves it from S29 into S26–S28, right after PANEL-62.
2. **The raise fires after 10 seconds without a keypress.** Waiting until nothing else runs is an option, off by
   default. The raise only moves the selection; it never opens or attaches anything.
3. **`a` attaches to the lane's tmux session in place of the UI,** by default: it works in any terminal and over SSH.
   Opening the lane in a new terminal tab is optional, through ST-15's backends where present; no terminal-specific
   behaviour is required.
4. **PANEL-37 and PANEL-38 stay out.** Either can return as its own row after v0.1.0.
5. **Colour follows the terminal's background:** ADR-0009's tokens in light and dark, detected at startup, falling
   back to dark; `NO_COLOR` still reads.
6. **Context by part goes into PANEL-50,** the same transcript reading as its top contributors. **A subagent's
   model, tokens, cost and last words get a new engine row, PANEL-66,** ahead of PANEL-61's detail screen, slotted in
   S26–S28; its proposal first checks whether the transcripts carry per-subagent tokens and cost. Neither blocks the
   Sessions screen.
