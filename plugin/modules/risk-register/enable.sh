#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# enable.sh: switch the risk-register module on. Creates the register from this module's stub when
# RISK_REGISTER (project.conf; docs/risk-register.md by default) does not exist; never touches one
# that does.
#
#   sh .claude/modules/risk-register/enable.sh          make the register if missing
#   sh .claude/modules/risk-register/enable.sh --check  only report whether it exists
#
# Then add risk-register to MODULES in .claude/project.conf.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
reg=${RISK_REGISTER:-docs/risk-register.md}
if [ -f "$ROOT/$reg" ]; then
  echo "ok   $reg exists ($(grep -c '^| R[0-9][0-9]* |' "$ROOT/$reg") risk rows)"
elif [ "${1:-}" = --check ]; then
  echo "FAIL $reg does not exist (run this without --check to make it from the stub)"; exit 1
else
  mkdir -p "$(dirname "$ROOT/$reg")" && cp "$CLAUDUCTOR_FW/modules/risk-register/risk-register.md" "$ROOT/$reg" && echo "made $reg from the stub (add your risks)"
fi
[ "${1:-}" = --check ] || echo "next: add risk-register to MODULES in .claude/project.conf"
