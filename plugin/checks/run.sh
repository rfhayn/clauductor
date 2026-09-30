#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# run.sh: run every process check in this directory, or the ones named.
#
#   sh .claude/checks/run.sh                 # all checks
#   sh .claude/checks/run.sh hooks roadmap   # just these (file names without .sh)
#
# A check is any executable-by-sh `*.sh` here other than run.sh and lib.sh. Each prints `ok`
# and `FAIL` lines and exits non-zero on any failure. The checks hold the operating model to
# what AGENTS.md says executes it: they are plain POSIX sh plus git and jq, so they run the same
# in any project whatever its language, and the project's gate should run this script as one of
# its steps (scripts/ci/steps.sh does).
#
# The list of checks is the directory itself, not a list typed here (*Enumerate the authority*):
# a new check runs the moment its file exists.

dir=$(cd "$(dirname "$0")" && pwd)
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
export ROOT

if [ $# -gt 0 ]; then
  names=$*
else
  names=$(cd "$dir" && ls ./*.sh | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib')
fi

failed=""
count=0
for n in $names; do
  f="$dir/$n.sh"
  if [ ! -f "$f" ]; then
    echo "FAIL $n: no such check ($f)"
    failed="$failed $n"
    continue
  fi
  count=$((count + 1))
  out=$(sh "$f" 2>&1)
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
