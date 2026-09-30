#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# verify-change.sh: the verify step before a change merges (D7 of the change process). Read-only.
#
#   sh .claude/verify-change.sh <id> [--base <ref>]     exit 1 on any FAIL line
#
# Checks, on the branch as committed against where it forks from <ref> (default origin/MAIN_BRANCH,
# else MAIN_BRANCH):
#   1. the approval still covers the design as written (.claude/change-approval.sh);
#   2. every task in tasks.md is ticked (the Slice line aside);
#   3. every scenario the change adds or modifies is cited by a test, or escaped
#      (.claude/scenario-trace.sh --change <id>);
#   4. the diff touches what tasks.md claims: a ticked task that names paths (in backticks) has at
#      least one of them in the diff. A task naming a path it only reads still names others it
#      changed; a task naming ONLY paths the diff never touched claims work that is not there.
# It also lists, as NOTE lines, changed files no task names: not a failure (a helper, a fixture), but
# the reviewer's first question.
#
# build-change runs it after the receipt, and merge-pr before it merges a change PR (step 2c).
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
. "$CLAUDUCTOR_FW/lib/conf.sh"
. "$CLAUDUCTOR_FW/lib/change.sh"

id=${1:-}; [ -n "$id" ] || { echo "usage: verify-change.sh <change-id> [--base <ref>]" >&2; exit 64; }
shift
base_ref=""
[ "${1:-}" = --base ] && base_ref=${2:-}
dir="$ROOT/$CHANGES_DIR/$id"
fails=0
ok() { echo "ok   $*"; }
fail() { echo "FAIL $*"; fails=$((fails + 1)); }
[ -f "$dir/tasks.md" ] || { echo "FAIL $CHANGES_DIR/$id/tasks.md does not exist"; exit 1; }

# 1. The approval.
out=$(sh "$CLAUDUCTOR_FW/change-approval.sh" "$id" 2>&1) && ok "approval: $out" || fail "approval: $out"

# 2. Every task ticked.
open=$(open_tasks "$dir/tasks.md")
if [ "$open" -eq 0 ]; then ok "every task in tasks.md is ticked ($(done_tasks "$dir/tasks.md") done)"
else
  fail "$open task(s) still open in tasks.md:"
  grep -E '^[[:space:]]*- \[ \]' "$dir/tasks.md" | grep -Ev "$_is_slice" | sed 's/^/       /'
fi

# 3. The scenarios.
out=$(sh "$CLAUDUCTOR_FW/scenario-trace.sh" --check --change "$id" 2>&1) && ok "scenarios: $(printf '%s\n' "$out" | tail -1)" \
  || { fail "scenarios: $(printf '%s\n' "$out" | tail -1)"; printf '%s\n' "$out" | grep -E '^(MISSING|PENDING)' | sed 's/^/       /'; }

# 4. The diff against the fork point.
if [ -z "$base_ref" ]; then
  if git -C "$ROOT" rev-parse -q --verify "origin/$MAIN_BRANCH" >/dev/null 2>&1; then base_ref="origin/$MAIN_BRANCH"; else base_ref=$MAIN_BRANCH; fi
fi
base=$(git -C "$ROOT" merge-base "$base_ref" HEAD 2>/dev/null) || { fail "cannot find where HEAD forks from $base_ref"; echo "verify-change: $fails FAIL"; exit 1; }
changed=$(git -C "$ROOT" diff --name-only "$base" HEAD)
[ -n "$changed" ] || fail "HEAD changes nothing since $base_ref"
# Ticked task lines, and the path-like tokens in backticks on each: containing a / or a file extension.
grep -nE '^[[:space:]]*- \[[xX]\]' "$dir/tasks.md" | grep -Ev "$_is_slice" | while IFS= read -r line; do
  paths=$(printf '%s\n' "$line" | grep -oE '`[^` ]+`' | tr -d '`' | grep -E '/|\.[A-Za-z0-9]+$' | grep -vE '^(https?:|-)' || true)
  [ -n "$paths" ] || continue
  hit=""
  for p in $paths; do
    p=${p#./}
    if printf '%s\n' "$changed" | grep -qxF "$p" || printf '%s\n' "$changed" | grep -q "^${p%/}/"; then hit=1; break; fi
  done
  if [ -n "$hit" ]; then echo "ok   task names a changed path: $(printf '%s' "${line#*:}" | sed 's/^[[:space:]]*//' | cut -c1-90)"
  else echo "FAIL task claims $(printf '%s' "$paths" | tr '\n' ' ')but the diff touches none of them: $(printf '%s' "${line#*:}" | sed 's/^[[:space:]]*//' | cut -c1-90)"; fi
done > "${TMPDIR:-/tmp}/verify.$$"
cat "${TMPDIR:-/tmp}/verify.$$"; fails=$((fails + $(grep -c '^FAIL' "${TMPDIR:-/tmp}/verify.$$")))
named=$(grep -oE '`[^` ]+`' "$dir/tasks.md" | tr -d '`' | sed 's|^\./||')
printf '%s\n' "$changed" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  case "$f" in "$CHANGES_DIR"/*|"$SPECS_DIR"/*) continue ;; esac
  printf '%s\n' "$named" | grep -qxF "$f" || echo "NOTE $f changed, and no task names it"
done
rm -f "${TMPDIR:-/tmp}/verify.$$"

if [ "$fails" -gt 0 ]; then echo "verify-change: $id FAILED ($fails) against $base_ref at $(git -C "$ROOT" rev-parse --short HEAD)"; exit 1; fi
echo "verify-change: $id verified against $base_ref at $(git -C "$ROOT" rev-parse --short HEAD)"
