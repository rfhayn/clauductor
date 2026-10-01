#!/bin/sh
# Health: the latest `push` run on the main branch of every workflow whose `on:` has `push`. A
# post-merge run catches what no PR can (two merges without a rebase between them), and its failure
# reaches no PR either. Each line names its subject: the commit the run tested against the current
# origin/<main> (STALE when the run is not of it), its age and its URL. The logic is
# .claude/lib/health.sh's (health_push_main), which `clauductor update` keeps current.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# The library: this checkout's, else the plugin's (CLAUDUCTOR_FW, exported by extensions.sh).
lib="$ROOT/.claude/lib/health.sh"; [ -f "$lib" ] || lib="${CLAUDUCTOR_FW:-}/lib/health.sh"
[ -f "$lib" ] || { echo "CANNOT CHECK — .claude/lib/health.sh is missing; push results are UNKNOWN, not healthy"; exit 0; }
# shellcheck disable=SC1090
. "$lib"
health_push_main
exit 0
