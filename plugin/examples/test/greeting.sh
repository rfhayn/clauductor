#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The example change's test, in the shape any language takes: each test names the scenario it
# proves ([GREETING-1-S1], [GREETING-1-S2], [FAREWELL-1-S1]), so .claude/scenario-trace.sh can
# find it. The reviewer then checks that each assertion is the scenario's THEN.
greet() { if [ -n "$1" ]; then echo "Hello, $1!"; else echo "Hello!"; fi; }
farewell() { echo "Goodbye, $1"; }

# GREETING-1-S1: an anonymous visitor is greeted
[ "$(greet "")" = "Hello!" ] || { echo "FAIL GREETING-1-S1"; exit 1; }
# GREETING-1-S2: a member is greeted by name
[ "$(greet Ana)" = "Hello, Ana!" ] || { echo "FAIL GREETING-1-S2"; exit 1; }
# FAREWELL-1-S1: a member signs out
[ "$(farewell Ana)" = "Goodbye, Ana" ] || { echo "FAIL FAREWELL-1-S1"; exit 1; }
echo "ok"
