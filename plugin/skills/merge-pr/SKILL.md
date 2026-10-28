---
name: merge-pr
model: opus
effort: medium
description: "Merge a PR to main the safe way, without waiting for the owner: gate evidence for the head commit, review converged, then squash-merge and prune; notify only if it stops. The paved road that keeps you off pr-merge-guard's blocks. TRIGGER when the user says 'merge this', 'merge the PR', 'land it', 'squash and merge', 'ready to merge', or asks to merge a pull request."
argument-hint: (optional) PR number, defaults to the current branch's PR; reportTo <session> to send it the outcome
---

# Merge a PR: the paved road

Land a PR on `main` without tripping `${CLAUDE_PLUGIN_ROOT}/hooks/pr-merge-guard.sh`. This skill is the happy
path; the hook is the gate that blocks a merge that skips these steps. The hook's header is the
authority on its rules; in short: no `--auto`/`--admin`; gate evidence for the head SHA; a `Slice:`
line on a change PR; no journal session number claimed twice; a build PR only with every task of
its change ticked; every enforced scenario cited by a test at the head; an archive only of a
finished change; the provenance trailers on the squash body; an advisory when more than one change
is proposed or the branch is behind `main`.

## Claude merges without waiting

When the guard's evidence is present **and** review has converged (step 3), merge; do not stop to
ask (AGENTS.md, *Who decides*). That makes this skill and the hook the whole gate, so neither is
ever worked around: no `--admin`, no `--auto`, no merge clicked on the website.

- **Merge only your own PRs**: authored by the `gh api user` login this session runs as. Where
  several people work, another person's PR is theirs to land unless they ask. Bot PRs (dependency
  updates) count as the owner's.
- **Merging is not deploying.** A deploy is the owner's decision (`/release-prep`).
- **When it stops**, notify the owner with `PushNotification` (a deferred tool: ToolSearch
  `select:PushNotification`), one line: PR, step, what you need; then stop. It stops when a guard
  block is not mechanically fixable (a stale receipt or a missing slice line IS mechanical: fix it
  and go on), when review does not converge, or when the gate stays red. If `PushNotification` is
  not in your tool list, say so in the session; do not improvise another channel.
- **When a session asked for the outcome** (`reportTo <session>` in the request, as `apply-change`
  and `build-change` pass it on), every outcome goes to it by `SendMessage` instead: the stop line
  above, or, once merged, a short summary (PR number, squash commit, the review line, anything left
  open and the change or issue that owns it). The session that asked notifies the owner, so a push
  from here as well would arrive twice. If SendMessage is not in your tool list, put the outcome
  in your final hand-back if you are a subagent, else send it with `PushNotification`.

## Context: the state, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/merge-pr/context.sh`
- Branch prefixes (`.claude/project.conf`; a branch is named by its key, never a literal): !`sh ${CLAUDE_PLUGIN_ROOT}/project-config.sh prefixes`

If the line above shows as literal text instead of output, run
`clauductor-model skills/merge-pr/context.sh` yourself and read the result before step 1.

## Steps

### 1. Resolve the PR
Use the argument if given, else the current branch's PR. If there is none, push the branch and
open one (`gh pr create --fill`, or a body that says what a user can now do). Check it is yours,
and that it is OPEN (`gh pr view <n> --json state`; the context shows only the current branch's
PR). A closed PR is another session's "do not land this":
find out why it was closed (`gh pr view <n> --comments`) and stop. Do not reopen it to merge it
unless that reason is gone. `pr-merge-guard` rule 14 refuses a merge of anything not open.

### 2. Gate evidence for the head commit (blocking)
Run the full gate from the branch's checkout, with no flags:

```bash
scripts/ci/gate.sh          # GATE: the full gate, filtered output; the receipt is written by GATE_RUN
```

It takes the machine-wide gate lease (so it may wait behind another lane: the output says so),
runs every step, and on a complete clean run writes `<git-dir>/ci-receipt` naming the SHA. The
guard reads receipts in every worktree's git dir. `--quick` writes no receipt; a run over
uncommitted or untracked files writes one marked `dirty`, which the guard refuses. If you commit
again, the receipt is stale: re-run.

**A prose-only PR** (Markdown outside `.claude/`, `agents/`, `commands/`, the specs, and the files
agents load, AGENTS.md, CLAUDE.md, CLAUDE.local.md and SKILL.md, in any case: a record, a roadmap
row, a close) may run the docs gate instead, where `GATE_DOCS` is on in
`.claude/project.conf`: `scripts/ci/gate.sh --docs`. It runs the
process checks, not the tests, and refuses any other diff before a step runs; its receipt (stage
`docs`) passes the guard only while the PR's own diff is still prose (`${CLAUDE_PLUGIN_ROOT}/lib/docs-gate.sh`).
If it refuses, run the full gate.

