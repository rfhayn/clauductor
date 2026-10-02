# Tasks: panel-30-remove-worktree-guidance

## Progress
- 2026-10-02 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The plan says what goes and what stays, in plain words
- [ ] 1.1 `RemoveWorktreePlan` (`framework/internal/panel/lanes/removewt.go`) writes design.md's Removes, Keeps and Notes lines: the folder relative to the project root, the kept branch "and every commit on it" with `branchVerdict`'s reason, a merged branch with where its commits are, "Everything on GitHub", and the detached note naming the ref that holds the commit; the safety line travels as its own plan field. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-1-S1] and [REMOVEWT-1-S2]; update the existing assertions there that match today's wording
- [ ] 1.2 Ignored files under Removes: up to three entries of `git status --porcelain --ignored` in the worktree (folders collapsed), then "and N more"; none, no line. Tested in the same file citing [REMOVEWT-1-S3]
- [ ] 1.3 `RemoveWorktree`'s Removed and Kept lines use the same words, tested in the same file citing [REMOVEWT-1-S4]

## 2. The plan names a change in flight
- [ ] 2.1 The plan gains an in-flight part (the change id, its changes directory, the open pull request number, and the warning's sentences from design.md), only when the worktree can go. The change comes from `signals.ReadChanges` over this worktree and the main checkout with `signals.ChangesDirs`, matched by `signals.BranchOfChange`; the pull request from one `gh pr list --head <branch> --state open --json number`. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-2-S1], [REMOVEWT-2-S2] and [REMOVEWT-2-S3]. The test runner's `gh` fake (`closeRepo.run` in `close_test.go`) answers every `gh` call alike today, so it learns to answer by argv (open versus merged)
- [ ] 2.2 A failed `gh` read is a note, never a guess and never a refusal; the change check still runs. Tested in the same file citing [REMOVEWT-2-S4]
- [ ] 2.3 `RemoveWorktree` doesn't re-check it (D1): a test in the same file removes a flagged worktree with the plan's consent

## 3. A commit on no branch is never removed
- [ ] 3.1 `worktreeVerdict` (`framework/internal/panel/lanes/close.go`) keeps a detached worktree whose HEAD no branch, remote-tracking branch or tag contains (`git for-each-ref --contains <head>`), with design.md's reason; a failed read keeps it too. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-3-S1] and [REMOVEWT-3-S2], and in `framework/internal/panel/lanes/close_test.go` citing [REMOVEWT-3-S3]
- [ ] 3.2 The existing detached test (`TestRemoveWorktreeRemovesACleanDetachedOne`) still removes a detached worktree at `main`, with the new note

## 4. The page and the docs
- [ ] 4.1 `removeWords` in `framework/internal/panel/web/static/panel.js` shows the in-flight warning above Removes, the safety line under the lists, and "Remove anyway" or "Remove worktree"; `doRemove` carries the plan's how-to-get-it-back sentence into the result. Wiring tested in `framework/internal/panel/web/worktrees_test.go` (as `close_test.go` tests Close lane) citing [REMOVEWT-2-S5]
- [ ] 4.2 [REMOVEWT-2-S5] and the plain wording checked in a throwaway panel against a scratch repo with an open change on `change/x`, removed and then started again from New lane (manual: the confirmation's rendering and the New lane round trip need a browser and a live panel; the plan's words and the wiring are unit-tested above)
- [ ] 4.3 `docs/panel.md`: "Remove a worktree" says what goes, what stays, the in-flight warning, the ignored files and the detached-commit rule; Close lane's worktree rules gain the detached-commit rule; the in-page Help (`framework/internal/panel/web/index.html`) says Remove keeps the branch and every commit

- [ ] Slice: an owner can see, before removing a worktree in the panel's Worktrees tree, that a change is in flight on it and how to get the worktree back
