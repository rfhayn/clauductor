#!/bin/sh
# change-cost.sh: what a change has cost so far, against its budget (item 12 of the change process).
#
#   sh .claude/change-cost.sh <id>          one line: "cost $12.34 of budget $40 …", or CANNOT CHECK
#   sh .claude/change-cost.sh <id> --json   {"change","costUsd","budgetUsd","budgetKind","over","messages","rejectedLanes","unpriced","source"}
#                                           (budgetKind: hard, soft, none or unreadable; lib/change.sh proposal_budget)
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
# Code's place for a lane) in which some SESSION's lead (one sessionId, over all its transcript
# files, in time order) recorded the branch LAST, main, a detached HEAD and an empty branch not
# counting; so a removed worktree is still found. Its agents do not vote: an agent records its
# lead's branch NOW, so one left working in the lane after its lead moved on records the next
# lane's branch. Handled: the lane stays the change's, and that agent counts toward it.
# A LEAD's branch is stale too, both ways. A session closing in its own lane recorded PANEL-48's
# branch there before its own, and ended on its own: that lane is not the change's, and the
# messages that recorded the branch in it do not count either. A lead hopping in from another lane
# carries that lane's branch into its first message, then records the change's: that lane is.
# The lanes it rejected are named (the one line's "rejected lanes:", --json's rejectedLanes), so a
# collapse of the figure is visible. Known limits: a lane reused by two sessions, one ending on the
# branch, counts whole for it; once removed, a lane is lost whose only session switched away from
# the branch, or in which only agents, never a lead, recorded it; one in which nothing recorded the
# branch is not found. A lane whose OWN branch its lead never recorded (session 33570cf0 worked in
# fix-93 recording only other lanes' branches) goes to whichever change its lead recorded last. An agent counts whole once ANY of its messages ran in the
# change's worktree, so an unrelated agent passing through (a reviewer of another PR, a
# session-close sweep) is billed whole. And a lead in the MAIN checkout that records the change's
# stale branch counts, though its work there is not the change's.
# AN AGENT'S OWN AGENTS (OPS-120) record the lead's cwd and branch too, never the parent's, so a
# reviewer a teammate lane spawned may never touch the worktree: each agent descended (by its
# .meta.json's parentAgentId, anywhere in its session's subagents/) from one that counts whole
# counts whole. Known limit: so does everything an unrelated agent passing through spawned.
# A line cut off mid-write (a transcript still being appended) is skipped, never the end of the read.
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
# The budget, read the one way (proposal_budget): the open change's proposal, else its archived one.
# Read before anything can fail, so a CANNOT CHECK still reports it: build-change takes budgetUsd
# and budgetKind (hard, soft, none or unreadable) from this script's --json (OPS-85).
budget=$(proposal_budget "$ROOT/$CHANGES_DIR/$id/proposal.md")
[ -n "$budget" ] || for a in "$ROOT/$CHANGES_DIR"/archive/*-"$id"/proposal.md; do
  [ -f "$a" ] && budget=$(proposal_budget "$a")
done
case $budget in
  '') budgetUsd=null budgetKind=none ;;
  unreadable) budgetUsd=null budgetKind=unreadable ;;
  *) budgetUsd=$(awk -v n="${budget% *}" 'BEGIN { print n + 0 }') budgetKind=${budget#* } ;;
esac
cannot() {
  if [ -n "$json" ]; then jq -cn --arg c "$id" --arg r "$1" --argjson b "$budgetUsd" --arg k "$budgetKind" '{change:$c, costUsd:null, budgetUsd:$b, budgetKind:$k, cannot:$r}' 2>/dev/null \
    || echo "{\"change\":\"$id\",\"costUsd\":null,\"budgetUsd\":$budgetUsd,\"budgetKind\":\"$budgetKind\"}"
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
# Every transcript is read tagged: each line as `<file>\t<json>`, every file's last line ended, and
# $tagged undoing it into one record with $file in scope. A transcript still being written ends in a
# cut-off line: jq reading the files stops there, dropping every file after it, and `jq -R` joins it
# to the next file's first line. awk ends each file's lines apart, and a cut-off one is skipped. One
# awk per file, since awk stops at a file it cannot open (one deleted since it was listed). The
# split is at the LAST tab, since a JSON line holds none raw (the file lists split on blanks anyway).
tag() { for f in "$@"; do awk '{ print FILENAME "\t" $0 }' "$f" 2>/dev/null; done; }
tagged='split("\t") | (.[:-1] | join("\t")) as $file | .[-1] | fromjson? // empty'
# The change's worktrees (see the header): git's live ones on the branch, and each lane directory a
# message recorded the branch in. Never the main checkout, which stays on main and holds every lane.
wt="$main/.claude/worktrees/"
# shellcheck disable=SC2086
lanes=$( [ -z "$bfiles" ] || tag $bfiles | jq -rR --arg b "$branch" --arg w "$wt" "$tagged"' | select(.gitBranch == $b) | (.cwd // "")
           | select(startswith($w)) | $w + (ltrimstr($w) | split("/")[0])' 2>/dev/null | sort -u)
# A lead records a stale branch as well (see the header). Each session (one sessionId) whose LEAD
# worked in a lane votes with the branch its lead recorded there LAST, main, a detached HEAD and an
# empty branch not counting: the lane is the change's when some session's vote is the change's
# branch. Agents do not vote: they record the lead's branch NOW, wherever the lead has gone.
# "+lane" is taken, "-lane" rejected.
lanesjson=$(printf '%s\n' "$lanes" | jq -Rsc 'split("\n") | map(select(. != ""))')
lfiles=$(printf '%s\n' "$lanes" | while read -r l; do [ -n "$l" ] || continue
  for p in $dirs; do grep -rlF --include='*.jsonl' "\"cwd\":\"$l" "$p" 2>/dev/null; done; done | sort -u)
# shellcheck disable=SC2086
votes=$( [ -z "$lfiles" ] || tag $lfiles | jq -cR --argjson ls "$lanesjson" "$tagged"' | select(.gitBranch != null)
           | select($file | contains("/subagents/") | not) | (.cwd // "") as $c
           | first($ls[] as $w | select($c == $w or ($c | startswith($w + "/"))) | $w) as $l
           | {l: $l, s: (.sessionId // $file), t: (.timestamp // ""), b: .gitBranch}' 2>/dev/null \
         | jq -rs --arg b "$branch" --arg m "$MAIN_BRANCH" 'group_by(.l)[] | .[0].l as $l
           | [ group_by(.s)[] | sort_by(.t) | map(.b | select(. != $m and . != "HEAD" and . != "")) | last ]
           | (if any(.[]; . == $b) then "+" else "-" end) + $l' 2>/dev/null)
roots=$( { git -C "$ROOT" worktree list --porcelain 2>/dev/null \
             | awk -v b="branch refs/heads/$branch" '/^worktree /{p=substr($0,10)} $0==b{print p}'
           printf '%s\n' "$votes" | sed -n 's/^+//p'
         } | grep -vxF "$main" | sort -u)
rejected=$(printf '%s\n' "$votes" | sed -n 's/^-//p' | grep -vxF -e "$roots" -e '' | sort -u)
rejectedjson=$(printf '%s\n' "$rejected" | jq -Rsc 'split("\n") | map(select(. != ""))')
rootsjson=$(printf '%s\n' "$roots" | jq -Rsc 'split("\n") | map(select(. != ""))')
wfiles=$(printf '%s\n' "$roots" | while read -r r; do [ -n "$r" ] || continue
  for p in $dirs; do grep -rlF --include='*.jsonl' "\"cwd\":\"$r" "$p" 2>/dev/null; done; done)
files=$(printf '%s\n%s\n' "$bfiles" "$wfiles" | grep -v '^$' | sort -u)
[ -n "$files" ] || cannot "no transcript on this machine ran on $branch (under $projects/$slug*)"
# An agent's own agents count with it (OPS-120). A nested agent records its LEAD session's cwd and
# branch, not its parent's: a teammate lane's reviewers, and their /code-review agents, recorded the
# hub session's main checkout and branch, so neither test above saw them: OPS-93's pre-group check
# read $22.13, and its archived $55.84 missed them too (it reads $85.30 with them). Claude Code
# writes each agent's parent beside its transcript (agent-<id>.meta.json's parentAgentId), so every agent descended from one that counts whole (a
# message in the change's worktree) counts whole too. The walk covers each such agent's whole
# session (its subagents/ directory, at any depth), as lib/usage.sh's rootrole does: a workflow's
# agents sit in subagents/workflows/<run>/, their own agents in subagents/. An agent is named by
# its session and its id. `whole` is the one test of an agent counting whole; the pricing below
# bills exactly `$agents`, never a second copy of it.
sub=$(printf '%s\n' "$files" | grep '/subagents/')
# shellcheck disable=SC2086
whole=$( [ -z "$sub" ] || tag $sub | jq -rR --argjson wts "$rootsjson" "$tagged"'
           | select(.type == "assistant" and .message.usage != null)
           | (.cwd // "") as $c | select(any($wts[] as $w | $c == $w or ($c | startswith($w + "/")); .))
           | $file' 2>/dev/null | sort -u)
wholejson=$(printf '%s\n' "$whole" | jq -Rsc 'split("\n") | map(select(. != ""))')
nested=$( [ -z "$whole" ] || printf '%s\n' "$whole" | sed 's|/subagents/.*|/subagents|' | sort -u \
  | while read -r s; do find "$s" -name 'agent-*.meta.json' 2>/dev/null; done | while read -r m; do tag "$m"; done \
  | jq -cR "$tagged"' | {f: $file, p: (.parentAgentId // "")}' 2>/dev/null \
  | jq -rs --argjson whole "$wholejson" '
      def ses: sub("/subagents/.*"; ""); def aid: sub(".*/agent-"; "") | sub("\\.(jsonl|meta\\.json)$"; "");
      def grow($m; $in): ([ $m[] | select(.p != null and $in[.p]) | {key: .k, value: true} ] | from_entries) as $add
        | ($in + $add) as $n | if ($n | length) == ($in | length) then $in else grow($m; $n) end;
      map({k: ((.f | ses) + " " + (.f | aid)), p: (if .p == "" then null else (.f | ses) + " " + .p end),
           t: (.f | sub("\\.meta\\.json$"; ".jsonl"))}) as $m
      | grow($m; $whole | map({key: ((. | ses) + " " + (. | aid)), value: true}) | from_entries) as $all
      | $m[] | select($all[.k]) | .t' 2>/dev/null \
  | while read -r k; do [ -f "$k" ] && echo "$k"; done | grep -vxF -e "$whole" -e '' | sort -u)
