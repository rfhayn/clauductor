#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# session-close's artifacts section, on THIS tree (--worktree): each core artifact against its
# authorities (the module's step 3 refreshes or stamps every line that is not OK, because the
# module's guard rule refuses this close's PR until none is), and, with ARTIFACT_PUBLISH=claude.ai,
# each page copy (step 7 records each STALE one). No network: CONTEXT_OFFLINE changes nothing.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
out=$(sh "$CLAUDUCTOR_FW/modules/artifacts/bin/currency.sh" --root "$ROOT" --worktree 2>&1) \
  || out="CANNOT CHECK — core-artifact currency: $(printf '%s' "$out" | tail -n 1 | sed 's/^artifacts: //'). UNKNOWN, not current."
printf '%s\n' "$out"
if [ "${ARTIFACT_PUBLISH:-off}" = claude.ai ]; then
  echo "Shared copies of this tree (record each STALE one you own before the close's PR):"
  sh "$CLAUDUCTOR_FW/modules/artifacts/bin/publish.sh" --root "$ROOT" --status --worktree 2>&1 | sed 's/^/  /'
fi
exit 0
