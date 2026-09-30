# PRD: The change process (OPS-7), and adopting it in an existing project

**Status:** decided 2026-09-30. OPS-7 builds it on top of OPS-1 (#19). StandingT adopts it after
everything below has shipped.

## Why

The template (#19) ships a change process: a roadmap row, a proposal just in time, owner approval, a
build task group by task group, merge, and archive with spec promotion. Three questions were still
open:

1. Should that process follow an existing format rather than a dialect of its own?
2. Does Claude Code have something built in that replaces it?
3. How do behaviour specs connect to tests?

StandingT, where the process was proven, has to be able to adopt the result without losing
anything.

## What the research found (2026-09-30)

- **Claude Code has no built-in durable change record.**
  - Plan mode writes to `~/.claude/plans` by default, and `plansDirectory` can move that. Approval
    happens inside one session.
  - Tasks do not persist.
  - Workflows, skills, hooks and `/code-review` run the work. They do not record a change.
  - So a thin record kept in git stays.
- **There is no plan or spec standard yet.** AGENTS.md is the only cross-tool standard, and it
  covers instructions. All the tools converge on plain Markdown in git, split into why/what, how and
  tasks: OpenSpec, GitHub Spec Kit, Kiro, BMAD, Agent OS and Superpowers.
- **OpenSpec is the only open tool with living specs.** It promotes spec deltas into standing specs
  when a change is archived. Spec Kit and Kiro write their specs once, and Kiro works only in its
  own IDE.
- **BDD fits as a practice, not as a runtime.** Given/When/Then scenarios are the acceptance format.
  Executable Gherkin needs a runner for each language plus step-definition glue, which breaks
  zero-install and tool-agnostic. LLMs write scenarios well and executable glue less well.
- **The OpenSpec CLI (v1.2.0) accepts files written by hand.** It needs no `config.yaml` and no
  `.openspec.yaml`. `list`, `validate` and `archive` work, and so do symlinks from `openspec/` to
  other folders. Scenario IDs in headers survive `archive`.

The full survey, with sources, is in the session record (market research, 2026-09-30).

## Decisions

- **D1 Format.** OpenSpec-compatible Markdown, written by clauductor's own skills, with no install.
  The folders are tool-neutral: `changes/<id>/` and `specs/<capability>/`. They stay configurable
  as `CHANGES_DIR` and `SPECS_DIR` in `.claude/project.conf`.
- **D2 The OpenSpec CLI is an optional module.** Turning it on creates `openspec/specs →
  ../specs` and `openspec/changes → ../changes` as symlinks, so `openspec validate` and
  `openspec archive` work on the same files. Nothing is migrated.
- **D3 Scenarios.** Every requirement has a SHALL or MUST line and at least one
  `#### Scenario: [CAP-n-Sn] title`, with **GIVEN** / **WHEN** / **THEN** / **AND** bullets.
  - The ID is `<CAPABILITY>-<requirement n>-S<scenario n>`, for example `AUTH-2-S1`.
  - IDs never change and are never reused. A removed scenario's ID is retired.
  - EARS wording ("WHEN … THE SYSTEM SHALL …") is allowed on the SHALL line.
- **D4 Scenario-to-test traceability is blocking from day one.**
  - Every scenario ID that a change adds or modifies must appear in at least one test file, in a
    test name, a comment or a tag, in any language. The other way to satisfy it is a line in the
    change's `tasks.md` that reads `(manual: <reason>)` or `(untestable: <reason>)`.
  - A grep-level shell check enforces this in the gate and as a merge-guard rule.
  - The reviewer agent checks that each citing test actually asserts the THEN.
  - After archive, the same check runs over the living specs, so deleting a test that covered a
    scenario fails the gate.
- **D5 Fast path.** A change whose diff fits in one sentence gets no proposal. It goes to a `fix/`
  or `ops/` lane. This is Anthropic's threshold, and Kiro's "quick spec" idea. The propose skill and
  the playbook say so.
- **D6 The build keeps a log.** `tasks.md` gains `## Progress` and `## Decision log` sections that
  the builder keeps current, borrowed from Codex ExecPlans, so a resumed lane continues from the
  file.
- **D7 Verify before merge.** Before `merge-pr`, a verify step checks that every task is ticked,
  that every scenario the change adds or modifies is cited (D4), and that the diff touches what
  `tasks.md` claims. This is borrowed from Spec Kit's analyze step.
- **D8 Approval covers the whole design.** Approval is the `**Approved:** date by owner` line, and
  it covers the design as written. A later edit to `design.md` removes the line, until the owner
  approves again. The wording is borrowed from Superpowers.
- **D9 Kept as they are:** ADRs in `docs/adr/` for decisions that outlive a change, the root
  `AGENTS.md`, and the owner-queue and change-queue cards.
- **Not adopted:**
  - executable Gherkin as a default; it stays an opt-in module, and its tags reuse the D3 IDs;
  - Spec Kit's layout;
  - Kiro's format;
  - plan mode as the record.

## OPS-7: building it in the template

Every item gets a check that fails when it is broken, per AGENTS.md rule 4.

1. **Scenario IDs.**
   - `specs/README.md`, `changes/README.md` and the propose skill give the D3 grammar.
   - `checks/changes.sh` refuses a scenario with no ID, a malformed ID, or a duplicate ID.
2. **The traceability check.**
   - `.claude/scenario-trace.sh` lists each scenario ID and the test files that cite it.
   - `checks/scenarios.sh` is its own falsified check.
   - It is a step in `scripts/ci/run-local.sh`, and a `pr-merge-guard` rule that blocks.
   - Where tests are found comes from `TEST_GLOBS` in `project.conf`, with defaults for common
     layouts.
3. **The fast path (D5):** added to the propose skill, the playbook and the lane templates' help.
4. **The log sections (D6):** added to the `tasks.md` shape, to `build-change` (the builder updates
   them), and to `checks/changes.sh`.
5. **The verify step (D7):** `/verify-change` runs as a step in `build-change` and in `merge-pr`.
6. **The approval rule (D8):** enforced in `checks/changes.sh`, by comparing the hash of
   `design.md` against the hash recorded beside the Approved line.
7. **The OpenSpec module (D2):**
   - The module creates the symlinks.
   - An optional check runs `openspec validate --all` when the CLI is present.
   - The template's own example change must pass `openspec validate` through the symlinks. This is
     tested in CI, and skipped with a stated reason where `openspec` is absent.
8. **Docs:** the playbook, principles and QUICKSTART are updated. The README change waits for
   REL-1.
9. **Guard what the OpenSpec CLI does not.** This was tested on 1.2.0 and 1.13.2.
   - **Refuse to archive in clauductor's own archive step and merge guard** a change that has:
     - no spec deltas, unless it declares `skip_specs: true` in `.openspec.yaml`, which clauductor
       also writes;
     - an unchecked task.

     `openspec archive -y` archives both, with only a warning.
   - **MODIFIED requirements copy the current block word for word first**, every scenario header
     included. OpenSpec matches scenarios by the whole header, not by the ID, and 1.2.0 silently
     deletes a scenario that a MODIFIED block leaves out. A check compares the living spec's
     scenario headers against the delta.
   - **A new capability's delta includes a `## Purpose`** of at least 50 characters. Otherwise
     archive writes "TBD", which fails strict validation.
   - **The OpenSpec module requires CLI 1.13 or later** and says so when it finds an older version.

## Adopting it in StandingT (after OPS-7 and the panel work ship)

StandingT keeps working as it is throughout. Every step below is an ordinary StandingT `ops/` PR
through `/merge-pr`.

- **Phase 0: the convergence map.** This is read-only.
  - Diff each file that exists in both StandingT and the template. `clauductor install --dry-run`
    listed 25 of them on 2026-09-30.
  - Classify each file one of three ways:
    - take the template's version;
    - move StandingT's additions into config, a module or the local layer;
    - upstream to the template first.
  - List every record format where StandingT and the template differ. Examples: the journal heading
    (`## Session N — date — author — focus`), the insight row (with Area and Topic, and StandingT's
    full status vocabulary), and the roadmap's Phase/Gate grammar with owner and started lines.
  - For each one, decide one of: the template parser accepts both, the format becomes a setting, or
    the skill stays StandingT's own.
  - **Existing records are never reformatted.** StandingT's roadmap, specs, archive, ADRs, journal,
    insights, owner queue and registries stay exactly where they are.
- **Phase 1: extension points in clauductor.**
  - Install and update merge `settings.json` instead of overwriting it.
  - Optional modules for StandingT's extras:
    - claude.ai artifacts and their currency (guard rule 8);
    - premise-check (rule 6);
    - write surfaces (rule 5);
    - people and the two-person lanes;
    - the living visual pages.
  - A project-local layer for health lines, guard rules and context sections that installs never
    overwrite.
  - A new `clauductor diff` command.
- **Phase 2: upstream.** Whatever the map marks as generic and better in StandingT moves into the
  template.
- **Phase 3: converge, file by file.**
  - Set `CHANGES_DIR=openspec/changes` and `SPECS_DIR=openspec/specs`, so no folders move.
  - Existing scenarios need no IDs. D4 applies only to scenarios that a change adds or modifies,
    and a backfill can come later.
  - StandingT's vitest meta-tests stay alongside the template's shell checks.
- **Phase 4: hand-over.**
  - Write `.claude/clauductor-template`.
  - From then on, `clauductor update` keeps StandingT current, and StandingT-specific parts live in
    config, modules and the local layer.

## Open

- **`TEST_GLOBS` defaults** for projects whose tests sit next to source, such as Go's `_test.go` and
  JS/TS's `*.test.ts`. Decide during OPS-7.
- **A backfill tool** that proposes IDs for existing scenarios, for StandingT's living specs.
  Optional, and Phase 3 or later.
