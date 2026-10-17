## Purpose
The panel's page is drawn as ADR-0009's hybrid: a home of lane tiles grouped by state, a lane opened as a stage with its terminal above Context, Pull request and Run, state carried in colour at rest and in words, one theme in two modes, and every function the page offered still reachable. It is served from the binary as native ES modules with no build step.

## ADDED Requirements

### Requirement: The page loads as native ES modules served from the binary
THE SYSTEM SHALL serve the page's script as one `type="module"` entry whose every import is a relative path to a file embedded in the binary, served behind the panel's cookie with a JavaScript content type, with no inline script, no import map and no build step.

#### Scenario: [PAGE-1-S1] Every import resolves to an embedded file, and no module runs an import at load
- **GIVEN** the embedded web files
- **WHEN** every `import` in `static/panel.js` and in the files it reaches is followed
- **THEN** each specifier is relative and names a file under `static/` that the embedded files hold
- **AND** `index.html` loads exactly one `<script type="module">`, holds no inline `<script>` and no import map
- **AND** no module calls an imported binding at its own top level

#### Scenario: [PAGE-1-S2] A module is served as JavaScript only with the cookie
- **GIVEN** a running panel
- **WHEN** `/static/js/store.js` is requested with the panel's cookie, and again without it
- **THEN** the first answers 200 with a `text/javascript` content type and `X-Content-Type-Options: nosniff`
- **AND** the second answers 401

### Requirement: A lane's state is drawn at rest as a dot and a chip in words
THE SYSTEM SHALL draw every lane, wherever it appears, with the dot and chip of one of five looks (working, waiting, needs you, idle, lost), mapped from the lane's state by one function, and the chip SHALL say the state in words so that no state is a colour alone.

#### Scenario: [PAGE-2-S1] Each state the server sends maps to its look and word
- **GIVEN** lanes whose state is busy, waiting on a permission, idle with a finished turn, idle with none, starting, and in the restore list
- **WHEN** their looks are computed
- **THEN** they are working ("working"), needs you ("needs you"), waiting ("your move"), idle ("idle"), idle ("starting") and lost ("lost"), in that order

#### Scenario: [PAGE-2-S2] An exited, orphaned or no-hooks lane is idle in look and says why
- **GIVEN** lanes whose state is dead, orphaned and stale
- **WHEN** their looks are computed
- **THEN** each is the idle look, with the chip "exited", "orphaned" or "no hooks"
- **AND** each sorts straight after the lanes that need you

#### Scenario: [PAGE-2-S3] A Needs-you item that is not a lane's ask leaves the lane's look alone
- **GIVEN** a lane that is idle, and a `warn` Needs-you item for it (`quota_auto_resume_stale`)
- **WHEN** the lane's look and the item's card are computed
- **THEN** the lane is idle, not needs you
- **AND** the card's edge is the warning colour, not the needs-you colour

### Requirement: The look holds ADR-0009's tokens and tells in both modes
THE SYSTEM SHALL draw the page with ADR-0009's tokens, one theme in a light and a dark mode, and SHALL hold ADR-0009's ten tells, its opacity rule and its contrast floors in both modes.

#### Scenario: [PAGE-3-S1] The tells hold over the stylesheet and the script
- **GIVEN** `panel.css` with its comments removed, and the page's script
- **WHEN** ADR-0009's ten tells are checked
- **THEN** none is broken
- **AND** the same check run against `panel.css` as it stood before this change fails

#### Scenario: [PAGE-3-S2] Both modes meet the contrast floors
- **GIVEN** the light and the dark mode's tokens
- **WHEN** their contrasts are computed
- **THEN** the three text tiers and each state's ink are at least 4.5:1 on their surfaces, focus and the accent at least 3:1, and the terminal's foreground at least 7:1

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
WHEN no lane is opened THE SYSTEM SHALL show home: the lane whose ask is oldest as the spotlight with its terminal, the other lanes as tiles grouped by state (needs you, working, waiting, idle, lost, then a New lane tile), a working tile's preview as the pane's last lines in text, and no terminal but the spotlight's.

#### Scenario: [PAGE-4-S1] Tiles are grouped by state in ADR-0009's order
- **GIVEN** lanes that need you (asked 5 and 2 minutes ago), two working, one waiting, two idle and one lost
- **WHEN** home's groups are computed
- **THEN** the lane that asked 5 minutes ago is the spotlight
- **AND** the tiles follow in this order: the other lane that needs you, the working lanes, the waiting lane, the idle lanes, the lost lane, then New lane

#### Scenario: [PAGE-4-S2] With no lane asking, home holds no terminal
- **GIVEN** lanes that are working and idle, none needing you
- **WHEN** home's groups are computed
- **THEN** there is no spotlight, and no tile holds a terminal

#### Scenario: [PAGE-4-S3] The preview is the four lines above the prompt's rule, as text
- **GIVEN** a lane whose pane shows six lines of output, a blank line, a line of `─` only, then the prompt `>`
- **WHEN** its tail is asked for
- **THEN** the answer is the last four lines of output, with no escape sequence, no rule and no prompt

