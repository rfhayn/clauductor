#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Status line for Claude Code.
#
# Renders:  "<mark> <focus> · <branch> <●|✓> <↑a↓b> · <N>% ctx"
#   - <mark> is STATUS_MARK from .claude/project.conf, so parallel repos are unmistakable.
#   - <focus> comes from a per-project, per-branch file written by status-write.sh; it falls back
#     to "[<branch>]" when no focus has been set.
#   - The git segment (branch, ● dirty / ✓ clean, ↑ahead ↓behind upstream) comes from git, so it
#     stays accurate even when the focus text is stale.
#   - <N>% ctx is the context-window use from Claude Code's stdin JSON.
#
# Reads a JSON blob on stdin (piped by Claude Code). Dependencies: jq; curl for the panel post.

input=$(cat)

# THE PANEL SNIPPET (clauductor docs/panel.md, "The status line"). The local panel reads a copy
# of this stdin: it carries the 5-hour and 7-day quota and the context %, which no hook payload
# does. It posts ONLY while ~/.clauductor/panel/pid names a live process: a panel killed with
# SIGKILL leaves `port` behind, and another program may hold that port by now. The pid must be
# digits and not start with 0, because `kill -0 -1` and `kill -0 0` succeed whatever runs. The
# port must be digits too, since it goes into a URL. Backgrounded with its output discarded, so
# it never delays or changes the line below; on a machine that never ran the panel it makes no
# call at all. Checked by .claude/checks/statusline.sh.
panel="$HOME/.clauductor/panel"
panel_port=$(cat "$panel/port" 2>/dev/null)
panel_pid=$(cat "$panel/pid" 2>/dev/null)
case "$panel_port" in '' | *[!0-9]*) panel_port="" ;; esac
case "$panel_pid" in '' | *[!0-9]* | 0*) panel_pid="" ;; esac
if [ -n "$panel_port" ] && [ -n "$panel_pid" ] && kill -0 "$panel_pid" 2>/dev/null; then
  { printf '%s' "$input" | curl -s --max-time 0.5 -X POST -H 'Content-Type: application/json' \
    --data-binary @- "http://127.0.0.1:${panel_port}/status"; } >/dev/null 2>&1 &
fi

used=$(printf '%s' "$input" | jq -r '.context_window.used_percentage // empty' 2>/dev/null)
cwd=$(printf '%s' "$input" | jq -r '.workspace.current_dir // .cwd // empty' 2>/dev/null)
[ -n "$cwd" ] || cwd=$(pwd)

# This script's own checkout supplies the config (the mark, the slug), whatever the cwd.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git -C "$cwd" rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"

branch=$(git -C "$cwd" --no-optional-locks branch --show-current 2>/dev/null)

focus=""
if [ -n "$branch" ]; then
  file=$(focus_file "$branch")
  [ -f "$file" ] && focus=$(cat "$file")
  [ -z "$focus" ] && focus="[$branch]"
fi

git=""
if [ -n "$branch" ]; then
  git="$branch"
  if [ -n "$(git -C "$cwd" --no-optional-locks status --porcelain 2>/dev/null)" ]; then
    git="$git ●"
  else
    git="$git ✓"
  fi
  counts=$(git -C "$cwd" --no-optional-locks rev-list --left-right --count '@{upstream}...HEAD' 2>/dev/null)
  if [ -n "$counts" ]; then
    behind=$(echo "$counts" | awk '{print $1}')
    ahead=$(echo "$counts" | awk '{print $2}')
    track=""
    [ "$ahead" -gt 0 ] 2>/dev/null && track="↑$ahead"
    [ "$behind" -gt 0 ] 2>/dev/null && track="${track}↓$behind"
    [ -n "$track" ] && git="$git $track"
  fi
fi

ctx=""
[ -n "$used" ] && ctx="$(printf '%.0f' "$used")% ctx"

out="$STATUS_MARK"
[ -n "$focus" ] && out="$out ${focus}"
[ -n "$git" ] && out="$out · ${git}"
[ -n "$ctx" ] && out="$out · ${ctx}"
[ "$out" = "$STATUS_MARK" ] && out="$STATUS_MARK $PROJECT_NAME"
echo "$out"
