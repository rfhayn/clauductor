#!/bin/sh
# Who is on what: for each person in the people registry (PEOPLE), the last thing they merged,
# what they have open now (PRs and PR-less remote branches), and the next roadmap row they own that
# nobody has a PR or branch for. session-start prints it through `extensions.sh context
# session-start` while the people module is on; the module's session-start fragment has the
# summary repeat it.
#
# This section does the GitHub reads and nothing else; people.sh --activity derives every line
# from what was read, so the derivation is tested on fixtures with no network. A file is written
# into the state directory only when its read SUCCEEDED: a missing one prints CANNOT CHECK there,
# never "none". No login is named here; every one comes from the registry.
ROOT=$(cd "$(dirname "$0")/../../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
cd "$ROOT" || exit 1
P="$ROOT/.claude/modules/people/people.sh"

echo "Per person: Last = latest merged PR, Now = open PRs and PR-less branches, Next = their top unfinished roadmap row nobody has a PR or branch for."
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then echo "CANNOT CHECK — offline"; exit 0; fi
if ! command -v gh >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
  echo "CANNOT CHECK — gh or jq is not installed; this learned NOTHING, do not read it as nobody working"; exit 0
fi
logins=$(sh "$P" --logins 2>&1) || { printf '%s\n' "$logins"; exit 0; }
# A failed mktemp must not end the briefing: say so and stop this section only.
state=$(mktemp -d "${TMPDIR:-/tmp}/people-who.XXXXXX" 2>/dev/null) || state=
[ -n "$state" ] || { echo "CANNOT CHECK — mktemp failed, so nothing read from GitHub could be handed over"; exit 0; }
trap 'rm -rf "$state"' EXIT

if prs=$(gh pr list --state open --limit 100 --json number,title,headRefName,author 2>/dev/null); then
  printf '%s' "$prs" > "$state/open-prs.json"
  # Remote branches with no open PR, each with GitHub's ACCOUNT for its last commit (GitHub maps
  # the commit's email to a login, so no hand-kept list of git author names is needed; `?` = the
  # lookup failed). Skipped when the PR list failed: every branch would look PR-less.
  if names=$(gh api --paginate 'repos/{owner}/{repo}/branches?per_page=100' --jq '.[].name' 2>/dev/null); then
    printf '%s\n' "$names" | while IFS= read -r b; do
      [ -z "$b" ] || [ "$b" = "$MAIN_BRANCH" ] && continue
      printf '%s' "$prs" | jq -e --arg b "$b" 'any(.[]; .headRefName == $b)' >/dev/null 2>&1 && continue
      info=$(gh api "repos/{owner}/{repo}/commits/$b" --jq '[(.author.login // ""), ((.commit.author.date // "")[0:10])] | @tsv' 2>/dev/null) || info=$(printf '?\t')
      printf '%s\t%s\n' "$b" "$info"
    done > "$state/branches.tsv"
  fi
fi
# Sorted by UPDATE, not gh's default creation order: a merge updates a PR, so an old PR merged
# today stays inside the limit. people.sh re-sorts by mergedAt.
for login in $logins; do
  gh pr list --state merged --author "$login" --search "sort:updated-desc" --limit 50 \
    --json number,title,headRefName,mergedAt > "$state/merged-$login.json" 2>/dev/null || rm -f "$state/merged-$login.json"
done
sh "$P" --activity --state "$state" 2>&1
exit 0
