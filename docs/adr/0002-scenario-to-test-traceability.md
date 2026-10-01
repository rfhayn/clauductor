# ADR 0002: Every scenario a change adds or modifies is cited by a test, blocking from day one

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: template, change-process, testing
- **Source**: PRD-change-process D3, D4, D7; OPS-7 (#22)

## Context
A spec scenario with no test reads exactly like a covered one. Without a link from scenario to
test, the specs drift into documentation and a deleted test leaves no trace.

## Decision
- Every requirement has a SHALL/MUST line and at least one `#### Scenario: [CAP-n-Sn] title` with
  GIVEN/WHEN/THEN bullets. IDs never change and are never reused.
- Every scenario ID a change adds or modifies must appear in at least one test file (in a name, a
  comment or a tag, any language), or be escaped in `tasks.md` with `(manual: <reason>)` or
  `(untestable: <reason>)`. Test files come from `TEST_GLOBS` in `project.conf`.
- After archive the same rule runs over the living specs, so deleting the last citing test fails.
- The check is grep-level on purpose; the reviewer agent checks that each citing test asserts the
  THEN.

## Consequences
### Positive
- Coverage of behaviour is visible and enforced without a framework or a coverage percentage.

### Negative / trade-offs
- A citation proves a test names the scenario, not that it tests it; that half is review.
- An adopting project with old scenarios sets `SCENARIO_IDS="new-only"`.

## Enforcement
`.claude/scenario-trace.sh --check`, a step of every `scripts/ci/run-local.sh` run and
`pr-merge-guard.sh` rule 10 (blocking); `/verify-change` before merge; `checks/scenarios.sh`
falsifies it. In this repo `TEST_GLOBS` is the Go tests plus `template/.claude/checks/*.sh`.

## Related
- ADR-0001
