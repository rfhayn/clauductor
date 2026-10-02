#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for session-close: what is still unlanded, unarchived, unrecorded or untriaged. Every
# section that cannot run says so; a check that could not run learned NOTHING. No `set -e`.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cd "$ROOT" || exit 0
ind() { sed 's/^/    /'; }
# The GitHub-reading sections (remote branches, queued-while-open, TBD specs). Tested for, never
# sourced blind: a `.` of a missing file ends the script under dash and hides every later section.
if [ -f "$CLAUDUCTOR_FW/lib/context.sh" ]; then
  # shellcheck disable=SC1091
  . "$CLAUDUCTOR_FW/lib/context.sh"
else
  ctx_open_prs() { gh pr list --state open --limit 100 --json number,title,headRefName,author,updatedAt,files 2>/dev/null; }
  ctx_loose_branches() { echo "CANNOT CHECK — .claude/lib/context.sh is missing"; }
  ctx_queued_open() { echo "CANNOT CHECK — .claude/lib/context.sh is missing"; }
  ctx_tbd_specs() { echo "CANNOT CHECK — .claude/lib/context.sh is missing"; }
fi

echo "- Branch: $(git --no-optional-locks branch --show-current 2>/dev/null)"
echo "- Uncommitted:"
( git --no-optional-locks status --short 2>/dev/null | head -30; true ) | ind
echo "- Ahead/behind: $(git --no-optional-locks status -sb 2>/dev/null | head -1)"
echo "- Commits today:"
git log --oneline --since=midnight 2>/dev/null | head -20 | ind

echo "- Open PRs (merge-pr lands only yours):"
prs=""
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then echo "    CANNOT CHECK — offline"
elif ! command -v gh >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
  echo "    CANNOT CHECK — gh or jq is not installed; this learned NOTHING, do not read it as none"
elif prs=$(ctx_open_prs); then
  printf '%s' "$prs" | jq -r '.[] | "#\(.number) \(.headRefName) by \(.author.login): \(.title)"' | ind
  [ "$prs" = "[]" ] && echo "    none"
else
  prs=""
  echo "    CANNOT CHECK — gh unavailable; this learned NOTHING, do not read it as none"
fi
echo "- Remote branches with no open PR (land, hand off or delete each one of yours):"
ctx_loose_branches "$prs" 2>&1 | ind

echo "- Changes in $CHANGES_DIR/ (a finished one is archived now; at most ONE may stay proposed):"
found=""
for d in "$CHANGES_DIR"/*/; do
  [ -d "$d" ] || continue
  n=$(basename "$d"); [ "$n" = archive ] && continue
  u=$(grep -cE '^[[:space:]]*-[[:space:]]*\[ \]' "$d/tasks.md" 2>/dev/null) || u=0
  echo "    $n: $u unchecked task(s)"; found=1
done
[ -n "$found" ] || echo "    none"
echo "- Proposed on an unmerged branch (invisible to the list above):"
git for-each-ref --format='%(refname:short)' "refs/remotes/origin/$BRANCH_CHANGE" "refs/heads/$BRANCH_CHANGE" 2>/dev/null | sed 's#^origin/##' | sort -u | while read -r b; do
  git rev-parse -q --verify "$b" >/dev/null 2>&1 || b="origin/$b"
  git ls-tree -r --name-only "$b" "$CHANGES_DIR" 2>/dev/null | grep -v "^$CHANGES_DIR/archive/" | cut -d/ -f2 | sort -u | while read -r i; do
    git cat-file -e "origin/$MAIN_BRANCH:$CHANGES_DIR/$i" 2>/dev/null || echo "    $i on $b"
  done
done

echo "- Roadmap queue (step 3 sets each row's status):"
roadmap_queue --text 2>&1 | ind

base="origin/$MAIN_BRANCH"
fetched=""
fetch_main && fetched=1
# Read against origin/<main>'s roadmap, not this branch's: main is what every other reader sees.
echo "- Open PRs whose roadmap row still reads queued on $base (step 3 sets each to \`⬜ in flight (#N)\`):"
if [ -n "$fetched" ] || [ "${CONTEXT_OFFLINE:-}" = 1 ]; then
  ctx_queued_open "$prs" 2>&1 | ind
else
  echo "    CANNOT CHECK — no $base to read the roadmap from"
fi
echo "- Living specs still carrying TBD (archive-change promotes a Purpose; fill each one):"
ctx_tbd_specs 2>&1 | ind
if [ -n "$fetched" ]; then
  top=$(git show "$base:$JOURNAL" 2>/dev/null | sed -n 's/^## Session \([0-9][0-9]*\).*/\1/p' | sort -n | tail -1)
  echo "- Journal: next session number on $base = $(( ${top:-0} + 1 )); author for the heading = $(git config user.name 2>/dev/null | cut -d' ' -f1)"
else
  echo "- Journal: next session number CANNOT CHECK (no $base); do not take it from this branch"
fi
echo "- Latest journal entry here: $(grep -m1 '^## Session [0-9]' "$JOURNAL" 2>/dev/null || echo none)"
echo "- Today: $(date +%F); insight rows dated today: $(count_rows "^\| $(date +%F) \|" "$INSIGHTS")"
echo "- Raw insights outstanding: $(count_rows '\| (\*\*)?Raw\b[^|]*\|[[:space:]]*$' "$INSIGHTS")"
echo "- Topics at the 3+ promotion trigger, Raw rows only:"
grep -E '\| (\*\*)?Raw\b[^|]*\|[[:space:]]*$' "$INSIGHTS" 2>/dev/null | cut -d'|' -f4 | cut -d/ -f1 | tr -d ' ' | sort | uniq -c | awk '$1 >= 3' | ind
echo "## Compound: the insights waiting for a decision (step 3b)"
sh "$CLAUDUCTOR_FW"/compound.sh 2>&1 | ind
echo "- ADRs not yet Accepted:"
( grep -iE '\| (Proposed|Draft)' "$ADR_DIR/README.md" 2>/dev/null || echo "none" ) | ind
echo "- Owner queue:"
sh "$CLAUDUCTOR_FW"/owner-queue.sh 2>&1 | ind
echo "- Lane worktrees:"
sh .claude/health/worktrees.sh 2>&1 | ind
# OPS-9: how the work flowed and what this session cost, for the journal entry and the Done line.
echo "- Flow: $(sh "$CLAUDUCTOR_FW"/metrics.sh --line --window 30d 2>&1)"
echo "- This session's cost (usage-report.sh: by role and model, at list price, this machine only):"
sh "$CLAUDUCTOR_FW"/usage-report.sh 2>&1 | sed -n '1p; /^| /p; /^Total/p; /^UNPRICED/p; /^CANNOT CHECK/p' | ind
# Sections the enabled modules and the local layer add (context.d/session-close/).
sh "$CLAUDUCTOR_FW"/extensions.sh context session-close
