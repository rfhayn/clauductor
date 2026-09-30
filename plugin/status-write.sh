#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Set the status-line focus for the current (or a given) branch.
#
# Usage:
#   sh .claude/status-write.sh "<focus text>"            # current branch
#   sh .claude/status-write.sh "<focus text>" <branch>   # explicit branch
#
# Stored per project and per branch (focus_file in .claude/lib/conf.sh) so parallel lanes on
# different branches never clobber each other. statusline.sh reads it, and the panel shows it on
# each lane's card. Skills call this at transitions, e.g. "[change/x] group 2/5: <title>".

label="$1"
[ -z "$label" ] && { echo "usage: status-write.sh \"<focus>\" [branch]" >&2; exit 1; }

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"

branch="${2:-$(git --no-optional-locks branch --show-current 2>/dev/null)}"
[ -z "$branch" ] && { echo "no branch resolved; pass one explicitly" >&2; exit 1; }

file=$(focus_file "$branch")
mkdir -p "$(dirname "$file")"
printf '%s\n' "$label" > "$file"
echo "focus set [$branch]: $label"
