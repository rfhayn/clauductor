# ADR 0001: Changes and specs are OpenSpec-compatible Markdown, written by our own skills

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: template, change-process
- **Source**: PRD-change-process D1, D2, D9; OPS-7 (#22)

## Context
The operating model needs a durable record of each unit of work (proposal, design, tasks) and of
what the system does (living specs). Spec Kit, Kiro, OpenSpec and plan mode each offer a format.
Adopting a tool's CLI as a dependency would make every project install it; inventing a private
format would strand projects that later want the tool.

## Decision
- A change lives in `changes/<id>/{proposal,design,tasks}.md` plus spec deltas; living specs in
  `specs/<capability>/spec.md`. The folders are configurable (`CHANGES_DIR`, `SPECS_DIR`).
- The format is OpenSpec-compatible Markdown, written by clauductor's own skills (`/propose`,
  `/archive-change`) with no install.
- The OpenSpec CLI is an optional module: it symlinks `openspec/{specs,changes}` to the same
  folders, so `openspec validate` works on them. Nothing is migrated.
- Not adopted: Spec Kit's layout, Kiro's format, plan mode as the record, executable Gherkin as a
  default.

## Consequences
### Positive
- No dependency; a project can still use the OpenSpec CLI on the same files.
- One shape for build-change, verify-change and the merge guard to read.

### Negative / trade-offs
- We guard what the OpenSpec CLI does not (archiving a change with no deltas or open tasks,
  MODIFIED blocks that drop scenarios), and must track its format across versions (tested on 1.2.0
  and 1.13.2).

## Enforcement
`template/.claude/checks/changes.sh` (the shape, the approval line, scenario IDs, the archive
rules); `checks/openspec.sh` validates the template's example change through the module's
symlinks (CI installs the CLI and sets `OPENSPEC_REQUIRED=1`); `pr-merge-guard.sh` rule 11.

## Related
- ADR-0002 (scenario traceability); `template/changes/README.md`, `template/specs/README.md`
