#!/bin/sh
# A worker gets its agents' results, and the session that asked gets the worker's (OPS-43, folding
# in OPS-25). In fork mode (the default) every spawn runs in the background: the Agent tool has no
# `run_in_background`. A nested agent that finishes after its launcher has ended reports to the
# main conversation instead (code.claude.com/docs/en/sub-agents), so a worker that hands back
# early waits in silence (Standing Tee #425 lost about 4h; on 2026-10-13 the top session relayed
# every builder and reviewer report by hand). So:
#   apply-change, merge-pr  never end the run or hand back while a spawned agent has not reported,
#                           never spawn one again (a second builder edits the same tree), and send
#                           every outcome to a named `reportTo` session by SendMessage: without
#                           SendMessage, in the hand-back, else by PushNotification;
#   build-change.js         its agents already return to the script; the script cannot SendMessage,
#                           so it hands args.reportTo back in its report, and the session that
#                           launched it sends the outcome there.
# The reviewer has no SendMessage, and agents/reviewer.md and build-change.js's marked review
# sections stay as they are (pr-merge-guard rule 13): nothing asks the reviewer to report anywhere
# but its own final message, which reaches a launcher that is still running.
#
# Each instruction is text a model follows, so what this executes is its PRESENCE: delete one and
# this fails. checks/build-change.sh's driver proves the report carries reportTo.
. "$(dirname "$0")/lib.sh"

# has FILE LABEL TEXT...: FILE names every TEXT (fixed strings). sh has no locals: _-names only.
has() {
  _hf=$1 _hl=$2; shift 2
  for _ht in "$@"; do
    grep -qF -- "$_ht" "$_hf" || { fail "$_hl: $(basename "$(dirname "$_hf")")/$(basename "$_hf") no longer says '$_ht'"; return; }
  done
  ok "$_hl"
}

for f in "$ROOT/.claude/skills/apply-change/SKILL.md" "$ROOT/.claude/skills/merge-pr/SKILL.md"; do
  s=$(basename "$(dirname "$f")")
  [ -f "$f" ] || { ok "no $s skill in this project"; continue; }
  has "$f" "$s waits for every agent it spawned, and never spawns one again" \
    'Never spawn it again' 'Do not end your run or hand back while any agent you spawned has not reported'
  grep -qF 'run_in_background' "$f" && fail "$s names run_in_background, which fork mode (the default) removes from the Agent tool"
  has "$f" "$s sends its outcome to the session named as reportTo" 'reportTo' 'SendMessage'
  has "$f" "$s has a channel without SendMessage: the hand-back, else PushNotification" 'If SendMessage is not in your tool list'
done
f="$ROOT/.claude/skills/apply-change/SKILL.md"
[ -f "$f" ] && has "$f" "apply-change notifies the owner itself when no session asked" 'With none, notify the owner yourself'
f="$ROOT/.claude/skills/merge-pr/SKILL.md"
[ -f "$f" ] && has "$f" "merge-pr notifies the owner itself when no session asked" 'notify the owner with `PushNotification`'

wf="$ROOT/.claude/workflows/build-change.js"
if [ -f "$wf" ]; then
  has "$wf" "build-change takes reportTo and tells the launching session to SendMessage it the outcome" \
    'reportTo?: "<session>"' 'SendMessage it' 'reportTo: typeof args.reportTo' 'If SendMessage is not in your tool list'
else
  ok "no build-change workflow in this project"
fi
finish