If the project names a remote workflow (`GATE_REMOTE_WORKFLOW`), its success on the head SHA counts
too. `gh pr checks` cannot see a dispatched run; poll `gh run list --workflow <file> --commit <sha>`.

- **Gate failed**: STOP. The fix is code, not a merge flag.
- **A reported check is red**: STOP; the guard blocks on anything reported and not passing.
- **A required check has not reported or is not green**: the project names the PR checks a merge
  waits for in `GATE_PR_CHECKS` (`.claude/project.conf`; empty = none). The guard blocks until each
  has reported and passed on the head. Wait with `gh pr checks <n> --watch`. If it says gh itself
  failed, fix gh (`gh auth status`): that is not CI.

### 2b. The slice line (change PRs only)
Find the change from the PR's own diff (`git diff --name-only origin/main...HEAD` under
`CHANGES_DIR`), not from the branch name. If the PR touches a change directory, that change's
`tasks.md` must end with `- [ ] Slice: a <role> can <action> at <where>` or `- [ ] Slice: exempt —
<reason>`. Missing: add it, commit, re-run the gate. Do not write an exempt line just to clear the
gate. A PR that touches no change directory is a continuation branch; the rule does not apply.

### 2c. Verify (change PRs that build a change)
If the PR builds a change (it touches a change directory AND code), run `/clauductor:verify-change <id>`
(`clauductor-model verify-change.sh <id>`): every task ticked, every scenario it adds or modifies cited by
a test or escaped, the approval still covering the design, and the diff touching what `tasks.md`
claims. `build-change` ran it already; run it again if anything was committed since. A FAIL is a
stop until fixed; `pr-merge-guard` rules 9 and 10 block the same things at the merge.

### 3. Review CONVERGED (blocking, and yours to check)
Converged means **the last review round found nothing medium or worse.** By how the PR was built:
- **`/build-change` or `/clauductor:apply-change`**: their report shows each group's last round clean. Commits
  made after the report (a merge of `origin/main`, a doc fix) are unreviewed: review that delta.
- **Built by hand** (a fix or ops branch, a session close): run `/code-review` (or the `clauductor:reviewer` agent)
  on `git diff origin/main...HEAD`, fix what is medium or worse, and re-run until a round comes back
  clean, or the cap. Stop and notify if peak severity rises between rounds.
- **Docs only**: when the context reads `review-lane: docs`, spawn the `clauductor:reviewer-docs` agent
  instead; same convergence rule. Anything `full` gets the full review.
- **A proposal PR** (`proposal: <id>`): `propose` step 2b reviewed it before the owner read it.
  Only a body line starting `Review: converged in` and ending `at <sha> · design <hash>` lets you
  skip review; any other (`Review: NOT converged …`, or none) gets the whole PR reviewed as above.
  With `<head>` the PR's head (`gh pr view <n> --json headRefOid`), review has converged when all
  hold: `git diff --name-only origin/main...<head>` touches only `changes/<id>/` and `ROADMAP`;
  `git diff <sha> <head> -- changes/<id>/ <ROADMAP>` changes only the approval line in `proposal.md`
  and the status cell of the change's roadmap row; and the Approved line names that design hash.
  Then replace the `Review:` line with one naming `<head>`, so the body holds one. Otherwise
  (another line in that diff, a design hash that differs, a `<sha>` git cannot find) review the
  whole PR.

