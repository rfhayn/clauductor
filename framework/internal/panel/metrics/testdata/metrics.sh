#!/bin/sh
# A fixture metrics command (PANEL-19): prints the contract's JSON, as a project's
# .claude/metrics.sh would. With METRICS_FIXTURE=bad it prints a payload that breaks
# the contract (a percent over 100), so a test can see the error shown, not a crash.
# Usage in panel.json: "metrics": { "command": ["sh", "<this file>"] }
here=$(cd "$(dirname "$0")" && pwd)
if [ "${METRICS_FIXTURE:-}" = bad ]; then
  printf '{"version":1,"windows":{"30d":{"flow":{"change_fail_rate":{"value":140}}}}}\n'
  exit 0
fi
cat "$here/metrics.json"
