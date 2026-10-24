#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
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
# The whole set runs ONCE, inside the no-clauductor environment (ADR-0006): `clauductor` off PATH, a
# fresh HOME with no panel, CI and CLAUDUCTOR_LOCK_HELD empty. So every check, the modules' and the
# local layer's included, runs without the caller's HOME (the gh login, ssh keys and git signing
# read from it) and without CI=true, and no-clauductor.sh asserts the environment instead of
# running the set again. Every other variable passes through: GH_TOKEN, SSH_AUTH_SOCK,
# GIT_CONFIG_GLOBAL and XDG_CONFIG_HOME still reach a check when the caller sets them. The run
# `exec`s itself into the environment, so it stays one process in the caller's foreground: an INT
# (Ctrl-C) or a TERM reaches it and its running check as before, stops the set, and removes the
# environment. A signal sent to run.sh's pid alone takes effect once the running check ends.
# Named runs are not wrapped: they run in the caller's environment, for working on one check.
# CHECKS_DOUBLE_PASS=1 restores the old double pass: no wrap, and no-clauductor.sh re-runs this
# directory's checks inside the environment. Markers every check inherits: CHECKS_SET=whole on
# every whole-set run, CHECKS_BARE (the environment's directory) inside it. A check that starts a
# whole-set run of its own clears them first (lib.sh's `unmarked`).

dir=$(cd "$(dirname "$0")" && pwd)
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
CHECKS_LIB="$dir/lib.sh"
export ROOT CHECKS_LIB
# A module's or the local layer's check sources CHECKS_LIB, whose plugin-build copy finds the
# model's libraries through CLAUDUCTOR_FW: pass it on when this runner has one.
[ -z "${CLAUDUCTOR_FW:-}" ] || export CLAUDUCTOR_FW

# The single pass (ADR-0006's 2026-10-18 amendment): a whole-set run builds the no-clauductor
# environment once (lib.sh's bare_env) in its scratch, and `exec`s itself inside it: one process,
# so INT and TERM reach the running check (a background child would ignore INT, and a foreground
# child would hold a TERM until the whole set ended). The `exec` drops lib.sh's traps, so the inner
# run is told the directory (CHECKS_BARE_RM) and removes it on EXIT, INT and TERM. CHECKS_BARE is
# the stop condition, and no-clauductor.sh checks the real state behind it. CHECKS_SET=whole tells
# no-clauductor.sh it runs under a whole-set run.
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
  # wrap: start (no-clauductor.sh's fixture deletes from here to `wrap: end` to remove the wrap)
  if [ "${CHECKS_DOUBLE_PASS:-}" != 1 ] && [ -z "${CHECKS_BARE:-}" ]; then
    use_lib
    # The exec window (OPS-121): a signal after the `exec` and before the inner run sets its traps
    # leaves the environment, and no shell can close that instant (a signal ignored across `exec`
    # cannot be trapped after it). So the run records its pid, which `exec` keeps, beside the
    # environment, and every whole-set run's start removes a scratch whose run is gone. Gone means
    # kill says "No such process": "Operation not permitted" is a live process, and a scratch with no
    # pid recorded (any check's, or a run before this line) is never touched.
    for _w in "${TMPDIR:-/tmp}"/checks.*/run.pid; do
      [ -f "$_w" ] || continue
      _wp=$(cat "$_w" 2>/dev/null)
      case $_wp in '' | *[!0-9]*) continue ;; esac
      LC_ALL=C sh -c 'kill -0 "$1"' kill "$_wp" 2>&1 | grep -q 'No such process' && rm -rf "${_w%/run.pid}"
    done
    echo $$ > "$_scratch/run.pid"
    bare_env
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
  fi
fi

# The extension checks: "<name> <file>" per line. A module that cannot load is checks/modules.sh's
# failure, never a silently shorter list.
ext=""
if [ -f "$CLAUDUCTOR_FW/lib/modules.sh" ]; then
  # shellcheck disable=SC1091
  . "$CLAUDUCTOR_FW/lib/conf.sh"
  # shellcheck disable=SC1091
  . "$CLAUDUCTOR_FW/lib/modules.sh"
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
# that runs two check sets over one model, as clauductor's own gate does). Named runs ignore it.
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

failed=""
count=0
for n in $names; do
  case $n in
    *:*) f=$(printf '%s\n' "$ext" | awk -v n="$n" '$1 == n { print $2; exit }') ;;
    *) f="$dir/$n.sh" ;;
  esac
  if [ -z "$f" ] || [ ! -f "$f" ]; then
    echo "FAIL $n: no such check (${f:-not in an enabled module or .claude/local/checks})"
    failed="$failed $n"
    continue
  fi
  count=$((count + 1))
  case $n in
    *:*) out=$(run_shebang "$f" 2>&1) ;;
    *) out=$(sh "$f" 2>&1) ;;
  esac
  rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "PASS $n ($(printf '%s\n' "$out" | grep -c '^ok'))"
    # A part a passing check could not run is named under its PASS line, never folded into the count.
    printf '%s\n' "$out" | grep '^ok   SKIPPED' | sed 's/^/    /'
  else
    echo "FAIL $n"
    printf '%s\n' "$out" | grep -v '^ok' | sed 's/^/    /'
    failed="$failed $n"
  fi
done

[ -z "$skipped" ] || echo "checks: left to another run by CHECKS_SKIP:$skipped"
if [ -n "$failed" ]; then
  echo "checks: FAILED:$failed (of $count run, in $ROOT)"
  exit 1
fi
# "without clauductor" only when this whole-set run is in the environment by its real state, never
# by the marker alone.
where=""
[ -z "$whole" ] || [ -z "${CHECKS_BARE:-}" ] || ! bare_assert >/dev/null || where=", without clauductor"
echo "checks: all $count passed (in $ROOT$where)"
