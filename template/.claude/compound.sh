#!/bin/sh
# compound.sh: the insights still waiting for a decision, and how long they have waited (the
# "compound" step, borrowed from Compound Engineering: every lesson is fed back into the rules
# deliberately, or deliberately not).
#
#   sh .claude/compound.sh            each Raw row of the insights log, oldest first, with its age
#   sh .claude/compound.sh --check    exit 1 when a Raw row has waited COMPOUND_MAX_SESSIONS or more
#   exit 2 either way: CANNOT CHECK (no log, or no table where INSIGHTS_TABLE_HEADING says)
#
# A row's AGE is the number of journal sessions dated after it (`## Session N — YYYY-MM-DD…` in
# JOURNAL, a date range such as 2026-09-28/29 counting from its first day, a heading with or
# without an author): the sessions that closed without deciding it. `session-close` runs the
# compound step over these rows (its step 3b); each leaves Raw as one of: `Promoted → ADR-NNNN`
# (or a rule or a check that now executes it), `Instance of ADR-NNNN (…)`, or `Not promoted —
# <why>` (docs/insights-log.md, the status vocabulary). A row logged since the last close is NEW.
#
# THE TABLE is the `| Date | … | Status |` table under the heading INSIGHTS_TABLE_HEADING
# (default `## Log`), up to the next heading of the same or a higher level. No such heading, or no
# such table under it, is CANNOT CHECK, never "no Raw insight": reading zero rows from a log whose
# table sits somewhere else would report a backlog of Raw rows as healthy.
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
. "$ROOT/.claude/lib/conf.sh"

check=""; [ "${1:-}" = --check ] && check=1
max=${COMPOUND_MAX_SESSIONS:-3}
[ -f "$ROOT/$INSIGHTS" ] || { echo "CANNOT CHECK — $INSIGHTS is missing, so the Raw insights are UNKNOWN"; exit 2; }

# Session dates, newest first as the journal keeps them; blank when there is no journal yet.
sessions=$(sed -n 's/^## Session [0-9][0-9.]* — \([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\).*/\1/p' "$ROOT/$JOURNAL" 2>/dev/null | tr '\n' ' ')
last=$(printf '%s\n' $sessions | sort -r | head -1)

# Raw rows of the table: | Date | Area | Topic | Observation | How to verify | Status |, by
# position, the status stripped of emphasis before matching (as the vocabulary says). The last line
# is the structure: `#S <heading found> <table found> <line and heading of a Date…Status table
# elsewhere, for the hint>`.
out=$(H="$INSIGHTS_TABLE_HEADING" awk -F'|' -v sessions="$sessions" -v last="$last" '
  function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
  function level(s) { match(s, /^#+/); return RLENGTH }
  BEGIN { ns = split(sessions, S, " "); want = trim(ENVIRON["H"]); wl = level(want); cur = "" }
  /^#+ / {
    if (inlog && level($0) <= wl) inlog = 0
    if (trim($0) == want) { found = 1; inlog = 1; table = 0 }
    cur = trim($0); next
  }
  /^\|[ \t]*Date[ \t]*\|/ && /\|[ \t]*Status[ \t]*\|[ \t]*$/ {
    if (inlog) { table = 1; tables = 1 } else if (other == "") other = NR " (under \"" cur "\")"
    next
  }
  inlog && table && /^\|/ {
    if (NF < 8) next
    date = trim($2); st = trim($(NF - 1)); gsub(/\*/, "", st)
    if (date !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) next
    if (st !~ /^Raw([^A-Za-z]|$)/) next
    age = 0; for (i = 1; i <= ns; i++) if (S[i] != "" && S[i] > date) age++
    topic = trim($4); obs = trim($5); if (length(obs) > 70) obs = substr(obs, 1, 70) "…"
    printf "%s\t%d\t%s\t%s\t%s\n", date, age, (last == "" || date >= last) ? "NEW" : "-", topic, obs
  }
  inlog && table && !/^\|/ { table = 0 }
  END { printf "#S\t%d\t%d\t%s\n", found, tables, other }' "$ROOT/$INSIGHTS")
struct=$(printf '%s\n' "$out" | grep '^#S')
found=$(printf '%s' "$struct" | cut -f2); tables=$(printf '%s' "$struct" | cut -f3); other=$(printf '%s' "$struct" | cut -f4)
hint=""; [ -n "$other" ] && hint=" A '| Date | … | Status |' table is at line $other: set INSIGHTS_TABLE_HEADING in .claude/project.conf to that heading."
if [ "$found" != 1 ]; then
  echo "CANNOT CHECK — $INSIGHTS has no '$INSIGHTS_TABLE_HEADING' heading (INSIGHTS_TABLE_HEADING), so the Raw insights are UNKNOWN, not none.$hint"
  exit 2
fi
if [ "$tables" != 1 ]; then
  echo "CANNOT CHECK — $INSIGHTS has no '| Date | … | Status |' table under '$INSIGHTS_TABLE_HEADING', so the Raw insights are UNKNOWN, not none.$hint"
  exit 2
fi
printf '%s\n' "$out" | grep -v '^#S' | grep . | sort > "${TMPDIR:-/tmp}/compound.$$"

n=$(grep -c . "${TMPDIR:-/tmp}/compound.$$" | tr -d ' ')
over=0
if [ "$n" -eq 0 ]; then
  echo "compound: no Raw insight waits for a decision"
else
  # "-", not an empty field, when a row is not NEW: a tab is IFS whitespace, so `read` would
  # collapse an empty field and shift the topic into it.
  while IFS="$(printf '\t')" read -r date age new topic obs; do
    flag=""; [ "$age" -ge "$max" ] && { flag=" OVERDUE"; over=$((over + 1)); }
    [ "$new" = NEW ] && new=" NEW" || new=""
    printf '  %s  %s session(s)%s%s  [%s] %s\n' "$date" "$age" "$new" "$flag" "$topic" "$obs"
  done < "${TMPDIR:-/tmp}/compound.$$"
  echo "compound: $n Raw insight(s) wait for a decision (promote, mark an instance, or 'Not promoted — <why>'); $over waited $max session(s) or more"
fi
rm -f "${TMPDIR:-/tmp}/compound.$$"
[ -n "$check" ] && [ "$over" -gt 0 ] && exit 1
exit 0
