# Design: ops-42-routines

## Where the behaviour lives today

- **Nothing runs while the owner is away.** The only scheduled job is the weekly dependency audit,
  a GitHub workflow (`.github/workflows/dependency-audit.yml`). Everything else starts from a
  session the owner opens.
- **What a routine would reuse:**
  - **Health lines** (`.claude/health/README.md`): every `*.sh` in `.claude/health/` runs at
    session start and prints verdict lines (`OK`, `FAILED`, `STALE`, `STUCK`, `RUNNING`,
    `NEVER RAN`, `CANNOT CHECK`). The directory is the project's; a thin caller puts the logic
    in a model script that `clauductor update` keeps current (`lib/health.sh` is the pattern).
  - **The git common dir** already holds the gate's lock (`<common>/clauductor/gate.lock`,
    `scripts/ci/run-local.sh`), so it is shared by every worktree and never committed.
  - **The lease's liveness test** (`scripts/ci/lease.sh`: `lease_alive`, `lease_pstart`,
    `lease_proc_dead`): a pid plus its start time, so a reused pid is not mistaken for the holder.
  - **A headless `claude -p` call** (`.claude/evals/run.sh`, verified on real runs in OPS-16):
    `--model`, `--effort`, `--output-format json`, `--no-session-persistence`,
    `--setting-sources project`, `--permission-mode dontAsk`, `--allowedTools`,
    `--max-budget-usd`, with the binary named by `EVAL_CLAUDE` so the checks use
    `fake-claude.sh`. It reads `total_cost_usd` from the JSON.
  - **The panel's quota file** (`~/.clauductor/panel/quota.json`, `state.Quota`, written by the
    panel from status-line posts): `windows[].{key, pct, resetsAt}` (seconds) and `at`
    (milliseconds). It is the account's, so it is machine-wide by nature.
  - **The panel's subscription rule** (`docs/panel.md`, *Subscription only*): a lane is refused
    while `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, and the lane command unsets both.
  - **PANEL-28's stop sequence** for a `claude` child (approved design D2): SIGTERM, then
    SIGKILL 15 s later, because SIGKILL is the one stop that guarantees a login refresh in
    progress cannot release its lock.
  - **One place for models** (`.claude/model-roles.json`, held by `checks/model-roles.sh`).
  - **The doc tier** (`framework/internal/template/tier.go`): a template file that is not a
    script directly under `.claude/` nor under a framework directory is created only if missing.
    `.claude/routines.json` falls there by rule, so `clauductor update` never overwrites it; a
    script directly under `.claude/` (`routines.sh`) is the model's and is refreshed.
- **What does not exist:** a registry, a runner, a run log, caps, or any process that starts work
  on a clock. The panel ticks nothing (PANEL-37 will).

## The shape of the change

1. **The registry, `.claude/routines.json`** (the project's file; the template ships it with the
   default caps and no routines):

   ```json
   {
     "version": 1,
     "caps": { "daily_usd": 1.00, "window_usd": 5.00, "window_days": 7, "daily_runs": 6, "quota_pct": 60 },
     "routines": [
       { "name": "nightly-checks", "schedule": "02:30", "catch_up": "once",
         "run": { "command": "sh .claude/checks/run.sh" }, "timeout_s": 1800 },
       { "name": "morning-digest", "schedule": "07:00", "catch_up": "once", "output": "show",
         "run": { "prompt": "…", "context": "sh .claude/skills/session-start/context.sh",
                  "role": "routine", "allowed_tools": ["Read", "Grep", "Glob"],
                  "max_turns": 6, "max_usd": 0.50 },
         "timeout_s": 600 }
     ]
   }
   ```

   Each routine: `name` (kebab-case, unique), `schedule` (D3), `run`, and optionally `where`
   (`local`, the default and the only value this runner runs: the field a cloud runner can
   follow, as PRD-maps asks), `machine` (`any`, the default, `darwin` or `linux`), `timeout_s`
   (default 900), `catch_up` (`once`, the default, or `skip`), `output` (`log`, the default, or
   `show`: the latest output is named in the health line and is what PANEL-38 shows), `writes`
   (default false) and `enabled` (default true).

   `run` is either `{command}` (a shell command run with `sh -c` in the repository's root; it
   costs no tokens) or a `claude -p` routine: `{prompt | skill, context?, role, allowed_tools,
   disallowed_tools?, max_turns, max_usd}` (D6, D7). `context` is a command whose stdout is put
   into the prompt, so the deterministic part runs before the model and the model needs no Bash.

2. **The runner, `.claude/routines.sh`** (POSIX sh and jq, ADR-0006), with these verbs:

   | Verb | What it does |
   |---|---|
   | `tick` | Takes the lock (D5), validates the registry, records the tick, and runs every routine that is due, one after another. Exits 0 when routines fail (their verdicts are in the log); non-zero only when the registry is invalid. |
   | `run <name>` | Runs one routine now (trigger `manual`), whatever its schedule; everything else applies. Exits with the run's outcome. |
   | `list` | Each routine: schedule, kind, next due, last verdict, and which runner or machine it waits for when it is not this one's. |
   | `log [<name>] [--days N] [--json]` | The run records, newest first. `--json` is the form PANEL-37 and PANEL-38 read. |
   | `validate` | The registry's checks, shared by `tick` and `checks/routines.sh`. |
   | `health` | The health lines (D10). `health/routines.sh` is a thin caller. |
   | `ticker [--launchd | --cron]` | Prints the panel-free ticker (D11). Installs nothing. |

   Global options `--registry FILE` and `--repo DIR` choose a registry outside the repository
   (D12). The runner finds its libraries beside itself, so the plugin's copy works as well as the
   project's.

3. **The run log and outputs** (D2): `<git common dir>/clauductor/routines/`:
   `runs.jsonl` (append-only), `tick.lock/`, `last-tick`, `ticker.log` and `out/<name>/<run id>/`
   (`stdout`, `stderr`, and for `claude -p` the JSON answer and `result.md`). A run is two
   records: `start` (`id`, `name`, `kind`, `trigger`, `slot`, `started`, `commit`, `host`, `pid`)
   and `end` (`verdict` of `ok`, `failed` or `timeout`, `exit`, `ended`, `duration_s`,
   `changed` paths, and for `claude -p`: `cost_usd` or null, `budget_usd`, `turns`, `model`,
   `effort`, `subtype`). A refusal is one `refused` record (`reason`: `capped`, `quota` or
   `api-key`, with the figures). A start with no end whose holder is dead is closed as failed,
   "the runner died", by the next tick that takes the lock.

4. **The health line, `.claude/health/routines.sh`**: one line per enabled routine for this
   machine, and one for the ticker (D10).

5. **The check, `.claude/checks/routines.sh`**: validates this project's registry (its process
   check), and holds every scenario of `specs/routines` against fixtures with a fake claude
   (D13). `checks/lib.sh` puts a refusing `claude` first on every check's PATH.

6. **A role, `routine`**, in `.claude/model-roles.json` (sonnet, low): "unattended routines
   (`claude -p`); read-only unless the routine says `writes: true`" (D6).

7. **This repository's three routines** in its own `.claude/routines.json` (D14), with the
   scripts they need under `scripts/routines/`.

8. **ADR-0005 amended** (D8) and **docs**: `template/docs/playbook.md` gains *Routines* (the
   registry, the verbs, the ticker line, the caps, reading the log); AGENTS.md's table gains one
   row, in the template and in this repository:
   `| Scheduled work stays within its caps, and every run reaches a reader | **`.claude/routines.sh`** (the lock, the caps, the quota, the run log) via the ticker; **`health/routines.sh`**; **`checks/routines.sh`** |`.

## Refusals

| Situation | What happens instead |
|---|---|
| The registry does not parse, or any entry is invalid | Nothing runs. `tick` exits 2 naming each fault; the health line says FAILED for the registry. A malformed entry is an error, not a shorter list. |
| Another tick or run holds the lock | `tick` exits 0 at once, naming the holder and since when; `run` exits 75 with the same words. Nothing is logged. |
| `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set | Each due `claude -p` routine is refused and logged `api-key` naming the variable; command routines run with both unset (D16). `CLAUDE_CODE_OAUTH_TOKEN` is the subscription and is allowed. |
| A cap is reached | The `claude -p` routine is refused and logged `capped` with the cap and the figure. |
| The quota is at or above `quota_pct` (current reading) | The `claude -p` routine is refused and logged `quota` with the window. An unknown reading never refuses. |
| A routine's `where` is not `local`, or its `machine` is another OS | It is not this runner's: skipped silently, shown by `list`. |
| A sidecar registry declares `writes: true` | Validation fails (D12). |
| `jq` is missing | `tick` exits 2; the health line says CANNOT CHECK. |
| The `claude` binary is missing | The run fails, "claude is not installed", logged; the tick carries on. |

