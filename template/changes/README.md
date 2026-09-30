# Changes

One directory per capability change that is proposed or in flight: `changes/<id>/`. It is created
**just in time** (when its roadmap row is next), approved by the owner, built group by group, and
**archived** when its PR merges: moved to `changes/archive/YYYY-MM-DD-<id>/`, its spec deltas
promoted into `specs/`. At most ONE change sits here proposed and unbuilt. To capture an idea for
later, write a roadmap row instead.

`.claude/checks/changes.sh` validates every open change against the shape below.

## `proposal.md`: what and why

```markdown
**Approved:** YYYY-MM-DD by <owner>          ← or: **Status:** awaiting approval
**Roadmap row:** <row id>

## Why
<the problem, and why now>

## What changes
<what a user can do afterwards that they cannot now>

## What the existing specs already guarantee
<read specs/ first; name the requirements this adds to, modifies or contradicts>

## Out of scope
<what it deliberately does not do, and which row or change owns each>
```

With the review-page module on (`REVIEW_PAGE="artifact"`), line 1 is instead
`**Review page:** https://claude.ai/artifact/<id>` and the Approved line follows it.

## `design.md`: how, and every decision

Each decision as the owner decided it, not as it was recommended: `D1 … Dn`, each with the choice,
the alternative it beat, and why. A decision still open says so and blocks the build of any group
that depends on it. A refusals table (each situation where the change does not do its thing, and
what happens instead) belongs here.

## `tasks.md`: the build plan, in task groups

```markdown
## 1. <group title>
- [ ] 1.1 <task>
- [ ] 1.2 <task, including its test>

## 2. <group title>
- [ ] 2.1 …

- [ ] Slice: a <role> can <action> at <where>
```

- A group is what one builder builds and one reviewer reviews: a coherent slice, reviewable in one
  round (`docs/principles.md`, *Size a change so one review round converges*).
- `build-change` (or `apply-change`) reads `## <n>. <title>` headings as groups and `- [ ]` lines
  as open tasks; builders tick them. Nothing else holds progress.
- The last line states the slice, or `Slice: exempt — <reason>` for pure substrate.
  `pr-merge-guard` rule 3 refuses a change PR without it.

## `specs/<capability>/spec.md`: deltas (optional)

What the change does to the living specs, in the same format they use (see `../specs/README.md`),
under `## ADDED Requirements`, `## MODIFIED Requirements` (the whole requirement as it will read)
and `## REMOVED Requirements` (the name, and why). `/archive-change` applies them to
`specs/<capability>/spec.md`.
