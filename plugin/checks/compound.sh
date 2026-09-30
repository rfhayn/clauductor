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

grep -q '^| `Not promoted — <why>` |' "$ROOT/$INSIGHTS" 2>/dev/null \
  && ok "the status vocabulary has 'Not promoted — <why>'" || fail "$INSIGHTS's status vocabulary lacks 'Not promoted — <why>'"
grep -qF 'sh "$CLAUDUCTOR_FW"/compound.sh' "$CLAUDUCTOR_FW/skills/session-close/context.sh" \
  && ok "session-close's context runs the compound step" || fail "session-close's context.sh does not run .claude/compound.sh"

out=$(sh "$CLAUDUCTOR_FW/compound.sh" --check 2>&1); rc=$?
expect_rc 0 "$rc" "this project's insights: $(printf '%s\n' "$out" | tail -1)"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | grep OVERDUE | sed 's/^/     /'
finish
