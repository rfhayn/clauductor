#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# run.sh: run every process check in this directory, or the ones named.
#
#   sh .claude/checks/run.sh                 # all checks
#   sh .claude/checks/run.sh hooks roadmap   # just these (file names without .sh)
#   sh .claude/checks/run.sh openspec:project local:ledger   # a module's or the local layer's
#   CHECKS_SKIP="gate hooks" sh .claude/checks/run.sh        # all but these: another run covers them
#
# A check is any executable-by-sh `*.sh` here other than run.sh and lib.sh. Each prints `ok`
# and `FAIL` lines and exits non-zero on any failure. The checks hold the operating model to
# what AGENTS.md says executes it: they are plain POSIX sh plus git and jq, so they run the same
# in any project whatever its language, and the project's gate should run this script as one of
# its steps (scripts/ci/steps.sh does).
#
# The list of checks is the directory itself, not a list typed here (*Enumerate the authority*):
# a new check runs the moment its file exists. The same holds for the enabled modules'
# `checks/*.sh` (named `<module>:<check>`) and the project's `.claude/local/checks/*.sh`
# (`local:<check>`), found by lib/modules.sh and run per their own #! line. Those source
# "$CHECKS_LIB" (this directory's lib.sh) for the helpers.
#
# The whole set runs ONCE, inside the no-stridal environment (ADR-0006): `stridal` off PATH, a
# fresh HOME with no panel, CI and STRIDAL_LOCK_HELD empty. So every check, the modules' and the
# local layer's included, runs without the caller's HOME (the gh login, ssh keys and git signing
# read from it) and without CI=true, and no-stridal.sh asserts the environment instead of
# running the set again. Every other variable passes through: GH_TOKEN, SSH_AUTH_SOCK,
# GIT_CONFIG_GLOBAL and XDG_CONFIG_HOME still reach a check when the caller sets them. The run
# `exec`s itself into the environment, so it stays one process in the caller's foreground: an INT
# (Ctrl-C) or a TERM reaches it, stops the set, and removes the environment. It TERMs every check
# running in the background, and their children, before it goes (the parallel phase, below); a
# serial check runs in its foreground, so a signal sent to run.sh's pid alone takes effect once
# that check ends.
# Named runs are not wrapped: they run in the caller's environment, for working on one check.
# CHECKS_DOUBLE_PASS=1 restores the old double pass: no wrap, and no-stridal.sh re-runs this
# directory's checks inside the environment. Markers every check inherits: CHECKS_SET=whole on
# every whole-set run, CHECKS_BARE (the environment's directory) inside it. A check that starts a
# whole-set run of its own clears them first (lib.sh's `unmarked`).

