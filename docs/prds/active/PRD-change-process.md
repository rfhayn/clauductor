# PRD: The change process (OPS-7), and adopting it in an existing project

**Status:** decided 2026-09-30. OPS-7 items 1–17 and the economy-mode mapping are built (branch
`feature/OPS-7-change-process`, on OPS-1 #19); see *OPS-7: what was built*. StandingT adopts it
after everything below has shipped.

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
- **D10 A repository works without clauductor installed** (an invariant, decided 2026-09-30).
  - Everything a contributor needs lives in the repo and runs in Claude Code alone: the skills,
    hooks, checks, the gate and its queue lock, the merge guard, and the records.
  - The panel, the `clauductor` binary and the plugin are conveniences on top, never dependencies.
  - Anything that talks to the panel treats a missing panel as a normal state, not an error:
    - the status line's post to the panel;
    - session-start's panel line;
    - the economy file;
    - project cards.
  - **Enforced by `checks/no-clauductor.sh`** (OPS-12), which runs:
    - the template's gate;
    - the process checks;
    - a representative skill context script, and
    - the status line,

    all with `clauductor` absent from `PATH` and no panel running. It fails if anything errors,
    or asks for the binary or the panel. It is falsified by adding a hard `clauductor` call to a
    gate step.
  - The playbook and `AGENTS.md`'s "what executes" table name it.
  - At convergence, StandingT gets the same check, beside its existing gate-lock conformance test.
    This matters there because its designer works without clauductor, and possibly on Windows.
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

### Practices adopted from the industry gap analysis (2026-09-30)

The model already meets or beats the norm on Definition of Ready/Done, small batches,
human-in-the-loop checkpoints, traceability and controls that execute. The gaps it has are in
measurement, security hygiene and outcomes. The owner chose the following.

10. **Agent least privilege** (OWASP LLM01/LLM06).
    - The template's `settings.json` gains a `permissions.deny` list and the sandbox settings.
    - `checks/settings.sh` fails if the deny list or the sandbox is missing.
11. **Secrets and dependency hygiene.**
    - A secret-scanning step (gitleaks) goes in `scripts/ci/run-local.sh`, and a receipt is
      written only if it is clean.
    - A Dependabot or Renovate config with a minimum release age.
    - A scheduled dependency-audit workflow, upstreamed from StandingT's `audit.yml`, with an
      advisories health line.
12. **A cost budget per change** (Shape Up's appetite).
    - A roadmap row can carry `Budget: $N`, and `proposal.md` repeats it.
    - `build-change` stops, with a notification, when the change's lanes exceed it.
    - Archive records the actual cost in the change's log, read from the panel's cost figures or
      the transcripts.
13. **An outcome hypothesis per change.**
    - `proposal.md` gains a `## How we'll know` section: the observable signal, and when to look.
    - `checks/changes.sh` requires it.
    - Archive queues a follow-up roadmap row ("check the outcome of <id>") dated to that time.
14. **AI provenance trailers.**
    - Squash commits carry `Change:`, `Agent-Role:`, `Model:` and `Session:` trailers.
    - `pr-merge-guard` requires them when `model-roles.json` turns provenance on.
    - The template default is on. Clauductor's own repo turns it off, keeping its "no
      Co-Authored-By" rule.

Separate milestones, also adopted:

- **OPS-9: flow and DORA metrics.** Everything is computed from data that already exists: git,
  PRs, change records and receipts. It measures:
  - lead time and cycle time;
  - how long a change waits for the owner's approval;
  - review rounds;
  - aging work in progress;
  - deploy and merge frequency;
  - change-fail rate, meaning reverts and fix PRs linked to a change.

  It ships as `.claude/metrics.sh`, a health line and a panel card. DORA 2025 found that AI
  raises both throughput and instability, and this makes both visible.
- **OPS-10: evals for the model's own agents.**
  - A seeded-defect suite measures the reviewer agent's recall, in the same way as mutation
    testing.
  - `pr-merge-guard` requires an eval receipt for any PR that changes `.claude/agents/`,
    `.claude/workflows/` or `model-roles.json`.
- **REL-1 also adds a changelog and versioning.** A `CHANGELOG.md` (Keep a Changelog) is generated
  from Slice lines and PR titles, and `release-prep` cuts semver tags.

### Model selection (decided 2026-09-30)

Models are already chosen per role in one file, `.claude/model-roles.json`. Skill and agent
frontmatter, `settings.json`, `build-change` and the panel's `lane_types` restate each choice, and
`checks/model-roles.sh` fails when any restatement disagrees. What is missing is evidence and
adaptation. The owner adopted all four of the following.

15. **Risk tiers** (OPS-7).
    - `proposal.md` declares `**Risk:** low | normal | high`, and the owner approves the tier along
      with the design.
    - `model-roles.json` gives roles tiered variants. For example, the builder is sonnet/high for
      low risk and opus/xhigh for high.
    - `build-change` picks the variant for the declared tier.
    - `checks/model-roles.sh` covers the variants.
- **Cost per role** (in OPS-9). StandingT's `usage-report.mjs` (cost per role and model, from
  transcripts) is upstreamed as a portable script. Its output feeds the Metrics view's Cost tab.
- **Evals across models** (in OPS-10).
  - The reviewer's seeded-defect suite runs on each model and effort combination. Roles are chosen
    by catch rate per dollar.
  - Changing a role's model in `model-roles.json` requires a passing eval receipt for the new
    choice.
  - Haiku is a candidate for the mechanic role.
- **Quota economy mode** (in PANEL-19, with its mapping kept in `model-roles.json`).
  - Above a set 5-hour quota threshold, roles that are not critical (scribe, orient, mechanic) drop
    one tier. Reviewer and planner never drop.
  - The panel shows an "economy" badge by the quota and names the roles affected.

### Metrics in the panel (decided 2026-09-30): PANEL-19

Metrics appear at three depths:

- **Act now, where the owner already looks:**
  - Needs you shows an approval waiting longer than a set time, a change over its budget, and
    in-flight work with no commits for N days.
  - The lane header shows a budget bar next to Cost.
  - The economy badge sits by the quota.
- **At a glance:** an optional **Flow** card in the side panel shows median cycle time, merges per
  week, change-fail % and spend per week, each with a sparkline. Clicking it opens the view.
- **To explore:** a **Metrics** header button, next to Activity, opens a view with tabs **Flow**
  (DORA), **Cost** (by role, model, change and project), **Quality** (review rounds, the
  reviewer's eval recall by model, escaped defects) and **Outcomes** (each hypothesis, when it is
  due, whether it was checked). It has ranges of 7d, 30d and 90d, and shows this project or all
  projects.
- **Where the data comes from:** the project's `metrics.sh` (OPS-9) outputs JSON that the panel
  draws. The panel stays standalone. A repo without the template still gets merge frequency, PR
  cycle time and spend, which the panel computes itself from git, `gh` and status posts.

Later, at StandingT's go-live, and otherwise as needed: feature flags, SLOs, postmortems,
runbooks, the risk register, flaky-test quarantine, contract tests, SBOM/SLSA, retros, mutation and
property testing, a STRIDE section for changes that cross a trust boundary, and a cap on lanes.

Skipped, because they assume a human team: sprints and velocity, review SLAs, coverage percentages,
on-call, SPACE surveys, a formal betting table, and the full NIST AI RMF.

### OPS-7: what was built (2026-09-30)

Every item is done, 16 and 17 included (from the competitive survey, below). Each row names what
enforces it; each check was falsified (the rule broken, the
check seen to fail, the rule restored).

| # | Built | Enforced by |
|---|---|---|
| 1 | `[CAP-n-Sn]` grammar in `specs/README.md`, `changes/README.md`, `propose`, OpenSpec `config.yaml`; `.claude/lib/change.sh` reads it | `checks/changes.sh`: missing, malformed, duplicate, reused (living specs + archived deltas), mixed `CAP-n` |
| 2 | `.claude/scenario-trace.sh` (`--check`, `--change`, `--specs`, `--rev`, `--now`); `TEST_GLOBS` with defaults | `checks/scenarios.sh`; a `run-local.sh` step; `pr-merge-guard` rule 10 |
| 3 | Fast path in `propose`, the playbook, the principles and the fix/ops/propose lane templates | `checks/change-process.sh` |
| 4 | `## Progress` and `## Decision log` in `tasks.md`; the builder and `build-change` keep them | `checks/changes.sh`; `checks/change-process.sh` |
| 5 | `.claude/verify-change.sh` and `/verify-change`; last step of `build-change`, step 2c of `merge-pr` | `checks/change-tools.sh`; `pr-merge-guard` rules 9–10 |
| 6 | `**Approved:** … · design <hash>` over `design.md` and the Risk line; `.claude/change-approval.sh` | `checks/changes.sh`; `checks/change-tools.sh` |
| 7 | `.claude/modules/openspec/enable.sh` (symlinks, 1.13 floor); the example in `.claude/examples` | `checks/openspec.sh` (skips with a reason without the CLI; CI installs 1.13.2 and sets `OPENSPEC_REQUIRED=1`) |
| 8 | Playbook, principles, QUICKSTART, `changes/README.md`, `specs/README.md`, AGENTS.md rows | `checks/change-process.sh` for the wiring |
| 9 | No-delta and open-task refusal, `skip_specs`, MODIFIED verbatim copy, Purpose ≥ 50 | `checks/changes.sh`; `archive-change`; `pr-merge-guard` rule 11 |
| 10 | `permissions.deny` and `sandbox` in `settings.json` (keys checked against the Claude Code docs) | `checks/settings.sh` |
| 11 | gitleaks step (skips locally with a reason, fails under CI), Dependabot `cooldown`, `dependency-audit.yml`, `health/dependency-audit.sh` | `checks/gate.sh`; `checks/supply-chain.sh` |
| 12 | `Budget: $N` in rows and proposals; `.claude/change-cost.sh` (transcripts, list prices in `model-roles.json`); `build-change` stops; archive records the actual | `checks/roadmap.sh`, `checks/changes.sh`, `checks/change-tools.sh`; rule 11 |
| 13 | `## How we'll know`; the dated `ops/check-outcome-<id>` row; `roadmap-queue.sh --text` lists it DUE | `checks/changes.sh`; `checks/roadmap.sh`; rule 11 |
| 14 | `provenance` in `model-roles.json` (on); trailers in `build-change` commits and the `merge-pr` squash | `pr-merge-guard` rule 12; `checks/model-roles.sh` |
| 15 | `**Risk:**` tiers; `roles.<role>.tiers`; `build-change` TIERS; economy mapping and `ECONOMY` | `checks/model-roles.sh` (variants, strictly cheaper economy, reviewer and planner never drop) |
| 16 | Round grades (pass / concern / fail) in each group's Progress line; the stuck-loop breaker (a fixed finding back unchanged, peak and count not falling, a diff already reviewed) stops as `STUCK` | `checks/build-change.sh` (the loop's pure block, in node) |
| 17 | The compound step: `.claude/compound.sh`, `session-close` step 3b, `Not promoted — <why>` in the vocabulary | `checks/compound.sh` (no Raw row older than `COMPOUND_MAX_SESSIONS`) |

