#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The compound step (session-close step 3b; .claude/compound.sh): no insight stays Raw for
# COMPOUND_MAX_SESSIONS journal sessions (default 3) without a decision. Falsified in fixtures: a Raw
# row three sessions old fails, a younger one passes, a decided one (promoted, an instance, or
# 'Not promoted — <why>') never counts, emphasis is stripped before matching, a row newer than the
# last session is marked NEW, and the ceiling reads from project.conf. Then this project's log.
. "$(dirname "$0")/lib.sh"

d=$(scratch)
mk() {  # mk NAME ROWS...: a fixture with three journal sessions and the given insight rows
  F="$d/$1"; shift; mkdir -p "$F/.claude/lib" "$F/docs"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$F/.claude/lib/"; cp "$CLAUDUCTOR_FW/compound.sh" "$F/.claude/"
  printf '# Journal\n\n## Session 3 — 2026-03-10 — a — x\n\n## Session 2 — 2026-02-10 — a — x\n\n## Session 1 — 2026-01-10 — a — x\n' > "$F/docs/development-journal.md"
  { printf '# Insights log\n\n## Log\n\n| Date | Area | Topic | Observation | How to verify | Status |\n|------|------|-------|-------------|---------------|--------|\n'
    for r in "$@"; do printf '%s\n' "$r"; done; } > "$F/docs/insights-log.md"
}
run() { (cd "$F" && sh .claude/compound.sh --check) > "$d/out" 2>&1; }
row() { printf '| %s | Ops | %s | an observation | a way to verify | %s |' "$1" "$2" "$3"; }

mk old "$(row 2026-01-01 gates Raw)"
run; expect_rc 1 $? "compound: a Raw insight three sessions old fails"
grep -q 'OVERDUE' "$d/out" && ok "...and is marked OVERDUE" || fail "no OVERDUE mark: $(cat "$d/out")"
mk young "$(row 2026-02-01 gates Raw)"
run; expect_rc 0 $? "compound: a Raw insight two sessions old passes"
mk emph "$(row 2026-01-01 gates '**Raw** — seen twice')"
run; expect_rc 1 $? "compound: an emphasised Raw status is still Raw"
for st in 'Promoted → ADR-0003' 'Instance of ADR-0002 (check 4)' 'Not promoted — one-off, no mechanism would have caught it'; do
  mk decided "$(row 2026-01-01 gates "$st")"
  run; expect_rc 0 $? "compound: a decided insight never counts ($st)"
done
mk new "$(row 2026-03-12 gates Raw)"
run; grep -q ' NEW' "$d/out" && ok "compound: an insight logged since the last session is marked NEW" || fail "no NEW mark: $(cat "$d/out")"
mk ceiling "$(row 2026-02-01 gates Raw)"
printf 'COMPOUND_MAX_SESSIONS="2"\n' > "$F/.claude/project.conf"
run; expect_rc 1 $? "compound: COMPOUND_MAX_SESSIONS in project.conf lowers the ceiling"
mk old-not-new "$(row 2026-01-01 gates Raw)"
run; grep -q ' NEW' "$d/out" && fail "compound: a row older than the last session was marked NEW: $(cat "$d/out")" || ok "compound: a row older than the last session is not marked NEW"

# Sessions a journal writes in other shapes still count: a date range (counted from its first day)
# and a heading from before the author field. Here the row is three sessions old only if both do.
mk ranges "$(row 2026-01-01 gates Raw)"
printf '# Journal\n\n## Session 3 — 2026-03-10/11 — a — x\n\n## Session 2 — 2026-02-10 — a focus from before authors\n\n## Session 1 — 2026-01-10 — a — x\n' > "$F/docs/development-journal.md"
run; expect_rc 1 $? "compound: a date-range session and a session heading with no author both age a row"

