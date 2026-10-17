#!/bin/sh
# change-cost.sh: what a change has cost so far, against its budget (item 12 of the change process).
#
#   sh .claude/change-cost.sh <id>          one line: "cost $12.34 of budget $40 …", or CANNOT CHECK
#   sh .claude/change-cost.sh <id> --json   {"change","costUsd","budgetUsd","over","messages","unpriced","source"}
#
# WHERE THE FIGURE COMES FROM. Claude Code's transcripts on THIS machine
# (${CLAUDE_CONFIG_DIR:-~/.claude}/projects/**/*.jsonl, subagents included). Every assistant message
# records its model, its token usage, the git branch and the working directory it ran in. The cost
# of change <id> is the sum over every message whose branch is `<BRANCH_CHANGE><id>` and whose
# directory is this repository or one of its worktrees: the proposal lane, the build lane, the
# build-change workflow's builders, reviewers and mechanics, and any fix round on that branch.
#
# THE BRANCH IS NOT ENOUGH (OPS-74). A subagent records its LEAD session's branch, and a stale one:
# the branch the lead started on, usually `main`, even while the lead and the agent both work in the
# change's worktree. PANEL-45's seven builders and reviewers all recorded `main`. What does name the
# change is the working directory: so a message also counts when it ran in one of the change's
# worktrees, and an agent (a file under subagents/) counts WHOLE when any of its messages did, since
# a builder may start in the main checkout and cd into the worktree. The change's worktrees are the
# ones git has on the branch now, and every <repo>/.claude/worktrees/<lane> (the panel's and Claude
# Code's place for a lane) in which some message recorded the branch and NO other branch but main
# was recorded, so a removed worktree is still found once its lead recorded the branch there.
# A LEAD's branch is stale too: a session closing in its own lane recorded PANEL-48's branch there,
# beside its own. Such a lane is not the change's, and the messages that recorded the branch in it
# do not count either. Known limits: a removed worktree whose lane also recorded another branch (a
# reused directory, a switched branch) is not found, nor one in which nothing recorded the branch;
# and an agent counts whole once ANY of its messages ran in the change's worktree, so an unrelated
# agent passing through it (a reviewer of another PR, a session-close sweep) is billed whole.
#
# Each message counts once (a streamed message repeats its id), priced at the list prices in
# .claude/model-roles.json `.prices` (dollars per million tokens). It is an ESTIMATE at list price,
# not a bill, and it covers only this machine: a lane run elsewhere is not in it.
#
# The panel's Cost figure is not used: it is the status line's per-session total, held in the
# panel's memory, not per branch and not stored, and a workflow's agents never reach the status line.
#
# Exit 0 with a figure, 2 when it CANNOT CHECK (no jq, no transcripts for the branch), so a caller
# never reads "no data" as "$0".
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
. "$ROOT/.claude/lib/conf.sh"
. "$ROOT/.claude/lib/change.sh"
# The pricing is lib/usage.sh's, shared with usage-report.sh and metrics.sh: one price per message.
. "$ROOT/.claude/lib/usage.sh"

id=${1:-}; [ -n "$id" ] || { echo "usage: change-cost.sh <change-id> [--json]" >&2; exit 64; }
json=""; [ "${2:-}" = --json ] && json=1
cannot() {
  if [ -n "$json" ]; then jq -cn --arg c "$id" --arg r "$1" '{change:$c, costUsd:null, cannot:$r}' 2>/dev/null || echo "{\"change\":\"$id\",\"costUsd\":null}"
  else echo "CANNOT CHECK — $1"; fi
  exit 2
}
command -v jq >/dev/null 2>&1 || cannot "jq is not installed"
branch="$BRANCH_CHANGE$id"
roles="$ROOT/.claude/model-roles.json"
jq -e '.prices | type == "object"' "$roles" >/dev/null 2>&1 || cannot "$roles has no .prices table"

