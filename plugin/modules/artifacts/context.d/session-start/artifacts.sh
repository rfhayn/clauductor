#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# session-start's artifacts section: every artifact the registry holds, with its link (walkthroughs
# with their roadmap row and step count, which the session reads progress for), and, with
# ARTIFACT_PUBLISH=claude.ai, each page copy's state against origin/MAIN_BRANCH: the OK lines carry
# the day main recorded the copy, the STALE lines the hash a republish would publish. The currency
# verdicts are the module's health line. No network: CONTEXT_OFFLINE changes nothing here.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
reg=${ARTIFACT_REGISTRY:-docs/artifacts.json}
command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — jq is not installed"; exit 0; }
if [ ! -f "$ROOT/$reg" ]; then echo "CANNOT CHECK — no $reg in this checkout (the artifacts module is on; start the registry, .claude/modules/artifacts/README.md)"; exit 0; fi
echo "Registry $reg (open every one: the module's session-start step):"
jq -r '
  to_entries[] | select(.key | startswith("$") | not) | .key as $s
  | (.value | if type == "object" then to_entries[] else empty end) | select(.key | startswith("$") | not)
  | select(.value | type == "object")
  | "  \($s)/\(.key) → \(.value.url // "NO URL")"
    + (if .value.steps != null or .value.row != null then " (walkthrough: row \(.value.row // "?"), \(.value.steps // "?") steps; read its progress)" else "" end)' \
  "$ROOT/$reg" 2>/dev/null || echo "  CANNOT CHECK — $reg is not readable JSON"
if [ "${ARTIFACT_PUBLISH:-off}" = claude.ai ]; then
  echo "Shared copies (ARTIFACT_PUBLISH=claude.ai; republish each STALE one you own):"
  sh "$CLAUDUCTOR_FW/modules/artifacts/bin/publish.sh" --root "$ROOT" --status --ref "origin/${MAIN_BRANCH:-main}" 2>&1 | sed 's/^/  /'
fi
exit 0
