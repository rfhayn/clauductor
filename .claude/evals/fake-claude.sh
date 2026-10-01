#!/bin/sh
# fake-claude.sh: a stand-in for `claude -p` that returns canned review findings, so the eval
# harness runs end to end without a model or a token. The checks use it (checks/evals.sh,
# checks/merge-guard.sh); you can too, to try the harness before paying for a run:
#
#   EVAL_CLAUDE=.claude/evals/fake-claude.sh sh .claude/evals/run.sh --role reviewer --model opus --effort high --out /tmp/r
#
# EVAL_FAKE chooses what it answers (run.sh passes the case's directory in EVAL_CASE_DIR):
#   perfect      (default) exactly the case's planted defects, at their expected severity
#   silent       no findings at all
#   error        exits 1, as a failed run does
#   dir:<path>   the findings array in <path>/<case id>.json, or none when that file is absent
# EVAL_FAKE_COST is each run's total_cost_usd (default 0.25). EVAL_FAKE_LOG, when set, receives
# each call's arguments and the `git diff HEAD --stat` it was run on.
prompt=""; prev=""
for a in "$@"; do
  [ "$prev" = "-p" ] && prompt=$a
  prev=$a
done
id=$(basename "${EVAL_CASE_DIR:-unknown}")
if [ -n "${EVAL_FAKE_LOG:-}" ]; then
  { echo "== $id"; printf 'arg: %s\n' "$@"; git diff HEAD --stat 2>/dev/null | tail -1; } >> "$EVAL_FAKE_LOG"
fi
mode=${EVAL_FAKE:-perfect}
case "$mode" in
  error) echo "fake-claude: simulated failure" >&2; exit 1 ;;
  silent) findings='[]' ;;
  perfect)
    findings=$(jq -c '[(.expected // [])[] | {severity, file, line: .lines[0], summary: (.why // .id), failure: (.why // .id), group: 1}]' "$EVAL_CASE_DIR/case.json") || exit 1 ;;
  dir:*)
    f="${mode#dir:}/$id.json"
    if [ -f "$f" ]; then findings=$(cat "$f"); else findings='[]'; fi ;;
  *) echo "fake-claude: unknown EVAL_FAKE '$mode'" >&2; exit 2 ;;
esac
[ -n "$prompt" ] || { echo "fake-claude: no -p prompt" >&2; exit 2; }
jq -cn --argjson f "$findings" --argjson c "${EVAL_FAKE_COST:-0.25}" '{
  type: "result", subtype: "success", is_error: false, result: "",
  structured_output: {findings: $f}, total_cost_usd: $c,
  usage: {input_tokens: 3000, output_tokens: 9000, cache_read_input_tokens: 240000, cache_creation_input_tokens: 30000}}'