# This repository's main checkout, whatever worktree we run in: transcripts from any of its lanes.
common=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || cannot "$ROOT is not a git checkout"
main=$(dirname "$common")
projects=${CLAUDE_PROJECTS_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects}
[ -d "$projects" ] || cannot "no transcripts at $projects"
# Claude Code names a project's transcript directory after its path, every non-alphanumeric
# character a '-'; a worktree's directory extends its repository's name.
slug=$(printf '%s' "$main" | sed 's/[^A-Za-z0-9]/-/g')
dirs=$(find "$projects" -maxdepth 1 -type d -name "$slug*" 2>/dev/null)
bfiles=$(for p in $dirs; do grep -rlF --include='*.jsonl' "\"gitBranch\":\"$branch\"" "$p" 2>/dev/null; done)
# The change's worktrees (see the header): git's live ones on the branch, and each lane directory a
# message recorded the branch in. Never the main checkout, which stays on main and holds every lane.
wt="$main/.claude/worktrees/"
# shellcheck disable=SC2086
lanes=$( [ -z "$bfiles" ] || jq -r --arg b "$branch" --arg w "$wt" 'select(.gitBranch == $b) | (.cwd // "")
           | select(startswith($w)) | $w + (ltrimstr($w) | split("/")[0])' $bfiles 2>/dev/null | sort -u)
# A lead records a stale branch as well (see the header): a lane where any other branch but main was
# also recorded belongs to that branch, not to this change.
recorded_alone() {
  # shellcheck disable=SC2086
  for p in $dirs; do grep -rlF --include='*.jsonl' "\"cwd\":\"$1" "$p" 2>/dev/null; done | sort -u \
  | while read -r f; do jq -r --arg l "$1" 'select(.gitBranch != null) | select((.cwd // "") == $l
      or ((.cwd // "") | startswith($l + "/"))) | .gitBranch' "$f" 2>/dev/null; done \
  | grep -vxF -e "$branch" -e "$MAIN_BRANCH" -e HEAD -e '' | grep -q . && return 1
  return 0
}
roots=$( { git -C "$ROOT" worktree list --porcelain 2>/dev/null \
             | awk -v b="branch refs/heads/$branch" '/^worktree /{p=substr($0,10)} $0==b{print p}'
           printf '%s\n' "$lanes" | while read -r l; do [ -n "$l" ] && recorded_alone "$l" && echo "$l"; done
         } | grep -vxF "$main" | sort -u)
rootsjson=$(printf '%s\n' "$roots" | jq -Rsc 'split("\n") | map(select(. != ""))')
wfiles=$(printf '%s\n' "$roots" | while read -r r; do [ -n "$r" ] || continue
  for p in $dirs; do grep -rlF --include='*.jsonl' "\"cwd\":\"$r" "$p" 2>/dev/null; done; done)
files=$(printf '%s\n%s\n' "$bfiles" "$wfiles" | grep -v '^$' | sort -u)
[ -n "$files" ] || cannot "no transcript on this machine ran on $branch (under $projects/$slug*)"

budget=$(proposal_field "$ROOT/$CHANGES_DIR/$id/proposal.md" Budget | tr -d '$')
[ -n "$budget" ] || for a in "$ROOT/$CHANGES_DIR"/archive/*-"$id"/proposal.md; do
  [ -f "$a" ] && budget=$(proposal_field "$a" Budget | tr -d '$')
done

# shellcheck disable=SC2086
jq -c --arg b "$branch" --arg root "$main" --arg lanes "$wt" --argjson wts "$rootsjson" '
  select(.type == "assistant" and .message.usage != null) | (.cwd // "") as $cwd
  | any($wts[] as $w | $cwd == $w or ($cwd | startswith($w + "/")); .) as $inwt
  | {f: input_filename, id: (.message.id // .uuid), model: (.message.model // ""), u: .message.usage,
     onb: (.gitBranch == $b and ($cwd == $root or ($cwd | startswith($root + "/")))
           and ($inwt or ($cwd | startswith($lanes) | not))),
     inwt: $inwt}' $files 2>/dev/null \
| jq -s --slurpfile r "$roles" --arg c "$id" --arg budget "${budget:-}" "$USAGE_JQ_DEFS"'
  ($r[0].prices) as $p
  | [ group_by(.f)[] | ((.[0].f | contains("/subagents/")) and any(.[]; .inwt)) as $agent
      | .[] | select(.onb or .inwt or $agent) ]
  | (map({key: .id, value: .}) | from_entries | [.[]]) as $msgs
  | [ $msgs[] | . as $x | usd($p; $x.model; $x.u) as $usd
    | { model: $x.model, priced: ($usd != null), usd: ($usd // 0) } ]
  | { change: $c,
      costUsd: ((map(.usd) | add // 0) * 100 | round / 100),
      budgetUsd: (if $budget == "" then null else ($budget | tonumber) end),
      messages: length,
      unpriced: ([ .[] | select(.priced | not) | .model ] | unique | map(select(. != "<synthetic>"))),
      source: "transcripts on this machine, at list price" }
  | .over = (.budgetUsd != null and .costUsd > .budgetUsd)' > "${TMPDIR:-/tmp}/change-cost.$$" \
  || { rm -f "${TMPDIR:-/tmp}/change-cost.$$"; cannot "jq could not read the transcripts"; }
out=$(cat "${TMPDIR:-/tmp}/change-cost.$$"); rm -f "${TMPDIR:-/tmp}/change-cost.$$"
if [ -n "$json" ]; then printf '%s\n' "$out"; exit 0; fi
printf '%s' "$out" | jq -r '"cost $\(.costUsd) " + (if .budgetUsd == null then "(no budget)" else "of budget $\(.budgetUsd)" + (if .over then " — OVER BUDGET" else "" end) end)
  + " over \(.messages) message(s) on \(.change), \(.source)"
  + (if (.unpriced | length) > 0 then "; NOT priced (add to .prices): \(.unpriced | join(", "))" else "" end)'
