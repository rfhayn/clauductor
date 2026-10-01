#!/bin/sh
# live-risks.sh: session-start's live-risks section (the risk-register module). Every row of
# RISK_REGISTER (project.conf) not marked closed, one line each: `R<n> <title> — gate <Gate> (<Issue>)`.
#
# The grammar is the stub's (.claude/modules/risk-register/risk-register.md): a table row starting
# `| R<n> |`; the Risk cell opens with a **bold title**; the last two cells are Gate and Issue. A
# row is CLOSED when its bold title holds ✅ (a ✅ elsewhere in the row, say in the mitigation of a
# half, leaves it live). A file with no rows in that shape lists nothing and says so: a register this
# cannot read must not look like one with no risks.
#
# One local file, no network, so CONTEXT_OFFLINE changes nothing here.
ROOT=$(cd "$(dirname "$0")/../../../../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
reg=${RISK_REGISTER:-docs/risk-register.md}
f="$ROOT/$reg"
[ -f "$f" ] || { echo "CANNOT CHECK — RISK_REGISTER $reg does not exist (sh .claude/modules/risk-register/enable.sh makes the stub)"; exit 0; }
rows=$(grep -c '^| R[0-9][0-9]* |' "$f")
[ "$rows" -gt 0 ] || { echo "none: $reg has no \`| R<n> |\` rows yet"; exit 0; }
live=$(awk -F'|' '/^\| R[0-9]+ \|/ { id=$2; r=$3; g=$(NF-2); i=$(NF-1); gsub(/^ +| +$/,"",id); gsub(/^ +| +$/,"",g); gsub(/^ +| +$/,"",i); if (match(r, /\*\*[^*]+\*\*/)) r=substr(r, RSTART+2, RLENGTH-4); gsub(/^ +| +$/,"",r); if (r !~ /✅/) print id" "r" — gate "g" ("i")" }' "$f")
if [ -n "$live" ]; then printf '%s\n' "$live"
else echo "none: all $rows rows of $reg are ✅ closed"; fi