agentsjson=$(printf '%s\n%s\n' "$whole" "$nested" | jq -Rsc 'split("\n") | map(select(. != ""))')
files=$(printf '%s\n%s\n' "$files" "$nested" | grep -v '^$' | sort -u)

# shellcheck disable=SC2086
tag $files | jq -cR --arg b "$branch" --arg root "$main" --arg lanes "$wt" --argjson wts "$rootsjson" "$tagged"'
  | select(.type == "assistant" and .message.usage != null) | (.cwd // "") as $cwd
  | any($wts[] as $w | $cwd == $w or ($cwd | startswith($w + "/")); .) as $inwt
  | {f: $file, id: (.message.id // .uuid), model: (.message.model // ""), u: .message.usage,
     onb: (.gitBranch == $b and ($cwd == $root or ($cwd | startswith($root + "/")))
           and ($inwt or ($cwd | startswith($lanes) | not))),
     inwt: $inwt}' 2>/dev/null \
| jq -s --slurpfile r "$roles" --arg c "$id" --argjson budget "$budgetUsd" --arg kind "$budgetKind" --argjson rej "$rejectedjson" --argjson agents "$agentsjson" "$USAGE_JQ_DEFS"'
  ($r[0].prices) as $p
  | [ group_by(.f)[] | (.[0].f | IN($agents[])) as $agent
      | .[] | select(.onb or .inwt or $agent) ]
  | (map({key: .id, value: .}) | from_entries | [.[]]) as $msgs
  | [ $msgs[] | . as $x | usd($p; $x.model; $x.u) as $usd
    | { model: $x.model, priced: ($usd != null), usd: ($usd // 0) } ]
  | { change: $c,
      costUsd: ((map(.usd) | add // 0) * 100 | round / 100),
      budgetUsd: $budget,
      budgetKind: $kind,
      messages: length,
      rejectedLanes: $rej,
      unpriced: ([ .[] | select(.priced | not) | .model ] | unique | map(select(. != "<synthetic>"))),
      source: "transcripts on this machine, at list price" }
  | .over = (.budgetUsd != null and .costUsd > .budgetUsd)' > "${TMPDIR:-/tmp}/change-cost.$$" \
  || { rm -f "${TMPDIR:-/tmp}/change-cost.$$"; cannot "jq could not read the transcripts"; }
out=$(cat "${TMPDIR:-/tmp}/change-cost.$$"); rm -f "${TMPDIR:-/tmp}/change-cost.$$"
if [ -n "$json" ]; then printf '%s\n' "$out"; exit 0; fi
printf '%s' "$out" | jq -r '"cost $\(.costUsd) " + (if .budgetKind == "unreadable" then "(budget UNREADABLE: the **Budget:** line must read $N, $N, soft or $N, hard)"
    elif .budgetUsd == null then "(no budget)"
    else "of \(if .budgetKind == "soft" then "soft " else "" end)budget $\(.budgetUsd)"
      + (if .over then " — OVER BUDGET" + (if .budgetKind == "soft" then " (soft: report it to the owner; the build goes on)" else "" end) else "" end) end)
  + " over \(.messages) message(s) on \(.change), \(.source)"
  + (if (.unpriced | length) > 0 then "; NOT priced (add to .prices): \(.unpriced | join(", "))" else "" end)
  + (if (.rejectedLanes | length) > 0 then "; rejected lanes: \(.rejectedLanes | map(split("/") | last) | join(", ")) (another branch was recorded there last)" else "" end)'
