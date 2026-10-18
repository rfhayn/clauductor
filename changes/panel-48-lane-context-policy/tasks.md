# Tasks: panel-48-lane-context-policy

## Progress
- 2026-10-16 proposed; nothing built yet
- 2026-10-17 group 1 built: probes P1–P5 run and recorded in `evidence/README.md` *Probe results*,
  and the audit tool committed under `evidence/tool/`. The working tree is not yet committed.
  - **Group 2 waits for the owner:** P1 contradicts the design's model of the tick, as the
    Decision log's last entry says. D7 and LANECTX-2-S5 need the owner before group 4, and D4
    may need them before group 2 or 3.
- 2026-10-17 design amended for P1 (D4, D7, new D13; D1, D5 and D8 take in P2–P5), with
  LANECTX-2-S1, S4 and S5 amended, S6 and S7 added, and tasks 4.1–4.3 to match. The approval of
  2026-10-16 no longer covers the design: **groups 2–4 wait for the owner's re-approval**.
- 2026-10-17 re-approved (design 8bcd6850fbb2). Group 2 built, uncommitted: config version 6 with
  the four keys, `LaneCommand`'s `--settings`, and the window on the lane's record at `Start` and
  `resumeLocked`. Its full gate and review are the orchestrator's. Groups 3 and 4 are left.
