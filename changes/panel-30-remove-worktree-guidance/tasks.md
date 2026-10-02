# Tasks: panel-30-remove-worktree-guidance

## Progress
- 2026-10-02 proposed; nothing built yet
- 2026-10-02 revised after review (merged changes, the way back, the page's pure function, the PR source, the fail-closed ref read)
- 2026-10-02 revised after the second review (the PR source wired in live.go, group 4's test needs PANEL-29, #77 owns D6's split)

## Decision log
- (none yet)

## 1. A commit on no branch is never removed
If D6 is chosen, issue #77's fix lane (`fix/77-detached-commit`) builds the rule, its two Keeps sentences, its tests and its docs first. Then 1.1 and 1.2 only add the REMOVEWT-3 IDs to #77's tests, and add a test for any of these scenarios they don't reach.
- [ ] 1.1 `worktreeVerdict` (`framework/internal/panel/lanes/close.go`) keeps a detached worktree whose HEAD no ref under `refs/heads`, `refs/remotes` or `refs/tags` contains (`git for-each-ref --contains <head> refs/heads refs/remotes refs/tags`), with design.md's reason; it returns the ref to name (a local branch first, then a remote-tracking branch, then a tag, each the first by name). Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-3-S1] and [REMOVEWT-3-S2], and in `framework/internal/panel/lanes/close_test.go` citing [REMOVEWT-3-S3]
- [ ] 1.2 A failed ref read keeps the worktree with design.md's "the refs can't be read" sentence, asserted in `removewt_test.go` with a runner that fails `for-each-ref --contains`, citing [REMOVEWT-3-S4]

## 2. The plan says what goes and what stays, in plain words
- [ ] 2.1 `RemoveWorktreePlan` (`framework/internal/panel/lanes/removewt.go`) writes design.md's Removes, Keeps and Notes lines, the folder relative to the project root or else in full, the detached note naming the ref from 1.1, and the safety line as its own plan field. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-1-S1], [REMOVEWT-1-S2], [REMOVEWT-1-S5] and [REMOVEWT-3-S2]; update 1.1's REMOVEWT-3-S2 test so that one test asserts both THENs (the worktree is removed, and the note names the ref); update the existing assertions there that match today's wording
- [ ] 2.2 Ignored files under Removes: `git status --porcelain --ignored=matching` in the worktree, with its own timeout (a timeout or failure is design.md's note, and the line is left out), leaving out a file whose bytes equal the project root's file at the same path; up to three, then "and N more"; none, no line. Tested in the same file citing [REMOVEWT-1-S3]
- [ ] 2.3 `RemoveWorktree`'s Removed and Kept lines use the same words, tested in the same file citing [REMOVEWT-1-S4]

## 3. The plan names unfinished work, with the command as the way back
- [ ] 3.1 The lane manager gains a function that returns the polled open pull requests, whether the last poll succeeded, and when it ran; the model gains the accessor it reads. It is set in `framework/internal/panel/live.go`'s `buildRuntime`, beside `lm.Trusted`: `newLaneManager` (`run.go`) runs before the model and the hub exist, so it can't be set there. No `gh` call is added to the plan. Tested in `framework/internal/panel/state/model_test.go` for the accessor, and in `framework/internal/panel/live_wiring_test.go` (new) that a built runtime's lane manager returns what `ApplyPRs` recorded on its hub, so a missed wiring fails a test instead of noting "Couldn't check" on every plan
- [ ] 3.2 The plan gains its unfinished-work part (the change id and its changes directory, the pull request number, the warning's sentences, the way back) only when the worktree can go and `branchVerdict` doesn't judge the branch merged. The change comes from `signals.ReadChanges` over this worktree and the main checkout with `signals.ChangesDirs`, matched by `signals.BranchOfChange`; the way back is the `git worktree add` command. Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-2-S2], [REMOVEWT-2-S3] and [REMOVEWT-2-S6] (squash-merged, through the `gh` fake's merged pull request)
- [ ] 3.3 A failed or pending poll is a note, never a guess and never a refusal; the change check still runs; a list older than two intervals adds its age as a note. The interval is `Ticks.PRs`, which `buildRuntime` hands the lane manager with the function (3.1), and the age is measured on the manager's `Clock`. Tested in the same file with a fake clock, citing [REMOVEWT-2-S4]
- [ ] 3.4 `RemoveWorktree` doesn't check it again (D1): a test in the same file removes a flagged worktree with the plan's consent

## 4. The way back names the template (needs PANEL-29 on main)
- [ ] 4.1 The plan renders each template (`config.RenderTemplate`) with the change id, or else the branch's last segment, skips one that needs an issue or fails to render, and keeps those whose branch equals the worktree's; one or more matches give design.md's New lane sentence, none keeps the command, and so does an untrusted panel.json (`m.Trusted`). Tested in `framework/internal/panel/lanes/removewt_test.go` citing [REMOVEWT-2-S1], that two matching templates are both named, and that an untrusted config gives the command
- [ ] 4.2 The way back works, on the server: a test in `removewt_test.go` removes an unfinished change's worktree as planned, then calls `StartLane` with the plan's template and name and PANEL-29's mode "branch", and asserts that `git worktree add <path> <branch>` runs and the lane is on that branch. It can't pass before PANEL-29 is on main, which is what holds this group, and the change's merge, behind it. `closeRepo` (`close_test.go`) can't run it as it is: its `Exec` fails every tmux call and its config has no template. The test needs an `Exec` that accepts tmux and a config with a template whose branch pattern is `change/{name}`; real git stays, for the `git worktree add` assertion. Citing [REMOVEWT-2-S1]

## 5. The page lays out the plan
- [ ] 5.1 `framework/internal/panel/web/static/remove-plan.js` exports a pure function that turns a plan into the confirmation's layout (the warning first, the lists, the safety line, the button's label) and the result's way-back sentence. Tested in node by `framework/internal/panel/web/remove_plan_test.go` (the `term_links_test.go` pattern) citing [REMOVEWT-2-S5]. Like `term_links_test.go` it skips under `-short` or without node, so it counts only where the full gate runs with node installed
- [ ] 5.2 `removeWords` and `doRemove` in `panel.js` render that layout; `index.html` loads the script. Wiring tested in `framework/internal/panel/web/worktrees_test.go`, as `close_test.go` tests Close lane
- [ ] 5.3 `framework/internal/panel/testdata/browser/lane-row-actions.cjs`: its detached-worktree assertion (line ~120, "no branch to delete") matches the new note, and it adds a case with an open change on `change/x` that asserts the warning and "Remove anyway"; run with `testdata/browser/run.sh`

## 6. Docs and the round trip
- [ ] 6.1 `docs/panel.md`: "Remove a worktree" says what goes, what stays, the warning and when it shows, the way back, the ignored files and the detached-commit rule; Close lane's worktree rules gain the detached-commit rule; the in-page Help (`framework/internal/panel/web/index.html`) says Remove keeps the branch and every commit. If D6 is chosen, the detached-commit rule is #77's text: check it matches, and don't rewrite it
- [ ] 6.2 The way back read and followed by a person in a throwaway panel against a scratch repo: the warning names the template and name that PANEL-29's dialog shows, and following it starts the lane with no git error (manual: whether the sentence matches what a person sees in PANEL-29's dialog needs a browser; the server's round trip is tested in 4.2, and the page's layout in 5.1. Once PANEL-29 lands, a browser case in `testdata/browser/` could replace it, run by `testdata/browser/run.sh`, which is also run by hand)

- [ ] Slice: an owner can see, before removing a worktree in the panel's Worktrees tree, that unfinished work is on it and how to get the worktree back
