#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# review-cap.sh: how many review rounds a row's PR gets before what is left goes to KNOWN LIMITS.
#
#   sh .claude/review-cap.sh <row>          the cap, a number ("3")
#   sh .claude/review-cap.sh <row> --why    the cap and where it came from ("3 — risk high, from …")
#
# <row> is a roadmap row's id (OPS-108), its change id (ops/ops-108-…, add-thing) or its branch
# (BRANCH_CHANGE + id). Exit 1, naming why, when the risk cannot be read: the caller then has no cap,
# and a cap guessed would let review stop early or run on.
#
# WHERE THE RISK COMES FROM, one way for every reader (build-change's maxRounds, merge-pr step 3):
#   a change with a proposal   its `**Risk:**` line (CHANGES_DIR/<id>/proposal.md), which the owner
#                              approved with the design;
#   an ops or fix row          a `Risk: low|normal|high` in its Change cell, outside backticks (a
#                              backticked one is the row talking ABOUT a risk), else normal;
#   no row and no proposal     normal, said on --why (a fix/<n>-<slug> from an issue has no row).
# The rounds per risk are REVIEW_CAPS (.claude/project.conf; low 2, normal 3, high 3 by default).
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/change.sh"

[ -n "${1:-}" ] || { echo "usage: sh "$CLAUDUCTOR_FW"/review-cap.sh <row id | change id | branch> [--why]" >&2; exit 1; }
arg=$1; why=""; [ "${2:-}" = --why ] && why=1
case $arg in "$BRANCH_CHANGE"*) arg=${arg#"$BRANCH_CHANGE"} ;; esac

say() {  # say RISK SOURCE: the cap for RISK, and on --why where it came from
  cap=$(printf '%s\n' $REVIEW_CAPS | sed -n "s/^$1:\([0-9][0-9]*\)$/\1/p" | head -1)
  [ -n "$cap" ] || { echo "ERROR: REVIEW_CAPS ($REVIEW_CAPS) has no '$1:<rounds>', so the review cap is UNKNOWN" >&2; exit 1; }
  if [ -n "$why" ]; then echo "$cap — risk $1, from $2"; else echo "$cap"; fi
  exit 0
}

tsv=$(roadmap_queue --tsv) || { printf '%s\n' "$tsv" >&2; echo "ERROR: the roadmap did not parse, so the review cap is UNKNOWN" >&2; exit 1; }
row=$(printf '%s\n' "$tsv" | awk -F'\t' -v a="$arg" '$4 == a || $5 == a { print; exit }')
id=$arg; [ -n "$row" ] && id=$(printf '%s\n' "$row" | cut -f5)

p="$ROOT/$CHANGES_DIR/$id/proposal.md"
# An archived change (archive-change's YYYY-MM-DD-<id>), for a review of what already merged.
[ -f "$p" ] || for a in "$ROOT/$CHANGES_DIR"/archive/????-??-??-"$id"/proposal.md; do [ -f "$a" ] && p=$a; done
if [ -f "$p" ]; then
  id=${p#"$ROOT/$CHANGES_DIR/"}; id=${id%/proposal.md}
  r=$(proposal_field "$p" Risk)
  case $r in low|normal|high) say "$r" "$CHANGES_DIR/$id/proposal.md" ;; esac
  echo "ERROR: $CHANGES_DIR/$id/proposal.md's **Risk:** is '${r:-missing}', not low, normal or high (checks/changes.sh refuses it), so the review cap is UNKNOWN" >&2
  exit 1
fi

if [ -n "$row" ]; then
  rid=$(printf '%s\n' "$row" | cut -f4); kind=$(printf '%s\n' "$row" | cut -f6)
  if [ "$kind" = change ]; then
    echo "ERROR: row $rid is a capability change, and $CHANGES_DIR/$id/proposal.md is not here: its risk is the proposal's, so the review cap is UNKNOWN (propose it first, or run this on its branch)" >&2
    exit 1
  fi
  # The ROADMAP line itself, not the summary column: a parser of the project's own may shorten it.
  line=$(printf '%s\n' "$row" | cut -f1)
  # The Change cell only (cell 3 of `| # | Change | Scope | Deps | Status |`), backticks out.
  risks=$(sed -n "${line}p" "$ROOT/$ROADMAP" | sed 's/`[^`]*`//g' | awk -F'|' '{ print $3 }' | grep -oE 'Risk: (low|normal|high)' | sed 's/^Risk: //' | sort -u)
  case $(printf '%s\n' "$risks" | grep -c .) in
    0) say normal "row $rid (no Risk: in its Change cell)" ;;
    1) say "$risks" "row $rid's Change cell" ;;
    *) echo "ERROR: row $rid's Change cell says more than one risk ($(echo $risks)), so the review cap is UNKNOWN: keep one Risk: and backtick any other mention" >&2; exit 1 ;;
  esac
fi

say normal "no roadmap row or proposal names '$arg'"
