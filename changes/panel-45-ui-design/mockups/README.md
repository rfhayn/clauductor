# PANEL-45 mockups

What this folder holds, and what each variant must keep.

- **`data.js`**: the real panel data every variant draws from (design D4). It was captured once on
  2026-10-15 from the UX harness's throwaway panel, never the installed one. It holds
  `window.PANEL_DATA = {projects, state: {alpha, beta, gamma, zeta}, terminal: {working, waiting,
  stream}}`, with the run's temp paths rewritten to `~/dev` and its token removed.
- **The inventory** (D6, below): every function the page offers today.
- **The parked rows** (D9, at the end): nine rows waiting on this review, each with a
  recommendation for the owner.
- **The three variants** (group 2, D2): `1-stage.html`, `2-overview.html`, `3-split.html`, and the owner's
  mix `4-hybrid.html` (2.5's redraw), linked side by side
  from `index.html`, with `shots/` for the screenshots. Where each puts every inventory item is in
  *The variants* below.

## The inventory: what the page offers today

Each line is one function. When a variant moves a function, puts it behind a click or ⌘K, or
drops it, it cites the function's ID (D6). Only the owner drops a function.

- **Sources:** `framework/internal/panel/web/static/panel.js`, `framework/internal/panel/web/index.html`
  (not under `static/`), and `docs/panel.md` (*The page*, *Appearance*, *Controls*, *Lane
  templates*, *Restore after a reboot*, *Alerts*, *Metrics*), read at `b1f56bc`.
- **Code column:** a number alone is a `panel.js` line. Anything else names its file:
  `web/index.html`, or a file beside `panel.js` in `web/static/`.
- **Count:** 135 functions in 11 groups.

### Page-wide (PG)

| ID | Function | Code |
|---|---|---|
| PG-1 | Tab title: `(N) <project>`, "⚠ Disconnected:", ", +N elsewhere" | 3314 |
| PG-2 | Favicon badge with the Needs-you count, "!" while disconnected | `renderFavicon` 3269 |
| PG-3 | New Needs-you items read out to screen readers (polite live region) | `announceNeeds` 3299 |
| PG-4 | Disconnected bar: as of when, reconnect status, **Retry now**. The page is dimmed, ages freeze, and actions are off | `renderConn` 269, `probe` 233 |
| PG-5 | Panel restarted with a new token: the bar says to reopen the URL, with no Retry | 284 |
| PG-6 | `?p=<project>` in the address opens that project, so a bookmark works | 23, 3747 |
| PG-7 | A changed figure flashes briefly (the page's one motion today) | `flash` 127 |
| PG-8 | Ages count up live ("4m", "oldest 12m") | `tickAges` 156 |
| PG-9 | Choices kept per browser: the selected lane per project, columns and sort, side tab, details shown, rail and side widths, card tab and fold, footer, closed warnings, Metrics tab, range and scope, and appearance | `loadSel` 32, and each `localStorage` call |
| PG-10 | "In view" signal: a visible page says so every minute. The dashboard's reads (ps, git, merge readiness, merged PRs) run only then | `seen` 4146 |
| PG-11 | The page is the window: it never scrolls sideways. Below 1180 px the side panel moves under the terminal. At a large text size the side panel, then the rail, fold to keep 85 terminal columns | `fitLayout` 3462 |
| PG-12 | Key-hint bar above the footer (TUI theme only) | `web/index.html`:119, `panel.css`:586 |

### Status bar (SB)

| ID | Function | Code |
|---|---|---|
| SB-1 | The project's name as a dropdown box: opens the project menu (MN-1) | `web/index.html`:33, 3761 |
| SB-2 | "N need you elsewhere" beside it, red when one blocks | `renderProjects` 3632 |
| SB-3 | **Live**, **Disconnected** or **Connecting** | `renderConn` 269 |
| SB-4 | Quota: one bar per window (5-hour, 7-day…), the plan on the first, reset countdown, "as of … ago" past 10 min, and on hover where a reading comes from | `quotaField` 1612, `quotaFields` 1624 |
| SB-5 | Quota projection: a mark where the burn window lands at its reset | 1637 |
| SB-6 | Quota by account: "Quota: none reported", or no quota at all for an API key (spend comes first) | 1626, 1659 |
| SB-7 | **Economy**: on, which roles dropped a tier; on hover, why and since when. Hidden while off | `economyField` 1643 |
| SB-8 | Burn rate: %/h, "full at HH:MM" (amber when before the reset), sparkline, "reset". For an API account it becomes **Spend rate** in $/h | 1665 |
| SB-9 | est. $ (list price): tracked sessions' cost, today, per hour, sparkline. For an API account it reads **API spend (est.)** | 1672, 3317 |
| SB-10 | Lanes: a stacked bar and counts of working, need you and idle | 1673 |
| SB-11 | Needs you: the count and the oldest wait | 1683 |
| SB-12 | Gate: the first queue's holder, how many wait, the oldest wait, "stale", "cannot read". A click shows the holder's lane, or opens Activity | 1686, 1720 |
| SB-13 | Claude processes: CPU %, MB and a sparkline, or "read while in view" / "cannot read" | 1700 |
| SB-14 | Interruptions today (OS notifications sent) | 1702 |
| SB-15 | Hooks: events / status posts / elsewhere (from 1600 px) | 3318, `web/index.html`:68 |
| SB-16 | **New lane**, disabled with its reason on hover (subscription only, quota guard, disconnected) | 3084, 3319 |
| SB-17 | **Activity**: the drawer (DR-1) | 1834 |
| SB-18 | **Metrics**: the drawer (DR-2) | 2118 |
| SB-19 | **Refresh**: re-poll every source now (the account, git and every card too) | 3401 |
| SB-20 | **Appearance**: the menu (MN-5) | 4123 |
| SB-21 | **?**: Help (DLG-5) | 3384 |
| SB-22 | Keeps to one line: above 1180 px, and from 140%, the secondary figures drop when the bar would wrap | `fitFields` 1709 |

### Needs you, alerts and the bars under the status bar (NY, BN)

| ID | Function | Code |
|---|---|---|
| NY-1 | Needs-you rows: Needs you or Blocking, the lane, the specific ask, how long it has waited, ≈ when not a current reading. A click on the row selects the lane | `needRow` 1563, `renderNeeds` 1576 |
| NY-2 | **Open terminal** / **Show lane**: selects the lane and focuses the terminal's frame, never inside it | `jumpTo` 1539 |
| NY-3 | What arrives as a row: permission, elicitation and input waits, quota auto-resume warnings, a stuck template lane, "Restored lane", and "PR merged: close lane?" | server side, drawn by `needRow` 1563 |
| NY-4 | Alerts rows: every alert that belongs to no lane, every blocking one, and approval wait, budget and stale. Each shows its kind, the lane or "All lanes", its text and its age | 1585 |
| NY-5 | **What to do** on a login-lock alert opens Help at that section | 1592, `openHelpAt` 3366 |
| NY-6 | Your move rows: finished turns that are not blocked | 1597 |
| NY-7 | Headings say "as of HH:MM" while disconnected | `asOf` 1548 |
| BN-1 | Banners: No hooks, Cannot read (claude agents, git worktree list, tmux), Config changed, Events dropped, Hooks, Lane registry | `renderBanners` 3190 |
| BN-2 | Config untrusted banner, with **Trust config…** | 3197 |
| BN-3 | Warnings (amber: an unverified Claude Code version, ignored events) with **×**. A closed one stays closed until it is about something new | 3205, `closeWarning` 3217 |
| BN-4 | Restore bar: lanes that lost their tmux session, **Restore all**, a quota-guard override, the result and **Dismiss** | `renderRestore` 3147 |
| BN-5 | Close and Remove result: what was removed and kept, and why, with **Dismiss** | `renderClosed` 2711 |

### The rail (RAIL)

| ID | Function | Code |
|---|---|---|
| RAIL-1 | Lanes heading: count, "as of" when disconnected, **New lane** | `renderRail` 690 |
| RAIL-2 | **Columns**: choose from 19 columns (5 by default) | `colPicker` 631, `COLS` 522 |
| RAIL-3 | Source note when `claude agents` cannot be read | 695 |
| RAIL-4 | Two-hour timeline of every lane's state, one line each | `timeline` 666 |
| RAIL-5 | Lane table: state word and square, name, the chosen columns. Rows that need you are tinted amber and failing ones red, ≈ marks an approximate status, and the selected row is marked | `laneTable` 589 |
| RAIL-6 | Sort by a column heading; a second click reverses it | 597 |
| RAIL-7 | Cache column: hit %, "cold in 1:52" in amber, "cold" | `cacheCell` 547 |
| RAIL-8 | **⋯** on each lane the panel started: its actions (MN-4) | `actsButton` 868 |
| RAIL-9 | Worktrees tree: the project, each worktree (type, branch, path, ahead/behind, changed files), its sessions, and agents nested by who started them, with finished ones folded | `renderTree` 784, `sessionNode` 770, `agentNodes` 752 |
| RAIL-10 | **New lane here** on a worktree with no lane | 819 |
| RAIL-11 | **Remove** a worktree: an inline plan (Removes, Keeps, why), then **Confirm remove** | 825, `askRemove` 2732, `removeWords` 2756 |
| RAIL-12 | "More below" when the rail runs past its foot | `moreBelow` 703 |
| RAIL-13 | **Lanes** toggle: hide or show the rail (hidden below 900 px) | 3490 |
| RAIL-14 | Rail edge: drag it, or use ←/→ and Home/End. Escape or a double-click resets it, and Enter hides the rail | 3505 |
| RAIL-15 | Empty: "No lane yet. Sessions started in this project's worktrees show here too." | 696 |

### The workspace (WS)

| ID | Function | Code |
|---|---|---|
| WS-1 | Terminal tabs: one per lane, with its state square and name | `renderTabs` 954 |
| WS-2 | **+** tab: the Start dialog | 3085 |
| WS-3 | **N more** after the strip, with a menu of the tabs out of sight (MN-7) | `fitTabs` 1020, 1070 |
| WS-4 | Lane header: name, state, branch and worktree | `renderLaneHead` 1093 |
| WS-5 | Header: model, effort, thinking, fast mode | 1102 |
| WS-6 | Header: the lane's own port (`CLAUDUCTOR_PORT`) | 1104 |
| WS-7 | Header: Remote (Remote Control on, for every session or the panel's lanes) | 1111 |
| WS-8 | Header: Up (uptime) | 1117 |
| WS-9 | Header: Context as a bar with the auto-compact mark, and % | 1118 |
| WS-10 | Header: Cost and $/h | 1119 |
| WS-11 | Header: Budget bar for a change with a budget (amber from 80%, red past it) | 1122 |
| WS-12 | **Hide details** / **Show details**: the side panel | 1132 |
| WS-13 | The live terminal (xterm), refitting to the space it gets | `ensureTerm` 2257, `fitTerm` 2472 |
| WS-14 | Hint line above the terminal: how to enter it, "Typing into…", "Scrolled back…" | `renderHint` 2152 |
| WS-15 | Enter or a click enters the terminal and Ctrl+] leaves it. Nothing focuses it by itself | `enterTerm` 2143, 2352 |
| WS-16 | The wheel scrolls history (tmux copy mode), and the next key leaves it and is typed | 2428 |
| WS-17 | A drag selects text, and the selection stays until it is copied (⌘C) | 2336 |
| WS-18 | Links: ⌘-click opens one, and hover names its target. A link whose text is not its address asks first (**Open link** / **Cancel**) | `urlLinks` 2491, `followLink` 2528, 2773 |
| WS-19 | Images: drop or paste one and its path is typed. A refusal says why | `dropImages` 2385, 2315, 2322 |
| WS-20 | **Attach in Terminal.app** | 2817 |
| WS-21 | **Interrupt (Esc)**, at once | 2818 |
| WS-22 | **Restart**: asks first, in words built from the lane's state | 2819, `stopWords` 2624 |
| WS-23 | **Stop lane**: asks first | 2820 |
| WS-24 | **Close lane**: shows the plan (what goes, what stays), then asks | `askClose` 2673, `closeWords` 2699 |
| WS-25 | Orphaned lane: an explanation, **Resume**, **Forget** (asks first), **Close lane** | 2780, 2604 |
| WS-26 | Remote control confirmation (from the ⋯ menu), only into an idle claude | 2798 |
| WS-27 | Notes under the terminal: the template's first-prompt state, "claude exited", the action under way, its result, "not in the lane registry" | 2826 |
| WS-28 | Empty states: no lane yet (with **+ New lane**), a lane started outside the panel, and why a start is blocked | `renderTerminals` 2580 |

### The side panel (SP)

| ID | Function | Code |
|---|---|---|
| SP-1 | Tabs: Agents, Figures, Git, Checks, Gate, Alerts, Activity. Agents and Alerts show counts. The choice is kept, and ←/→ move between tabs | `SIDE_TABS` 1147, `renderSide` 1186 |
| SP-2 | Agents: each session (pid, state and for how long, what it waits for, compaction, last failure, unknown notification, agent state). Then the Approximate note: agent pairing was verified on another Claude Code version, with the check's progress ("1 of 3 subagent launches confirmed") or the break | `sideAgents` 1347, 1363 |
| SP-3 | Agents: a Gantt over up to two hours, nested by starter, with workflow runs marked ≈ | 1366 |
| SP-4 | Figures: state, context with tokens, cache (sparkline, cold-in, last miss, tokens to rebuild), cost (sparkline), busy ratio, wall and API time, last failure, subagents, CPU and memory, mode | `sideFigures` 1395 |
| SP-5 | Figures: a rate table of lines, turns, asks and compactions, in total and per hour | 1414 |
| SP-6 | Git: branch, HEAD, path, upstream with ahead and behind, changes, diff stat, last commit, read age, the template's first prompt | `sideGit` 1430 |
| SP-7 | Git: the pull request (number, checks, review, title, author) | 1445 |
| SP-8 | Checks: the merge-readiness verdict, then a row per check | `sideChecks` 1455 |
| SP-9 | Gate: each queue, where this lane stands, the holder and the line, **Cancel wait**, **Run in `<lane>`** | `sideGate` 1468, `laneQueue` 1518 |
| SP-10 | Alerts: this lane's needs and alerts | `sideAlerts` 1473 |
| SP-11 | Activity: this lane's events, newest first | 1227, `feedList` 1763 |
| SP-12 | Pinned cards: a tab per card, **▾** to fold, rows that open, read age, "cannot read" | `projectBox` 1288, `pinnedRows` 1324 |
| SP-13 | Cards stale note: "main is N commits behind … cards may be stale" | `staleNote` 1281 |
| SP-14 | No cards: one line, with **How cards work** | `noCards` 1270 |
| SP-15 | Flow (30d) card: cycle time, merges, change-fail rate and spend, with sparklines. It opens Metrics | `flowCard` 1163 |
| SP-16 | Side edge: drag it, or use the keys; it resets like the rail's | 3561 |
| SP-17 | No lane selected: "Select a lane, or start one." | 1191 |

### Drawers (DR)

| ID | Function | Code |
|---|---|---|
| DR-1 | Activity: each queue (holder, waiters, **Cancel wait**), the open PRs, the project's cards (with the stale note), every lane's events grouped by lane, and "OS notifications are off" | `renderDrawer` 1781 |
| DR-2 | Metrics: Flow, Cost, Quality and Outcomes tabs; 7d, 30d or 90d; this project or all projects. A source line says where each figure comes from, and each figure has its series, how many items it covers and its source. "—" comes with a reason, and a change over budget is amber | `renderMetrics` 2026, `renderMetricsBody` 1976, `loadMetrics` 2077 |

### Dialogs (DLG)

| ID | Function | Code |
|---|---|---|
| DLG-1 | Start a lane: Up next, then template and lane type (each with a help line), where it runs (4 choices), worktree, name and issue. It previews the prompt and the branch, has a quota-guard override, and shows warnings (no Approved line, branch in use) that turn Start into **Start anyway**. A refusal shows in the dialog; **Cancel** closes it | `openStart` 3037, `stUpdate` 2934, `renderNext` 2999, `start-plan.js` |
| DLG-2 | Add a project: the path is checked as you type (root, common dir, id and socket, or why not). **Create this config** shows the init preview first. The trust report is followed by **Trust and add** or **Add without trusting** | `openAddDlg` 3822, `adValidate` 3875, `adAdd` 3915 |
| DLG-3 | Trust config…: the trust report, then trust exactly those bytes | `openTrustDlg` 3981 |
| DLG-4 | Remove from panel…: says what stays, and is refused while the project has lanes (each one a link). It names the new default and gives the sidecar's line back | `openRemoveDlg` 3958 |
| DLG-5 | Help: the keyboard table, how-tos, the login-lock section, and a link to the guide | `openHelp` 3357, `web/index.html`:198 |

### Menus (MN)

| ID | Function | Code |
|---|---|---|
| MN-1 | Project menu: each project with its working, waiting and idle lanes, lanes to restore, "config untrusted", "cannot load" and why, "reads <file>, outside the repository", and a needs-you badge. Picking one switches the whole page | `renderProjects` 3632, `switchProject` 3742, `project-config.js` |
| MN-2 | **+ Add a project…** at the menu's foot (DLG-2) | 3794 |
| MN-3 | A project's **⋯**: **Trust config…** (while untrusted or changed) and **Remove from panel…** | `openProjActs` 3693 |
| MN-4 | A lane's **⋯**: Interrupt, Restart, Remote control, Stop lane and Close lane, or for an orphan Resume, Forget and Close lane. Each one asks first | `rowActs` 861, `renderRowMenu` 888, `pickRowAct` 925 |
| MN-5 | Appearance: Theme (6, with swatches); Mode (System, Light, Dark); Tuned's sliders (base hue, accent hue, contrast); Type (Theme's choice, or one of 6); Size (page 85–175%, terminal 11–24 px, A− and A+, Reset both) | `renderPicker` 4032, `theme.js`, `tuned.js` |
| MN-6 | Columns: a checkbox per column | `colPicker` 631 |
| MN-7 | The tabs out of sight | `renderTabMenu` 1036 |

### Footer (FT)

| ID | Function | Code |
|---|---|---|
| FT-1 | Counters: events, status posts, other projects, dropped (amber when any), notifications | `renderObs` 3226 |
| FT-2 | **All counters**: overflow, malformed, unknown event, unknown notification, `claude agents` latency and interval, the filter, and the Claude Code version against the one it was verified on | 3246 |

### Keys (K)

| ID | Function | Code |
|---|---|---|
| K-1 | **?** opens Help (not in the terminal or a field) | 3384 |
| K-2 | **Enter** on the terminal's frame enters it | 2172 |
| K-3 | **Ctrl+]** leaves the terminal | 2352 |
| K-4 | **← → Home End** on the lane tabs: move and select at once | 969 |
| K-5 | **Ctrl+Alt+= / − / 0**: the page's text size, inside the terminal too | 3414, 2356 |
| K-6 | **⌘-click** (Ctrl-click off a Mac) opens a link | `followLink` 2528 |
| K-7 | **⌘C** copies; **⌘V** pastes text or an image | 2322 |
| K-8 | **Escape** closes a dialog, a menu, Activity or Metrics (in the terminal it is claude's) | 1836, 2120, 3088, 3391 |
| K-9 | Menu keys (project, project ⋯, lane ⋯, Appearance, tabs out of sight): ↓ or Enter opens; arrows, Home and End move; End reaches + Add a project…; → opens a row's ⋯ and ← comes back | 3729, 3762, 935, 4124, 1071 |
| K-10 | Pane edges: ←/→, Home/End, Escape resets, Enter hides the rail | 3527, 3584 |
| K-11 | Tablists (side tabs, card tabs, Metrics tabs): ←/→, plus Home/End on Metrics | 1209, 1302, 2041 |
| K-12 | Dialogs keep Tab inside them; Enter in Add's path field checks it at once | `trapTab` 3813, 3901 |
| K-13 | A lane row or tree node: Enter or Space selects it, and never enters its terminal | `pressable` 502 |

## The parked rows (D9)

These nine rows were parked by the rescope of 2026-10-14 to wait for this review (PRD-ui decision
5). Each recommendation is one of three: **into PANEL-46**, **a row after PANEL-46**, or
**dropped**. The owner decides each row, and group 3 applies those decisions to the roadmap. An
approved proposal whose code the redesign moves goes back to the owner first.

| Row | What it fixes | Inventory it touches | Where its work stands | Recommendation | Why | Owner's decision |
|---|---|---|---|---|---|---|
| PANEL-25 | An idle panel spawns about 35 processes a minute and calls GitHub every minute per project with no page open. The login agent opens a browser tab at every login | PG-10, SB-19, NY-3 (auto-close on merge), FT-1 and FT-2 (spawn rates in the footer, its task 1.5), DR-2 (the metrics command runs only while a page is in view, its task 3.7) | Proposal approved 2026-10-02; not built | A row after PANEL-46; its proposal goes back to the owner | Most of it is server polling, but task 1.5 changes `panel.js`, which PANEL-46 splits into modules, so PRD-ui decision 5 applies | As recommended (2026-10-16): a row after PANEL-46, its proposal back to the owner |
| PANEL-30 | Remove worktree speaks git, doesn't warn when a change is still in flight, and deletes ignored files without saying so | RAIL-11, BN-5, WS-24 (Close lane shares its rules) | Proposal approved 2026-10-02. Group 1 (a commit on no branch is kept) merged in #80, but its `tasks.md` on main still shows 1.1 and 1.2 unticked: the re-proposal must tick them. Groups 2–6 not built | A row after PANEL-46; its proposal goes back to the owner | The plain words and the in-flight warning are server side and still needed, but its group 5 lays the plan out on a page PANEL-46 redraws | As recommended (2026-10-16): a row after PANEL-46, its proposal back to the owner; group 5's plan is laid out on the hybrid's page (Remove sits behind ⌘K, Worktrees) |
| PANEL-31 | A new Claude Code version puts an amber bar across the whole page until three subagent launches confirm it, and one project's verification doesn't reach the others | BN-3, FT-2, SP-2 (the Approximate note), DLG-5, and a new status field beside SB-12 | Proposal approved 2026-10-02. Groups 1–2 built on `change/panel-31-build` (2026-10-13), unmerged; group 3 not built | A row after PANEL-46; its proposal goes back to the owner and takes in PANEL-44 and PANEL-32's review notes | Group 1 (server: a verification holds everywhere) lands as built. Group 2's status field is drawn again on the new status bar, which each variant should leave room for | As recommended (2026-10-16): a row after PANEL-46, its proposal back to the owner, taking in PANEL-44 and PANEL-32's notes; its quiet version field goes where the hybrid places it, at the sidebar's foot beside Live |
| PANEL-32 | **Verify now**: a button that runs a throwaway session to confirm a new version on demand, at a stated cost | The PANEL-31 field, DLG-5 | Not proposed; it carries review notes from PANEL-28 and PANEL-31 | Dropped, as a scope cut: the redesign does not remove what it fixes | PANEL-31 makes the check clear by itself and turns the page-wide bar into a quiet field, so verifying on demand is not worth a probe that holds the panel's single `claude` slot for about 105 s. Its review notes move into PANEL-31's re-proposal, among them "revisit Accept this version (PANEL-31 D4) if the check proves too slow to clear" | As recommended (2026-10-16): dropped as a scope cut; its notes move into PANEL-31's re-proposal |
| PANEL-33 | Find out whether the panel's own `claude agents` and `auth status` calls take or spend a login refresh | NY-4, NY-5 (the login-lock alerts), DLG-5 (its login-lock section) | Not proposed | A row after PANEL-46, started on an observed condition | It is an investigation with no page in it. Run it when a login-lock alert, or "another Claude Code process is refreshing it", is seen again after PANEL-28 | As recommended (2026-10-16): a row after PANEL-46, started when a login-lock alert is seen again after PANEL-28 |
| PANEL-34 | A session cannot close its own lane: a `close-lane` skill and an authenticated `clauductor lane close` | WS-24, MN-4 (the page's Close lane, whose plan it reuses) | Not proposed | A row after PANEL-46 | The redesign neither does it nor removes the need: the page's Close needs the owner at the page, while a session closing itself is a CLI and skill change with no page in it. OPS-31's cancellation removed one user, not the need | As recommended (2026-10-16): a row after PANEL-46 |
| PANEL-44 | A version break that one project found is lost when the panel restarts | BN-3, FT-2 | Not proposed; it depends on PANEL-31 | A row after PANEL-46, with PANEL-31 (folded into its re-proposal) | It is server-side state and a residual of PANEL-31's own code, so the redesign does not touch it | As recommended (2026-10-16): folded into PANEL-31's re-proposal, after PANEL-46 |
| OPS-62 | A key typed while scrolled back can be lost to a wheel-up that tmux has not read yet | WS-16, WS-14 | Not proposed | A row after PANEL-46 | The terminal and its tmux bridge are outside PANEL-45 and PANEL-46 (proposal, *Out of scope*), so the redesign keeps the bug | As recommended (2026-10-16): a row after PANEL-46 |
| OPS-64 | The tab strip's "N more" can count one more tab than is out of sight | WS-3, MN-7 | Not proposed | A row after PANEL-46, which PANEL-46's chosen layout either keeps or cancels with its reason | Today's page already has a left rail and a tab strip, and D2 does not remove the strip, so only the chosen layout can say whether the count survives | As recommended (2026-10-16): kept, a row after PANEL-46. The hybrid keeps a lane tab strip (the filmstrip above an opened lane's terminal), so its "N more" count survives |

## The variants (D2, D6)

Each file is self-contained: its CSS, its script and the subset of `data.js` it draws are inline,
so it opens from disk with no network. Query parameters draw every state without a click (the
orchestrator's screenshots use them; `index.html` lists each shot's URL):

| File | Parameters |
|---|---|
| `1-stage.html` | `mode=light\|dark` (default: the system's), `screen=lane\|menu\|palette\|rules`, `lane=<name>` (default `working`) |
| `2-overview.html` | the same, and `view=home\|lane` (default `home`); `lane` defaults to `waiting`, the lane that needs you |
| `3-split.html` | `mode`, `screen`, and `term=wide\|narrow` (default `narrow`, lane `ready`; `wide` types into `working`) |
| `4-hybrid.html` | as `2-overview.html`: `mode`, `view=home\|lane`, `screen`, `lane` (default `waiting`) |

`screen=rules` opens the variant's rules panel (D3); otherwise the **Rules** button in the top bar,
or **R**, opens it. Clicking switches the same states; Escape closes.

**Sample data, marked as samples.** Pulse's routines, Today and next 14 days (D11, for PANEL-37 and
PANEL-38), and the per-lane top context contributors (for PANEL-50: rows such as *Read src/app.ts
18k*, scaled to each lane's real context against its 200k window). Everything else is
`data.js`: the projects, lanes, states, figures, alerts, pull requests, budget, quota, Flow and the
pinned cards. Lanes with no terminal capture of their own show the harness's banner for that
lane, which the fake prints for every lane. One more sample: the capture has no lane waiting
without blocking, so the idle `quiet` lane is drawn as **waiting** ("your move", amber) and says it
is a sample state; the project menu's and home's counts follow the lanes as drawn.

### Shared placement (all four variants)

What every variant does with the inventory. A variant's section below says only where it differs,
and lists every status-bar figure in full.

| Inventory | Where it goes |
|---|---|
| PG-1, PG-2, PG-3, PG-6, PG-8, PG-10 | Kept as they are; not drawn |
| PG-4, PG-5 | Kept: the Live mark at the sidebar's foot turns to Disconnected, and the bar (as of, Retry now) slides under the top bar; not drawn |
| PG-7 | Replaced by the motion rules: a chip crossfades on a change of state; a changed figure still flashes |
| PG-9 | Kept, less what D3 removes (theme and type) |
| PG-11 | Kept, restated per variant in its rules panel (what folds first) |
| **PG-12** | **Dropped by D3** (the owner's decision): it belongs to the TUI theme |
| NY-1, NY-2, NY-3 | Sidebar, *Needs you*: a card per ask (lane, kind, the ask, its age); a click shows the lane |
| NY-4, NY-5 | Sidebar, under *Needs you*: one amber row per alert, with its lane or *All lanes* and its age; **What to do** stays on a login-lock row |
| NY-6 | The lane itself turns waiting (amber, "your move"), drawn on the sample `quiet` lane; the capture has no *Your move* row |
| NY-7 | Kept: headings say "as of" while disconnected |
| BN-1, BN-3 | A notice under the top bar, one line each, with ×; not drawn (the capture has none) |
| BN-2 | The project menu: Zeta's row says *Config untrusted*, and its ⋯ holds **Trust config…** |
| BN-4 | Sidebar: the lost lane sits under the lanes with **Restore**; the project menu counts *1 to restore* |
| BN-5 | A toast at the bottom left, with **Dismiss**; not drawn |
| RAIL-1, RAIL-15 | Sidebar, *Lanes*: the count and **+** (New lane) |
| RAIL-3 | Kept as a note under *Lanes*; not drawn |
| RAIL-4 | Moved to Activity (⌘K or the sidebar's Activity), at its top |
| RAIL-5 | Sidebar: one row per lane, sorted by state; the state's dot, the name, one figure (age of the ask, context %, cost past budget, or the PR) |
| RAIL-7 | The lane's *Context* section: hit %, cold in mm:ss |
| RAIL-8, MN-4 | The ⋯ on a lane's row (on hover and when selected), the ⋯ in the terminal's header, and ⌘K under *Lane* |
| RAIL-9, RAIL-10, RAIL-11 | Behind ⌘K: **Worktrees** opens the tree as a sheet, with New lane here and Remove |
| RAIL-12 | The lane list fades at its foot and scrolls |
| RAIL-13 | Behind a key and ⌘K (⌘\\ hides the sidebar) |
| WS-2, SB-16 | **New lane**: the top bar's one primary button, the sidebar's **+**, and ⌘K (N), which offers *Up next* first |
| WS-4 to WS-8 | The terminal's header: name, state, branch, then model and effort, thinking, port, Remote, uptime |
| WS-9, WS-10, WS-11 | The lane's sections: *Context* (bar, auto-compact mark, %), *Run* (cost, the budget bar amber from 80% and red past 100%) |
| WS-13 to WS-19 | Kept: the terminal itself (outside PANEL-45 and PANEL-46) |
| WS-14 | The terminal's header: *↵ to type*, *Typing into …, Ctrl ] to leave* |
| WS-20, WS-21 | The terminal's header: **Interrupt** and Attach in Terminal.app |
| WS-22, WS-23, WS-24, WS-26 | The terminal's ⋯ and ⌘K: Restart…, Stop lane…, Close lane… (its plan), Remote control… |
| WS-25 | The lost lane: **Restore** in the sidebar; Resume, Forget and Close lane in its ⋯ |
| WS-27 | The terminal's foot: one line (the lane's activity, a note, an action's result) |
| WS-28, SP-17 | The terminal's empty state; not drawn |
| SP-1 | Replaced: no tabs. The lane's sections are Context, Pull request and Run |
| SP-2, SP-3 | *Run* (or *Agents*): what runs now, finished count, a 45 s strip; the full Gantt behind a click on it |
| SP-4, SP-5 | *Context* and *Run* carry the figures that matter; the rest (busy ratio, rate table, CPU and memory) behind a click on *Run* |
| SP-6, SP-7, SP-8 | *Pull request*: the PR, the merge-readiness verdict, a row per check, the diff (+412 −97) |
| SP-9 | The top bar's Gate figure opens the queue: this lane's place, **Cancel wait**, **Run in …** |
| SP-10 | The sidebar's alerts, filtered to the lane when one is selected |
| SP-11 | Activity, filtered to the lane (⌘K: Activity) |
| SP-12, SP-13, SP-14 | Pulse, *Pinned*: the project's pinned cards (real), read age, the stale note |
| SP-15 | Pulse, *Flow, 30 days* (real), which opens Metrics |
| DR-1, DR-2, SB-17, SB-18 | Activity and Metrics: the sidebar's nav (A, M) and ⌘K; each opens as a sheet |
| DLG-1 to DLG-5 | Kept as dialogs, restyled; opened from ⌘K and their buttons |
| MN-1, MN-2, MN-3 | The project menu, drawn open: from the project at the sidebar's top |
| MN-5 | Mode (System, Light, Dark) at the sidebar's foot and in ⌘K; text size in ⌘K. **Theme, Tuned and Type are dropped by D3** (the owner's decision) |
| FT-1, FT-2 | Behind a click on *Live* at the sidebar's foot: every counter |
| K-1 to K-3, K-5 to K-9, K-12, K-13 | Kept |
| K-11 | Only Metrics' tabs remain; the side tabs and card tabs are gone with SP-1 |
| New keys | ⌘K the palette; R the rules (in the mockups); ⌘J fold the sections; ⌘\\ hide the sidebar |

**PANEL-31's quiet version field** has its place at the sidebar's foot in every variant, beside
*Live*: "✓ 2.1.284" (the Claude Code version, verified on 2.1.284). It replaces BN-3's page-wide
amber bar for a version that is not yet verified.

### Variant 1, Stage (`1-stage.html`)

The terminal is always the main pane; the lane's three sections sit in a row beneath it, Pulse in
a column at the right.

- **Moved:** as the shared placement.
- **Behind a click or ⌘K:** as the shared placement.
- **Proposed drops (the owner's choice):**
  - WS-1 the terminal tabs, with WS-3, MN-7 and K-4: the sidebar's lane list is the only switcher
    (↑/↓ move in it). OPS-64 is then cancelled with its reason.
  - RAIL-2 and MN-6 the column picker: each row shows one figure, chosen by the lane's state.
  - RAIL-6 sorting by a column: the list sorts by state, then by last activity.
  - SP-16 the side panel's edge: the sections strip folds (⌘J) rather than resizes. RAIL-14 and
    K-10 stay for the sidebar's edge.

| Status-bar figure | Where it went |
|---|---|
| SB-1 | The project at the sidebar's top; it opens the project menu |
| SB-2 | The badge on the project's tile, and *Other projects* in the sidebar |
| SB-3 | *Live* at the sidebar's foot |
| SB-4 | The top bar's figure strip: *5h* and *7d*, each a bar, %, reset countdown; the plan and source on hover |
| SB-5 | A mark on the 5h bar when a burn rate is known (none in the capture) |
| SB-6 | The quota figures' empty state; not drawn |
| SB-7 | The strip: *Economy* (only while on); the roles on hover |
| SB-8 | Behind a click: the 5h figure and the strip's ⋯ |
| SB-9 | The strip: *Spend $60.55* |
| SB-10 | The strip: *Lanes*, a stacked bar and 9 |
| SB-11 | The sidebar's *Needs you* heading: the count and the oldest wait |
| SB-12 | The strip: *Gate free*; a click shows the holder's lane or the queue |
| SB-13, SB-14, SB-15 | Behind the strip's ⋯ |
| SB-16 | **New lane** in the top bar, **+** in the sidebar, ⌘K |
| SB-17, SB-18 | The sidebar's Activity and Metrics, and ⌘K |
| SB-19 | ⌘K only (Refresh) |
| SB-20 | The mode switch at the sidebar's foot; ⌘K |
| SB-21 | The ? key (K-1) and ⌘K (Help) |
| SB-22 | The strip's rule: it gives up Gate, then Lanes, then Economy into its ⋯ before it would wrap |

### Variant 2, Overview first (`2-overview.html`)

Home is a grid of lane tiles grouped by state; opening one brings its terminal forward.

- **Moved, beyond the shared placement:**
  - RAIL-5 also to home's tiles: needs you as a spotlight with its terminal ready, working tiles
    with a four-line preview, idle tiles compact, the lost lane and New lane last.
  - WS-1 and K-4 to the filmstrip above an open lane's terminal (←/→ move in it); WS-3 and MN-7
    become its scroll.
  - The lane's sections take the right column when a lane is open; Pulse is home's, and one click
    away (*Lane* / *Pulse*) in a lane.
  - SB-10 leaves the strip on home and in a lane: the tiles and the filmstrip are the count.
- **Behind a click or ⌘K:** as the shared placement.
- **Proposed drops (the owner's choice):** RAIL-2 and MN-6 (the column picker), RAIL-6 (sorting),
  and SP-16 (the side panel's edge), as in Stage.

| Status-bar figure | Where it went |
|---|---|
| SB-1, SB-2, SB-3 | As in Stage: the sidebar's top, the tile badge and *Other projects*, *Live* at its foot |
| SB-4, SB-5, SB-6, SB-7, SB-8, SB-9 | As in Stage: the figure strip, and its ⋯ for burn rate |
| SB-10 | Out of the strip: home's heading ("3 working, 1 needs you, 5 idle, 1 lost") and the filmstrip |
| SB-11 | The sidebar's *Needs you* heading, and home's spotlight |
| SB-12 | The strip: *Gate free* |
| SB-13, SB-14, SB-15 | Behind the strip's ⋯ |
| SB-16 | **New lane** in the top bar, **+** in the sidebar, the New lane tile on home, ⌘K |
| SB-17 to SB-21 | As in Stage |
| SB-22 | As in Stage |

### Variant 3, Split (`3-split.html`)

The lane list and the terminal trade width: the terminal is wide while you type and narrow when
its lane is idle. Both widths are drawn (`term=wide`, `term=narrow`).

- **Moved, beyond the shared placement:**
  - RAIL-5 to a table in the sidebar while the terminal is narrow: state, context, cost, PR, last
    activity per lane; RAIL-6 sorting stays, by a column heading.
  - The selected lane's sections open under its row in the table: Context and Pull request side by
    side, and Run as one line beneath them (WS-10 and WS-11 cost against the budget bar, SP-2 the
    agents running, time; SP-3 to SP-5 behind a click on it, as in the shared placement). While you
    type they shrink to one line under the terminal: context with its top two contributors, cost
    against budget, agents running, readiness.
  - Pulse folds to a rail of icons while you type, and unfolds when you leave the terminal.
  - *Other projects* leaves the sidebar while the table is shown; they stay in the project menu.
  - SB-10 leaves the strip while the table is shown, and the countdowns move into the quota
    figures' hover.
- **Behind a click or ⌘K:** as the shared placement.
- **Proposed drops (the owner's choice):**
  - WS-1 the terminal tabs, with WS-3, MN-7 and K-4, as in Stage.
  - RAIL-2 and MN-6 the column picker: the table's columns are fixed.
  - RAIL-13, RAIL-14, SP-16 and K-10 manual pane widths: the widths follow typing and idleness.
    ⌘\\ still hides the sidebar.

| Status-bar figure | Where it went |
|---|---|
| SB-1, SB-2, SB-3 | As in Stage; *Other projects* only while the terminal is wide |
| SB-4 | The strip: *5h* and *7d*; the reset countdowns on hover while the terminal is narrow |
| SB-5 to SB-9 | As in Stage |
| SB-10 | The strip while the terminal is wide; the table is the count while it is narrow |
| SB-11 | The sidebar's *Needs you* heading |
| SB-12 | The strip: *Gate free* |
| SB-13, SB-14, SB-15 | Behind the strip's ⋯ |
| SB-16 | **New lane** in the top bar while wide; the sidebar's **+** and ⌘K always |
| SB-17, SB-18 | ⌘K (A, M) only: Split's sidebar has no nav rows, so the lanes get the room |
| SB-19, SB-20, SB-21 | As in Stage |
| SB-22 | As in Stage, and narrower still while the terminal is narrow |

### Variant 4, Hybrid (`4-hybrid.html`): the owner's mix

The owner's named mix (D7, 2026-10-16): "I like the overview first, but in the stage view, I like
the boxes at the bottom around context, pull requests, and run." Home is Overview first's,
unchanged; a lane opened is laid out as Stage lays it out. Placed relative to Overview first:

- **Unchanged from Overview first:** home (RAIL-5's tiles and spotlight, SB-10 out of the strip,
  the New lane tile), the filmstrip (WS-1, WS-3, MN-7, K-4), the way back home (the crumb's All
  lanes, Esc, a filmstrip tile), and everything in the shared placement.
- **Moved, from Overview first's right column to one row of three boxes under the terminal, as in
  Stage:** WS-9 (Context, with the sample PANEL-50 contributors), WS-10 and WS-11 (Run: cost and
  the budget bar), SP-2 and SP-3 (Run: agents), SP-4 and SP-5 (Context and Run, the rest behind a
  click on Run), SP-6, SP-7 and SP-8 (Pull request), RAIL-7 (Context's cache line).
- **Moved back into the right column in a lane:** Pulse (SP-12 to SP-15 and the samples), which
  Overview first gave up to the lane's sections. Overview first's *Lane* / *Pulse* switch goes.
- **WS-12 (hide details)** becomes Stage's fold: ⌘J folds the three boxes and the terminal grows.
- **Proposed drops (the owner's choice):** as Overview first: RAIL-2 and MN-6 (the column picker),
  RAIL-6 (sorting), and SP-16 (the side panel's edge; the boxes fold instead).

| Status-bar figure | Where it went |
|---|---|
| SB-1 to SB-22 | As in Overview first: SB-10 stays out of the strip on home and in a lane |
