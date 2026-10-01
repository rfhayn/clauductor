#!/usr/bin/env bash
# gate.sh: run-local.sh for an AGENT (GATE in .claude/project.conf). The full log goes to a file;
# stdout gets the stage markers, the failure lines and the verdict tail, nothing else. Same flags
# as run-local.sh, same exit code.
#
# WHY. A full gate can print tens of thousands of lines. An agent that runs it in the foreground
# carries every line in its context for the rest of its life, and each later turn re-reads them
# as cache. An instruction to "read only the failures" is prose an agent can skip; this makes the
# small output the only output (*Reach is set by structure, not by the prompt*).
#
# NOTHING IS DROPPED, only moved: the complete log is at the path printed on the last line, so a
# failure the filter did not match is one `grep` away. The verdict still comes from run-local.sh's
# own lines (PASS/FAIL, receipt), which the tail always includes.
#
#   scripts/ci/gate.sh --quick     the inner loop (build-change's gate)
#   scripts/ci/gate.sh             the full gate; writes the receipt as usual
#
# GATE_FAIL_PATTERN (project.conf) is the extended regex of failure lines for YOUR tools; it is
# matched against a colour-stripped copy, and never anchored at line start, because many runners
# prefix their lines. GATE_RUNNER overrides the command (the checks use it).
set -uo pipefail

ROOT=$(git rev-parse --show-toplevel)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
LOG="$(git rev-parse --absolute-git-dir)/ci-gate.log"
RUNNER=${GATE_RUNNER:-"$ROOT/$GATE_RUN"}
MAX_FAIL=${GATE_MAX_FAIL_LINES:-80}
TAIL=${GATE_TAIL_LINES:-25}

bash "$RUNNER" "$@" >"$LOG" 2>&1
code=$?

# Generic failure shapes: the gate's own markers, the lease's lines (so a slow gate reads as
# QUEUED, not hung), and the words most test runners and compilers print on failure.
DEFAULT='^==> |^(lease|gate): |(^|[^A-Za-z])(FAIL|FAILED|ERROR|Error|error)(:|\b)|panic:|Traceback|AssertionError|✗|✘|×|[0-9]+ (failed|errors?)\b'
PATTERN=${GATE_FAIL_PATTERN:-$DEFAULT}
PLAIN="$LOG.plain"
sed $'s/\033\\[[0-9;]*m//g' "$LOG" >"$PLAIN"
echo "gate: ${*:-full} (exit $code)"
grep -nE "$PATTERN" "$PLAIN" | head -n "$MAX_FAIL"
matched=$(grep -cE "$PATTERN" "$PLAIN")
[ "$matched" -gt "$MAX_FAIL" ] && echo "… $((matched - MAX_FAIL)) more matching lines in the log"
echo "--- last $TAIL lines ---"
tail -n "$TAIL" "$PLAIN"
rm -f "$PLAIN"
echo "--- full log ($(wc -l <"$LOG" | tr -d ' ') lines): $LOG"
exit $code
