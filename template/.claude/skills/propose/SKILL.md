---
name: propose
model: opus
effort: xhigh
description: "Propose the next change just in time: check nothing else is proposed ahead, read the specs it touches, draft changes/<id>/{proposal,design,tasks}.md (and spec deltas), review it to convergence, then approve it on the owner's delegation (APPROVAL_DELEGATED) or put it in front of the owner and STOP until they approve. On approval, record it and land the proposal. TRIGGER when the user says 'propose <row>', 'propose the next change', 'write the proposal for', or a panel propose lane starts."
argument-hint: <change-id or roadmap row> [description]
---

# Propose a change

A proposal is written when its roadmap row is NEXT, never earlier, and nothing is built until the
owner approves its design (AGENTS.md, *Who decides*), or Claude does on the owner's delegation
(`APPROVAL_DELEGATED`, step 3a). The format is in `changes/README.md`, and
`.claude/examples/changes/add-greeting-name/` is a complete one that the checks hold correct.

## The fast path: no proposal at all

**If the diff fits in one sentence, it gets no proposal.** A typo, a dependency bump, a one-line
fix, a rename, a doc edit: it goes to a fix or ops lane (`BRANCH_FIX`, `BRANCH_OPS`; the panel's *Fix an issue* or *Ops
task* template), is gated, reviewed and merged with `/merge-pr`, and leaves no change directory.
Anthropic's threshold for when a plan is worth writing, and Kiro's "quick spec": a proposal costs
the owner a review, so it has to buy one. The test: *does it change what a user can do?* If it
does, or you cannot say it in one sentence, it is a change: carry on below.

