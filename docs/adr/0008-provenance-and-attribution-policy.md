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

## Related
- ADR-0001; `.claude/model-roles.json`
