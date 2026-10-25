#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# What each panel lane template could start next, for the panel's New lane dialog
# (`templates[].suggest` in .clauductor/panel.json; clauductor docs/panel.md, "Suggestions").
#
#   sh .claude/panel-suggest.sh build     queued change rows whose proposal is on main (CHANGES_DIR/<id>)
#   sh .claude/panel-suggest.sh propose   the next queued change row with no proposal yet, or none
#   sh .claude/panel-suggest.sh ops       queued ops/<name> rows, named <name>
#   sh .claude/panel-suggest.sh fix       open GitHub issues (gh), newest first, named <n>-<title slug>
#   sh .claude/panel-suggest.sh close     the daily close, session-<N>-close, once the day's rows landed
#
# close (OPS-125): a row's PR leaves its fragment on main (JOURNAL_FRAGMENTS) and its row queued
# until the daily close folds it, so nothing else would say the close is due. With a plan, the
# session is the first not `(done)` with a row the roadmap does not show merged; it is offered once
# every one of its rows is merged, or landed: named by a fragment on main (its file, or its
# **Also closes**, lib/fragments.sh). With no plan, it is offered while any fragment waits, N one
# past the journal's top session. No fragment waiting offers nothing. A planned `ID (proposal)`
# lands with its proposal on main, since it writes no fragment (OPS-126).
# build, ops, propose (OPS-126): never a row a fragment on main names, nor a batch's later rows
# (`A + B` in the plan: only A gets a lane).
# It prints the panel's JSON: [{"name","title","detail","issue"}]. The rows come from
# .claude/roadmap-queue.sh through roadmap_queue (one parser, never a second copy of the table).
# Their order: when the roadmap has a `## Session plan` (.claude/roadmap-plan.sh, its one parser),
# its queued rows in the plan's order, so Up next is the next session's work; with no plan, the
# CURRENT phase's queued rows in table order, as the Change queue card shows them.
# propose: none while a proposal awaits the owner (on a change branch, not yet on main); otherwise,
# with a plan, the first planned change row with no proposal on main, approved ones passed; with no
# plan, the first queued row, and none while any queued row has a proposal.
#
# WHY origin/<MAIN_BRANCH>: the panel runs this in the main checkout, which nothing pulls between
# the owner's steps, while every lane's fetch moves the shared origin ref. So the roadmap and the
# proposals are read as origin has them, and the working tree only where there is no such ref.
# A lane name must match [a-z0-9][a-z0-9-]{0,40}; a row that cannot be one is left out, not
# mangled. A failure exits non-zero, so the panel says "cannot read" and keeps the last list.
set -u
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/change.sh"
cd "$ROOT" || exit 1
kind=${1:-}
case "$kind" in build | propose | ops | fix | close) ;; *) echo "usage: panel-suggest.sh build|propose|ops|fix|close" >&2; exit 2 ;; esac
command -v jq >/dev/null 2>&1 || { echo "panel-suggest: jq is not installed" >&2; exit 1; }

if [ "$kind" = fix ]; then
  issues=$(gh issue list --state open --limit 50 --json number,title,labels 2>/dev/null) || { echo "panel-suggest: gh issue list failed" >&2; exit 1; }
  printf '%s' "$issues" | jq -c '[ .[] | {
      name: ((("\(.number)-" + (.title | ascii_downcase | gsub("[^a-z0-9]+"; "-") | ltrimstr("-")))[:41]) | rtrimstr("-")),
      title: .title,
      detail: ([.labels[].name] | join(" · ") | if . == "" then null else . end),
      issue: "#\(.number)" } ]'
  exit 0
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/panel-suggest.XXXXXX") || { echo "panel-suggest: no temporary directory" >&2; exit 1; }
trap 'rm -rf "$tmp"' EXIT
ref="origin/${MAIN_BRANCH:-main}"
# `./`: paths relative to ROOT (this script cd'd there), not to the top of the git repository.
if git rev-parse -q --verify "$ref^{commit}" >/dev/null 2>&1 && git cat-file -e "$ref:./$ROADMAP" 2>/dev/null; then
  git show "$ref:./$ROADMAP" > "$tmp/roadmap.md" || { echo "panel-suggest: cannot read $ROADMAP on $ref" >&2; exit 1; }
  have=$(git ls-tree --name-only "$ref" "./$CHANGES_DIR/" 2>/dev/null | sed 's|.*/||')
  done_ids=$(git ls-tree --name-only "$ref" "./$CHANGES_DIR/archive/" 2>/dev/null | sed 's|.*/||')
