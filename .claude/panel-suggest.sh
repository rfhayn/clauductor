#!/bin/sh
# What each panel lane template could start next, for the panel's New lane dialog
# (`templates[].suggest` in .clauductor/panel.json; clauductor docs/panel.md, "Suggestions").
#
#   sh .claude/panel-suggest.sh build     queued change rows whose proposal is on main (CHANGES_DIR/<id>)
#   sh .claude/panel-suggest.sh propose   the next queued change row with no proposal yet, or none
#   sh .claude/panel-suggest.sh ops       queued ops/<name> rows, named <name>
#   sh .claude/panel-suggest.sh fix       open GitHub issues (gh), newest first, named <n>-<title slug>
#
# It prints the panel's JSON: [{"name","title","detail","issue"}]. The rows come from
# .claude/roadmap-queue.sh through roadmap_queue (one parser, never a second copy of the table).
# Their order: when the roadmap has a `## Session plan` (.claude/roadmap-plan.sh, its one parser),
# its queued rows in the plan's order, so Up next is the next session's work; with no plan, the
# CURRENT phase's queued rows in table order, as the Change queue card shows them.
# propose: with a plan, the first planned change row with no proposal, unless an earlier planned
# row's proposal still waits for the owner (no **Approved:** line): then none, since that one is
# ahead. With no plan, the first queued row, and none while any queued row has a proposal.
#
# WHY origin/<MAIN_BRANCH>: the panel runs this in the main checkout, which nothing pulls between
# the owner's steps, while every lane's fetch moves the shared origin ref. So the roadmap and the
# proposals are read as origin has them, and the working tree only where there is no such ref.
# A lane name must match [a-z0-9][a-z0-9-]{0,40}; a row that cannot be one is left out, not
# mangled. A failure exits non-zero, so the panel says "cannot read" and keeps the last list.
set -u
ROOT=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
cd "$ROOT" || exit 1
kind=${1:-}
case "$kind" in build | propose | ops | fix) ;; *) echo "usage: panel-suggest.sh build|propose|ops|fix" >&2; exit 2 ;; esac
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
if git rev-parse -q --verify "$ref^{commit}" >/dev/null 2>&1 && git cat-file -e "$ref:$ROADMAP" 2>/dev/null; then
  git show "$ref:$ROADMAP" > "$tmp/roadmap.md" || { echo "panel-suggest: cannot read $ROADMAP on $ref" >&2; exit 1; }
  have=$(git ls-tree --name-only "$ref" "$CHANGES_DIR/" 2>/dev/null | sed "s|^$CHANGES_DIR/||")
  proposal() { git show "$ref:$CHANGES_DIR/$1/proposal.md" 2>/dev/null; }
else
  ref=""
  cp "$ROADMAP" "$tmp/roadmap.md" 2>/dev/null || { echo "panel-suggest: $ROADMAP is missing" >&2; exit 1; }
  have=$(ls "$CHANGES_DIR" 2>/dev/null)
  proposal() { cat "$CHANGES_DIR/$1/proposal.md" 2>/dev/null; }
fi
have=$(printf '%s\n' "$have" | grep -vx 'archive\|README.md' | tr '\n' ' ')
# Proposals the owner approved: the Approved line change-approval.sh records, on its first lines.
approved=""
for cid in $have; do
  proposal "$cid" | head -n 3 | grep -q '^\*\*Approved:\*\*' && approved="$approved $cid"
done

tsv=$(roadmap_queue --tsv "$tmp/roadmap.md") || { echo "panel-suggest: the roadmap queue could not be parsed" >&2; exit 1; }
plan=$(sh "$ROOT/.claude/roadmap-plan.sh" "$tmp/roadmap.md") || { echo "panel-suggest: the Session plan could not be parsed" >&2; exit 1; }
printf '%s\n' "$tsv" > "$tmp/tsv"
# The planned ids in the plan's order (the R records are printed in it).
printf '%s\n' "$plan" | awk -F'\t' '$1 == "R" { print $2 }' > "$tmp/plan"

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

# Fields: line phase section id change kind state pr owner summary. awk, not `read`: a tab is IFS
# whitespace to the shell, so an empty field (no section) would collapse and shift the rest.
awk -F'\t' -v kind="$kind" -v have=" $have " -v ok=" $approved " -v planned="$planned" '
  {
    name = ""; proposed = index(have, " " $5 " ") > 0
    if (kind == "ops" && $6 == "ops") name = substr($5, 5)
    else if (kind == "build" && $6 == "change" && proposed) name = $5
    else if (kind == "propose" && $6 == "change") {
      # Just in time, at most one ahead. With a plan, an earlier planned proposal awaiting the
      # owner IS the one ahead, and an approved one is not. With no plan, any queued row with a
      # proposal is, wherever it sits in the table.
      if (proposed) { if (!planned) ahead = 1; else if (pick == "" && index(ok, " " $5 " ") == 0) ahead = 1 }
      else if (pick == "") { pick = $5; pline = $0 }
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