The cost's source: Claude Code's transcripts on the machine (`~/.claude/projects`, subagents
included), every assistant message on the change's branch in this repository or its worktrees,
once per message id, at the list prices in `model-roles.json`. The panel's Cost figure is not used:
it is per session, held in memory, and never sees a workflow's agents.

### Borrowed from the competitive survey (2026-09-30)

The survey of parallel-session panels and operating models (session record, "competitors")
found seven things worth taking. Where each is built:

| # | Borrowed | From | Built in |
|---|---|---|---|
| 1 | Auto-archive a lane when its PR merges | Claude Code Desktop, Conductor | PANEL-20 |
| 2 | A merge-readiness panel per lane (CI, review threads, todos) | Claude Code Desktop, Conductor | PANEL-20 |
| 3 | Auto-resume lanes when the 5-hour window resets | Codeman | PANEL-20 |
| 4 | Per-lane ports, setup and teardown scripts, `.worktreeinclude` parity | Webmux, Superset, uzi | PANEL-20 |
| 5 | Per-step grading and a stuck-loop breaker in the build → review loop | Kimchi Ferment | OPS-7 item 16 |
| 6 | An explicit "compound" step feeding insights back into rules | Compound Engineering | OPS-7 item 17 |
| 7 | Distribution of the operating model as a Claude Code plugin | Superpowers | OPS-11 (its own branch; packaging only) |

