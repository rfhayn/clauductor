# Tasks: panel-31-version-check

## Progress
- 2026-10-02 proposed; nothing built yet
- 2026-10-02 revised after review (#75): Verify now split out to PANEL-32 (design.md D3); scenario IDs VERCHECK-4-S1..S6 and VERCHECK-5-S1..S4, drafted for it, are retired and never reused

## Decision log
- (none yet)

## 1. The check's state, and a verification that holds everywhere
- [ ] 1.1 The view's `Verification` carries one explicit state (unread, changed, unmatched, checking, verified, in that order of precedence) plus the count, the orphans and what broke, computed in `state/verify.go`. A failed latest `claude --version` is *unread* even after an earlier read worked: `ApplyClaudeVersion` keeps the version, but the state and `heuristicsApprox` treat the latest read's failure as unknown. The `version:<v>` warning for checking is no longer added (`state/model.go`, the view); the `:broken` and `version:unread` warnings stay. Tested in `framework/internal/panel/state/verify_test.go` citing [VERCHECK-1-S1], [VERCHECK-1-S3], [VERCHECK-2-S1], [VERCHECK-2-S2] and [VERCHECK-2-S3], with S3 run first against today's code to see it show the check instead of the bar
- [ ] 1.2 Three confirmations with no orphan still verify, and `lv.SubagentsApprox` goes false with it; one orphan with three or more confirmations reads as unmatched rather than counting past three. Tested in `verify_test.go` citing [VERCHECK-1-S2] and [VERCHECK-1-S4], each first run against today's code to see it fail where the behaviour changes
- [ ] 1.3 A verified version replaces an older one in the model (a setter that overwrites, used by the machine; `RestoreAutoVerified` stays fill-only for the start-up read of `verified.json`), and `saveReadings` in `framework/internal/panel/machine.go` saves the version verified for the running Claude Code whichever project holds it, then pushes it to every project with the overwriting setter. Tested in `framework/internal/panel/multiproject_integration_test.go` citing [VERCHECK-3-S1], [VERCHECK-3-S2] and [VERCHECK-3-S3] (two projects restored to 2.1.287; the second, then separately the first, verifies 2.1.288; then a restart from the saved file), each run first against today's code to see the other project stay stuck

## 2. The field, the version dialog and Help
- [ ] 2.1 A "Claude Code" status-bar field in `framework/internal/panel/web/index.html` beside `#tm-gate`, drawn in `web/static/panel.js` from `S.verification` with design.md's values, ink and tooltip, shown only for checking, unmatched and changed; `fitFields` treats it as secondary. Wiring tested in a new `framework/internal/panel/web/versioncheck_test.go` (the `rowactions_test.go` pattern) citing [VERCHECK-1-S1] and [VERCHECK-1-S3]
- [ ] 2.2 Clicking the field opens a version dialog (a `.modal` like `#helpdlg`: labelled, Escape closes it, focus returns to the field) with design.md's text for the checking, unmatched and changed states. Tested in `versioncheck_test.go` citing [VERCHECK-1-S4]
- [ ] 2.3 `renderBanners` no longer draws a bar for the checking state; the break and unread bars are drawn as today. Tested in `versioncheck_test.go` citing [VERCHECK-2-S1] and [VERCHECK-2-S3]
- [ ] 2.4 Help's "Claude Code versions" section in `index.html`, word for word from design.md, tested in `framework/internal/panel/web/help_test.go` citing [VERCHECK-6-S1]. PANEL-29 also edits Help (its task 4.1): whichever merges second rebases onto the other's Help, keeping both

## 3. Docs and a look in a real browser
- [ ] 3.1 `docs/panel.md`: rewrite "Version pinning" and "Re-verifying a new version, by itself (PANEL-13)" to describe the field, its states and their precedence, the dialog, and that one project's verification holds for all and is kept
- [ ] 3.2 A throwaway panel on its own port and tmux socket, with a Claude Code version it hasn't verified, viewed in a browser: the field sits on the status bar's line at 100% and 175%, the dialog opens and closes from the keyboard, and no bar shows; the installed panel and its lanes are untouched (manual: layout and focus need a real browser; the states and the wiring are unit-tested above)

- [ ] Slice: an owner can see a new Claude Code version's check progress in the panel's status bar, and read what is approximate meanwhile, without a page-wide alert
