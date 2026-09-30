#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Journal session numbers: the collision rule pr-merge-guard applies at merge
# (.claude/hooks/lib/journal-sessions.sh), in both directions; and the project's journal carries
# no duplicate number and only well-formed session headings.
. "$(dirname "$0")/lib.sh"
. "$CLAUDUCTOR_FW/hooks/lib/journal-sessions.sh"

h() { printf '## Session %s — 2026-01-0%s — a — x\n' "$@"; }
col() {  # col WANT LABEL BASE HEAD MAIN
  got=$(journal_session_collisions "$3" "$4" "$5"); rc=$?
  if [ "$1" = none ]; then
    [ "$rc" -eq 0 ] && ok "allows: $2" || fail "blocked $2 (collides: $got)"
  else
    [ "$rc" -eq 1 ] && [ "$got" = "$1" ] && ok "blocks: $2 ($got)" || fail "$2: want collision $1, got '${got:-none}'"
  fi
}
B="$(h 1 1)"
col none "a new next number" "$B" "$(h 2 2)
$B" "$B"
col 2 "the other person merged 2 after this branch was cut" "$B" "$(h 2 2)
$B" "$(h 2 3)
$B"
col 2 "keep-both: two entries numbered 2" "$B" "$(h 2 2)
$(h 2 3)
$B" "$B"
col none "a retitled old entry" "$B" "## Session 1 — 2026-01-01 — a — retitled" "$B"
col none "this branch's own heading already squash-merged" "$B" "$(h 2 2)
$B" "$(h 2 2)
$B"
col none "a sub-numbered session (2 and 2.1 differ)" "$B" "## Session 2.1 — x
$B" "$(h 2 2)
$B"
col none "a duplicate main already carried" "$(h 1 1)
$(h 1 1)" "$(h 1 1)
$(h 1 1)" "$(h 1 1)
$(h 1 1)"

j="$ROOT/$JOURNAL"
if [ -f "$j" ]; then
  d=$(grep -E '^## Session [0-9]' "$j" | sed -n 's/^## Session \([0-9.]*\).*/\1/p' | sort | uniq -d)
  [ -z "$d" ] && ok "the journal ($JOURNAL) has no duplicate session number" || fail "$JOURNAL has duplicate sessions: $(echo $d)"
  # Numbered headings only: the file's own preamble shows the shape as `## Session N — …`.
  bad=$(grep -nE '^## Session [0-9]' "$j" | grep -vE '^[0-9]+:## Session [0-9]+(\.[0-9]+)* — [0-9]{4}-[0-9]{2}-[0-9]{2}[^ ]* — [^ ].* — .+' | head -3)
  [ -z "$bad" ] && ok "every session heading reads '## Session N — YYYY-MM-DD — author — focus'" || fail "malformed session headings: $bad"
else
  fail "JOURNAL $JOURNAL does not exist"
fi
finish