## Context: the preconditions, computed
!`sh .claude/skills/propose/context.sh`
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh .claude/project-config.sh prefixes`

If the line above shows as literal text instead of output, run
`sh .claude/skills/propose/context.sh` yourself and read the result before step 0.

## Steps
0. **Preconditions. Not optional.**
   - **a. At most ONE change proposed ahead.** If the context lists a proposed change, in this tree
     or on a branch, that is not finished and awaiting archive, **stop and ask** before creating
     another. A proposal written before its turn goes stale invisibly, and a stale design is still
     well-formed. To capture scope for later, add a roadmap row instead. **With a `## Session
     plan`** in the roadmap, an approved proposal, on main or on a branch, is not ahead (it waits
     for its build session); one awaiting the owner is, wherever it is: the rule `panel-suggest.sh`
     applies, and the context marks each one's state the same way.
   - **b. Read the specs this change touches** (`SPECS_DIR/<capability>/spec.md`) before drafting,
     and say in the proposal what they already guarantee and which requirements it adds to,
     modifies or contradicts.
   - **c. Re-measure the row.** If it was split off a larger row or has taken on items since, count
     its write surfaces and its MODIFIED requirements. If it now looks like what a split existed
     to prevent (more than about three write surfaces, more than one screen), split the row in the
     roadmap first (*Size a change so one review round converges*).
1. **Name it.** A kebab-case id from the row (`add-member-invites`). Work on `<BRANCH_CHANGE><id>`
   (`sh .claude/project-config.sh branch change <id>` prints it), cut from `origin/main`, in a lane worktree (never the main checkout).
2. **Draft** `changes/<id>/proposal.md`, `design.md`, `tasks.md` and any
   `specs/<capability>/spec.md` deltas, in the shapes `changes/README.md` gives:
   - `proposal.md` opens with `**Status:** awaiting approval`, the `**Roadmap row:**`, a
     `**Risk:** low | normal | high` (your recommendation: it picks the build's models and review cap,
     and is approved with the design, by the owner or on their delegation: step 3a) and, when the row carries `Budget: $N`, `**Budget:** $N`:
     write `**Budget:** $N, soft` when the owner wants an overrun reported while the build goes on
     (this line, not the design's prose, is what `build-change` reads; unmarked is hard).
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
   `sh .claude/checks/run.sh changes` checks every one of these; only the approval may fail yet.
2b. **Review it to convergence BEFORE the owner reads it.** The approval covers `design.md` as
   written, so a finding after it voids the approval and the owner approves again (PANEL-48 took
   three). Spend the review rounds first:
   - Register the draft as a diff, `git add -N changes/<id>`, and spawn the `reviewer` agent (a
     proposal always carries a spec delta or `.openspec.yaml`, so it is never prose only).
   - Tell it only the change id, and that it reviews a PROPOSAL, not a task group: all of
     `changes/<id>/` as `git diff HEAD` shows it. It judges whether the design does what the
     roadmap row asks and no more; whether each recommendation holds against the living specs and
     the code it names, and beats its alternative; whether every path, script and key it names
     exists; whether each scenario has a task naming a test that can drive it, or an honest
     escape. Say: tests are the build's; an untested scenario is not a finding here.
   - Fix every finding of medium or worse and review again, until a round finds nothing medium or
     worse (the convergence rule of `merge-pr` step 3). If peak severity rises between rounds, or
     after 3 rounds, stop fixing: it still goes to the owner, with the open findings listed.
   - The spawn may come back as launched in the background (fork mode, the default, always does).
     With nothing else to do, end your turn with no final report and without SubagentHandback:
     Claude Code delivers its findings as your next turn. Never poll, sleep or re-spawn. If the
     Agent tool offers `run_in_background` (fork mode off: `-p`, the Agent SDK), pass `false`.
   - Commit the draft. Its short sha is what was reviewed, and `sh .claude/change-approval.sh
     <id>` prints its design hash. The proposal PR's body carries exactly one `Review:` line:
     `Review: converged in <n> round(s), peak <sev>, at <short sha> · design <hash>`, or, when the
     rounds stopped first, `Review: NOT converged after <n> round(s), peak <sev>, open: <short
     list> at <short sha> · design <hash>`. Only the first lets `merge-pr` skip its review. When
     the owner approves, that hash must be the one in the Approved line. Nothing checks this line.
3a. **A delegated approval** (OPS-127). Three stops are judgments the script cannot make, so they
   are this skill step and nothing checks them:
   - step 2b's rounds did not converge: go to step 3;
   - the change, or a decision in it, is money, legal or business, outward beyond this repo (a
     public site, a payment, a signer, a release), irreversible, or needs the owner at the
     computer;
   - its row, or the session plan's *The owner decides* cell for it, keeps the approval for the
     owner in words other than `Approval: owner` (the script reads only that marker).
   For the last two, add `**Approval:** owner — <why>` under the proposal's Risk line, so
   `--delegate` refuses it from then on, and go to step 3.
   Otherwise run `sh .claude/change-approval.sh <id> --delegate "<OWNER_NAME>"`. It records
   `**Approved:** <date> by Claude (delegated by <owner>) · design <hash>` only while
   `APPROVAL_DELEGATED="1"` on `origin/<MAIN_BRANCH>` (the disk, with no remote); the change
   touches no path `.claude/lib/approval-guarded.txt` or `.claude/local/approval-guarded.txt`
   lists, nor `ADR_DIR` or `PRACTICES_DIR`; neither the proposal nor its roadmap row (as
   `origin/<MAIN_BRANCH>` has it) says `Approval: owner`; its `**Risk:**` is not below the row's
   `Risk:` (a row with none is normal); and it was never approved before (a standing Approved
   line, a Status line `--revoke` marked `(revoked; was approved by …)`, or an Approved line in
   `proposal.md`'s history on the branch or `origin/<MAIN_BRANCH>`, whoever approved it).
   Otherwise it refuses and says why: go to step 3. On success: run `sh .claude/checks/run.sh
   changes`, open the PR `proposal: <id>` with the decisions you took (each with the alternative it
   beat), the slice line, the `Review:` line and a line `Approved: by Claude on delegation · design
   <hash>`; then `PushNotification` the owner ONE line naming the change, its decisions and the PR,
   and go on to step 5's landing without waiting.
   **The owner overrules** on their word: before the proposal lands, `--revoke` in its PR and go to
   step 4 (from then on `--delegate` refuses it; only their `--record` approves it). Once it has landed, revise `design.md` as they decide in a new `proposal: <id>` PR, on a
   branch of its own cut from `origin/<MAIN_BRANCH>` (`sh .claude/project-config.sh branch ops
   <id>-revise`; never the build's `<BRANCH_CHANGE><id>`), that carries their `--record` (a revoke alone can never pass the gate), and stop any build lane
   already started from the landed design until that PR merges.
3. **Put it in front of the owner, and STOP** (any case step 3a sends here).
   - `REVIEW_PAGE="none"`: push, and open the PR `proposal: <id>`, whose body lists every
     decision awaiting the owner (recommendation and alternative), the slice line and the
     `Review:` line.
   - `REVIEW_PAGE="artifact"`: build and publish the review page as
     `.claude/modules/review-page/README.md` describes, and put its link on `proposal.md`'s first
     line.
   Then `PushNotification` the owner one line (the PR or page link, and "awaiting your approval"),
   and **stop**. Do not start building.
4. **The owner adjusts; you revise in place** (the same PR, or the same page URL), and record every
   decision in `design.md` **as they decided it**, not as recommended. Review the revision as in
   step 2b (the diff since the reviewed commit is enough) and replace the `Review:` line before
   asking again, so what the owner approves is what was reviewed.
5. **On approval** (step 3a recorded a delegated one already: skip to the landing):
   `sh .claude/change-approval.sh <id> --record "<owner>"` (it writes
   `**Approved:** <date> by <owner> · design <hash>`: the approval covers `design.md` and the Risk
   line exactly as they stand), run `sh .claude/checks/run.sh changes`, then land the proposal alone through `merge-pr`
   (under a review page, the PR's body carries the `Review:` line too). Step 2b reviewed it, so
   merge-pr's step 3 reviews only what changed since. Its roadmap row stays `⬜ queued`, untouched:
   a row's Status is the daily close's (OPS-108), which sets it in flight once the build PR opens.
   A proposal session writes no journal fragment; the build's PR writes the row's. The build then starts from `main`
   (`/build-change {"change": "<id>"}`, or the panel's build template).

## Rules
- No `changes/<id>/` on `main` without an `**Approved:**` line: `checks/changes.sh` fails on
  `awaiting approval`, so the gate cannot write a receipt and `pr-merge-guard` refuses the merge.
  Expect the proposal PR's gate to be red until it is approved (by the owner, or step 3a); that
  red is the control.
- **An edit to `design.md` or the Risk line after approval voids it** (the hash no longer
  matches, and the check fails). `--revoke`, review the edit as in step 2b, and put it in front of
  the owner (step 3): `--delegate` refuses a change approved before, whoever approved it, so only
  their `--record` approves it again. Never re-record an approval the owner did not give.
- A design decision is the owner's even when the answer seems obvious: recommend, never decide,
  unless step 3a's delegation holds for the change. Then decide, and name each decision in the push.
- Prefer a smaller change that converges over a complete one that does not.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/propose/`) add to this
skill. Follow them as part of the steps above:

!`sh .claude/extensions.sh fragments propose`
