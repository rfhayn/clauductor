#!/bin/sh
# gate-wait.sh: wait, in the FOREGROUND, for the gate already started in this checkout, and say PASS
# or FAIL for the tree in front of you. It never starts a gate.
#
# WHY (OPS-41). A subagent is not woken when its own background task ends; only the main session is.
# A lane that left its gate in the background and ended its turn "waiting for the completion notice"
# sat silent on a red gate (two lanes, ~5 h, 2026-10-03). .claude/hooks/no-background-gate.sh refuses
# a subagent's background gate, but a FOREGROUND full gate can outlive the Bash tool's 10-minute
# ceiling, and the tool then moves it to the background itself. This is what the agent runs next:
# each call waits at most GATE_WAIT_SECONDS, under that ceiling, and says whether to call again.
#
#   scripts/ci/gate-wait.sh        from the checkout whose gate is running; Bash timeout 600000
#
# Exit codes:
#   0  PASS  the latest gate here exited 0 on this tree (or, with no state at all, the tree is clean
#            and the receipt names HEAD)
#   1  FAIL  the latest gate here failed on this tree, or was interrupted before its verdict
#   3  STILL RUNNING: call again
#   4  NO RESULT: no gate has run here, one died without a verdict, or the last one tested another
#      tree (another commit, or uncommitted changes made since it started)
#   2  not inside a git repository, or lib/gate-state.sh is missing
#
# WHAT IT READS, all in THIS checkout's git dir (a linked worktree has its own): ci-gate.state, which
# run-local.sh writes through lib/gate-state.sh (its header has the format), and ci-receipt.
# THE SUBJECT IS THE TREE, not HEAD: the inner loop gates uncommitted work whose HEAD does not move
# between rounds, so a verdict keyed to HEAD would read an old green as a PASS for an edited tree. A
# clean-room run (`subject=commit`) is HEAD's verdict whatever else is on disk, and says so.
# A RUNNING gate is awaited even when a receipt already names HEAD: the caller asked for the result of
# the run it started.
#
# GATE_WAIT_SECONDS (default 540) bounds one call; GATE_WAIT_POLL (default 5) is the look interval.
# Checked by .claude/checks/gate.sh.

GD=$(git rev-parse --absolute-git-dir 2>/dev/null) || { echo "gate-wait: not inside a git repository" >&2; exit 2; }
LIB="$(dirname "$0")/lib/gate-state.sh"
[ -f "$LIB" ] || { echo "gate-wait: $LIB is missing, so the gate's state cannot be read (restore scripts/ci from the template)" >&2; exit 2; }
# shellcheck disable=SC1090
. "$LIB"
STATE="$GD/ci-gate.state"
RECEIPT="$GD/ci-receipt"
LOG="$GD/ci-gate.log"
# Whole seconds only: anything else would never compare as elapsed, and the call would never end.
LIMIT=${GATE_WAIT_SECONDS:-540}
case $LIMIT in ''|*[!0-9]*) echo "gate-wait: GATE_WAIT_SECONDS=$LIMIT is not whole seconds; using 540" >&2; LIMIT=540 ;; esac
POLL=${GATE_WAIT_POLL:-5}
case $POLL in ''|*[!0-9]*|0) echo "gate-wait: GATE_WAIT_POLL=$POLL is not whole seconds; using 5" >&2; POLL=5 ;; esac
TOP=$(git rev-parse --show-toplevel)
GATE_CMD=$( [ -f "$TOP/.claude/project.conf" ] && . "$TOP/.claude/project.conf" >/dev/null 2>&1; printf '%s' "${GATE:-scripts/ci/gate.sh}")
HEAD_SHA=$(git rev-parse -q --verify HEAD 2>/dev/null || echo none)
CUR=$(gate_state_tree)
short() { printf '%.9s' "$1"; }
if [ "$CUR" = "$HEAD_SHA" ]; then SUBJECT="HEAD $(short "$HEAD_SHA")"
else SUBJECT="HEAD $(short "$HEAD_SHA") + uncommitted changes (tree $(short "$CUR"))"; fi

# receipt_for_head: the receipt line when it names HEAD; fails otherwise.
receipt_for_head() {
  [ -f "$RECEIPT" ] || return 1
  _r=$(head -n 1 "$RECEIPT")
  [ "$(printf '%s' "$_r" | cut -f1)" = "$HEAD_SHA" ] || return 1
  printf '%s\n' "$_r"
}

