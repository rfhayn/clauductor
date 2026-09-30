#!/bin/sh
# Health: each lane worktree under .claude/worktrees/, and whether it still holds unlanded work.
# A merged, clean one is machine-quiet's to remove at close; a dirty or unmerged one is a lane
# someone owns, and a stale one is a finding.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
WT="$ROOT/.claude/worktrees"
list=$(git -C "$ROOT" worktree list --porcelain 2>/dev/null | awk '/^worktree /{print substr($0, 10)}') || {
  echo "CANNOT CHECK — git worktree list failed"; exit 0; }
n=0
printf '%s\n' "$list" | while IFS= read -r w; do
  case "$w" in "$WT"/*) ;; *) continue ;; esac
  br=$(git -C "$w" branch --show-current 2>/dev/null)
  [ -n "$br" ] || br="(detached)"
  state=clean; [ -n "$(git -C "$w" status --porcelain 2>/dev/null)" ] && state=DIRTY
  merged="not merged"
  if git -C "$ROOT" rev-parse -q --verify "origin/$MAIN_BRANCH" >/dev/null 2>&1; then
    git -C "$ROOT" merge-base --is-ancestor "$(git -C "$w" rev-parse HEAD)" "origin/$MAIN_BRANCH" 2>/dev/null && merged="merged"
  else
    merged="merge state unknown (no origin/$MAIN_BRANCH)"
  fi
  last=$(git -C "$w" log -1 --format=%cr 2>/dev/null)
  echo "$( [ "$state" = clean ] && [ "$merged" = merged ] && echo OK || echo LANE) ${w#"$ROOT"/} · $br · $state · $merged · last commit $last"
done | grep . || echo "OK no lane worktrees"