16. **Per-round grading and a stuck-loop breaker in `build-change`.** Each review round is graded
    `pass` (gate green, nothing medium or worse), `concern` (worst finding medium) or `fail` (gate
    red, or a high or critical finding), and the grades go into the group's Progress line. The loop
    stops, as `STUCK` with its own notify line, when a finding the builder fixed and did not dispute
    comes back unchanged, when the peak severity and the count of findings both stop falling, or
    when a round reviews a diff already reviewed. It no longer burns rounds to `maxRounds`.
    Three grades rather than Ferment's A–F, because each grade has a consequence and A–F's middle
    grades would not.
17. **The compound step.** `session-close` step 3b walks every insight still `Raw` (with its age in
    journal sessions) and decides each: promote it (an ADR, a rule or a check), mark it an instance
    of an existing rule, or record `Not promoted — <why>`. `archive-change` may do it early. A row
    still `Raw` after `COMPOUND_MAX_SESSIONS` sessions (default 3) fails `checks/compound.sh`.

## OPS-8: clauductor runs its own operating model

This comes after OPS-7 and before StandingT converges, as the rehearsal. Clauductor's own repo still
runs the old lock-based skills, has no ADRs, and does not use the model it ships.

- **Install the model into this repo**, and replace the old skills (claim, spawn, supervisor and the
  rest).
  - `AGENTS.md`, `project.conf` and `model-roles.json`, with the owner named. The commit trailer is
    **off**, to keep this repo's rule.
  - The roadmap rows carry the milestone ids (PANEL-n, OPS-n, REL-n).
