#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for dev-journal: the next session number (from origin/<main>, never this branch), the
# author, and what this session changed. Prints CANNOT CHECK rather than a guess.
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cd "$ROOT" || exit 0

echo "- Branch: $(git --no-optional-locks branch --show-current 2>/dev/null)"
echo "- Today: $(date +%Y-%m-%d)"
echo "- Author (first word of git user.name): $(git config user.name | awk '{print $1}')"

base="origin/$MAIN_BRANCH"
if fetch_main; then
  top=$(git show "$base:$JOURNAL" 2>/dev/null | sed -n 's/^## Session \([0-9][0-9]*\).*/\1/p' | sort -n | tail -1)
  if [ -n "$top" ]; then
    echo "- Latest session on $base: $top, so this one is $((top + 1)) (unless this session already wrote its entry)"
  else
    echo "- No '## Session N' heading on $base:$JOURNAL yet, so this one is 1"
  fi
else
  echo "- Next session number: CANNOT CHECK (no fetch of $base); do not take it from this branch"
fi
here=$(sed -n 's/^## Session \([0-9][0-9]*\).*/\1/p' "$JOURNAL" 2>/dev/null | sort -n | tail -1)
echo "- Latest session on this branch: ${here:-none}"
echo "- Recent commits:"
git log --oneline -8 2>/dev/null | sed 's/^/    /'
echo "- Uncommitted:"
git status --short 2>/dev/null | head -20 | sed 's/^/    /'
