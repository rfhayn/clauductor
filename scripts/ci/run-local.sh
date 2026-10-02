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
# project's steps.sh says (it is the project's to edit; these are not). They live in lib/steps.sh,
# which a project's own runner can source to run the same steps:
#   scenario trace  every enforced scenario is cited by a test (.claude/scenario-trace.sh --check;
#                   in the clean room, at the tested commit);
#   secrets         gitleaks over the tracked files and the branch's commits. Without gitleaks the
#                   step says SKIPPED and why; with CI set (CI=true, as every hosted CI sets it) a
#                   missing gitleaks FAILS, so a remote gate cannot pass without the scan.

HERE=$(cd "$(dirname "$0")" && pwd)
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
step() {  # step NAME COMMAND [ARGS...]: one gate step, marked for the agent-facing filter.
  local name=$1; shift
  echo "==> $name"
  if "$@"; then echo "==> ok: $name"; else local rc=$?; echo "==> FAIL: $name (exit $rc)"; FAILED="$FAILED $name"; return 1; fi
}
# The model's own steps (scenario trace, secrets) are a library a project's own runner can call
# too: lib/steps.sh beside this file. Sourced after step() above, so they record into FAILED.
[ -f "$HERE/lib/steps.sh" ] || { echo "==> FAIL: $HERE/lib/steps.sh (the model's gate steps) is missing" >&2; exit 1; }
# shellcheck disable=SC1091
. "$HERE/lib/steps.sh"
gate_model_steps() {
  if [ "${GATE_CLEAN_ROOM:-none}" = archive ] && [ "$DIRTY" -eq 0 ]; then model_steps "$TESTED_SHA"; else model_steps; fi
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
( cd "$workdir" && gate_model_steps && gate_steps "$MODE" ) || code=$?

state=clean
if [ "$DIRTY" -eq 1 ]; then state=dirty
elif [ "${GATE_CLEAN_ROOM:-none}" != archive ]; then
  { [ "$START_DIRTY" -eq 1 ] || [ -n "$(porcelain)" ] || [ "$(git rev-parse HEAD)" != "$TESTED_SHA" ]; } && state=dirty
fi
# Did the steps run exactly TESTED_SHA's tree? A failure says something about that commit only then:
# a clean room of it, or this checkout unchanged at the start and HEAD unmoved (files a failing
# step leaves behind do not change what it ran). Otherwise a red would land on a commit never run.
tested_commit=0
if [ "$DIRTY" -eq 0 ]; then
  if [ "${GATE_CLEAN_ROOM:-none}" = archive ]; then tested_commit=1
  elif [ "$START_DIRTY" -eq 0 ] && [ "$(git rev-parse HEAD)" = "$TESTED_SHA" ]; then tested_commit=1; fi
fi

# Still the lease's holder? A gate that lost its lease mid-run may have run beside another gate (two
# gates binding one port fail each other falsely, or pass falsely), so its result cannot count:
# lease.sh's lease_verify, by the nonce lease_run exported or, under lock-run, by the holder's pid.
if [ "$code" -eq 0 ] && [ -n "${CLAUDUCTOR_LOCK_HELD:-}" ] && [ -f "$HERE/lease.sh" ]; then
  command -v lease_verify >/dev/null 2>&1 || . "$HERE/lease.sh"
  if ! lease_verify "$CLAUDUCTOR_LOCK_HELD" "$PPID"; then
    echo "==> FAIL: lease — this gate no longer holds $CLAUDUCTOR_LOCK_HELD (now: $(cat "$CLAUDUCTOR_LOCK_HELD/owner.json" 2>/dev/null || echo nobody)); another gate may have run beside it"
    FAILED="$FAILED lease"; code=70
  fi
fi

if [ "$code" -eq 0 ]; then
  echo
  echo "==> PASS ($MODE) on $(printf %.9s "$TESTED_SHA")"
  if [ "$MODE" = full ]; then
    printf '%s\tfull\t%s\tall\n' "$TESTED_SHA" "$state" > "$RECEIPT"
    echo "    receipt: $RECEIPT ($state)"
    # The picture for a PR's reader (the ci-status module, when on), only for a clean run: a status
    # has no `dirty` field the way the receipt does, and would show green for a commit that is not
    # what was tested.
    if [ "$state" = clean ]; then model_publish_status pass "$TESTED_SHA"; fi
    # Said HERE, at the end, where the verdict is read: a note at the top of a long log is not read.
    [ "$state" = clean ] || echo "    !!  The tree tested was NOT the commit (uncommitted or untracked files, --dirty, or HEAD moved): the merge guard refuses this receipt. Commit and re-run."
  else
    echo "    NO receipt: a --quick run is not the full gate. Run with no flags for evidence the merge guard accepts."
  fi
else
  echo
  echo "==> FAIL ($MODE):$FAILED"
  if [ "$MODE" = full ] && [ -f "$RECEIPT" ]; then rm -f "$RECEIPT"; echo "    removed $RECEIPT: a stale pass must not outlive a fail"; fi
  # Evidence and picture are retracted together: a failed full run of the committed tree also turns
  # an earlier green status on that commit red. A run over anything else (--dirty, uncommitted
  # changes, HEAD moved) did not run the commit, so it posts nothing either way.
  if [ "$MODE" = full ] && [ "$tested_commit" -eq 1 ]; then model_publish_status fail "$TESTED_SHA"; fi
fi
exit "$code"
