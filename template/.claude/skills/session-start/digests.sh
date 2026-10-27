#!/bin/sh
# The five long sections of the session-start context block, as DIGESTS: the change queue, the
# latest journal entry, the recent insights, and the ADR and practice-record indexes. Called by
# context.sh, from the repo root.
#
# WHY DIGESTS (2026-10-15 transcript audit, ported ST-11). Printed whole, these five made the block
# too large for Claude Code to hold in context; the orchestrator spilled and re-read the file whole.
# Each now prints what session-start's steps use, a count, and where the rest is. Tests:
# sh .claude/checks/run.sh session-start-digests
#
# Overridable for tests: JOURNAL, INSIGHTS, ADR_INDEX, PRAC_INDEX (defaults from project.conf).

ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
_o_j=${JOURNAL-}; _o_i=${INSIGHTS-}; _o_a=${ADR_INDEX-}; _o_p=${PRAC_INDEX-}
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
[ -n "$_o_j" ] && JOURNAL="$_o_j"
[ -n "$_o_i" ] && INSIGHTS="$_o_i"
cd "$ROOT" || exit 1

ADR_INDEX=${_o_a:-${ADR_INDEX:-$ADR_DIR/README.md}}
PRAC_INDEX=${_o_p:-${PRAC_INDEX:-$PRACTICES_DIR/README.md}}

indent() { cut -c1-"$1" | sed 's/^/    /'; }

echo "- Change queue (the roadmap's current phase; top = next up; the full queue: roadmap_queue --text):"
# `2>&1` is pinned by session-start-digests: a malformed row must reach the block, not fail silently.
roadmap_queue --text 2>&1 | awk 'NR <= 8 { print "    " substr($0, 1, 260) } END { if (NR > 8) print "    … " NR - 8 " more rows" }'

echo "- Latest journal (heading, first paragraph, What's next; the whole entry is the first ## Session in $JOURNAL):"
if [ ! -r "$JOURNAL" ]; then
  echo "    ($JOURNAL unreadable)"
else
entry=$(awk '/^## Session [0-9]/{n++} n == 1' "$JOURNAL" 2>/dev/null)
if [ -n "$entry" ]; then
  printf '%s\n' "$entry" | head -1 | indent 300
  if printf '%s\n' "$entry" | grep -q -m 1 '^\*\*What happened'; then
    printf '%s\n' "$entry" | grep -m 1 '^\*\*What happened' | indent 600
  elif printf '%s\n' "$entry" | grep -q -m 1 '^#### What happened'; then
    printf '%s\n' "$entry" | awk '/^#### What happened/{getline; while (/^[[:space:]]*$/) getline; print; exit}' | indent 600
  else
    printf '%s\n' "$entry" | sed -n '2,8p' | grep -v '^[[:space:]]*$' | head -2 | indent 600
  fi
  next=$(printf '%s\n' "$entry" | sed -n "/^\*\*What's next/,\$p")
  if [ -n "$next" ]; then
    printf '%s\n' "$next" | indent 400
  else
    echo "    (this entry has no **What's next** paragraph)"
  fi
else
  echo "    ($JOURNAL has no ## Session entry)"
fi
fi

echo "- Recent insights (date · area · topic · status; the rows themselves: $INSIGHTS):"
if [ -r "$INSIGHTS" ]; then
  status_of='{ s = $(NF-1); gsub(/^ +| +$/, "", s); gsub(/\*\*/, "", s) }'
  raw=$(grep -E '^\| [0-9]{4}-' "$INSIGHTS" | awk -F'|' "$status_of"' s ~ /^Raw/ { c++ } END { print c+0 }')
  echo "    $raw row(s) at Raw in the whole log"
  grep -m 6 -E '^\| [0-9]{4}-' "$INSIGHTS" | awk -F'|' "$status_of"' { for (i = 2; i <= 4; i++) gsub(/^ +| +$/, "", $i); print "    " $2 " · " $3 " · " $4 " · " substr(s, 1, 60) }'
else
  echo "    ($INSIGHTS unreadable)"
fi

index_digest() {
  if [ ! -r "$1" ]; then
    echo "    ($1 unreadable)"
    return
  fi
  sed -n '/^## Index/,$p' "$1" 2>/dev/null | awk -F'|' -v f="$1" '
    /^\| *\[/ {
      n++; s = $(NF-1); gsub(/^ +| +$/, "", s); t = $3; gsub(/^ +| +$/, "", t)
      id = $2; gsub(/^ *\[|\].*$/, "", id); k = s; sub(/ .*/, "", k); c[k]++
      last = id " " substr(t, 1, 90)
      if (k != "Accepted" && k != "Superseded") open = open "\n    NOT ACCEPTED: " id " " substr(t, 1, 90) " — " s
    }
    END {
      if (!n) { print "    (" f " index has no rows)"; exit }
      line = "    " n " records:"; for (k in c) line = line " " c[k] " " k ";"; print line
      print "    newest: " last; if (open) print substr(open, 2); print "    full index: " f
    }'
}
echo "- ADR index (application architecture):"
index_digest "$ADR_INDEX"
echo "- Practice record index (PRAC-NNNN, how we work; one number space with the ADRs):"
index_digest "$PRAC_INDEX"
