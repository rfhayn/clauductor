#!/bin/sh
# The lanes: who owns which part of the repository, read from the people registry's "lanes" array
# (PEOPLE). Printed while the people module is on; the module's session-start fragment tells the
# session to stay in its lane. Derived each time, so the table cannot drift from the registry.
ROOT=$(cd "$(dirname "$0")/../../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
sh "$ROOT/.claude/modules/people/people.sh" --lanes 2>&1
exit 0
