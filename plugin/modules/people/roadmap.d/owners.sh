#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The roadmap's owner rule, while the people module is on: every row's owner (the `**Owner:**`
# line over it) must be a person in the people registry (PEOPLE). roadmap_queue (lib/conf.sh)
# runs this on every read of the queue, stdin = the --tsv rows; a non-zero exit makes the queue
# UNKNOWN for every reader, naming each row. An owner that parsed as nobody would drop that
# person's rows from who-is-on-what's Next: a smaller, plausible answer instead of an error.
# PEOPLE_OWNERS="off" in .claude/project.conf turns the rule off and keeps the rest of the module.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
[ "${PEOPLE_OWNERS:-required}" = off ] && exit 0
exec sh "$CLAUDUCTOR_FW/modules/people/people.sh" --owners
