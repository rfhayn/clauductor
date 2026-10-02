# ADR 0008: Provenance and attribution trailers default on in the template, off in this repo

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: template, git, process
- **Source**: PRD-change-process item 14; OPS-7 (#22); OPS-8

## Context
Commits written by agents can say which change, role, model and session wrote them (`Change:`,
`Agent-Role:`, `Model:`, `Session:`) and carry a `Co-Authored-By` trailer. That history is useful
for cost, quality and audit questions. This repository has always had the opposite rule: commits
carry no `Co-Authored-By`.

## Decision
- The template ships both ON (`model-roles.json`: `attribution.enabled`, `provenance.enabled`):
  a new project gets the history by default.
- Clauductor's own repository turns both OFF and keeps its rule: no `Co-Authored-By`, no
  provenance trailers; commits and PR titles start with the roadmap row id (`PREFIX-N:`).
- `model-roles.json` is the one place the choice is made; `build-change.js` restates it because a
  workflow cannot read the file.

## Consequences
### Positive
- Each project decides once, in one file, and the merge guard follows it.

### Negative / trade-offs
- Turning it off means editing `build-change.js`, a framework-tier file, so `clauductor update`
  reports it as modified on every run (OPS-8 finding; owned by OPS-14).
- Without provenance, this repo's cost and quality metrics cannot attribute a commit to a role.

## Enforcement
`pr-merge-guard.sh` rule 12 requires the trailers while `provenance.enabled` is true (off here);
`checks/model-roles.sh` fails when `build-change.js`'s `PROVENANCE` and `ATTRIBUTION_DEFAULT`
disagree with `model-roles.json`. Nothing checks that a hand-written commit in this repo has no
`Co-Authored-By`: that is review.

## Amendment — 2026-10-01: build-change reads the choice at run time
- **Source**: OPS-14 (#36), which closed the OPS-8 finding above; OPS-21 records it here.

The decision stands: on in the template, off here, chosen in `model-roles.json` alone. Three things
above no longer hold, and this amendment replaces them:
- `build-change.js` no longer restates the choice. It has no `PROVENANCE` or `ATTRIBUTION_DEFAULT`
  constant. At run time it reads `attribution` and `provenance` (with the branch prefixes, the
  changes directory, the gate and its quick flags) from `.claude/project-config.sh --json`.
- So turning attribution or provenance off edits only `model-roles.json`. No framework file differs
  on purpose, and `clauductor update` no longer reports `build-change.js` as modified. The first
  negative trade-off is gone; the second (no role attribution here) stands.
- `checks/model-roles.sh` no longer compares constants in `build-change.js` with `model-roles.json`,
  because there are none to compare. The original Enforcement paragraph's second clause is void.

**Enforcement** (replacing the original's second clause): `checks/build-change.sh` fails when
`build-change.js` carries a project setting as a constant (`const PROVENANCE =`, `const
ATTRIBUTION`, a literal branch prefix). It also runs `project-config.sh --json` against a project
with both off and feeds the output to the workflow's own `projectSettings` and `trailerBlock`.
`checks/model-roles.sh` holds `provenance` and `attribution` in `model-roles.json` to their shape.
`pr-merge-guard.sh` rule 12 is unchanged. Nothing checks a hand-written commit here for
`Co-Authored-By`: that is still review.

## Related
- ADR-0001; `.claude/model-roles.json`; `.claude/project-config.sh`
