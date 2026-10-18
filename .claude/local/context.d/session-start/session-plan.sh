#!/bin/sh
# session-start's session-plan section: the next session of the roadmap's Session plan (the first
# whose When does not say "(done)"), its focus, its rows in the plan's order with their states, and
# what the owner decides. WHY: the change queue prints rows in table order, and nothing else at
# session start read the plan the owner agreed, so session 5 would have opened on PANEL-49 with
# OPS-90, the owner's first, far down the queue (2026-10-18). It reuses roadmap-tracker.sh's reading
# of the plan (rows in plan order: its roadmap.json sorts by session, then plan position), never a
# second parser. Local git only, so it reads the same with CONTEXT_OFFLINE=1.
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
out=$(mktemp -d "${TMPDIR:-/tmp}/session-plan.XXXXXX") || { echo "session plan: CANNOT CHECK — no temporary directory"; exit 0; }
trap 'rm -rf "$out"' EXIT
if ! sh "$ROOT/.claude/local/roadmap-tracker.sh" "$out/data" >/dev/null 2>"$out/err"; then
  echo "session plan: CANNOT CHECK — $(tail -n 1 "$out/err")"
  exit 0
fi
next=$(jq -r '[.[] | select(.when | test("\\(done\\)") | not)] | sort_by(.order) | .[0] // empty | .session' "$out/data/sessions.json")
if [ -z "$next" ]; then
  echo "session plan: every planned session is done; plan the next ones with the owner"
  exit 0
fi
jq -r --arg s "$next" --slurpfile rows "$out/data/roadmap.json" '
  .[] | select(.session == $s)
  | "session plan: \(.session)\(if .when == "" then "" else " (\(.when))" end) — \(.focus)",
    "  rows, in the plan'"'"'s order: \([$rows[0][] | select(.session == $s) | "\(.id) [\(.state)]"] | join(", "))",
    ([$rows[0][] | select(.session == $s and (.note | test("(^|; )cloud: "))) | "\(.id) (\(.note | sub("^(.*; )?cloud: "; "")))"]
     | if length > 0 then "  cloud lanes, run their routines first (session-start step): \(join(", "))" else empty end),
    "  the owner decides: \(if .owner == "" then "—" else .owner end)",
    "  Run the session in this order, not the change queue'"'"'s table order: it is the order the owner agreed."' \
  "$out/data/sessions.json"
exit 0
