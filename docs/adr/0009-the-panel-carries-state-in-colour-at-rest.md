# ADR 0009: The panel's look is the hybrid: state in colour at rest, one theme, one type system

- **Status**: Accepted
- **Date**: 2026-10-16
- **Tags**: panel, design
- **Source**: PANEL-45 (`changes/archive/2026-10-16-panel-45-ui-design/`): the owner's choice of variant 4, the hybrid,
  on 2026-10-16 (`tasks.md` 2.5); its rules panel (`mockups/4-hybrid.html`, `SHARED_RULES`,
  `tellsRules` and the variant's `rules`); `docs/prds/active/PRD-ui.md` decision 1

## Context
The owner judged the panel's UI the weakest part of Stridal (PRD-ui, *Why*). One cause was a
rule, not a bug: `docs/panel.md` drew the page "the way control rooms and trading desks are: grey
at rest, and colour only when something is abnormal", and `TestPanelAvoidsTheTells` failed any
rounding, shadow, motion, gradient or badge. Six themes and six type systems, each in light and
dark, made every visual change pass contrast tests in twelve combinations.

The austerity was chosen, so it was the owner's to choose again. PANEL-45 drew one direction, a
modern developer tool (in the manner of Linear, Vercel and Raycast) with mission control's
terminal, in three terminal models, on real data from the UX harness. The owner looked at the
three and asked for a named mix: Overview first's home, and a lane opened as Stage lays it out,
with Context, Pull request and Run in boxes beneath the terminal. That is variant 4, the hybrid,
chosen on 2026-10-16.

This record makes the hybrid's rules exact, so that PANEL-46 can build them and
`TestPanelAvoidsTheTells` can hold them. It is a decision about the system because the rules are a
tested contract: the look is not re-decided by whoever next edits `panel.css`.

## Decision
**The panel is drawn as `changes/archive/2026-10-16-panel-45-ui-design/mockups/4-hybrid.html`, under the rules
below.** Where the mockup and these rules disagree, the rules win; the mockup's rules panel is an
annotation and is outside them. Sizes are CSS pixels at 100% (`panel.css` sets rem, at a 16 px
root: 11 px is `.6875rem`).

### Layout and density
- **The page:** a left sidebar (the project menu at its top; Needs you, alerts, lanes and the lost
  lane; Activity and Metrics; Live, the version field and Mode at its foot), a top bar of figures,
  and a body. The sidebar is 252 px wide, 268 px from 1700 px wide.
- **The top bar's figures** are those of `mockups/README.md`'s *Shared placement*. The Lanes figure
  leaves the strip on home and in a lane: the tiles and the filmstrip are the count.
- **Home** is a grid of lane tiles grouped by state: the lane that needs you as a wide spotlight
  with its terminal, working lanes as tiles with a four-line text preview, waiting and idle lanes
  as compact tiles, then the lost lane (with Restore) and a New lane tile.
- **A lane opened:** the filmstrip of every live lane on top (the lost lane stays in the sidebar
  and on home), the terminal beneath it, and one row of three boxes under the terminal: Context
  (with the top contributors, PANEL-50), Pull request (readiness, checks, diff) and Run (cost
  against budget, agents, time). No tabs.
- **Pulse keeps its column at the right in both views:** routines, Today, the next 14 days, Flow
  and the pinned cards. It is 304 px wide, 340 px from 1700 px wide.
- **The terminal's header** keeps its name, state chip, branch and actions. Its chips give way
  whole, never clipped, from the end: uptime, then the port, then Remote, then thinking; the model
  and effort stay.
- A ⌘K command palette replaces most buttons. Every function in `mockups/README.md`'s inventory is
  where its *Shared placement* and *Variant 4* sections put it, and the drops named there are the
  owner's.

### When the terminal shows, grows, shrinks or hides
- **On home the only terminal is the asking lane's,** ready in the spotlight with the ask in view.
  A working tile's preview is the pane's last four lines as text, with no prompt or cursor; waiting
  and idle tiles show none.
- **Opened, the terminal is the stage:** the largest pane, between the filmstrip and the three
  boxes, which never share its row. Opening a lane (a click, ↵ on a tile, or ⌘K) brings it forward
  from 94% in 260 ms.
- **It grows** when the boxes fold (⌘J) or Pulse folds (⌘.); the same key unfolds them. Below
  1440 px wide Pulse folds by itself, and below 800 px high the boxes do. Below 960 px high Context
  lists four contributors, not five.
- **Back home:** Esc, All lanes in the crumb, or a filmstrip tile, which opens that lane instead.
- **A lane that needs you surfaces itself.** On home it takes the spotlight with its terminal
  ready. In another lane it is not opened for you: its filmstrip tile breathes red and it leads
  Needs you.
- **The terminal is dark in both modes,** and its cursor never blinks.

### Colour
- **Lanes carry their state at rest,** as a dot and a chip wherever the lane appears (a lane row,
  a table row, a tile, the filmstrip, the terminal's header): working green (a filled dot), waiting
  amber (filled), needs you red (filled), idle a hollow grey ring, lost a dashed ring. The chip
  says the state in words, so no state is a colour alone. A lane whose ask blocks reads as needs
  you, not waiting.
- **Needs you adds red in four places:** the ask card's left edge and its age; the lane row's tint;
  the ring on its terminal, tile or spotlight; and the needs-you count (the badge and the heading's
  number). The Lanes figure's stacked bar and a running agent's bar use the state colours of what
  they count.
- **Otherwise green, amber and red are verdicts only:** a figure past its threshold (amber from
  context 85%, a budget 80% and quota 75%; red past a budget's 100%, and from quota's 90%); a pass,
  pending or fail mark (a check, a routine's runs, Live); the diff's added and removed counts; an
  alert's mark (amber); a project that is untrusted (amber) or cannot load (red); a destructive
  menu item (red).
- **One accent, indigo,** marks selection (the open lane in the filmstrip, the palette's active
  row, the current project, today), focus and the palette's caret, the typing ring, the sample
  badges, and data series that are not states: the heatmap, Flow's sparklines, the context bar's
  segments and the 7-day reset mark. It never marks a state.
- **Each project's tile has a fixed hue,** its identity. Everything else is ink on neutral
  surfaces, Economy included.
- **`panel.css` references tokens, never a literal colour;** the project hues are tokens too.

**The tokens.** Each mode is designed on its own, not inverted: light lifts panes with soft shadow
on a grey page; dark lifts them with a one-pixel light ring on near-black, and keeps real shadow
for what floats. The values are the mockup's; PANEL-46 may move a value to meet a contrast floor
(*Enforcement*), never the role.

| Token | Light | Dark | Role |
|---|---|---|---|
| `--bg` | `#F2F2F4` | `#0A0A0D` | the page |
| `--surface`, `--surface-2`, `--sunk` | `#FFFFFF`, `#F8F8FA`, `#EDEDF0` | `#141519`, `#18191E`, `#0F1013` | panes, raised rows, wells |
| `--line`, `--line-2` | `rgba(18,20,30,.08)`, `.13` | `rgba(255,255,255,.065)`, `.11` | hairlines |
| `--ink`, `--ink-2`, `--ink-3` | `#191A20`, `#555965`, `#71757F` | `#ECEDEF`, `#A2A5AF`, `#7F838E` | text, three tiers |
| `--accent`, `--accent-soft`, `--accent-ink` | `#5B5BD6`, `rgba(91,91,214,.10)`, `#4646C0` | `#8F8DFF`, `rgba(143,141,255,.14)`, `#ADABFF` | selection, focus, samples, data series |
| `--work`, `--work-soft`, `--work-ink` | `#18A058`, `rgba(24,160,88,.11)`, `#12733F` | `#3CD68A`, `rgba(60,214,138,.12)`, `#5BE2A0` | working; a pass |
| `--wait`, `--wait-soft`, `--wait-ink` | `#D98307`, `rgba(217,131,7,.13)`, `#9A5A00` | `#F4A62A`, `rgba(244,166,42,.13)`, `#FFC25C` | waiting; a warning verdict |
| `--need`, `--need-soft`, `--need-ink`, `--need-ring` | `#E5484D`, `rgba(229,72,77,.10)`, `#C22A30`, `rgba(229,72,77,.32)` | `#FF6168`, `rgba(255,97,104,.12)`, `#FF8A8F`, `rgba(255,97,104,.38)` | needs you; a failure |
| `--idle`, `--idle-soft`, `--idle-ink` | `#A3A6B0`, `rgba(110,114,126,.10)`, `#5D616C` | `#6E717C`, `rgba(255,255,255,.06)`, `#A2A5AF` | idle |
| `--seg1`…`--seg6`, `--heat0`…`--heat4` | the accent's ramp, light to grey | the accent's ramp, light to dark | data series |
| `--term-bg`, `--term-head` | `#0C0D11`, `#121318` | `#08090B`, `#0E0F12` | the terminal, dark in both |
| `--shadow-1`, `--shadow-2`, `--shadow-pop`, `--shadow-term` | soft shadows with a hairline | a light ring; real shadow only on `--shadow-pop` | the four elevations |

The terminal's ANSI palette is not set here: the mockup's `--t-*` colours draw a still image. The
terminal keeps today's default theme's palette (Grey HMI's) unless PANEL-46's design changes it;
the terminal itself is outside PANEL-45 and PANEL-46 (PRD-ui, *Out of scope*).

### Mode and type
- **One theme, two modes:** System by default, with System, Light and Dark at the sidebar's foot
  and in ⌘K. The six themes, the Tuned sliders and the theme picker go.
- **One type system, three faces:** a UI face (the system sans), a data face (the system mono, for
  figures, branches, paths and tokens; tabular, lining, slashed-zero digits) and a terminal face
  (JetBrains Mono, then SF Mono, then the system mono). The six type systems and the type picker
  go.
- Body text is 13 px. Nothing is under 11 px. Weights are 400, 500 and 600, with `b`, `strong`
  and headings at 600.
- The page's and the terminal's text size stay choosable (⌘K), as today.

### Motion
- A working dot pings every 2 s. A needs-you dot pings every 1.4 s, and its lane breathes a red
  ring every 2.8 s until it is answered. Every other dot is still (a failed routine run, Live,
  idle, lost).
- After an action, popovers and the palette enter in 160 ms (a fade and a 4 px rise), the
  palette's scrim fades in over 160 ms, and an opened lane comes forward in 260 ms. A page
  loaded from its address draws its final state.
- On a change of state a chip crossfades to its new colour in 200 ms, and the lane moves to its
  state's place in every list. A changed figure still flashes (a transition, as today).
- Hovered tiles rise 1 px. Nothing else moves at rest. `prefers-reduced-motion` stops all of it;
  state still reads from colour and words.

### What `TestPanelAvoidsTheTells` holds
Over `panel.css` (comments removed) and the page's script:
1. **No `text-transform: uppercase`.** `letter-spacing` only negative, and only on a large figure
   (`.big`) and the home heading.
2. **`font-weight` 400, 500 or 600 only,** with `b`, `strong` and headings set to 600. **No font
   size under 11 px** (`.6875rem`).
3. **`border-radius` only from the scale:** 3 px marks, 6 px small controls and tags, 8 px
   controls and rows, 10 px cards, tiles and popovers, 14 px panes, 999 px pills, 50% dots. A
   multi-value radius may also hold 0, for a square end (the context bar's `999px 0 0 999px`).
4. **`box-shadow` only as the four elevation tokens** (`--shadow-1`, `--shadow-2`,
   `--shadow-pop`, `--shadow-term`), **or as a ring with no blur** (spread only): a hairline
   outline, the idle dot's ring, the heatmap's now mark, the accent's selection and typing ring,
   the needs-you ring.
5. **Gradients only as the sample badges' hatch** (`repeating-linear-gradient`) **and as
   `mask-image` fades where a list runs on:** the lane list's foot, Pulse's foot, the filmstrip's
   end. **`backdrop-filter` only on the palette's scrim.** No `text-shadow` and no `filter`.
6. **`@keyframes` only `ping`, `breathe`, `breatheT`, `breatheS`, `enter`, `fade` and
   `forward`,** each off under `prefers-reduced-motion`. **`transform: scale` only in `ping` (a
   dot) and `forward` (an opened lane).** **Transitions only on `background`,
   `background-color` (the changed-figure flash), `color`, `box-shadow`, `transform` and
   `grid-template-columns`.**
7. **`--work`, `--wait` and `--need` are read only by the selectors that draw the uses named under
   *Colour*.** The accent is read by no selector that draws a state.
8. **Figures** set `font-variant-numeric: tabular-nums lining-nums slashed-zero` on the data face.
9. **The words:** the script joins no words with " · ", puts no arrow on a button, and the
   terminal's cursor does not blink (`cursorBlink: false`).
10. **The hybrid's layout:** home holds one terminal at most, the asking lane's, and a working
    tile's preview is drawn from the pane's last lines; in a lane, the terminal is the stage's
    first track, and no rule sets its width to zero.

### What `TestPanelCSSFadesNothingButDisabledControls` holds
Opacity multiplies every colour under it and hides it from the contrast floors, so `panel.css`
sets `opacity` only:
- inside the `ping`, `enter`, `fade` and `forward` keyframes;
- on a `:disabled` control, as today;
- as 0 or 1 on a control hidden until its row is hovered or selected (the lane row's ⋯:
  `.lrow .more`, shown by `.lrow:hover` and `.lrow.sel`).

Anywhere else a fainter colour is a token, which the floors see: the mockup's `.btn .kbd` (`.6`)
and `.runs i` (`.75`) become token colours in PANEL-46.

### Open for PANEL-46
These are not decided here; PANEL-46's design decides them, and its ADR amendment or a new ADR
records them:
- **Whether to vendor a component helper** (Preact with htm), as PRD-ui decision 2 allows only if
  an ADR chooses it. The hybrid was drawn without one.
- **Whether the accent stays tunable** (one accent hue the owner may change, held to the same
  contrast floors), or is fixed at indigo. `TestTunedMeetsTheFloorsForAnyInput` stays or goes with
  this answer.
- **Whether the terminal face is self-hosted,** so a terminal's cell is the same on every machine,
  or taken from what is installed, as the mockup does.

### What this supersedes
"Grey at rest", in each place it is written. These lines stay in place until PANEL-46 changes the
code, the tests and the doc together (PANEL-45 D8):
- **`docs/panel.md`, *The page*:** "It is drawn the way control rooms and trading desks are: grey
  at rest, and colour only when something is abnormal. Normal work (working, idle, finished) is
  plain text in three tones. Amber means it needs you, red means it failed or is stale, and the one
  link colour marks what you can click or type into. Every state is a word and a small square,
  never a colour alone. … The one motion on the page is a brief flash on a figure whose value just
  changed." Replaced by *Colour* and *Motion* above: state is coloured at rest, needs you is red
  and waiting amber, the accent replaces the link colour, and a dot and a chip replace the square.
  The rail, workspace and side panel it describes are replaced by *Layout and density*.
- **`docs/panel.md`, *Appearance*:**
  - the **Theme** choice and its table of six themes (Grey HMI, Terminal Amber, Glass Cockpit,
    Tuned, TUI, System Native), and Tuned's sliders: replaced by one theme in two modes;
  - the **Type** choice and its table of six type systems: replaced by one type system;
  - "Rules that hold for every theme × type: weights 400 and 700 only; x-heights evened with
    `font-size-adjust: ex-height .52`; … nothing under 11.5 px at 100%; sentence-case headings; no
    chips, pills, cards, gradients, glows or shadows except on what floats": replaced by *Mode and
    type* and the tells above (sentence-case headings stay). The
    self-hosted faces under the SIL OFL go with the six type systems, subject to the open question
    on the terminal face;
  - "At a large size the layout folds. When the rail and the side panel would leave the terminal
    fewer than 85 columns of its face, the side panel folds, then the rail": replaced by the
    hybrid's folds (*When the terminal shows*);
  - *Adding a theme or a type system: the token contract*: one theme leaves no theme or type
    system to add; the rule that `panel.css` names tokens, never a colour, stays;
  - *What checks them*, the `themes_test.go` and `tells_test.go` bullets: the floors stay, for one
    theme in two modes (*Enforcement*), opacity follows the rule above, and the tells are the list
    above. The `types_test.go` bullet, which holds six type systems and their self-hosted faces
    to `theme.js` and `types.css`, goes with them.
- **`framework/internal/panel/web/themes_test.go`, `TestPanelCSSFadesNothingButDisabledControls`**
  (lines 478–497), "panel.css may use [opacity] only on a disabled control": replaced by the
  opacity rule above. The test's tables over six themes become one theme in two modes, and
  `TestTunedMeetsTheFloorsForAnyInput` follows the open question on the accent.
- **`framework/internal/panel/web/types_test.go`:** it holds six type systems, their tokens, their
  `@font-face`s and each one's font budget; one type system leaves it what the open question on
  the terminal face decides.
- **`framework/internal/panel/web/tells_test.go`, `TestPanelAvoidsTheTells`:**
  - lines 20–26, each banned pattern: `letter-spacing` ("no tracked labels"), `gradient\(` ("no
    gradients, stripes or hatching"), `@keyframes|animation` ("no animation but the
    changed-figure flash"), weights other than 400 and 700, `backdrop-filter` ("no glows or
    glass"), and any `border-radius` ("square panes; controls take --r-ctl"): replaced by tells
    1–6. The uppercase ban stays;
  - lines 32–41, "Shadows only on what floats: menus, the drawer, the dialog": replaced by tell 4;
  - lines 42–47: `font-variant-numeric: tabular-nums lining-nums slashed-zero` stays, as tell 8;
    `font-size-adjust: ex-height .52` and `font-feature-settings: var(--num)` go with the six type
    systems they served;
  - lines 48–53, "nothing under 11.5 px (.71875rem)": replaced by 11 px (tell 2);
  - lines 54–66, a control on an `abn-warn` or `abn-crit` row takes the row's ink: those rows go;
    a control on a state-tinted surface (the needs-you row) takes that state's ink, held by the
    contrast floors;
  - lines 67–78, the words and the cursor: kept as tell 9.

## Consequences
### Positive
- The look is the owner's choice, drawn on real data and seen in both modes, not inherited.
- One theme and one type system: a visual change passes contrast in two combinations, not twelve.
- State reads at a glance, and a lane that needs you finds the owner rather than waiting to be
  found.
- The rules stay a tested contract: PANEL-46 rewrites the tells test to them, so polish cannot
  drift into noise unseen.

### Negative / trade-offs
- The owner's six themes and six type systems, and the accessibility choices among them (Atkinson
  Hyperlegible, the TUI theme), go. Text size stays the one accessibility control.
- Colour at rest costs attention that grey at rest saved; the motion rules and the verdict-only
  use of green, amber and red are what keep it from becoming noise, and they hold only while the
  tells test does.
- The system faces differ between macOS, Linux and Windows, so the page will not look identical on
  every machine.
- Until PANEL-46 merges, `docs/panel.md` and the code describe grey at rest while this record
  describes the hybrid.

## Enforcement
`TestPanelAvoidsTheTells` (`framework/internal/panel/web/tells_test.go`), rewritten in PANEL-46 to
the ten tells above and shown failing on today's `panel.css` first; `themes_test.go`, rewritten in
PANEL-46 for the one theme in both modes: its contrast floors (the text tiers and each state's ink
at 4.5:1 on their surfaces, focus and the accent at 3:1, the terminal foreground at 7:1), and
`TestPanelCSSFadesNothingButDisabledControls` rewritten to the opacity rule above; and the UX
harness's screenshots in the review of every UI row (PANEL-46). Until PANEL-46 merges, nothing enforces this
record: today's test holds the rules it supersedes.

## Related
- `changes/archive/2026-10-16-panel-45-ui-design/` (the mockups, the inventory, the parked rows); PANEL-46, which
  builds this; `docs/prds/active/PRD-ui.md`
- PANEL-37, PANEL-38 (Pulse) and PANEL-50 (the top contributors), drawn into the layout above