else
  cp "$ROADMAP" "$tmp/roadmap.md" 2>/dev/null || { echo "panel-suggest: $ROADMAP is missing" >&2; exit 1; }
  have=$(ls "$CHANGES_DIR" 2>/dev/null)
  done_ids=$(ls "$CHANGES_DIR/archive" 2>/dev/null)
fi
have=$(printf '%s\n' "$have" | grep -vx 'archive\|README.md' | tr '\n' ' ')
done_ids=$(printf '%s\n' "$done_ids" | sed 's/^[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}-//' | tr '\n' ' ')
tsv=$(roadmap_queue --tsv "$tmp/roadmap.md") || { echo "panel-suggest: the roadmap queue could not be parsed" >&2; exit 1; }
# Proposals not yet on main, on change branches (branch_proposals, which propose's context reads
# too): `branched` are proposed (propose never offers them again); `waiting`, the ones awaiting the
# owner, are the one ahead.
open=$(printf '%s\n' "$tsv" | awk -F'\t' '$7 == "queued" || $7 == "inflight" || $7 == "open" { print $5 }' | tr '\n' ' ')
props=$(branch_proposals "$have $done_ids" "$open")
branched=$(printf '%s\n' "$props" | awk -F'\t' 'NF { print $1 }' | tr '\n' ' ')
waiting=$(printf '%s\n' "$props" | awk -F'\t' '$2 == "waiting" { print $1 }' | tr '\n' ' ')
plan=$(sh "$CLAUDUCTOR_FW/roadmap-plan.sh" "$tmp/roadmap.md") || { echo "panel-suggest: the Session plan could not be parsed" >&2; exit 1; }
printf '%s\n' "$tsv" > "$tmp/tsv"
# The planned ids in the plan's order (the R records are printed in it), each once: a change
# planned as its `(proposal)` and then its build keeps its first place.
printf '%s\n' "$plan" | awk -F'\t' '$1 == "R" && !seen[$2]++ { print $2 }' > "$tmp/plan"

# The rows a fragment on main names (its file, or its **Also closes**, lib/fragments.sh): landed,
# though their rows stay queued until the daily close folds them. close waits for them; build and
# ops never offer them again (OPS-126: OPS-125 was offered all through S11 after #222 merged).
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/fragments.sh"
mkdir -p "$tmp/frags"
if git rev-parse -q --verify "$ref^{commit}" >/dev/null 2>&1 && git cat-file -e "$ref:./$ROADMAP" 2>/dev/null; then
  for p in $(git ls-tree --name-only "$ref" "./$JOURNAL_FRAGMENTS/" 2>/dev/null); do
    case $p in *.md) [ "$(basename "$p")" = README.md ] || git show "$ref:./$p" > "$tmp/frags/$(basename "$p")" ;; esac
  done
  git show "$ref:./$JOURNAL" > "$tmp/journal" 2>/dev/null
