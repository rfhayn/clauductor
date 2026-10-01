#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# usage-report.sh: what Claude Code has cost here, by role and by model (OPS-9, "Cost per role").
#
#   sh .claude/usage-report.sh                    the calling session (CLAUDE_CODE_SESSION_ID), its agents included
#   sh .claude/usage-report.sh --session <id>     one session by id
#   sh .claude/usage-report.sh --since YYYY-MM-DD every session of this repository with activity since then
#   sh .claude/usage-report.sh --days N           the same, for the last N days
#   ... --json                                    {subject,since,sessions,messages,total_usd,unpriced,rows,by_role,by_model}
#   ... --records                                 one JSON line per message, deduplicated (metrics.sh reads this)
#
# WHY IT EXISTS. The model chosen for each role (.claude/model-roles.json) is a hypothesis until its
# cost is measured next to what the role catches; a note asking for the number is not a mechanism
# (AGENTS.md rule 4), so this prints it: session-close shows it, and metrics.sh's Cost tab sums it.
# StandingT's usage-report.mjs found that cost is dominated by CACHE READS (context re-read on every
# turn), not output, so the table splits each row into cache read, cache write and output.
#
# The reading and the pricing are .claude/lib/usage.sh's (shared with metrics.sh): this repository's
# transcripts on THIS machine, subagents and workflow agents included, each agent charged to the role
# at the root of its spawn chain, each message once, at the list prices in model-roles.json `.prices`.
# An unpriced model is listed as UNPRICED, never costed at zero. Dollars are list-price API
# equivalents, not a bill. SUBJECT FIRST: the report opens by naming what it summed.
#
# Exit 0 with a report, 2 when it CANNOT CHECK (no jq, no session to report, no transcripts), so a
# caller never reads "no data" as "$0".
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/usage.sh"

session="" since="" days="" mode=text
while [ $# -gt 0 ]; do
  case "$1" in
    --session) session=${2:-}; shift 2 ;;
    --since) since=${2:-}; shift 2 ;;
    --days) days=${2:-}; shift 2 ;;
    --json) mode=json; shift ;;
    --records) mode=records; shift ;;
    *) echo "usage: usage-report.sh [--session <id> | --since YYYY-MM-DD | --days N] [--json | --records]" >&2; exit 64 ;;
  esac
done
cannot() {
  if [ "$mode" = json ]; then printf '{"total_usd":null,"cannot":"%s"}\n' "$(printf '%s' "$1" | sed 's/["\\]/\\&/g')"
  else echo "CANNOT CHECK — $1"; fi
  exit 2
}
command -v jq >/dev/null 2>&1 || cannot "jq is not installed"

now=${USAGE_NOW:-$(date +%s)}
if [ -n "$days" ]; then
  case "$days" in '' | *[!0-9]*) cannot "--days wants a whole number of days, not '$days'" ;; esac
  since=$(jq -rn --argjson t "$((now - days * 86400))" '$t | todate')
  subject="every session, last $days day(s) (since $since)"
elif [ -n "$since" ]; then
  printf '%s' "$since" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' || cannot "--since wants YYYY-MM-DD, not '$since'"
  since="${since}T00:00:00Z"
  subject="every session since $since"
else
  session=${session:-${CLAUDE_CODE_SESSION_ID:-}}
  [ -n "$session" ] || cannot "no session to report: pass --session <id>, --since YYYY-MM-DD or --days N, or run inside Claude Code (CLAUDE_CODE_SESSION_ID)"
  subject="session $session, whole session"
fi

main=$(usage_main) || cannot "$ROOT is not a git checkout"
[ -n "$(usage_dirs "$main")" ] || cannot "no transcripts for this repository under $(usage_projects)"
tmp=$(mktemp "${TMPDIR:-/tmp}/usage-report.XXXXXX") || cannot "mktemp failed"
trap 'rm -f "$tmp" "$tmp.err" "$tmp.sum"' EXIT
usage_records "$since" "$session" 2> "$tmp.err" | jq -cn 'reduce inputs as $r ({}; .[$r.id] = $r) | .[]' > "$tmp" \
  || cannot "the transcripts could not be read: $(head -1 "$tmp.err")"
[ -s "$tmp.err" ] && [ ! -s "$tmp" ] && cannot "$(head -1 "$tmp.err")"

if [ "$mode" = records ]; then cat "$tmp"; exit 0; fi
if [ -n "$session" ] && [ ! -s "$tmp" ]; then cannot "no transcript of session $session in this repository"; fi

jq -s --arg subject "$subject" --arg since "$since" '
  def r2: . * 100 | round / 100;
  def sumby(f): group_by(f) | map({name: (.[0] | f), usd: (map(.usd // 0) | add | r2), calls: length})
                | sort_by(-.usd);
  { subject: $subject,
    since: (if $since == "" then null else $since end),
    sessions: (map(.sid) | unique | length),
    messages: length,
    total_usd: (map(.usd // 0) | add // 0 | r2),
    unpriced: (map(select(.usd == null) | .model) | unique),
    rows: (group_by([.role, .model]) | map({role: .[0].role, model: .[0].model, calls: length,
            read: (map(.read) | add), write: (map(.write) | add), out: (map(.out) | add),
            usd: (if all(.usd == null) then null else (map(.usd // 0) | add | r2) end)}) | sort_by(-(.usd // -1))),
    by_role: sumby(.role),
    by_model: sumby(.model) }' "$tmp" > "$tmp.sum" || cannot "jq could not total the records"

if [ "$mode" = json ]; then cat "$tmp.sum"; exit 0; fi
jq -r '
  def m: if . >= 1000000 then "\(. / 100000 | round / 10)M" else "\(. / 1000 | round)k" end;
  "Usage — \(.subject): \(.messages) message(s) in \(.sessions) session(s)",
  (if .messages == 0 then "No API calls recorded in this window." else
    "",
    "| role | model | calls | cache read | cache write | output | ~$ |",
    "|---|---|---|---|---|---|---|",
    (.rows[] | "| \(.role) | \(.model) | \(.calls) | \(.read | m) | \(.write | m) | \(.out | m) | \(if .usd == null then "?" else .usd end) |"),
    "",
    "By role (an agent is charged to the root of its spawn chain; \"session\" is the main conversation):",
    (.by_role[] | "- \(.name): ~$\(.usd)"),
    "",
    "By model:",
    (.by_model[] | "- \(.name): ~$\(.usd)"),
    "",
    "Total ~$\(.total_usd) (list-price API equivalent, this machine only)",
    (if (.unpriced | length) > 0 then "UNPRICED (excluded from the total; add to .prices in .claude/model-roles.json): \(.unpriced | join(", "))" else empty end)
  end)' "$tmp.sum"
