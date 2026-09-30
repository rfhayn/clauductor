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
# build-change workflow's builders, reviewers and mechanics, and any fix round on that branch. Each
# message counts once (a streamed message repeats its id), priced at the list prices in
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
files=$(find "$projects" -maxdepth 1 -type d -name "$slug*" 2>/dev/null | while read -r p; do
  grep -rlF --include='*.jsonl' "\"gitBranch\":\"$branch\"" "$p" 2>/dev/null; done)
[ -n "$files" ] || cannot "no transcript on this machine ran on $branch (under $projects/$slug*)"

budget=$(proposal_field "$ROOT/$CHANGES_DIR/$id/proposal.md" Budget | tr -d '$')
[ -n "$budget" ] || for a in "$ROOT/$CHANGES_DIR"/archive/*-"$id"/proposal.md; do
  [ -f "$a" ] && budget=$(proposal_field "$a" Budget | tr -d '$')
done

# shellcheck disable=SC2086
jq -c --arg b "$branch" --arg root "$main" '
  select(.type == "assistant" and .gitBranch == $b and .message.usage != null
         and ((.cwd // "") == $root or ((.cwd // "") | startswith($root + "/"))))
  | {id: (.message.id // .uuid), model: (.message.model // ""), u: .message.usage}' $files 2>/dev/null \
| jq -s --slurpfile r "$roles" --arg c "$id" --arg budget "${budget:-}" '
  ($r[0].prices) as $p
  | (map({key: .id, value: .}) | from_entries | [.[]]) as $msgs
  | def price($m): ($p | to_entries | map(.key as $k | select($k != "_why" and ($m | startswith($k))))
                    | sort_by(.key | length) | last | .value);
  [ $msgs[] | . as $x | price($x.model) as $pr
    | ($x.u.cache_creation.ephemeral_5m_input_tokens // null) as $w5
    | ($x.u.cache_creation.ephemeral_1h_input_tokens // null) as $w1
    | { model: $x.model, priced: ($pr != null),
        usd: (if $pr == null then 0 else
          ( ($x.u.input_tokens // 0) * $pr.input
          + ($x.u.output_tokens // 0) * $pr.output
          + ($x.u.cache_read_input_tokens // 0) * $pr.cache_read
          + (if $w5 == null and $w1 == null then ($x.u.cache_creation_input_tokens // 0) * $pr.input * 1.25
             else ($w5 // 0) * $pr.input * 1.25 + ($w1 // 0) * $pr.input * 2 end) ) / 1000000 end) } ]
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
