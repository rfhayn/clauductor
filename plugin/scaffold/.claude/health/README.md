# Health lines

`session-start` runs every `*.sh` in this directory and prints each one's output under its name.
The directory is the list (nothing to register): add a script and it runs next session; delete it
and it stops. After these come the enabled modules' `health/*.sh` and the project's own
`.claude/local/health/*.sh` (which `clauductor update` never touches), all through
`${CLAUDE_PLUGIN_ROOT}/extensions.sh health`. Each runs under the interpreter its `#!` line names
(`#!/usr/bin/env bash` runs under bash, never forced through `sh`, which is dash on Ubuntu); a file
with no `#!` line runs under `sh`.

**Why they exist.** A control with no natural reader (a scheduled job, a push-to-main run, a
timer) fails where nobody is looking. The session-start context block is the one place a person
and an agent look at every session, so it is the addressee of record for all of them (*A control
needs a named addressee*, `docs/principles.md`).

## The contract

- Print one line per subject, starting with its verdict: `OK`, `FAILED`, `STALE`, `STUCK` (queued
  or running for more than a day), `RUNNING`, `NEVER RAN`, or `CANNOT CHECK — <why>`. Anything that
  is not `OK` is repeated in the session summary.
- **Name the subject**: the commit, the date, the run a verdict covers. A green result for a
  two-week-old run is consumed as coverage.
- **A check that could not run says `CANNOT CHECK`**, never nothing and never `OK`: a missing tool,
  a failed `gh`, no network. Absence must not read as health.
- Exit 0 even when reporting a failure: the verdict is the text (a non-zero exit, or no output at
  all, is shown as `CANNOT CHECK`). Keep it under a few seconds; it
  runs at every session start.
- With `CONTEXT_OFFLINE=1` (set by `${CLAUDE_PLUGIN_ROOT}/checks`), make no network call: print
  `CANNOT CHECK — offline`.

## Shipped

| Script | Reports |
|---|---|
| `worktrees.sh` | Each lane worktree under `.claude/worktrees/`: branch, clean or dirty, merged or not. |
| `scheduled-workflows.sh` | Each workflow whose `on:` has a `schedule:` (every spelling of `on:`): its last scheduled run's result, date, age and cron, `STALE` past 45 days (GitHub disables a schedule at 60), `STUCK` when queued for days. A failure says whether the run ever started (zero steps: a billing block, not a finding) and when the last clean run on the default branch was, drawing no conclusion from it. |
| `push-main.sh` | Each workflow whose `on:` has `push`: its latest push run on `main`, its age and URL, and its subject: `OK` only when it ran on the current `origin/main`, `STALE` with the count of commits it does not cover otherwise. |
| `flow.sh` | Flow and cost over the last 30 days, from `${CLAUDE_PLUGIN_ROOT}/metrics.sh --line`: median cycle and lead time, merges per week, change-fail rate, approval wait, review rounds, spend per week, work in flight. A figure it cannot compute is "—", and the line says why. |
| `dependency-audit.sh` | The weekly dependency audit (`.github/workflows/dependency-audit.yml`): `ADVISORIES` when its last run found one of high severity, `STALE` past two weeks. |

The two GitHub ones are thin callers: their logic is `${CLAUDE_PLUGIN_ROOT}/lib/health.sh`, which `clauductor
update` keeps current while this directory stays yours. Delete them if you do not use Actions. Project-specific health (a dependency audit, a
migration ledger, a backup's age) goes here as its own script.