pass() {  # pass MODE
  if [ "$1" = quick ]; then
    echo "PASS (--quick) for $SUBJECT. A quick run writes no receipt: the merge guard needs a full one."
  elif r=$(receipt_for_head) && [ "$(printf '%s' "$r" | cut -f3)" = clean ]; then
    echo "PASS for $SUBJECT. Receipt: $RECEIPT ($(printf '%s' "$r" | tr '\t' ' '))"
  elif [ -n "${r:-}" ]; then
    echo "PASS for $SUBJECT, but the receipt says $(printf '%s' "$r" | cut -f3): the tree tested was not the commit, and the merge guard refuses it. Commit, then run the gate again."
  else
    echo "PASS for $SUBJECT, with no receipt naming HEAD (HEAD moved after the run, or the receipt was removed)."
  fi
  exit 0
}

# nothing WHY: no verdict for this tree. A clean tree whose HEAD has a clean receipt still passes.
nothing() {
  if [ "$CUR" = "$HEAD_SHA" ] && r=$(receipt_for_head) && [ "$(printf '%s' "$r" | cut -f3)" = clean ]; then
    echo "PASS for $SUBJECT, on its receipt ($1). Receipt: $RECEIPT ($(printf '%s' "$r" | tr '\t' ' '))"
    exit 0
  fi
  echo "NO RESULT for $SUBJECT: $1. Start a gate in the FOREGROUND: $GATE_CMD (Bash timeout 600000)." >&2
  exit 4
}

# alive PID SCRIPT: the gate's process is still running. Where ps works, its command line must name
# SCRIPT (a dead gate's pid can be reused). Where it does not (a sandbox), kill -0 tells a gone pid
# ("No such process") from one this shell may not signal, which counts as alive: the call is bounded
# either way, so an unverifiable pid costs a "call again", never a hang.
alive() {
  if ps -p $$ -o command= >/dev/null 2>&1; then
    case "$(ps -p "$1" -o command= 2>/dev/null)" in *"$2"*) return 0 ;; *) return 1 ;; esac
  fi
  _k=$(kill -0 "$1" 2>&1) && return 0
  case "$_k" in *[Ss]uch\ process*) return 1 ;; esac
  return 0
}

start=$(date +%s)
while :; do
  [ -f "$STATE" ] || nothing "no gate has run in this checkout ($STATE does not exist)"
  first=$(head -n 1 "$STATE")
  case "$first" in "started "*) ;; *) nothing "$STATE has no start line" ;; esac
  pid=$(gate_state_field "$first" pid)
  mode=$(gate_state_field "$first" mode)
  ex=$(grep "^exit pid=$pid " "$STATE" | tail -n 1)
  if [ -n "$ex" ]; then
    code=$(gate_state_field "$ex" code)
    head=$(gate_state_field "$first" head)
    tree=$(gate_state_field "$first" tree)
    match=""
    if [ "$(gate_state_field "$first" subject)" = commit ]; then
      if [ "$head" = "$HEAD_SHA" ]; then
        match=1
        [ "$CUR" = "$HEAD_SHA" ] || SUBJECT="HEAD $(short "$HEAD_SHA") (a clean-room run of the commit: your uncommitted changes were not tested)"
      fi
    elif [ "$tree" = "$CUR" ] && [ "$tree" != unknown ]; then
      match=1
    fi
    if [ -n "$match" ]; then
      [ "$code" = 0 ] && pass "$mode"
      if [ "$code" = signal ]; then echo "FAIL for $SUBJECT: the gate was interrupted (a signal) before its verdict. Run it again in the foreground: $GATE_CMD (Bash timeout 600000)." >&2
      else echo "FAIL for $SUBJECT: the gate exited $code. Its failure lines, when it ran through $GATE_CMD: grep -nE '^==> ' $LOG" >&2; fi
      exit 1
    fi
    nothing "the last gate in this checkout tested $([ "$head" = "$HEAD_SHA" ] && echo "an earlier state of this tree (tree $(short "$tree"))" || echo "HEAD $(short "$head")"), not this one, and exited $code"
  fi
  script=$(gate_state_field "$first" script)
  alive "$pid" "${script:-run-local.sh}" || nothing "the gate (pid $pid) is no longer running and wrote no verdict: it was killed or crashed"
  elapsed=$(( $(date +%s) - start ))
  if [ "$elapsed" -ge "$LIMIT" ]; then
    echo "STILL RUNNING after ${elapsed}s (pid $pid, $mode, testing HEAD $(short "$(gate_state_field "$first" head)"); this is $SUBJECT). Call $0 again, in the foreground (Bash timeout 600000)."
    [ -f "$LOG" ] && echo "  last line of $LOG: $(tail -n 1 "$LOG" | cut -c1-200)"
    exit 3
  fi
  sleep "$POLL"
done
