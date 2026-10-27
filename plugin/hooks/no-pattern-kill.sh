#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# PreToolUse hook (matcher: Bash). Refuses a Bash call whose text names a PATH to pkill or killall
# (`/usr/bin/pkill`, `./killall`), from the main session and from a subagent alike (ST-14).
#
# WHY SO LITTLE. The refusal itself is .claude/bin/pkill (and killall, which hands off to it), put
# first on PATH by pattern-kill-path.sh. Whatever the shell resolves to pkill or killall BY NAME
# ends there. A path skips PATH, and that is the one way past the wrappers a command's text shows
# literally, so it is the one thing read here.
#
# HOW IT READS: an over-read of the raw text, never a parse. The command (the whole payload if jq
# cannot read it: broader, never narrower) has its backslash-newlines joined and is lower-cased,
# and its quotes and backslashes are deleted. `$`, `{` and a backtick are read twice, deleted and
# as the end of a word, and either reading can refuse. Every `.claude/bin/pkill` and
# `.claude/bin/killall` (the wrappers themselves) is deleted, and so is the prose pair
# `pkill/killall` in either order. What is left is refused if a `/` is followed by `pkill` or
# `killall` and then by anything that cannot continue a file name.
#
# WHAT IT DOES NOT SEE: a path the shell computes (`$P/pkill`, a glob), a PATH rebuilt before the
# name is looked up, and `kill`, a shell builtin. `kill "$pid"` is never refused.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow.
payload=$(cat)
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || cmd=""
end='([^a-z0-9_.-]|$)'
if [ -z "$cmd" ]; then
  cmd=$payload
  end=''
fi
base=$(printf '%s\n' "$cmd" | LC_ALL=C awk '{ if (sub(/\\$/, "")) printf "%s", $0; else print }' \
  | LC_ALL=C tr 'A-Z' 'a-z' | LC_ALL=C tr -d "\"'\\\\")
left=$(printf '%s\n%s\n' "$(printf '%s' "$base" | LC_ALL=C tr -d '${`')" \
  "$(printf '%s' "$base" | LC_ALL=C tr '${`' '   ')" \
  | LC_ALL=C sed -e 's#\.claude/bin/pkill##g' -e 's#\.claude/bin/killall##g' \
    -e 's#pkill/killall##g' -e 's#killall/pkill##g')
if printf '%s\n' "$left" | LC_ALL=C grep -Eq "/(pkill|killall)$end"; then
  echo "BLOCKED by no-pattern-kill: this call names a path to pkill or killall, which skips the refusing wrapper first on PATH (.claude/bin/pkill). Pattern kills are refused in every session: on 2026-10-11 one killed four worktree sessions and Colima. Kill only what you started, by the pid you recorded: cmd & pid=\$!; ...; kill \"\$pid\", or a group you started with kill -- -<pgid>. If the path is only prose (a commit message, a PR body), put that text in a file and pass the file (-F, --body-file)." >&2
  exit 2
fi
exit 0
