## Purpose
The panel's page is drawn as ADR-0009's hybrid: a home of lane tiles grouped by state, a lane opened as a stage with its terminal above Context, Pull request and Run, state carried in colour at rest and in words, one theme in two modes, and every function the page offered still reachable. It is served from the binary as native ES modules with no build step.

## ADDED Requirements

### Requirement: The page loads as native ES modules served from the binary
THE SYSTEM SHALL serve the page's script as one `type="module"` entry whose every import is a relative path to a file embedded in the binary, served behind the panel's cookie with the content type `text/javascript` set by the panel itself, with no inline script, no import map and no build step; and no module SHALL call an imported binding or touch a browser global at its own top level.

#### Scenario: [PAGE-1-S1] Every import resolves to an embedded file, and no module runs anything at load
- **GIVEN** the embedded web files
- **WHEN** every `import` in `static/panel.js` and in the files it reaches is followed
- **THEN** each specifier is relative and names a file under `static/` that the embedded files hold
- **AND** `index.html` loads exactly one `<script type="module">`, holds no inline `<script>` and no import map
- **AND** no module calls an imported binding or reads `document`, `window`, `location`, `localStorage`, `navigator`, `matchMedia` or `ResizeObserver` at its own top level, while a top-level function that does so when called (`const f = () => g()`) passes
- **AND** each pure module (`states`, `home`, `layout`, `boxes`, `chips`, `pulse`, `commands`) imports only pure modules

#### Scenario: [PAGE-1-S2] A module is served as JavaScript only with the cookie
- **GIVEN** a running panel
- **WHEN** `/static/js/store.js` is requested with the panel's cookie, and again without it
- **THEN** the first answers 200 with `Content-Type: text/javascript; charset=utf-8` and `X-Content-Type-Options: nosniff`, whatever the machine's mime tables say
- **AND** the second answers 401

### Requirement: A lane's state is drawn at rest as a dot and a chip in words
THE SYSTEM SHALL draw every lane, wherever it appears, with the dot and chip of one of five looks (working, waiting, needs you, idle, lost), mapped from the lane by one function in a fixed order of precedence (lost, needs you, working, the idle problems, waiting, idle), and the chip SHALL say the state in words so that no state is a colour alone.

#### Scenario: [PAGE-2-S1] Each state the server sends maps to its look and word
- **GIVEN** lanes whose state is busy, waiting on a permission, idle with a finished turn, idle with none, and starting, and a lane whose tmux session is gone and which is in the restore list
- **WHEN** their looks are computed
- **THEN** they are working ("working"), needs you ("needs you"), waiting ("your move"), idle ("idle"), idle ("starting") and lost ("lost"), in that order

#### Scenario: [PAGE-2-S2] An exited, orphaned or no-hooks lane is idle in look and says why
- **GIVEN** lanes whose state is dead, orphaned and not in the restore list, and stale
- **WHEN** their looks are computed and the sidebar's lanes are sorted
- **THEN** each is the idle look, with the chip "exited", "orphaned" or "no hooks"
- **AND** in the sidebar each sorts straight after the lanes that need you

#### Scenario: [PAGE-2-S3] A warning Needs-you item leaves the lane's look alone
- **GIVEN** a lane that is idle, and a `warn` Needs-you item for it (`quota_auto_resume_stale`)
- **WHEN** the lane's look and the item's card are computed
- **THEN** the lane is idle, not needs you
- **AND** the card's edge is the warning colour, not the needs-you colour

