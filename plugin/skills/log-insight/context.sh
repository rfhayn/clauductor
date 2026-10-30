#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/../.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# Context for log-insight: today, the Area buckets this project uses, and the most recent rows.
ROOT=$(case $STRIDAL_FW in (*/.claude) dirname "$STRIDAL_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$STRIDAL_FW/lib/conf.sh"
cd "$ROOT" || exit 0

echo "- Today: $(date +%Y-%m-%d)"
br=$(git --no-optional-locks branch --show-current 2>/dev/null)
echo "- Branch: $br"
# OPS-108: a row's PR never edits INSIGHTS; its rows go in its fragment, and the close folds them.
if [ -z "$BRANCH_SESSION_CLOSE" ]; then
  echo "- Writer: BRANCH_SESSION_CLOSE is empty (rule 8 off), so no branch is told apart as a close's: write $INSIGHTS"
else
  # shellcheck disable=SC2254
  case $br in
    $BRANCH_SESSION_CLOSE) echo "- Writer: a daily close's branch, so write the row in $INSIGHTS" ;;
    *) echo "- Writer: NOT a close's branch ($BRANCH_SESSION_CLOSE): write the row under ## Insights in the row's fragment, $JOURNAL_FRAGMENTS/<row id>.md, never $INSIGHTS" ;;
  esac
fi
if [ -f "$INSIGHTS" ]; then
  echo "- Area buckets (INSIGHT_AREAS in project.conf): ${INSIGHT_AREAS:-not set; use the areas already in the log}"
  echo "- Areas in use:"
  awk -F'|' '/^\| [0-9]{4}-[0-9]{2}-[0-9]{2} /{gsub(/^ +| +$/, "", $3); print $3}' "$INSIGHTS" | sort | uniq -c | sort -rn | head -12 | sed 's/^/    /'
  echo "- Latest rows:"
  grep -E '^\| [0-9]{4}-[0-9]{2}-[0-9]{2} ' "$INSIGHTS" | head -5 | cut -c1-160 | sed 's/^/    /'
else
  echo "- CANNOT CHECK: $INSIGHTS is missing"
fi
