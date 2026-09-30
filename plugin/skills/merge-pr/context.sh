#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Context for merge-pr: the branch, which review lane the diff takes, the PR, and gate evidence.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"

echo "- Branch: $(git branch --show-current 2>/dev/null)"
echo "- Review $(git diff --name-only --no-renames "origin/$MAIN_BRANCH...HEAD" 2>/dev/null | sh "$(dirname "$0")/review-lane.sh")"
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then
  echo "- Open PR for this branch: not asked (offline)"
elif command -v gh >/dev/null 2>&1; then
  echo "- Open PR for this branch: $(gh pr view --json number,title,state,url,author -q '"#\(.number) \(.title) [\(.state)] by \(.author.login) \(.url)"' 2>/dev/null || echo "none: push and open one with gh pr create")"
  echo "- You are: $(gh api user --jq .login 2>/dev/null || echo 'CANNOT CHECK (gh api user failed)')"
else
  echo "- Open PR: CANNOT CHECK (gh is not installed)"
fi
head=$(git rev-parse HEAD 2>/dev/null)
rec="$(git rev-parse --absolute-git-dir 2>/dev/null)/ci-receipt"
if [ -f "$rec" ]; then
  if [ "$(cut -f1 "$rec")" = "$head" ]; then
    echo "- Gate receipt in this checkout: names HEAD ($(cut -f3,4 "$rec" | tr '\t' ' '))"
  else
    echo "- Gate receipt in this checkout: names $(cut -c1-9 "$rec"), NOT HEAD $(printf %.9s "$head"): re-run $GATE_RUN"
  fi
else
  echo "- Gate receipt in this checkout: none (run $GATE_RUN)"
fi
