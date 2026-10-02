#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The lanes: who owns which part of the repository, read from the people registry's "lanes" array
# (PEOPLE). Printed while the people module is on; the module's session-start fragment tells the
# session to stay in its lane. Derived each time, so the table cannot drift from the registry.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
sh "$CLAUDUCTOR_FW/modules/people/people.sh" --lanes 2>&1
exit 0
