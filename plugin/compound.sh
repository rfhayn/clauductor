#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# compound.sh: the insights still waiting for a decision, and how long they have waited (the
# "compound" step, borrowed from Compound Engineering: every lesson is fed back into the rules
# deliberately, or deliberately not).
#
#   sh .claude/compound.sh            each Raw row of the insights log, oldest first, with its age
#   sh .claude/compound.sh --check    exit 1 when a Raw row has waited COMPOUND_MAX_SESSIONS or more
#
# A row's AGE is the number of journal sessions dated after it (`## Session N — YYYY-MM-DD — …` in
# JOURNAL): the sessions that closed without deciding it. `session-close` runs the compound step
# over these rows (its step 3b); each leaves Raw as one of: `Promoted → ADR-NNNN` (or a rule or a
# check that now executes it), `Instance of ADR-NNNN (…)`, or `Not promoted — <why>`
# (docs/insights-log.md, the status vocabulary). A row logged since the last close is marked NEW.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
. "$CLAUDUCTOR_FW/lib/conf.sh"

check=""; [ "${1:-}" = --check ] && check=1
max=${COMPOUND_MAX_SESSIONS:-3}
[ -f "$ROOT/$INSIGHTS" ] || { echo "CANNOT CHECK — $INSIGHTS is missing, so the Raw insights are UNKNOWN"; exit 2; }

# Session dates, newest first as the journal keeps them; blank when there is no journal yet.
sessions=$(sed -n 's/^## Session [0-9][0-9]* — \([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\) — .*/\1/p' "$ROOT/$JOURNAL" 2>/dev/null | tr '\n' ' ')
last=$(printf '%s\n' $sessions | sort -r | head -1)

# Raw rows of the Log table: | Date | Area | Topic | Observation | How to verify | Status |, the
# status stripped of emphasis before matching (as the vocabulary says).
awk -F'|' -v sessions="$sessions" -v last="$last" -v max="$max" '
  BEGIN { ns = split(sessions, S, " ") }
  function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
  /^## Log/ { inlog = 1; next }
  /^## / { inlog = 0 }
  inlog && /^\|/ {
    if (NF < 8) next
    date = trim($2); st = trim($(NF - 1)); gsub(/\*/, "", st)
    if (date !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) next
    if (st !~ /^Raw([^A-Za-z]|$)/) next
    age = 0; for (i = 1; i <= ns; i++) if (S[i] != "" && S[i] > date) age++
    topic = trim($4); obs = trim($5); if (length(obs) > 70) obs = substr(obs, 1, 70) "…"
    printf "%s\t%d\t%s\t%s\t%s\n", date, age, (last == "" || date >= last) ? "NEW" : "", topic, obs
  }' "$ROOT/$INSIGHTS" | sort > "${TMPDIR:-/tmp}/compound.$$"

n=$(grep -c . "${TMPDIR:-/tmp}/compound.$$" | tr -d ' ')
over=0
if [ "$n" -eq 0 ]; then
  echo "compound: no Raw insight waits for a decision"
else
  while IFS="$(printf '\t')" read -r date age new topic obs; do
    flag=""; [ "$age" -ge "$max" ] && { flag=" OVERDUE"; over=$((over + 1)); }
    printf '  %s  %s session(s)%s%s  [%s] %s\n' "$date" "$age" "${new:+ NEW}" "$flag" "$topic" "$obs"
  done < "${TMPDIR:-/tmp}/compound.$$"
  echo "compound: $n Raw insight(s) wait for a decision (promote, mark an instance, or 'Not promoted — <why>'); $over waited $max session(s) or more"
fi
rm -f "${TMPDIR:-/tmp}/compound.$$"
[ -n "$check" ] && [ "$over" -gt 0 ] && exit 1
exit 0
