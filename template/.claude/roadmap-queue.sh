#!/bin/sh
# roadmap-queue.sh: THE parser of the roadmap's change queue (ROADMAP in .claude/project.conf).
# session-start, session-close, the panel's card and suggestions, and the checks all read the
# queue through this script. Do not write a second parser: two parsers of one table disagree
# silently, and the table is the authority.
#
#   sh .claude/roadmap-queue.sh [--text]      the current phase's open rows, next first (default)
#   sh .claude/roadmap-queue.sh --check       validate the whole file; exit 1 naming each bad line
#   sh .claude/roadmap-queue.sh --tsv         every row: line phase section id change kind state pr owner summary
#   sh .claude/roadmap-queue.sh --queued [kind]   change ids of queued rows (kind: change|fix|ops)
#   ... [file]                                any mode takes an explicit roadmap path last
#
# THE GRAMMAR (docs/roadmap.md states it for humans):
#   ## Phase <N> — <title>            a phase; rows belong to the phase above them
#   ### <section>                     optional grouping inside a phase (a gate, a track)
#   **Owner:** <name>                 under a phase heading: that phase's owner; under a ### heading:
#                                     that section's, overriding the phase's for its rows
#   | # | Change | Scope | Deps | Status |    a queue table header, exactly these five columns
#   | 1.2 | `add-thing` — what a user can now do | … | 1.1 | ⬜ queued |
#   Status leads with a symbol:  ⬜ queued · ⬜ in flight (#N) · ✅ merged (#N) · ❌ cancelled …
#   The Change cell's first `backticked` token is the change id; `fix/…` and `ops/…` ids are those
#   lanes, anything else is a capability change (branch change/<id>).
#
# IT IS A PARSER, so it FAILS LOUDLY on a shape it cannot classify (*A check that reads source is a
# parser*): a queue row with the wrong number of cells, no change id, an unknown status, a
# duplicate row id, a table header that is nearly but not exactly the queue header, an owner line
# it cannot read. A row dropped silently would vanish from the queue, session-start's "next" and
# the panel's suggestions at once, and read as a shorter queue. The current phase is DERIVED: the
# first phase that still has a queued or in-flight row.
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"

mode=--text
kindf=""
case "${1:-}" in --text|--check|--tsv) mode=$1; shift ;; --queued) mode=$1; shift; case "${1:-}" in change|fix|ops) kindf=$1; shift ;; esac ;; esac
file=${1:-$ROOT/$ROADMAP}
if [ ! -f "$file" ]; then
  echo "ERROR: $file is missing, so the change queue is UNKNOWN (not empty)."
  exit 1
fi

