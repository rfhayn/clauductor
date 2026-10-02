#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The premise-check module's merge-guard rule (BLOCKING): a PR on a PREMISE_REQUIRED_ON branch
# (default fix/) is blocked unless its body carries a premise-check receipt for every issue it
# fixes. Standing Tee's merge-guard rule 6, moved into a module.
#
# WHY. Fix units start from issue write-ups the code no longer matches. premise-check.sh reports
# that drift; this makes its report a condition of landing the fix, by requiring its receipt line
# (`premise-check: #N @ <sha>`) in the PR body for every issue the PR fixes.
#
# WHICH ISSUES, the UNION of everything that names one, because each can be the only one that does:
#   - GitHub's `closingIssuesReferences` (what the merge will close), this repository's only;
#   - the PR body's closing keywords (that field can read empty at merge time);
#   - the squash commit's own message: its subject (--subject/-t, an API merge's commit_title) and
#     its body (--body/--body-file, commit_message; merge-pr writes a body file), else the
#     branch's commit messages, which GitHub uses as the default squash body. Any of them can say
#     "Fixes #239" and close the issue on merge (change-guard.sh's cg_field reads every form);
#   - the `#N` in the PR title ("Fix #151/#152:" uses no keyword, yet names the issues);
#   - the number in a `<prefix><n>-<slug>` branch name (fix/239-card is about #239; a date-shaped
#     prefix, fix/2026-10-01-cleanup, is not an issue).
# A PR naming none is told so and allowed: there is no premise to check.
#
# Fails CLOSED when it cannot read the PR or the merge's body: the remedy is one command away.
# NOT CHECKED: that the receipt's sha exists, or that the check ran BEFORE the fix. The receipt
# proves the report was produced and put where review reads it: presence.
#
# Contract (.claude/local/README.md): GUARD_* and ROOT in the environment; exit 0 allows, each
# stdout line an advisory; exit 2 blocks, stderr the reason. Needs gh, jq and git.
HERE=$(cd "$(dirname "$0")/.." && pwd)
no() { echo "$1" >&2; exit 2; }
# Tested, not sourced blind: a `.` of a missing file ends the shell with a status that is not 2.
[ -f "$CLAUDUCTOR_FW/lib/conf.sh" ] || no "cannot read the project's configuration (lib/conf.sh is missing), so the premise check cannot be evaluated."
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
[ -f "$HERE/lib/receipt.sh" ] || no "cannot find $HERE/lib/receipt.sh, so the premise-check receipt cannot be checked. Restore it."
# shellcheck disable=SC1091
. "$HERE/lib/receipt.sh"

pr=$GUARD_PR branch=$GUARD_BRANCH
on=""
for p in ${PREMISE_REQUIRED_ON:-fix/}; do
  case $branch in "$p"*) on=$p ;; esac
done
[ -n "$on" ] || exit 0

# The merge's own body is read the way rule 12 reads it (cg_body, hooks/lib/change-guard.sh).
cg="$CLAUDUCTOR_FW/hooks/lib/change-guard.sh"
{ [ -f "$cg" ] && sh -n "$cg" 2>/dev/null; } || no "cannot read $cg, so the merge's own body cannot be checked for the issues it closes."
# shellcheck disable=SC1090
. "$cg"
for f in cg_body cg_subject; do
  command -v "$f" >/dev/null 2>&1 || no "$cg did not define $f, so the merge's own message cannot be checked."
done

cd "$ROOT" || exit 2
repo=${GUARD_REPO:-}
pr_json=$(gh pr view "$pr" --json title,body,closingIssuesReferences 2>/dev/null)
[ -n "$pr_json" ] || no "could not read PR #$pr's body and closing issues, so the premise check cannot be evaluated."
printf '%s' "$pr_json" | jq -e 'type == "object"' >/dev/null 2>&1 \
  || no "PR #$pr's details did not come back as JSON ($(printf '%s' "$pr_json" | head -c 80)), so the premise check cannot be evaluated."
gh_closes=$(printf '%s' "$pr_json" | jq -r --arg repo "$repo" '[.closingIssuesReferences[]?
    | select($repo == "" or .repository == null
             or ((((.repository.owner.login // "") + "/" + (.repository.name // "")) | ascii_downcase) == $repo))
    | .number | tostring] | join(" ")' 2>/dev/null) \
  || no "could not parse PR #$pr's closing issues, so the premise check cannot be evaluated."
pr_body=$(printf '%s' "$pr_json" | jq -r '.body // ""')
title_refs=$(printf '%s' "$pr_json" | jq -r '.title // ""' | grep -oE '#[0-9]+' | tr -d '#')
# fix/<n>-<slug>: <n> is an issue number by convention. A date-shaped prefix (fix/2026-10-01-…) is
# not one, and must not demand a receipt for #2026.
branch_ref=""
case ${branch#"$on"} in
  [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] | [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9][-_.]*) ;;
  *) branch_ref=$(printf '%s\n' "${branch#"$on"}" | sed -n 's/^\([0-9][0-9]*\)\(-.*\)\{0,1\}$/\1/p') ;;
esac

# The squash commit's message: the body the merge command sets, else the branch's commits.
cwd=$(jq -r '.cwd // empty' "${GUARD_PAYLOAD:-/dev/null}" 2>/dev/null)
merge_body=$(cg_body "${GUARD_COMMAND:-}" "${cwd:-$ROOT}"); mrc=$?
case $mrc in
  0) ;;
  1) merge_body=$(git -C "$ROOT" log --format=%B "$GUARD_BASE..$GUARD_HEAD" 2>/dev/null) \
       || no "could not read the commit messages of $GUARD_BASE..$GUARD_HEAD (the squash commit's default body), so the issues they close cannot be checked." ;;
  *) no "cannot read this merge's --body/--body-file (a substitution, an unreadable file, stdin, or an API merge's --input), so the issues the squash commit closes cannot be checked. Write the body to a file and pass --body-file <file>." ;;
esac
# The squash SUBJECT closes issues too ("Fixes #239: the card"); unset, GitHub uses the PR title,
# which is read below.
merge_subject=$(cg_subject "${GUARD_COMMAND:-}" "${cwd:-$ROOT}"); src=$?
case $src in
  0 | 1) ;;
  *) no "cannot read this merge's --subject/-t (a substitution, or an API merge's --input), so the issues the squash commit closes cannot be checked. Pass the subject as plain text." ;;
esac

# shellcheck disable=SC2086
closes=$(printf '%s\n' $gh_closes $(closing_refs_in_body "$pr_body" "$repo") \
  $(closing_refs_in_body "$merge_body" "$repo") $(closing_refs_in_body "$merge_subject" "$repo") $title_refs $branch_ref | grep . | sort -un | tr '\n' ' ')
closes=${closes% }
if [ -z "$closes" ]; then
  echo "PR #$pr ($branch) names no issue (GitHub's closing list, its body, the squash message, its title or its branch), so there is no premise to check. Not blocking."
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
