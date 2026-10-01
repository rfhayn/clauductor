#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for session-start: everything the session needs to orient, COMPUTED. Every section that
# cannot run says CANNOT CHECK (or "learned NOTHING") instead of printing nothing or "none": an
# absent answer must never read as a healthy one. No `set -e`: one failing section must not hide
# the ones after it.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cd "$ROOT" || exit 0
ind() { sed 's/^/    /'; }

branch=$(git --no-optional-locks branch --show-current 2>/dev/null)
if [ "$(git rev-parse --git-dir 2>/dev/null)" = "$(git rev-parse --git-common-dir 2>/dev/null)" ]; then
  where="the MAIN checkout"
  [ "$branch" = "$MAIN_BRANCH" ] || where="$where, NOT on $MAIN_BRANCH: hooks run from here, so put it back on $MAIN_BRANCH before new work"
else
  where="a worktree ($(basename "$ROOT"))"
fi
echo "- Branch: ${branch:-detached} in $where"

# First on purpose: work only the owner can do at the computer.
echo "- $(printf '%s' "$OWNER_ROLE" | tr '[:lower:]' '[:upper:]') QUEUE (needs the $OWNER_ROLE at the computer; repeat each item in the summary):"
sh "$CLAUDUCTOR_FW"/owner-queue.sh 2>&1 | ind

# The local panel: up = its port file holds digits only AND /healthz answers within 0.3 s.
port=$(cat "$HOME/.clauductor/panel/port" 2>/dev/null) || port=
case "$port" in '' | *[!0-9]*) port= ;; esac
if [ "${CONTEXT_OFFLINE:-}" != 1 ] && [ -n "$port" ] && curl -s -o /dev/null --max-time 0.3 "http://127.0.0.1:$port/healthz" 2>/dev/null; then
  echo "- Panel: up (http://127.0.0.1:$port)"
else
  echo "- Panel: not running (optional; nothing in this repo needs it. If installed: clauductor panel)"
fi

echo "- Status:"
git --no-optional-locks status --short 2>/dev/null | head -20 | ind
echo "- Recent commits:"
git log --oneline -8 2>/dev/null | ind

echo "- Open PRs (★ = another person's: read before building on anything they touch; bots unmarked):"
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then
  echo "    CANNOT CHECK — offline"
elif ! command -v gh >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
  echo "    CANNOT CHECK — gh or jq is not installed; this learned NOTHING, do not read it as none"
else
  git --no-optional-locks fetch origin --prune --quiet 2>/dev/null || echo "    (git fetch failed: OVERLAP below reads a possibly stale origin)"
  me=$(gh api user --jq .login 2>/dev/null) || me=
  if prs=$(gh pr list --state open --limit 100 --json number,title,headRefName,author,updatedAt,files 2>/dev/null); then
    [ -n "$me" ] || echo "    (gh api user failed: cannot tell yours from theirs, so nothing is starred and OVERLAP is skipped)"
    printf '%s' "$prs" | jq -r --arg me "$me" '.[] | "\(if $me == "" or .author.login == $me or .author.is_bot or (.author.login | startswith("app/")) then " " else "★" end) #\(.number) \(.author.login) \(.headRefName) (updated \(.updatedAt[:10])): \(.title)"' | ind
    [ "$prs" = "[]" ] && echo "    none"
    mine=$(git --no-optional-locks diff --name-only "origin/$MAIN_BRANCH...HEAD" 2>/dev/null)
    if [ -n "$mine" ] && [ -n "$me" ]; then
      printf '%s' "$prs" | jq -r --arg me "$me" '.[] | select(.author.login != $me) | .number as $n | .files[]? | "\($n) \(.path)"' \
        | while read -r n p; do printf '%s\n' "$mine" | grep -qxF -- "$p" && echo "    OVERLAP: #$n (not yours) also changes $p"; done
    fi
  else
    echo "    CANNOT CHECK — gh pr list failed; this learned NOTHING, do not read it as none"
  fi
fi

echo "- Change queue (the roadmap's current phase; top = next up):"
roadmap_queue --text 2>&1 | ind
echo "- Proposed, not yet built ($CHANGES_DIR/, at most one ahead):"
found=""
for d in "$CHANGES_DIR"/*/; do
  [ -d "$d" ] || continue
  n=$(basename "$d"); [ "$n" = archive ] && continue
  u=$(grep -cE '^[[:space:]]*-[[:space:]]*\[ \]' "$d/tasks.md" 2>/dev/null) || u=0
  echo "    $n: $u unchecked task(s)"; found=1
done
[ -n "$found" ] || echo "    none in this tree"

echo "- Latest journal entry:"
if [ -f "$JOURNAL" ]; then
  entry=$(awk '/^## Session [0-9]/{n++} n==1' "$JOURNAL" | head -25)
  printf '%s\n' "${entry:-none yet}" | ind
else
  echo "    CANNOT CHECK — $JOURNAL is missing"
fi
echo "- Insights: $(count_rows '\| (\*\*)?Raw\b[^|]*\|[[:space:]]*$' "$INSIGHTS") Raw row(s); latest:"
grep -m 4 -E '^\| [0-9]{4}-' "$INSIGHTS" 2>/dev/null | cut -c1-150 | ind
echo "- ADRs not yet Accepted:"
( grep -iE '\| (Proposed|Draft)' "$ADR_DIR/README.md" 2>/dev/null || echo "none" ) | ind

# Pluggable health lines: every script in .claude/health, the directory being the list, then the
# enabled modules' and the local layer's (.claude/extensions.sh), each run per its own #! line.
sh "$CLAUDUCTOR_FW"/extensions.sh health
# Sections the enabled modules and the local layer add (context.d/session-start/).
sh "$CLAUDUCTOR_FW"/extensions.sh context session-start
