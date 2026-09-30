#!/bin/sh
# Context for log-insight: today, the Area buckets this project uses, and the most recent rows.
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
cd "$ROOT" || exit 0

echo "- Today: $(date +%Y-%m-%d)"
echo "- Branch: $(git --no-optional-locks branch --show-current 2>/dev/null)"
if [ -f "$INSIGHTS" ]; then
  echo "- Area buckets (INSIGHT_AREAS in project.conf): ${INSIGHT_AREAS:-not set; use the areas already in the log}"
  echo "- Areas in use:"
  awk -F'|' '/^\| [0-9]{4}-[0-9]{2}-[0-9]{2} /{gsub(/^ +| +$/, "", $3); print $3}' "$INSIGHTS" | sort | uniq -c | sort -rn | head -12 | sed 's/^/    /'
  echo "- Latest rows:"
  grep -E '^\| [0-9]{4}-[0-9]{2}-[0-9]{2} ' "$INSIGHTS" | head -5 | cut -c1-160 | sed 's/^/    /'
else
  echo "- CANNOT CHECK: $INSIGHTS is missing"
fi