**The cap.** `clauductor-model review-cap.sh <row id or branch> --why` prints how many rounds this PR gets:
its proposal's `**Risk:**`, or an ops or fix row's `Risk:` in its Change cell, against `REVIEW_CAPS`
(low 2, normal 3, high 3 by default), and names `REVIEW_AT_CAP` (`stop` or `known-limits` in
`.claude/project.conf`; the template's default is `stop`). An exit 1 is a stop: fix what it names.
When the cap's round still finds something **medium or worse**, `REVIEW_AT_CAP stop` (the default)
**STOPS** this step: notify the owner (raise the cap, split the work, or send it back to design);
do not merge. With `REVIEW_AT_CAP known-limits`, stop reviewing instead: put each finding left
under `KNOWN LIMITS` in the PR body, with the issue or roadmap row that owns it (create one if none
does: AGENTS.md rule 1), and merge; the owner may raise the cap for this PR instead. **Low** findings
still merge either way: each goes under `KNOWN LIMITS` with its owner. `build-change` reads the same
cap and key.

A reviewer spawn may come back as launched in the background (fork mode, the default, always
does). Do any other work this step has. If there is none,
end your turn with no final report and without SubagentHandback: Claude Code keeps you running
while an agent you spawned is still working, and delivers its findings as your next turn. A hand-back finishes you, and the findings
then go elsewhere (OPS-25). Never poll, sleep or re-spawn. A lane that is itself a background
agent of another session never receives it: the report goes to that main session (seen
2026-10-28, OPS-151), so wait for the main session to relay it.
If the Agent tool offers `run_in_background` (fork mode off: `-p`, the Agent SDK), pass `false`:
there, a launcher that ends its turn has ended.

Write the result into the PR body as one line: `Review: converged in <n> round(s), peak <sev>, at
<short sha>` (low findings only: add them under `KNOWN LIMITS`, each with its owner). With
`known-limits` and medium or worse left at the cap: `Review: capped at <n> round(s), peak <sev>, at
<short sha>; see KNOWN LIMITS`. Do not write that capped line when `stop` blocked the merge.
Nothing checks this line; it is a skill step (AGENTS.md, *What executes*).

### 4. Squash-merge and prune
A row's PR opens as a draft (`session-start/marker.sh`), and GitHub refuses to merge a draft: when
the Context block's PR line says `draft`, run `gh pr ready <n>` first.

Write the squash body to a file: the PR's summary, then ONE trailer block. While
`provenance.enabled` in `.claude/model-roles.json` is true (the template's default), the block has
the provenance trailers, and `pr-merge-guard` rule 12 refuses a merge without them:

```text
Change: <change id, or none for a fix or ops PR>
Agent-Role: <the role that wrote most of it: builder for build-change, thinker by hand, …>
Model: <that role's model, from model-roles.json>
Session: <this session's id: $CLAUDE_CODE_SESSION_ID>
```

and, if `attribution.enabled` is true, `attribution.trailer` as its last line. Then:
```bash
gh pr merge <n> --squash --delete-branch --body-file <file>
```
Run it on its own, from a checkout of this repository: with no repo named, anything before it in
the command (a change of directory, an environment assignment) is refused. From elsewhere, name
the repo with a hosted -R HOST/OWNER/NAME or the PR's URL; a bare -R OWNER/NAME allows only a
change of directory before it. After any command, write the `--body-file` path out in full.
The guard reads the file; a `--body "$(…)"` it cannot read is refused. If it names a different
`Session:` value, use the one it names: it is the session the hook sees.

### 5. Sync
Find the main checkout and ask whether the panel catches it up. The main checkout is the first
`worktree` entry of `git worktree list --porcelain`, whichever checkout you run from: a lane's own
worktree is never the root you ask about.

```bash
main=$(git worktree list --porcelain | sed -n '1s/^worktree //p')
if cu=$(clauductor panel catch-up --project "$main" 2>/dev/null) && [ "$cu" = on ]; then echo on; else echo off; fi
```

Catch-up counts as on only when that command exits 0 and prints `on`. Anything else is off:
`clauductor` not on PATH, an older one with no `panel catch-up`, or an error. Then take the one
branch that applies:

- **Catch-up on:** leave the main checkout to the panel, and do not pull it, even from the main
  checkout itself. The panel trusts a merged `panel.json` on arrival, and rebuilds itself, only
  after a fast-forward it made; a pull from here takes that from it. Confirm the squash commit is
  on the remote's base: `git fetch origin main`, then `git log -1 --oneline origin/main` names it
  (when a later merge sits on top, `git merge-base --is-ancestor <squash> origin/main` exits 0).
- **Catch-up off, run from the main checkout itself** (`git rev-parse --path-format=absolute
  --git-dir` prints what `git rev-parse --path-format=absolute --git-common-dir` prints; from a
  lane they differ): `git pull --ff-only origin main`, then confirm the squash commit landed
  (`git log --oneline -3`). Leave the main checkout on `main`.
- **Catch-up off, from a lane:** confirm the squash commit as with catch-up on, and do not pull:
  a lane's sandbox and worktree isolation refuse a write to the main checkout.

**The owner's step, in your report.** Unless catch-up is on, the report of the merge (your
summary, or the `reportTo` message) ends with what the owner runs in the main checkout, since
nothing else runs it:
- `git pull --ff-only`, unless step 5 pulled;
- after a change to the panel's own source (`framework/`, in the repository the panel is built
  from): `./install.sh`, then `clauductor panel install`;
- after a change to `.clauductor/panel.json`: `clauductor panel trust`.

With catch-up on, the report names none of them: the panel fast-forwards the checkout, trusts or
holds the config, rebuilds itself, and sends a notice for whatever it could not do.

## After the merge
- Leave the roadmap row's Status alone: the daily close's fold sets `✅ merged (#N)` from the
  row's fragment (`${CLAUDE_PLUGIN_ROOT}/skills/session-close/fold.sh`), and no row's PR sets it (OPS-108).
- If the PR completed a change, archive it (`/clauductor:archive-change`).
- Refresh the focus: `clauductor-model status-write.sh "[main] <next focus>"`.
- Remove the lane's worktree once it is merged and clean (`git worktree remove <path>`), or leave it
  to `session-close`'s `machine-quiet.sh`.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/merge-pr/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments merge-pr`
