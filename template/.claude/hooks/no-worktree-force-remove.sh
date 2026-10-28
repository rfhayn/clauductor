#!/bin/sh
# PreToolUse hook (matcher: Bash). Refuses a Bash call that runs `git worktree remove` with
# `--force` or `-f` (any unique prefix of --force, or a short cluster containing f).
#
# WHY (ST-14). On 2026-10-05 Standing Tee's orchestrator ran `git worktree remove --force` on a
# worktree that still held a session's edits, and about 20 minutes of uncommitted work was
# destroyed. The plain form refuses a tree with modified or untracked files; `--force` is what
# removed it. Removal of a lane worktree goes through .claude/machine-quiet.sh, which keeps a
# dirty, locked or occupied one. A script the call runs is not read, so machine-quiet.sh's own
# `worktree remove -f`, after those checks, is unaffected.
#
# The command is read by lib/worktree-remove.awk (no node: ADR-0006). A removal it cannot read
# (a computed argument before `--`, a script on a shell's stdin) is refused. Exit 2 blocks;
# exit 0 allows. If awk cannot answer and the raw text names a worktree removal with a dash-f,
# this fails closed.
payload=$(cat)
SAFE="Look first: git -C <worktree> status --short. If it lists anything, STOP: that is someone's uncommitted work, so do not discard it (no checkout --, reset --hard, clean or rm) and report it to whoever owns the worktree. If it lists nothing, the plain 'git worktree remove <worktree>' removes it (ignored files such as node_modules do not count). A worktree under .claude/worktrees/ is removed by machine-quiet.sh, which keeps a dirty, locked or occupied one. If a tree really holds nothing worth keeping, say so to the owner and let them remove it; do not route around this hook."
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || cmd=""
verdict=unreadable
reader=$(dirname "$0")/lib/worktree-remove.awk
if [ -n "$cmd" ] && [ -f "$reader" ] && command -v awk >/dev/null 2>&1; then
  verdict=$(printf '%s' "$cmd" | awk -f "$reader" 2>/dev/null) || verdict=unreadable
fi
case "$verdict" in
  allow) exit 0 ;;
  force*)
    echo "BLOCKED by no-worktree-force-remove: 'git worktree remove ${verdict#force }' deletes a worktree's uncommitted work without asking. On 2026-10-05 it destroyed 20 minutes of a session's edits." >&2
    echo "$SAFE" >&2
    exit 2
    ;;
  computed*)
    echo "BLOCKED by no-worktree-force-remove: this call removes a worktree in a form the hook cannot read (${verdict#computed }), so it cannot tell whether it forces. Write 'git worktree remove' and its flags literally, and put a computed path after '--'." >&2
    echo "$SAFE" >&2
    exit 2
    ;;
esac
case "$payload" in
  *worktree*remove*)
    case "$payload" in
      *' -f'* | *' --f'* | *' -'[A-Za-z]*f*)
        echo "BLOCKED by no-worktree-force-remove: awk could not read the hook payload, and this call names a worktree removal with what may be --force, so the hook refuses rather than let it through unchecked." >&2
        echo "$SAFE" >&2
        exit 2
        ;;
    esac
    ;;
esac
exit 0
