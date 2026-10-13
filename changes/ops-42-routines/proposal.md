**Status:** awaiting approval
**Roadmap row:** OPS-42
**Risk:** high

## Why

Nothing in this repository, or in any project the template reaches, runs while the owner is away.
The only scheduled job is the weekly dependency audit on GitHub. Two things wait on that:

- **OPS-35 waits on an observation nothing makes.** The metrics check's flake (#62) has only ever
  failed under a full gate, and 64 runs and 300 checkout cycles by hand reproduced nothing. OPS-33
  made its next failure name the git step that failed; something has to run the gate often enough
  to catch one.
- **PRD-maps' Pulse layer** (decided 2026-10-12): routines from a registry, through one runner,
  with a run log and caps. PANEL-37 (the panel ticks them and shows the table) and PANEL-38 (Today
  shows their runs and cost) are built on this row.

The risk is unattended spend on the owner's subscription, and processes stopped by a script.
ADR-0005 exists because a cleanup script once killed every live lane. Hence **Risk: high**: the
build's models are its top tier.

## What changes

A project can:

- **name routines** in `.claude/routines.json`: a time (`02:30`, `Mon-Fri 18:15`, `every 30m`),
  and either a shell command, which costs no tokens, or a `claude -p` run with a role, read-only
  tools, a turn limit and a dollar budget;
- **run them** with `sh .claude/routines.sh tick`, every 5 minutes from a launchd or cron line that
  `routines.sh ticker` prints (and, after PANEL-37, from the panel). Due routines run one at a
  time, a missed slot catches up once, and an overlapping tick leaves at once;
- **bound them**: a timeout that stops only the routine's own child (SIGTERM, then SIGKILL 15 s
  later); for `claude -p`, a daily and a 7-day dollar cap, a daily run count, the account's quota
  when the panel has read one, and a refusal while an API key is set;
- **read what happened**: every run in `<git common dir>/clauductor/routines/runs.jsonl` with its
  commit, verdict, duration and, for `claude -p`, the cost and turns from claude's own JSON; each
  run's output kept; and at the next session start, one health line per routine (OK, FAILED,
  CAPPED, NEVER RAN, STALE, RUNNING) plus one that says when nothing is ticking;
- **use `routines.sh run`, `list` and `log`** by hand, and the same registry beside an attached
  repository's sidecar config (`--registry`, `--repo`), which never writes to that repository.

This repository gets three routines: `nightly-checks` (its process checks), `flake-watch` (the
full gate nightly with `claude` off PATH, for #62) and `morning-digest` (a read-only `claude -p`
summary of session-start's computed context, kept as a file the health line names).

The checks gain a refusing `claude` first on PATH for every check, so no test of a routine, or of
anything else, can start the owner's real Claude Code (#93, #95).

## What the existing specs already guarantee

`specs/` holds only its README: no living spec covers scheduling, health lines or the checks, so
nothing here modifies or contradicts a requirement. This change adds the capability spec
`routines`. What it builds on is documented, not specified:

- the health-line contract (`.claude/health/README.md`), which the new line follows;
- ADR-0005, which this change **amends** (design.md D8): its decision forbids a script to kill a
  `claude` process whatever its parent says, and a routine's timeout stops the `claude -p` it
  started. The amendment allows exactly that and nothing else;
- ADR-0006 (a repository works without clauductor): the runner is POSIX sh and jq, and the quota
  file is optional;
- PANEL-28's approved stop sequence for a `claude` child (SIGTERM, 15 s, SIGKILL), reused;
- AGENTS.md's "a model or effort is chosen in ONE place", which D6 keeps by naming a role.

**Scope.** Re-measured under `/propose` step 0c: the row's four files are new, and the change also
touches `checks/lib.sh`, `model-roles.json`, ADR-0005, the playbook and AGENTS.md's table (D1
weighs a split). This PR updates two roadmap rows: OPS-42's scope gains those files, and PANEL-37's
row gains the machine-wide cap across projects (D4). Both are revised to the owner's decisions.

## Out of scope

- **The panel ticking routines, the routines table and ▶ run now:** PANEL-37. So is a machine-wide
  dollar cap across projects (D4), added to its row by this PR.
- **Showing the digest and routine costs in Today:** PANEL-38.
- **Cloud routines:** the registry's `where` field is reserved for them. This runner runs only
  `local` and skips the rest. No row owns a cloud runner; PRD-maps keeps it for later.
- **Installing the ticker:** the owner's act (D11). `routines.sh ticker` prints it.
- **A routine that writes to the main checkout's tracked files:** allowed only with `writes: true`.
  None of the three needs it, and a sidecar registry refuses it.
- **Fixing #62 itself:** OPS-35, once `flake-watch` has caught a failure.
- **#93** (the panel's Go tests call the real `claude auth status`): its own fix. `flake-watch`
  strips `claude` from PATH so a nightly gate cannot reach the owner's login meanwhile.

## How we'll know

- **Signal:** with the ticker loaded, on at least 5 of the first 7 mornings the session start
  shows `OK` for `nightly-checks` and `morning-digest`, with a digest file the owner finds worth
  reading. No `CAPPED` line appears. Each digest costs under $0.30. `flake-watch` either stays OK
  or shows a FAILED line whose log holds the `fx` line OPS-35 needs.
- **Check after:** 14 days
