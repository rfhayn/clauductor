# Design: panel-48-lane-context-policy

The evidence for every figure below is in `evidence/README.md`. It covers Standing Tee's week of
2026-10-08..14, $1,208.28 at list price, and the Claude Code docs as read on 2026-10-15.

**Amended 2026-10-17, after group 1's probes** (`evidence/README.md`, *Probe results* and
*The cap re-run at the compaction point*). P1 showed that Claude Code compacts at the window less
a fixed 33k buffer, not at a share of the window. The owner decided the tick that day (D7). D4's
saving is re-stated at the point where a 200k window really compacts. D13, the alert below the
tick, is new and is the owner's decision of the same day. D1, D5, D7 and D8 take in P2 to P5.
Each amended decision says so.

## Decisions

- **D1 The policy travels as `--settings '<json>'` on every launch, built in `LaneCommand`.**
  - **The JSON:** it holds an `env` object only. It is built with `json.Marshal` from validated
    config values, never from repository text, and is one argv element. tmux execs a command of two
    or more arguments directly, so no shell reads it.
  - **Every launch:** Start, Restart, Resume and Restore (and `launchSession`'s fast-exit retry)
    all build their argv in `LaneCommand`. So the settings go on every launch, which `--resume`
    needs: it does not restore `--settings`.
  - **Why `--settings`:** it ranks above the repository's project and local settings. A
    repository's own `env` block therefore cannot undo the policy, while it could undo an exported
    variable ("the settings file value applies in most sessions").
  - **Confirmed 2026-10-17:** P2 saw `--settings`' 100000 beat the repository's own 1000000 and
    300000. P5 saw a `--resume` keep the window only when `--settings` was passed again.
  - **Beat:**
    - an exported env var, which the repository's settings override;
    - a settings file written per lane under `~/.clauductor`, which means a file to write, name and
      clean up for what one argv element carries.

- **D2 The config keys: `launch.context_window`, `launch.claudeai_connectors`, and per lane type
  `lane_types.<t>.context_window` and `lane_types.<t>.subagent_model`, in config version 6.**
  - **`context_window`:** an integer from 100000 to 1000000, Claude Code's own range for
    `CLAUDE_CODE_AUTO_COMPACT_WINDOW`. A value outside it fails the config's load, naming the key.
  - **`claudeai_connectors`:** a boolean. `false` turns claude.ai's connectors off.
  - **`subagent_model`:** must match `LaunchOptRe`, as `model` does today.
  - **Precedence:** a lane type's `context_window` overrides `launch.context_window`.
  - **Beat:**
    - per-type keys only, which repeat one number for every type when the measured gain of a
      per-type window is $0.26 a week (propose lanes at 300k);
    - a free-form `launch.env` map, which would let a repository-controlled `panel.json` set any
      variable in a lane. `lane_types` is read without a trust check (D11).

- **D3 No hidden default.** A config without these keys launches lanes exactly as today: no
  `--settings`.
  - **Who writes the default:** `panel init` and `panel attach` write
    `"launch": {"context_window": 200000, "claudeai_connectors": false}` and declare version 6.
  - **Existing configs:** this repository's `.clauductor/panel.json` and the template's preset
    (`template/.clauductor/panel.json`) get the same keys, so the operating model's own lanes run
    the policy and show the calibrated bar (D7).
  - **Beat:** a built-in default for every lane once the panel is upgraded. That would change how
    each registered project's lanes run the moment the binary changes, with nothing in its config
    saying so.

- **D4 The default window is 200k. (Amended 2026-10-17: the saving is re-stated, and the window
  stays 200k by the owner's decision of that day.)**
  - **What a 200k window does:** Claude Code compacts it at about 167k, the window less its fixed
    33k buffer (P1). The proposal simulated compaction AT the cap, so its $163.33 is what a 233k
    window would save, not a 200k one.
  - **The measurement, re-run at the compaction point:** the same simulation on the same week,
    with each window's cap at the window less 33k (`evidence/README.md`, *The cap re-run*):

    | Window | Compacts at | Net saving per week |
    |---|---|---|
    | 150k | 117k | −$31.73 (a loss) |
    | 183k | 150k | $133.20 |
    | **200k** | **167k** | **$153.79** |
    | 233k | 200k | $163.33 |
    | 300k | 267k | $145.48 |

    Each cap's cost includes the compaction calls. Together with the lean startup, keep-alive and
    batched wakes, the combined figure is $286.41 (23.7%), against $286.24 at a 200k cap.
  - **Why 200k still:** it is within $9.54 a week (0.8% of the week) of the best simulated window,
    233k, and the owner kept it. The 2026-10-15 reason holds: 150k and 300k windows save less.
  - **Named limits:**
    - On 2026-10-15's partial day, the cap lost money, because threads were cut at the window's
      edge.
    - The simulation does not model the quality cost of compaction, such as re-reading files it
      dropped. The outcome check watches review rounds and change-fail rate for that.
    - A small window loses money: at 150k the compaction calls cost more than they save.
      `docs/panel.md` says so where it lists the key (task 4.3).
  - **Beat:**
    - 233k, the best measured, which the owner declined on 2026-10-17;
    - 150k and 300k.

- **D5 Connectors go off through `ENABLE_CLAUDEAI_MCP_SERVERS=false` in the settings `env`.**
  - **Why this switch:** it is the one documented variable for claude.ai's connectors. MCP servers
    the repository configures itself (`.mcp.json`) are untouched.
  - **What a repository can still do (amended 2026-10-17 with P3):** the docs say a `true` from
    any source wins, which read as letting a repository's own `env` turn the connectors back on.
    P3 found it does not on 2.1.302: with `--settings` at `false`, the repository's `true` left all
    six claude.ai connectors off. `docs/panel.md` states that, and the version it was observed on
    (task 4.3).
  - **What it saves (P3):** 1.4k tokens at startup, about $2.80 a week at section 3's $2 per 1k,
    below the $4–10 first estimated. `/context` reads low, so the real figure may be larger.
  - **Beat:** `--strict-mcp-config` with an empty `--mcp-config`. It would drop the repository's
    own servers, and whether it drops connectors at all is not verified.

- **D6 The subagent model is an option per lane type, off by default.** It sets
  `CLAUDE_CODE_SUBAGENT_MODEL` only, not `CLAUDE_CODE_SUBAGENT_MODEL_FORCE`.
  - **What that means:** an agent definition in the repository that names its own model keeps it.
    The built-in Explore and Plan agents and the main model are untouched.
  - **The money:** up to $151.78 a week if every subagent ran on Sonnet 5.5, on price alone. The
    effect on quality is not measured, and the owner turns it on per lane type if they choose.
  - **Beat:**
    - forcing it, which overrides the repository's own agent choices;
    - leaving the option out, which the owner asked to keep (the roadmap row).

- **D7 A lane's bar and context alert measure against the window it was launched with.**
  - **Recording it:** the effective window is set on the lane's record by the two functions that
    write it, on every launch: `Start`, in the record it passes to `Registry.Begin`, and
    `resumeLocked` (Restart, Resume and Restore), on `rec` before its `Registry.Begin`. Both later
    `Done` the same record whole, so a window set anywhere else would be overwritten. `model` and
    `effort`, by contrast, are written once, at creation (`withTemplate`). `LaneCommand` stays a pure
    argv builder, since `launchSession`'s fast-exit retry calls it twice. A config edit therefore
    moves a lane's bar only once the lane is relaunched.
  - **The figure:** for a lane with a recorded window, context is `current_usage`'s input, cache
    creation and cache read tokens, divided by that window. Today the figure is Claude Code's
    `used_percentage`, which is measured against the model's whole window. With a 200k window on
    a 1M-token model, it would read 18% at 180k, and the alert would never fire.
    - **Three of the state's figures change (amended 2026-10-17):**
      - the session's `CtxPct`, which the alert, the lane row and the lane's detail read;
      - the metrics' `CtxPct`, which is merged across a lane's sessions, drawn in the metrics
        view and read by the trends;
      - the metrics' `CtxWindow`, which the metrics view draws as the window. It comes from
        `context_window_size`, which P4 saw stay at 1000000 under a set window. For a lane with a
        recorded window it carries that window, so a 200k lane at 180k reads "90% … 200k window"
        and not "1000k window". A lane without one keeps `context_window_size`.
    - **Confirmed by P4 (2026-10-17):** under a 100000 window the status line still read
      `context_window_size` 1000000 and `used_percentage` 6 at 64,912 tokens. `current_usage`'s
      three counts summed to 64,912, which reads 65% against the window. So the calibration is
      needed, and the bar stays a share of the window the lane was launched with.
  - **The alert and the warning colour (amended 2026-10-17):** the alert compares the new figure
    with the lane's own alert threshold (D13), and the page's warning colour reads that threshold
    from the state instead of the hard-coded 85 (`panel.js:1118`, `:1401`).
  - **The tick (amended 2026-10-17, the owner's decision):** the bar's auto-compact tick sits at
    `(window − 33000) / window` of the bar: 83.5% for a 200k window, 89% for 300k, 67% for 100k.
    - **Why:** P1 saw compaction fire at the window less a fixed 33k buffer, the same 33k at 100k,
      200k, 300k and 1m, not at one share of every window. The proposal's "P1's share" would
      have drawn a 200k lane's tick at 67%.
    - **The same on every model:** P1 ran on Sonnet, and this repository's lanes run Opus. Read
      again under `--model opus`, the buffer was 33k at 100k, 200k, 300k and 1m, and a Sonnet
      control read the same (`evidence/README.md`, *P1 on Opus*). So one buffer serves every lane.
    - **Where the 33000 lives:** one named constant in `state`, with its source (P1 and its Opus
      reading, Claude Code 2.1.302) beside it. The status line has no field for it.
    - **Named limit:** a Claude Code release that changes the buffer moves compaction and not the
      tick. The outcome check reads where compactions fired (`compact_boundary`'s `preTokens` in
      the transcripts) against the window less 33k.
    - **Named limit:** a lane whose model's own window is smaller than the configured window. Its
      compaction point under the setting is not verified, so the panel does not clamp the window
      to the model's. It draws the tick from the configured window as for any other lane.
    - **Named limit:** compaction under Opus was inferred from `/context`'s buffer, not observed
      by a fill as P1 observed it on Sonnet. The outcome check's `preTokens` reading covers it.
    - **A lane without a window** keeps today's page-wide 95 (`Trends.AutocompactPct`). P1 puts a
      1M window's compaction at 96.7%, so 95 stays a close approximation, and leaving it keeps
      today's figures exactly.
    - **Beat:** the percentage measured against the compaction point instead of the window, which
      the owner declined: the bar would no longer read as a share of the window the lane was
      launched with.
  - **Lanes without a window:** they keep today's figures exactly.
  - **Beat:**
    - reading the config at display time, which moves a running lane's bar on an edit it has not
      launched with;
    - keeping `used_percentage`, which hides the policy's effect.

- **D8 The build starts with probes, because three points are only in the docs or not in them at
  all.**
  - **Who runs them, and where:** the orchestrator, outside the sandbox.
    - It uses a throwaway git repository in the session's scratchpad, on its own tmux socket
      `p48-probe`, never `clauductor` or `standingtee`.
    - It runs with the owner's own login (a probe needs a real session), on Sonnet, at the
      smallest window (100000).
    - It spends at most $5 of the budget.
  - **The probes:**
    - **P1:** a window set through `--settings` env makes compaction fire near it on the installed
      Claude Code (2.1.300). It records at what share of the window, which places the tick (D7).
      (Superseded: D7. P1 found a fixed 33k buffer, not a share.)
    - **P2:** `--settings` env beats the same key in the repository's `.claude/settings.json`
      `env`.
    - **P3:** `/context` at startup, with connectors on and then off. This measures D5's real
      saving, in place of the $2-per-1k bound. Then, with the repository's `.claude/settings.json`
      `env` setting `ENABLE_CLAUDEAI_MCP_SERVERS=true` against `--settings`' `false`: whether the
      connectors come back (D5).
    - **P4:** what the status line reports as `context_window_size` and `used_percentage` under the
      window (D7).
    - **P5:** a `--resume` that passes `--settings` again keeps the window.
  - **Results (2026-10-17, on Claude Code 2.1.302, for $0.25):** P1 fired, at the window less 33k
    (D4, D7). P2 and P5 confirm D1. P3 confirms D5 and prices it. P4 confirms D7's calibration.
    After the amendment's review, `/context` under `--model opus` reported the same 33k buffer
    at 100k, 200k, 300k and 1m (D7).
  - **Beat:**
    - building on the docs alone, where `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is absent from
      `claude --help`;
    - probes in the gate, which would need the owner's login and real spend on every run.

- **D9 One change in four groups, not two rows.**
  - **The size:** it writes six surfaces (lanes, config, install, state, web and docs), more than
    the "about three" a single review round is sized for.
  - **Why keep it whole:** the calibration has to ship with the window. In this repository's lanes a
    status line posts, so a window without calibration would show a bar that reads low and an
    alert that never fires.
  - **How it stays reviewable:** each group is one builder's and one reviewer's worth (`tasks.md`).
  - **Beat:** splitting the calibration into a row after this one, which ships the misleading bar
    in between.

- **D10 Risk normal. Budget $40, soft (the owner, 2026-10-15): reaching it is reported to the
  owner and the build goes on.**
  - **Why normal risk:** it changes the argv of every lane whose config opts in, and the owner's
    live lanes run on the panel.
  - **How the build runs:** through `apply-change` by hand. `build-change.js` stops on its budget,
    and its gate fails in the sandbox until OPS-72.
  - **How the cost is checked:** before each group, the build prices its agents' transcripts
    directly as well as running `change-cost.sh`, which misses agents spawned in a session whose
    working directory is the main checkout. That is PANEL-45's finding, owned by OPS-74 (#137).
  - **Beat:** a hard $40 stop, which the owner declined.

- **D11 The policy needs no trust check, as `model` and `effort` need none today.**
  - **What a config can do with it:** the new keys yield only validated values into three named
    variables. An untrusted in-repository config can at most choose a window in Claude Code's own
    range, turn connectors off, or name a subagent model. That is the reach `lane_types.<t>.model`
    already has.
  - **Beat:** gating the policy on trust. An attached project is untrusted until its owner trusts
    it, so its lanes would launch uncapped in the meantime, though the sidecar is the owner's own
    file.

- **D12 The evidence is kept with the change.**
  - **What is kept:** `evidence/README.md` holds the measurement, the controls verified in the
    docs, the ranked levers and what could not be measured. `evidence/tool/` holds the three audit
    scripts the measurement ran (`usage-report.mjs`, `audit.mjs`, `driver.mjs`), with no
    transcript data.
  - **How the scripts arrive:** they come in with group 1 (task 1.5), so review sees them. The
    transcript folder becomes an argument, since today it is a hard-coded path naming the owner's
    home.
  - **Why:** the outcome check re-runs the same measurement.
  - **Beat:** leaving the tool in a session scratchpad, which is deleted with the session.

- **D13 A windowed lane's context alert sits at the lower of `alerts.context_pct` and its tick
  less 5 points. (The owner's decision, 2026-10-17.)**
  - **The problem:** `alerts.context_pct` defaults to 85, and a 200k lane compacts at 83.5% (D7).
    Its context drops before it reaches 85, so the alert would never fire.
  - **What it gives:** at the default 200k window, 78.5%: 157k, 10k before compaction at 167k.
    At 300k, 84% (15k before). At 100k, 62% (5k before). At 1m, the 85 default, as today.
  - **A set value still governs where it is earlier:** this repository's and the template's
    `context_pct` of 80 give 78.5% at 200k and 80% at 300k.
  - **Off stays off:** `alerts.context_pct` 0 turns the alert off (`AlertConfig`), and it stays
    off for every lane. The lower of 0 and the tick would be 0, which the colour would read as
    "always warn", so 0 is taken before the rule: no alert and no warning colour (LANECTX-2-S7).
  - **A lane without a window** keeps `alerts.context_pct` exactly as today.
  - **The alert's text** shows the lane's threshold to one decimal where it has one ("alert at
    78.5%"), and as today where it is whole ("alert at 85%").
  - **Why:** it fires before compaction at every window the config allows, with no new key and
    no change for a config without a window.
  - **What it costs:** the threshold becomes a per-lane value, like the tick, so the state
    carries it per lane and the page's warning colour reads the lane's own (tasks 4.1, 4.2).
  - **Under the cap, compaction becomes routine:** about 130 a week across the lanes at 167k
    (D4's re-run), so the alert fires before most of them. Context alerts stay on the page and
    never notify (`docs/panel.md`, *Notifications*), so this is a busier Alerts panel, not
    interruptions.
  - **Beat:**
    - a fixed default of 80 for every lane (`DefaultContextPct` 85 → 80). It is one page-wide
      figure and the value this repository and the template already set. But a window of 165k or
      less compacts at or below 80% (the config allows 100k), so there the alert never fires:
      the silence D7 exists to end;
    - a margin in tokens rather than points, such as 20k before compaction. It gives the same
      headroom at every window, but fires at 47% of a 100k window.

## Refusals
| Situation | What happens instead |
|---|---|
| P1 shows no compaction near the window | The build stops after group 1 and reports to the owner (a design issue): the window is not built on a variable that does nothing |
| A config sets `context_window` outside 100000–1000000, or a `subagent_model` that fails `LaunchOptRe` | The config does not load, and the error names the key, as an invalid `model` does today |
| Managed settings on the machine set the same variable | Managed settings rank above `--settings`, so they win. That is Claude Code's precedence, and the panel does not try to override it |
| An attached lane has no status line | No bar is shown (unchanged). Reading its transcript is PANEL-49 |
| The build's cost reaches $40 | The owner is told at once, and the build goes on (soft budget) |
