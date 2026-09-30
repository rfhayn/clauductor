---
name: propose
model: opus
effort: xhigh
description: "Propose the next change just in time: check nothing else is proposed ahead, read the specs it touches, draft changes/<id>/{proposal,design,tasks}.md (and spec deltas), put it in front of the owner for approval, and STOP until they approve. On approval, record it and land the proposal. TRIGGER when the user says 'propose <row>', 'propose the next change', 'write the proposal for', or a panel propose lane starts."
argument-hint: <change-id or roadmap row> [description]
---

# Propose a change

A proposal is written when its roadmap row is NEXT, never earlier, and nothing is built until the
owner approves its design (AGENTS.md, *Who decides*). The format is in `changes/README.md`.

## Context: the preconditions, computed
!`sh .claude/skills/propose/context.sh`

If the line above shows as literal text instead of output, run
`sh .claude/skills/propose/context.sh` yourself and read the result before step 0.

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
1. **Name it.** A kebab-case id from the row (`add-member-invites`). Work on `change/<id>`, cut
   from `origin/main`, in a lane worktree (never the main checkout).
2. **Draft** `changes/<id>/proposal.md`, `design.md`, `tasks.md` and any
   `specs/<capability>/spec.md` deltas, in the shapes `changes/README.md` gives. `proposal.md`
   opens with `**Status:** awaiting approval`. Every open decision in `design.md` carries your
   recommendation AND the alternative it beat. `tasks.md` groups are each one builder's and one
   reviewer's worth, with tests named in the tasks, and it ends with the `Slice:` line.
   With `PROPOSALS=openspec`, scaffold and validate through the CLI instead
   (`.claude/modules/openspec/README.md`).
3. **Put it in front of the owner, and STOP.**
   - `REVIEW_PAGE="none"`: commit, push, and open the PR `proposal: <id>`, whose body lists every
     decision awaiting the owner (recommendation and alternative) and the slice line.
   - `REVIEW_PAGE="artifact"`: build and publish the review page as
     `.claude/modules/review-page/README.md` describes, and put its link on `proposal.md`'s first
     line.
   Then `PushNotification` the owner one line (the PR or page link, and "awaiting your approval"),
   and **stop**. Do not start building.
4. **The owner adjusts; you revise in place** (the same PR, or the same page URL), and record every
   decision in `design.md` **as they decided it**, not as recommended.
5. **On approval**: replace the status line with `**Approved:** <YYYY-MM-DD> by <owner>`, run
   `sh .claude/checks/run.sh changes`, then land the proposal alone through `merge-pr` (a docs-only
   diff takes the `reviewer-docs` lane). Its roadmap row stays `⬜ queued` with a note
   `proposed (#N)`: it goes in flight when the build PR opens. The build then starts from `main`
   (`/build-change {"change": "<id>"}`, or the panel's build template).

## Rules
- No `changes/<id>/` on `main` without an `**Approved:**` line: `checks/changes.sh` fails on
  `awaiting approval`, so the gate cannot write a receipt and `pr-merge-guard` refuses the merge.
  Expect the proposal PR's gate to be red until the owner approves; that red is the control.
- A design decision is the owner's even when the answer seems obvious: recommend, never decide.
- Prefer a smaller change that converges over a complete one that does not.