else
  for p in "$JOURNAL_FRAGMENTS"/*.md; do [ -f "$p" ] && [ "$(basename "$p")" != README.md ] && cp "$p" "$tmp/frags/"; done
  cp "$JOURNAL" "$tmp/journal" 2>/dev/null
fi
nf=0; : > "$tmp/landed"
for f in "$tmp/frags"/*.md; do [ -f "$f" ] || continue; nf=$((nf + 1)); fragment_rows "$f" >> "$tmp/landed"; done

if [ "$kind" = close ]; then
  top=$(sed -n 's/^## Session \([0-9][0-9]*\).*/\1/p' "$tmp/journal" 2>/dev/null | head -1)
  [ "$nf" -gt 0 ] || { echo "[]"; exit 0; }
  # A planned `ID (proposal)` lands with its proposal on main (or archived): it writes no fragment.
  # It is counted per entry, never added to landed, so the same id's build entry still waits for
  # its own fragment.
  printf '%s\n' "$plan" | awk -F'\t' -v have=" $have $done_ids " 'NR == FNR { ch[$4] = $5; next }
    $1 == "R" && $6 == "proposal" && ($2 in ch) && index(have, " " ch[$2] " ") { print $2 }' "$tmp/tsv" - > "$tmp/proposed"
  if [ -s "$tmp/plan" ]; then
    # The open session: the first not (done) with a planned row the roadmap does not show merged or
    # cancelled, nor a proposal entry whose proposal has landed (as `waiting` below counts them).
    sess=$(printf '%s\n' "$plan" | awk -F'\t' 'FILENAME == ARGV[1] { if ($7 == "merged" || $7 == "cancelled") m[$4] = 1; next }
      FILENAME == ARGV[2] { p[$0] = 1; next }
      $1 == "S" && $4 !~ /\(done\)/ { order[++k] = $2; num[$2] = $3; next }
      $1 == "R" && !($2 in m) && !($6 == "proposal" && ($2 in p)) { open[$3] = 1 }
      END { for (i = 1; i <= k; i++) if (order[i] in open) { print order[i] "\t" num[order[i]]; exit } }' "$tmp/tsv" "$tmp/proposed" -)
    s=$(printf '%s\n' "$sess" | cut -f1); n=$(( ${top:-0} + 1 ))
    # The close's N is the one fold.sh and session-close name (OPS-108): the first plan session it
    # covers that no entry holds. A plain label is that session (`S4.5` folds Session 4.5); one the
    # journal already holds (an entry before it ran long), or a session no label names, takes one
    # past the journal's top, the first number free.
    if [ -z "$s" ]; then
      # No open session, yet a fragment waits (its row in no plan entry, or one marked done early):
      # offered as with no plan, never held until someone edits the plan.
      title="Close session $n: fold $nf fragment(s)"
    elif ! printf '%s\n' "$s" | grep -qE '^S[0-9]+(\.[0-9]+)*$'; then
      # An entry spanning days (`S15–S18`, `S29+`) is no one day's plan: each day's close is
      # offered while a fragment waits.
      title="Close session $n ($s): fold $nf fragment(s)"
    else
      grep -qE "^## Session $(printf '%s' "${s#S}" | sed 's/\./\\./g')( |$)" "$tmp/journal" 2>/dev/null || n=${s#S}
      # Every one of its rows merged on the roadmap or landed in a fragment, or the day is not done.
      waiting=$(printf '%s\n' "$plan" | awk -F'\t' -v s="$s" 'FILENAME == ARGV[1] { if ($7 == "merged" || $7 == "cancelled") d[$4] = 1; next }
        FILENAME == ARGV[2] { l[$0] = 1; next }
        FILENAME == ARGV[3] { p[$0] = 1; next }
        $1 == "R" && $3 == s && !($2 in d) && !($6 == "proposal" ? ($2 in p) : ($2 in l)) { print $2 }' "$tmp/tsv" "$tmp/landed" "$tmp/proposed" -)
      [ -z "$waiting" ] || { echo "[]"; exit 0; }
      if [ "$n" = "${s#S}" ]; then title="Close $s: fold $nf fragment(s)"; else title="Close $s (session $n): fold $nf fragment(s)"; fi
    fi
  else
    n=$(( ${top:-0} + 1 )); title="Close session $n: fold $nf fragment(s)"
  fi
  # A lane's name takes no dot: Session 4.5's close is the lane session-4-5-close.
  name="session-$(printf '%s' "$n" | tr . -)-close"
  printf '%s\n' "$name" | grep -qE '^[a-z0-9][a-z0-9-]{0,40}$' || { echo "[]"; exit 0; }
  jq -n -c --arg name "$name" --arg t "$title" --arg d "rows landed: $(sort -u "$tmp/landed" "$tmp/proposed" | paste -sd ',' - | sed 's/,/, /g')" \
    '[{name: $name, title: $t, detail: $d}]'
  exit 0
fi

if [ -s "$tmp/plan" ]; then
  # The queued rows the plan names, in its order; a planned id with no queue row is skipped.
  awk -F'\t' 'NR == FNR { row[$4] = $0; next } ($1 in row) { print row[$1] }' "$tmp/tsv" "$tmp/plan" \
    | awk -F'\t' '$7 == "queued"' > "$tmp/rows"
  planned=1
else
  # The current phase: the first phase with a queued, in-flight or open row (the parser's own rule).
  cur=$(awk -F'\t' '$2 != "-" && ($7 == "queued" || $7 == "inflight" || $7 == "open") { print $2; exit }' "$tmp/tsv")
  [ -n "$cur" ] || { echo "[]"; exit 0; }
  awk -F'\t' -v cur="$cur" '$2 == cur && $7 == "queued"' "$tmp/tsv" > "$tmp/rows"
  planned=0
fi
# Never offered: a row a fragment says has landed, and a batch's later rows, which land in its
# first row's PR (`A + B` in the plan): only A gets a lane. Once A has merged, been cancelled or
# landed without closing B, B is offered after all, never stranded.
printf '%s\n' "$plan" | awk -F'\t' 'FILENAME == ARGV[1] { if ($7 == "merged" || $7 == "cancelled") d[$4] = 1; next }
  FILENAME == ARGV[2] { d[$0] = 1; next }
  $1 == "R" && $7 != "" && $7 != $2 && !($7 in d) { print $2 }' "$tmp/tsv" "$tmp/landed" - > "$tmp/batched"
cat "$tmp/batched" >> "$tmp/landed"
awk -F'\t' 'FILENAME == ARGV[1] { skip[$0] = 1; next } !($4 in skip)' "$tmp/landed" "$tmp/rows" > "$tmp/rows.kept" && mv "$tmp/rows.kept" "$tmp/rows"

# Fields: line phase section id change kind state pr owner summary. awk, not `read`: a tab is IFS
# whitespace to the shell, so an empty field (no section) would collapse and shift the rest.
awk -F'\t' -v kind="$kind" -v have=" $have " -v branched=" $branched " -v waiting="$waiting" -v planned="$planned" '
  BEGIN { ahead = (waiting != "") }
  {
    name = ""; proposed = index(have, " " $5 " ") > 0
    if (kind == "ops" && $6 == "ops") name = substr($5, 5)
    else if (kind == "build" && $6 == "change" && proposed) name = $5
    else if (kind == "propose" && $6 == "change") {
      # Just in time, at most one ahead: a proposal awaiting the owner always is (BEGIN). With a
      # plan, an approved one on main is not: it waits for its build session. With no plan, any
      # queued row with a proposal is, wherever it sits in the table.
      if (proposed) { if (!planned) ahead = 1 }
      else if (pick == "" && index(branched, " " $5 " ") == 0) { pick = $5; pline = $0 }
    }
    if (name == "") next
    emit(name, $0)
  }
  function emit(nm, row,   f) {
    split(row, f, "\t")
    if (nm !~ /^[a-z0-9][a-z0-9-]*$/ || length(nm) > 41) return
    printf "%s\t%s\t%s%s%s\n", nm, (f[10] == "" ? f[4] : f[10]), f[4], (f[3] == "" ? "" : " · " f[3]), (f[9] == "" ? "" : " · owner " f[9])
  }
  END { if (kind == "propose" && !ahead && pick != "") emit(pick, pline) }' "$tmp/rows" \
  | jq -R -s -c 'split("\n") | map(select(length > 0) | split("\t") | {name: .[0], title: .[1], detail: .[2]})'
