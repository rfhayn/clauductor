#!/bin/sh
# Context for propose: the preconditions, computed. What is already proposed (in this tree AND on
# unmerged branches: a proposal on a branch is invisible to `ls`, which is exactly where a stale one
# ends up), the queue's next rows, and the specs that exist.
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
cd "$ROOT" || exit 0
ind() { sed 's/^/    /'; }

echo "- Mode: PROPOSALS=$PROPOSALS, REVIEW_PAGE=$REVIEW_PAGE, CHANGES_DIR=$CHANGES_DIR, SPECS_DIR=$SPECS_DIR"
echo "- Proposed in this tree:"
found=""
for d in "$CHANGES_DIR"/*/; do
  [ -d "$d" ] || continue; n=$(basename "$d"); [ "$n" = archive ] && continue
  echo "    $n"; found=1
done
[ -n "$found" ] || echo "    none"
echo "- Proposed on a branch (local or origin) and not on $MAIN_BRANCH:"
git for-each-ref --format='%(refname:short)' "refs/heads/$BRANCH_CHANGE" "refs/remotes/origin/$BRANCH_CHANGE" 2>/dev/null | sed 's#^origin/##' | sort -u | while read -r b; do
  ref=$b; git rev-parse -q --verify "$ref" >/dev/null 2>&1 || ref="origin/$b"
  git ls-tree -r --name-only "$ref" "$CHANGES_DIR" 2>/dev/null | grep -v "^$CHANGES_DIR/archive/" | cut -d/ -f2 | sort -u | while read -r i; do
    git cat-file -e "origin/$MAIN_BRANCH:$CHANGES_DIR/$i" 2>/dev/null && continue
    code=$(git diff --name-only "origin/$MAIN_BRANCH...$ref" -- . ":!$CHANGES_DIR" ':!docs' 2>/dev/null | wc -l | tr -d ' ')
    echo "    $i on $b ($code non-doc file(s) changed)"
  done
done
echo "- Change queue:"
sh .claude/roadmap-queue.sh --text 2>&1 | ind
echo "- Living specs ($SPECS_DIR/):"
ls "$SPECS_DIR" 2>/dev/null | grep -v README.md | ind
