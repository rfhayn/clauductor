# Tasks: panel-30-remove-worktree-guidance

## Progress
- 2026-10-02 proposed; nothing built yet
- 2026-10-02 revised after review (merged changes, the way back, the page's pure function, the PR source, the fail-closed ref read)

## Decision log
- (none yet)

## 1. A commit on no branch is never removed
If D6 splits this out, the fix lane builds 1.1 and 1.2 and this group only adds the REMOVEWT-3 IDs to its tests.
- [ ] 1.1 `worktreeVerdict` (`framework/internal/panel/lanes/close.go`) keeps a detached worktree whose HEAD no ref under `refs/heads`, `refs/remotes` or `refs/tags` contains (`git for-each-ref --contains <head> refs/heads refs/remotes refs/tags`), with design.md's reason; it returns the ref to name (a local branch first, then a remote-tracking branch, then a tag, each the first by name). Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-3-S1] and [REMOVEWT-3-S2], and in `framework/internal/panel/lanes/close_test.go` citing [REMOVEWT-3-S3]
- [ ] 1.2 A failed ref read keeps the worktree, tested in `removewt_test.go` with a runner that fails `for-each-ref --contains`, citing [REMOVEWT-3-S4]

## 2. The plan says what goes and what stays, in plain words
- [ ] 2.1 `RemoveWorktreePlan` (`framework/internal/panel/lanes/removewt.go`) writes design.md's Removes, Keeps and Notes lines, the folder relative to the project root or else in full, the detached note naming the ref from 1.1, and the safety line as its own plan field. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-1-S1], [REMOVEWT-1-S2] and [REMOVEWT-1-S5]; update the existing assertions there that match today's wording
- [ ] 2.2 Ignored files under Removes: `git status --porcelain --ignored=matching` in the worktree, leaving out a file whose bytes equal the project root's file at the same path; up to three, then "and N more"; none, no line. Tested in the same file citing [REMOVEWT-1-S3]
- [ ] 2.3 `RemoveWorktree`'s Removed and Kept lines use the same words, tested in the same file citing [REMOVEWT-1-S4]

## 3. The plan names unfinished work, with the command as the way back
- [ ] 3.1 The lane manager gains a function that returns the polled open pull requests and whether the last poll succeeded, set in `framework/internal/panel/run.go` from the state's `prs` source, as `RemoteControl` is; the model gains the accessor it reads. No `gh` call is added to the plan. Tested in `framework/internal/panel/state/model_test.go` for the accessor
- [ ] 3.2 The plan gains its unfinished-work part (the change id and its changes directory, the pull request number, the warning's sentences, the way back) only when the worktree can go and `branchVerdict` doesn't judge the branch merged. The change comes from `signals.ReadChanges` over this worktree and the main checkout with `signals.ChangesDirs`, matched by `signals.BranchOfChange`; the way back is the `git worktree add` command. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-2-S2], [REMOVEWT-2-S3] and [REMOVEWT-2-S6]
- [ ] 3.3 A failed or pending poll is a note, never a guess and never a refusal; the change check still runs. Tested in the same file citing [REMOVEWT-2-S4]
- [ ] 3.4 `RemoveWorktree` doesn't check it again (D1): a test in the same file removes a flagged worktree with the plan's consent

## 4. The way back names the template (needs PANEL-29 merged)
- [ ] 4.1 The plan renders each template (`config.RenderTemplate`) with the change id, or else the branch's last segment, skips one that needs an issue or fails to render, and keeps those whose branch equals the worktree's; one or more matches give design.md's New lane sentence, none keeps the command. Build this group only once PANEL-29 is on main. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-2-S1], and that two matching templates are both named

## 5. The page lays out the plan
- [ ] 5.1 `framework/internal/panel/web/static/remove-plan.js` exports a pure function that turns a plan into the confirmation's layout (the warning first, the lists, the safety line, the button's label) and the result's way-back sentence. Tested in node by `framework/internal/panel/web/remove_plan_test.go` (the `term_links_test.go` pattern: skipped under `-short` or without node) citing [REMOVEWT-2-S5]
- [ ] 5.2 `removeWords` and `doRemove` in `panel.js` render that layout; `index.html` loads the script. Wiring tested in `framework/internal/panel/web/worktrees_test.go`, as `close_test.go` tests Close lane
- [ ] 5.3 `framework/internal/panel/testdata/browser/lane-row-actions.cjs`: its detached-worktree assertion (line ~120, "no branch to delete") matches the new note, and it adds a case with an open change on `change/x` that asserts the warning and "Remove anyway"; run with `testdata/browser/run.sh`

## 6. Docs and the round trip
- [ ] 6.1 `docs/panel.md`: "Remove a worktree" says what goes, what stays, the warning and when it shows, the way back, the ignored files and the detached-commit rule; Close lane's worktree rules gain the detached-commit rule; the in-page Help (`framework/internal/panel/web/index.html`) says Remove keeps the branch and every commit
- [ ] 6.2 The way back checked end to end in a throwaway panel against a scratch repo: remove an unfinished change's worktree, then start its lane again from New lane as the warning says, with no git error (manual: the round trip crosses PANEL-29's dialog and a real tmux lane; the plan's words and the page's layout are unit-tested above)

- [ ] Slice: an owner can see, before removing a worktree in the panel's Worktrees tree, that unfinished work is on it and how to get the worktree back
