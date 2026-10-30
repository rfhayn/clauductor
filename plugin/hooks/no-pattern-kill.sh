#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# PreToolUse hook (matcher: Bash). Refuses a Bash call that runs pkill or killall, from the main
# session and from a subagent alike (ST-14).
#
# WHY. On 2026-10-11 a pattern kill killed four worktree sessions and Colima. The wrappers in
# .claude/bin refuse the name when pattern-kill-path.sh has put them first on PATH. A path, a
# missing CLAUDE_ENV_FILE, `command -p` and `env PATH=` all skip that wrapper, and those forms
# are visible in the command text, so this hook refuses them too.
#
# HOW IT READS. lib/pattern-kill.awk decides from the command jq reads. pkill or killall in
# command position blocks (a path, the bare name, or the name after a wrapper, including the
# script of sh -c). A mention in an argument does not: grep, or a quoted PR body. A script
# file is not read. If jq or awk cannot answer, the raw text is over-read and a path refuses
# (broader, never narrower).
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow.
payload=$(cat)
say() {
  echo "BLOCKED by no-pattern-kill: this call runs pkill or killall (by path, by name, or after a rebuilt PATH such as command -p or env PATH=). Pattern kills are refused in every session: on 2026-10-11 one killed four worktree sessions and Colima. Kill only what you started, by the pid you recorded: cmd & pid=\$!; ...; kill \"\$pid\", or a group you started with kill -- -<pgid>." >&2
}
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || cmd=""
reader=$(dirname "$0")/lib/pattern-kill.awk
if [ -n "$cmd" ] && [ -f "$reader" ]; then
  verdict=$(printf '%s' "$cmd" | awk -f "$reader" 2>/dev/null) || verdict=""
  case "$verdict" in
    allow) exit 0 ;;
    block) say; exit 2 ;;
  esac
fi
# jq or awk did not answer: refuse a path-shaped mention rather than allow one.
text=$cmd
end='([^a-z0-9_.-]|$)'
if [ -z "$text" ]; then
  text=$payload
  end=''
fi
base=$(printf '%s\n' "$text" | LC_ALL=C awk '{ if (sub(/\\$/, "")) printf "%s", $0; else print }' \
  | LC_ALL=C tr 'A-Z' 'a-z' | LC_ALL=C tr -d "\"'\\\\")
left=$(printf '%s\n%s\n' "$(printf '%s' "$base" | LC_ALL=C tr -d '${`')" \
  "$(printf '%s' "$base" | LC_ALL=C tr '${`' '   ')" \
  | LC_ALL=C sed -e 's#\.claude/bin/pkill##g' -e 's#\.claude/bin/killall##g' \
    -e 's#pkill/killall##g' -e 's#killall/pkill##g')
if printf '%s\n' "$left" | LC_ALL=C grep -Eq "/(pkill|killall)$end"; then
  say
  exit 2
fi
exit 0
