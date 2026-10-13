#!/bin/sh
# PreToolUse hook (matcher: Bash). Refuses a gate (GATE, GATE_RUN) or a gate wait (gate-wait.sh)
# that would end up where nobody hears it finish.
#
# WHY (OPS-41). A subagent is not re-invoked when its own background task finishes; only the main
# session is. A lane that waited on its own background gate went silent when the gate failed (two
# lanes stalled ~5 h on 2026-10-03, docs/insights-log.md), and "run the gate in the foreground" was
# already in the prompts: an instruction is documentation, not a control (AGENTS.md rule 4).
#
# WHO IS A SUBAGENT. Claude Code's hook input carries `agent_id` only inside a subagent or an
# in-process teammate (code.claude.com/docs/en/hooks, "common input fields"); the main session's
# payload has no such key. Standing Tee observed the same in a headless probe (2026-10-05): no key
# from the main thread, `agent_id` from a general-purpose subagent and from a Workflow agent.
#
# THREE DOORS TO THE BACKGROUND, for a call that INVOKES a gate (lib/gate-reader.awk: a gate in
# command position, not a mention in `cat`, `git log` or a commit message):
#   1. the shell's own `&` (`gate.sh &`, `nohup … &`, `( gate.sh ) &`): the call returns at once
#      and NOTHING waits for the gate, in a subagent or in the main session alike, so it is refused
#      for both (any unquoted `&` in a command that invokes a gate counts);
#   2. `run_in_background: true`, from a subagent;
#   3. a subagent's FOREGROUND call whose `timeout` is unset or under 600000 ms: the Bash tool moves
#      a command that outlives its timeout to the background (the default is 120 s; 600000 is the
#      tool's ceiling), and a subagent is not woken when that ends either.
# The main session's `run_in_background` and short timeouts are allowed: it is woken when they end.
# A full gate can still outlive 600 s; the tool then backgrounds it, and gate-wait.sh (named in the
# message) is how the agent follows it, in the foreground.
#
# KNOWN LIMIT: doors 2 and 3 rest on `agent_id`. A Claude Code that stopped sending it would make
# them allow everything, silently; door 1 does not depend on it.
#
# WITHOUT jq THIS HOOK FAILS CLOSED, for a subagent call that names a gate, and only then (as
# no-blind-source-rewrite does): the raw payload carries `"agent_id"` and a gate's file name, so it
# is refused, even a mention. Everything else is allowed: blocking every Bash call over a risk
# confined to one command would stop the session dead.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow. Checked by .claude/checks/hooks.sh.

payload=$(cat)

root=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd) || root=""
# From the clauductor plugin this file is not in the project: its GATE settings are under
# CLAUDE_PROJECT_DIR.
[ -n "${CLAUDE_PROJECT_DIR:-}" ] && [ -f "$CLAUDE_PROJECT_DIR/.claude/project.conf" ] && root=$CLAUDE_PROJECT_DIR
ROOT=$root
# A `.` of a missing file is fatal with a shell-dependent status (dash exits 2, which Claude Code
# reads as a BLOCK of every Bash call): test first, and fall back to the defaults.
if [ -f "$(dirname "$0")/../lib/conf.sh" ]; then
  # shellcheck disable=SC1091
  . "$(dirname "$0")/../lib/conf.sh"
fi
run=${GATE_RUN:-scripts/ci/run-local.sh}
wrap=${GATE:-scripts/ci/gate.sh}
waiter="$(dirname "$run")/gate-wait.sh"
gates="${run##*/} ${wrap##*/} gate-wait.sh"

HINT="Run it in the FOREGROUND, with the Bash timeout at 600000 and without '&'. If the tool still moves it to the background (a gate longer than 10 minutes), call $waiter in the foreground (timeout 600000) until it prints PASS or FAIL: exit 3 means still running, call it again. Never end your turn while a gate is pending."

block() { echo "BLOCKED by no-background-gate: $1" >&2; echo "$2" >&2; exit 2; }

if ! command -v jq >/dev/null 2>&1 || ! printf '%s' "$payload" | jq -e . >/dev/null 2>&1; then
  case "$payload" in *'"agent_id"'*) ;; *) exit 0 ;; esac
  for g in $gates; do
    case "$payload" in *"$g"*)
      block "jq could not read the hook payload (jq missing, or a malformed payload), and this subagent call names the gate ($g), so it is refused rather than let it run unwatched." \
        "Install jq (macOS: brew install jq; Debian/Ubuntu: apt-get install jq), check that 'jq --version' works, then retry. $HINT" ;;
    esac
  done
  exit 0
fi

cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // "" | tostring')
# The reader failing (missing, or an awk that cannot run it) leaves the raw text: a gate's file name
# anywhere in it counts as an invocation, broader than the reader and never narrower.
if ! verdict=$(printf '%s' "$cmd" | awk -v gates="$gates" -f "$(dirname "$0")/lib/gate-reader.awk" 2>/dev/null); then
  verdict=""
  for g in $gates; do case "$cmd" in *"$g"*) verdict=GATE ;; esac; done
fi
case "$verdict" in *GATE*) ;; *) exit 0 ;; esac

case "$verdict" in *AMP*)
  block "this command sends the gate to the shell's background with '&'. The call returns at once and nothing waits for the gate, so a failure is read by nobody." \
    "In the main session, use the Bash tool's run_in_background (you are woken when it ends); in a subagent: $HINT" ;;
esac

agent=$(printf '%s' "$payload" | jq -r '.agent_id // "" | tostring')
[ -n "$agent" ] || exit 0

if [ "$(printf '%s' "$payload" | jq -r '.tool_input.run_in_background // false | tostring')" = true ]; then
  block "a subagent started the gate with run_in_background. A subagent is not woken when its own background task ends, so the result would sit unread." "$HINT"
fi
# An unset timeout is the tool's default: BASH_DEFAULT_TIMEOUT_MS, which the hook inherits, else 120 s.
timeout=$(printf '%s' "$payload" | jq -r --arg d "${BASH_DEFAULT_TIMEOUT_MS:-120000}" \
  '.tool_input.timeout // ($d | tonumber? // 120000) | tonumber? // 0 | floor')
if [ "$timeout" -lt 600000 ]; then
  block "a subagent started the gate with the Bash timeout at $timeout ms. The tool moves a command that outlives its timeout to the background, and a subagent is not woken when that ends." "$HINT"
fi
exit 0
