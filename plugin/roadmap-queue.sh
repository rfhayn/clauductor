#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# roadmap-queue.sh: THE parser of the roadmap's change queue (ROADMAP in .claude/project.conf).
# session-start, session-close, the panel's card and suggestions, and the checks all read the
# queue through this script's contract, by way of roadmap_queue in .claude/lib/conf.sh. Do not
# write a second parser: two parsers of one table disagree silently, and the table is the authority.
# A project whose roadmap has its own grammar keeps its own parser INSTEAD of this one, named by
# ROADMAP_PARSER in project.conf, to the contract docs/roadmap.md states (*Plugging in your own
# parser*); this script then hands every call to it.
#
#   sh .claude/roadmap-queue.sh [--text]      the current phase's open rows, next first (default)
#   sh .claude/roadmap-queue.sh --check       validate the whole file; exit 1 naming each bad line
#   sh .claude/roadmap-queue.sh --tsv         every row: line phase section id change kind state pr owner summary
#                                             budget due  (budget: dollars or empty; due: YYYY-MM-DD or empty)
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
#   `Budget: $N` anywhere in a row is the change's cost budget (item 12 of the change process);
#   `(due YYYY-MM-DD)` in the Change cell dates a row, as archive-change dates the "check the
#   outcome of <id>" row it queues. --text lists every queued dated row that is due, whatever its
#   phase: an outcome check sits in `## Outcome checks`, outside every phase.
#
# IT IS A PARSER, so it FAILS LOUDLY on a shape it cannot classify (*A check that reads source is a
# parser*): a queue row with the wrong number of cells, no change id, an unknown status, a
# duplicate row id, a table header that is nearly but not exactly the queue header, an owner line
# it cannot read. A row dropped silently would vanish from the queue, session-start's "next" and
# the panel's suggestions at once, and read as a shorter queue. The current phase is DERIVED: the
# first phase that still has a queued or in-flight row.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"

# The front door: a project that keeps its own parser (ROADMAP_PARSER) gets it through every
# command that names this script (skills, panel cards, people), via the one helper. Scripts call
# roadmap_queue directly; the helper sets ROADMAP_BUILTIN, so this grammar is parsed below. The
# same holds while a module or the local layer has a roadmap rule (roadmap.d/), so a direct call
# (`sh .claude/roadmap-queue.sh --check`) is held to the rules every reader is. Inside a rule
# (ROADMAP_IN_RULE), or when the rules cannot be listed, the helper refuses with the reason.
if [ -z "${ROADMAP_BUILTIN:-}" ] && { [ -n "$ROADMAP_PARSER" ] || [ -n "${ROADMAP_IN_RULE:-}" ] || ! _fd_rules=$(roadmap_rules) || [ -n "$_fd_rules" ]; }; then
  roadmap_queue "$@"
  exit $?
fi

mode=--text
kindf=""
case "${1:-}" in --text|--check|--tsv) mode=$1; shift ;; --queued) mode=$1; shift; case "${1:-}" in change|fix|ops) kindf=$1; shift ;; esac ;; esac
file=${1:-$ROOT/$ROADMAP}
if [ ! -f "$file" ]; then
  echo "ERROR: $file is missing, so the change queue is UNKNOWN (not empty)."
  exit 1
fi

awk -v mode="$mode" -v kindf="$kindf" -v file="$file" -v today="${ROADMAP_TODAY:-$(date +%Y-%m-%d)}" '
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
# A near miss is bold LABEL text ending in "owner…:" (`**Phase 1 owner:** Ana`), not any bold
# sentence that mentions an owner (`**Stage 1. Owner: #116. Met.**`), which is prose.
/^\*\*[^*]*[Oo]wner[^*]*:\*\*/ { err("looks like an owner line but is not \"**Owner:** <name>\""); next }
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
  budget = ""
  if (index(line, "Budget:")) {
    if (match(line, /Budget: \$[0-9]+(\.[0-9][0-9]?)?([^0-9.]|$)/)) { budget = substr(line, RSTART + 9, RLENGTH - 9); sub(/[^0-9.]$/, "", budget) }
    else { err("row " id ": a budget must read \"Budget: $<dollars>\""); next }
  }
  due = ""
  if (index(chg, "(due")) {
    if (match(chg, /\(due [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]\)/)) due = substr(chg, RSTART + 5, 10)
    else { err("row " id ": a date must read \"(due YYYY-MM-DD)\""); next }
  }
  n++; R_line[n] = NR; R_phase[n] = phase; R_sec[n] = section; R_id[n] = id; R_cid[n] = cid
  R_kind[n] = kind; R_state[n] = state; R_pr[n] = pr; R_owner[n] = owner
  R_scope[n] = trim(c[4])
  R_sum[n] = sm; R_budget[n] = budget; R_due[n] = due
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
    for (i = 1; i <= n; i++) printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", R_line[i], R_phase[i], R_sec[i], R_id[i], R_cid[i], R_kind[i], R_state[i], R_pr[i], R_owner[i], R_sum[i], R_budget[i], R_due[i]
    exit 0
  }
  if (mode == "--queued") {
    for (i = 1; i <= n; i++) if (R_state[i] == "queued" && (kindf == "" || R_kind[i] == kindf)) print R_cid[i]
    exit 0
  }
  dues = ""
  for (i = 1; i <= n; i++) if (R_state[i] == "queued" && R_due[i] != "" && R_due[i] <= today && R_phase[i] != cur) dues = dues sprintf("  DUE    %-6s %-24s %s\n", R_id[i], R_cid[i], R_sum[i])
  if (cur == "") { print "queue: empty (every row is merged or cancelled); add rows to the roadmap"; printf "%s", dues; exit 0 }
  k = 0; for (i = 1; i <= n; i++) if (R_phase[i] == cur && (R_state[i] == "queued" || R_state[i] == "inflight")) k++
  printf "Phase %s%s: %d open row(s), in table order\n", cur, (ptitles[cur] == "" ? "" : " — " ptitles[cur]), k
  nx = 0
  for (i = 1; i <= n; i++) {
    if (R_phase[i] != cur || (R_state[i] != "queued" && R_state[i] != "inflight")) continue
    tag = R_state[i] == "inflight" ? "in flight #" R_pr[i] : (nx++ == 0 ? "NEXT" : "queued")
    printf "  %-6s %-24s [%s]%s%s\n", R_id[i], R_cid[i], tag, (R_owner[i] == "" ? "" : " owner " R_owner[i]), (R_sec[i] == "" ? "" : " · " R_sec[i])
  }
  printf "%s", dues
}
' "$file"