awk -v mode="$mode" -v kindf="$kindf" -v file="$file" '
function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
function err(msg) { errs = errs file ":" NR ": " msg "\n"; nerr++ }
BEGIN { phase = "-"; ptitle = ""; section = ""; powner = ""; sowner = ""; insec = 0; intable = 0; n = 0 }
/^## / {
  intable = 0; section = ""; sowner = ""; insec = 0
  if (match($0, /^## Phase [0-9]+/)) {
    phase = substr($0, 10, RLENGTH - 9); ptitle = trim(substr($0, RLENGTH + 1)); sub(/^[—–-][ \t]*/, "", ptitle)
    if (phase in seenphase) err("a second \"## Phase " phase "\" heading")
    seenphase[phase] = 1; order[++np] = phase; ptitles[phase] = ptitle; powner = ""
  } else { phase = "-"; powner = "" }
  next
}
/^###+ / { intable = 0; section = trim(substr($0, index($0, " ") + 1)); sowner = ""; insec = 1; next }
/^\*\*Owner:\*\*/ {
  o = trim(substr($0, 11)); sub(/[ \t]+—.*$/, "", o)
  if (o == "") err("an owner line with no name")
  else if (insec) { if (sowner != "") err("a second owner line in one section"); sowner = o }
  else { if (powner != "") err("a second owner line in one phase"); powner = o }
  next
}
/^\*\*[^*]*[Oo]wner/ { err("looks like an owner line but is not \"**Owner:** <name>\""); next }
/^\|/ {
  line = $0; gsub(/\\\|/, "\001", line)
  nc = split(line, c, "|")
  if (!intable) {
    h = ""
    for (i = 2; i < nc; i++) h = h "|" trim(c[i])
    if (h == "|#|Change|Scope|Deps|Status") { intable = 1; sep = 1; next }
    if (index(h, "|Change|") && (index(h, "|Status") || h ~ /^\|#\|/)) err("a table header that is nearly the queue header; it must be exactly | # | Change | Scope | Deps | Status |")
    next
  }
  if (sep) { sep = 0; if (line ~ /^\|[ \t:|-]+\|?[ \t]*$/) next; err("the queue header is not followed by its |---| separator line") }
  if (nc != 7 || trim(c[7]) != "") { err("a queue row needs exactly 5 cells, this has " (nc - 2)); next }
  id = trim(c[2]); chg = c[3]; st = trim(c[6])
  if (id !~ /^[A-Za-z0-9][A-Za-z0-9._-]*$/) { err("row id \"" id "\" is empty or not [A-Za-z0-9._-]"); next }
  if (id in seenid) { err("row id " id " is also on line " seenid[id]); next }
  seenid[id] = NR
  if (!match(chg, /`[^`]+`/)) { err("row " id ": the Change cell has no `change-id`"); next }
  cid = substr(chg, RSTART + 1, RLENGTH - 2)
  # The summary: what follows the id (RSTART is read here, before any later match() moves it).
  sm = substr(chg, RSTART + RLENGTH); gsub(/\001/, "|", sm); gsub(/\*\*|~~/, "", sm); sm = trim(sm); sub(/^[—–-]+[ \t]*/, "", sm)
  kind = "change"; if (cid ~ /^fix\//) kind = "fix"; else if (cid ~ /^ops\//) kind = "ops"
  pr = ""; if (match(st, /\(#[0-9]+\)/)) pr = substr(st, RSTART + 2, RLENGTH - 3)
  if (index(st, "⬜ queued") == 1) state = "queued"
  else if (index(st, "⬜ in flight") == 1) { state = "inflight"; if (pr == "") err("row " id ": in flight needs its PR, \"⬜ in flight (#N)\"") }
  else if (index(st, "✅ merged") == 1) { state = "merged"; if (pr == "") err("row " id ": merged needs its PR, \"✅ merged (#N)\"") }
  else if (index(st, "❌ cancelled") == 1) state = "cancelled"
  else { err("row " id ": status must lead with ⬜ queued, ⬜ in flight (#N), ✅ merged (#N) or ❌ cancelled, not \"" substr(st, 1, 30) "\""); next }
  owner = sowner != "" ? sowner : powner
  n++; R_line[n] = NR; R_phase[n] = phase; R_sec[n] = section; R_id[n] = id; R_cid[n] = cid
  R_kind[n] = kind; R_state[n] = state; R_pr[n] = pr; R_owner[n] = owner
  R_scope[n] = trim(c[4])
  R_sum[n] = sm
  if ((state == "queued" || state == "inflight") && phase != "-" && !(phase in openphase)) openphase[phase] = 1
  next
}
{ intable = 0 }
END {
  if (nerr) { printf "ERROR: the roadmap queue is UNKNOWN; %d line(s) could not be parsed:\n%s", nerr, errs; exit 1 }
  cur = ""
  for (i = 1; i <= np; i++) if (order[i] in openphase) { cur = order[i]; break }
  if (mode == "--check") {
    printf "roadmap ok: %d row(s) in %d phase(s); current phase %s\n", n, np, (cur == "" ? "none (every row is merged or cancelled)" : cur)
    exit 0
  }
  if (mode == "--tsv") {
    for (i = 1; i <= n; i++) printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", R_line[i], R_phase[i], R_sec[i], R_id[i], R_cid[i], R_kind[i], R_state[i], R_pr[i], R_owner[i], R_sum[i]
    exit 0
  }
  if (mode == "--queued") {
    for (i = 1; i <= n; i++) if (R_state[i] == "queued" && (kindf == "" || R_kind[i] == kindf)) print R_cid[i]
    exit 0
  }
  if (cur == "") { print "queue: empty (every row is merged or cancelled); add rows to the roadmap"; exit 0 }
  k = 0; for (i = 1; i <= n; i++) if (R_phase[i] == cur && (R_state[i] == "queued" || R_state[i] == "inflight")) k++
  printf "Phase %s%s: %d open row(s), in table order\n", cur, (ptitles[cur] == "" ? "" : " — " ptitles[cur]), k
  nx = 0
  for (i = 1; i <= n; i++) {
    if (R_phase[i] != cur || (R_state[i] != "queued" && R_state[i] != "inflight")) continue
    tag = R_state[i] == "inflight" ? "in flight #" R_pr[i] : (nx++ == 0 ? "NEXT" : "queued")
    printf "  %-6s %-24s [%s]%s%s\n", R_id[i], R_cid[i], tag, (R_owner[i] == "" ? "" : " owner " R_owner[i]), (R_sec[i] == "" ? "" : " · " R_sec[i])
  }
}
' "$file"
