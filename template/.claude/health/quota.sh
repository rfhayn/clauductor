#!/bin/sh
# Health: the account's quota, from the reading the status line saves (~/.clauductor/quota-reading.json,
# OPS-67): each window's use and its reset on the owner's clock, and when the reading was saved. A
# window past its reset shows no number (the old one says nothing); no reading at all is CANNOT
# CHECK. HIGH when a seven-day window is at or above QUOTA_LANE_STOP_PCT, where .claude/quota-gate.sh
# stops a new build-change run; FAILED when that threshold is not a number. The logic is .claude/lib/quota.sh's (quota_health), which `clauductor update`
# keeps current.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# The library: this checkout's, else the plugin's (CLAUDUCTOR_FW, exported by extensions.sh).
lib="$ROOT/.claude/lib/quota.sh"; [ -f "$lib" ] || lib="${CLAUDUCTOR_FW:-}/lib/quota.sh"
[ -f "$lib" ] || { echo "CANNOT CHECK — .claude/lib/quota.sh is missing; the quota is UNKNOWN"; exit 0; }
# shellcheck disable=SC1090
. "$lib"
quota_health
exit 0
