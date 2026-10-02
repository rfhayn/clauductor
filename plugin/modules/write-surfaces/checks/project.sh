#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The write-surfaces module's own check, run by checks/run.sh only while the module is on: this
# project says what a write surface is (WRITE_SURFACE_GLOBS, which has no default that fits every
# project), each pattern matches at least one file in the tree (a pattern that matches nothing
# counts nothing, silently), WRITE_SURFACE_MAX is a number, and the guard rule parses. The rule's
# behaviour is tested whether the module is on or not, by .claude/checks/write-surfaces.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need git
set -f

M=$(cd "$(dirname "$0")/.." && pwd)
sh -n "$M/guard.d/write-surfaces.sh" 2>/dev/null && ok "guard.d/write-surfaces.sh parses" || fail "guard.d/write-surfaces.sh does not parse"
case ${WRITE_SURFACE_MAX:-} in
  '' | *[!0-9]* | 0) fail "WRITE_SURFACE_MAX=\"${WRITE_SURFACE_MAX:-}\" is not a positive number" ;;
  *) ok "WRITE_SURFACE_MAX is $WRITE_SURFACE_MAX: that many added write surfaces or more draws the advisory" ;;
esac
if [ -z "${WRITE_SURFACE_GLOBS:-}" ]; then
  fail "WRITE_SURFACE_GLOBS is empty, so the module counts nothing: name your routes and screens in .claude/project.conf (e.g. \"*app/api/*route.ts *app/*page.tsx\")"
else
  files=$(git -C "$ROOT" ls-files 2>/dev/null)
  # first_match PATTERN: the first tracked file PATTERN matches. A function, not inline in $(…): a
  # case pattern's lone `)` inside a command substitution trips bash's parser.
  first_match() {
    printf '%s\n' "$files" | while IFS= read -r f; do
      # shellcheck disable=SC2254
      case $f in $1) echo "$f"; break ;; esac
    done
  }
  for g in $WRITE_SURFACE_GLOBS; do
    hit=$(first_match "$g")
    if [ -n "$hit" ]; then ok "WRITE_SURFACE_GLOBS pattern $g matches files in this tree (e.g. $hit)"
    else fail "WRITE_SURFACE_GLOBS pattern $g matches no file in this tree, so it counts nothing: a typo, or a surface that no longer exists"; fi
  done
fi
finish
