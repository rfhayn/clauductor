#!/bin/sh
# Health: every GitHub Actions workflow that runs on a `schedule:`, and its last run. A scheduled
# run's failure reaches no PR and no person, so this line is its reader.
#
# The set comes from the workflows' OWN `schedule:` keys (the authority), not from a list here: a
# new scheduled workflow is reported the moment it exists. Each verdict names its subject: the
# run's date and commit.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
dir="$ROOT/.github/workflows"
[ -d "$dir" ] || { echo "OK no .github/workflows (nothing scheduled)"; exit 0; }
files=$(grep -lE '^[[:space:]]*schedule:' "$dir"/*.yml "$dir"/*.yaml 2>/dev/null)
[ -n "$files" ] || { echo "OK no workflow has a schedule: trigger"; exit 0; }
[ "${CONTEXT_OFFLINE:-}" = 1 ] && { echo "CANNOT CHECK — offline"; exit 0; }
command -v gh >/dev/null 2>&1 || { echo "CANNOT CHECK — gh is not installed; scheduled results are UNKNOWN"; exit 0; }
command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — jq is not installed"; exit 0; }
for f in $files; do
  wf=$(basename "$f")
  runs=$(cd "$ROOT" && gh run list --workflow "$wf" --event schedule --limit 1 --json conclusion,status,createdAt,headSha 2>/dev/null) || {
    echo "CANNOT CHECK — $wf: gh run list failed (auth? offline?); this learned NOTHING"; continue; }
  if [ "$(printf '%s' "$runs" | jq 'length')" = 0 ]; then
    echo "NEVER RAN $wf: no scheduled run on record"
    continue
  fi
  printf '%s' "$runs" | jq -r --arg wf "$wf" '.[0] |
    (if .status != "completed" then "RUNNING" elif .conclusion == "success" then "OK" else "FAILED" end)
    + " \($wf): \(.conclusion // .status) on \(.createdAt[:10]) at \(.headSha[:9])"'
done
