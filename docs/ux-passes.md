# UX passes: the panel's UI/UX harness

`framework/internal/panel/testdata/ux/` (UX-1) starts throwaway panels with realistic data and
drives them with Playwright. It takes screenshots over a matrix of viewports, appearances and
views, checks each shot against layout rules, and runs scripted end-to-end flows. Several runs
can go at once, because each one has its own run id, port, temp `HOME` and tmux sockets
(`ux-<runid>-<n>`). It never touches `~/.claude`, `~/.clauductor`, launchd or any other tmux
socket. It uses synthetic data only.

It is not part of `go test`: it needs node, Playwright with Chromium, tmux, git, curl and perl.

## Run it

```sh
export PLAYWRIGHT=/path/to/node_modules/playwright     # or NODE_PATH, so require("playwright") works
cd framework/internal/panel/testdata/ux
./run-parallel.sh                          # every shard at once; prints the report's path
./run-parallel.sh --shards flows-lanes --run me1 --ports 4750-4759 --out /tmp/ux-me
```

`run-parallel.sh` builds the checkout once (or uses `$CLAUDUCTOR_BIN`). It then runs each shard
on a panel of its own, and writes `<out>/report.md`, `<out>/findings.json`, and each shard's
`shots/`. Two people or agents running it at the same time need different `--run` ids and
`--ports` ranges. The default range is 4700–4799.

| Shard | What | Time on PANEL-18 (M-series Mac, all five at once) |
|---|---|---|
| `layout-views` | 6 viewports (1280×800, 1440×900, 1920×1080, 2000×900, 2560×1440, 900×800) × 23 views: 138 shots | ~180 s |
| `layout-appearance` | every theme × light/dark, every type system, and the text sizes (85–175%) at 1280, 1920 and 900 px: 75 shots | ~115 s |
| `flows-lanes` | start (from New lane, from New lane here), type and Ctrl+], interrupt, restart, stop, close, remove, forget, kill tmux and Restore all | ~60 s |
| `flows-attachments` | PNG and JPEG dropped, image pasted, non-image refused, selection survives the pointer, ⌘-click URL, OSC 8 asks | ~35 s |
| `flows-projects` | switching projects keeps each one's selection; a project that cannot load | ~20 s |

Each run takes about 6 s to set up. With all five shards in parallel, the whole pass takes about
3 minutes (the time of the slowest shard). `UX_MATRIX_ARGS=--full` adds every theme × type × mode
combination (72 more shots).

The pieces also run alone:

```sh
./up.sh r1 4710 > /tmp/r1.json    # one JSON line: base, token, home, sockets, projects, lanes…
node matrix.cjs /tmp/r1.json --areas views --viewports 1440x900 --out /tmp/m
node flows.cjs /tmp/r1.json --flows attachments --only drop-png --out /tmp/f
./down.sh r1                      # --keep leaves the temp dir (panel.log, HOME) to read
```

The flows act: they start and close lanes and kill the run's tmux server. Give them a run of their
own. `matrix.cjs` only opens things and cancels them.

## What a run holds

`up.sh` creates three git repositories and registers them with `panel add` and `panel trust`:

- **Alpha** (the default project) has a bare `origin`. Its `main` is 2 commits behind, so the
  cards say they may be stale. It has pinned cards, a template with Up next and a queue, and these
  lanes: `working` (busy, with subagents starting and stopping), `waiting` (a permission prompt,
  so it is in Needs you), `idle` (the project root), `stream` (busy, printing a log line every
  0.4 s), `merged` (its PR is merged, so Close lane removes its branch) and `orphan` (its tmux
  session killed). There are also two worktrees with no lane: a clean detached one and a dirty
  one. A `budget` lane starts only if the build shows budgets; PANEL-18 does not, so it is
  skipped.
- **Beta** has one lane and one card.
- **Gamma**'s `panel.json` is broken after it is added, so the project menu shows it as a project
  that cannot load.

