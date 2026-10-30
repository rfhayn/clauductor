#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# PreToolUse hook (matchers: Read, Bash). Refuses a call that puts more than 400 lines of one
# file into the context at once: a Read with no offset/limit/pages of a file over 400 lines, or a
# Bash `cat`, `sed -n` range, `head` or `tail` that prints over 400 lines into the result.
#
# WHY THIS EXISTS (Standing Tee transcript audit 2026-10-08..14, items C and D; ST-9). Whole reads
# of long files and Bash file views dominate context cost. An instruction is documentation, not a
# control (AGENTS.md rule 4).
#
# WHY REFUSE, NOT TRIM. PostToolUse could replace output, but a schema mismatch is silently ignored;
# a refusal is loud and tested by exit code and message.
#
# Bash rules are read with lib/view-reader.awk (ported from Standing Tee's lib/view-command.mjs).
# KNOWN LIMITS versus that reader: same class as gate-reader.awk — computed command words, aliases,
# `find -exec`, a gate-style script on stdin, and `env -u` before a viewer are not seen (fail open).
# Redirection glued to the prior word (`>/dev/null>&2`) is one token; `/dev/fd/01` is a file.
#
# FAIL OPEN: empty/unreadable payload, no jq, unreadable paths, computed paths, non-numeric sed
# addresses, sed without -n, bytes/follow head, images/pdf/ipynb, any Read with offset/limit/pages,
# and a file this hook cannot read.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow. Checked by .claude/checks/hooks.sh.

payload=$(cat)
[ -n "$payload" ] || exit 0

command -v jq >/dev/null 2>&1 || exit 0

tool=$(printf '%s' "$payload" | jq -r '.tool_name // empty' 2>/dev/null) || exit 0
[ -n "$tool" ] || exit 0

cwd=$(printf '%s' "$payload" | jq -r '
  if (.cwd | type) == "string" and (.cwd | length) > 0 then .cwd else empty end
' 2>/dev/null)
[ -n "$cwd" ] || cwd=$(pwd -P 2>/dev/null || pwd)

MAX=400
HOOKDIR=$(dirname "$0")
VIEW_AWK="$HOOKDIR/lib/view-reader.awk"

block() {
  printf 'BLOCKED by read-by-range: %s\n' "$1" >&2
  printf '%s\n' "Find what you need first (grep -n for the symbol or heading), then read by range: Read with offset and limit, or sed -n 'A,Bp' of at most 400 lines. To read a whole file (an Artifact publish needs every line read), page it: Read offset 1 limit 400, then offset 401 limit 400, and so on." >&2
  exit 2
}

# abs PATH CWD → absolute path, or empty when it cannot be resolved (fail open).
abs_path() {
  _p=$1
  _d=$2
  case $_p in
    "") return 1 ;;
    ~) printf '%s' "${HOME:-}"; return 0 ;;
    ~/*) printf '%s' "${HOME:-}${_p#~}"; return 0 ;;
    /*) printf '%s' "$_p"; return 0 ;;
  esac
  _r=$(cd "$_d" 2>/dev/null && cd "$(dirname "$_p")" 2>/dev/null && pwd -P 2>/dev/null) || return 1
  printf '%s' "$_r/$(basename "$_p")"
}

# line_count PATH → count or empty when missing, unreadable, not a regular file, or binary.
line_count() {
  _f=$1
  [ -f "$_f" ] || return 1
  [ -r "$_f" ] || return 1
  if head -c 8192 "$_f" 2>/dev/null | awk '{for(i=1;i<=length($0);i++) if(substr($0,i,1)=="\0") exit 0} END{exit 1}' 2>/dev/null; then
    return 1
  fi
  _n=$(awk 'BEGIN{n=0} {n+=1} END{print n+0}' "$_f" 2>/dev/null) || return 1
  _last=$(tail -c 1 "$_f" 2>/dev/null | od -An -tx1 | tr -d ' \n')
  if [ -n "$_last" ] && [ "$_last" != "0a" ]; then _n=$((_n + 1)); fi
  printf '%s' "$_n"
}

not_text_ext() {
  case $(printf '%s' "$1" | awk -F. '{print tolower($NF)}') in
    png|jpg|jpeg|gif|webp|bmp|ico|pdf|ipynb) return 0 ;;
    *) return 1 ;;
  esac
}

if [ "$tool" = Read ]; then
  off=$(printf '%s' "$payload" | jq -r '.tool_input.offset // empty' 2>/dev/null)
  lim=$(printf '%s' "$payload" | jq -r '.tool_input.limit // empty' 2>/dev/null)
  pgs=$(printf '%s' "$payload" | jq -r '.tool_input.pages // empty' 2>/dev/null)
  [ -n "$off" ] || [ -n "$lim" ] || [ -n "$pgs" ] && exit 0

  file=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // ""' 2>/dev/null) || exit 0
  [ -n "$file" ] || exit 0
  not_text_ext "$file" && exit 0

  ap=$(abs_path "$file" "$cwd") || exit 0
  n=$(line_count "$ap") || exit 0
  [ "$n" -gt "$MAX" ] || exit 0
  block "$file has $n lines, over $MAX: a whole Read puts all of them in your context, and every later call re-reads them as cache."
fi

if [ "$tool" = Bash ]; then
  cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // ""' 2>/dev/null) || exit 0
  [ -f "$VIEW_AWK" ] || exit 0
  verdict=$(printf '%s' "$cmd" | awk -v cwd="$cwd" -v max="$MAX" -f "$VIEW_AWK" 2>/dev/null) || exit 0
  case $verdict in
    refuse*)
      block "${verdict#refuse	}"
      ;;
  esac
fi

exit 0
