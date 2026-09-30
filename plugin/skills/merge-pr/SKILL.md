---
name: merge-pr
model: opus
effort: medium
description: "Merge a PR to main the safe way, without waiting for the owner: gate evidence for the head commit, review converged, then squash-merge and prune; notify only if it stops. The paved road that keeps you off pr-merge-guard's blocks. TRIGGER when the user says 'merge this', 'merge the PR', 'land it', 'squash and merge', 'ready to merge', or asks to merge a pull request."
argument-hint: (optional) PR number; defaults to the current branch's PR
---

# Merge a PR: the paved road

Land a PR on `main` without tripping `${CLAUDE_PLUGIN_ROOT}/hooks/pr-merge-guard.sh`. This skill is the happy
path; the hook is the gate that blocks a merge that skips these steps. The hook's header is the
authority on its rules; in short: no `--auto`/`--admin`; gate evidence for the head SHA; a `Slice:`
line on a change PR; no journal session number claimed twice; an advisory when more than one
change is proposed or the branch is behind `main`.

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

## Context: the state, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/merge-pr/context.sh`

If the line above shows as literal text instead of output, run
`clauductor-model skills/merge-pr/context.sh` yourself and read the result before step 1.

## Steps

### 1. Resolve the PR
Use the argument if given, else the current branch's PR. If there is none, push the branch and
open one (`gh pr create --fill`, or a body that says what a user can now do). Check it is yours.

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

If the project names a remote workflow (`GATE_REMOTE_WORKFLOW`), its success on the head SHA counts
too. `gh pr checks` cannot see a dispatched run; poll `gh run list --workflow <file> --commit <sha>`.

- **Gate failed**: STOP. The fix is code, not a merge flag.
- **A reported check is red**: STOP; the guard blocks on anything reported and not passing.

### 2b. The slice line (change PRs only)
Find the change from the PR's own diff (`git diff --name-only origin/main...HEAD` under
`CHANGES_DIR`), not from the branch name. If the PR touches a change directory, that change's
`tasks.md` must end with `- [ ] Slice: a <role> can <action> at <where>` or `- [ ] Slice: exempt —
<reason>`. Missing: add it, commit, re-run the gate. Do not write an exempt line just to clear the
gate. A PR that touches no change directory is a continuation branch; the rule does not apply.

### 3. Review CONVERGED (blocking, and yours to check)
Converged means **the last review round found nothing medium or worse.** By how the PR was built:
- **`/build-change` or `/apply-change`**: their report shows each group's last round clean. Commits
  made after the report (a merge of `origin/main`, a doc fix) are unreviewed: review that delta.
- **Built by hand** (`fix/`, `ops/`, a session close): run `/code-review` (or the `clauductor:reviewer` agent)
  on `git diff origin/main...HEAD`, fix what is medium or worse, and re-run until a round comes back
  clean. Stop and notify if peak severity rises between rounds, or after 3 rounds.
- **Docs only**: when the context reads `review-lane: docs`, spawn the `clauductor:reviewer-docs` agent
  instead; same convergence rule. Anything `full` gets the full review.

Write the result into the PR body as one line: `Review: converged in <n> round(s), peak <sev>, at
<short sha>`. Nothing checks this line; it is a skill step (AGENTS.md, *What executes*).

### 4. Squash-merge and prune
```bash
gh pr merge <n> --squash --delete-branch
```
If `attribution.enabled` in `.claude/model-roles.json` is true and the squash body lacks the
trailer, pass `--body` with the PR summary ending in `attribution.trailer`.

### 5. Sync
From the main checkout: `git -C <main checkout> pull --ff-only origin main`, then confirm the
squash commit landed (`git log --oneline -3`). Leave the main checkout on `main`.

## After the merge
- Set the roadmap row to `✅ merged (#N)` (the parser refuses any other shape) in the next
  change's PR or the session close.
- If the PR completed a change, archive it (`/archive-change`).
- Refresh the focus: `clauductor-model status-write.sh "[main] <next focus>"`.
- Remove the lane's worktree once it is merged and clean (`git worktree remove <path>`), or leave it
  to `session-close`'s `machine-quiet.sh`.
