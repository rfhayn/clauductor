#!/bin/sh
# Health: every GitHub Actions workflow whose `on:` carries a `schedule:`, and its last scheduled
# run. A scheduled run's failure reaches no PR and no person, so this line is its reader. The set
# comes from the workflows' OWN keys (the authority), never a list; each line names the run's date,
# its age (GitHub disables a schedule after 60 days of repo inactivity), its cron and its URL, and a
# failure says whether the run ever started and when the last clean run was. The logic is
# .claude/lib/health.sh's (health_scheduled), which `clauductor update` keeps current.
#   --list   the workflows it would check, one per line, with no network call
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
[ -f "$ROOT/.claude/lib/conf.sh" ] && . "$ROOT/.claude/lib/conf.sh"
# The library: this checkout's, else the plugin's (CLAUDUCTOR_FW, exported by extensions.sh).
lib="$ROOT/.claude/lib/health.sh"; [ -f "$lib" ] || lib="${CLAUDUCTOR_FW:-}/lib/health.sh"
[ -f "$lib" ] || { echo "CANNOT CHECK — .claude/lib/health.sh is missing; scheduled results are UNKNOWN, not healthy"; exit 0; }
# shellcheck disable=SC1090
. "$lib"
health_scheduled "$@"
exit 0
