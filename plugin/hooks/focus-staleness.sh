#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# UserPromptSubmit hook: nudge Claude to refresh a stale status-line focus.
#
# The status line's "focus" is written by hand with status-write.sh, so it drifts as work moves
# on, and on the panel every lane shows it. This hook checks the current branch's focus file and,
# when it looks stale, prints ONE short line. A UserPromptSubmit hook's stdout on exit 0 is added
# to Claude's context, so that line becomes the reminder. When the focus looks fresh it prints
# nothing.
#
# Stale means any of: the focus file is missing; it is older than the branch's latest commit
# (work moved on); it is older than FOCUS_STALE_SECONDS (default 30 minutes).

branch=$(git --no-optional-locks branch --show-current 2>/dev/null)
[ -z "$branch" ] && exit 0   # not a git repo, or detached: nothing to nudge about

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
file=$(focus_file "$branch")

nudge() {
  echo "[status] focus may be stale: run sh "$CLAUDUCTOR_FW"/status-write.sh \"<new focus>\" if the work has shifted."
  exit 0
}

[ -f "$file" ] || nudge

# BSD/macOS stat, then GNU.
mtime=$(stat -f %m "$file" 2>/dev/null || stat -c %Y "$file" 2>/dev/null)
[ -z "$mtime" ] && exit 0   # cannot read the mtime: stay quiet rather than nag

commit=$(git --no-optional-locks log -1 --format=%ct 2>/dev/null)
[ -n "$commit" ] && [ "$mtime" -lt "$commit" ] && nudge

[ "$(( $(date +%s) - mtime ))" -gt "${FOCUS_STALE_SECONDS:-1800}" ] && nudge
exit 0
