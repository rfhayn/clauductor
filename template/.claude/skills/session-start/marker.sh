#!/bin/sh
# marker.sh: a row's session shows on GitHub from its first minute (OPS-108, the job OPS-102 had).
#
#   sh .claude/skills/session-start/marker.sh [<row id> [S<n>]]
#
# Run from the row's worktree, on the row's branch. With no row id it finds the row whose branch
# this is, since a lane template's first prompt knows only its lane's name (OPS-125); on a branch
# no row names it says so in one line and does nothing: that lane is no row's session. It opens the row's PR as a DRAFT whose body
# starts with the marker line, and the session-start cost under it:
#   session-start: S<n> <YYYY-MM-DD HH:MM ET> row <id>
#   session-start cost: <tokens> tokens, <minutes> min (this session so far: what orienting cost)
# S<n> is the argument, else the session of the roadmap's Session plan that names the row
# (.claude/roadmap-plan.sh, its one parser), else one past the top `## Session` of the journal on
# origin/main. The cost line is what the row's fragment copies into **Session start**
# (docs/journal.d/README.md); a reading that failed says CANNOT CHECK, never 0.
#
# WHY A DRAFT PR AND NOT A WRITE TO MAIN. A row's records reach main only through the row's own PR
# and the daily close (pr-merge-guard rules 7 and 8); the marker needs no file at all. The branch
# gets one empty commit when it has none ahead of origin/main, since GitHub opens no PR without one.
#
# FAIL SOFT, ONCE. Session start never blocks and never asks: any failure prints ONE line starting
# `marker: WARNING` and exits 0. A second run finds the marker on the open PR and adds nothing; an
# open PR without one gets the marker put first in its body.
ROOT=$(cd "$(dirname "$0")/../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"

warn() { echo "marker: WARNING — $*; the session goes on without a marker"; exit 0; }
case ${1:-} in -*) warn "usage: sh .claude/skills/session-start/marker.sh [<row id> [S<n>]]" ;; esac
rowid=${1:-}; sn=${2:-}
command -v gh >/dev/null 2>&1 || warn "gh is not installed"

tsv=$(roadmap_queue --tsv 2>&1) || warn "the roadmap did not parse ($(printf '%s' "$tsv" | head -1))"
here=$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null)
# Each row's branch: an ops or fix row's change id IS its branch (it starts with BRANCH_OPS or
# BRANCH_FIX); a capability change's branch takes BRANCH_CHANGE in front.
branches=$(printf '%s\n' "$tsv" | awk -F'\t' -v c="$BRANCH_CHANGE" 'NF >= 6 { print $4 "\t" ($6 == "change" ? c $5 : $5) }')
if [ -z "$rowid" ]; then
  rowid=$(printf '%s\n' "$branches" | awk -F'\t' -v b="$here" '$2 == b { print $1; exit }')
  [ -n "$rowid" ] || { echo "marker: no roadmap row is built on '$here', so this lane is no row's session; no marker"; exit 0; }
fi
row=$(printf '%s\n' "$tsv" | awk -F'\t' -v a="$rowid" '$4 == a { print; exit }')
[ -n "$row" ] || warn "no roadmap row has the id '$rowid'"
summary=$(printf '%s\n' "$row" | cut -f10 | cut -c1-60 | sed 's/[[:space:]]*$//')
branch=$(printf '%s\n' "$branches" | awk -F'\t' -v a="$rowid" '$1 == a { print $2; exit }')
[ "$here" = "$branch" ] || warn "this checkout is on '$here', not row $rowid's branch '$branch'"