- **Session start and session close run here as in StandingT:**
  - the context scripts;
  - merging own PRs through `merge-pr`, with evidence and a converged review;
  - archiving;
  - the roadmap, journal and insights updates;
  - the status line;
  - `machine-quiet`.
- **The gate:** `scripts/ci/run-local.sh` runs these and writes the receipt:
  - `go test -short` and `go test -race`;
  - gofmt and vet;
  - the template checks;
  - the browser suite.
- **The records:**
  - The journal and the insights log keep their history.
  - ADRs start at 0001 with this cycle's decisions: the change format, scenario traceability, the
    multi-repo panel, the install guard, and never killing a tmux server from a script.
- **The panel:** clauductor becomes the second project in the owner's panel, the first real
  multi-repo use.
- **Everything OPS-8 surfaces** (friction in install, update, the guard or the skills) is fixed in
  the template before StandingT's Phase 0.

## Adopting it in StandingT (after OPS-8)

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
  - **List the designer onboarding sections each change touches.** The files are
    `docs/onboarding-designer.md` and `docs/designer-welcome.html`. Both are written for a second
    contributor, and neither mentions the panel or clauductor today.
    - The sections expected to change are:
      - §5.6, Claude Code setup: the deny list and sandbox, a plugin install if adopted, and
        gitleaks;
      - §9, the operating model: the session steps, including compound; the new guards; and the
        skill names;
      - §10, branch, PR, merge: the merge evidence and the provenance trailers.
    - The designer's lane, the tokens, the app setup, the tools and the first task are unaffected.
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
  - Add the D10 check (`no-clauductor`) to StandingT's gate.
  - **Each PR that changes something the designer meets updates the matching onboarding section in
    the same PR,** using the Phase 0 list. The whole revision reaches the designer as one change,
    not a series.
  - **The panel is suggested to the designer, never required.** This comes after REL-1, so the
    install is a released download or Homebrew, not a source build.
    - Add an optional §5.9, "The panel (recommended on a Mac)": why it helps design work (dropping
      images into a lane, session status and notifications, lanes that survive, remote control),
      the install, and `panel init`, `trust` and `add` for StandingT. It also says to skip the
      panel on Windows, and that nothing else depends on it.
    - The welcome page gains one optional checklist item.
    - Open: whether the designer uses a Mac or Windows. That decides between "recommended" and
      "optional, Mac only".
    - Optional: the welcome checklist could save its ticks to the artifact's database, as the
      walkthroughs do, so session-start can report the designer's progress. Today it uses
      localStorage only.
- **Phase 4: hand-over.**
  - Write `.claude/clauductor-template`.
  - From then on, `clauductor update` keeps StandingT current, and StandingT-specific parts live in
    config, modules and the local layer.

## Open

- ~~**`TEST_GLOBS` defaults**~~ Decided in OPS-7: `*_test.go *.test.* *.spec.* test_*.py
  *_test.py tests/ test/ __tests__/ spec/`, over the files git knows, CHANGES_DIR and SPECS_DIR
  excluded.
- **A backfill tool** that proposes IDs for existing scenarios, for StandingT's living specs.
  Optional, and Phase 3 or later.
