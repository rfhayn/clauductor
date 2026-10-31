# ADR 0006: A repository works without stridal installed

- **Status**: Superseded by ADR-0016 (2026-10-31)
- **Date**: 2026-09-30
- **Tags**: template, panel, plugin
- **Source**: PRD-change-process D10 (OPS-12, #23); built in OPS-8

## Context
Standing Tee's designer works without stridal, possibly on Windows. If a gate step, a hook or a
skill quietly called the binary or the panel, the repository would work for the owner and break
for every other contributor, and nothing on the owner's machine would show it.

## Decision
- Everything a contributor needs lives in the repo and runs in Claude Code alone: skills, hooks,
  checks, the gate and its queue lock (`lease.sh` when there is no binary), the merge guard and
  the records.
- The panel, the binary and the plugin are conveniences on top, never dependencies.
- Anything that talks to the panel treats a missing panel as a normal state: the status line's
  post, session-start's panel line (no longer reported as unhealthy), the economy file, the cards.

## Consequences
### Positive
- A second contributor needs only Claude Code, git, jq and the project's own toolchain.

### Negative / trade-offs
- Every check runs a second time inside `no-stridal.sh` (about 45 s in the template), and every
  new panel feature needs a no-panel path.

## Enforcement
`template/.claude/checks/no-stridal.sh`: with `stridal` absent from PATH (its directory
mirrored minus the binary) and a HOME with no panel, it runs the gate in a throwaway repo, every
other process check, session-start's context script and the status line, and fails on an error or
a line asking for the binary or the panel; it scans `GATE_STEPS` for a bare `stridal` call.
Falsified in fixtures (a gate step and a context script that call it) and by hand during OPS-8 (a
hard call in `steps.sh`, and in `statusline.sh`, each failed the check). Named in AGENTS.md's table
and the playbook. This repo's own gate runs it, and its plugin step uses `go run`, not the binary.

## Amendment — 2026-10-18: the checks run once, inside the no-stridal environment
- **Source**: OPS-93; the owner's decision R23 (2026-10-18). Measured the same day: in the template's
  check set, `no-stridal` took 201.9 s of 400.25 s, the other 43 checks 198.1 s together.

The decision stands: everything a contributor needs runs in Claude Code alone, and the binary, the
panel and the plugin stay conveniences. How the check proves it changes. This amendment replaces the
first negative trade-off above and the Enforcement paragraph's "every other process check":
- The check set no longer runs twice. `checks/run.sh`, run for the whole set, builds the
  no-stridal environment once and runs every check inside it: this directory's checks, the enabled
  modules' and `.claude/local/checks`. That environment is unchanged: every `PATH` directory that
  holds `stridal` replaced by a mirror of links to everything else in it, a fresh `HOME` with no
  `~/.stridal` and only a git identity, and `CI` and `STRIDAL_LOCK_HELD` emptied. One helper in
  `checks/lib.sh` builds it for `run.sh` and `no-stridal.sh` alike, so the two cannot drift.
- `no-stridal.sh` keeps its own parts: the output scan, the gate in a throwaway repo and its
  falsification, the `GATE_STEPS` scan, session-start's context script and the status line. Instead of
  re-running the set, it asserts that it runs inside that environment, started by a whole-set
  `run.sh`. Run on its own, it re-runs the set as before.
- What the double pass caught and this does not: a check that passes without stridal but fails
  with it on `PATH`, with the caller's `HOME`, or with `CI` set. No check in the template reads any of
  these from its caller today; each builds its own. A weekly job on `main` runs the old double pass
  (`CHECKS_DOUBLE_PASS=1`) with a freshly built `stridal` on `PATH`, so such a check is still found,
  days after merge instead of before it.
- The first trade-off's "about 45 s" was measured with 26 checks. With 44 the second pass cost 201.9 s
  on an 8-core Linux machine, half the set. The single pass removes it. In its place, a module's or a
  project's own check now also runs without the caller's `HOME` (its gh login, ssh keys, git signing).
  A check that needs them sets them up in its own scratch, as the template's do.

**Enforcement** (replacing the original's "every other process check" clause): `checks/run.sh` runs the
whole set inside the environment `checks/lib.sh` builds. `checks/no-stridal.sh` fails when it runs
outside that environment under a whole-set `run.sh`, and when `stridal` is still on the stripped
`PATH`. Its gate, `GATE_STEPS`, context-script and status-line parts and their fixtures are unchanged.
Falsified in fixtures: a check that calls `stridal` fails the single pass; a `run.sh` that skips the
environment fails `no-stridal`. The scheduled double pass on `main` keeps the case the single pass
gives up. `CHECKS_DOUBLE_PASS=1` restores the old behaviour in any run.

## Related
- Superseded by ADR-0016 (the owner, 2026-10-31): its reason stopped binding when ADR-0011 made the
  plugin the install.
- ADR-0007; Standing Tee gets the same check at convergence (roadmap ST-3)
