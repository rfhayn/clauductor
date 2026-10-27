#!/bin/sh
# The roadmap's history rule (OPS-147): no row of the roadmap reuses an id that
# docs/roadmap-history.md holds. The reset of 2026-10-27 moved every merged or cancelled row there,
# so "the next free id" read off the roadmap alone would hand out a taken one, and a commit titled
# with it would name two rows. roadmap_queue (lib/conf.sh) runs this on every read of the queue,
# stdin = the --tsv rows; a non-zero exit makes the queue UNKNOWN for every reader, naming each
# reused id. The history's ids are its queue tables' first cells; its session plan, whose first
# cells are sessions (S4), is skipped. No history file, no rule.
ROOT=${ROOT:-$(cd "$(dirname "$0")/../../.." 2>/dev/null && pwd)}
h="$ROOT/docs/roadmap-history.md"
[ -f "$h" ] || exit 0
awk -F'\t' '
  FILENAME == ARGV[1] {
    if ($0 ~ /^## /) plan = ($0 ~ /^## Session plan/)
    if (plan || $0 !~ /^\| /) next
    split($0, c, "|"); id = c[2]; gsub(/^[ \t]+|[ \t]+$/, "", id)
    if (id != "" && id != "#" && id !~ /^-+$/) taken[id] = 1
    next
  }
  NF >= 4 && ($4 in taken) { printf "line %s: %s is an id docs/roadmap-history.md already holds; take the next id neither file has\n", $1, $4; bad = 1 }
  END { exit bad }
' "$h" -
