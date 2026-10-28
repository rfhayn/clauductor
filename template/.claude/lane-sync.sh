#!/bin/sh
# lane-sync.sh: the operations that write a lane worktree's .claude/hooks and .claude/skills, for a
# lane under Claude Code's sandbox, which denies those writes (OPS-151).
#
#   sh .claude/lane-sync.sh update [--dry-run] [--create-missing] [--prune]
#                                              clauductor update in this worktree; its answers on stdin
#   sh .claude/lane-sync.sh rebase [--continue | --abort]     git rebase origin/<MAIN_BRANCH>
#   sh .claude/lane-sync.sh merge  [--continue | --abort]     git merge --no-edit origin/<MAIN_BRANCH>
#   sh .claude/lane-sync.sh reset                             git reset --hard origin/<MAIN_BRANCH>
#
# Run it from inside the lane's worktree. It fetches nothing: fetch first (that works sandboxed).
#
# WHY A SCRIPT. The sandbox's write deny on .claude/hooks and .claude/skills is intended: it stops
# an agent editing the hooks that guard it, and the owner keeps it. But the same deny fails a lane's
# `clauductor update`, rebase, merge or reset whenever one brings a hook or skill along, and the
# owner copied files by hand about six times on 2026-10-25/26 (the `sandbox/lane-sync` insight).
# So these few operations are one named command, the ONE a lane asks to run outside the sandbox,
# and the owner (or auto mode's classifier) approves each run. Anything else that wants out of the
# sandbox is still a request of its own.
#
# WHAT IT REFUSES (exit 2, writing nothing):
#   - the main checkout: its git dir IS the common git dir. The main checkout stays on MAIN_BRANCH
#     and every hook runs from it, so a reset or an update there is never a lane's to make;
#   - any directory that is not a linked worktree of the repository this script belongs to: outside
#     git, another repository's worktree, or a path `git worktree list` does not name (the
#     authority, not a path pattern). The plugin's copy belongs to no repository: it syncs the
#     current directory's, and still refuses its main checkout;
#   - an unknown subcommand, and any argument not listed above (update's own --template-dir and
#     --force are not passed: an approved run stays the run the approver read).
# Checked by .claude/checks/lane-sync.sh.
#
# THE TRUST IT NEEDS. The approver reads the command, not this file, and a sandboxed lane can edit
# this file (it is outside the denied directories). `git diff origin/<main> -- .claude/lane-sync.sh`
# empty in the lane is what makes the name mean the reviewed script; the file is on
# approval-guarded.txt, so a change to it is the owner's to approve.
set -u

refuse() { echo "lane-sync: REFUSED: $*" >&2; exit 2; }
usage() {
  echo "usage: sh .claude/lane-sync.sh update [--dry-run] [--create-missing] [--prune]" >&2
  echo "       sh .claude/lane-sync.sh rebase|merge [--continue | --abort]" >&2
  echo "       sh .claude/lane-sync.sh reset" >&2
  exit 2
}
# real DIR: DIR with every symlink resolved (macOS reaches /var through /private/var).
real() { (cd "$1" 2>/dev/null && pwd -P); }

op=${1:-}; [ $# -gt 0 ] && shift
case $op in
  update)
    for a in "$@"; do
      case $a in --dry-run | --create-missing | --prune) ;; *) refuse "update takes --dry-run, --create-missing and --prune, not '$a'" ;; esac
    done ;;
  rebase | merge)
    [ $# -le 1 ] || usage
    case ${1:-} in '' | --continue | --abort) ;; *) refuse "$op takes --continue or --abort, not '$1'" ;; esac ;;
  reset) [ $# -eq 0 ] || refuse "reset takes no arguments" ;;
  '') usage ;;
  *) refuse "unknown subcommand '$op' (update, rebase, merge or reset)" ;;
esac

# This script's repository, the one whose worktrees it may sync: the checkout whose .claude/ holds
# it. The plugin's copy sits in the plugin's own directory, which serves every project, so there the
# repository is the current directory's, and only the main-checkout and worktree-list tests apply.
self="" self_common=""
sdir=$(real "$(dirname "$0")") || refuse "cannot find the directory this script is in"
if [ "${sdir##*/}" = .claude ]; then
  self=${sdir%/.claude}
  self_common=$(cd "$self" && real "$(git rev-parse --git-common-dir 2>/dev/null)") ||
    refuse "$self is not in a git repository"
fi

here=$(git rev-parse --show-toplevel 2>/dev/null) || refuse "$(pwd) is not inside a git worktree"
here=$(real "$here")
cd "$here" || refuse "cannot enter $here"
git_dir=$(real "$(git rev-parse --absolute-git-dir)")
common=$(real "$(git rev-parse --git-common-dir)")
[ "$git_dir" != "$common" ] || refuse "$here is the main checkout (its git dir is the common dir); lane-sync runs only in a lane's linked worktree"
[ -z "$self" ] || [ "$common" = "$self_common" ] || refuse "$here is a worktree of another repository ($common), not of $self"
listed=$(git worktree list --porcelain | sed -n 's/^worktree //p' | while IFS= read -r w; do
  [ "$(real "$w")" = "$here" ] && echo 1
done)
[ -n "$listed" ] || refuse "git worktree list does not name $here"

ROOT=$here
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
base="origin/$MAIN_BRANCH"
case $op in rebase | merge | reset)
  git rev-parse -q --verify "$base^{commit}" >/dev/null || refuse "$base does not exist here: fetch it first" ;;
esac

echo "lane-sync: $op in $here ($(git rev-parse --abbrev-ref HEAD) at $(git rev-parse --short HEAD))"
case $op in
  update)
    # This repository builds clauductor itself (its template/ is the product): build the binary
    # from THIS checkout, where it reads the template beside it, as CLAUDE.md's `clauductor update`
    # step says. CLAUDUCTOR_FRAMEWORK comes before the binary's own checkout in the template search,
    # and the owner's shell may point it at the MAIN checkout, so it is pinned to this one. Any other
    # project runs the clauductor on PATH, whose template is its release's (or CLAUDUCTOR_FRAMEWORK's).
    if [ -f framework/cmd/clauductor/main.go ] && [ -d template/.claude ]; then
      command -v go >/dev/null 2>&1 || refuse "this checkout builds clauductor, and go is not on PATH"
      (cd framework && go build -o clauductor ./cmd/clauductor) || exit 1
      bin=$here/framework/clauductor
      CLAUDUCTOR_FRAMEWORK=$here; export CLAUDUCTOR_FRAMEWORK
    else
      bin=$(command -v clauductor) || refuse "update needs the clauductor binary, and none is on PATH (the model has no other way to copy the template)"
    fi
    "$bin" update "$@"
    ;;
  rebase)
    if [ -n "${1:-}" ]; then GIT_EDITOR=: git rebase "$1"; else git rebase "$base"; fi ;;
  merge)
    if [ -n "${1:-}" ]; then GIT_EDITOR=: git merge "$1"; else git merge --no-edit "$base"; fi ;;
  reset)
    # Say what goes before it goes: the reset is the approved run, and the lane may not have pushed.
    ahead=$(git log --oneline "$base..HEAD")
    [ -z "$ahead" ] || printf 'lane-sync: dropping commits not on %s:\n%s\n' "$base" "$ahead"
    dirty=$(git status --short --untracked-files=no)
    [ -z "$dirty" ] || printf 'lane-sync: discarding uncommitted changes:\n%s\n' "$dirty"
    git reset --hard "$base"
    ;;
esac
rc=$?
echo "lane-sync: $op exited $rc; HEAD now $(git rev-parse --short HEAD)"
exit "$rc"
