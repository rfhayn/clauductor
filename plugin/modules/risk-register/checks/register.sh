#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The risk-register module's own check, run by checks/run.sh only while the module is on: this
# project's register (RISK_REGISTER) exists and every `| R<n> |` row is in the grammar the
# session-start listing reads (the module's README): a unique id, a **bold title** first in the Risk
# cell, and a non-empty Gate and Issue as the last two cells. A row outside it would list wrongly or
# not at all, and a listing that is quietly short reads exactly like a project with fewer risks.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"

reg=${RISK_REGISTER:-docs/risk-register.md}
f="$ROOT/$reg"
[ -f "$f" ] || { fail "RISK_REGISTER $reg does not exist (sh "$CLAUDUCTOR_FW"/modules/risk-register/enable.sh makes it from the stub)"; finish; }
grep -qE '^\|[[:space:]]*#[[:space:]]*\|[[:space:]]*Risk[[:space:]]*\|' "$f" && ok "$reg has the register's header row" \
  || fail "$reg has no '| # | Risk | … | Gate | Issue |' header row"
bad=$(awk -F'|' '/^\| *R[0-9]+ *\|/ {
    id=$2; gsub(/^ +| +$/, "", id)
    if ($0 !~ /^\| R[0-9]+ \|/) print id ": starts other than `| " id " |` (one space each side), so the listing skips it"
    if (id in seen) print id ": the id is used twice (lines " seen[id] " and " NR ")"; seen[id]=NR
    if ($3 !~ /^ *\*\*[^*]+\*\*/) print id ": the Risk cell does not open with a **bold title**"
    g=$(NF-2); i=$(NF-1); gsub(/^ +| +$/, "", g); gsub(/^ +| +$/, "", i)
    if (g == "") print id ": the Gate cell (second to last) is empty"
    if (i == "") print id ": the Issue cell (last) is empty; write — for none"
    if ($0 !~ /\| *$/) print id ": the row does not end with |"
  }' "$f")
n=$(grep -cE '^\| *R[0-9]+ *\|' "$f")
if [ -n "$bad" ]; then printf '%s\n' "$bad" | while IFS= read -r l; do echo "FAIL $reg $l"; done; _fails=$((_fails + $(printf '%s\n' "$bad" | wc -l)))
else ok "all $n risk rows of $reg are in the grammar the session-start listing reads"; fi
finish
