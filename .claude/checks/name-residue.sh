#!/bin/sh
# name-residue: no tracked file names the product's earlier name, in any case, in its text or
# its path (OPS-104). The owner's ruling of 2026-10-30 left no exception: no legacy reader, no
# shim, the records renamed too. A machine set up under the earlier name is migrated by a one-off
# script outside the repository, so nothing here reads what it left. Git history and past PR
# titles are not files and keep the earlier name.
#
# Every tracked file is read, binary-attributed ones too (git grep -a): a binary attribute is a
# diff setting, and a name inside such a file still ships.
#
# The name split into pieces on one line counts as the name: each line is compared with every
# character but a letter deleted, so any joiner goes (quotes, +, ., a Join's commas and brackets,
# however many pieces). Out of reach, and so not claimed: a split across lines, and pieces joined
# through a variable whose name holds letters (`x=ab; y="${x}cd"` leaves "xabyxcd"). The split
# name is allowed in the two files that must build it, and nothing else:
#   - this file, which needs the name to look for it;
#   - framework/internal/template/oldmodel_test.go, whose fixture gives back the bytes the old
#     model shipped, which oldmodel.go's hashes are of.
# The plain name is allowed nowhere, those two included.
#
# Repo-only: the template does not ship it.
. "$(dirname "$0")/lib.sh"

old="clau""ductor"
split_ok=".claude/checks/name-residue.sh framework/internal/template/oldmodel_test.go"

# residue DIR: the tracked files of DIR's repository that name the earlier name in their text or
# their path, and those outside split_ok that build it from quoted pieces. Fails (status 2) when
# DIR is not a repository or git cannot read it, so an unread tree is never a clean one.
residue() {
  git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 2
  _r=$(mktemp "${TMPDIR:-/tmp}/name-residue.XXXXXX")
  git -C "$1" grep -a -i -l -e "$old" >"$_r"; rc=$?
  [ "$rc" -le 1 ] || { rm -f "$_r"; return 2; }
  git -C "$1" ls-files | grep -i -e "$old" >>"$_r"
  # Every tracked line, with every character but a letter deleted, in one awk pass: whatever
  # joins the pieces on one line (quotes, +, ., a Join's commas and brackets) goes with it.
  git -C "$1" grep -a -n -e '' >"$_r.c"; rc=$?
  [ "$rc" -le 1 ] || { rm -f "$_r" "$_r.c"; return 2; }
  # LC_ALL=C: awk reads bytes, so a binary file's bytes are not a multibyte error.
  LC_ALL=C awk -v ok=" $split_ok " -v n="$old" '{
    f = $0; sub(/:.*/, "", f); if (index(ok, " " f " ")) next
    l = $0; sub(/^[^:]*:[0-9]*:/, "", l); l = tolower(l); gsub(/[^a-z]/, "", l)
    if (index(l, n)) print f
  }' "$_r.c" >>"$_r" || { rm -f "$_r" "$_r.c"; return 2; }
  sort -u "$_r"
  rm -f "$_r" "$_r.c"
}

t=$(mktemp -d "${TMPDIR:-/tmp}/name-residue.XXXXXX")
trap 'rm -rf "$t"' EXIT

# The negative cases first, so a pass below means the scan looked: the name in a file's text, in
# a path, in upper case, in a binary-attributed file, split into quoted pieces; and a directory
# that is not a repository fails rather than passing as clean.
git -C "$t" init -q
printf 'run %s now\n' "$old" > "$t/text.md"
printf 'x\n' > "$t/$old-path.txt"
printf 'X_%s=1\n' "$(printf '%s' "$old" | tr '[:lower:]' '[:upper:]')" > "$t/upper.sh"
printf '%s\n' "$old" > "$t/blob.bin"
printf '*.bin binary\n' > "$t/.gitattributes"
a=$(printf '%s' "$old" | cut -c1-3); b=$(printf '%s' "$old" | cut -c4-6); c=$(printf '%s' "$old" | cut -c7-)
p=$(printf '%s' "$old" | cut -c1-4); q=$(printf '%s' "$old" | cut -c5-)
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$t/split.go"
printf 'n = "%s" + "%s" + "%s"\n' "$a" "$b" "$c" > "$t/three.go"
printf '$n = "%s" . "%s";\n' "$p" "$q" > "$t/dot.php"
printf 'n := strings.Join([]string{"%s", "%s"}, "")\n' "$p" "$q" > "$t/join.go"
printf 'clean, and Claude is not the name\n' > "$t/clean.md"
git -C "$t" add -A
got=$(residue "$t" | tr '\n' ' ')
want=$(printf '%s\n' "$old-path.txt" blob.bin dot.php join.go split.go text.md three.go upper.sh | sort | tr '\n' ' ')
if [ "$got" = "$want" ]; then
  ok "the scan finds the earlier name in text, a path, upper case, a binary file, and split by +, ., a Join and into three"
else
  fail "the scan's own negative case found: '$got' (want the eight planted files)"
fi
mkdir "$t/not-a-repo"
if GIT_CEILING_DIRECTORIES="$t" residue "$t/not-a-repo" >/dev/null; then
  fail "a directory that is not a repository passed as clean"
else
  ok "a directory that is not a repository fails the scan"
fi

if found=$(residue "$ROOT"); then
  if [ -z "$found" ]; then
    ok "no tracked file in $ROOT names the earlier name (split forms only in: $split_ok)"
  else
    fail "tracked files name the earlier name (rename it; only the two files above may build it from pieces):"
    printf '%s\n' "$found" | sed 's/^/     /'
  fi
else
  fail "could not read $ROOT's tracked files (not a repository, or git failed)"
fi
finish
