# OPS-88 — a plugin check's child shell gets the plugin root; the window cases check exit and posts

- **PR**: #227
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3 low: its comment finding fixed after it, not re-reviewed; two detector lows left to #232)
- **Gate**: CANNOT CHECK — the only full gate runs after this commit, on the tree that holds this fragment
- **Cost**: $5.19 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122384 tokens, 0 min

## What happened

A plugin script sets `CLAUDUCTOR_FW` in its prologue without exporting it. A `sh -c '…'` child
whose body names the variable therefore read it as empty, so `plugin/checks/statusline.sh` failed
its two unwritable-HOME cases when run directly. Through `checks/run.sh`, which exports the
variable, those cases passed. The plugin builder (`rewrite.go`, `shChildFW`) now puts
`CLAUDUCTOR_FW="$CLAUDUCTOR_FW"` among such a child's assignments. Each child is judged on its own,
and a value the template sets on that command is kept. `TestShellChildGetsPluginRoot` walks the
built plugin with a detector written separately from the rewrite. It fails without the rewrite,
and it found a third instance the row did not name: `checks/health-lines.sh`'s scheduled-list
case. In `checks/statusline.sh`, `window()` now requires exit 0 and exactly one post before it
compares the line, and a status line that exits 3 now fails every window case.

## Decisions

The fix is in the builder, not the template line, and the export stays off. `health-lines.sh`
asserts the prologue does not export the variable, and a check's children point it at fixtures on
purpose. Passing the variable per child keeps both of those true. This repo's copy of the check
was synced by copying the file. `clauductor update` was not used because it also rewrote
`.claude/model-roles.json`, which is out of this row's scope.

## Surprises

Round 1's medium came from the rewrite's first form, which appended its own assignment after a
deliberate `CLAUDUCTOR_FW=` and so overrode it. Round 2's medium was that this repo's own copy of
the check was never synced after the template edit. A gate on such a tree runs every shared check
twice, because `framework_matches_template` fails.

## Insights

| 2026-10-24 | Plugin | plugin/unexported-fw | The plugin prologue sets CLAUDUCTOR_FW without exporting it, so any child shell that names it reads it empty, and only checks/run.sh's export hid that; a check that passes only through its runner has a dependency on the runner it does not state | `go test -run TestShellChildGetsPluginRoot ./internal/plugin/`; run a plugin check with `env -u CLAUDUCTOR_FW sh plugin/checks/<x>.sh` | Raw |
