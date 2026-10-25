#!/bin/sh
# roadmap-plan.sh: THE parser of the roadmap's optional `## Session plan` table, which orders the
# queue's rows into working sessions. The panel's Up next (panel-suggest.sh) and a project's own
# readers (Clauductor's roadmap tracker) call it, never a second copy of its grammar. The rows
# themselves, and their states, come from roadmap_queue (lib/conf.sh): this script reads only the
# plan's own table, whose header is not the queue header, so the queue parser skips it.
#
#   sh .claude/roadmap-plan.sh FILE    the plan in FILE, as tab-separated records, in table order:
#     S  session  order  when  focus  owner     a session (order: the first number in its name)
#     R  id  session  order  pos  kind  lead    a row it names (pos: its place in the whole plan;
#                                               kind: `proposal` or empty; lead: its batch's first
#                                               id, empty when it is not batched)
#     D  id  decision                           an `ID: decision` from *The owner decides*
#
# THE GRAMMAR:
#   ## Session plan
#   | Session | When | Focus | Rows | The owner decides |
#   |---|---|---|---|---|
#   | S8 | (done) | what it is for | OPS-111, OPS-120 | OPS-111: approve its design; OPS-120: … |
# Rows: row ids, comma-separated, each planned once. An entry `ID (proposal)` is a change row this
# session only proposes: it has landed once its proposal is on main (OPS-126), and the same id may
# be planned once more, in a later entry, as its build. An entry `A + B` is a batch, one PR titled
# by A that closes B too (its fragment's **Also closes**): only A gets a lane. *The owner decides*:
# `—`, or `ID: decision` parts separated by `;`. A section with no table, a second table, a wrong
# header or cell count, a session planned twice, an id planned twice (beyond its proposal and its
# build) or built before its proposal is an error (exit 1, each named on stderr), never a
# shorter plan. No `## Session plan` section prints nothing and exits 0: the plan is optional.
set -u
f=${1:-}
[ -n "$f" ] || { echo "usage: roadmap-plan.sh FILE" >&2; exit 2; }
[ -f "$f" ] || { echo "roadmap-plan: $f is missing" >&2; exit 1; }
awk '
function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
function err(m) { printf "%s:%d: %s\n", FILENAME, NR, m > "/dev/stderr"; bad = 1 }
/^## / { insec = ($0 ~ /^## Session plan[ \t]*$/); if (insec) found = 1; next }
!insec { next }
/^\|/ && done { err("a second table in the Session plan section"); next }
/^\|/ {
  nc = split($0, c, "|")
  if (!hdr) {
    h = ""; for (i = 2; i < nc; i++) h = h "|" trim(c[i])
    if (h != "|Session|When|Focus|Rows|The owner decides") err("the Session plan header must be | Session | When | Focus | Rows | The owner decides |")
    hdr = 1; sep = 1; next
  }
  if (sep) { sep = 0; if ($0 !~ /^\|[ \t:|-]+\|?[ \t]*$/) err("the Session plan header is not followed by its |---| line"); next }
  if (nc != 7) { err("a Session plan row needs exactly 5 cells, this has " (nc - 2)); next }
  s = trim(c[2]); w = trim(c[3]); fo = trim(c[4]); rows = trim(c[5]); o = trim(c[6])
  if (o == "—" || o == "-") o = ""
  if (!match(s, /[0-9]+/)) { err("session \"" s "\" has no number to order it by"); next }
  ord = substr(s, RSTART, RLENGTH) + 0
  if (s in seen) { err("session " s " is planned twice"); next }
  seen[s] = 1
  printf "S\t%s\t%d\t%s\t%s\t%s\n", s, ord, w, fo, o
  n = split(rows, r, ",")
  for (i = 1; i <= n; i++) {
    e = trim(r[i]); if (e == "") continue
    kind = ""
    if (e ~ /[ \t]\(proposal\)$/) { kind = "proposal"; e = trim(substr(e, 1, length(e) - 10)) }
    m = split(e, b, "+"); lead = (m > 1 ? trim(b[1]) : "")
    if (m > 1 && kind != "") { err("\"" trim(r[i]) "\": a batch is one PR that lands its rows, never a proposal"); continue }
    for (k = 1; k <= m; k++) {
      id = trim(b[k])
      if (id !~ /^[A-Za-z0-9][A-Za-z0-9._-]*$/) { err("\"" id "\" is not a row id"); continue }
      # Once, or twice as the two sessions of a change: its `(proposal)`, then its build.
      if ((id SUBSEP kind) in where) { err(id " is planned in " where[id SUBSEP kind] " and " s); continue }
      if (kind != "" && ((id SUBSEP "") in where)) { err(id " is built in " where[id SUBSEP ""] " before its proposal in " s); continue }
      where[id SUBSEP kind] = s; printf "R\t%s\t%s\t%d\t%d\t%s\t%s\n", id, s, ord, ++pos, kind, lead
    }
  }
  n = split(o, d, ";")
  for (i = 1; i <= n; i++) {
    p = trim(d[i])
    if (!match(p, /^[A-Za-z0-9][A-Za-z0-9._-]*(, *[A-Za-z0-9][A-Za-z0-9._-]*)*: /)) continue
    ids = substr(p, 1, RLENGTH - 2); txt = substr(p, RLENGTH + 1)
    m = split(ids, x, ",")
    for (k = 1; k <= m; k++) printf "D\t%s\t%s\n", trim(x[k]), txt
  }
  next
}
hdr && !sep { done = 1 }
END {
  if (found && !hdr) err("the Session plan section has no table")
  exit bad
}' "$f"
