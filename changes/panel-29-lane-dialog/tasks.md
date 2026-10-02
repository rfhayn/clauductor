# Tasks: panel-29-lane-dialog

## Progress
- 2026-10-02 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The server starts a template where its branch already is
- [ ] 1.1 `StartLane` accepts a template with mode "existing" when the worktree's branch is the template's rendered branch, and refuses another branch's worktree naming both (D1); "root" stays refused with a plain message, tested in `framework/internal/panel/lanes/template_modes_test.go` citing [NEWLANE-1-S1] and [NEWLANE-1-S3]
- [ ] 1.2 Mode "branch": `git worktree add <path> <branch>` for a local branch, and `--track -b <branch> <path> origin/<branch>` for an origin-only one; it refuses a branch that exists nowhere, and the folder rule is unchanged, tested in the same file citing [NEWLANE-1-S2]
- [ ] 1.3 "new" checks the branch (local and origin) before `git worktree add -b`, and answers 409 `branch-exists` with the plain sentence and `{branch, worktree, remote}`, tested in the same file citing [NEWLANE-2-S1]
- [ ] 1.4 The first prompt and the registry record are the same in all three modes, asserted in the same file for "existing" and "branch"

## 2. The page's start plan, as a tested pure function
- [ ] 2.1 Expose each open change's approval (approved or not) in the page state, keyed by change id, from the changes the metrics source already reads; no new polling. Tested in `framework/internal/panel/state` with a fixture change, approved and not
- [ ] 2.2 `framework/internal/panel/web/static/start-plan.js` exports `planStart(state, intent)` for the intents next, worktree, template and conflict, returning `{template, name, mode, worktree, choices, warnings}`. Tested in node by `framework/internal/panel/web/start_plan_test.go` (the `term-links` pattern: skipped under `-short` or without node) citing [NEWLANE-2-S2], [NEWLANE-3-S1], [NEWLANE-3-S2], [NEWLANE-3-S3], [NEWLANE-4-S1], [NEWLANE-4-S2] and [NEWLANE-4-S3]

## 3. The dialog uses the plan
- [ ] 3.1 panel.js calls `planStart` from `openStart` (including "New lane here"), `pickNext`, the template and name handlers, and a 409 `branch-exists` reply; "Where it runs" stays enabled with a template, with "Its existing branch, in a new worktree" added (D2); the submit sends `mode` and `worktree` with a template. Wiring tested in `framework/internal/panel/web/startdialog_test.go` (as `rowactions_test.go` does)
- [ ] 3.2 Warnings render above Start, and the button reads "Start anyway" while any is shown; the empty option reads "No template: a plain Claude session", tested in `startdialog_test.go` citing [NEWLANE-5-S1]
- [ ] 3.3 [NEWLANE-1-S1], [NEWLANE-1-S2] and [NEWLANE-2-S2] checked end to end in a throwaway panel against a scratch repo with a `change/x` branch, with and without a worktree (manual: the dialog's rendering and a real tmux lane need a browser and a live panel; the logic and the server are unit-tested above)

## 4. Docs
- [ ] 4.1 `docs/panel.md`'s New lane section and the in-page Help: what each "Where it runs" choice does with a template, the existing-branch choice, the warnings, and the plain-session option

- [ ] Slice: an owner can start a build lane on an in-flight change's existing worktree from "New lane here" in one click, in the panel's New lane dialog