# A log shaped like an adopting project's: the table under another heading, no '## Log', 14 Raw rows
# (some emphasised) among statuses of its own. Read by the default heading, it is CANNOT CHECK, never
# healthy; with INSIGHTS_TABLE_HEADING naming its heading, all 14 are read.
mk adopted
{ printf '# Insights log\n\nRaw observations, newest at the top.\n\n## Promotion rules\nRouting follows the ADR README.\n\n'
  printf 'Status values: `Raw` · `Promoted → ADR-NNNN` · `Instance of ADR-NNNN (check N)` · `Fixed → <where>` · `Technique — no mechanism`.\n\n'
  printf '| Date | Area | Topic | Insight | Verify by | Status |\n|------|------|-------|---------|-----------|--------|\n'
  i=1; while [ "$i" -le 14 ]; do
    st=Raw; [ $((i % 3)) -eq 0 ] && st='**Raw** — seen twice'
    printf '| 2026-01-%02d | Infrastructure | area/topic-%d | **An observation.** With detail. | a way to verify | %s |\n' "$i" "$i" "$st"
    i=$((i + 1))
  done
  printf '| 2026-01-20 | Ops | ops/x | y | z | Mechanism built |\n| 2026-01-21 | Ops | ops/y | y | z | Fixed 2026-02-01 |\n'
} > "$F/docs/insights-log.md"
(cd "$F" && sh .claude/compound.sh --check) > "$d/out" 2>&1; rc=$?
expect_rc 2 "$rc" "compound: 14 Raw rows under another heading, no '## Log': CANNOT CHECK, not healthy"
grep -q 'CANNOT CHECK' "$d/out" && grep -q 'line 10 (under "## Promotion rules")' "$d/out" \
  && ok "...and it names where the table is and the setting that reads it" || fail "no CANNOT CHECK naming the table: $(cat "$d/out")"
(cd "$F" && sh .claude/compound.sh) > "$d/out" 2>&1; expect_rc 2 $? "compound: the same without --check is CANNOT CHECK too (session-close reads it)"
printf 'INSIGHTS_TABLE_HEADING="## Promotion rules"\n' > "$F/.claude/project.conf"
run; expect_rc 1 $? "compound: INSIGHTS_TABLE_HEADING names the heading, and the overdue Raw rows fail"
grep -q '14 Raw insight(s)' "$d/out" && ok "...all 14 Raw rows read, emphasised ones included; 'Mechanism built' and 'Fixed <date>' are decided" || fail "wrong count: $(tail -1 "$d/out")"
printf 'INSIGHTS_TABLE_HEADING="## Status vocabulary"\n' > "$F/.claude/project.conf"
run; expect_rc 2 $? "compound: a heading that is not in the log is CANNOT CHECK"
mk notable; printf '# Insights log\n\n## Log\n\nNothing tabulated here.\n\n## Archive\n\n| Date | Area | Topic | Observation | How to verify | Status |\n|---|---|---|---|---|---|\n%s\n' "$(row 2026-01-01 gates Raw)" > "$F/docs/insights-log.md"
run; expect_rc 2 $? "compound: a '## Log' heading with no table under it (the table in another section) is CANNOT CHECK"

grep -qE '^\| `Not promoted — <why>` \||(^|· )`Not promoted — <why>`' "$ROOT/$INSIGHTS" 2>/dev/null \
  && ok "the status vocabulary has 'Not promoted — <why>'" || fail "$INSIGHTS's status vocabulary lacks 'Not promoted — <why>'"
grep -qF 'sh "$CLAUDUCTOR_FW"/compound.sh' "$CLAUDUCTOR_FW/skills/session-close/context.sh" \
  && ok "session-close's context runs the compound step" || fail "session-close's context.sh does not run .claude/compound.sh"

out=$(sh "$CLAUDUCTOR_FW/compound.sh" --check 2>&1); rc=$?
expect_rc 0 "$rc" "this project's insights: $(printf '%s\n' "$out" | tail -1)"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | grep OVERDUE | sed 's/^/     /'
finish
