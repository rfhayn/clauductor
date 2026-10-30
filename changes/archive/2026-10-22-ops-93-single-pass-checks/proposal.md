**Approved:** 2026-10-21 by Rich · design e88f01d40f03
**Roadmap row:** OPS-93
**Risk:** normal
**Budget:** $40

## Why

Every process check runs twice today. `checks/run.sh` runs the set, and one check in it,
`no-stridal.sh`, runs the whole set again inside the no-stridal environment
(`template/.claude/checks/no-stridal.sh:110-111`). That is ADR-0006's enforcement. This repo's
gate runs two sets, this repo's and the template's (`scripts/ci/steps.sh:100,107`), so each of them
pays for the second pass. ADR-0006 put the second pass at "about 45 s in the template". On
2026-10-18 a probe outside this repo measured 201.9 s of a 400.25 s template set on an 8-core Linux
machine. That probe is not in the repo, and its numbers have not been reproduced here (design.md,
*What rests on sources outside the repo*). Locally, the last 15 full passing gates in
`gate-runs.log` (2026-10-17 to 2026-10-18) had a median run of 2144 s, about 36 minutes. One gate
runs per machine at a time, so the second pass sets the pace of a session (OPS-89).

On 2026-10-18 the owner approved amending ADR-0006. Every check now runs once, inside the
no-stridal environment. A weekly double pass on `main` covers the one case the single pass
gives up, and the release notes tell adopters about the new risk. The decision arrived as a
handoff written outside this repo, with the amendment text attached. design.md records both, so
the amendment's `Source` line ("the owner's decision R23") points at a record this repo keeps.

**Step 0a.** Proposed changes already sit ahead of this one: PANEL-49, PANEL-46, OPS-42, OPS-40,
PANEL-25, PANEL-30 and PANEL-31 in `changes/`, and others on branches. The owner asked for this
proposal directly on 2026-10-18 and placed it pre-launch. That request answers step 0a. The row
stays queued behind the session plan's order (see *Placement*).

## What changes

- **One pass.** A whole-set `sh .claude/checks/run.sh` builds the no-stridal environment once
  and runs every check inside it: this directory's checks, the enabled modules' and
  `.claude/local/checks`. That environment is today's: `stridal` off `PATH`, a fresh `HOME`
  with no panel, and `CI` and `STRIDAL_LOCK_HELD` emptied. `no-stridal.sh` keeps its own
  parts (the output scan, the gate in a throwaway repo, the `GATE_STEPS` scan, the context script,
  the status line). It asserts that it runs inside that environment instead of running the set a
  second time.
- **The old behaviour on demand.** `CHECKS_DOUBLE_PASS=1` restores the double pass in any run,
  for rollback without a revert. `no-stridal.sh` run on its own still runs the set again, as
  it does today.
- **A weekly double pass on this repo's `main`.** It runs with a freshly built `stridal` on
  `PATH`, the runner's `HOME` and `CI=true`, so a check that fails only when those are present is
  still found: days after merge instead of before it. A failed run reaches session-start through
  `.claude/health/scheduled-workflows.sh`, which already reads every scheduled workflow.
- **ADR-0006 gets the amendment the owner approved**, word for word, in the build PR: design.md,
  *The amendment*.
- **The adopter risk goes into the release notes.** After the change, a project's own check (a
  module's, or one in `.claude/local/checks`) runs without the caller's `HOME`, so without its gh
  login, ssh keys or git signing, and without `CI=true`. A check that needs any of these fails
  after `update`. That is a build task (design D8). No release-notes file exists on `main` yet:
  `CHANGELOG.md` comes with REL-1 (#25, open).

**Why this PR does not insert the amendment into ADR-0006.** The amendment's Enforcement
paragraph describes a `run.sh` that builds the environment, and a `no-stridal.sh` that fails
outside it. Neither exists until the build. Merged now, the ADR would state a control that nothing
executes (AGENTS.md rule 4), and nothing would show it. The build PR inserts the text together
with the code that makes it true (design D9).

## What the existing specs already guarantee

Nothing. `specs/` holds four panel capabilities (`panel-attach`, `panel-claude-calls`,
`panel-lane-context`, `panel-new-lane`), and none of them covers the operating model's process
checks. The record for this invariant is ADR-0006, and the checks' own fixtures test it. So this
change has no spec delta (`.openspec.yaml`, `skip_specs: true`). Writing a new capability spec for
the check runner would be scope this change does not need.

## Placement

The owner wants this row pre-launch, after "rename step 2", and paired with OPS-89's "no second
run of the checks the template and this repository share". **No rename row exists in
`docs/roadmap.md`.** Nothing on `main` or on any pushed branch mentions it. Deps can name only
row ids and issues (`.claude/roadmap-queue.sh:29-31`), so an order against the rename cannot be
written there. The row sits right after OPS-89 in *Found on 2026-10-12*, and its Change cell
states the pairing and the rename order. Design O2 asks the owner how to make that order
executable. Launch is REL-1 (Phase 4), so any Phase 2 row comes before it.

## Out of scope

- **Running the checks the template and this repo share only once**: OPS-89, the pairing above.
- **Per-step timings in the gate log**: OPS-89.
- **Renaming the old name's `no-` check** (now `no-stridal`): the rename, which has no row yet (O2).
- **A double pass for adopters' own CI.** The weekly job is this repo's alone. An adopter can set
  `CHECKS_DOUBLE_PASS=1` (O5).

## How we'll know

- **Signal:** the median run time of full passing gates in `gate-runs.log` falls from 2144 s by
  at least 270 s. 2144 s is the median of the last 15 full passes before this change; 270 s is
  4.5 minutes, the low end of the saving the owner was quoted for the Mac. The weekly double pass
  has run on `main`, and has either passed or had its failure shown in session-start's
  scheduled-workflows line.
- **Check after:** 14 days
