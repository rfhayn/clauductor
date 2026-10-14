# Owner queue: things that need the owner at the computer

`session-start` prints every open item here at the top of its context, every session, until it is
ticked; the panel pins the same list as a card. This is where a session puts work it cannot do
unattended: an interactive login, a production deploy, a decision that needs a screen, anything
irreversible.

**How to use it.** One item per line, `- [ ]` open and `- [x]` done, with the date it was queued,
what it needs first, and the change or PR it belongs to. Tick it (or delete it) in the PR that does
the work. The printer is `.claude/owner-queue.sh`; `.claude/checks/owner-queue.sh` holds it to
printing every open item and nothing else.

## Open
- [ ] 2026-10-14 — OPS-42 (`changes/ops-42-routines/`): approve its design again once it is amended to read the quota through `lib/quota.sh` (OPS-67, #124) instead of the panel's `quota.json` (task 4.3, ROUTINES-7-S6/S7/S8 and 8-S6). The amendment is drafted before OPS-42's build resumes; this is your decision of 2026-10-14.
- [ ] 2026-10-14 — Six lane worktrees from 2026-09-30 hold uncommitted work and were never merged: `.claude/worktrees/agent-*` on `feature/OPS-10-evals`, `feature/P1-records-parsers`, `feature/OPS-9-metrics`, `feature/P1-install-update-diff`, `ops/OPS-14-rehearsal-fixes` and `ux/harness-19-20`. Say keep or discard; removing them cannot be undone, so no session does it without you.

## Done
- [x] 2026-10-13 — Three cloud sessions were started overnight from Claude desktop: the OPS-40 build (told to push `change/ops-40-build`), OPS-39 (`ops/ops-39-roadmap-order`) and OPS-67 (`ops/ops-67-quota-reading`, the only one pushed at close). Find what each pushed (`git ls-remote origin`, `gh pr list`; a cloud session may pick its own branch name); the next session gates, reviews and merges them. Check OPS-40 added no `prac` node (its prompt was corrected mid-run; OPS-69 owns that).
- [x] 2026-10-13 — PANEL-31 group 3, task 3.2 (the real-browser look at 175%): judge the two departures in its Decision log (the changed-state tooltip reuses the checking sentence; the version field is measured but never dropped from a crowded bar), and the " · " exemption in `TestPanelAvoidsTheTells`. Branch `change/panel-31-build`.
- [x] 2026-10-13 — OPS-42 (`change/ops-42-build`, WIP): read the daylight-saving note in its tasks.md Decision log (a slot in the spring-forward gap is read an hour early; a `skip` routine misses that day); it is kept as a named limit unless you say otherwise.
