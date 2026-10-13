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
   default caps, `"billing": null` and no routines; `routines.sh setup` sets `billing`, D16):

   ```json
   {
     "version": 1,
     "billing": "subscription",
     "api_key_file": null,
     "caps": {
       "subscription": { "five_hour_pct": 80, "seven_day_pct": 80, "unknown_quota_daily_runs": 3,
                         "daily_runs": null },
       "api": { "daily_usd": 1.00, "window_usd": 5.00, "window_days": 7, "daily_runs": null }
     },
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
   disallowed_tools?, max_turns, max_usd?}` (D6, D7). `max_usd` bounds one run, in either billing
   mode; it is not one of the caps (D4). `context` is a command whose stdout is put
   into the prompt, so the deterministic part runs before the model and the model needs no Bash.

2. **The runner, `.claude/routines.sh`** (POSIX sh and jq, ADR-0006), with these verbs:

   | Verb | What it does |
   |---|---|
   | `tick` | Takes the lock (D5), validates the registry, records the tick, and runs every routine that is due, one after another. Exits 0 when routines fail (their verdicts are in the log); non-zero only when the registry is invalid. |
   | `run <name>` | Runs one routine now (trigger `manual`), whatever its schedule; everything else applies. Exits with the run's outcome. |
   | `list` | Each routine: schedule, kind, next due, last verdict, and which runner or machine it waits for when it is not this one's. |
   | `log [<name>] [--days N] [--json]` | The run records, newest first. `--json` is the form PANEL-37 and PANEL-38 read. |
   | `validate` | The registry's checks, shared by `tick` and `checks/routines.sh`. |
   | `setup --billing subscription\|api [--key-file PATH]` | Records how this project's `claude -p` routines are paid for, and for `api` the key file's path, never the key (D16). Run once by the user. |
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
   `effort`, `subtype`, `auth`, `billing`). A refusal is one `refused` record (`reason`:
   `capped`, `quota` or `billing`, with the figures). A start with no end whose holder is dead is closed as failed,
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
| A registry with an enabled `claude -p` routine and no `billing` | Validation fails: "run `routines.sh setup --billing subscription\|api`" (D16). Command routines need no `billing`. |
| `billing` is `subscription` and `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` (or a provider switch) is set | Each due `claude -p` routine is refused and logged `billing`, naming the variable: `-p` would bill it instead of the subscription (D16). Command routines run with both unset. `CLAUDE_CODE_OAUTH_TOKEN` is the subscription and is allowed. |
| `billing` is `api` and the key file is missing, empty of `ANTHROPIC_API_KEY`, inside the repository, or readable by group or others; or `ANTHROPIC_AUTH_TOKEN` or a provider switch is set | Each due `claude -p` routine is refused and logged `billing`, naming the problem, never the key (D16). |
| Subscription: a current reading of the 5-hour or 7-day window is at or above its cap | The `claude -p` routine is refused and logged `quota` with the window and the reading (D4). |
| Subscription: no current reading of a window | Runs up to `unknown_quota_daily_runs` a day, then refused and logged `capped`; the health line says the quota is unknown (D17). |
| API: a dollar cap or the run count is reached | The `claude -p` routine is refused and logged `capped` with the cap and the figure (D4). |
| A routine's `where` is not `local`, or its `machine` is another OS | It is not this runner's: skipped silently, shown by `list`. |
| A sidecar registry declares `writes: true` | Validation fails (D12). |
| `jq` is missing | `tick` exits 2; the health line says CANNOT CHECK. |
| The `claude` binary is missing | The run fails, "claude is not installed", logged; the tick carries on. |

## Decisions (the owner approved D1-D17 (2026-10-13))

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

**D4. The caps: they follow how Claude Code is paid for.** *(Revised to the owner's direction of
2026-10-13.)*
- **Recommended:** the registry's `billing` (D16) picks the caps. Only the block for that mode
  applies. Caps count `claude -p` runs only; a command costs no tokens and is never capped.
  - **`subscription` (a Claude login): quota-only.**
    - Refuse a new `claude -p` run when the **5-hour** window is at or above `five_hour_pct`
      (default **80**), or the **7-day** window is at or above `seven_day_pct` (default **80**).
      Both windows are in the panel's `quota.json` (`state.Quota`: `five_hour`, `seven_day`).
      When the reading has a model-specific 7-day window for the routine's model,
      `seven_day_<alias>` with the alias the routine's role names in `model-roles.json`
      (`seven_day_sonnet`, `seven_day_opus`), `seven_day_pct` applies to it too. A model window
      past its reset is ignored, not unknown.
    - **A reading counts until its window resets**, whatever its age. A window's use only grows
      until its reset, so an old reading is a floor: if it is already at the cap, the run is
      refused. After the reset (`resetsAt` passed) the 5-hour or 7-day window is unknown (D17).
      A window with no `resetsAt`, or a null `pct`, has no counting reading either: the panel's
      `quota.go` omits both at times, and carries forward a window a newer post left out. The age
      a refusal reports is the file's `at`.
    - **No dollar caps,** and **no runs-per-day limit by default**: `caps.subscription.daily_runs`
      exists and is null.
    - `--max-budget-usd` is passed only when the routine sets `max_usd`, as a bound on that one
      run (a runaway guard), never as a daily cap.
  - **`api` (the Anthropic API key from the key file, D16): dollar caps.**
    - `daily_usd` (default **1.00**) and `window_usd` over `window_days` (default **5.00** over
      **7**), from the run log's recorded costs; `caps.api.daily_runs` optional, null by default.
    - Each run's `--max-budget-usd` is the least of its `max_usd` and what the two caps leave, so
      one run cannot cross a cap by more than claude's own overshoot. A run whose cost is unknown
      counts at the budget it was given.
    - The quota is not read: an API key does not draw on a subscription's windows.
  - **The scope is per registry**, in its own run log. In subscription mode the quota is the
    machine-wide guard already, because it is the account's. A machine-wide **dollar** cap across
    projects matters only for `api` mode, and is **PANEL-37's**: the panel is the one process that
    ticks every project. This PR adds that to PANEL-37's row.
