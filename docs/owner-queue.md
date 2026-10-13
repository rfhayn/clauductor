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
- [ ] 2026-10-13 — Three cloud sessions ran overnight (OPS-40 build on `change/ops-40-build`, OPS-39 on `ops/ops-39-roadmap-order`, OPS-67 on `ops/ops-67-quota-reading`): read their PRs; the next session gates, reviews and merges them. Check OPS-40 added no `prac` node (its prompt was corrected mid-run; OPS-69 owns that).
- [ ] 2026-10-13 — PANEL-31 group 3, task 3.2 (the real-browser look at 175%): judge the two departures in its Decision log (the changed-state tooltip reuses the checking sentence; the version field is measured but never dropped from a crowded bar), and the " · " exemption in `TestPanelAvoidsTheTells`. Branch `change/panel-31-build`.
- [ ] 2026-10-13 — OPS-42 (`change/ops-42-build`, WIP): read the daylight-saving note in its tasks.md Decision log (a slot in the spring-forward gap is read an hour early; a `skip` routine misses that day); it is kept as a named limit unless you say otherwise.

## Done
