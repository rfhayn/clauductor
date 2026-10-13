#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for new-adr: the next record number, taken from origin/<main> AND the open PRs (two
# branches cut from one main both see the same "highest + 1", and git raises no conflict because
# the file names differ after the number). ADRs (ADR_DIR) and practice records (PRACTICES_DIR)
# share one number space, so both directories count.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cd "$ROOT" || exit 0

echo "- Today: $(date +%Y-%m-%d)"
echo "- Directories: ADRs in $ADR_DIR, practice records in $PRACTICES_DIR$([ -d "$PRACTICES_DIR" ] || echo ' (not created yet)'); one number space"
base="origin/$MAIN_BRANCH"
unknown=""
nums_main=""
if fetch_main; then
  nums_main=$(git ls-tree --name-only "$base" "$ADR_DIR/" "$PRACTICES_DIR/" 2>/dev/null | sed -n 's|.*/\([0-9]\{4\}\)-[^/]*$|\1|p')
  echo "- Highest record number on $base: $(printf '%s\n' "$nums_main" | sort | tail -1 | grep . || echo none)"
else
  echo "- Highest record number on $base: CANNOT CHECK (no fetch); do not take the number from this branch"
  unknown="$base"
fi
nums_prs=""
if [ "${CONTEXT_OFFLINE:-}" != 1 ] && command -v gh >/dev/null 2>&1 && prs=$(gh pr list --state open --limit 100 --json number,headRefName,files 2>/dev/null); then
  claims=$(printf '%s' "$prs" | jq -r --arg d "$ADR_DIR/" --arg p "$PRACTICES_DIR/" '.[] | . as $pr | .files[].path | select(startswith($d) or startswith($p)) | select(test("/[0-9]{4}-")) | "\($pr.number) \($pr.headRefName) \(.)"' 2>/dev/null)
  if [ -n "$claims" ]; then
    echo "- Record numbers claimed by open PRs:"
    printf '%s\n' "$claims" | sed 's/^/    #/'
    nums_prs=$(printf '%s\n' "$claims" | sed -n 's|.*/\([0-9]\{4\}\)-.*|\1|p')
  else
    echo "- Record numbers claimed by open PRs: none"
  fi
else
  echo "- Record numbers claimed by open PRs: CANNOT CHECK (gh unavailable or failed)"
  unknown="${unknown:+$unknown and }the open PRs"
fi
top=$(printf '%s\n%s\n' "$nums_main" "$nums_prs" | grep -E '^[0-9]{4}$' | sort | tail -1)
next=$(printf '%04d' $(( $(printf '%s' "${top:-0}" | sed 's/^0*//; s/^$/0/') + 1 )))
if [ -n "$unknown" ]; then
  echo "- Next number: at least $next, but UNKNOWN: $unknown could not be read. Read it before numbering."
else
  echo "- Next number: $next  (one past the highest of both answers above)"
fi
echo "- Records on THIS branch:"
for d in "$ADR_DIR" "$PRACTICES_DIR"; do
  ls "$d" 2>/dev/null | grep -E '^[0-9]{4}-' | sed "s|^|    $d/|"
done