- 2026-10-17 group 2 (The config and the launch (D1, D2, D6, D11)) built and reviewed: converged in 1 round(s), peak none; grades R1 pass
- 2026-10-17 group 3 built, uncommitted: init and attach write the default `launch` object at
  version 6, and both presets (and the plugin's scaffold) carry it. Its gate and review are the
  orchestrator's. Group 4 is left.
- 2026-10-17 group 3 (Init, attach and the presets (D3, D4)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass
- 2026-10-17 group 4 built, uncommitted: the calibrated figures, the per-lane tick and threshold
  in `state` (`state/context.go`), the page's `context-colour.js`, and `docs/panel.md`'s *The
  context policy*. Its gate and review are the orchestrator's. The Slice line is left.
- 2026-10-17 task 4.4 built, uncommitted (D7 amended with P6): the status line posts
  `auto_compact_window`, `signals.StatusPayload` reads it, `recordWindow` takes it first and
  has no folder fallback, and tests cite LANECTX-2-S8 and S9. Group 4's gate and review are the
  orchestrator's. The Slice line is left.
- 2026-10-17 group 4 (The bar and the alert (D7)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass

## Decision log
- 2026-10-17, group 1: **The probes read the window from `/context`'s "Auto-compact window" and
  "Autocompact buffer" lines, and filled a session to compaction only once, in P1.**
  - **The alternative:** filling a session to compaction in P2, P5 and at each window.
  - **Why:** in P1, `/context`'s buffer predicted where compaction fired, 67k against a
    `preTokens` of 67,216. So it reads the effective window without spending a fill each time.
- 2026-10-17, group 1: **P2 and P3 each got a control case beyond the task's.**
  - **P2:** the repository's value was 300000, alone and against `--settings`.
  - **P3:** the repository set `false` with no `--settings` key.
  - **The alternative:** only the cases tasks.md names.
  - **Why:** a repository value of 1000000 is indistinguishable from the 1M default. Without a
    control, P2 and P3 could have passed without the repository's settings being read at all.
- 2026-10-17, group 1: **`audit.mjs` takes the main checkout's transcript folder as a fourth
  argument. It finds the worktree folders beside it, and lists scratchpad sessions by the folder
  name's last segment.**
  - **The alternative:** a home-relative default.
  - **Why:** D12 says no home path is written in. Over 2026-10-14..15 its three output files were
    byte-identical to the original's.
  - **What stays:** `normPath`'s `~/Development/standingtee` pattern holds no one's home. Making it
    general is not in 1.5, so the README names it as a limit.
- 2026-10-17, group 1: **Not decided: raised to the owner as a design issue.** P1 shows that
  compaction fires at the window minus a fixed 33k, not at a share of it.
  - **The facts:** the same 33k buffer appeared at 100k, 200k, 300k and 1m. P1's share, 67%, holds
    only at 100k. At the default 200k window, compaction fires at 83.5%.
  - **What it breaks:**
    - LANECTX-2-S5's "P1's share of its window" would draw a 200k lane's tick at 67%.
    - The default 85% alert would sit above a 200k lane's compaction point, so it would never
      fire.
    - D4's $163.33 simulated compaction at 200k, but a 200k window compacts at about 167k.
  - **The choices are the owner's:**
    - the tick at `(window − 33000) / window`, or the percentage measured against the compaction
      point;
    - whether the alert's default changes;
    - whether the default window stays 200k or becomes 233k, to keep D4's measured cap.
- 2026-10-17, **the owner's decision** on the entry above: the tick sits at
  `(window − 33000) / window` of the bar, the bar stays a share of the window, and the default
  window stays 200k. design.md D7 and D4 are amended to it.
  - **The alert, the owner's decision of the same day (D13):** a windowed lane's alert sits at
    the lower of `alerts.context_pct` and its tick less 5 points. The alternative, a fixed
    default of 80, was beaten: it never fires at a window of 165k or less.
  - **D4 re-run:** the evidence tool's caps were run at the window less 33k, in a scratchpad copy
    of `driver.mjs` whose cap list and combined cap were made settable; the committed tool is
    unchanged. A 200k window saves $153.79 a week, not $163.33 (`evidence/README.md`, *The cap
    re-run*).
- 2026-10-17, after the amendment's review: **P1's buffer was read again under `--model opus`,
  in print mode (`claude -p … "/context"`), not tmux.**
  - **The alternative:** the interactive `/context` in tmux, as group 1 did.
  - **Why:** the sandbox refused the tmux socket, and running outside it was refused. A Sonnet
    control at 200k read P1's own 33k, so print mode reports the same figure.
  - **Found:** 33k on Opus at 100k, 200k, 300k and 1m. D7 keeps one buffer, and cites the Opus
    reading.
- 2026-10-17, after the amendment's review: **two context figures are calibrated, not one.**
  - `Session.CtxPct` feeds the alert, the lane row and the lane's detail. `Metrics.CtxPct` is
    merged across sessions, drawn in the metrics view and read by the trends.
  - Tasks 4.1 and 4.2 name both sites, and LANECTX-2-S1 asserts both figures.
  - `alerts.context_pct` 0 keeps the alert off for a windowed lane too (LANECTX-2-S7). Without
    that, the lower of 0 and the tick would read as "always warn".
- 2026-10-17, group 2: **`launch.claudeai_connectors: true` puts nothing in `--settings`; only
  `false` sets `ENABLE_CLAUDEAI_MCP_SERVERS`.**
  - **The alternative:** passing `"true"`, which would override a repository's own `false`.
  - **Why:** D5 defines only `false`. `true` is Claude Code's behaviour with no key, and D3 wants
    a config that asks for nothing to launch as today.
- 2026-10-17, group 2: **The window keys are `*int`, so an explicit `0` fails the load like
  any other value outside 100000–1000000; an absent key sets nothing.**
  - **The alternative:** a plain `int`, where `0` would silently mean "no window".
  - **Why:** D2 says a value outside the range fails the load, naming the key.
- 2026-10-17, group 2: **At version 5, `launch.context_window` is refused with an error naming
  `"launch"` and version 6** (LANECTX-3-S2's "names the key").
  - **The alternative:** changing `checkVersion` to name the deepest key set.
  - **Why:** `launch` is new in version 6 as a whole. The gate already names the outermost new
    key, as it does for `templates`, and the message tells the owner what to change.
- 2026-10-17, group 2: **The registry reads a recorded `contextWindow` outside 100000–1000000 as
  a corrupt record** (`validateRecord`), which is never launched.
  - **The alternative:** trusting the field, since it never becomes argv.
  - **Why:** group 4 divides by it. Every other record field is checked on load, and a value the
    panel never writes means the file was edited.
- 2026-10-17, group 2: **The policy is read in `config`, through `ContextWindowOf` and
  `LaunchEnv`. `LaneCommand` marshals it and puts `--settings` after `--effort`, before
  `--remote-control` and `-n`.**
  - **The alternative:** building the env in `lanes`.
  - **Why:** `Start` and `resumeLocked` need the same window that `LaneCommand` passes, so one
    function gives both.
- 2026-10-17, group 2: **The tests reach LANECTX-1-S1, S2 and LANECTX-2-S3 through the real
  `Start`, `Restart`, `Resume` and `RestoreAll`, on a fake tmux that keeps each launch's argv.
  They test S3 to S6 on `LaneCommand`.**
  - **Why:** S2 depends on every launch path reaching `LaneCommand`. S3 to S6 are about the argv
    alone.
  - **Also:** `docs/panel.md`'s hand-written version table and its "outside 1–6" line moved to
    version 6, beside the generated reference.
- 2026-10-17, group 2 (review fix): **`TestLaneRecordsTheWindowOfEachLaunch` no longer cites
  LANECTX-2-S3; it names itself that scenario's launch-side precondition, in a form the trace's
  ID pattern does not match. Task 2.2's last line says so.**
  - **The alternative:** keep the citation and rely on 4.1's test asserting the readings.
  - **Why:** the scenario's THEN is a context reading (90% at 180000, then against 300000), and
    this test asserts only the recorded window. Cited here, the trace would read CITED even if
    group 4 never asserted the reading. Task 4.1 already lists S3, so the trace now waits for it.
- 2026-10-17, group 3: **The starter's default is two named constants in `install`
  (`starterContextWindow`, `starterClaudeaiConnectors`), and `launch` is written after `base`.**
  - **The alternative:** an exported default in `config`.
  - **Why:** D3 puts the default in the file init writes, never in what the panel reads, so
    nothing in `config` may hold it. After `base` is where both presets put it too.
- 2026-10-17, group 3: **LANECTX-3-S1's test attaches a real repository, reads the sidecar's raw
  JSON, and builds each of its lane types' argv with `LaneCommand` under the loaded config. It
  does not start a lane on tmux.**
  - **The alternative:** a real `Start` on a fake tmux, as group 2's S2 test does.
  - **Why:** group 2's S2 test already proves every launch path reaches `LaneCommand`, and its
    fake tmux is the `lanes` package's own test code. The raw JSON is read so that a misspelt
    key cannot pass as an absent one. Without the starter's `launch`, the test fails (run).
- 2026-10-17, group 3: **`clauductor update` was run as `--dry-run --template-dir` this
  worktree's `template/`, and not applied.**
  - **The alternative:** the plain `framework/clauductor update` that 3.2 names.
  - **Why:**
    - Plain, it read the MAIN checkout's template (`CLAUDUCTOR_FRAMEWORK`), not this branch's.
    - Against this branch's template it reports every skill, hook and template up to date. Its
      only offers are `docs/practices/` and a `model-roles.json` key, which are missing on
      `origin/main` too, so they predate this change and are not group 3's.
    - It does not manage `.clauductor/panel.json`, which 3.2 edits by hand.
    - Applying it writes `.claude/settings.json`, which the sandbox refuses.
- 2026-10-17, group 4: **The window is read at each status post, from the lane record holding
  the post's session id, and the session keeps the window its figure was measured against.**
  - **The alternative:** reading the record when the view is computed.
  - **Why:** at view time a restart would divide the old process's tokens by the new window
    before the relaunched lane posts. The cost: a post that arrives before the registry's first
    poll (a panel restart) reads `used_percentage` until the next post.
- 2026-10-17, group 4: **A windowed session's post without `current_usage` keeps its last
  figure; it never falls back to `used_percentage`.**
  - **Why:** `used_percentage` is against the model's whole window (P4), so a fallback would
    flip the bar between two scales. A corrupt record counts as no window.
- 2026-10-17, group 4: **The per-lane marks are `ctxTick` and `ctxAlertPct` on the lane row
  (`LaneView`), the terminal lane (`TermLaneView`) and `Metrics`, taken from the session the
  figure comes from.** A terminal lane that has not posted takes them from its record.
  - **The alternative:** one pair per lane, from its record.
  - **Why:** a worktree can hold more than one session, and the figure is the highest of them.
    Its marks must be that session's.
- 2026-10-17, group 4: **On the page, a missing threshold colours nothing; only the tick falls
  back to `trends.autocompactPct`.** The lane row takes its figure and its marks from the same
  view (`ContextColour.laneMarks`), as `laneCtx` picks the figure.
  - **The alternative:** falling back to 85, as before.
  - **Why:** the server sets a threshold wherever it sets a figure, and both come from the same
    binary. Task 4.2 names a fallback for the tick alone.
- 2026-10-17, group 4: **`docs/panel.md` gets a *The context policy* section under *Lanes*, and
  the launch argv there gains `[--settings '{"env":{…}}']`**, which group 2 left out.
- 2026-10-17, group 4: **LANECTX-2-S3's test uses a config whose window is already 300000,
  with the lane's record at 200000.**
  - **Why:** the state never reads the config's window. A test that edits the config would prove
    nothing more. The restart is the record moving to 300000; group 2's test proves that.
- 2026-10-17, group 4: **The `web` and `panel` packages' short tests ran outside the sandbox.**
  - **Why:** the sandbox refuses local port binds (`bind: operation not permitted`). Both
    passed there.
- 2026-10-17, group 4 (review): **A status post's window is its session's lane record's, by
  session id, else by the worktree the session is bound to; where several records share that
  worktree, the smallest window.**
  - **The alternative:** the session id alone, as first built. Or matching the lane's process
    by PID (`claude agents` against the tmux pane), which is a new mechanism.
  - **Why:** `/clear` gives the lane's process a new session id that no record holds. By id
    alone, that session read against the model's window and its alert sat past compaction.
    The status post carries no lane or PID. So a non-lane session in a windowed lane's
    worktree is also measured against the window, as is the owner's own session in the root
    when a windowed lane runs there. Those alert early. The smallest-window rule errs the
    same way: a figure read high alerts early, and one read low never alerts.
  - **Superseded** by the owner's amendment of D7 (P6) and task 4.4: the folder fallback is gone.
- 2026-10-17, group 4 (4.4): **`auto_compact_window` decodes as `signals.PostedInt`, which reads
  any value that is not a JSON integer (a string, a fraction, `null`) as 0 instead of failing the
  decode.**
  - **The alternative:** a plain `*int64`, where a status line that posts `"200000"` would fail
    `ParseStatus` and cost the post its context, cost and quota.
  - **Why:** the field comes from a project's own status line, not Claude Code; `RateLimits`
    already decodes leniently for the same reason.
- 2026-10-17, group 4 (4.4): **The status line posts the field only for digits with no leading
  zero, through `jq -c --argjson`, and posts the stdin unchanged if jq fails or prints nothing.**
  The docs snippet does the same.
  - **The alternative:** any digits, or splicing the text without jq.
  - **Why:** JSON forbids a leading zero, so `--argjson` would refuse one; splicing breaks on an
    empty object. jq is already the template status line's dependency.
- 2026-10-17, group 4 (4.4): **`.claude/statusline.sh` and `.claude/checks/statusline.sh` were
  copied from `template/` by hand, not by applying `clauductor update`.**
  - **The alternative:** `clauductor update --template-dir template`, applied.
  - **Why:** as in group 3, applying it also adds `docs/practices/` and a `model-roles.json` key
    that predate this change, and writes `.claude/settings.json`, which the sandbox refuses. Its
    dry run, before the copy, named exactly these two files as modified; after it, none.
- 2026-10-17, group 4 (4.4): **The status-line checks unset `CLAUDE_CODE_AUTO_COMPACT_WINDOW`
  before their cases, and `install`'s snippet test strips it from the environment it passes.**
  - **Why:** a lane runs them under its own window, which would add the field to every post and
    fail the cases that expect none.
  - **Also:** the snippet test gained cases for a 200000 window (posted) and `200k` (not posted).
    The check's window cases use their own 30 s live pid, since the first one's 5 s have passed
    by then. Against HEAD's status line, the check's 200000 case fails (run). With the posted
    window ignored in `recordWindow`, S8's test fails at 18% (run).

## 1. The probes (D8)
The orchestrator runs these outside the sandbox. It uses a throwaway git repository in the
session's scratchpad, tmux socket `p48-probe`, Sonnet, a 100000 window, and at most $5. It
records the results in `changes/panel-48-lane-context-policy/evidence/README.md` under
*Probe results*, and in the Decision log.
- [x] 1.1 P1 and P4: start claude in the probe repository with `--settings '{"env":{"CLAUDE_CODE_AUTO_COMPACT_WINDOW":"100000"}}'`, and a `statusLine` that appends its stdin to a file.
  - Fill the context past 100k, then record where compaction fires (the `PreCompact` moment against the last status post's tokens), and what `context_window_size` and `used_percentage` read.
  - If no compaction fires near the window, stop here and report to the owner (Refusals).
  - (manual: needs the owner's login and real spend)
- [x] 1.2 P2: put `"env": {"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "1000000"}` in the probe repository's settings file (.claude/settings.json there, not this repository's), launch with the `--settings` of 1.1, and confirm the settings' 100000 applies. (manual: as 1.1)
- [x] 1.3 P3: the /context command at startup with connectors on, then with `ENABLE_CLAUDEAI_MCP_SERVERS=false` in the settings `env`; record both totals and the difference. Then put `"env": {"ENABLE_CLAUDEAI_MCP_SERVERS": "true"}` in the probe repository's settings file (as in 1.2), launch with the `false` settings, and record whether the connectors come back (D5). (manual: as 1.1)
- [x] 1.4 P5: exit the 1.1 session, relaunch it with `--resume <id>` and the same `--settings`, and confirm the window still applies. (manual: as 1.1)
- [x] 1.5 Commit the audit scripts the measurement ran into `changes/panel-48-lane-context-policy/evidence/tool/` (D12): `usage-report.mjs`, `audit.mjs` and `driver.mjs`.
  - The transcript folder becomes an argument; no home path is written in.
  - No transcript data or CSV is committed.
  - A README says how the outcome check re-runs them.
  - (manual: an evidence tool for the outcome check, not shipped code)

## 2. The config and the launch (D1, D2, D6, D11)
- [x] 2.1 Config version 6 in `framework/internal/panel/config/fields.go` and `config.go`.
  - New keys: `launch.context_window`, `launch.claudeai_connectors`, `lane_types.*.context_window` and `lane_types.*.subagent_model`, with their validation.
  - Tested in `framework/internal/panel/config/config_test.go`, citing [LANECTX-1-S7].
  - Version gating tested in `config/version_test.go`, citing [LANECTX-3-S2].
  - Regenerate `docs/panel.schema.json` and the reference table in `docs/panel.md`, as `config/generated_test.go` requires.
- [x] 2.2 `LaneCommand` in `framework/internal/panel/lanes/lanes.go` builds `--settings` from the lane's effective policy, with `json.Marshal` over the validated values, on every launch.
  - `LaneCommand` stays a pure argv builder. `Start` (in the record passed to `Registry.Begin`) and `resumeLocked` (on `rec` before its `Registry.Begin`) set the effective window on the lane's record on every launch (D7).
  - Tested in `framework/internal/panel/lanes/lanes_test.go`, citing [LANECTX-1-S1], [LANECTX-1-S2], [LANECTX-1-S3], [LANECTX-1-S4], [LANECTX-1-S5] and [LANECTX-1-S6].
  - Also in `lanes_test.go`: a lane started at a 200000 window, its config edited to 300000, then restarted, has 300000 in its record: the launch-side precondition of LANECTX-2's third scenario, which 4.1's test cites (Decision log).

## 3. Init, attach and the presets (D3, D4)
- [x] 3.1 `framework/internal/panel/install/init.go`'s starter writes `"launch": {"context_window": 200000, "claudeai_connectors": false}` and version 6.
  - Tested in `framework/internal/panel/install/attach_test.go`, citing [LANECTX-3-S1].
  - `install/init_test.go`'s expected configs and ATTACH-1-S2's test still pass.
- [x] 3.2 Add the same `launch` object, and version 6, to `.clauductor/panel.json` and `template/.clauductor/panel.json`.
  - Rebuild the plugin with `scripts/build-plugin.sh`.
  - Run `framework/clauductor update` from the repository root with the checkout's binary.

## 4. The bar and the alert (D7)
- [x] 4.1 `framework/internal/panel/state/model.go` computes a lane's context against its recorded window: `current_usage`'s input, cache creation and cache read tokens over the window.
  - Both of the state's context figures:
    - the session's `CtxPct`, set from the status post in `model.go` (today `:629-632`), which the alert, the lane row and `panel.js:1118` read;
    - the metrics' `CtxPct`, set in `Metrics.fold` (today `:958-962`), merged across a lane's sessions (`:1080`), drawn at `panel.js:1401` and read by `state/trends.go` (`:241`).
  - The metrics' `CtxWindow` (set from `context_window_size` in `Metrics.fold`, today `:968`, merged at `:1081`, drawn at `panel.js:1401-1402`) carries the lane's recorded window (D7).
  - Lanes with no recorded window keep `used_percentage` in both figures and `context_window_size` in `CtxWindow`.
  - The tick becomes a per-lane value: a lane with a recorded window carries `(window − 33000) * 100 / window` (D7), computed in one division, so 83.5 and 89 come out exact (`0.89 * 100` would give 89.00000000000001). The 33000 is one named constant in `state` that cites P1, its Opus reading and Claude Code 2.1.302. A lane without one keeps `Trends.AutocompactPct` (`state/trends.go`).
  - Tests compare the tick and the threshold against exactly representable values, or within a tolerance.
  - The alert threshold becomes a per-lane value too (D13): a lane with a recorded window carries the lower of `alerts.context_pct` and its tick less 5; a lane without one carries `alerts.context_pct`. An `alerts.context_pct` of 0 is carried as 0, off, for every lane. The alert in `state/alerts.go` compares the new figure with the lane's own threshold.
  - The alert's text (`alerts.go`, today `%.0f` at `:225`) shows the threshold to one decimal where it has one ("alert at 78.5%") and whole otherwise ("alert at 85%").
  - Tested in `framework/internal/panel/state/alerts_test.go` and `state/metrics_test.go`, citing [LANECTX-2-S1] (both figures and the window), [LANECTX-2-S2], [LANECTX-2-S3], [LANECTX-2-S5], [LANECTX-2-S6] and [LANECTX-2-S7].
- [x] 4.2 The page draws the context warning colour from each lane's own alert threshold, which the state carries (task 4.1), not a hard-coded 85, at both draw sites: the lane's context at `panel.js:1118` (the session's figure) and the metrics view's at `:1401` (the metrics' figure).
  - A threshold of 0 draws no warning colour.
  - The decision is a pure function in its own static file, which `panel.js` calls, as `term-links.js` is.
  - `panel.js` draws each lane's own auto-compact tick from the state (task 4.1), in place of the page-wide value at `:1118` and `:1401`. It falls back to `trends.autocompactPct` only for state from an older server, since 4.1 gives every lane a tick.
  - The tick's choice sits in the same pure file as the colour, and the same node test covers it.
  - Tested under node, as `web/term_links_test.go` runs `term-links.js`, in a new `web/context_colour_test.go`, citing [LANECTX-2-S4], [LANECTX-2-S6] and [LANECTX-2-S7].
- [x] 4.3 `docs/panel.md` describes the launch policy (the keys, `--settings`, and what a repository cannot override) and the calibrated bar, and names the limits.
  - The bar: the tick at the window less Claude Code's 33k buffer, and the alert below it (D13); the alerts table's `context` row says what a windowed lane compares.
  - `alerts.context_pct`'s `Doc` in `framework/internal/panel/config/fields.go` (`:146-147`) says a windowed lane alerts at the lower of it and the tick less 5, and 0 is off. It is generated into `docs/panel.md`'s reference table and `docs/panel.schema.json`, so regenerate both. Also fix `AlertContext`'s comment in `state/alerts.go` (`:25`), which still says `used_percentage`.
  - The limits: managed settings win; a repository's own `ENABLE_CLAUDEAI_MCP_SERVERS=true` does not turn the connectors back on, as observed on Claude Code 2.1.302 (P3); a Claude Code release that changes the 33k buffer moves compaction and not the tick (D7); a small window loses money, 150k by $31.73 a week in the simulation (D4); and an attached lane with no status line shows no bar until PANEL-49.
- [x] 4.4 The session reports its own window, and the folder fallback goes (D7, amended 2026-10-17 with P6).
  - The status line posts the window: `template/.claude/statusline.sh` adds `"auto_compact_window": <n>` at the top level of the JSON it posts when `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is digits, and posts the stdin unchanged otherwise. The line it prints does not change.
    - Bring this repository's `.claude/statusline.sh` along with `framework/clauductor update`, and rebuild `plugin/` with `scripts/build-plugin.sh`.
    - `docs/panel.md`'s status-line snippet (between `<!-- statusline begin -->` and its end marker) and its prose say what the field is and why a repository's own status line should post it.
    - A case in `template/.claude/checks/statusline.sh` (and its plugin copy, through the rebuild): the curl stand-in records the posted body. With the variable set to 200000 the body holds `auto_compact_window` 200000; unset, and set to a non-number, it holds no such key.
  - `signals.StatusPayload` (`framework/internal/panel/signals/hooks.go`) reads `auto_compact_window`.
  - `framework/internal/panel/state/context.go`: the window a post is measured against is the posted `auto_compact_window` when it is within `config.MinContextWindow`–`MaxContextWindow`; else the record carrying the session id; else 0. `model.go` (today `:635`) passes the post's window in place of `s.Lane`.
  - Remove `recordWindow`'s folder fallback (the loop over records by `rec.Path`, and the comment that defends it), and update its doc comment and the comment at the `model.go` call site.
  - Tests in `framework/internal/panel/state/alerts_test.go`, replacing the `"cleared"` case at today's `:309`:
    - a post under a new session id carrying `auto_compact_window` 200000 reads against 200000, citing [LANECTX-2-S8];
    - a post under a session id no record holds, in a windowed lane's worktree and with no `auto_compact_window`, reads `used_percentage` and raises no context alert, citing [LANECTX-2-S9];
    - a posted window outside the range is ignored, and the record by id applies.

- [ ] Slice: an owner can start a lane in an attached repository from the panel at http://127.0.0.1:<port> and see it launched with a 200k context window and connectors off