if [ -z "$sn" ] && [ -f "$ROOT/.claude/roadmap-plan.sh" ]; then
  # A plan that does not parse numbers nothing here: the journal's count below still does.
  # An entry spanning sessions (`S15–S18`) names no one day: the journal's count below numbers
  # it, as panel-suggest numbers that day's close.
  # A change planned twice, `ID (proposal)` then its build: the build's session once its proposal
  # exists here, else the proposal's. A proposal with no build planned yet names no session for
  # the build, so the journal's count below numbers it; with no proposal, any entry does.
  want=proposal; [ -f "$ROOT/$CHANGES_DIR/$(printf '%s\n' "$row" | cut -f5)/proposal.md" ] && want=""
  sn=$(sh "$ROOT/.claude/roadmap-plan.sh" "$ROOT/$ROADMAP" 2>/dev/null | awk -F'\t' -v a="$rowid" -v w="$want" '$1 == "R" && $2 == a { s[$6] = $3 }
    END { v = (w in s) ? s[w] : (w == "proposal" ? s[""] : ""); if (v ~ /^S[0-9]+(\.[0-9]+)*$/) print v }')
fi
if [ -z "$sn" ]; then
  top=$(git -C "$ROOT" show "origin/$MAIN_BRANCH:$JOURNAL" 2>/dev/null | sed -n 's/^## Session \([0-9][0-9]*\).*/\1/p' | head -1)
  [ -n "$top" ] || warn "no session number given and none on origin/$MAIN_BRANCH's $JOURNAL to count from"
  sn="S$((top + 1))"
fi
case $sn in S[0-9]*) ;; *) sn="S$sn" ;; esac
when="$(TZ="$TIME_ZONE" date '+%Y-%m-%d %H:%M') $TIME_ZONE_LABEL"
marker="session-start: $sn $when row $rowid"

# The cost so far: every message of THIS session (its agents included), from its first.
cost="session-start cost: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this shell"
if [ -n "${CLAUDE_CODE_SESSION_ID:-}" ] && command -v jq >/dev/null 2>&1; then
  if recs=$(sh "$ROOT/.claude/usage-report.sh" --session "$CLAUDE_CODE_SESSION_ID" --records 2>&1) && [ -n "$recs" ]; then
    cost=$(printf '%s\n' "$recs" | jq -rs --arg now "$(date +%s)" '
      (map(.read + .write + .out) | add) as $t | (map(.ts) | min) as $first
      | "session-start cost: \($t) tokens, \((($now | tonumber) - $first) / 60 | floor) min"' 2>/dev/null) \
      || cost="session-start cost: CANNOT CHECK — usage-report.sh's records did not total"
  else
    cost="session-start cost: CANNOT CHECK — usage-report.sh: $(printf '%s' "$recs" | head -1)"
  fi
fi

open=$(gh pr list --head "$branch" --state open --json number,body --jq '.[0] // empty | "\(.number)\t\(.body | split("\n")[0])"' 2>&1) \
  || warn "gh could not list $branch's PRs ($(printf '%s' "$open" | head -1))"
if [ -n "$open" ]; then
  n=${open%%"	"*}; first=${open#*"	"}
  case $first in session-start:*) echo "marker: PR #$n already carries it ($first)"; exit 0 ;; esac
  body=$(gh pr view "$n" --json body --jq .body 2>/dev/null) || warn "gh could not read PR #$n"
  gh pr edit "$n" --body "$(printf '%s\n%s\n\n%s' "$marker" "$cost" "$body")" >/dev/null 2>&1 || warn "gh could not edit PR #$n"
  echo "marker: put first in PR #$n's body ($marker)"; exit 0
fi

if [ "$(git -C "$ROOT" rev-list --count "origin/$MAIN_BRANCH..HEAD" 2>/dev/null)" = 0 ]; then
  # From HEAD's tree, never the index: build-change's preflight runs this beside a resumed run's
  # staged, unreviewed work, which must stay uncommitted (OPS-126).
  c=$(git -C "$ROOT" commit-tree "HEAD^{tree}" -p HEAD -m "$rowid: Start the row's session ($sn)" 2>/dev/null) \
    && git -C "$ROOT" update-ref HEAD "$c" || warn "the empty marker commit failed"
fi
git -C "$ROOT" push -q -u origin "$branch" 2>/dev/null || warn "git push of $branch failed"
url=$(gh pr create --draft --head "$branch" --base "$MAIN_BRANCH" --title "$rowid: $summary" \
  --body "$(printf '%s\n%s\n\n%s' "$marker" "$cost" "A draft from the session's start: the row's work, its fragment ($JOURNAL_FRAGMENTS/$rowid.md) and its receipts follow.")" 2>&1) \
  || warn "gh pr create failed ($(printf '%s' "$url" | tail -1))"
echo "marker: opened $url as a draft ($marker)"
