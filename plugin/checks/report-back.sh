#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# A worker gets its agents' results, and the session that asked gets the worker's (OPS-43, folding
# in OPS-25). An agent spawned in the background hands its result to whoever is listening when it
# ends. A worker that is itself a subagent has ended its turn by then, so the result reaches the
# top session and the worker waits in silence (Standing Tee #425 lost about 4h; on 2026-10-13 the
# top session relayed every builder and reviewer report by hand). So:
#   apply-change, merge-pr  spawn the builder and the reviewer in the FOREGROUND
#                           (`run_in_background: false`), whose result is the spawn's tool result,
#                           and send every outcome to a named `reportTo` session by SendMessage;
#   build-change.js         its agents already return to the script; the script cannot SendMessage,
#                           so it hands args.reportTo back in its report, and the session that
#                           launched it sends the outcome there.
# The reviewer has no SendMessage, and agents/reviewer.md and build-change.js's marked review
# sections stay as they are (pr-merge-guard rule 13): nothing asks the reviewer to report anywhere
# but its own final message, which a foreground spawn returns.
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
  has "$f" "$s spawns its agents in the foreground, so their results come back to it" 'run_in_background: false'
  grep -qF 'run_in_background: true' "$f" && fail "$s tells an agent to run in the background"
  has "$f" "$s sends its outcome to the session named as reportTo" 'reportTo' 'SendMessage'
done

wf="$CLAUDUCTOR_FW/workflows/build-change.js"
if [ -f "$wf" ]; then
  has "$wf" "build-change takes reportTo and tells the launching session to SendMessage it the outcome" \
    'reportTo?: "<session>"' 'SendMessage it' 'reportTo: typeof args.reportTo'
else
  ok "no build-change workflow in this project"
fi
finish
