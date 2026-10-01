#!/bin/sh
# archive-change.sh: promote a finished change's spec deltas into the living specs, safely. Step 1 of
# the archive-change skill: the skill decides WHEN (the PR merged, every task ticked); this decides
# WHAT the living specs become, the same way every time.
#
#   sh .claude/archive-change.sh <id>                     the plan; exit 1 on any STOP
#   sh .claude/archive-change.sh <id> --apply             the plan, then write SPECS_DIR (refused on a STOP)
#   ... --superseded "<old phrase>"                       (repeatable) wording of a decision this
#                                                         change reversed, which must not reach the specs
#
# What it guards, each a way an archive used to be wrong and still look right:
#   HELD BACK   a capability whose delta directory holds NOT-SYNCED.md is never promoted: it documents
#               a design with no production caller, and syncing it would assert the system does
#               something it does not. The file says why and when to sync.
#   SUPERSEDED  each --superseded phrase is grepped in the change directory. A hit in a spec delta is a
#               STOP (it would reach the living specs); a hit elsewhere is listed to confirm it is
#               recorded as rejected. The check is only as complete as the list of reversed
#               decisions, so the plan prints the design.md lines that record one.
#   RENAMED     a renamed requirement keeps its body under the new heading.
#   SCENARIOS   a MODIFIED requirement whose delta has fewer scenarios than the living one is merged,
#               not replaced (lib/change.sh spec_merge), and after --apply every requirement's
#               scenario count is compared before and after: none may fall unless design.md names
#               the scenarios removed.
#
# It moves nothing: `git mv` to the archive stays the skill's step 4. Plain POSIX sh and awk.
ROOT=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/change.sh"

usage="usage: archive-change.sh <id> [--apply] [--superseded '<old phrase>']..."
id=""; apply=""; nsup=0
w=$(mktemp -d "${TMPDIR:-/tmp}/archive-change.XXXXXX") || exit 2
trap 'rm -rf "$w"' EXIT INT TERM
: > "$w/sup"
while [ $# -gt 0 ]; do
  case $1 in
    --apply) apply=1 ;;
    --superseded) [ -n "${2:-}" ] || { echo "usage: --superseded needs a phrase" >&2; exit 2; }; printf '%s\n' "$2" >> "$w/sup"; nsup=$((nsup + 1)); shift ;;
    -*) echo "$usage" >&2; exit 2 ;;
    *) id=$1 ;;
  esac
  shift
done
[ -n "$id" ] || { echo "$usage" >&2; exit 2; }
c="$ROOT/$CHANGES_DIR/$id"
[ -d "$c" ] || { echo "STOP $CHANGES_DIR/$id does not exist"; exit 1; }

stops=0
say() { printf '%s\n' "$*"; }
stop() { say "  STOP    $*"; stops=$((stops + 1)); }