dir=$(cd "$(dirname "$0")" && pwd)
ROOT=$(case $STRIDAL_FW in (*/.claude) dirname "$STRIDAL_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
CHECKS_LIB="$dir/lib.sh"
export ROOT CHECKS_LIB
# A module's or the local layer's check sources CHECKS_LIB, whose plugin-build copy finds the
# model's libraries through STRIDAL_FW: pass it on when this runner has one.
[ -z "${STRIDAL_FW:-}" ] || export STRIDAL_FW

# The single pass (ADR-0006's 2026-10-18 amendment): a whole-set run builds the no-stridal
# environment once (lib.sh's bare_env) in its scratch, and `exec`s itself inside it: one process,
# so INT and TERM reach run.sh itself (a background run.sh would ignore INT, and a foreground child
# would hold a TERM until the whole set ended), which passes them on to its checks. The `exec` drops lib.sh's traps, so the inner
# run is told the directory (CHECKS_BARE_RM) and removes it on EXIT, INT and TERM. CHECKS_BARE is
# the stop condition, and no-stridal.sh checks the real state behind it. CHECKS_SET=whole tells
# no-stridal.sh it runs under a whole-set run.
# run.sh sources lib.sh for its helpers only: lib.sh's CONTEXT_OFFLINE=1 is for the checks that
# source it, so the caller's value (or its absence) is put back, and a check run by its own #! line
# sees what it would see unwrapped.
use_lib() {
  _co=${CONTEXT_OFFLINE-run.sh:unset}
  # shellcheck disable=SC1090
  . "$CHECKS_LIB"
  if [ "$_co" = run.sh:unset ]; then unset CONTEXT_OFFLINE; else CONTEXT_OFFLINE=$_co; fi
}
whole=""
if [ $# -eq 0 ]; then
  whole=1
  CHECKS_SET=whole; export CHECKS_SET
  # wrap: start (no-stridal.sh's fixture deletes from here to `wrap: end` to remove the wrap)
  if [ "${CHECKS_DOUBLE_PASS:-}" != 1 ] && [ -z "${CHECKS_BARE:-}" ]; then
    use_lib
    # The exec window (OPS-121): a signal after the `exec` and before the inner run sets its traps
    # leaves the environment, and no shell can close that instant (a signal ignored across `exec`
    # cannot be trapped after it). So the run records its pid, which `exec` keeps, beside the
    # environment, and the inner run deletes it once its traps exist: a run.pid still there is a run
    # that died inside the window or is about to leave it. Every whole-set run's start removes such a
    # scratch once its run is gone. Gone means kill says "no such process" (in any case: zsh and ksh
    # say it lower case); "Operation not permitted" is a live process, a pid in another pid namespace
    # (a Linux sandbox) can read as gone but only while its run is still in the instant before its
    # traps, and a scratch with no pid recorded (any check's, or a live run's) is never touched.
    for _w in "${TMPDIR:-/tmp}"/checks.*/run.pid; do
      [ -f "$_w" ] || continue
      _wp=$(cat "$_w" 2>/dev/null)
      case $_wp in '' | *[!0-9]*) continue ;; esac
      LC_ALL=C sh -c 'kill -0 "$1"' kill "$_wp" 2>&1 | grep -qi 'no such process' && rm -rf "${_w%/run.pid}"
    done
    bare_env
    # Recorded only now: until the exec, lib.sh's own traps remove the scratch.
    echo $$ > "$_scratch/run.pid"
    bare_exec CHECKS_BARE="$BARE_DIR" CHECKS_BARE_RM="$_scratch" sh "$dir/run.sh"
  fi
  # wrap: end
  if [ -n "${CHECKS_BARE:-}" ]; then
    # For bare_assert, which the summary line uses. The inner run takes the outer run's scratch (the
    # environment) as its own, so lib.sh's traps remove it, and unsets the name before any check
    # runs: a nested whole-set run must never remove its parent's environment. Only the directory
    # that holds CHECKS_BARE is taken, never another a caller names. An INT or a TERM removes it, then
    # kills this run with the same signal, so a caller sees it die of it and stops too.
    use_lib
    if [ -n "${CHECKS_BARE_RM:-}" ] && [ "$CHECKS_BARE" = "$CHECKS_BARE_RM/bare" ]; then
      rmdir "$_scratch"; _scratch=$CHECKS_BARE_RM
    fi
    unset CHECKS_BARE_RM
    trap 'rm -rf "$_scratch"; trap - INT; kill -INT $$' INT
    trap 'rm -rf "$_scratch"; trap - TERM; kill -TERM $$' TERM
    # Out of the exec window: from here these traps remove the environment, so no sweep may.
    rm -f "$_scratch/run.pid"
  fi
fi

# The extension checks: "<name> <file>" per line. A module that cannot load is checks/modules.sh's
# failure, never a silently shorter list.
ext=""
if [ -f "$STRIDAL_FW/lib/modules.sh" ]; then
  # shellcheck disable=SC1091
  . "$STRIDAL_FW/lib/conf.sh"
  # shellcheck disable=SC1091
  . "$STRIDAL_FW/lib/modules.sh"
  # A function, not inline in $(…): a case pattern's lone `)` there trips bash's parser.
  ext_checks() {
    ext_files checks .sh | while IFS="$(printf '\t')" read -r label f; do
      p=${label#module }
      echo "$p:$(basename "$f" .sh) $f"
    done
  }
  ext=$(ext_checks)
fi

if [ $# -gt 0 ]; then
  names=$*
else
  names="$(cd "$dir" && ls ./*.sh | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib') $(printf '%s\n' "$ext" | cut -d' ' -f1)"
fi
# CHECKS_SKIP: the checks a whole-set run leaves out because another run covers them (a repository
# that runs two check sets over one model, as stridal's own gate does). Named runs ignore it.
# Read once and unset, so no check, and no fixture run a check starts, inherits it; every check it
# left out is named below, before the verdict.
skipped=""
if [ $# -eq 0 ] && [ -n "${CHECKS_SKIP:-}" ]; then
  kept=""
  for n in $names; do
    case " $CHECKS_SKIP " in *" $n "*) skipped="$skipped $n" ;; *) kept="$kept $n" ;; esac
  done
  names=$kept
fi
unset CHECKS_SKIP

# ── Parallel (OPS-175) ──────────────────────────────────────────────────────────────────────────
# The checks run CHECKS_JOBS at a time (default: the processor count), each one's output kept in
# its own file and printed in the order above, so the report reads as a serial run's. A check that
# shares state with another (a fixed path, a port, a socket, a lease, the shared HOME, a process
# table it scans, or a signal it must receive in the foreground) says so in a line of its own:
#
#   # run.sh: serial (<the state it shares>)
#
# and runs alone, in this shell's foreground, after the parallel ones, so it overlaps no other
# check and keeps INT (a background job starts with INT ignored, and no sh can undo that). Which
# checks declare it is found by reading what each writes (checks/run-parallel.sh holds a pair that
# fails in parallel without it). The declaration is the file's, so a new check that needs it
# carries it, as the directory is the list. Only the leading comment block is read, so a fixture
# written by a heredoc further down never declares its writer. Nothing checks that a check sharing
# state declares it: the reading is the control (Nothing. You.).
[ -n "${_scratch:-}" ] || use_lib
RES="$_scratch/results"; mkdir -p "$RES"
# The default is the processor count, at most 8: a check's own fixture runs of run.sh take the same
# default again, so load multiplies.
jobs=${CHECKS_JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null)}
case $jobs in '' | *[!0-9]* | 0) jobs=4 ;; esac
[ -n "${CHECKS_JOBS:-}" ] || [ "$jobs" -le 8 ] || jobs=8
# kill_tree PID: TERM PID's descendants, then PID. A background check starts with INT ignored and
# holds a TERM until its foreground child ends, so the children are told first. By pid, never by
# pattern; where ps cannot run (a sandbox) only PID is told.
kill_tree() {
  for _c in $(ps -A -o pid= -o ppid= 2>/dev/null | awk -v p="$1" '$2 == p { print $1 }'); do
    kill_tree "$_c"
  done
  kill -TERM "$1" 2>/dev/null
}
# stop_jobs: for a run stopped by INT or TERM. Every check still running in the background is told
# (and its children); a launch that has not yet recorded its check's pid is TERMed itself, before
# it starts one. Then every launch is waited for, so none writes into the scratch the trap removes.
# Waiting launches never start: the scheduler is this shell.
stop_jobs() {
  for _w in "$RES"/*.wpid; do
    [ -f "$_w" ] || continue
    _i=${_w%.wpid}
    [ -f "$_i.done" ] && continue
    if [ -s "$_i.pid" ]; then kill_tree "$(cat "$_i.pid")"; else kill -TERM "$(cat "$_w")" 2>/dev/null; fi
  done
  wait
}
trap 'stop_jobs; rm -rf "$_scratch"; trap - INT; kill -INT $$' INT
trap 'stop_jobs; rm -rf "$_scratch"; trap - TERM; kill -TERM $$' TERM

failed=""
count=0
i=0
for n in $names; do
  i=$((i + 1))
  case $n in
    *:*) f=$(printf '%s\n' "$ext" | awk -v n="$n" '!got && $1 == n { print $2; got = 1 }') ;;
    *) f="$dir/$n.sh" ;;
  esac
  if [ -z "$f" ] || [ ! -f "$f" ]; then k=missing
  elif sed -n '1,/^[^#]/p' "$f" | grep -q '^# run.sh: serial'; then k=serial
  else k=parallel
  fi
  eval "cn_$i=\$n cf_$i=\$f ck_$i=\$k"
done
total=$i

# launch I: start check I in the background; its status lands in $RES/I.rc, then I.done marks it
# finished (a rename, so a reader never sees half a status), and its number goes to fd 4.
launch() {
  eval "_n=\$cn_$1 _f=\$cf_$1"
  (
    case $_n in
      *:*)
        _ri=$(shebang_interp "$_f")
        if command -v "${_ri%% *}" >/dev/null 2>&1; then
          # shellcheck disable=SC2086
          $_ri "$_f" > "$RES/$1.out" 2>&1 4>&- &
        else
          echo "CANNOT CHECK — $_f needs ${_ri%% *} (its #! line), which is not installed" > "$RES/$1.out"
          (exit 127) &
        fi ;;
      *) sh "$_f" > "$RES/$1.out" 2>&1 4>&- & ;;
    esac
    echo $! > "$RES/$1.pid.w" && mv "$RES/$1.pid.w" "$RES/$1.pid"
    wait $!
    echo $? > "$RES/$1.rc"
    : > "$RES/$1.done.w" && mv "$RES/$1.done.w" "$RES/$1.done"
    echo "$1" >&4
  ) &
  echo $! > "$RES/$1.wpid"
}

# report I [RC]: print check I's result, once it has one.
report() {
  eval "_n=\$cn_$1 _f=\$cf_$1 _k=\$ck_$1"
  if [ "$_k" = missing ]; then
    echo "FAIL $_n: no such check (${_f:-not in an enabled module or .claude/local/checks})"
    failed="$failed $_n"
    return
  fi
  count=$((count + 1))
  out=$(cat "$RES/$1.out")
  rc=${2:-$(cat "$RES/$1.rc")}
  if [ "$rc" -eq 0 ]; then
    echo "PASS $_n ($(printf '%s\n' "$out" | grep -c '^ok'))"
    # A part a passing check could not run is named under its PASS line, never folded into the count.
    printf '%s\n' "$out" | grep '^ok   SKIPPED' | sed 's/^/    /'
  else
    echo "FAIL $_n"
    printf '%s\n' "$out" | grep -v '^ok' | sed 's/^/    /'
    failed="$failed $_n"
  fi
}

# The parallel phase: keep `jobs` running; print each result once every check before it has been
# printed, up to the first serial one.
# A run that cannot make its fifo stops: without one it could not tell when a check ends, and a
# serial check could start beside a running one.
mkfifo "$RES/fifo" || { echo "FAIL run.sh: cannot make the fifo $RES/fifo the parallel phase needs"; exit 1; }
exec 4<>"$RES/fifo"
next=1 running=0 shown=1
while :; do
  while [ "$running" -lt "$jobs" ] && [ "$next" -le "$total" ]; do
    eval "_k=\$ck_$next"
    [ "$_k" = parallel ] && { launch "$next"; running=$((running + 1)); }
    next=$((next + 1))
  done
  while [ "$shown" -le "$total" ]; do
    eval "_k=\$ck_$shown"
    [ "$_k" = serial ] && break
    [ "$_k" = missing ] || [ -f "$RES/$shown.done" ] || break
    report "$shown"; shown=$((shown + 1))
  done
  [ "$running" -gt 0 ] || break
  read -r _d <&4 || break
  running=$((running - 1))
done
exec 4>&-
# A serial check never starts beside a parallel one, whatever ended the loop.
wait
# The serial phase: each declared check alone, in the foreground; the rest printed in order around it.
while [ "$shown" -le "$total" ]; do
  eval "_k=\$ck_$shown _f=\$cf_$shown _n=\$cn_$shown"
  if [ "$_k" = serial ]; then
    case $_n in
      *:*) run_shebang "$_f" > "$RES/$shown.out" 2>&1 ;;
      *) sh "$_f" > "$RES/$shown.out" 2>&1 ;;
    esac
    report "$shown" $?
  else
    report "$shown"
  fi
  shown=$((shown + 1))
done
[ -z "$skipped" ] || echo "checks: left to another run by CHECKS_SKIP:$skipped"
if [ -n "$failed" ]; then
  echo "checks: FAILED:$failed (of $count run, in $ROOT)"
  exit 1
fi
# "without stridal" only when this whole-set run is in the environment by its real state, never
# by the marker alone.
where=""
[ -z "$whole" ] || [ -z "${CHECKS_BARE:-}" ] || ! bare_assert >/dev/null || where=", without stridal"
echo "checks: all $count passed (in $ROOT$where)"
