#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# What each panel lane template could start next, for the panel's New lane dialog
# (`templates[].suggest` in .clauductor/panel.json; clauductor docs/panel.md, "Suggestions").
#
#   sh .claude/panel-suggest.sh build     queued change rows whose proposal is on this checkout (CHANGES_DIR/<id>)
#   sh .claude/panel-suggest.sh propose   queued change rows with no proposal yet: the NEXT one only
#   sh .claude/panel-suggest.sh ops       queued ops/<name> rows, named <name>
#   sh .claude/panel-suggest.sh fix       open GitHub issues (gh), newest first, named <n>-<title slug>
#
# It prints the panel's JSON: [{"name","title","detail","issue"}]. The rows are the CURRENT phase's
# queued rows as .claude/roadmap-queue.sh parses them (one parser, never a second copy of the
# table), so the dialog offers what the Change queue card shows. A lane name must match
# [a-z0-9][a-z0-9-]{0,40}; a row that cannot be one is left out, not mangled. A failure exits
# non-zero, so the panel says "cannot read" and keeps the last list rather than showing none.
set -u
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
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

tsv=$(roadmap_queue --tsv) || { echo "panel-suggest: the roadmap queue could not be parsed" >&2; exit 1; }
# The current phase: the first phase with a queued, in-flight or open row (the parser's own rule).
cur=$(printf '%s\n' "$tsv" | awk -F'\t' '$2 != "-" && ($7 == "queued" || $7 == "inflight" || $7 == "open") { print $2; exit }')
[ -n "$cur" ] || { echo "[]"; exit 0; }

# Which change ids already have a proposal in this checkout (the main checkout, where the panel runs).
have=$(ls "$CHANGES_DIR" 2>/dev/null | grep -vx 'archive\|README.md' | tr '\n' ' ')
# Fields: line phase section id change kind state pr owner. awk, not `read`: a tab is IFS
# whitespace to the shell, so an empty field (no section) would collapse and shift the rest.
printf '%s\n' "$tsv" | awk -F'\t' -v cur="$cur" -v kind="$kind" -v have=" $have " '
  $2 != cur || $7 != "queued" { next }
  {
    name = ""; proposed = index(have, " " $5 " ") > 0
    if (kind == "ops" && $6 == "ops") name = substr($5, 5)
    else if (kind == "build" && $6 == "change" && proposed) name = $5
    else if (kind == "propose" && $6 == "change") {
      # Just in time, at most one ahead: a queued row that already has a proposal IS the one
      # ahead, so offer nothing; otherwise offer only the first queued change row.
      if (proposed) ahead = 1
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
  END { if (kind == "propose" && !ahead && pick != "") emit(pick, pline) }' | jq -R -s -c 'split("\n") | map(select(length > 0) | split("\t") | {name: .[0], title: .[1], detail: .[2]})'
