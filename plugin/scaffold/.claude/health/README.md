# Health lines

`session-start` runs every `*.sh` in this directory and prints each one's output under its name.
The directory is the list (nothing to register): add a script and it runs next session; delete it
and it stops.

**Why they exist.** A control with no natural reader (a scheduled job, a push-to-main run, a
timer) fails where nobody is looking. The session-start context block is the one place a person
and an agent look at every session, so it is the addressee of record for all of them (*A control
needs a named addressee*, `docs/principles.md`).

## The contract

- Print one line per subject, starting with its verdict: `OK`, `FAILED`, `STALE`, `NEVER RAN`, or
  `CANNOT CHECK — <why>`. Anything that is not `OK` is repeated in the session summary.
- **Name the subject**: the commit, the date, the run a verdict covers. A green result for a
  two-week-old run is consumed as coverage.
- **A check that could not run says `CANNOT CHECK`**, never nothing and never `OK`: a missing tool,
  a failed `gh`, no network. Absence must not read as health.
- Exit 0 even when reporting a failure: the verdict is the text. Keep it under a few seconds; it
  runs at every session start.
- With `CONTEXT_OFFLINE=1` (set by `${CLAUDE_PLUGIN_ROOT}/checks`), make no network call: print
  `CANNOT CHECK — offline`.

## Shipped

| Script | Reports |
|---|---|
| `worktrees.sh` | Each lane worktree under `.claude/worktrees/`: branch, clean or dirty, merged or not. |
| `scheduled-workflows.sh` | Each GitHub Actions workflow with a `schedule:` trigger: its last run's result and date. |
| `push-main.sh` | The latest `push` run on `main` per workflow, when not a success. |
| `flow.sh` | Flow and cost over the last 30 days, from `${CLAUDE_PLUGIN_ROOT}/metrics.sh --line`: median cycle and lead time, merges per week, change-fail rate, approval wait, review rounds, spend per week, work in flight. A figure it cannot compute is "—", and the line says why. |
| `dependency-audit.sh` | The weekly dependency audit (`.github/workflows/dependency-audit.yml`): `ADVISORIES` when its last run found one of high severity, `STALE` past two weeks. |

Delete the GitHub ones if you do not use Actions. Project-specific health (a dependency audit, a
migration ledger, a backup's age) goes here as its own script.
