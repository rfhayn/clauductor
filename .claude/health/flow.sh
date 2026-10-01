#!/bin/sh
# Health: how the work flows and what it costs, over the last 30 days (OPS-9): median cycle and lead
# time, merges per week, change-fail rate, approval wait, review rounds, spend per week and the work
# in flight, from .claude/metrics.sh --line. DORA 2025 found AI raises throughput AND instability;
# this line puts both in front of the session every time it starts, not only when someone opens the
# panel's Metrics view. A figure it cannot compute shows "—" and the line says why.
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then echo "CANNOT CHECK — offline"; exit 0; fi
# This checkout's metrics.sh, else the plugin's (CLAUDUCTOR_FW, exported by extensions.sh).
m="$ROOT/.claude/metrics.sh"; [ -f "$m" ] || m="${CLAUDUCTOR_FW:-}/metrics.sh"
[ -f "$m" ] || { echo "CANNOT CHECK — .claude/metrics.sh is missing"; exit 0; }
sh "$m" --line --window 30d
