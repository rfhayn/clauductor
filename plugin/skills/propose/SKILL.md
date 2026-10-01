---
name: propose
model: opus
effort: xhigh
description: "Propose the next change just in time: check nothing else is proposed ahead, read the specs it touches, draft changes/<id>/{proposal,design,tasks}.md (and spec deltas), put it in front of the owner for approval, and STOP until they approve. On approval, record it and land the proposal. TRIGGER when the user says 'propose <row>', 'propose the next change', 'write the proposal for', or a panel propose lane starts."
argument-hint: <change-id or roadmap row> [description]
---

# Propose a change

A proposal is written when its roadmap row is NEXT, never earlier, and nothing is built until the
owner approves its design (AGENTS.md, *Who decides*). The format is in `changes/README.md`, and
`${CLAUDE_PLUGIN_ROOT}/examples/changes/add-greeting-name/` is a complete one that the checks hold correct.

## The fast path: no proposal at all

**If the diff fits in one sentence, it gets no proposal.** A typo, a dependency bump, a one-line
fix, a rename, a doc edit: it goes to a fix or ops lane (`BRANCH_FIX`, `BRANCH_OPS`; the panel's *Fix an issue* or *Ops
task* template), is gated, reviewed and merged with `/clauductor:merge-pr`, and leaves no change directory.
Anthropic's threshold for when a plan is worth writing, and Kiro's "quick spec": a proposal costs
the owner a review, so it has to buy one. The test: *does it change what a user can do?* If it
does, or you cannot say it in one sentence, it is a change: carry on below.

## Context: the preconditions, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/propose/context.sh`
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh ${CLAUDE_PLUGIN_ROOT}/project-config.sh prefixes`

If the line above shows as literal text instead of output, run
`clauductor-model skills/propose/context.sh` yourself and read the result before step 0.

## Steps
0. **Preconditions. Not optional.**
   - **a. At most ONE change proposed ahead.** If the context lists a proposed change, in this tree
     or on a branch, that is not finished and awaiting archive, **stop and ask** before creating
     another. A proposal written before its turn goes stale invisibly, and a stale design is still
     well-formed. To capture scope for later, add a roadmap row instead.
   - **b. Read the specs this change touches** (`SPECS_DIR/<capability>/spec.md`) before drafting,
     and say in the proposal what they already guarantee and which requirements it adds to,
     modifies or contradicts.
   - **c. Re-measure the row.** If it was split off a larger row or has taken on items since, count
     its write surfaces and its MODIFIED requirements. If it now looks like what a split existed
     to prevent (more than about three write surfaces, more than one screen), split the row in the
     roadmap first (*Size a change so one review round converges*).
1. **Name it.** A kebab-case id from the row (`add-member-invites`). Work on `<BRANCH_CHANGE><id>`
   (`clauductor-model project-config.sh branch change <id>` prints it), cut from `origin/main`, in a lane worktree (never the main checkout).
2. **Draft** `changes/<id>/proposal.md`, `design.md`, `tasks.md` and any
   `specs/<capability>/spec.md` deltas, in the shapes `changes/README.md` gives:
   - `proposal.md` opens with `**Status:** awaiting approval`, the `**Roadmap row:**`, a
     `**Risk:** low | normal | high` (your recommendation: it picks the build's models, and the
     owner approves it with the design) and, when the row carries `Budget: $N`, `**Budget:** $N`.
     It ends with `## How we'll know`: the `**Signal:**` you will observe, and `**Check on:**
     YYYY-MM-DD` or `**Check after:** <n> days` (after merge).
   - Every open decision in `design.md` carries your recommendation AND the alternative it beat.
   - `tasks.md` has `## Progress` and `## Decision log` (the builder keeps them), groups each one
     builder's and one reviewer's worth, a test named in each task that needs one, **with the
     scenario IDs it covers**, and ends with the `Slice:` line. A scenario no test can reach gets
     `(manual: <reason>)` or `(untestable: <reason>)` on the line naming its ID.
   - **Scenarios** are `#### Scenario: [CAP-n-Sn] title` with **GIVEN** / **WHEN** / **THEN** /
     **AND** bullets, under a requirement with a SHALL or MUST line. `CAP` is the capability,
     `n` the requirement's number in it, `Sn` the scenario's: the context lists the IDs already
     taken. An ID never changes and is never reused, not even a removed one's.
   - **MODIFIED**: copy the current requirement block from `SPECS_DIR` word for word first, every
     scenario header included, then edit it. A header left out or reworded is deleted on archive.
   - **A new capability's** delta opens with `## Purpose` of at least 50 characters.
   - **No spec delta at all** (a refactor, tooling): write `changes/<id>/.openspec.yaml` with
     `skip_specs: true`.
   `clauductor-model checks/run.sh changes` checks every one of these; only the approval may fail yet.
3. **Put it in front of the owner, and STOP.**
   - `REVIEW_PAGE="none"`: commit, push, and open the PR `proposal: <id>`, whose body lists every
     decision awaiting the owner (recommendation and alternative) and the slice line.
   - `REVIEW_PAGE="artifact"`: build and publish the review page as
     `${CLAUDE_PLUGIN_ROOT}/modules/review-page/README.md` describes, and put its link on `proposal.md`'s first
     line.
   Then `PushNotification` the owner one line (the PR or page link, and "awaiting your approval"),
   and **stop**. Do not start building.
4. **The owner adjusts; you revise in place** (the same PR, or the same page URL), and record every
   decision in `design.md` **as they decided it**, not as recommended.
5. **On approval**: `clauductor-model change-approval.sh <id> --record "<owner>"` (it writes
   `**Approved:** <date> by <owner> · design <hash>`: the approval covers `design.md` and the Risk
   line exactly as they stand), run `clauductor-model checks/run.sh changes`, then land the proposal alone through `merge-pr` (a docs-only
   diff takes the `clauductor:reviewer-docs` lane). Its roadmap row stays `⬜ queued` with a note
   `proposed (#N)`: it goes in flight when the build PR opens. The build then starts from `main`
   (`/build-change {"change": "<id>"}`, or the panel's build template).

## Rules
- No `changes/<id>/` on `main` without an `**Approved:**` line: `checks/changes.sh` fails on
  `awaiting approval`, so the gate cannot write a receipt and `pr-merge-guard` refuses the merge.
  Expect the proposal PR's gate to be red until the owner approves; that red is the control.
- **An edit to `design.md` or the Risk line after approval voids it** (the hash no longer
  matches, and the check fails). Put it back in front of the owner (`--revoke`, then `--record` on
  their word). Never re-record an approval the owner did not give.
- A design decision is the owner's even when the answer seems obvious: recommend, never decide.
- Prefer a smaller change that converges over a complete one that does not.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/propose/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments propose`
