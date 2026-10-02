#!/bin/sh
# session-close's living-visuals section, on THIS tree: each living page's currency with how to
# refresh it (living.sh --list --worktree), then every rule the gate will hold the pages to
# (living.sh --check), FAIL lines only, so the close fixes them before its PR rather than after a
# red gate. No network. Never silent: a tool that cannot run says CANNOT CHECK.
ROOT=$(cd "$(dirname "$0")/../../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
lv="$ROOT/.claude/modules/living-visuals/bin/living.sh"
out=$(sh "$lv" --root "$ROOT" --list --worktree 2>&1)
case $? in 0 | 1) printf '%s\n' "$out" ;;
  *) echo "CANNOT CHECK — living pages' currency: $(printf '%s' "$out" | tail -n 1 | sed 's/^living-visuals: //'). UNKNOWN, not current." ;; esac
chk=$(sh "$lv" --root "$ROOT" --check 2>&1); rc=$?
n=$(printf '%s\n' "$chk" | grep -c '^ok')
if [ "$rc" -eq 0 ]; then echo "Page rules (the gate's living-visuals:pages): all $n pass on this tree"
elif printf '%s\n' "$chk" | grep -q '^FAIL'; then
  echo "Page rules (the gate's living-visuals:pages) failing on this tree; fix each before the close's PR:"
  printf '%s\n' "$chk" | grep '^FAIL' | sed 's/^FAIL /  /'
else echo "CANNOT CHECK — the page rules: $(printf '%s' "$chk" | tail -n 1)"; fi
exit 0
