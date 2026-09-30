#!/usr/bin/env bash
set -euo pipefail
# Serialise the full gate across every worktree of this repo.
lock=""
if common=$(git rev-parse --git-common-dir 2>/dev/null); then
  case $common in /*) ;; *) common="$PWD/$common" ;; esac
  lock="$common/clauductor/gate.lock"
fi
lane="${CLAUDUCTOR_LANE:-$(basename "$PWD")}"
if [ -z "$lock" ]; then
  echo "gate: not in a git checkout; running without the gate queue" >&2
elif [ "${CLAUDUCTOR_LOCK_HELD:-}" != "$lock" ]; then
  if command -v clauductor >/dev/null 2>&1; then
    # TERM=dumb skips a terminal query at clauductor's startup (up to 5 s on a
    # pty that does not answer); lock-run gives the gate the real TERM back, or
    # leaves it unset if it was unset. Ask the environment, not the shell: bash
    # sets an unexported TERM=dumb of its own when TERM is unset.
    term_set="" term_val=""
    if printenv TERM >/dev/null 2>&1; then term_set=1 term_val=$(printenv TERM); fi
    CLAUDUCTOR_TERM="$term_val" CLAUDUCTOR_TERM_SET="$term_set" TERM=dumb \
      exec clauductor lock-run --lane "$lane" "$lock" -- bash "$0" "$@"
  fi
  if [ -f "$(dirname "$0")/lease.sh" ]; then
    . "$(dirname "$0")/lease.sh"        # the plain-shell protocol below
    lease_run "$lock" "$lane" bash "$0" "$@" && exit 0 || exit $?
  fi
  echo "gate: neither clauductor nor lease.sh found; running without the gate queue" >&2
fi
# ── Above: the clauductor docs/panel.md "In a project's gate script" snippet, verbatim. ─────
#
# run-local.sh: THE full gate (GATE_RUN in .claude/project.conf). Runs every step in GATE_STEPS
# and, only after a COMPLETE run of a CLEAN tree, writes the receipt pr-merge-guard reads:
#     $(git rev-parse --git-dir)/ci-receipt   =   <sha> TAB full TAB clean|dirty TAB all
#
#   scripts/ci/run-local.sh            the full gate on the committed tree: writes the receipt
#   scripts/ci/run-local.sh --dirty    the working tree as it stands (receipt marked dirty: refused)
#   scripts/ci/run-local.sh --quick    the quick steps only: writes no receipt, deletes none
#
# Agents run it through scripts/ci/gate.sh (GATE), which keeps the log in a file.
#
# WHAT "CLEAN" MEANS. The receipt names a commit, so it may only vouch for exactly that commit.
# GATE_CLEAN_ROOM (project.conf) chooses how:
#   "archive": the steps run in a temp directory holding `git archive <sha>`: tracked files only,
#              nothing untracked, nothing built locally. The strongest form; your steps must then
#              install what they need (npm ci, go mod download, pip install …).
#   "none" (default): the steps run in this checkout, and the receipt says `clean` only when the
#              tree had no changes and no untracked files before AND after the run, and HEAD did not
#              move. Otherwise it says `dirty`, which the guard refuses.
# The SHA is resolved BEFORE the run: a receipt for a commit made during the run would vouch for
# code the run never executed.
#
# A failed FULL run deletes any receipt (a stale pass must not outlive a fail); a --quick run never
# spoke for the whole gate, so it neither writes nor deletes.
#
# Two steps are the operating model's own and run before GATE_STEPS in every gate, whatever the
# project's steps.sh says (it is the project's to edit; these are not):
#   scenario trace  every enforced scenario is cited by a test (.claude/scenario-trace.sh --check;
#                   in the clean room, at the tested commit);
#   secrets         gitleaks over the tracked files and the branch's commits. Without gitleaks the
#                   step says SKIPPED and why; with CI set (CI=true, as every hosted CI sets it) a
#                   missing gitleaks FAILS, so a remote gate cannot pass without the scan.

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"

DIRTY=0 MODE=full
while [ $# -gt 0 ]; do
  case "$1" in
    --dirty) DIRTY=1 ;;
    --quick) MODE=quick ;;
    -h|--help) sed -n '/^# run-local.sh:/,/^# spoke/p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "run-local: unknown flag $1 (see --help)" >&2; exit 64 ;;
  esac
  shift
done

GIT_DIR_ABS=$(git rev-parse --absolute-git-dir)
RECEIPT="$GIT_DIR_ABS/ci-receipt"
TESTED_SHA=$(git rev-parse HEAD)
porcelain() { git status --porcelain --untracked-files=normal 2>/dev/null; }
START_DIRTY=$([ -n "$(porcelain)" ] && echo 1 || echo 0)

[ -f "$ROOT/$GATE_STEPS" ] || { echo "==> FAIL: GATE_STEPS $GATE_STEPS does not exist" >&2; exit 1; }
# shellcheck disable=SC1090
. "$ROOT/$GATE_STEPS"

FAILED=""
# The secret scan. Tracked files only (an ignored .env on this machine is not a leak), as they are in
# the tree under test, plus the commits this branch adds (a secret committed and then deleted is
# still in the history a push publishes).
secret_scan() {
  if ! command -v gitleaks >/dev/null 2>&1; then
    case "${CI:-}" in
      ''|false|0) echo "secrets: SKIPPED — gitleaks is not installed, so NO secret scan ran (brew install gitleaks, or see github.com/gitleaks/gitleaks). Under CI this fails."; return 0 ;;
      *) echo "secrets: FAIL — gitleaks is not installed, and CI=$CI requires the scan"; return 1 ;;
    esac
  fi
  local src=. tmpd=""
  if [ -d .git ] || [ -f .git ]; then
    tmpd=$(mktemp -d "${TMPDIR:-/tmp}/secrets.XXXXXX")
    git ls-files -z | xargs -0 tar -cf - 2>/dev/null | tar -xf - -C "$tmpd" 2>/dev/null
    src=$tmpd
  fi
  local rc=0
  if gitleaks dir --help >/dev/null 2>&1; then gitleaks dir --no-banner --redact "$src" || rc=$?
  else gitleaks detect --no-git --no-banner --redact --source "$src" || rc=$?; fi
  [ -n "$tmpd" ] && rm -rf "$tmpd"
  if [ "$rc" -eq 0 ] && { [ -d .git ] || [ -f .git ]; } && git rev-parse -q --verify "origin/$MAIN_BRANCH" >/dev/null 2>&1; then
    local range; range="$(git merge-base "origin/$MAIN_BRANCH" HEAD 2>/dev/null)..HEAD"
    if gitleaks git --help >/dev/null 2>&1; then gitleaks git --no-banner --redact --log-opts="$range" . || rc=$?
    else gitleaks detect --no-banner --redact --log-opts="$range" --source . || rc=$?; fi
  fi
  return "$rc"
}
model_steps() {
  local rev=""
  [ "${GATE_CLEAN_ROOM:-none}" = archive ] && [ "$DIRTY" -eq 0 ] && rev="--rev $TESTED_SHA"
  # shellcheck disable=SC2086
  step "scenario trace" sh "$ROOT/.claude/scenario-trace.sh" --check $rev || return 1
  step "secrets" secret_scan || return 1
}
step() {  # step NAME COMMAND [ARGS...]: one gate step, marked for the agent-facing filter.
  local name=$1; shift
  echo "==> $name"
  if "$@"; then echo "==> ok: $name"; else local rc=$?; echo "==> FAIL: $name (exit $rc)"; FAILED="$FAILED $name"; return 1; fi
}

workdir=$ROOT
cleanup() { :; }
if [ "${GATE_CLEAN_ROOM:-none}" = archive ] && [ "$DIRTY" -eq 0 ]; then
  workdir=$(mktemp -d "${TMPDIR:-/tmp}/gate.XXXXXX")
  cleanup() { rm -rf "$workdir"; }
  trap cleanup EXIT
  git archive "$TESTED_SHA" | tar -x -C "$workdir"
  echo "==> clean room: $(printf %.9s "$TESTED_SHA") from git archive, in $workdir"
fi

echo "==> gate $MODE on $(printf %.9s "$TESTED_SHA") ($(git branch --show-current 2>/dev/null || echo detached))"
code=0
( cd "$workdir" && model_steps && gate_steps "$MODE" ) || code=$?

state=clean
if [ "$DIRTY" -eq 1 ]; then state=dirty
elif [ "${GATE_CLEAN_ROOM:-none}" != archive ]; then
  { [ "$START_DIRTY" -eq 1 ] || [ -n "$(porcelain)" ] || [ "$(git rev-parse HEAD)" != "$TESTED_SHA" ]; } && state=dirty
fi

if [ "$code" -eq 0 ]; then
  echo
  echo "==> PASS ($MODE) on $(printf %.9s "$TESTED_SHA")"
  if [ "$MODE" = full ]; then
    printf '%s\tfull\t%s\tall\n' "$TESTED_SHA" "$state" > "$RECEIPT"
    echo "    receipt: $RECEIPT ($state)"
    # Said HERE, at the end, where the verdict is read: a note at the top of a long log is not read.
    [ "$state" = clean ] || echo "    !!  The tree tested was NOT the commit (uncommitted or untracked files, --dirty, or HEAD moved): the merge guard refuses this receipt. Commit and re-run."
  else
    echo "    NO receipt: a --quick run is not the full gate. Run with no flags for evidence the merge guard accepts."
  fi
else
  echo
  echo "==> FAIL ($MODE):$FAILED"
  if [ "$MODE" = full ] && [ -f "$RECEIPT" ]; then rm -f "$RECEIPT"; echo "    removed $RECEIPT: a stale pass must not outlive a fail"; fi
fi
exit "$code"