- **The ToS posture of each mode:**
  - **subscription:** `claude -p` is Claude Code itself, run by the subscriber on their own
    machine with their own login, drawing on their plan's usage limits like any session. The
    runner never calls another harness, the Agent SDK or the API with that login, and never
    passes `--bare`. The quota caps keep unattended runs from using up the window the owner
    works in.
  - **api:** usage is billed per token to the key's account, under the API's terms. The runner
    still uses only `claude -p`, so both modes run the same code.
- **Alternatives:**
  - one set of caps for both modes (dollars and quota together, the first draft): dollar caps
    mean nothing on a subscription, and a quota means nothing on an API key;
  - detecting the mode at each tick from the environment alone: an unset variable would silently
    turn an `api` project into a subscription one, with no quota guard (D16);
  - a machine-wide ledger in `~/.clauductor/routines/` for `api` mode without the panel: a shared
    file every project must lock, for a case nobody has yet.
- **Why these numbers:** 80% leaves the owner a fifth of each window for their own sessions.
  In `api` mode the digest is expected at $0.05–0.30 a run on sonnet at low effort, so $1 a day is
  three to twenty runs with room for one mistake, and $5 a week stops a daily routine that costs
  ten times what was expected within a week.

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
    with stdin from `/dev/null` and the environment D16 sets for the billing mode.
    `--max-budget-usd` is passed in `api` mode always, and on a subscription only when the
    routine sets `max_usd` (D4). Never `--bare`, which never reads the subscription login.
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
  (code.claude.com/docs/en/headless). Its record therefore says `timeout` and cost unknown, counted
  at its budget in `api` mode (D9). The runner never looks a process up by name, argv or parent.
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
  and `subtype`. A run that gives no readable JSON is recorded with cost unknown and, in `api`
  mode, counted at its budget (D4): a SIGTERMed run writes no result (exit 143), and on
  `error_max_budget_usd` the docs say `total_cost_usd` still includes the call that crossed the
  budget. The run log is the only record: a headless run has no status line, so it never reaches the panel's spend ledger, and with
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
  - `CAPPED routine morning-digest — refused at 07:00: 7-day quota at 83% (cap 80%, read 02:10)`;
  - `RUNNING routine flake-watch — since 03:30 (pid 4211)`;
  - `NEVER RAN routine morning-digest — due since 2026-10-13 07:00`;
  - `FAILED routine morning-digest — refused at 07:00: billing is subscription but
    ANTHROPIC_API_KEY is set` (a `billing` refusal, D16: FAILED, never CAPPED, naming the
    variable or the key file's problem);
  - `CANNOT CHECK — routines quota: no current reading of the 5-hour window since 2026-10-12
    23:40; claude -p routines limited to 3 runs a day (2 of 3 runs used today)` (D17);
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

**D16. How the billing mode is set, and which credentials each mode accepts.** *(Revised to the
owner's direction of 2026-10-13.)*
- **Recommended: the user declares it once, with `routines.sh setup --billing subscription|api`,
  and the runner checks the environment against it at every run.**
  - **Setup** writes `billing` into the registry and calls nothing: no `claude`, no network. It
    cross-checks the environment and refuses a declaration the environment contradicts,
    naming the variable: `subscription` while `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY` or a
    provider switch is set; `api` while `ANTHROPIC_AUTH_TOKEN` or a provider switch is set, or
    without a usable key file. The template's registry ships `"billing": null`; validation
    fails a registry with an enabled `claude -p` routine and no `billing`, pointing at `setup`.
    The playbook's *Routines* section says to run it; this repository's registry is
    `subscription`.
  - **Set** means present and non-empty, for every variable named here. The **provider
    switches** are `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX` and
    `CLAUDE_CODE_USE_FOUNDRY`. Billing through Bedrock, Vertex or Foundry is out of scope: a
    switch is refused in both modes.
  - **The API key comes from a key file** (the owner's decision of 2026-10-13), so a scheduled
    run, which inherits no shell's environment, can reach it without the key ever entering the
    repository or the ticker:
    - `routines.sh setup --billing api --key-file <path>` records the path in the registry's
      `api_key_file` (a leading `~/` kept as written), never the key. It refuses a path inside
      the repository (its top-level directory, `.git` included); a sidecar's directory is
      outside it and allowed. The same on macOS and Linux.
    - Each `api` run reads only the line `ANTHROPIC_API_KEY=<key>` from that file. It never
      sources the file, so nothing else in it runs or is read. It puts the key only in the
      `claude -p` child's environment, never in a `context` command's or a command routine's,
      and never copies, prints or logs it.
    - The run is refused, logged `billing`, when the file is missing, is not a regular file,
      holds no non-empty `ANTHROPIC_API_KEY`, lies inside the repository, or is readable by
      group or others (its mode bits, read the same way on both systems). An
      `ANTHROPIC_API_KEY` in the runner's own environment, empty or not, is ignored: the key
      comes from the file.
    - The printed ticker (D11) carries no secret, only the repository's path and `PATH`.
  - **At every run,** Claude Code's precedence decides what `-p` would bill:
    `ANTHROPIC_AUTH_TOKEN`, then `ANTHROPIC_API_KEY`, then `CLAUDE_CODE_OAUTH_TOKEN`, then the
    `/login` subscription, and "in non-interactive mode (`-p`), the key is always used when
    present" (code.claude.com/docs/en/authentication). So:
    - **`subscription`:** refuse while `ANTHROPIC_AUTH_TOKEN` or `ANTHROPIC_API_KEY` is set, or a
      provider switch: `-p` would silently bill that instead. A variable present but empty is
      unset before `claude` starts. `CLAUDE_CODE_OAUTH_TOKEN` is allowed: it is a subscription
      token (`claude setup-token`).
    - **`api`:** run with the key from the key file; refuse when the key file is not usable (as
      above), or while `ANTHROPIC_AUTH_TOKEN` (which outranks the key) or a provider switch is
      set. Without a key the run would fall through to the subscription with no quota guard. An
      `apiKeyHelper` in settings is not detected and not used.
    - Command routines run with `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY` unset in both
      modes: they cost no tokens, and a command that calls claude should not bill a key by
      accident.
  - The run record says `billing` and which source was in use (`auth`: `login`, `oauth-token`
    or `key-file`), never a value. A `billing` refusal reads FAILED in the health line (D10).
- **Alternatives:**
  - **`claude auth status --json` once at setup** (its `authMethod`: `claude.ai`, `oauth_token`,
    `api_key`, …, as `signals/auth.go` reads it): it would detect rather than ask, but
    #95822 names `auth status` as a short-lived command that can start a login refresh and exit
    before saving it, which is why PANEL-28 keeps the panel's own auth calls rare. One call is
    probably harmless; asking costs nothing, and a check would have to fake it anyway.
  - **asking in `start-project`:** it runs once, for new projects only, so this repository and
    every installed project would never be asked. `setup` is what `start-project` can call later.
  - **detecting at each tick from the environment alone:** an unset key would silently switch an
    `api` project to the subscription.
  - **the key in the ticker's environment** (the plist's `EnvironmentVariables`, the crontab
    line): the secret would sit in a file the user copies around and in `ps` output. A key file
    with tight permissions is read only by the runner, at run time.
  - **refusing `CLAUDE_CODE_OAUTH_TOKEN` in `subscription` mode:** it would block a machine that
    runs Claude Code headless on a setup token, which is still the subscription and still Claude
    Code. The token could belong to another account; the record's `auth` shows it was used.
- **Never `--bare`**, which reads neither OAuth credentials nor the keychain.

**D17. The quota is unknown: no panel, or no session since the window reset.** *(A gap the
owner asked to decide.)*
- **The gap is wider than "no panel".** `quota.json` is written only from an interactive
  session's status-line post. So even with the panel running, at 07:00 the 5-hour window has
  usually reset since the owner's last session, and its reading is unknown. The 7-day reading
  usually still counts (D4: a floor until its reset).
- **Recommended: (c) a small run count while unknown, and a health line that says so.** While
  either window has no counting reading, `claude -p` routines may start at most
  `unknown_quota_daily_runs` (default **3**) runs a day: every `claude -p` run started today
  (the local day) counts, from the run log, whatever the quota was when it started. Past it they
  are refused and logged `capped`. When `daily_runs` is also set, both apply, the smaller binds,
  and the refusal names which limit refused. The health line adds `CANNOT CHECK — routines
  quota: no current reading of the 5-hour window since <when>; claude -p routines limited to 3
  runs a day (N of 3 runs used today)`, which the session summary repeats.
- **Alternatives:**
  - **(a) run anyway, and warn:** only the schedule bounds them, so a mis-typed `every 5m` on a
    `claude -p` routine would run 288 times a day against the owner's subscription before anyone
    reads the warning;
  - **(b) refuse while unknown:** the morning digest would almost never run (its 07:00 slot
    almost always finds the 5-hour window reset), and without the panel no `claude -p` routine
    could ever run, against ADR-0006.
- **Why (c):** it keeps a scheduled digest working (one run a day is well inside 3), bounds the
  worst mistake to three runs, and tells the owner why. A project that wants a different bound
  sets the number; `0` gives (b).
