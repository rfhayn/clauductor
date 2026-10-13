## Purpose
A project runs scheduled work while its owner is away: routines named in a registry, run one at a time by one runner on this machine, each bounded by a timeout, the `claude -p` ones also by caps and the account's quota, every run recorded with its cost and its subject, and every outcome reported in a health line at the next session start.

## ADDED Requirements

### Requirement: The registry is validated before anything runs
THE SYSTEM SHALL read routines only from the registry (`.claude/routines.json`, or a registry named with `--registry`), and SHALL refuse to run any routine from a registry that does not validate, naming every entry at fault.

#### Scenario: [ROUTINES-1-S1] A valid registry lists its routines
- **GIVEN** a registry with a command routine and a `claude -p` routine, both enabled
- **WHEN** `routines.sh list` runs
- **THEN** it prints each routine's name, schedule, kind, next due time and last verdict
- **AND** it exits 0

#### Scenario: [ROUTINES-1-S2] A malformed entry stops every routine, not just itself
- **GIVEN** a registry where one routine's schedule is `25:00` and another is valid and due
- **WHEN** `routines.sh tick` runs
- **THEN** no routine runs
- **AND** it exits non-zero naming the routine and its field, and the health line says FAILED for the registry

#### Scenario: [ROUTINES-1-S3] A write tool needs writes: true
- **GIVEN** a `claude -p` routine whose `allowed_tools` holds `Edit`, or `Bash` in any form, or a tool the runner does not know to be read-only, and no `writes: true`
- **WHEN** `checks/routines.sh` validates the registry
- **THEN** it fails, naming the routine and the tool
- **AND** the same routine with `writes: true` validates

#### Scenario: [ROUTINES-1-S4] A routine names a role that exists
- **GIVEN** a `claude -p` routine whose `role` is not a role in `.claude/model-roles.json`, or whose `skill` names no skill directory
- **WHEN** the registry is validated
- **THEN** validation fails, naming the routine and the missing role or skill

#### Scenario: [ROUTINES-1-S5] A routine for another runner is left alone
- **GIVEN** a valid routine whose `where` is not `local`, or whose `machine` names another operating system
- **WHEN** the tick runs while it is due
- **THEN** it is not run and nothing is logged for it
- **AND** `list` shows it with the runner or machine it waits for

#### Scenario: [ROUTINES-1-S6] An MCP tool is refused even with writes
- **GIVEN** a `claude -p` routine whose `allowed_tools` holds an `mcp__` tool, with or without `writes: true`
- **WHEN** the registry is validated
- **THEN** validation fails, saying routines load no MCP server

#### Scenario: [ROUTINES-1-S7] A claude -p routine needs a declared billing mode
- **GIVEN** a registry with `"billing": null` and an enabled `claude -p` routine
- **WHEN** it is validated
- **THEN** validation fails, saying to run `routines.sh setup --billing subscription|api`
- **AND** a registry with only command routines validates with no `billing`

### Requirement: A routine runs when its schedule says, and catches up as it declares
THE SYSTEM SHALL run an enabled routine once for each schedule slot it reaches, and WHEN a slot passed while nothing ticked THE SYSTEM SHALL run it once at the next tick if its `catch_up` is `once`, and SHALL skip that slot if its `catch_up` is `skip`.

#### Scenario: [ROUTINES-2-S1] A daily routine runs once at its time
- **GIVEN** a routine scheduled `02:30` that last ran yesterday at 02:30
- **WHEN** the tick runs at 02:31, and again at 02:36
- **THEN** the routine runs at the first tick only

#### Scenario: [ROUTINES-2-S2] Missed slots are caught up once
- **GIVEN** a routine scheduled `07:00` with `catch_up: once` whose last run was three days ago
- **WHEN** the first tick in three days runs at 09:12
- **THEN** the routine runs once, logged with the trigger `catch-up` and the slot it covers

#### Scenario: [ROUTINES-2-S3] A skip routine waits for its next slot
- **GIVEN** a routine scheduled `07:00` with `catch_up: skip` whose last run was yesterday at 07:00
- **WHEN** the first tick today runs at 09:12
- **THEN** the routine does not run