#### Scenario: [PAGE-4-S4] A pane with no rule gives its last four lines with words
- **GIVEN** a lane whose pane shows ten lines of build output and two blank lines, and no rule
- **WHEN** its tail is asked for
- **THEN** the answer is the last four lines of output

#### Scenario: [PAGE-4-S5] The tail answers only for the project's own lanes, with the cookie
- **GIVEN** a running panel serving a project with the lane `lane-a`
- **WHEN** `GET /api/p/<project>/lanes/lane-b/tail` and `GET /api/p/<project>/lanes/lane-a/tail` are each sent with the cookie, and the second again without it
- **THEN** they answer 404, 200, and 401

### Requirement: An opened lane is a stage: filmstrip, terminal, then Context, Pull request and Run
WHEN a lane is opened THE SYSTEM SHALL show the filmstrip of the live lanes, the lane's terminal as the stage's first track, and one row of the Context, Pull request and Run boxes beneath it, fold the boxes and Pulse by the keys and the window's size ADR-0009 sets, and SHALL never open a lane for the owner because it needs them.

#### Scenario: [PAGE-5-S1] Opening a lane and going back home
- **GIVEN** home with a working lane's tile
- **WHEN** the tile is clicked, and then Escape is pressed outside the terminal
- **THEN** the filmstrip, the lane's terminal and the three boxes are shown, the terminal above the boxes
- **AND** after Escape, home is shown again

#### Scenario: [PAGE-5-S2] The boxes and Pulse fold by size and by key
- **GIVEN** a lane opened
- **WHEN** the layout is computed at 1920×1080, 1366×900, 1920×780 and 1920×900, then at 1920×1080 with ⌘J and with ⌘.
- **THEN** at 1920×1080 the boxes and Pulse both show, and Context lists five contributors
- **AND** at 1366×900 Pulse is folded; at 1920×780 the boxes are folded; at 1920×900 Context lists four
- **AND** ⌘J folds the boxes and ⌘. folds Pulse, and pressing either again unfolds what it folded

#### Scenario: [PAGE-5-S3] A lane that needs you while another is open is not opened for you
- **GIVEN** lane A opened, and lane B working
- **WHEN** lane B asks for a permission
- **THEN** lane A stays opened
- **AND** B's filmstrip tile breathes the needs-you ring, and B leads Needs you

#### Scenario: [PAGE-5-S4] The Context box keeps PANEL-48's measures
- **GIVEN** `alerts.context_pct` 80, and a lane whose context reads 82% with its auto-compact tick at 92
- **WHEN** its Context bar is computed
- **THEN** the figure takes the warning colour and the tick sits at 92
- **AND** no module carries the literal 85 or 95 as a context threshold or tick

#### Scenario: [PAGE-5-S5] The terminal's header chips give way whole, from the end
- **GIVEN** a lane with its model and effort, thinking, a port, Remote and an uptime
- **WHEN** its header chips are computed for widths that fit all, all but one, two, three and four fewer
- **THEN** uptime goes first, then the port, then Remote, then thinking
- **AND** the model and effort stay at every width, and no chip is clipped

### Requirement: Every function the page offered stays reachable where the hybrid puts it
THE SYSTEM SHALL offer every function in PANEL-45's inventory at the place its shared placement and variant 4 name, through one command registry that the palette, the menus and the buttons all call, and SHALL offer none of the functions the owner dropped.

#### Scenario: [PAGE-6-S1] The palette holds every command the placement puts behind ⌘K
- **GIVEN** the command registry
- **WHEN** its entries are listed for a selected lane that runs in tmux
- **THEN** they include Restart…, Stop lane…, Close lane…, Remote control…, Interrupt, Worktrees, New lane, Activity, Metrics, Help, each mode, the text sizes and every project's switch
- **AND** each lane action's entry calls the same function as that action's ⋯ menu item

#### Scenario: [PAGE-6-S2] The functions the owner dropped are gone
- **GIVEN** the page's files
- **WHEN** they are searched for the theme picker, the type picker, Tuned's sliders, the lane table's column picker, its sorting and the side panel's drag edge
- **THEN** none is found

#### Scenario: [PAGE-6-S3] New lane offers Up next first
- **GIVEN** a project whose roadmap allows two suggestions next
- **WHEN** New lane's entries are listed in the palette
- **THEN** the two suggestions come before the templates and the plain lane

### Requirement: Pulse holds only what exists
THE SYSTEM SHALL draw Pulse's column from a list of sections, holding the pinned cards and Flow, and SHALL draw no section, heading or sample row for a feature no row has built.

#### Scenario: [PAGE-7-S1] Pulse draws Pinned and Flow and nothing marked as a sample
- **GIVEN** a project with pinned cards and flow figures
- **WHEN** Pulse's sections are listed and the page's files are searched for PANEL-45's sample rows
- **THEN** the sections are Pinned and Flow, in that order
- **AND** no routines, Today or next-14-days heading and no sample row is found
