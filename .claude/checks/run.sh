#!/bin/sh
# run.sh: run every process check in this directory, or the ones named.
#
#   sh .claude/checks/run.sh                 # all checks
#   sh .claude/checks/run.sh hooks roadmap   # just these (file names without .sh)
#   sh .claude/checks/run.sh openspec:project local:ledger   # a module's or the local layer's
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

dir=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$dir/../.." && pwd)
CHECKS_LIB="$dir/lib.sh"
export ROOT CHECKS_LIB
# A module's or the local layer's check sources CHECKS_LIB, whose plugin-build copy finds the
# model's libraries through CLAUDUCTOR_FW: pass it on when this runner has one.
[ -z "${CLAUDUCTOR_FW:-}" ] || export CLAUDUCTOR_FW

# The extension checks: "<name> <file>" per line. A module that cannot load is checks/modules.sh's
# failure, never a silently shorter list.
ext=""
if [ -f "$ROOT/.claude/lib/modules.sh" ]; then
  # shellcheck disable=SC1091
  . "$ROOT/.claude/lib/conf.sh"
  # shellcheck disable=SC1091
  . "$ROOT/.claude/lib/modules.sh"
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
  else
    echo "FAIL $n"
    printf '%s\n' "$out" | grep -v '^ok' | sed 's/^/    /'
    failed="$failed $n"
  fi
done

if [ -n "$failed" ]; then
  echo "checks: FAILED:$failed (of $count run, in $ROOT)"
  exit 1
fi
echo "checks: all $count passed (in $ROOT)"
