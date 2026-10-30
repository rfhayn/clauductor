#!/bin/sh
# name-residue: no tracked file names the product's earlier name, in any case, in its text or
# its path (OPS-104). The owner's ruling of 2026-10-30 left no exception: no legacy reader, no
# shim, the records renamed too. A machine set up under the earlier name is migrated by a one-off
# script outside the repository, so nothing here reads what it left. Git history and past PR
# titles are not files and keep the earlier name.
#
# The allow-list is empty. The earlier name is spelled in two pieces below, so this file does not
# match itself. Repo-only: the template does not ship it.
. "$(dirname "$0")/lib.sh"

old="clau""ductor"

# residue DIR: the tracked files of DIR's repository that name the earlier name, in their text
# (git grep, case-insensitive) or in their path.
residue() {
  {
    git -C "$1" grep -I -i -l -e "$old" 2>/dev/null
    git -C "$1" ls-files | grep -i -e "$old"
  } | sort -u
}

# The negative case first: a repository holding the name in a file's text, in another's path and
# in another case is caught on each, so a pass below means the scan looked.
t=$(mktemp -d "${TMPDIR:-/tmp}/name-residue.XXXXXX")
trap 'rm -rf "$t"' EXIT
git -C "$t" init -q
printf 'run %s now\n' "$old" > "$t/text.md"
printf 'x\n' > "$t/$old-path.txt"
printf 'X_%s=1\n' "$(printf '%s' "$old" | tr '[:lower:]' '[:upper:]')" > "$t/upper.sh"
printf 'clean\n' > "$t/clean.md"
git -C "$t" add -A
got=$(residue "$t" | tr '\n' ' ')
if [ "$got" = "$old-path.txt text.md upper.sh " ]; then
  ok "the scan finds the earlier name in a file's text, its path and in upper case"
else
  fail "the scan's own negative case found: '$got' (want the three planted files)"
fi

found=$(residue "$ROOT")
if [ -z "$found" ]; then
  ok "no tracked file in $ROOT names the earlier name"
else
  fail "tracked files name the earlier name (rename it; there is no allow-list):"
  printf '%s\n' "$found" | sed 's/^/     /'
fi
finish