## Decisions (awaiting the owner)

**D1. One change, or two.**
- **Recommended: one change, in five groups.** Re-measured (`/propose` step 0c): the row's four
  files are all new, one capability, no MODIFIED requirement. It also touches `checks/lib.sh`
  (the refusing claude, D13), `model-roles.json` (one role, D6), ADR-0005 (D8), the playbook,
  and AGENTS.md's table. Each group is one builder's and one reviewer's worth.
- **Alternative: split** into OPS-42 (command routines: registry, runner, lock, schedule, log,
  health, ticker, sidecar, `nightly-checks` and `flake-watch`) and a new row for `claude -p`
  routines (role, caps, quota, cost, ADR-0005's amendment, `morning-digest`).
- **Why not split:** the registry's schema, the log's records and the health line must carry the
  `claude -p` fields from the first version either way, and the caps are why the registry exists.
  A split costs a second proposal review for a seam that is one group wide. The split stays
  available: groups 1–3 alone are a working command-only runner.

**D2. Where the run log and outputs live.**
- **Recommended: `<git common dir>/clauductor/routines/`**, beside the gate's lock. Every
  worktree sees the same log, it is never committed, `git clean` in a worktree cannot reach it,
  and it needs no panel. Outputs: the last 30 runs per routine; the log is never pruned (a
  record is about 300 bytes). A sidecar registry keeps the same layout beside itself (D12).
- **Alternatives:**
  - `<git common dir>/routines/`, PRD-maps' literal path: unnamespaced, so another tool's
    directory could collide with it;
  - `~/.clauductor/projects/<hash>/routines/`: machine-wide and readable by the panel, but a path
    the repository does not own, and a second place to look for a non-attached project;
  - committed under `docs/`: every run would be a commit, conflicting with every squash.

**D3. The schedule syntax, and catch-up.**
- **Recommended:** three forms, local time, read at a 5-minute grain:
  - `HH:MM` (daily);
  - `<days> HH:MM`, days as `Mon,Thu` or a range `Mon-Fri` (three-letter English names);
  - `every <N>m` (N ≥ 5) or `every <N>h`, measured from the last run's start.
- **Catch-up,** per routine: **`once`** (the default) runs a missed slot once at the next tick,
  however many slots were missed, logged `catch-up` with the slot it covers. **`skip`** runs only
  within 15 minutes of the slot (three ticks), and otherwise waits for the next one.
- **Alternatives:**
  - 5-field cron: familiar, but its catch-up needs enumerating past occurrences, the parser is a
    larger POSIX-sh surface, and the first routines need only a daily time;
  - catch-up `all`: a sleeping laptop would wake to a burst of the same run.
- **Why:** the grammar covers the three routines and the cadences PRD-maps names, and a
  `version` bump can add cron later without breaking a registry.

**D4. The caps: defaults and scope.**
- **Recommended: per registry, in its own run log**, with these defaults: `daily_usd` 1.00,
  `window_usd` 5.00 over `window_days` 7, `daily_runs` 6, `quota_pct` 60.
  - Caps count `claude -p` runs only. A command costs no tokens and is never capped.
  - **The quota is the machine-wide guard**: it is the account's, read from the panel's
    `quota.json` when there is one. A reading counts when it is under 6 hours old and its window
    has not reset; otherwise the quota is unknown, and unknown never refuses (the panel's quota
    guard's rule), but the record says so.
  - **The budget given** to each run is the least of its `max_usd` and what the daily and window
    caps leave, passed as `--max-budget-usd`, so one run cannot cross a cap by more than claude's
    own overshoot. A run whose cost is unknown counts at that budget.
  - **A machine-wide dollar cap across projects** (PRD-maps' open question) is **PANEL-37's**:
    the panel is the one process that ticks every project. This PR adds that to PANEL-37's row.
- **Alternatives:**
  - a machine-wide ledger in `~/.clauductor/routines/` that every project's runner reads and
    writes: one cap across projects without the panel, but a shared file every project must lock,
    outside every repository, for a case (several projects with costly routines and no panel)
    nobody has yet;
  - caps on command routines too: they spend no money, and their schedule bounds them.
- **Why these numbers:** the digest is expected at $0.05–0.30 a run on sonnet at low effort, so
  $1 a day is three to twenty runs with room for one mistake, and $5 a week stops a daily
  routine that costs ten times what was expected within a week. 60% leaves the owner at least
  40% of the 5-hour window when they sit down.

**D5. The tick's lock, and an overlapping tick.**
- **Recommended: one lock per registry, held for the whole tick, routines run one at a time.**
  A tick that finds a live holder exits 0 at once, saying who holds it; the next tick picks up
  what is still due. `run <name>` takes the same lock and is refused while it is held. A holder
  that is dead or whose pid was reused (`lease.sh`'s pid-plus-start-time test, reused, not
  rewritten) is taken over, and its unfinished run is closed as failed.
  - The lock is a directory (`mkdir` is atomic) with `owner.json`: pid, start time, host, the
    routine running, since when.
  - A routine's timeout bounds how long a tick can hold it.
- **Alternatives:**
  - a lock per routine, different routines running at once: concurrent spend makes the caps
    inexact, and two routines would contend for the same tree;
  - waiting for the lock (the gate lease's queue): ticks would pile up behind a long run, each
    holding a process;
  - the gate lease itself: it serialises gates, not routines, and a routine that runs the gate
    (`flake-watch`) would wait on itself.

**D6. Which model and effort a `claude -p` routine runs on.**
- **Recommended: a role.** A routine names a role (`"role": "routine"`), and the runner reads
  that role's model and effort from `.claude/model-roles.json` at run time. The change adds the
  role `routine` (sonnet, low). Validation fails on a role that does not exist. Nothing is
  restated, so `checks/model-roles.sh` needs no new table.
- **Alternative: `model` and `effort` in the registry,** as PRD-maps sketched, restated and held
  by `checks/model-roles.sh` like the panel's `lane_types`.
- **Why:** AGENTS.md: "A model or effort is chosen in ONE place." A routine reads the file
  itself, so there is no reason to restate it, and a role's model change already goes through
  rule 13's evidence when the role has a suite.

**D7. How a `claude -p` routine's tools are restricted, and checked.**
- **Recommended:**
  - **The invocation,** the shape `evals/run.sh` already runs (verified in OPS-16), plus the two
    flags that make the tool set exact: `claude -p <prompt> --output-format json --model <m>
    --effort <e> --max-turns <n> --max-budget-usd <b> --no-session-persistence
    --setting-sources project --permission-mode dontAsk --tools <allowed> --allowedTools
    <allowed> --disallowedTools <disallowed> --strict-mcp-config`, run in the repository's root,
    with stdin from `/dev/null` and both API key variables unset. Never `--bare`, which never
    reads the subscription login.
  - **Why each bound** (Claude Code's docs; sources and what is inferred below):
    - `--tools` "restricts which built-in tools Claude can use": a tool left out is not in the
      model's context, so a project's `permissions.allow` (say `Bash(git log *)`) cannot hand it
      back. `--allowedTools` alone only pre-approves; it restricts nothing.
    - `--disallowedTools` with a bare name "removes the matching tools from Claude's context",
      and a deny beats an allow at every scope. Unless the routine says `writes: true`, the
      runner always adds `Bash`, `Edit`, `Write`, `MultiEdit` and `NotebookEdit`, whatever the
      registry lists: a second layer.
    - `--strict-mcp-config` with no `--mcp-config` loads no MCP server: `--tools` does not
      govern MCP tools.
    - `--permission-mode dontAsk` "auto-denies every tool call that would otherwise prompt"; the
      docs say to set it explicitly under `-p`.
  - **The check uses an allowlist, not a list of write tools.** Without `writes: true`, every
    entry in `allowed_tools` must be one of `Read`, `Grep`, `Glob`, `WebFetch`, `WebSearch`.
    `Bash` in any form, `Agent`, and a tool the runner does not know need `writes: true`. An
    `mcp__` tool is refused outright: no MCP server is loaded, so it could never be reached.
  - **`writes: true`** is the owner's opt-in, visible in review. The run still records every
    path it changed. A run without it that changes the tree is a failed run (ROUTINES-4-S3), for
    command routines too: the main checkout stays clean.
  - **`context`** gives the model what it would otherwise need Bash for: the command runs first,
    under the same timeout, and its stdout is appended to the prompt.
  - **`skill`** runs `/<skill>` as the prompt; validation fails when `.claude/skills/<skill>/`
    does not exist.
- **Alternatives:**
  - the registry's lists alone as `--allowedTools`, with no `--tools` and no forced disallow:
    under `dontAsk` a settings allow rule still runs, so a routine called read-only could run
    `Bash(git log *)` or worse;
  - `--setting-sources user`: no project permissions at all, but the project's hooks, its merge
    guard included, would not load either;
  - a list of write tools in the check: a new or misspelt tool would pass as read-only.
- **What the docs say, and what is inferred.** Explicit: `--tools` restricts the built-in set;
  a bare name in `--disallowedTools` removes the tool from context; deny beats allow at every
  scope; `--tools` does not govern MCP tools; `dontAsk` denies anything that would prompt; set the
  mode explicitly under `-p`; in a folder never trusted, `-p` does not apply the project's
  `permissions.allow`. Inferred, and so not relied on:
  - that `--tools` keeps a settings-allowed tool out. The forced `--disallowedTools` covers every
    write tool by an explicit rule, so a read-only routine stays read-only either way;
  - that `--tools Read,Grep,Glob` gives all three (the docs say the default set on macOS and
    Linux leaves out `Glob` and `Grep`). The digest's input comes from `context`, so `Read` alone
    is enough;
  - the CLI's `subtype` names when `--max-turns` or `--max-budget-usd` stops a run
    (`error_max_turns`, `error_max_budget_usd` are documented for the Agent SDK, and the CLI's
    snake_case fields by examples). The runner treats any subtype but `success`, and any
    `is_error: true`, as failed, and records the name it got.
  - **How they are checked:** in the build, with the fake claude only (D13), answering each
    documented and inferred shape: every subtype, `is_error`, no JSON, exit 143 with no result.
    No check runs the real one. The first scheduled `morning-digest`, a production run, records
    the real argv, subtype, cost and turns in the run log; task 5.5 reads that record.
- **Sources:** code.claude.com/docs/en/cli-reference, /en/headless, /en/permissions,
  /en/permission-modes, /en/agent-sdk/agent-loop and /en/agent-sdk/cost-tracking.

**D8. A timeout, and ADR-0005.**
- **Recommended:** the runner starts each routine as a child it holds by the pid `$!` gave it,
  made a process-group leader where the shell allows (`set -m`). At `timeout_s` it sends SIGTERM
  to that child (and its group), then SIGKILL 15 s later if it is still running: PANEL-28's
  sequence, so a `claude` refreshing the login gets the chance to finish. The docs say a
  SIGTERMed `claude -p` stops starting tool calls and model requests, kills its Bash process
  trees, runs its `SessionEnd` hooks and exits 143, writing no result
  (code.claude.com/docs/en/headless). Its record therefore says `timeout`, cost unknown, counted
  at its budget (D9). The runner never looks a
  process up by name, argv or parent.
  - **ADR-0005 is amended**, because its decision reads "No script in the model kills … a
    `claude` process … whatever its argv or parent says", and the timeout stops a `claude -p`.
    The amendment adds one sentence: *"Except a process the script started itself and holds by
    the pid it got from starting it, stopped with SIGTERM and, 15 s later, SIGKILL; it is never
    found by name, argv or parent."* Its enforcement gains ROUTINES-5-S3 (a process with the same
    argv survives).
- **Alternatives:**
  - no timeout, relying on `--max-turns` and `--max-budget-usd`: a hung network read is bounded
    by neither, and the tick's lock would be held for ever;
  - SIGKILL at the timeout: the one stop that strands a login refresh's lock (PANEL-28 D2);
  - a timeout that only logs and leaves the child running: the lock would still be held.

**D9. How cost is recorded.**
- **Recommended:** from the JSON answer only: `total_cost_usd` and `num_turns`, with `is_error`
  and `subtype`. A run that gives no readable JSON is recorded with cost unknown and counted at
  its budget: a SIGTERMed run writes no result (exit 143), and on `error_max_budget_usd` the
  docs say `total_cost_usd` still includes the call that crossed the budget. The run log is the only record: a headless run
  has no status line, so it never reaches the panel's spend ledger, and with
  `--no-session-persistence` it leaves no transcript for `change-cost.sh` either. PANEL-38 adds
  the run log to Today's cost.
- **Alternatives:**
  - keeping the session (no `--no-session-persistence`) so transcripts count it: the routine
    would then show in `change-cost.sh` and `/resume` lists as if it were the owner's work;
  - estimating from tokens at list price: the JSON already carries Claude Code's own figure.

**D10. The health line.**
- **Recommended:** one line per enabled routine for this machine, newest run first:
  - `OK routine nightly-checks — 2026-10-13 02:30 on a1b2c3d, 41 s`;
  - `FAILED routine flake-watch — exit 1 at 2026-10-13 03:30 on a1b2c3d; output <path>`;
  - `CAPPED routine morning-digest — refused at 07:00: daily cap $1.00 reached ($1.02)`;
  - `RUNNING routine flake-watch — since 03:30 (pid 4211)`;
  - `NEVER RAN routine morning-digest — due since 2026-10-13 07:00`;
  - `STALE routine nightly-checks — last ran 2026-10-10 02:30; 3 slots missed`;
  - a `show` routine's OK line names its latest output (`… ; digest <path>`).
  - **The ticker:** `OK routines ticker — last tick 02:35` while ticks arrive at least hourly,
    else `STALE routines ticker — last tick <when>: nothing runs the routines. Start the panel
    (PANEL-37) or install the line \`sh .claude/routines.sh ticker\` prints`. `NEVER RAN` before
    the first tick.
  - With nothing enabled: `OK routines — none enabled in <registry>`.
- **Alternative:** one summary line for all routines: shorter, but "1 of 3 failed" makes the
  reader open the log to learn which, and a green summary names no subject.

**D11. The panel-free ticker.**
- **Recommended: OPS-42 prints it; the owner installs it.** `routines.sh ticker --launchd`
  prints a LaunchAgent plist (`StartInterval` 300, `WorkingDirectory` the repository, `PATH`
  as the current shell has it so `claude`, `jq` and `git` resolve, output appended to
  `ticker.log`), and `--cron` prints a crontab line; with no flag it picks by `uname`. The
  playbook's *Routines* section says how to load and remove each. The health line names a
  stopped ticker (D10).
- **Alternatives:**
  - `routines.sh ticker --install`, writing `~/Library/LaunchAgents/` and loading it: a change to
    the machine outside the repository, unattended spend switched on by a script, and a second
    ticker once PANEL-37 ticks too (harmless under the lock, but confusing);
  - docs only: nothing to test, and a hand-copied plist drifts.
- **Why:** rule 3 needs a production caller without the panel (ADR-0006), and printing it gives
  one the check can hold (ROUTINES-9-S1) while installing stays the owner's act.

**D12. An attached repository's registry.**
- **Recommended: in OPS-42, as `--registry` and `--repo`.** When the registry lies outside the
  repository, the lock, log and outputs go beside the registry
  (`~/.clauductor/projects/<id>/routines/`), commands run in the repository, `writes: true` is
  refused, and the tree check (ROUTINES-4-S3) still runs, so a command that writes there fails.
  The runner is the plugin's copy or a clauductor checkout's; the ticker line takes the same
  flags. PANEL-37 ticks it from the panel.
- **Alternative: leave the sidecar to PANEL-37.** The row and PRD-maps put it here, and the
  options are a few lines of path resolution in a runner that has to be location-independent for
  the plugin anyway.

**D13. A test never starts the real claude.**
- **Recommended: two layers.**
  - The runner's binary is `ROUTINES_CLAUDE`, else `claude`. `checks/routines.sh` always sets it
    to a fake in its scratch directory, which records its argv and answers as each scenario
    needs.
  - **`checks/lib.sh` puts a refusing `claude` first on PATH for every check**, present and
    future. It writes a marker and exits 97; `finish` fails a check whose run left the marker.
    So a check that forgets its fake fails loudly and spends nothing (ROUTINES-11-S1, run first
    against a check without the guard to see it reach the stub).
- **Alternative: the stub in `checks/routines.sh` only.** It would protect this check and leave
  the next one to remember, which is how #93 and #95 happened.
- **Why:** the owner's rule of 2026-10-12. No check today starts the real claude (`evals.sh` uses
  `fake-claude.sh` by path; `machine-quiet.sh` and `no-clauductor.sh` strip it from PATH), so the
  stub changes nothing that passes.

**D14. This repository's first three routines.**
- **Recommended:**

  | Routine | Schedule | Run | Notes |
  |---|---|---|---|
  | `nightly-checks` | `02:30`, catch-up once | `sh .claude/checks/run.sh` | The process checks on main's HEAD. Timeout 1800 s. |
  | `flake-watch` | `03:30`, catch-up skip, `machine: darwin` | `sh scripts/routines/flake-watch.sh` | The full gate on main's HEAD with `claude` stripped from PATH (as on CI, so #93 cannot reach the owner's login at night). On a failure it keeps the whole log, the `fx` line OPS-33 added included: the observation OPS-35 waits on, since #62 only ever failed under a full gate. Timeout 5400 s. |
  | `morning-digest` | `07:00`, catch-up once, `output: show` | `claude -p`, role `routine`, tools `Read, Grep, Glob`, 6 turns, $0.50; `context` = session-start's context script | The prompt asks for at most 15 lines: what merged since yesterday, what is next, what waits on the owner, and every health line not OK, quoting the context's figures. Its `result.md` is named in the health line. |

  - **What they run against:** the main checkout as it stands. The runner never fetches or
    pulls; each record names the commit, and the health line says when that commit is behind
    the last-fetched `origin/main`.
- **Alternatives:**
  - `flake-watch` looping the one test #62 names: 64 runs and 300 checkout cycles reproduced
    nothing outside a full gate (OPS-35's row);
  - the digest by PushNotification (D15);
  - routines that fetch and fast-forward the main checkout first: an unattended write to the
    owner's main checkout, which fails on a dirty tree anyway.

**D15. Where the morning digest goes.**
- **Recommended: a file, and the health line.** `out/morning-digest/<run id>/result.md`, named by
  the health line at the next session start; PANEL-38's Today column shows it.
- **Alternative: PushNotification under `-p`.** PRD-maps lists it as unverified, and Claude
  Code's docs do not mention it under `-p` (phone push is documented for Remote Control only). A
  notification at 07:00 would also go to the owner before they sit down; the session start is
  where they already look (*A control needs a named addressee*).

**D16. Which credentials count as the subscription.**
- **Recommended:** refuse a `claude -p` routine while `ANTHROPIC_AUTH_TOKEN` or
  `ANTHROPIC_API_KEY` is set, and unset both for every command routine. Claude Code's
  precedence is `ANTHROPIC_AUTH_TOKEN`, then `ANTHROPIC_API_KEY`, then
  `CLAUDE_CODE_OAUTH_TOKEN`, then the `/login` subscription, and "in non-interactive mode
  (`-p`), the key is always used when present" (code.claude.com/docs/en/authentication). So
  either variable would bill an API account instead. **`CLAUDE_CODE_OAUTH_TOKEN` is allowed**:
  it is a subscription token (`claude setup-token`). The run record says which source was in use
  (`auth`: `oauth-token` or `login`), never its value.
- **Alternative:** refuse `CLAUDE_CODE_OAUTH_TOKEN` too, so only the `/login` session counts. It
  would block a machine that runs Claude Code headless on a setup token, which is still the
  subscription and still Claude Code. The token could belong to another account; the record's
  `auth` field shows it was used.
- **Never `--bare`**, which reads neither OAuth credentials nor the keychain.
