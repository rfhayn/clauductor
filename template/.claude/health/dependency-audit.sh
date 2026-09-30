#!/bin/sh
# Health: the scheduled dependency audit (.github/workflows/dependency-audit.yml) and the
# advisories it last found. A failed run means an advisory of high severity or worse in a production
# dependency, and reaches no PR, so this line is its reader. It names the run's date and commit; a
# run older than two weeks is STALE (the schedule is weekly).
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
wf="dependency-audit.yml"
[ -f "$ROOT/.github/workflows/$wf" ] || { echo "OK no $wf in .github/workflows (no scheduled dependency audit)"; exit 0; }
[ "${CONTEXT_OFFLINE:-}" = 1 ] && { echo "CANNOT CHECK — offline"; exit 0; }
command -v gh >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — gh or jq is not installed; advisories are UNKNOWN"; exit 0; }
runs=$(cd "$ROOT" && gh run list --workflow "$wf" --limit 1 --json conclusion,status,createdAt,headSha,url 2>/dev/null) || {
  echo "CANNOT CHECK — gh run list failed (auth? offline?); advisories are UNKNOWN"; exit 0; }
[ "$(printf '%s' "$runs" | jq 'length')" = 0 ] && { echo "NEVER RAN $wf: no audit on record, so advisories are UNKNOWN (gh workflow run $wf)"; exit 0; }
stale=$(date -u -v-14d +%Y-%m-%d 2>/dev/null || date -u -d '14 days ago' +%Y-%m-%d)
printf '%s' "$runs" | jq -r --arg wf "$wf" --arg stale "$stale" '.[0] |
  (if .status != "completed" then "RUNNING"
   elif .conclusion == "success" then (if .createdAt[:10] < $stale then "STALE" else "OK" end)
   else "ADVISORIES" end)
  + " \($wf): " + (if .conclusion == "success" then "no advisory of high severity" elif .status != "completed" then .status else "\(.conclusion): an advisory of high severity or worse, see \(.url)" end)
  + " (run of \(.createdAt[:10]) at \(.headSha[:9]))"'
