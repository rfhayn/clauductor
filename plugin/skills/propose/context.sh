#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for propose: the preconditions, computed. What is already proposed (in this tree AND on
# unmerged branches: a proposal on a branch is invisible to `ls`, which is exactly where a stale one
# ends up), the queue's next rows, and the specs that exist.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
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
sh "$CLAUDUCTOR_FW"/roadmap-queue.sh --text 2>&1 | ind
echo "- Living specs ($SPECS_DIR/):"
ls "$SPECS_DIR" 2>/dev/null | grep -v README.md | ind
echo "- Budgets on queued rows (repeat the row's in proposal.md):"
sh "$CLAUDUCTOR_FW"/roadmap-queue.sh --tsv 2>/dev/null | awk -F'\t' '$7 == "queued" && $11 != "" { printf "    %s %s: $%s\n", $4, $5, $11 }' | grep . || echo "    none"
echo "- Scenario IDs already taken (living specs and archived changes; never reuse one):"
. "$CLAUDUCTOR_FW/lib/change.sh"
for s in "$SPECS_DIR"/*/spec.md "$CHANGES_DIR"/archive/*/specs/*/spec.md; do
  [ -f "$s" ] && spec_scenarios "$s" | awk -F'\t' '$3 != "-" { print $3 }'
done | awk '{ id = $0; sub(/-S[0-9]+$/, "", id); cap = id; sub(/-[0-9]+$/, "", cap); req = id; sub(/^.*-/, "", req)
             if (req + 0 > max[cap]) max[cap] = req + 0; n[cap]++ }
             END { for (c in n) printf "    %s: %d ID(s), highest requirement %d (a new requirement is %s-%d)\n", c, n[c], max[c], c, max[c] + 1 }' | sort | grep . || echo "    none"
