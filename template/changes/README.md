# Changes

One directory per capability change that is proposed or in flight: `changes/<id>/`. It is created
**just in time** (when its roadmap row is next), approved by the owner, built group by group, and
**archived** when its PR merges: moved to `changes/archive/YYYY-MM-DD-<id>/`, its spec deltas
promoted into `specs/`. At most ONE change sits here proposed and unbuilt. To capture an idea for
later, write a roadmap row instead.

**The fast path.** A diff that fits in one sentence (a typo, a bump, a one-line fix) gets no change
directory: it goes to a `fix/` or `ops/` lane and `/merge-pr`. A change is for what alters what a
user can do.

The format is OpenSpec's, written by hand with nothing installed; the optional OpenSpec module
(`.claude/modules/openspec`) lets its CLI read the same files. A complete example the checks hold
correct is in `.claude/examples/changes/add-greeting-name/`. `.claude/checks/changes.sh` validates
every open change against the shape below.

## `proposal.md`: what and why

```markdown
**Approved:** YYYY-MM-DD by <owner> · design <hash>   ← or: **Status:** awaiting approval
**Roadmap row:** <row id>
**Risk:** low | normal | high
**Budget:** $N                                         ← when the roadmap row has `Budget: $N`

## Why
<the problem, and why now>

## What changes
<what a user can do afterwards that they cannot now>

## What the existing specs already guarantee
<read specs/ first; name the requirements this adds to, modifies or contradicts>

## Out of scope
<what it deliberately does not do, and which row or change owns each>

## How we'll know
- **Signal:** <what you will observe once it is in use>
- **Check after:** <n> days          ← or: **Check on:** YYYY-MM-DD
```

- **The approval covers the design as written.** `sh .claude/change-approval.sh <id> --record
  "<owner>"` writes the Approved line with a hash of `design.md` and the Risk line, and only on the
  owner's word. An edit to either afterwards voids it: the check fails until the owner approves
  again (`--revoke`, then `--record`).
- **Risk** picks the build's models: a role with a variant for the tier in
  `.claude/model-roles.json` runs on it (the builder is sonnet for low risk, opus/xhigh for high).
- **Budget** is the change's appetite in dollars, repeated from its roadmap row.
  `.claude/change-cost.sh <id>` measures spend from this machine's transcripts at list price;
  `build-change` stops when a group takes it over, and archive records the actual cost.
- **How we'll know** is the outcome hypothesis: archive queues a roadmap row to check it then.

With the review-page module on (`REVIEW_PAGE="artifact"`), line 1 is instead
`**Review page:** https://claude.ai/artifact/<id>` and the Approved line follows it.

## `design.md`: how, and every decision

Each decision as the owner decided it, not as it was recommended: `D1 … Dn`, each with the choice,
the alternative it beat, and why. A decision still open says so and blocks the build of any group
that depends on it. A refusals table (each situation where the change does not do its thing, and
what happens instead) belongs here.

## `tasks.md`: the build plan, in task groups, and its log

```markdown
## Progress
- YYYY-MM-DD group 1 (<title>) built and reviewed: converged in 2 round(s), peak low

## Decision log
- YYYY-MM-DD group 1: <the choice> over <the alternative>, because <why>

## 1. <group title>
- [ ] 1.1 <task>, tested in `<test file>` citing [CAP-1-S1]
- [ ] 1.2 [CAP-1-S2] checked by hand (manual: <why no test can reach it>)

## 2. <group title>
- [ ] 2.1 …

- [ ] Slice: a <role> can <action> at <where>
```

- A group is what one builder builds and one reviewer reviews: a coherent slice, reviewable in one
  round (`docs/principles.md`, *Size a change so one review round converges*).
- `build-change` (or `apply-change`) reads `## <n>. <title>` headings as groups and `- [ ]` lines
  as open tasks; builders tick them.
- **`## Progress` and `## Decision log`** are the build's log: the builder records each decision
  `design.md` does not settle, and build-change a line per committed group, so a resumed lane
  continues from the file. Archive adds the actual cost to Progress.
- A task names the scenario IDs its test covers. A scenario no test can reach is escaped on the
  line naming it: `(manual: <reason>)` or `(untestable: <reason>)`, the reason never blank.
- The last line states the slice, or `Slice: exempt — <reason>` for pure substrate.
  `pr-merge-guard` rule 3 refuses a change PR without it.
- A build PR merges only with every task ticked (the Slice line aside): `pr-merge-guard` rule 9,
  and `/verify-change` before it.

## `specs/<capability>/spec.md`: deltas

What the change does to the living specs, in the same format they use (see `../specs/README.md`):

- `## ADDED Requirements`: new requirements, each with new scenario IDs.
- `## MODIFIED Requirements`: the whole requirement as it will read. **Copy the current block word
  for word first**, every scenario header included, then edit it: OpenSpec matches scenarios by
  their whole header, and one left out or reworded is deleted on archive. The check compares them.
- `## REMOVED Requirements`: the name, and why. Its scenario IDs are retired, never reused.
- A **new capability**'s delta opens with `## Purpose` of at least 50 characters.
- **A change with no delta** (a refactor, tooling) says so: `changes/<id>/.openspec.yaml` with
  `skip_specs: true`. Archive refuses a change with neither.

`/archive-change` applies them to `specs/<capability>/spec.md`.

## Scenarios and their tests

Every scenario is `#### Scenario: [CAP-n-Sn] <title>` with **GIVEN** / **WHEN** / **THEN** /
**AND** bullets (`specs/README.md` gives the grammar). Every scenario ID a change adds or modifies
must appear in at least one test file (in a test name, a comment or a tag, in any language), or be
escaped in `tasks.md`. `.claude/scenario-trace.sh` lists each ID and the tests that cite it; the
gate and `pr-merge-guard` rule 10 fail an uncited one once the change's tasks are all ticked, and
for the living specs always. Where tests live is `TEST_GLOBS` in `.claude/project.conf`.
