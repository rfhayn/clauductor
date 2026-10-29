#!/bin/sh
# quota-gate.sh: may a new lane start, as far as the account's quota goes? (OPS-167)
#
#   sh .claude/quota-gate.sh      one line, starting "quota-gate: ", and the exit code:
#     0  below    every seven-day window with a current reading is under the threshold
#     0  no reading   nothing to judge (no file, no jq, every seven-day window past its reset): it
#                 fails OPEN, and says so, since a missing reading is not a spent quota
#     0  off      QUOTA_LANE_STOP_PCT is 0
#     1  STOP     a seven-day window (seven_day, seven_day_opus, any seven_day_* a plan adds) is at or
#                 above QUOTA_LANE_STOP_PCT (.claude/project.conf, default 90): start no new lane
#     2  ERROR    QUOTA_LANE_STOP_PCT is not a whole number from 0 to 100
#
# The reading is the status line's (~/.clauductor/quota-reading.json, read by lib/quota.sh, which never
# reports a window past its reset). Only the seven-day windows are judged: the five-hour one resets
# within hours and the panel's own quota_guard holds it. Who calls it: build-change's preflight, the
# one caller that enforces it (a STOP or an ERROR stops a new run, with notify, unless
# args.overrideQuota; a resume only warns). session-start's "Run as an orchestrator of lanes" tells the
# session to run it before a lane, which nothing checks; a panel lane or /apply-change does not run it.
# checks/quota-reading.sh holds it.
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/quota.sh"

pct=$(quota_stop_pct) || { echo "quota-gate: ERROR — $pct; no lane starts behind this guard until it is one"; exit 2; }
[ "$pct" -gt 0 ] || { echo "quota-gate: off — QUOTA_LANE_STOP_PCT is 0, so the quota holds back no lane"; exit 0; }
command -v jq >/dev/null 2>&1 || { echo "quota-gate: no reading — jq is not installed; the quota is NOT checked, so the lane may start"; exit 0; }
q=$(quota_read) || { echo "quota-gate: no reading — no readable ~/.clauductor/quota-reading.json; the quota is NOT checked, so the lane may start"; exit 0; }

TAB=$(printf '\t')
# describe: quota_read's lines on stdin as "7d 63% (resets Mon 2026-11-02 12:00 ET), ...".
describe() {
  while IFS="$TAB" read -r k p r; do
    [ -n "$k" ] || continue
    printf '%s %s (resets %s), ' "$(quota_label "$k")" "$(_quota_pct_text "$p")" "$(quota_when "$r")"
  done
}
weekly=$(printf '%s\n' "$q" | awk -F '\t' '$1 ~ /^seven_day/ && $2 != "-"')
[ -n "$weekly" ] || { echo "quota-gate: no reading — no seven-day window has a current reading (none saved, or each one's reset has passed); the quota is NOT checked, so the lane may start"; exit 0; }
over=$(printf '%s\n' "$weekly" | quota_over "$pct" | describe)
if [ -n "$over" ]; then
  echo "quota-gate: STOP — ${over%, } at or above ${pct}% (QUOTA_LANE_STOP_PCT); start no new lane until it resets"
  exit 1
fi
echo "quota-gate: below — $(printf '%s\n' "$weekly" | describe | sed 's/, $//') under ${pct}% (QUOTA_LANE_STOP_PCT)"
exit 0
