#!/bin/sh
# The write-surfaces module's merge-guard rule (ADVISORY, never blocking): a capability change
# (BRANCH_CHANGE, default change/) that ADDS WRITE_SURFACE_MAX (default 4) or more write surfaces
# (files matching WRITE_SURFACE_GLOBS: routes, screens, handlers) is told to consider splitting.
# Standing Tee's merge-guard rule 5, moved into a module.
#
# WHY. Size a change so its SECOND review round finds nothing inside the first round's fixes. In
# Standing Tee the variable was slice size, and of three proxies measured against its merged PRs
# only "added routes + added screens" separated the change that never converged (10 surfaces, four
# rounds) from those that did; lines added and commit count INVERTED the ranking (a data corpus is
# not logic). Across all 19 of its merged change PRs the count was bimodal (10, 4, 1, 1, 1, 1, then
# zeroes), and 4 fired on exactly the two that needed splitting. It is a proxy for how many distinct
# write surfaces one head has to hold at once.
#
# STATED LIMITS: it counts ADDED files only, so a change that heavily MODIFIES existing routes
# scores 0; it cannot tell a correctly large change from an oversized one. It refuses to let breadth
# go unremarked; it does not adjudicate. Advisory because wedging a green merge over a scoping
# judgement trains the habit of bypassing the guard, and that would take the blocking rules with it.
#
# Counted from local git (GUARD_BASE..GUARD_HEAD; the guard has the head by now), not the API:
# offline, and the same answer for the same commits. A rename is not an addition.
# Contract (.claude/local/README.md): GUARD_* and ROOT in the environment; exit 0 allows, each
# stdout line an advisory. This rule never exits 2: a fault is said as an advisory.
set -f   # the globs are patterns for `case`, never for the shell to expand against files
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh" || { echo "CANNOT CHECK — cannot read the project's configuration, so write surfaces were not counted."; exit 0; }

case $GUARD_BRANCH in "${BRANCH_CHANGE:-change/}"*) ;; *) exit 0 ;; esac
if [ -z "${WRITE_SURFACE_GLOBS:-}" ]; then
  echo "WRITE_SURFACE_GLOBS is empty, so no write surface is counted: set it in .claude/project.conf (the module's checks fail until it is)."
  exit 0
fi
max=${WRITE_SURFACE_MAX:-4}
case $max in '' | *[!0-9]*) echo "CANNOT CHECK — WRITE_SURFACE_MAX=\"$max\" is not a number, so write surfaces were not judged."; exit 0 ;; esac

added=$(git -C "$ROOT" diff --no-ext-diff -M --name-only --diff-filter=A "$GUARD_BASE" "$GUARD_HEAD" 2>/dev/null) \
  || { echo "CANNOT CHECK — could not diff $GUARD_BASE..$GUARD_HEAD, so write surfaces were not counted."; exit 0; }
n=0 list=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  for g in $WRITE_SURFACE_GLOBS; do
    # shellcheck disable=SC2254
    case $f in $g) n=$((n + 1)); list="$list$f
"; break ;; esac
  done
done <<EOF
$added
EOF
[ "$n" -ge "$max" ] || exit 0
shown=$(printf '%s' "$list" | head -n 6 | awk 'NR > 1 { printf ", " } { printf "%s", $0 }')
[ "$n" -gt 6 ] && shown="$shown, … and $((n - 6)) more"
echo "PR #$GUARD_PR adds $n write surfaces (WRITE_SURFACE_GLOBS; at most $((max - 1)) passes quietly): $shown. Size a change so its second review round finds nothing inside the first round's fixes. If this is one capability, carry on. If it is several, the cheap split is a roadmap edit, not a mid-review one. Not blocking."
