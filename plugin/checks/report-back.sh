#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# A worker gets its agents' results, and the session that asked gets the worker's (OPS-43, folding
# in OPS-25). In an interactive session (fork mode, the default) every spawn runs in the background
# and the Agent tool has no `run_in_background`; Claude Code holds a launcher that ends its TURN
# until its spawned agents report, and delivers each result as its next turn. What loses a result
# is HANDING BACK: the launcher has finished, so the result goes elsewhere (Standing Tee #425 lost
# about 4h; on 2026-10-13 the top session relayed every builder and reviewer report by hand). In
# `-p` and the Agent SDK (fork mode off) a launcher that ends its turn has ended, and passing
# `run_in_background: false` is what keeps the result (code.claude.com/docs/en/sub-agents). So:
#   apply-change, merge-pr  wait by ending the turn without a hand-back, never poll, sleep or
#                           re-spawn (a second builder edits the same tree), pass `false` where the
#                           tool offers run_in_background, and send every outcome to a named
#                           `reportTo` session by SendMessage: without SendMessage, in the final
#                           hand-back, else by PushNotification;
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

for f in "$CLAUDUCTOR_FW/skills/apply-change/SKILL.md" "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md"; do
  s=$(basename "$(dirname "$f")")
  [ -f "$f" ] || { ok "no $s skill in this project"; continue; }
  has "$f" "$s waits for every agent it spawned by ending its turn, never by handing back, polling or re-spawning" \
    'end your turn with no final report and without SubagentHandback' 'Never poll, sleep or re-spawn'
  has "$f" "$s keeps a child's result where fork mode is off (-p, the Agent SDK)" 'If the Agent tool offers `run_in_background`'
  # Fork mode (the default) has no run_in_background, so an unconditional instruction to pass it is
  # inert there, and `true` is the hand-back this check exists to prevent.
  grep -F 'run_in_background' "$f" | grep -vqF 'If the Agent tool offers `run_in_background`' &&
    fail "$s names run_in_background outside the clause for when the Agent tool offers it"
  has "$f" "$s sends its outcome to the session named as reportTo" 'reportTo' 'SendMessage'
  has "$f" "$s has a channel without SendMessage: the hand-back, else PushNotification" \
    'If SendMessage is not in your tool list' 'in your final hand-back'
done
f="$CLAUDUCTOR_FW/skills/apply-change/SKILL.md"
[ -f "$f" ] && has "$f" "apply-change notifies the owner itself when no session asked" 'With no reportTo, notify the owner yourself'
f="$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md"
[ -f "$f" ] && has "$f" "merge-pr notifies the owner itself when no session asked" 'notify the owner with `PushNotification`'

wf="$CLAUDUCTOR_FW/workflows/build-change.js"
if [ -f "$wf" ]; then
  has "$wf" "build-change takes reportTo and tells the launching session to SendMessage it the outcome" \
    'reportTo?: "<session>"' 'SendMessage it' 'reportTo: typeof args.reportTo' 'If SendMessage is not in your tool list' 'in your final hand-back'
else
  ok "no build-change workflow in this project"
fi
finish
