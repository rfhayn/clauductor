#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
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
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cd "$ROOT" || exit 1
P="$CLAUDUCTOR_FW/modules/people/people.sh"

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

# Every gh call is bounded (PEOPLE_GH_TIMEOUT seconds each), and the serial per-branch lookups
# share one budget (PEOPLE_WHO_BUDGET seconds): a slow GitHub must not hold session-start. A call
# that runs out is a failed read, said by name below. Not lib/modules.sh's with_timeout: its
# watchdog polls once a second and is waited for, which adds up to a second to EVERY call, and
# this section makes one call per branch. Here the watchdog is killed (not waited for) when gh
# returns, and kills its own sleep as it goes; its output is /dev/null, so it holds no command
# substitution open.
T=${PEOPLE_GH_TIMEOUT:-20}; BUDGET=${PEOPLE_WHO_BUDGET:-90}
notes="$state/notes"; : > "$notes"
ghb() {  # ghb WHAT ARGS...: gh ARGS, bounded; a timeout is noted under WHAT
  _gw=$1; shift
  gh "$@" 2>/dev/null &
  _gp=$!
  # The watchdog's sleeps run in the background and are waited for, so the TERM that ends the
  # watchdog (gh returned in time) reaches the trap at once and takes the sleep with it: no
  # orphaned sleep per call.
  (
    trap 'kill "$_gs" 2>/dev/null; exit 0' TERM
    sleep "$T" & _gs=$!; wait "$_gs" || exit 0
    kill -TERM "$_gp" 2>/dev/null
    sleep 2 & _gs=$!; wait "$_gs" || exit 0
    kill -KILL "$_gp" 2>/dev/null
  ) </dev/null >/dev/null 2>&1 &
  _gd=$!
  wait "$_gp"; _gr=$?
  kill "$_gd" 2>/dev/null
  case $_gr in 143 | 137) echo "CANNOT CHECK — gh timed out after ${T}s reading $_gw" >> "$notes" ;; esac
  return "$_gr"
}
LIMIT=100
if prs=$(ghb "the open-PR list" pr list --state open --limit "$LIMIT" --json number,title,headRefName,author); then
  printf '%s' "$prs" > "$state/open-prs.json"
  # A full page means there may be more: everything below read the first LIMIT only.
  [ "$(printf '%s' "$prs" | jq 'length' 2>/dev/null)" = "$LIMIT" ] \
    && echo "CANNOT CHECK — the open-PR list hit its limit of $LIMIT: Now, Next and the unmapped list read the first $LIMIT only" >> "$notes"
  # Remote branches with no open PR, each with GitHub's ACCOUNT for its last commit (GitHub maps
  # the commit's email to a login, so no hand-kept list of git author names is needed; `?` = the
  # lookup failed). Skipped when the PR list failed: every branch would look PR-less. The file is
  # written only when every lookup ran inside the budget.
  if names=$(ghb "the remote branches" api --paginate 'repos/{owner}/{repo}/branches?per_page=100' --jq '.[].name'); then
    start=$(date +%s)
    if printf '%s\n' "$names" | while IFS= read -r b; do
        [ -z "$b" ] || [ "$b" = "$MAIN_BRANCH" ] && continue
        printf '%s' "$prs" | jq -e --arg b "$b" 'any(.[]; .headRefName == $b)' >/dev/null 2>&1 && continue
        [ $(( $(date +%s) - start )) -lt "$BUDGET" ] || { echo "CANNOT CHECK — the branch lookups ran past ${BUDGET}s (PEOPLE_WHO_BUDGET)" >> "$notes"; exit 1; }
        info=$(ghb "branch $b's last commit" api "repos/{owner}/{repo}/commits/$b" --jq '[(.author.login // ""), ((.commit.author.date // "")[0:10])] | @tsv') || info=$(printf '?\t')
        printf '%s\t%s\n' "$b" "$info"
      done > "$state/branches.part"; then
      mv "$state/branches.part" "$state/branches.tsv"
    fi
  fi
fi
# Sorted by UPDATE, not gh's default creation order: a merge updates a PR, so an old PR merged
# today stays inside the limit. people.sh re-sorts by mergedAt.
for login in $logins; do
  ghb "$login's merged PRs" pr list --state merged --author "$login" --search "sort:updated-desc" --limit 50 \
    --json number,title,headRefName,mergedAt > "$state/merged-$login.json" || rm -f "$state/merged-$login.json"
done
cat "$notes"
sh "$P" --activity --state "$state" 2>&1
exit 0