say "archive-change $id: promote $CHANGES_DIR/$id/specs into $SPECS_DIR"
found=""
for s in "$c"/specs/*/spec.md; do
  [ -f "$s" ] || continue
  found=1; cap=$(basename "$(dirname "$s")")
  if [ -f "$(dirname "$s")/NOT-SYNCED.md" ]; then
    why=$(grep -v '^[[:space:]]*$' "$(dirname "$s")/NOT-SYNCED.md" | grep -v '^#' | head -1 | cut -c1-110)
    say "  HELD    $cap: NOT-SYNCED.md holds it back, so it is not promoted (${why:-no reason given in the file})"
    continue
  fi
  echo "$cap" >> "$w/caps"
  spec_merge plan "$cap" "$s" "$ROOT/$SPECS_DIR/$cap/spec.md" "$c/design.md" > "$w/plan.$cap"
  [ -s "$w/plan.$cap" ] || stop "$cap: the delta has no requirement in an ADDED, MODIFIED, REMOVED or RENAMED section"
  while IFS="$(printf '\t')" read -r op name detail; do
    [ "$name" = "-" ] && name=""
    case $op in
      STOP) stop "$cap: ${name:+\"$name\": }$detail" ;;
      *) say "  $(printf '%-7s' "$op") $cap: ${name:+\"$name\"}${name:+${detail:+: }}$detail" ;;
    esac
  done < "$w/plan.$cap"
done
if [ -z "$found" ]; then
  if grep -qE '^skip_specs:[[:space:]]*true[[:space:]]*$' "$c/.openspec.yaml" 2>/dev/null; then say "  NONE    no spec delta, and .openspec.yaml says skip_specs: true"
  else stop "no spec delta, and no 'skip_specs: true' in $CHANGES_DIR/$id/.openspec.yaml"; fi
fi

# ── Superseded wording ────────────────────────────────────────────────────────────────────────────
if [ "$nsup" -eq 0 ]; then
  say "  CHECK   superseded wording: no --superseded phrase given. List each decision this change reversed (below, and any a review round changed) and re-run with --superseded '<old phrase>' for each"
else
  while IFS= read -r p; do
    hits=$(cd "$c" && grep -rnF -- "$p" . 2>/dev/null | sed 's|^\./||')
    if [ -z "$hits" ]; then say "  ok      superseded wording \"$p\": not in $CHANGES_DIR/$id"; continue; fi
    printf '%s\n' "$hits" | while IFS= read -r h; do
      case $h in
        specs/*) echo "S" ;;
        *) echo "C" ;;
      esac
    done > "$w/kinds"
    printf '%s\n' "$hits" | while IFS= read -r h; do
      case $h in
        specs/*) say "  STOP    superseded wording \"$p\" in a delta, which would reach the living specs: $(printf '%s' "$h" | cut -c1-140)" ;;
        *) say "  CHECK   superseded wording \"$p\" in $(printf '%s' "$h" | cut -c1-140) (not in a delta: confirm it is recorded as rejected)" ;;
      esac
    done
    stops=$((stops + $(grep -c '^S' "$w/kinds")))
  done < "$w/sup"
fi
if [ -f "$c/design.md" ]; then
  grep -niE 'supersed|rejected|reversed|instead of|no longer|dropped' "$c/design.md" | cut -c1-140 | sed 's/^/  design  /'
fi

if [ "$stops" -gt 0 ]; then say "archive-change $id: $stops STOP(s); nothing written. Fix the delta (or design.md) and re-run"; exit 1; fi
if [ -z "$apply" ]; then say "archive-change $id: the plan is clean; re-run with --apply to write $SPECS_DIR"; exit 0; fi

# ── Apply, then hold every requirement's scenario count ──────────────────────────────────────────
[ -f "$w/caps" ] || { say "archive-change $id: nothing to promote"; exit 0; }
bad=0
while IFS= read -r cap; do
  living="$ROOT/$SPECS_DIR/$cap/spec.md"
  : > "$w/before"; [ -f "$living" ] && scenario_counts "$living" > "$w/before"
  spec_merge apply "$cap" "$c/specs/$cap/spec.md" "$living" "$c/design.md" > "$w/new.$cap" || { say "  FAIL    $cap: the merge did not run"; bad=1; continue; }
  mkdir -p "$(dirname "$living")" && cp "$w/new.$cap" "$living"
  scenario_counts "$living" > "$w/after"
  # A requirement's count may fall only where the plan replaced it with design.md naming the drop,
  # or removed it. A rename carries its count to the new name.
  awk -F'\t' -v plan="$w/plan.$cap" '
    BEGIN { while ((getline l < plan) > 0) { split(l, f, "\t")
              if (f[1] == "RENAME") { from = f[3]; sub(/^from "/, "", from); sub(/"$/, "", from); ren[from] = f[2] }
              if (f[1] == "REMOVE") gone[f[2]] = 1
              if (f[1] == "REPLACE" && f[3] ~ /design\.md names/) named[f[2]] = 1 } }
    FNR == NR { after[$2] = $1; next }
    { n = ($2 in ren) ? ren[$2] : $2
      if (n in gone || n in named) next
      if (!(n in after)) { print "  FAIL    " n ": was in the living spec with " $1 " scenario(s), and is gone"; bad = 1; next }
      if (after[n] < $1) { print "  FAIL    " n ": " $1 " scenario(s) before, " after[n] " after"; bad = 1 } }
    END { exit bad }' "$w/after" "$w/before" || bad=1
  say "  wrote   ${living#"$ROOT"/} ($(awk -F'\t' '{ s += $1 } END { print s + 0 }' "$w/after") scenario(s) in $(wc -l < "$w/after" | tr -d ' ') requirement(s))"
done < "$w/caps"
if [ "$bad" -ne 0 ]; then say "archive-change $id: a scenario count FELL; the living specs are written but wrong. Inspect 'git diff $SPECS_DIR' and restore what was lost"; exit 1; fi
say "archive-change $id: promoted; no requirement lost a scenario. Next: the cost line, the outcome row, then git mv (the skill's steps 2-4)"
