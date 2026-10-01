#!/bin/sh
# Health: the latest `push` run on the main branch, per workflow. A post-merge run catches what no
# PR can (two merges without a rebase between them), and its failure reaches no PR either. Reports
# each workflow's latest push run that is not a success, naming the commit it ran on.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
[ -d "$ROOT/.github/workflows" ] || { echo "OK no .github/workflows"; exit 0; }
grep -lqE '^[[:space:]]*push:' "$ROOT"/.github/workflows/*.y*ml 2>/dev/null || { echo "OK no workflow runs on push"; exit 0; }
[ "${CONTEXT_OFFLINE:-}" = 1 ] && { echo "CANNOT CHECK — offline"; exit 0; }
command -v gh >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — gh or jq is not installed"; exit 0; }
runs=$(cd "$ROOT" && gh run list --branch "$MAIN_BRANCH" --event push --limit 30 --json workflowName,conclusion,status,createdAt,headSha 2>/dev/null) || {
  echo "CANNOT CHECK — gh run list failed; push results on $MAIN_BRANCH are UNKNOWN"; exit 0; }
printf '%s' "$runs" | jq -r '
  if length == 0 then "OK no push runs on record" else
  (group_by(.workflowName) | map(max_by(.createdAt)) | .[] |
    (if .status != "completed" then "RUNNING" elif .conclusion == "success" then "OK" else "FAILED" end)
    + " \(.workflowName): \(.conclusion // .status) on \(.createdAt[:10]) at \(.headSha[:9])")
  end'
