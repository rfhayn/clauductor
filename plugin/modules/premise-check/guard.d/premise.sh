#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The premise-check module's merge-guard rule (BLOCKING): a PR on a PREMISE_REQUIRED_ON branch
# (default fix/) is blocked unless its body carries a premise-check receipt for every issue it
# fixes. Standing Tee's merge-guard rule 6, moved into a module.
#
# WHY. Fix units start from issue write-ups the code no longer matches. premise-check.sh reports
# that drift; this makes its report a condition of landing the fix, by requiring its receipt line
# (`premise-check: #N @ <sha>`) in the body for every issue the PR fixes.
#
# WHICH ISSUES: GitHub's `closingIssuesReferences` (what the merge will close), unioned with the
# body's closing keywords (the field can read empty at merge time), and when both are EMPTY, the
# `#N` references in the PR TITLE ("Fix #151/#152:" uses no closing keyword, yet names the issues
# the fix is about). A PR with neither is told so and allowed: there is no premise to check.
#
# Fails CLOSED when it cannot read the PR: the remedy is one command the blocked reader can run.
# NOT CHECKED: that the receipt's sha exists, or that the check ran BEFORE the fix. The receipt
# proves the report was produced and put where review reads it: presence.
#
# Contract (.claude/local/README.md): GUARD_* and ROOT in the environment; exit 0 allows, each
# stdout line an advisory; exit 2 blocks, stderr the reason. Needs gh and jq.
HERE=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh" || { echo "cannot read the project's configuration, so the premise check cannot be evaluated." >&2; exit 2; }
[ -f "$HERE/lib/receipt.sh" ] || { echo "cannot find $HERE/lib/receipt.sh, so the premise-check receipt cannot be checked. Restore it." >&2; exit 2; }
# shellcheck disable=SC1091
. "$HERE/lib/receipt.sh"

pr=$GUARD_PR branch=$GUARD_BRANCH
on=""
for p in ${PREMISE_REQUIRED_ON:-fix/}; do
  case $branch in "$p"*) on=$p ;; esac
done
[ -n "$on" ] || exit 0

cd "$ROOT" || exit 2
pr_json=$(gh pr view "$pr" --json title,body,closingIssuesReferences 2>/dev/null)
[ -n "$pr_json" ] || { echo "could not read PR #$pr's body and closing issues, so the premise check cannot be evaluated." >&2; exit 2; }
closes=$(printf '%s' "$pr_json" | jq -r '[.closingIssuesReferences[]?.number] | map(tostring) | join(" ")') \
  || { echo "could not parse PR #$pr's closing issues, so the premise check cannot be evaluated." >&2; exit 2; }
pr_body=$(printf '%s' "$pr_json" | jq -r '.body // ""')
body_closes=$(closing_refs_in_body "$pr_body" "${GUARD_REPO:-}")
# shellcheck disable=SC2086
closes=$(printf '%s\n' $closes $body_closes | grep . | sort -un | tr '\n' ' ')
closes=${closes% }
if [ -z "$closes" ]; then
  closes=$(printf '%s' "$pr_json" | jq -r '.title // ""' | grep -oE '#[0-9]+' | tr -d '#' | sort -un | tr '\n' ' ')
  closes=${closes% }
fi
if [ -z "$closes" ]; then
  echo "PR #$pr ($branch) closes no issue and names none in its title, so there is no premise to check. Not blocking."
  exit 0
fi
if missing=$(premise_receipt_missing "$pr_body" "$closes"); then
  echo "premise check present for every issue PR #$pr fixes: #$(printf '%s' "$closes" | sed 's/ / #/g')."
  exit 0
fi
cat >&2 <<EOF
PR #$pr ($branch) fixes $missing with no premise-check receipt in its body ($on PRs need one: PREMISE_REQUIRED_ON).
Run:  $(premise_command "$ROOT" "$HERE") $(printf '%s' "$missing" | tr -d '#')
read it against the issue BEFORE fixing, and paste its output (it ends 'premise-check: #N @ <sha>') into the PR body.
EOF
exit 2
