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
- **AND** the caps, the quota, the lock and the subscription rule apply to it as to a scheduled run

### Requirement: One routine at a time, under one lock
THE SYSTEM SHALL hold one lock per registry while a tick or a run-now is deciding or running, SHALL run due routines one after another inside it, and WHEN the lock is held by a live process THE SYSTEM SHALL exit without running anything and without waiting.

#### Scenario: [ROUTINES-3-S1] An overlapping tick leaves at once
- **GIVEN** a tick is running a routine that takes 60 s
- **WHEN** a second tick starts
- **THEN** the second tick exits 0 within a second, saying which routine holds the lock and since when
- **AND** it runs nothing and logs nothing

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

### Requirement: A claude -p routine runs on the subscription, bounded, and records its cost
THE SYSTEM SHALL run a `claude -p` routine only through the `claude` binary, with `--output-format json`, the model and effort of its role in `.claude/model-roles.json`, its tools, its turn limit and a budget no larger than what its caps leave; SHALL refuse it while `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set; and SHALL record the cost and turns its JSON reports.

#### Scenario: [ROUTINES-6-S1] A key in the environment refuses the run
- **GIVEN** `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set in the runner's environment
- **WHEN** a due `claude -p` routine would run
- **THEN** claude is not started, and the run log records a refusal naming the variable
- **AND** a due command routine in the same tick still runs, with both variables removed from its environment

#### Scenario: [ROUTINES-6-S2] The invocation carries the role, the tools and the bounds
- **GIVEN** a `claude -p` routine with role `routine`, `allowed_tools` `Read, Grep, Glob`, `max_turns` 6 and `max_usd` 0.50
- **WHEN** it runs against a fake claude that records its argv
- **THEN** the argv has `-p`, `--output-format json`, `--permission-mode dontAsk`, the role's model and effort, `--tools` and `--allowedTools` with exactly those tools, `--max-turns 6`, a `--max-budget-usd` no larger than 0.50, the write tools in `--disallowedTools`, and `--strict-mcp-config`
- **AND** it has no `--bare`
- **AND** the prompt holds the routine's context command's output

#### Scenario: [ROUTINES-6-S3] Cost and turns come from claude's JSON
- **GIVEN** a fake claude that answers with `total_cost_usd` 0.12 and `num_turns` 4
- **WHEN** the routine runs
- **THEN** the end record holds cost 0.12, turns 4, the model and the effort
- **AND** the result text is kept as the run's output

#### Scenario: [ROUTINES-6-S4] An unreadable answer counts at its budget
- **GIVEN** a fake claude that exits 1, or answers with no JSON, or is stopped by the timeout
- **WHEN** the run ends
- **THEN** its verdict is `failed` or `timeout`, its cost is recorded as unknown
- **AND** the caps count that run at the budget it was given

#### Scenario: [ROUTINES-6-S5] An error result is a failed run
- **GIVEN** a fake claude whose JSON has `is_error: true`, or a subtype other than `success`
- **WHEN** the run ends
- **THEN** its verdict is `failed`, with the subtype recorded, and its reported cost is still counted

### Requirement: Caps and the quota bound unattended spend
THE SYSTEM SHALL refuse to start a `claude -p` run when today's recorded spend has reached `daily_usd`, when the spend over the last `window_days` has reached `window_usd`, when today's `claude -p` runs have reached `daily_runs`, or when a current reading of the account's quota has any window at or above `quota_pct`; and SHALL log each refusal.

#### Scenario: [ROUTINES-7-S1] The daily dollar cap refuses the next run
- **GIVEN** `daily_usd` 1.00 and today's runs recorded at 1.02
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `capped` naming the daily cap and the spend

#### Scenario: [ROUTINES-7-S2] The window cap and the run count refuse too
- **GIVEN** `window_usd` 5.00 over 7 days with 5.10 recorded, or `daily_runs` 6 with six `claude -p` runs today
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `capped` naming that cap

#### Scenario: [ROUTINES-7-S3] The budget given is what the caps leave
- **GIVEN** `daily_usd` 1.00 with 0.80 spent today and a routine with `max_usd` 0.50
- **WHEN** it runs against a fake claude
- **THEN** it is given `--max-budget-usd 0.20`

#### Scenario: [ROUTINES-7-S4] Command routines are never capped
- **GIVEN** every cap is reached
- **WHEN** a command routine is due
- **THEN** it runs

#### Scenario: [ROUTINES-7-S5] A high quota reading refuses, an unknown one does not
- **GIVEN** the panel's `quota.json` reports the 5-hour window at 72% with `quota_pct` 60, read 10 minutes ago and not yet reset
- **WHEN** a `claude -p` routine is due
- **THEN** it does not run, and the log records `quota` with the window and its percentage
- **AND** with no `quota.json`, a reading older than 6 hours, or a window past its reset, it runs and the record says the quota was unknown

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

### Requirement: Something ticks the runner without the panel
THE SYSTEM SHALL print, on `routines.sh ticker`, a launchd job (macOS) or a crontab line (Linux and WSL2) that ticks this registry every 5 minutes from the repository's root, and SHALL install nothing itself.

#### Scenario: [ROUTINES-9-S1] The ticker line names this repository
- **GIVEN** a repository at a path with a space in it
- **WHEN** `routines.sh ticker --launchd` and `routines.sh ticker --cron` run
- **THEN** each prints a job that runs `sh <repo>/.claude/routines.sh tick` every 300 s with that path quoted, its PATH, and its output appended to a file under the run-log directory
- **AND** neither writes a file outside its own output or changes any scheduler

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
