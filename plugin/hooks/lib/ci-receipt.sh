# Sourced by .claude/hooks/pr-merge-guard.sh (rule 2(b)) and .claude/checks/merge-guard.sh. POSIX
# sh; needs only git, cut, head. No jq, so the check can drive it anywhere.
#
# THE RECEIPT CONTRACT. The full gate (GATE_RUN, scripts/ci/run-local.sh) writes
# `$(git rev-parse --git-dir)/ci-receipt` after a COMPLETE, CLEAN run, and only then: one line,
# tab-separated,
#     <sha>  <scope>  <clean|dirty>  <stage>
# where <sha> is the commit that was tested (resolved before the run starts), <stage> is `all` for
# the whole gate, and <clean|dirty> says whether the tree tested was the commit or a working tree.
# A partial run (--quick) neither writes nor deletes a receipt; a failed full run deletes it. It
# lives in the git dir, so it is per clone, never committed, and cannot vouch on another machine
# for something it never ran.
#
# WHY EVERY WORKTREE. In a linked worktree the git dir is `.git/worktrees/<name>/`, so a lane's
# receipt is `.git/worktrees/<name>/ci-receipt`. A lookup from the hook's own cwd (the main
# checkout) reads only `.git/ci-receipt`, and refused three genuine full runs while blaming "a
# receipt for a different commit". So every git dir `git worktree list` names is examined, and a
# refusal lists what was examined (a result names its subject).
#
# Only this repository's own git dirs are read: the common dir and `<common>/worktrees/*`. A
# worktree whose directory has been deleted is skipped. The SHA match does the real work.

# Print, one per line, the absolute git dir of the main checkout and of every linked worktree whose
# directory still exists. Never fails; prints nothing if this is not a repository.
ci_receipt_git_dirs() {
  _common=$(git rev-parse --git-common-dir 2>/dev/null) || return 0
  _common=$(cd "$_common" 2>/dev/null && pwd -P) || return 0
  printf '%s\n' "$_common"
  git worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p' | while IFS= read -r _wt; do
    [ -d "$_wt" ] || continue
    _gd=$(git -C "$_wt" rev-parse --absolute-git-dir 2>/dev/null) || continue
    _gd=$(cd "$_gd" 2>/dev/null && pwd -P) || continue
    case "$_gd" in
      "$_common") ;;
      "$_common"/worktrees/*) printf '%s\n' "$_gd" ;;
    esac
  done
}

# ci_receipt_verdict <head_sha> <pr>: prints a message; returns 0 = allow, 2 = block.
ci_receipt_verdict() {
  _head=$1; _pr=$2
  _run=${GATE_RUN:-scripts/ci/run-local.sh}
  _wf=${GATE_REMOTE_WORKFLOW-ci.yml}
  _alt=""; [ -n "$_wf" ] && _alt="   (or: gh workflow run $_wf --ref <branch>)"
  _nl='
'
  _dirs=$(ci_receipt_git_dirs)
  _full=""; _partial=""; _partial_stage=""; _dirty=""; _examined=""; _dirlist=""; _n=0

  if [ -z "$_head" ]; then
    echo "no head SHA was given for PR #$_pr, so no receipt can be matched."
    return 2
  fi

  while IFS= read -r _gd; do
    [ -n "$_gd" ] || continue
    _n=$((_n + 1))
    _dirlist="$_dirlist$_nl    $_gd"
    _rf="$_gd/ci-receipt"
    [ -f "$_rf" ] || continue
    _sha=$(head -n 1 "$_rf" | cut -f1)
    _state=$(head -n 1 "$_rf" | cut -f3)
    _stage=$(head -n 1 "$_rf" | cut -f4)
    _examined="$_examined$_nl    $_rf -> ${_sha:-?} (${_state:-?}, stage ${_stage:-?})"
    [ "$_sha" = "$_head" ] || continue
    if [ "$_state" = "clean" ] && [ "$_stage" = "all" ]; then
      [ -n "$_full" ] || _full=$_rf
    elif [ "$_state" = "clean" ]; then
      [ -n "$_partial" ] || { _partial=$_rf; _partial_stage=$_stage; }
    else
      [ -n "$_dirty" ] || _dirty=$_rf
    fi
  done <<EOF
$_dirs
EOF

  if [ -n "$_full" ]; then
    echo "${_wf:+no successful $_wf run for PR #$_pr, but }$_run passed in full on this exact commit ($(printf %.9s "$_head")…). Receipt: $_full. Allowing."
    return 0
  elif [ -n "$_partial" ]; then
    # A partial receipt is the most dangerous of the set: honest, current, and covering less than
    # it appears to.
    echo "the local gate receipt for PR #$_pr ($_partial) records stage '${_partial_stage:-?}', not the full gate. The gate writes a receipt only for a complete run, so this one was hand-edited or left by an older version. Re-run: $_run"
  elif [ -n "$_dirty" ]; then
    echo "the local gate receipt for PR #$_pr ($_dirty) tested a working tree, not the commit. Commit, then re-run without --dirty: $_run"
  elif [ -n "$_examined" ]; then
    echo "PR #$_pr has no gate evidence: no local receipt names its head $_head. Receipts examined across this repository's $_n git dir(s):$_examined${_nl}A receipt for another commit is the same lie as a stale green check. Run: $_run$_alt"
  else
    echo "PR #$_pr has no gate evidence for $_head: no local receipt in any of this repository's $_n git dir(s):$_dirlist${_nl}Other green checks do not count. Run: $_run$_alt"
  fi
  return 2
}
