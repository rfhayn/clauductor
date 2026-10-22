#!/bin/sh
# Context for propose: the preconditions, computed. What is already proposed (in this tree AND on
# unmerged branches: a proposal on a branch is invisible to `ls`, which is exactly where a stale one
# ends up), the queue's next rows, and the specs that exist.
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/change.sh"
cd "$ROOT" || exit 0
ind() { sed 's/^/    /'; }

echo "- Mode: PROPOSALS=$PROPOSALS, REVIEW_PAGE=$REVIEW_PAGE, CHANGES_DIR=$CHANGES_DIR, SPECS_DIR=$SPECS_DIR"
echo "- Proposed in this tree:"
found=""
for d in "$CHANGES_DIR"/*/; do
  [ -d "$d" ] || continue; n=$(basename "$d"); [ "$n" = archive ] && continue
  st="awaiting the owner"; head -n 3 "$d/proposal.md" 2>/dev/null | grep -q '^\*\*Approved:\*\*' && st=approved
  echo "    $n ($st)"; found=1
done
[ -n "$found" ] || echo "    none"
# Main's ids, live or archived, its roadmap (the open rows, the queue, the budgets) and its specs
# (living and archived), as panel-suggest.sh reads them: from origin, else this tree (a main
# checkout lags what the lanes merged). tmp is set here, so an inherited one is never the directory
# the trap removes; a signal leaves through the EXIT trap too.
main="origin/$MAIN_BRANCH"
road="$ROADMAP"; base=.; tmp=""
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
if git rev-parse -q --verify "$main^{commit}" >/dev/null 2>&1; then
  skip=$(git ls-tree --name-only "$main" "./$CHANGES_DIR/" "./$CHANGES_DIR/archive/" 2>/dev/null)
  if tmp=$(mktemp -d "${TMPDIR:-/tmp}/propose-main.XXXXXX"); then
    git show "$main:./$ROADMAP" > "$tmp/roadmap.md" 2>/dev/null && road=$tmp/roadmap.md
    # ls-tree and show take paths from here, so a project in a subdirectory reads its own specs.
    git ls-tree -r --name-only "$main" -- "$SPECS_DIR" "$CHANGES_DIR/archive" 2>/dev/null | grep '/spec\.md$' \
      | while IFS= read -r p; do mkdir -p "$tmp/$(dirname "$p")" && git show "$main:./$p" > "$tmp/$p"; done
    base=$tmp
  fi
else
  skip=$(ls "$CHANGES_DIR" "$CHANGES_DIR/archive" 2>/dev/null)
fi
skip=$(printf '%s\n' "$skip" | sed 's|.*/||; s/^[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}-//' | tr '\n' ' ')
open=$(roadmap_queue --tsv "$road" 2>/dev/null | awk -F'\t' '$7 == "queued" || $7 == "inflight" || $7 == "open" { print $5 }' | tr '\n' ' ')
echo "- Proposed on a branch (local or origin) for an open row, not on $MAIN_BRANCH (branch_proposals, panel-suggest.sh's reading):"
props=$(branch_proposals "$skip" "$open")
[ -n "$props" ] || echo "    none"
printf '%s\n' "$props" | while IFS="$(printf '\t')" read -r i st b; do
  [ -n "$i" ] || continue
  [ "$st" = waiting ] && st="awaiting the owner"
  code=$(git diff --name-only "$main...$b" -- . ":!$CHANGES_DIR" ':!docs' 2>/dev/null | wc -l | tr -d ' ')
  echo "    $i on $b ($st; $code non-doc file(s) changed)"
done
echo "- Change queue:"
# An error names the roadmap as main has it, never the temporary copy.
roadmap_queue --text "$road" 2>&1 | if [ "$road" = "$ROADMAP" ]; then cat; else sed "s|$road|$main:$ROADMAP|g"; fi | ind
echo "- Living specs ($SPECS_DIR/):"
ls "$base/$SPECS_DIR" 2>/dev/null | grep -v README.md | ind
echo "- Budgets on queued rows (repeat the row's in proposal.md):"
roadmap_queue --tsv "$road" 2>/dev/null | awk -F'\t' '$7 == "queued" && $11 != "" { printf "    %s %s: $%s\n", $4, $5, $11 }' | grep . || echo "    none"
echo "- Scenario IDs already taken (living specs and archived changes; never reuse one):"
for s in "$base/$SPECS_DIR"/*/spec.md "$base/$CHANGES_DIR"/archive/*/specs/*/spec.md; do
  [ -f "$s" ] && spec_scenarios "$s" | awk -F'\t' '$3 != "-" { print $3 }'
done | awk '{ id = $0; sub(/-S[0-9]+$/, "", id); cap = id; sub(/-[0-9]+$/, "", cap); req = id; sub(/^.*-/, "", req)
             if (req + 0 > max[cap]) max[cap] = req + 0; n[cap]++ }
             END { for (c in n) printf "    %s: %d ID(s), highest requirement %d (a new requirement is %s-%d)\n", c, n[c], max[c], c, max[c] + 1 }' | sort | grep . || echo "    none"
