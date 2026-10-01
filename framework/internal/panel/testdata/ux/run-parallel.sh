#!/bin/bash
# UX harness (UX-1): run the UX passes as shards, all at once, each on its own panel
# (its own run id, port, temp HOME and tmux sockets), then merge their findings into
# one report: <out>/report.md and <out>/findings.json.
#
#   run-parallel.sh [--out dir (default $TMPDIR/clauductor-ux-out/<run>)] [--shards a,b,…] [--ports 4700-4799] [--run id] [--keep]
#
# Shards (default: all eight):
#   layout-views       matrix.cjs --areas views        (6 viewports × every view)
#   layout-appearance  matrix.cjs --areas appearance,sizes  (themes × modes, types, text sizes)
#   flows-lanes        flows.cjs --flows lanes         (start, type, stop, close, remove, restore)
#   flows-attachments  flows.cjs --flows attachments   (images, selection, links)
#   flows-projects     flows.cjs --flows projects      (switching projects)
#   layout-metrics     matrix.cjs --areas features,metrics  (UX-2: PANEL-19/20 views, Metrics tab × range × scope)
#   flows-metrics      flows.cjs --flows metrics       (UX-2: metrics, economy, readiness, ports, setup, remote control)
#   flows-lifecycle    flows.cjs --flows lifecycle     (UX-2, UX_LIFECYCLE=1: auto-resume, auto-close on merge)
# Environment: CLAUDUCTOR_BIN (else the checkout is built once), PLAYWRIGHT (the
# playwright package dir), UX_TMP (where runs live), UX_MATRIX_ARGS (extra matrix args,
# e.g. --full). --keep leaves each run's temp dir after its panel is stopped.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fw=$(cd "$here/../../../.." && pwd)
out="" shards="layout-views,layout-appearance,flows-lanes,flows-attachments,flows-projects,layout-metrics,flows-metrics,flows-lifecycle"
ports="4700-4799" run="ux$(date +%H%M%S)" keep=""
while [ $# -gt 0 ]; do
  case "$1" in
    --out) out=$2; shift ;;
    --shards) shards=$2; shift ;;
    --ports) ports=$2; shift ;;
    --run) run=$2; shift ;;
    --keep) keep=--keep ;;
    *) echo "unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done
[[ "$run" =~ ^[a-z0-9]{1,12}$ ]] || { echo "--run must be [a-z0-9]{1,12}" >&2; exit 2; }
[ -n "$out" ] || out="${TMPDIR:-/tmp}/clauductor-ux-out/$run" # outside the checkout: nothing to ignore
pmin=${ports%-*} pmax=${ports#*-}
export UX_PORT_MIN=$pmin UX_PORT_MAX=$pmax
mkdir -p "$out"
out=$(cd "$out" && pwd -P)
command -v node >/dev/null || { echo "node is needed" >&2; exit 2; }

if [ -z "${CLAUDUCTOR_BIN:-}" ]; then
  mkdir -p "$out/.bin"
  echo "building clauductor from $fw" >&2
  (cd "$fw" && go build -o "$out/.bin/clauductor" ./cmd/clauductor) || exit 1
  export CLAUDUCTOR_BIN="$out/.bin/clauductor"
fi

IFS=, read -r -a list <<< "$shards"
runids=()
cleanup() {
  for r in "${runids[@]}"; do "$here/down.sh" "$r" $keep >/dev/null 2>&1; done
}
trap 'cleanup; exit 130' INT TERM
trap cleanup EXIT

# Distinct free ports for the shards, chosen before any starts: nothing answers or
# listens on them now.
port_free() { ! curl -s --max-time 0.3 -o /dev/null "http://127.0.0.1:$1/" && ! lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
portlist=()
p=$pmin
while [ "${#portlist[@]}" -lt "${#list[@]}" ] && [ "$p" -le "$pmax" ]; do
  port_free "$p" && portlist+=("$p")
  p=$((p + 1))
done
[ "${#portlist[@]}" -eq "${#list[@]}" ] || { echo "not enough free ports in $ports for ${#list[@]} shards" >&2; exit 2; }

shard() { # shard <i> <name>
  local i=$1 name=$2 rid="$run$1" dir="$out/$2" port t0 status=0
  mkdir -p "$dir"
  port=${portlist[$i]}
  t0=$(date +%s)
  local life=""
  [ "$name" = flows-lifecycle ] && life=1 # its lanes only: every other shard keeps the same set
  if ! UX_LIFECYCLE=$life "$here/up.sh" "$rid" "$port" > "$dir/up.json" 2> "$dir/up.log"; then
    echo "up.sh failed: $(tail -3 "$dir/up.log")" > "$dir/error.txt"; return 1
  fi
  local t1; t1=$(date +%s)
  sleep 5 # the fakes start their roles 4 s in, as claude takes seconds to start
  case "$name" in
    layout-views) node "$here/matrix.cjs" "$dir/up.json" --out "$dir" --areas views ${UX_MATRIX_ARGS:-} ;;
    layout-appearance) node "$here/matrix.cjs" "$dir/up.json" --out "$dir" --areas appearance,sizes ${UX_MATRIX_ARGS:-} ;;
    flows-lanes) node "$here/flows.cjs" "$dir/up.json" --out "$dir" --flows lanes ;;
    flows-attachments) node "$here/flows.cjs" "$dir/up.json" --out "$dir" --flows attachments ;;
    flows-projects) node "$here/flows.cjs" "$dir/up.json" --out "$dir" --flows projects ;;
    layout-metrics) node "$here/matrix.cjs" "$dir/up.json" --out "$dir" --areas features,metrics ${UX_MATRIX_ARGS:-} ;;
    flows-metrics) node "$here/flows.cjs" "$dir/up.json" --out "$dir" --flows metrics ;;
    flows-lifecycle) node "$here/flows.cjs" "$dir/up.json" --out "$dir" --flows lifecycle ;;
    *) echo "unknown shard $name" > "$dir/error.txt"; status=1 ;;
  esac > "$dir/run.log" 2>&1 || status=$?
  local t2; t2=$(date +%s)
  "$here/down.sh" "$rid" $keep > "$dir/down.log" 2>&1
  printf '{"shard":"%s","runid":"%s","port":%s,"upSecs":%s,"runSecs":%s,"totalSecs":%s,"status":%s}\n' \
    "$name" "$rid" "$port" $((t1 - t0)) $((t2 - t1)) $(( $(date +%s) - t0 )) "$status" > "$dir/time.json"
  return $status
}

t0=$(date +%s)
pids=()
i=0
for name in "${list[@]}"; do
  runids+=("$run$i")
  shard "$i" "$name" &
  pids+=($!)
  i=$((i + 1))
done
fail=0
for p in "${pids[@]}"; do wait "$p" || fail=1; done
echo "all shards done in $(( $(date +%s) - t0 )) s" >&2
node "$here/report.cjs" "$out" "${list[@]}"
exit $fail
