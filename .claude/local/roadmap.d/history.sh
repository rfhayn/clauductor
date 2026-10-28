#!/bin/sh
# The roadmap's history rule (OPS-147): no row of the roadmap reuses an id that
# docs/roadmap-history.md holds. The reset of 2026-10-27 moved every merged or cancelled row there,
# so "the next free id" read off the roadmap alone would hand out a taken one, and a commit titled
# with it would name two rows. roadmap_queue (lib/conf.sh) runs this on every read of the queue,
# stdin = the --tsv rows; a non-zero exit makes the queue UNKNOWN for every reader, naming each
# reused id. The history's ids are its queue tables' first cells; its session plan, whose first
# cells are sessions (S4), is skipped. No history file, no rule.
#
# A reuse is the history's id on a row with ANOTHER change id (column 5). The same id with the same
# change id is that row itself, read from a roadmap that predates its move (origin/main before the
# reset, an open PR cut before it): refusing it would make every such reader UNKNOWN (OPS-147's
# review, M2).
#
#   sh .claude/local/roadmap.d/history.sh --ids    the history's ids, one per line, for the code
#                                                  that hands out ids (team-queue.sh --next-id)
ROOT=${ROOT:-$(cd "$(dirname "$0")/../../.." 2>/dev/null && pwd)}
h="$ROOT/docs/roadmap-history.md"
[ -f "$h" ] || exit 0
# taken: "<id>\t<change id>" per history row; the change id is the Change cell's first `...`.
taken() {
  awk '
    /^## / { plan = ($0 ~ /^## Session plan/) }
    plan || !/^\| / { next }
    { split($0, c, "|"); id = c[2]; gsub(/^[ \t]+|[ \t]+$/, "", id)
      if (id == "" || id == "#" || id ~ /^-+$/) next
      cid = ""; if (match(c[3], /`[^`]*`/)) cid = substr(c[3], RSTART + 1, RLENGTH - 2)
      print id "\t" cid }
  ' "$h"
}
t=$(mktemp "${TMPDIR:-/tmp}/roadmap-history.XXXXXX") || { echo "could not make a temporary file"; exit 1; }
trap 'rm -f "$t"' EXIT
taken > "$t" || { echo "docs/roadmap-history.md could not be read"; exit 1; }
# A history that cannot be read is an error, never an empty list (OPS-147's review, L6).
if [ "${1:-}" = --ids ]; then cut -f1 "$t"; exit; fi
awk -F'\t' '
  FILENAME == ARGV[1] { cid[$1] = $2; next }
  NF >= 4 && ($4 in cid) && $5 != cid[$4] {
    printf "line %s: %s is an id docs/roadmap-history.md already holds (for %s); take the next id neither file has\n", $1, $4, cid[$4]; bad = 1 }
  END { exit bad }
' "$t" -
