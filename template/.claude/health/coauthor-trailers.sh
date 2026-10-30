#!/bin/sh
# Health: no commit on origin/<main> carries a Co-authored-by trailer while attribution is off
# (.claude/model-roles.json), since the commit that added this line or COAUTHOR_HEALTH_SINCE. It
# reads what reached the main branch by any road: a merge from Cursor or the website, where no hook
# runs, or a co-author line GitHub appended itself (OPS-169). The logic is .claude/lib/health.sh's
# (health_coauthor), which `stridal update` keeps current.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# The library: this checkout's, else the plugin's (STRIDAL_FW, exported by extensions.sh).
lib="$ROOT/.claude/lib/health.sh"; [ -f "$lib" ] || lib="${STRIDAL_FW:-}/lib/health.sh"
[ -f "$lib" ] || { echo "CANNOT CHECK — .claude/lib/health.sh is missing; Co-authored-by on main is UNKNOWN, not healthy"; exit 0; }
# shellcheck disable=SC1090
. "$lib"
command -v health_coauthor >/dev/null 2>&1 || { echo "CANNOT CHECK — .claude/lib/health.sh has no health_coauthor (an older model? stridal update); Co-authored-by on main is UNKNOWN, not healthy"; exit 0; }
health_coauthor ".claude/health/$(basename "$0")"
exit 0