#### Scenario: [ROUTINES-2-S4] The schedule grammar is the one documented
- **GIVEN** the schedules `02:30`, `Mon,Thu 06:00`, `Mon-Fri 18:15`, `every 30m` and `every 2h`
- **WHEN** the tick runs at a series of fixed clock times
- **THEN** each routine is due exactly at the times the grammar defines
- **AND** `every 3m`, `9:5`, `Sun-Mon 01:00` and a 5-field cron line are rejected by validation

#### Scenario: [ROUTINES-2-S5] Run now ignores the schedule but nothing else
- **GIVEN** an enabled routine that is not due
- **WHEN** `routines.sh run <name>` runs
- **THEN** the routine runs once, logged with the trigger `manual`
- **AND** the caps, the quota, the lock and the billing check apply to it as to a scheduled run

### Requirement: One routine at a time, under one lock
THE SYSTEM SHALL hold one lock per registry while a tick or a run-now is deciding or running, SHALL run due routines one after another inside it, and WHEN the lock is held by a live process THE SYSTEM SHALL exit without running anything and without waiting.

#### Scenario: [ROUTINES-3-S1] An overlapping tick leaves at once
- **GIVEN** a tick is running a routine that takes 60 s
- **WHEN** a second tick starts
- **THEN** the second tick exits 0 within a second, saying which routine holds the lock and since when
- **AND** it runs nothing and logs no run, but records the tick in `last-tick`, so a long run does not make the ticker STALE

#### Scenario: [ROUTINES-3-S2] Run now is refused while the lock is held
- **GIVEN** a tick is running a routine
- **WHEN** `routines.sh run <another>` runs
- **THEN** it exits non-zero, naming the routine that holds the lock, and runs nothing

#### Scenario: [ROUTINES-3-S3] A lock whose holder died is taken over
- **GIVEN** the lock names a pid that is no longer running, or one reused by another process
- **WHEN** the tick runs
- **THEN** it takes the lock and runs what is due
- **AND** the dead holder's unfinished run is logged as failed, "the runner died"

### Requirement: Every run is recorded with its subject and kept output
THE SYSTEM SHALL append a start record and an end record for every run to the run log, naming the routine, the trigger, the commit it ran on, the verdict, the duration and, for a `claude -p` run, the cost, turns, model and effort; SHALL keep each run's output under `out/<name>/`; and SHALL write nothing inside the repository's tree itself.

#### Scenario: [ROUTINES-4-S1] A command run is logged and its output kept
- **GIVEN** a command routine that prints a line and exits 0
- **WHEN** it runs
- **THEN** the run log gains a start and an end record with the verdict `ok`, the exit 0, the commit HEAD named and the duration
- **AND** its stdout and stderr are kept in `out/<name>/<run id>/`

#### Scenario: [ROUTINES-4-S2] A failing command is a failed run
- **GIVEN** a command routine that exits 1
- **WHEN** it runs
- **THEN** the end record says `failed` with exit 1, and the tick itself exits 0

#### Scenario: [ROUTINES-4-S3] A run that changes the tree says so
- **GIVEN** a routine without `writes: true` whose command creates a file in the repository
- **WHEN** it runs and exits 0
- **THEN** its verdict is `failed`, with the paths it left changed
- **AND** a routine with `writes: true` doing the same is `ok`, with the paths recorded

#### Scenario: [ROUTINES-4-S4] Old output is pruned, the log is not
- **GIVEN** a routine with 30 kept runs
- **WHEN** it runs again
- **THEN** its oldest output directory is removed and 30 remain
- **AND** every record in the run log stays

#### Scenario: [ROUTINES-4-S5] The run log lives outside the tree, shared by every worktree
- **GIVEN** a registry in the repository
- **WHEN** a routine runs from the main checkout, and `routines.sh log` runs in a lane worktree
- **THEN** the log and the output are under `<git common dir>/clauductor/routines/`, and the worktree's `log` shows the run
- **AND** `git status` in the main checkout is unchanged by the runner

