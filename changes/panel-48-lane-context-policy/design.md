# Design: panel-48-lane-context-policy

The evidence for every figure below is in `evidence/README.md`. It covers Standing Tee's week of
2026-10-08..14, $1,208.28 at list price, and the Claude Code docs as read on 2026-10-15.

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

- **D4 The default window is 200k.**
  - **The measurement:** it is the best of the three caps simulated on the baseline week: 150k saves
    $133.20, **200k $163.33**, and 300k $134.17. Each cap's cost includes the compaction calls.
  - **Named limits:**
    - On 2026-10-15's partial day, the cap lost money, because threads were cut at the window's
      edge.
    - The simulation does not model the quality cost of compaction, such as re-reading files it
      dropped. The outcome check watches review rounds and change-fail rate for that.
  - **Beat:** 150k and 300k.

- **D5 Connectors go off through `ENABLE_CLAUDEAI_MCP_SERVERS=false` in the settings `env`.**
  - **Why this switch:** it is the one documented variable for claude.ai's connectors. MCP servers
    the repository configures itself (`.mcp.json`) are untouched.
  - **What a repository can still do:** the docs say a `true` from any source wins. Read that way,
    a repository's own `env` setting `ENABLE_CLAUDEAI_MCP_SERVERS=true` turns the connectors back
    on despite `--settings`. Probe P3 tests exactly that (task 1.3), and `docs/panel.md` states the
    result: either the repository cannot override it, or it can, as a named limit (task 4.3).
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
  - **The alert and the warning colour:** the alert (`alerts.context_pct`) compares the new figure.
    The page's warning colour reads the configured threshold instead of the hard-coded 85
    (`panel.js:1118`, `:1401`).
  - **The tick:** the bar's auto-compact tick sits where probe P1 sees compaction fire, as a share
    of the window. It is today's inferred 95% until the probe says otherwise. The tick becomes a
    per-lane value: a lane launched with a window draws it at P1's share, and a lane without one
    keeps today's page-wide 95 (`Trends.AutocompactPct`).
  - **Lanes without a window:** they keep today's figures exactly.
  - **If the probe shows otherwise:** if P4 shows that Claude Code already reports
    `context_window_size` and `used_percentage` against the set window, the panel uses those
    figures, and the Decision log says so.
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
    - **P2:** `--settings` env beats the same key in the repository's `.claude/settings.json`
      `env`.
    - **P3:** `/context` at startup, with connectors on and then off. This measures D5's real
      saving, in place of the $2-per-1k bound. Then, with the repository's `.claude/settings.json`
      `env` setting `ENABLE_CLAUDEAI_MCP_SERVERS=true` against `--settings`' `false`: whether the
      connectors come back (D5).
    - **P4:** what the status line reports as `context_window_size` and `used_percentage` under the
      window (D7).
    - **P5:** a `--resume` that passes `--settings` again keeps the window.
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
    working directory is the main checkout. That is PANEL-45's finding, raised for a fix row at
    session close.
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

## Refusals
| Situation | What happens instead |
|---|---|
| P1 shows no compaction near the window | The build stops after group 1 and reports to the owner (a design issue): the window is not built on a variable that does nothing |
| A config sets `context_window` outside 100000–1000000, or a `subagent_model` that fails `LaunchOptRe` | The config does not load, and the error names the key, as an invalid `model` does today |
| Managed settings on the machine set the same variable | Managed settings rank above `--settings`, so they win. That is Claude Code's precedence, and the panel does not try to override it |
| An attached lane has no status line | No bar is shown (unchanged). Reading its transcript is PANEL-49 |
| The build's cost reaches $40 | The owner is told at once, and the build goes on (soft budget) |
