# Tasks: panel-29-lane-dialog

## Progress
- 2026-10-02 proposed; nothing built yet
- 2026-10-02 the proposal's review: D3 narrowed and D6 added (back to the owner); tasks and spec tightened; NEWLANE-4-S3 retired (dependency warnings left out)

## Decision log
- (none yet)

## 1. The server starts a template where its branch already is
- [ ] 1.1 `StartLane` accepts a template with mode "existing" when the worktree's branch is the template's rendered branch, and refuses another branch's worktree naming both (D1). "root" stays refused with a plain message, and "branch" without a template is refused. Tested in `framework/internal/panel/lanes/template_modes_test.go` citing [NEWLANE-1-S1] and [NEWLANE-1-S3]
- [ ] 1.2 Mode "branch": add it to the registry's lane modes (`registry.go` `laneModes`), with a test that reloads the registry from disk and finds a "branch" lane valid, not Corrupt. It runs the same fetch, `.worktreeinclude` and `worktree_setup` as "new", then `git worktree add <path> <branch>` for a local branch, or `--track -b <branch> <path> origin/<branch>` for an origin-only one. It refuses a branch that exists nowhere, and the folder rule is unchanged. Tested in the same file citing [NEWLANE-1-S2] and [NEWLANE-1-S4]
- [ ] 1.3 "new" checks the branch (local, and origin after the fetch) before `git worktree add -b`, and refuses with 409 `branch-exists`: the plain sentence plus `{branch, worktree, remote}`. Carry those facts through `LaneError` and `writeLaneErr` (web/routes.go) into the HTTP reply. Tested on a POST /api/lanes in `framework/internal/panel/web` citing [NEWLANE-2-S1]
- [ ] 1.4 `GET /api/lanes/branch?name=…` (D6): read-only, it answers `{branch, local, remote, worktree}` for a template's rendered branch, with no fetch and no polling. Tested in `framework/internal/panel/web` for each of the three cases
- [ ] 1.5 The first prompt and the registry record are the same in "new", "existing" and "branch", asserted in `template_modes_test.go`

## 2. The page's start plan, as a tested pure function
- [ ] 2.1 Expose each open change's approval (approved, not approved, or no proposal) in the page state, keyed by change id, from `signals.ReadChanges` / `m.changes`, which the metrics already read; no new polling. Tested in `framework/internal/panel/state` with fixture changes for all three
- [ ] 2.2 `framework/internal/panel/web/static/start-plan.js` exports `planStart(state, intent)` for the intents next, worktree, template, branch and conflict, returning `{template, name, mode, worktree, choices, warnings}`. A build template is one whose first prompt runs `/build-change`. Tested in node by `framework/internal/panel/web/start_plan_test.go` (the `term-links` pattern: skipped under `-short` or without node, so the full gate runs it) citing [NEWLANE-2-S2], [NEWLANE-2-S3], [NEWLANE-2-S4], [NEWLANE-3-S1], [NEWLANE-3-S2], [NEWLANE-3-S3], [NEWLANE-4-S1], [NEWLANE-4-S2] and [NEWLANE-4-S4]

## 3. The dialog uses the plan
- [ ] 3.1 panel.js calls `planStart` from `openStart` (including "New lane here"), `pickNext`, the template and name handlers (which also call `GET /api/lanes/branch`), and a 409 `branch-exists` reply. "Where it runs" stays enabled with a template, with "Its existing branch, in a new worktree" added (D2). The submit sends `mode` and `worktree` with a template, and `index.html` loads `/static/start-plan.js` before panel.js. Wiring tested in `framework/internal/panel/web/startdialog_test.go` (as `rowactions_test.go` does)
- [ ] 3.2 Warnings render above Start, and the button reads "Start anyway" while any is shown and "Start lane" otherwise; the empty option reads "No template: a plain Claude session". Tested in `startdialog_test.go` citing [NEWLANE-4-S1], [NEWLANE-4-S2] and [NEWLANE-5-S1]
- [ ] 3.3 An end-to-end pass in a throwaway panel against a scratch repo with a `change/x` branch (with a worktree, without one, and origin-only), starting the build template from "New lane here" and from Up next, checked by hand (manual: the dialog's rendering and a real tmux lane need a browser and a live panel; every scenario is also cited by a unit test above)

## 4. Docs
- [ ] 4.1 `docs/panel.md`'s New lane section and the in-page Help: what each "Where it runs" choice does with a template, the existing-branch choice, the approval warning, and the plain-session option

- [ ] Slice: an owner can start a build lane on an in-flight change's existing worktree from "New lane here" in one click, in the panel's New lane dialog