### Requirement: A timeout stops only the routine's own child
WHEN a run passes its `timeout_s` THE SYSTEM SHALL send SIGTERM to the process the runner started for it (and to that process's group when the runner made it a group leader), SHALL send SIGKILL 15 s later if it is still running, and SHALL NOT signal any process it did not start.

#### Scenario: [ROUTINES-5-S1] A run past its timeout is stopped and logged
- **GIVEN** a routine with `timeout_s: 2` whose command sleeps for 60 s
- **WHEN** it runs
- **THEN** it is stopped within 3 s of its timeout with SIGTERM, and its verdict is `timeout`

#### Scenario: [ROUTINES-5-S2] A child that ignores SIGTERM is killed after the grace
- **GIVEN** a routine with `timeout_s: 2` whose command traps SIGTERM and keeps running
- **WHEN** it runs
- **THEN** it receives SIGKILL 15 s after the SIGTERM (scaled by the test's grace setting), and its verdict is `timeout`

#### Scenario: [ROUTINES-5-S3] A process with the same command line is left alone
- **GIVEN** a process outside the runner whose argv is identical to the routine's, a fake `claude -p` included
- **WHEN** the routine times out and is stopped
- **THEN** that other process is still running afterwards

### Requirement: A claude -p routine runs as its billing mode says, bounded, and records its cost
THE SYSTEM SHALL run a `claude -p` routine only through the `claude` binary, with `--output-format json`, the model and effort of its role in `.claude/model-roles.json`, its tools and its turn limit; SHALL refuse it when the environment contradicts the registry's `billing`; and SHALL record the cost, turns, billing mode and credential source its run used. A variable is set when it is present and non-empty; the provider switches are `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX` and `CLAUDE_CODE_USE_FOUNDRY`. In `subscription` mode it SHALL refuse while `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` or a provider switch is set. In `api` mode it SHALL take the key only from the registry's `api_key_file`: from exactly one line matching `^[[:blank:]]*(export[[:blank:]]+)?ANTHROPIC_API_KEY=` (lines starting with `#` after leading blanks are comments and ignored; the last line may lack a newline), the value being everything after the first `=` with a trailing `\r` stripped, then trailing blanks stripped, then one pair of matching surrounding quotes stripped. A `~/` at the start of `api_key_file` is expanded against `$HOME` when it is read. It SHALL never source the file, SHALL put the key only in the `claude -p` child's environment by an export, never on any process's argv, and SHALL never copy or log it. It SHALL refuse, with a reason of its own for each, when that file is missing, is not a regular file (a symlink included), is unreadable, has no matching line, an empty value or more than one matching line, lies inside the repository, or is readable by group or others; and while `ANTHROPIC_AUTH_TOKEN` or a provider switch is set. Inside the repository means that one of the file's physical ancestors (its directory resolved with `cd -P`, then each parent up to `/`) is the same file (`-ef`) as the top level of a worktree `git worktree list --porcelain` lists, or as the git common dir; a listed worktree whose directory no longer exists is skipped. "Under" is by path components, so a sibling sharing a prefix is outside. Billing through Bedrock, Vertex or Foundry is out of scope.

#### Scenario: [ROUTINES-6-S1] An API credential refuses a subscription routine
- **GIVEN** `billing` is `subscription` and `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set in the runner's environment
- **WHEN** a due `claude -p` routine would run
- **THEN** claude is not started, and the run log records a `billing` refusal naming the variable
- **AND** a due command routine in the same tick still runs, with both variables removed from its environment
- **AND** `CLAUDE_CODE_USE_BEDROCK=1` is refused the same way, naming the switch
- **AND** an `ANTHROPIC_API_KEY` that is present but empty is not a refusal: it is removed before claude starts

#### Scenario: [ROUTINES-6-S2] The invocation carries the role, the tools and the bounds
- **GIVEN** `billing` is `subscription` and a `claude -p` routine with role `routine`, `allowed_tools` `Read, Grep, Glob`, `max_turns` 6 and `max_usd` 0.50
- **WHEN** it runs against a fake claude that records its argv
- **THEN** the argv has `-p`, `--output-format json`, `--permission-mode dontAsk`, the role's model and effort, `--tools` and `--allowedTools` with exactly those tools, `--max-turns 6`, `--max-budget-usd 0.50`, the write tools in `--disallowedTools`, and `--strict-mcp-config`
- **AND** it has no `--bare`
- **AND** the prompt holds the routine's context command's output

#### Scenario: [ROUTINES-6-S3] Cost and turns come from claude's JSON
- **GIVEN** a fake claude that answers with `total_cost_usd` 0.12 and `num_turns` 4
- **WHEN** the routine runs
- **THEN** the end record holds cost 0.12, turns 4, the model and the effort
- **AND** the result text is kept as the run's output

#### Scenario: [ROUTINES-6-S4] An unreadable answer is cost unknown
- **GIVEN** a fake claude that exits 1, or answers with no JSON, or is stopped by the timeout
- **WHEN** the run ends
- **THEN** its verdict is `failed` or `timeout`, and its cost is recorded as unknown
- **AND** in `api` mode the caps count that run at the budget it was given

#### Scenario: [ROUTINES-6-S5] An error result is a failed run
- **GIVEN** a fake claude whose JSON has `is_error: true`, or a subtype other than `success`
- **WHEN** the run ends
- **THEN** its verdict is `failed`, with the subtype recorded, and its reported cost is still counted
- **AND** a subtype the runner has never seen is treated the same way

#### Scenario: [ROUTINES-6-S6] A subscription token is allowed and named
- **GIVEN** `billing` is `subscription`, `CLAUDE_CODE_OAUTH_TOKEN` is set and neither API key variable is
- **WHEN** a due `claude -p` routine runs against a fake claude
- **THEN** it runs, and its record's `auth` is `oauth-token`, with no part of the token's value in the log or the output
- **AND** with no token set, the record's `auth` is `login`

#### Scenario: [ROUTINES-6-S7] An api routine runs on the key from its key file
- **GIVEN** `billing` is `api` and `api_key_file` names a file outside the repository, mode 0600, holding `ANTHROPIC_API_KEY=<key>` and other lines
- **WHEN** a due `claude -p` routine runs against a fake claude that records its environment
- **THEN** claude's environment has that key as `ANTHROPIC_API_KEY`, and its record says `billing` `api` and `auth` `key-file`
- **AND** no part of the key is in the run log, the output, the ticker line or any file the runner wrote, and the file's other lines were never run or passed on
- **AND** a `context` command and a command routine in the same tick do not get the key
- **AND** with a shim for every executable name found in the test's PATH directories first on PATH (each linked to one recorder that records its argv, then execs the real tool), no recorded argv, the fake claude's included, and no run-log line holds any part of the key; builtins such as `printf`, `echo` and `test` start no process, so they cannot put it on an argv
- **AND** `routines.sh` invokes no tool by an absolute path and never uses `command -p`, so nothing it runs bypasses the shims
- **AND** a runner that passes the key as `env ANTHROPIC_API_KEY=… claude` fails this scenario on every run of the check (a permanent negative case)
- **AND** the same key is read from each accepted form of the line: `export ANTHROPIC_API_KEY=<key>`, leading blanks, `"<key>"` and `'<key>'`, a CRLF line ending, trailing blanks after the value, the line last in the file with no trailing newline, and with a `# ANTHROPIC_API_KEY=old` comment line beside it
- **AND** an `api_key_file` recorded as `~/keys/anthropic` reads `$HOME/keys/anthropic`, and with `HOME` empty or unset the run is refused `billing`, "HOME is not set", never reading `/keys/anthropic`
- **AND** a key file in a sibling directory sharing the repository's prefix (`<top>-keys/key`) is accepted by the run as by setup: one function decides "inside the repository" for both

#### Scenario: [ROUTINES-6-S8] An api routine whose key file is not usable is refused
- **GIVEN** `billing` is `api` and the key file is missing, empty, unreadable (mode 0000), holds an empty `ANTHROPIC_API_KEY=` or `ANTHROPIC_API_KEY=""`, holds two matching lines, holds the key only in a commented-out line, lies inside the repository, or is readable by group or others (mode 0640 or 0644)
- **WHEN** a due `claude -p` routine would run
- **THEN** claude is not started, and the run log records a `billing` refusal with that case's own reason, never the key
- **AND** a key file that is a symlink is refused as not a regular file, including a symlink outside the repository that points at a file inside it
- **AND** a key file under another worktree of the repository, or under the git common dir, is refused as inside the repository, and so is one whose path reaches the repository through a symlinked or differently cased parent: one of its physical ancestors `-ef` the top level
- **AND** a prunable worktree (listed, its directory gone) neither refuses nor breaks the check
- **AND** an `ANTHROPIC_API_KEY` set in the runner's own environment does not stand in for the file: the key comes only from the file
- **AND** with a usable file, `ANTHROPIC_AUTH_TOKEN` or a provider switch set is refused too, naming the variable
- **AND** the same holds on macOS and Linux

### Requirement: The caps follow the billing mode
THE SYSTEM SHALL apply only the caps of the registry's `billing` mode to `claude -p` runs, and never to command runs. In `subscription` mode it SHALL refuse a new run when a counting reading of the 5-hour window is at or above `five_hour_pct`, or of the 7-day window, or of the model window `seven_day_<alias>` for the alias the routine's role names in `.claude/model-roles.json`, at or above `seven_day_pct`. A reading counts until its window's `resetsAt`; a window with no `resetsAt` or a null `pct` has no counting reading; a model window with no counting reading is ignored, not unknown; and the age a refusal reports is the file's `at`. It SHALL apply no dollar cap, and a daily run count only when `caps.subscription.daily_runs` is set; and WHILE the 5-hour or the 7-day window has no counting reading it SHALL start no `claude -p` run once `unknown_quota_daily_runs` of them have started today (the local day), counting every `claude -p` run the run log shows started today. In `api` mode it SHALL refuse a new run when today's recorded spend has reached `daily_usd`, the spend over the last `window_days` has reached `window_usd`, or, when set, today's runs have reached `caps.api.daily_runs`, and SHALL give each run a budget no larger than what those caps leave. When two run limits apply, both hold and the smaller binds. It SHALL log each refusal, naming the limit that refused.

#### Scenario: [ROUTINES-7-S1] The daily dollar cap refuses the next api run
- **GIVEN** `billing` is `api`, `daily_usd` 1.00 and today's runs recorded at 1.02
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `capped` naming the daily cap and the spend

#### Scenario: [ROUTINES-7-S2] The window cap, and a run count when set, refuse too
- **GIVEN** `billing` is `api`, with `window_usd` 5.00 over 7 days and 5.10 recorded, or `daily_runs` set to 6 with six `claude -p` runs today
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `capped` naming that cap

#### Scenario: [ROUTINES-7-S3] The budget given is what the caps leave
- **GIVEN** `billing` is `api`, `daily_usd` 1.00 with 0.80 spent today, and a routine with `max_usd` 0.50
- **WHEN** it runs against a fake claude
- **THEN** it is given `--max-budget-usd 0.20`

#### Scenario: [ROUTINES-7-S4] Command routines are never capped
- **GIVEN** every cap of either mode is reached
- **WHEN** a command routine is due
- **THEN** it runs

#### Scenario: [ROUTINES-7-S5] A subscription window at its cap refuses
- **GIVEN** `billing` is `subscription`, the defaults, and the panel's `quota.json` with the 5-hour window at 83%, or the 7-day window at 80%, or `seven_day_sonnet` at 90% for a routine whose role runs on sonnet, none of them past its reset
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `quota` with the window, its percentage and when it was read
- **AND** with every window below 80% it runs, and its record holds the readings

#### Scenario: [ROUTINES-7-S6] A subscription has no dollar cap and no run count by default
- **GIVEN** `billing` is `subscription`, the defaults, every window below 80%, $40 recorded today and twenty `claude -p` runs today
- **WHEN** a `claude -p` routine with no `max_usd` is due
- **THEN** it runs, and its argv has no `--max-budget-usd`

#### Scenario: [ROUTINES-7-S7] A reading counts until its window resets
- **GIVEN** `billing` is `subscription` and a `quota.json` whose `at` (unix milliseconds) is three days ago, with the 7-day window at 85% whose `resetsAt` (unix seconds) is tomorrow, and the 5-hour window at 10% whose `resetsAt` has passed
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `quota` naming the 7-day window and the reading's age, taken from the file's `at` and stated as at least that old
- **AND** with the 7-day window at 40% instead, the 5-hour window counts as unknown (ROUTINES-7-S8 applies)

#### Scenario: [ROUTINES-7-S8] An unknown quota allows only a few runs a day
- **GIVEN** `billing` is `subscription`, `unknown_quota_daily_runs` 3, and no counting reading of the 5-hour window: no `quota.json`, its reset has passed, it has no `resetsAt`, or its `pct` is null
- **WHEN** a `claude -p` routine is due with two `claude -p` runs today
- **THEN** it runs, and its record says the quota was unknown
- **AND** with three runs today it does not run, and the log records `capped` naming the unknown quota and the limit
- **AND** with five `claude -p` runs started this morning while the quota was known, and the quota unknown now, it does not run: every run started today counts
- **AND** with `caps.subscription.daily_runs` 2 and two runs today, it does not run, and the refusal names `daily_runs` as the limit that refused
- **AND** a `seven_day_sonnet` window past its reset is ignored, and does not by itself make the quota unknown

### Requirement: The health line reports every routine and the ticker
THE SYSTEM SHALL print, at session start, one line per enabled routine for this machine and one for the ticker, each opening with its verdict (`OK`, `FAILED`, `NEVER RAN`, `STALE`, `CAPPED`, `RUNNING` or `CANNOT CHECK`) and naming the run and the time it covers, and for a finished run its commit.

#### Scenario: [ROUTINES-8-S1] Each verdict names its run
- **GIVEN** routines whose last runs were ok, failed, capped and still running, and one due since yesterday that never ran
- **WHEN** `health/routines.sh` runs
- **THEN** it prints OK, FAILED, CAPPED, RUNNING and NEVER RAN lines, each naming the routine and the time, a finished run its commit, and a failure where its output is

#### Scenario: [ROUTINES-8-S2] A routine whose slots pass unrun is STALE
- **GIVEN** a daily routine whose last run was three days ago
- **WHEN** the health line runs
- **THEN** it says STALE with the date of the last run and the slots missed

#### Scenario: [ROUTINES-8-S3] A ticker that stopped is named
- **GIVEN** enabled routines and no tick recorded for more than an hour
- **WHEN** the health line runs
- **THEN** it says STALE for the ticker with the last tick's time and how to start one (`routines.sh ticker`)

#### Scenario: [ROUTINES-8-S4] Nothing to run is said, not silent
- **GIVEN** a registry with no enabled routine, or no registry at all
- **WHEN** the health line runs
- **THEN** it prints one OK line saying no routine is enabled, and where it looked

#### Scenario: [ROUTINES-8-S5] A health line that cannot read says so
- **GIVEN** jq is missing, or the run log is unreadable, or the registry does not validate
- **WHEN** the health line runs
- **THEN** it prints CANNOT CHECK, or FAILED for an invalid registry, with the reason, and exits 0

#### Scenario: [ROUTINES-8-S6] An unknown quota is said, with the limit it brings
- **GIVEN** `billing` is `subscription`, an enabled `claude -p` routine, and no counting reading of the 5-hour window
- **WHEN** the health line runs
- **THEN** it prints `CANNOT CHECK — routines quota: no current reading of the 5-hour window since <when>; claude -p routines limited to 3 runs a day (N of 3 runs used today)`, N counting every `claude -p` run started today
- **AND** `<when>` is the 5-hour window's `resetsAt` when it has passed, else the file's `at` (a null `pct` with a future `resetsAt`, or no `resetsAt`); with no `quota.json` the clause reads `no reading ever of the 5-hour window`
- **AND** with `caps.subscription.daily_runs` 2, the line says 2 where it says 3: the limit that binds
- **AND** the line passes the health-line contract's verdict check

#### Scenario: [ROUTINES-8-S7] A billing refusal is a failure, not a cap
- **GIVEN** a routine whose last run was refused `billing`: an API key variable set on a subscription, or an unusable key file in `api` mode
- **WHEN** the health line runs
- **THEN** it prints a FAILED line, never CAPPED, naming the routine, the time, and the variable or the key file's problem, never the key

### Requirement: Something ticks the runner without the panel
THE SYSTEM SHALL print, on `routines.sh ticker`, a launchd job (macOS) or a crontab line (Linux and WSL2) that ticks this registry every 5 minutes from the repository's root, and SHALL install nothing itself.

#### Scenario: [ROUTINES-9-S1] The ticker line names this repository
- **GIVEN** a repository at a path with a space in it
- **WHEN** `routines.sh ticker --launchd` and `routines.sh ticker --cron` run
- **THEN** each prints a job that runs `sh <repo>/.claude/routines.sh tick` every 300 s with that path quoted, its PATH, and its output appended to a file under the run-log directory
- **AND** neither writes a file outside its own output or changes any scheduler
- **AND** with `billing` `api` and a key file, neither carries the key or any line of the key file

### Requirement: A registry may live beside an attached repository's sidecar config
WHEN the registry is named with `--registry` outside the repository given with `--repo` THE SYSTEM SHALL keep its lock, run log and output beside that registry, run its commands in the repository, write nothing in the repository, and refuse a routine with `writes: true`.

#### Scenario: [ROUTINES-10-S1] A sidecar registry keeps its log beside it
- **GIVEN** a registry at `<home>/.clauductor/projects/<id>/routines.json` and a repository with no `.claude/` directory
- **WHEN** `routines.sh --registry <it> --repo <repo> tick` runs a due command routine
- **THEN** the command runs with the repository as its working directory
- **AND** the run log and output are under `<home>/.clauductor/projects/<id>/routines/`, and `git status` and every file in the repository are unchanged

#### Scenario: [ROUTINES-10-S2] A sidecar registry cannot declare writes
- **GIVEN** a sidecar registry with a routine marked `writes: true`
- **WHEN** it is validated
- **THEN** validation fails, saying a routine of an attached repository never writes to it

### Requirement: The checks never start the real claude
THE SYSTEM SHALL run every process check with a refusing `claude` first on PATH, so a check reaches only a fake claude it names itself, and a check that reaches the refusing one fails.

#### Scenario: [ROUTINES-11-S1] A check that forgets its fake claude fails, and spends nothing
- **GIVEN** a `claude -p` routine run from a check without `ROUTINES_CLAUDE`
- **WHEN** the runner calls `claude`
- **THEN** the refusing `claude` answers, the run's verdict is `failed` naming it
- **AND** the check that did so fails, and the real `claude` is never started

### Requirement: The billing mode is declared once, without calling claude
THE SYSTEM SHALL record the registry's `billing` only through `routines.sh setup --billing subscription|api`, and for `api` the path given with `--key-file` (never the key) as `api_key_file`; SHALL start no `claude` process and make no network call doing so; and SHALL refuse a declaration the environment or the key file contradicts.

#### Scenario: [ROUTINES-12-S1] Setup writes the mode and calls nothing
- **GIVEN** a registry with `"billing": null`, no API credential in the environment, and the checks' refusing `claude` first on PATH
- **WHEN** `routines.sh setup --billing subscription` runs
- **THEN** the registry's `billing` is `subscription`, every other field is unchanged
- **AND** the refusing `claude` was never called

#### Scenario: [ROUTINES-12-S2] Setup refuses a contradicted declaration
- **GIVEN** `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, or `CLAUDE_CODE_USE_BEDROCK=1`
- **WHEN** `routines.sh setup --billing subscription` runs
- **THEN** it exits non-zero naming the variable, and the registry is unchanged
- **AND** `--billing api` with no `--key-file`, or with `ANTHROPIC_AUTH_TOKEN` or a provider switch set, is refused the same way

#### Scenario: [ROUTINES-12-S3] Setup records a key file's path, never its key
- **GIVEN** a file outside the repository, mode 0600, holding `ANTHROPIC_API_KEY=<key>`
- **WHEN** `routines.sh setup --billing api --key-file <it>` runs
- **THEN** the registry's `billing` is `api` and `api_key_file` is that path, and no part of the key is in the registry or in setup's output
- **AND** `--key-file ~/keys/anthropic` is recorded as `~/keys/anthropic`, as written, never with the home directory expanded
- **AND** `--key-file "$HOME/keys/anthropic"`, already expanded by the user's shell, is recorded as `~/keys/anthropic`: any resolved path beginning with `$HOME/` is rewritten to `~/…`, so the registry never holds a home directory
- **AND** a relative `--key-file` without `~/` is recorded made absolute against the directory setup ran in, its logical `pwd` (a symlinked directory's link path, not its target), and rewritten to `~/…` when that lies under `$HOME/`
- **AND** with `HOME` empty or unset, setup refuses a `~/` path, "HOME is not set"
- **AND** a prunable worktree (listed, its directory gone) neither refuses the key file nor breaks setup
- **AND** a key file in a sibling directory sharing the repository's prefix (`<top>-keys/key`) is accepted: it is not under the repository
- **AND** a key file that is missing, a symlink or otherwise not a regular file, unreadable, inside the repository (as ROUTINES-6-S8 defines it), readable by group or others, without a matching line, with an empty value, or with two matching lines is refused with that case's own reason, and the registry is unchanged
- **AND** the accepted line forms of ROUTINES-6-S7 (export, quoted, CRLF, trailing blanks, no final newline, beside a comment) are accepted by setup too
