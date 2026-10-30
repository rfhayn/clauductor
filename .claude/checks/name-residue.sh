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
# however many pieces). Out of reach, and so not claimed: pieces joined by anything holding a
# letter (a variable, a method such as .concat, a conversion, a format string with its pieces out
# of order), and a split across lines. The split name is allowed in the two files that must build it, and nothing else:
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
# their path, and those outside split_ok that split it on one line with joiners holding no letter. Fails (status 2) when
# DIR is not a repository or git cannot read it, so an unread tree is never a clean one.
residue() {
  git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 2
  _r=$(mktemp "${TMPDIR:-/tmp}/name-residue.XXXXXX")
  git -C "$1" grep -a -i -l -e "$old" >"$_r"; rc=$?
  [ "$rc" -le 1 ] || { rm -f "$_r"; return 2; }
  git -C "$1" ls-files | grep -i -e "$old" >>"$_r"
  # Every tracked line, with every character but a letter deleted, in one awk pass: whatever
  # joins the pieces on one line (quotes, +, ., a Join's commas and brackets) goes with it.
  # A line is "path:number:text", and a path may hold a colon, so the path is the first prefix
  # ending at a colon that `git ls-files` names and that a line number follows (git grep -z would
  # end each line at a NUL too, which awk cannot tell from a NUL inside a binary file's line).
  # A NUL inside a line is turned into \001 first: macOS awk would end a record there.
  git -C "$1" ls-files >"$_r.p" || { rm -f "$_r" "$_r.p"; return 2; }
  git -C "$1" grep -a -n -e '' >"$_r.g"; rc=$?
  [ "$rc" -le 1 ] || { rm -f "$_r" "$_r.g" "$_r.p"; return 2; }
  # LC_ALL=C: tr and awk read bytes, so a binary file's bytes are not an illegal sequence (tr
  # stopped there, cutting the scan short with no error), nor a multibyte error in awk.
  LC_ALL=C tr '\000' '\001' <"$_r.g" >"$_r.c" || { rm -f "$_r" "$_r.g" "$_r.c" "$_r.p"; return 2; }
  [ "$(wc -l <"$_r.c")" -eq "$(wc -l <"$_r.g")" ] || { rm -f "$_r" "$_r.g" "$_r.c" "$_r.p"; return 2; }
  LC_ALL=C awk -v ok=" $split_ok " -v n="$old" -v paths="$_r.p" '
    BEGIN { while ((getline p < paths) > 0) tracked[p] = 1 }
    {
      f = ""; rest = $0; pre = ""
      while ((i = index(rest, ":")) > 0) {
        pre = pre substr(rest, 1, i - 1); rest = substr(rest, i + 1)
        if ((pre in tracked) && rest ~ /^[0-9]+:/) { f = pre; break }
        pre = pre ":"
      }
      if (f == "" || index(ok, " " f " ")) next
      l = rest; sub(/^[0-9]+:/, "", l); l = tolower(l); gsub(/[^a-z]/, "", l)
      if (index(l, n)) print f
    }' "$_r.c" >>"$_r" || { rm -f "$_r" "$_r.g" "$_r.c" "$_r.p"; return 2; }
  sort -u "$_r"
  rm -f "$_r" "$_r.g" "$_r.c" "$_r.p"
}

t=$(mktemp -d "${TMPDIR:-/tmp}/name-residue.XXXXXX")
trap 'rm -rf "$t"' EXIT

# The negative cases first, so a pass below means the scan looked: the name in a file's text, in
# a path, in upper case, in a binary-attributed file, split on one line (also in a file whose path
# holds a colon, and after a NUL byte in a binary file's line); and a directory
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
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$t/a:b.go"
printf '\377\376 not UTF-8\nx\000y = "%s" + "%s"\n' "$p" "$q" > "$t/nul.bin"
printf 'clean, and Claude is not the name\n' > "$t/clean.md"
git -C "$t" add -A
got=$(residue "$t" | tr '\n' ' ')
want=$(printf '%s\n' "$old-path.txt" a:b.go blob.bin dot.php join.go nul.bin split.go text.md three.go upper.sh | sort | tr '\n' ' ')
if [ "$got" = "$want" ]; then
  ok "the scan finds the earlier name in text, a path, upper case, a binary file, and split by +, ., a Join and into three"
else
  fail "the scan's own negative case found: '$got' (want the ten planted files)"
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
