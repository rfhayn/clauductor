#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# SessionStart: put .claude/bin first on PATH, and drop any alias or function named pkill or
# killall, so those names run the refusing wrappers (ST-14). Claude Code runs CLAUDE_ENV_FILE in
# each Bash command's own shell, after the snapshot, which is the last word on how a name resolves.
# Never blocks a session: exit 0 always. Without CLAUDE_ENV_FILE the wrappers are not on PATH.
# no-pattern-kill.sh still refuses a Bash call it can read: a path, the bare name, command -p,
# or env PATH=. A script file is not read. The payload is drained so a closed pipe is not a
# failed launch.
cat >/dev/null
# Sibling of hooks/: .claude/bin in a project, the plugin's bin/ when the hook ships there.
bin=$(CDPATH= cd -- "$(dirname -- "$0")/../bin" 2>/dev/null && pwd -P) || {
  echo "pattern-kill-path: cannot find the pkill wrapper directory" >&2
  exit 0
}
case "$bin" in
  *"'"* | *'
'*) echo "pattern-kill-path: checkout path is not safe to put on PATH" >&2; exit 0 ;;
esac
[ -x "$bin/pkill" ] && [ -x "$bin/killall" ] || {
  echo "pattern-kill-path: .claude/bin/pkill or killall is missing" >&2
  exit 0
}
if [ -z "${CLAUDE_ENV_FILE:-}" ]; then
  echo "pattern-kill-path: no CLAUDE_ENV_FILE, so pkill and killall stay unwrapped this session"
  exit 0
fi
{
  printf 'unalias pkill 2>/dev/null; unalias killall 2>/dev/null; unset -f pkill 2>/dev/null; unset -f killall 2>/dev/null; true\n'
  printf "export PATH='%s':\"\$PATH\"\n" "$bin"
} >>"$CLAUDE_ENV_FILE" || {
  echo "pattern-kill-path: cannot write CLAUDE_ENV_FILE" >&2
  exit 0
}
echo "pattern-kill-path: .claude/bin first"
exit 0