A fake `claude` answers `--version` (2.1.284), `agents --json` (each live session, with its
role's status), `auth status --json` (Max) and interactive mode. In interactive mode it draws a
TUI-ish screen that asks for mouse modes 1000/1002/1003/1006 and bracketed paste, and prints a URL
and two OSC 8 links. It records the bytes typed into it in `$HOME/typed.log` (and
`$HOME/fake/typed/<lane>.log`), and it posts hooks and status lines (5-hour quota at 42%, 7-day
at 61%). On `/exit` or a hangup it sends `SessionEnd`. A fake `gh` answers `pr list` with PRs and
checks, and a merged PR for `change/merged`.

## The layout rules (every shot)

`lib.cjs` `layoutAudit`:

- **hscroll**: the page scrolls sideways.
- **clip**: an element's own text overflows a box that hides it without an ellipsis.
- **overlap**: two controls intersect (only the parts that are visible, and not a menu or dialog
  over the page).
- **covered**: something that is not a control sits over the centre of a control.
- **offscreen**: a control runs past the window's side.
- **selected-hidden**: a selected tab is scrolled out of its strip.
- **xterm-fill** / **xterm-fit**: the terminal does not fill its host within 2 px (one cell of
  slack), or its cols/rows differ from what a fit gives. Both must hold for 1.5 s, so a refit
  that is still in progress does not count.
- **console**: page errors and console errors.
- **focus-ring**: checked once per viewport and per theme × mode. Tabbing through the first stops,
  each focused control must draw an outline or a box-shadow.

Flows run the same rules on the state they leave behind: more lanes, result bars and restore
bars. Each finding records its selector, viewport, theme, mode, type, size and screenshot. The
report groups repeats.

## Findings on PANEL-18 (bb63d02)

The harness surfaced these. None is fixed here.

1. **The footer hides behind the terminal below 1180 px** (the automated rule `covered`, seen in
   every 900×800 shot). Once the side panel moves under the terminal, `#wsbody` (938 px tall)
   overflows `#shell` (352 px). The footer (`#obs`: hook counters, **All counters**) is laid out
   at y≈561, in the middle of the terminal, and the terminal paints over it. A faint band shows
   through the terminal, and **All counters** cannot be clicked.
2. **The footer clips its counters at 900 px and 175%** (`clip`). "notifications 0" runs 59 px
   past the footer, with no ellipsis and no wrap.
3. **A session can be bound to the project root for good** (seen through the API, and reproduced
   with `UX_FAKE_EARLY=1 ./up.sh …`). If a lane's claude shows in `claude agents` (or sends a
   hook) before the panel's worktree list has the lane's new worktree, `bindLane` matches the
   record's path to the enclosing worktree (the root). `sess` binds a session once, so every
   such lane's session, status, context and subagents then show on the root lane, and the lanes
   themselves show "none". The worktree list's kick is throttled to 2 s, and the agents poll is
   kicked right after a start. A real claude that registers within about 2 s of its lane
   starting (likely when several lanes start together) could hit this. The fake waits 4 s by
   default, as claude takes seconds to start.
4. **Without a `SessionEnd`, a stopped or closed lane lingers** (seen before the fake sent
   `SessionEnd`). A lane stopped or closed through the panel went on showing as a "≈" row, with
   no ⋯, and as a tab, while status posts or hooks from its session kept coming in the seconds
   after it ended. Real claude sends `SessionEnd` on `/exit`, but a crash or `SIGKILL` does not.
   A closed lane whose worktree is gone showed as a row too.
5. **Copy**: the orphan message says "RESUME … FORGET … CLOSE LANE" in capitals, but its buttons
   read Resume, Forget and Close lane.
6. **The tab strip at 175%** (from reading the screenshots): at 1280 px the last tab is cut
   mid-word ("worki") with no fade or overflow hint. The selected tab stays in view (the
   `selected-hidden` rule holds).

All 19 flows passed on PANEL-18. These all work: starting lanes, typing and Ctrl+], Interrupt,
Restart, Stop, Close, Remove, Forget, Restore all, images dropped and pasted (typed as a
bracketed paste with no Enter, the file 0600 and outside every worktree), the non-image refusal,
the selection surviving the pointer, ⌘-click and OSC 8 links, the per-project selection, and the
Gamma row. Every view had no console errors, no page-level horizontal scroll, no overlapping
controls and no focus ring missing.

## Limitations

- The fake claude is a shell script, not claude's TUI. Layout findings inside the terminal (how
  claude redraws on a resize) are out of scope; the rules check the terminal's box and grid only.
- There is no Metrics view in PANEL-18, and no budgets: those views are feature-detected and
  skipped. The side panel's tabs are skipped where the layout folds the side panel (1280 px).
- The rules are heuristics. `clip` skips ellipsis truncation, and `overlap` and `covered` skip
  floating layers, so a real problem inside an open menu or dialog needs the screenshot.
- Drag-and-drop and paste are synthetic DOM events (`DataTransfer`, `ClipboardEvent`), not OS
  drags. ⌘-click popups are captured, and the linked sites are answered by a stub so nothing
  leaves the machine.
- Only Chromium. Screenshots are the viewport, not the full page (the page may scroll down).
- The flows share one run in order: `restore-all` kills the server, so it runs last.
