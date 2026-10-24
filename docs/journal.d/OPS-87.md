# OPS-87 — settings.sh reads an empty GATE as build-change does

- **PR**: #226
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 2: one low, the row text, left here)
- **Gate**: 0 min (no full run before this commit; the final gate follows it)
- **Cost**: $2.01 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122283 tokens, 0 min

## What happened

With `GATE=""` in `project.conf`, `checks/settings.sh` reported `lacks ''`, but build-change runs
`scripts/ci/gate.sh` then (`c.gate || 'scripts/ci/gate.sh'` over `gate_cmd`'s output). The check
now falls back the same way, only on empty. Three self-tests hold it: an empty GATE passes on the
template's settings (it failed before the fix), an empty GATE without the default exclusion fails,
and an absolute GATE with only the default exclusion still fails. `selftest` now passes an
explicit empty fifth argument through (`${5-$GATE}`).

## Decisions

The fallback is empty-only. Round 1 widened it to absolute, `~` and `../` paths, mirroring Go's
`cleanRel`; review showed build-change runs those as given, so the wider fallback turned a correct
FAIL into a silent pass.

## Surprises

The row's premise was half wrong: `install` and `update` do not fall back on `GATE=""`, they
refuse it (`Conf.Mapping`: "not a path inside the repository"). Only build-change falls back, so the
row's text in `docs/roadmap.md` overstates install. Whether install should accept an empty GATE as
"not moved" is the owner's call; nothing in this row changes it.

## Insights

| 2026-10-24 | Process | review/mirror-authority | A check that mirrors another component's fallback must mirror the component that RUNS the value (build-change's `\|\|` on empty), not a sibling that merely parses it (Go's `cleanRel`, never reached because install refuses first) | `sh .claude/checks/settings.sh \| grep 'GATE absolute'` | Raw |