#### Scenario: [PAGE-2-S4] A blocking Needs-you item makes its lane need you
- **GIVEN** a lane whose status is idle, and a `block` Needs-you item naming it (a template's first prompt not taken)
- **WHEN** the lane's look is computed
- **THEN** it is needs you ("needs you")

### Requirement: The look holds ADR-0009's tokens and tells in both modes
THE SYSTEM SHALL draw the page with ADR-0009's tokens, one theme in a light and a dark mode, and SHALL hold ADR-0009's ten tells, its opacity rule and its contrast floors in both modes.

#### Scenario: [PAGE-3-S1] The tells hold over the stylesheet and the script, and fail on today's
- **GIVEN** `panel.css` with its comments removed, the page's script, and a fixture copy of `panel.css` as it stood before this change
- **WHEN** ADR-0009's ten tells are checked over each
- **THEN** none is broken by the page's own
- **AND** the fixture copy breaks at least tell 2 (its `font-weight: 700`), and the check names every tell it breaks

#### Scenario: [PAGE-3-S2] Both modes meet the contrast floors
- **GIVEN** the light and the dark mode's tokens
- **WHEN** their contrasts are computed
- **THEN** the three text tiers and each state's ink are at least 4.5:1 on their surfaces, focus and the accent at least 3:1, and the terminal's foreground at least 7:1 on ADR-0009's `--term-bg`

#### Scenario: [PAGE-3-S3] The stylesheet names tokens, never a colour
- **GIVEN** `panel.css` with its comments removed
- **WHEN** it is searched for a hex, `rgb(`, `rgba(`, `hsl(` or named colour outside a `var(--…)`
- **THEN** none is found

#### Scenario: [PAGE-3-S4] Opacity fades nothing but what ADR-0009 allows
- **GIVEN** `panel.css` with its comments removed
- **WHEN** every `opacity` it sets is listed
- **THEN** each is inside the `ping`, `enter`, `fade` or `forward` keyframes, on a `:disabled` control, or 0 or 1 on the lane row's ⋯

#### Scenario: [PAGE-3-S5] A stored theme, type or Tuned choice opens in its stored mode
- **GIVEN** a browser that stored the theme Glass Cockpit, the mode Dark, a type system, Tuned's sliders and a text size of 110%
- **WHEN** the page loads
- **THEN** it draws in Dark at 110%, with no error
- **AND** the theme, type and Tuned entries are gone from storage after the next choice is stored

### Requirement: Home is a grid of lane tiles with at most one terminal
WHEN no lane is opened THE SYSTEM SHALL show home: the lane whose ask is oldest as the spotlight with its terminal, the other lanes as tiles grouped by state (needs you, working, waiting, idle, lost, then a New lane tile), a working tile's preview as the pane's last lines above its prompt in text, and no terminal but the spotlight's.

#### Scenario: [PAGE-4-S1] Tiles are grouped by state in ADR-0009's order
- **GIVEN** lanes that need you (asked 5 and 2 minutes ago), two working, one waiting, one exited, two idle and one lost
- **WHEN** home's groups are computed
- **THEN** the lane that asked 5 minutes ago is the spotlight
- **AND** the tiles follow in this order: the other lane that needs you, the working lanes, the waiting lane, the exited lane, the idle lanes, the lost lane, then New lane

#### Scenario: [PAGE-4-S2] With no lane asking, home holds no terminal
- **GIVEN** lanes that are working and idle, none needing you
- **WHEN** home's groups are computed
- **THEN** there is no spotlight, and no tile holds a terminal

#### Scenario: [PAGE-4-S3] The preview is the four lines above the prompt's frame, as text
- **GIVEN** a lane whose pane shows six lines of output, a blank line, a line of `─` only, the prompt `> run the tests` the owner has typed, a line of `─` only, and a status line
- **WHEN** its tail is asked for
- **THEN** the answer is the last four lines of output, with no escape sequence, no rule, no prompt, no typed text and no status line

#### Scenario: [PAGE-4-S4] A pane with no rule gives its last four lines with words
- **GIVEN** a lane whose pane shows ten lines of build output, a box whose bottom is `╰────╯`, and two blank lines, and no line of `─` only
- **WHEN** its tail is asked for
- **THEN** the answer is the pane's last four lines with words in them, the box's bottom among them

#### Scenario: [PAGE-4-S5] The tail answers only for the project's own lanes, with the cookie
- **GIVEN** a running panel serving a project with the lane `lane-a`
- **WHEN** `GET /api/p/<project>/lanes/lane-b/tail` and `GET /api/p/<project>/lanes/lane-a/tail` are each sent with the cookie, and the second again without it
- **THEN** they answer 404, 200, and 401

#### Scenario: [PAGE-4-S6] A real Claude Code pane is cut above its prompt frame
- **GIVEN** the recorded capture of a working Claude Code lane's pane in `lanes/testdata/`
- **WHEN** its tail is computed
- **THEN** the answer holds the four output lines above the prompt frame's upper rule, and none of the frame's lines

### Requirement: An opened lane is a stage: filmstrip, terminal, then Context, Pull request and Run
WHEN a lane is opened THE SYSTEM SHALL show the filmstrip of the live lanes, the lane's terminal as the stage's first track, and one row of the Context, Pull request and Run boxes beneath it, fold the boxes and Pulse by the keys and the window's size ADR-0009 sets, and SHALL never open a lane for the owner because it needs them.

#### Scenario: [PAGE-5-S1] Opening a lane and going back home
- **GIVEN** home with a working lane's tile
- **WHEN** the tile is clicked, and then Escape is pressed outside the terminal
- **THEN** the filmstrip, the lane's terminal and the three boxes are shown, the terminal above the boxes
- **AND** after Escape, home is shown again

#### Scenario: [PAGE-5-S2] The boxes and Pulse fold by size and by key
- **GIVEN** a lane opened
- **WHEN** the layout is computed at 1920×1080, 1366×900, 1920×780 and 1920×900, then at 1920×1080 with the boxes' fold key and with Pulse's
- **THEN** at 1920×1080 the boxes and Pulse both show, and Context lists five contributors
- **AND** at 1366×900 Pulse is folded; at 1920×780 the boxes are folded; at 1920×900 Context lists four
- **AND** the boxes' key folds the boxes and Pulse's key folds Pulse, and pressing either again unfolds what it folded

#### Scenario: [PAGE-5-S3] A lane that needs you while another is open is not opened for you
- **GIVEN** lane A opened, and lane B working
- **WHEN** lane B asks for a permission, and the route and Needs you are computed
- **THEN** lane A stays opened
- **AND** B leads Needs you, and its filmstrip tile is marked to breathe the needs-you ring

#### Scenario: [PAGE-5-S4] The Context box draws PANEL-48's decision, and none of its own
- **GIVEN** PANEL-48's context-colour function, and a lane whose context reads 82% with its auto-compact tick at 92
- **WHEN** its Context view is computed with `alerts.context_pct` 80, and again with 90
- **THEN** with 80 the figure takes the warning colour, and with 90 it does not
- **AND** the tick sits at 92 both times

#### Scenario: [PAGE-5-S5] The terminal's header chips give way whole, from the end
- **GIVEN** a lane with its model and effort, thinking, a port, Remote and an uptime
- **WHEN** its header chips are computed for widths that fit all, all but one, two, three and four fewer
- **THEN** uptime goes first, then the port, then Remote, then thinking
- **AND** the model and effort stay at every width, and no chip is clipped

### Requirement: Every function the page offered stays reachable where the hybrid puts it
THE SYSTEM SHALL offer every function in PANEL-45's inventory at the place its shared placement and variant 4 name, through one command registry that the palette, the menus and the buttons all dispatch through, and SHALL offer none of the functions the owner dropped.

#### Scenario: [PAGE-6-S1] The registry holds every command the placement puts behind ⌘K
- **GIVEN** the command registry
- **WHEN** its entries are listed for a selected lane that runs in tmux, and for a lost lane
- **THEN** the first holds Restart…, Stop lane…, Close lane…, Remote control…, Interrupt, Worktrees, Refresh, hide the sidebar, New lane, Activity, Metrics, Help, each mode, the text sizes and every project's switch
- **AND** the second holds Restore, Resume, Forget and Close lane
- **AND** the page's lane action requests are sent from one module only, and every ⋯ item and palette entry dispatches by the registry's id

#### Scenario: [PAGE-6-S2] The functions the owner dropped are gone
- **GIVEN** the page's files
- **WHEN** they are searched for the theme picker, the type picker, Tuned's sliders, the lane table's column picker, its sorting and the side panel's drag edge
- **THEN** none is found

#### Scenario: [PAGE-6-S3] New lane offers Up next first
- **GIVEN** a project whose roadmap allows two suggestions next
- **WHEN** New lane's entries are listed in the palette
- **THEN** the two suggestions come before the templates and the plain lane

#### Scenario: [PAGE-6-S4] The new keys are ⌘ on a Mac and Ctrl+Alt elsewhere
- **GIVEN** the registry's keys
- **WHEN** they are listed for a Mac and for any other platform
- **THEN** the palette, the boxes' fold, Pulse's fold and the sidebar are ⌘K, ⌘J, ⌘. and ⌘\ on a Mac, and Ctrl+Alt+K, Ctrl+Alt+J, Ctrl+Alt+. and Ctrl+Alt+\ elsewhere
- **AND** none of them is a plain Ctrl key the terminal uses

### Requirement: Pulse holds only what exists
THE SYSTEM SHALL draw Pulse's column from a list of sections in ADR-0009's order, holding Flow and the pinned cards, and SHALL draw no section, heading or sample row for a feature no row has built.

#### Scenario: [PAGE-7-S1] Pulse draws Flow and Pinned and nothing marked as a sample
- **GIVEN** a project with pinned cards and flow figures
- **WHEN** Pulse's sections are listed and the page's files are searched for PANEL-45's sample rows
- **THEN** the sections are Flow and Pinned, in that order
- **AND** no routines, Today or next-14-days heading and no sample row is found
