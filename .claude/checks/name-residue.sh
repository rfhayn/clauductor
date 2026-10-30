#!/bin/sh
# name-residue: no tracked file names the product's earlier name, in any case, in its text or
# its path (OPS-104). The owner's ruling of 2026-10-30 left no exception: no legacy reader, no
# shim, the records renamed too. A machine set up under the earlier name is migrated by a one-off
# script outside the repository, so nothing here reads what it left. Git history and past PR
# titles are not files and keep the earlier name.
#
# Every tracked file is read, binary-attributed ones too (git grep -a): a binary attribute is a
# diff setting, and a name inside such a file still ships. Each is read twice: from the index
# (--cached), what a commit ships whatever the working tree holds (a sparse checkout's file, a
# deleted one, one changed in type or shut, one behind a link or folded into another name's case),
# and from the working tree, for an edit not yet staged. git grep skips a tracked symlink, so each
# link's target, the blob `git ls-files -s` names with mode 120000, is read on its own. A
# submodule is another repository's files, not this one's: only its path is read.
#
# The name split into pieces on one line counts as the name: each line, and each tracked path, is
# compared with every character but a letter deleted, so any joiner goes (quotes, +, ., /, a Join's
# commas and brackets, however many pieces). Out of reach, and so not claimed: pieces joined by
# anything holding a letter (a variable, a method such as .concat, a conversion, a format string
# with its pieces out of order), and a split across lines. The split name is allowed in the two
# files that must build it, and nothing else:
#   - this file, which needs the name to look for it;
#   - framework/internal/template/oldmodel_test.go, whose fixture gives back the bytes the old
#     model shipped, which oldmodel.go's hashes are of.
# The plain name is allowed nowhere, those two included.
#
# Repo-only: the template does not ship it.
. "$(dirname "$0")/lib.sh"

old="clau""ductor"
split_ok=".claude/checks/name-residue.sh framework/internal/template/oldmodel_test.go"
tab=$(printf '\t')
nl='
'

# scan DIR ARGS...: git grep over DIR's tracked files, and tracked DIR ARGS...: git ls-files.
# Every setting that shapes what git grep prints is pinned on its call line, so no configuration,
# the repository's or the user's, changes what the scan reads: color.grep (--no-color),
# submodule.recurse (--no-recurse-submodules: a submodule's lines name no path ls-files lists),
# grep.fullName (--no-full-name), grep.column (--no-column), grep.patternType and
# grep.extendedRegexp (-F), grep.lineNumber (-n, given by the caller that reads line numbers),
# core.quotePath (pinned on both calls, so the two quote a path alike). grep.threads changes no
# output; grep.fallbackToNoIndex applies only outside a repository, which residue refuses first.
# Two pins hold settings that today change nothing the scan compares, so no case fails without
# them: a column is digits and a colon, which the letters-only compare deletes, and the one
# pattern, '', matches every line as Perl or extended too. The "shaped" case below sets every grep
# key `git help --config` lists, so a key a later git adds fails the check until it is pinned here.
scan() {
  _d=$1; shift
  git -c core.quotePath=true -C "$_d" grep --no-color --no-recurse-submodules --no-full-name \
    --no-column -F -a "$@"
}
tracked() {
  _d=$1; shift
  git -c core.quotePath=true -C "$_d" ls-files "$@"
}

# credit PATHS LINES: the paths whose lines in LINES (git grep's "path:number:text", a NUL turned
# into \001) hold the name, and those outside split_ok whose lines split it with joiners holding no
# letter. A path may hold a colon, so every prefix ending at a colon that PATHS names and that a
# line number follows is a candidate, and each is read: a file named "x.go:1:<the name's first
# piece>" beside x.go cannot take the credit for x.go's line and hide its split, nor a file named
# "<exempt path>:1:x" pass as the exempt one. (git grep -z would end each path at a NUL too, which
# awk cannot tell from a NUL inside a binary file's line.) Every line git grep prints belongs to a
# tracked path, so a line with no candidate fails (status 3): a scan that reads less than every
# tracked line never reports clean (color forced on in the repository's config was one such road,
# before the pins above). The other way round, a file named "x.go:1:<first piece>" holding "<rest>"
# reads as x.go's line "<first piece>:1:<rest>": x.go is reported though neither file holds the
# name in its own text (nothing is, if x.go is exempt). A false report on a deliberately mirrored
# name, never a missed one.
credit() {
  LC_ALL=C awk -v ok=" $split_ok " -v n="$old" -v paths="$1" '
    BEGIN { while ((getline p < paths) > 0) tracked[p] = 1 }
    {
      rest = $0; pre = ""; read = 0
      while ((i = index(rest, ":")) > 0) {
        pre = pre substr(rest, 1, i - 1); rest = substr(rest, i + 1)
        if ((pre in tracked) && rest ~ /^[0-9]+:/) {
          read++
          l = rest; sub(/^[0-9]+:/, "", l); l = tolower(l)
          if (index(l, n)) print pre
          else if (!index(ok, " " pre " ")) { gsub(/[^a-z]/, "", l); if (index(l, n)) print pre }
        }
        pre = pre ":"
      }
      if (!read) bad++
    }
    END { if (bad) exit 3 }' "$2"
}

# residue DIR: the tracked files under DIR (all of them when DIR is its repository's top, as $ROOT
# is) that name the earlier name in their text, their path or their link target, and those outside
# split_ok that split it on one line with joiners holding no letter. Fails (status 2) when DIR is
# not a repository, git cannot read it, or a regular file the index lists was read by neither scan,
# so an unread tree is never a clean one.
residue() {
  git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 2
  _r=$(mktemp -d "${TMPDIR:-/tmp}/name-residue.XXXXXX") || return 2
  tracked "$1" >"$_r/p" || { rm -rf "$_r"; return 2; }
  # The index's own regular files (mode 100644 or 100755), the authority, must each be read by one
  # of the two scans (-l: those with a line; -L: the empty ones). --cached reads every one but an
  # intent-to-add entry and an unmerged path, which the working tree's scan reads. A file neither
  # reads (such an entry the working tree cannot open) fails the scan rather than passing clean.
  tracked "$1" -s >"$_r/s" || { rm -rf "$_r"; return 2; }
  LC_ALL=C awk -F "$tab" '$1 ~ /^100(644|755) / { print $2 }' "$_r/s" >"$_r/w" ||
    { rm -rf "$_r"; return 2; }
  LC_ALL=C sort -u "$_r/w" >"$_r/want" || { rm -rf "$_r"; return 2; }
  for _a in '-l' '-L' '--cached -l' '--cached -L'; do
    # shellcheck disable=SC2086 # $_a is one or two options, split on purpose
    scan "$1" $_a -e '' >>"$_r/r"; rc=$?
    [ "$rc" -le 1 ] || { rm -rf "$_r"; return 2; }
  done
  LC_ALL=C sort -u "$_r/r" >"$_r/read" || { rm -rf "$_r"; return 2; }
  LC_ALL=C comm -23 "$_r/want" "$_r/read" >"$_r/unread" || { rm -rf "$_r"; return 2; }
  [ ! -s "$_r/unread" ] || { rm -rf "$_r"; return 2; }
  # What credit reads: every line of both scans; each tracked path as its own line 0; and each
  # symlink's target, read from its blob, as the link's line 1, with any newline in it a space.
  scan "$1" -n -e '' >"$_r/g"; rc=$?
  [ "$rc" -le 1 ] || { rm -rf "$_r"; return 2; }
  scan "$1" --cached -n -e '' >>"$_r/g"; rc=$?
  [ "$rc" -le 1 ] || { rm -rf "$_r"; return 2; }
  LC_ALL=C awk '{ print $0 ":0:" $0 }' "$_r/p" >>"$_r/g" || { rm -rf "$_r"; return 2; }
  while IFS= read -r _l; do
    case $_l in 120000\ *) ;; *) continue ;; esac
    _o=${_l#120000 }; _o=${_o%% *}
    _tg=$(git -C "$1" cat-file blob "$_o") || { rm -rf "$_r"; return 2; }
    while :; do
      case $_tg in *"$nl"*) _tg="${_tg%%"$nl"*} ${_tg#*"$nl"}" ;; *) break ;; esac
    done
    printf '%s:1:%s\n' "${_l#*"$tab"}" "$_tg" >>"$_r/g" || { rm -rf "$_r"; return 2; }
  done <"$_r/s"
  # LC_ALL=C: tr and awk read bytes, so a binary file's bytes are not an illegal sequence (tr
  # stopped there, cutting the scan short with no error), nor a multibyte error in awk.
  # A NUL inside a line is turned into \001 first: macOS awk would end a record there.
  LC_ALL=C tr '\000' '\001' <"$_r/g" >"$_r/c" || { rm -rf "$_r"; return 2; }
  [ "$(wc -l <"$_r/c")" -eq "$(wc -l <"$_r/g")" ] || { rm -rf "$_r"; return 2; }
  credit "$_r/p" "$_r/c" >"$_r/found" || { rm -rf "$_r"; return 2; }
  sort -u "$_r/found" || { rm -rf "$_r"; return 2; }
  rm -rf "$_r"
}

t=$(scratch)
# commit DIR: commit what DIR's index holds, whoever runs the check and whatever hooks they set.
commit() {
  git -C "$1" -c user.name=t -c user.email=t@example.com -c commit.gpgsign=false \
    -c core.hooksPath=/dev/null commit -q --no-verify -m case
}

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
# The fail-closed count, fed a line itself: a line whose path git does not list must fail the
# scan, never be skipped. With every setting pinned no road to it is known (grep.fullName from a
# subdirectory was the last; the shaped case below shows it closed), and a count nothing reaches
# is untested unless a case reaches it.
mkdir "$t/credit"
printf 'kept.go\n' > "$t/credit/p"
printf 'kept.go:1:fine\ngone.go:1:n = "%s" + "%s"\n' "$p" "$q" > "$t/credit/c"
credit "$t/credit/p" "$t/credit/c" >/dev/null; rc=$?
expect_rc 3 "$rc" "a line whose path git does not list fails the scan rather than being skipped"
# Every setting that shapes what git grep prints, set against the scan in one repository: color
# forced on, a column, no line numbers, Perl patterns, full names (the scan run from a
# subdirectory, where they bite), submodules recursed into (one under that subdirectory, holding
# the plain name), paths unquoted (a planted file's name holds a byte git quotes). The scan must
# find exactly the planted splits. Unpinned, it failed (status 2): loudly, but falsely.
s="$t/shaped"
git init -q "$t/inner"
printf '%s\n' "$old" > "$t/inner/named.txt"
git -C "$t/inner" add -A
commit "$t/inner"
git init -q "$s"
mkdir "$s/d"
git -C "$s" -c protocol.file.allow=always -c core.hooksPath=/dev/null submodule add -q "$t/inner" d/sub >/dev/null 2>&1 ||
  fail "the shaped case could not add its submodule"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$s/d/split.go"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$s/d/$(printf '\303\251').go"
git -C "$s" add -A
shaped="color.ui=always color.grep=always core.quotePath=false grep.column=true
  grep.extendedRegexp=true grep.fallbackToNoIndex=true grep.fullName=true grep.lineNumber=false
  grep.patternType=perl grep.threads=1 submodule.recurse=true"
for kv in $shaped; do git -C "$s" config "${kv%%=*}" "${kv#*=}"; done
for k in $(git help --config 2>/dev/null | grep '^color\.grep\.'); do git -C "$s" config "$k" red; done
if got=$(residue "$s/d"); then
  got=$(printf '%s\n' "$got" | tr '\n' ' ')
  if [ "$got" = '"\303\251.go" split.go ' ]; then
    ok "no setting of git grep's output changes what the scan finds (color, column, line numbers, patterns, full names, submodules, quoting)"
  else
    fail "a setting of git grep's output changed what the scan finds: '$got' (want '\"\\303\\251.go\" split.go ')"
  fi
else
  fail "a setting of git grep's output failed the scan (status 2): a setting the scan does not pin"
fi
# The authority is git's own list of its grep settings: one this case does not set (a key a later
# git adds) fails here until scan's call line pins it and the case above sets it.
keys=$(git help --config 2>/dev/null | grep '^grep\.')
if [ -z "$keys" ]; then
  fail "git help --config listed no grep.* setting: cannot tell whether the scan pins them all"
else
  miss=""
  for k in $keys; do
    case " $(printf '%s' "$shaped" | tr '\n' ' ') " in *" $k="*) ;; *) miss="$miss $k" ;; esac
  done
  if [ -z "$miss" ]; then ok "every grep.* setting git lists is pinned and set against the scan"
  else fail "git lists grep settings the scan does not pin (pin each in scan, set it in the shaped case):$miss"; fi
fi
# A file named after another and a line number cannot take the credit for that file's line: x.go's
# line 1 reads "x.go:1:<first piece>:2:<rest>", and the longest tracked path, "x.go:1:<first
# piece>", would leave "<rest>" alone. Every candidate is read, so x.go's split is found.
m="$t/mirror"
git init -q "$m"
printf '%s:2:%s\n' "$p" "$q" > "$m/x.go"
printf 'clean\n' > "$m/x.go:1:$p"
git -C "$m" add -A
got=$(residue "$m" | tr '\n' ' ')
if [ "$got" = "x.go " ]; then
  ok "a file named after another file and a line number does not hide that file's split"
else
  fail "a file named 'x.go:1:<first piece>' hid x.go's split: found '$got' (want 'x.go')"
fi
# git grep skips a symlink, but its target ships: one naming the name, one splitting it with a /,
# one at an exempt path naming it (the plain name is allowed nowhere), and two clean ones, one of
# them with a newline in its target. A tracked path that splits the name with a / is found too.
lk="$t/links"
git init -q "$lk"
ln -s "../$old/x" "$lk/plain"
ln -s "$(printf '%s' "$p" | tr '[:lower:]' '[:upper:]')/$q" "$lk/split"
mkdir -p "$lk/framework/internal/template"
ln -s "../$old/x" "$lk/framework/internal/template/oldmodel_test.go"
ln -s ../elsewhere "$lk/clean"
ln -s "$(printf 'else\nwhere')" "$lk/newline"
mkdir "$lk/$p"
printf 'clean\n' > "$lk/$p/$q.txt"
printf 'clean\nclean\n' > "$lk/notes.md"
git -C "$lk" add -A
if got=$(residue "$lk"); then
  got=$(printf '%s\n' "$got" | tr '\n' ' ')
  want="$p/$q.txt framework/internal/template/oldmodel_test.go plain split "
  if [ "$got" = "$want" ]; then
    ok "a symlink whose target names or splits the name, and a path split by a /, are found"
  else
    fail "a symlink's target or a split path was not read: found '$got' (want '$want')"
  fi
else
  fail "the links case failed the scan (status 2): a link's target was not read as one line"
fi
# Every exit by which residue fails closed, reached on that repository: each command whose output
# it reads failing in turn ("cmd:arguments"), having written all it would have, so that no later
# step catches what the exit under test lets through ("short:" cuts tr's output to one line
# instead, which the line count's compare must catch), must fail the scan with status 2, never
# pass on what was read before it. credit is reached through its awk.
open=""
mkdir "$t/stubtmp"
for call in 'mktemp:*' 'git:* ls-files' 'git:* ls-files -s' 'awk:-F*' 'sort:-u */w' \
  'git:* -a -l -e *' 'git:* -a -L -e *' 'git:* --cached -l -e *' 'git:* --cached -L -e *' \
  'sort:-u */r' 'comm:*' 'git:* -a -n -e *' 'git:* --cached -n -e *' 'awk:{*' \
  'git:* cat-file blob *' 'printf:%s:1:%s*' 'tr:*001' 'short:*001' 'awk:-v ok=*' \
  'sort:-u */found'; do
  (
    TMPDIR="$t/stubtmp"
    mktemp() { command mktemp "$@"; _s=$?; case "mktemp:$*" in $call) return 1 ;; esac; return $_s; }
    git() { command git "$@"; _s=$?; case "git:$*" in $call) return 128 ;; esac; return $_s; }
    awk() { command awk "$@"; _s=$?; case "awk:$*" in $call) return 2 ;; esac; return $_s; }
    sort() { command sort "$@"; _s=$?; case "sort:$*" in $call) return 2 ;; esac; return $_s; }
    comm() { command comm "$@"; _s=$?; case "comm:$*" in $call) return 2 ;; esac; return $_s; }
    printf() { command printf "$@"; _s=$?; case "printf:$*" in $call) return 1 ;; esac; return $_s; }
    tr() {
      case "short:$*" in $call) command tr "$@" | head -n 1; return ;; esac
      command tr "$@"; _s=$?; case "tr:$*" in $call) return 1 ;; esac; return $_s
    }
    residue "$lk" >/dev/null
  )
  [ $? -eq 2 ] || open="$open '$call'"
done
if [ -z "$open" ]; then
  ok "each command residue reads, failing, fails the scan (status 2) rather than passing on a partial read"
else
  fail "the scan did not fail (status 2) with these failing:$open"
fi
# A tracked file git grep cannot read in the working tree is read from the index, so its split is
# found: a sparse checkout leaves it out, a deletion not yet committed removes it, a link put in
# its place changes its type, mode 000 shuts it, a link put in place of its directory leads
# elsewhere, and on a filesystem that folds case another name's file stands for it.
sp="$t/sparse"
git init -q "$sp"
mkdir "$sp/a" "$sp/b"
printf 'clean\n' > "$sp/a/ok.txt"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$sp/b/split.go"
git -C "$sp" add -A
commit "$sp"
# unread CASE DIR WANT: residue on DIR must find exactly WANT.
unread() {
  if got=$(residue "$2" 2>/dev/null); then
    if [ "$got" = "$3" ]; then ok "$1: its split is read from the index"
    else fail "$1: found '$got' (want '$3')"; fi
  else
    fail "$1: the scan failed (status 2) rather than reading it from the index"
  fi
}
if git -C "$sp" sparse-checkout set a >/dev/null 2>&1; then
  unread "a sparse checkout's file outside it" "$sp" b/split.go
else
  fail "the sparse case could not set up its sparse checkout"
fi
gone="$t/gone"
git init -q "$gone"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$gone/split.go"
printf 'clean\n' > "$gone/other.go"
git -C "$gone" add -A
rm "$gone/split.go"
unread "a tracked file deleted from the working tree" "$gone" split.go
ln -s other.go "$gone/split.go"
unread "a tracked file turned into a link" "$gone" split.go
rm "$gone/split.go"
git -C "$gone" checkout -q -- split.go
chmod 000 "$gone/split.go"
if [ -r "$gone/split.go" ]; then
  fail "the unreadable case cannot shut a file here (run as root?), so it cannot check the scan"
else
  unread "a tracked file git grep may not open" "$gone" split.go
fi
dl="$t/dirlink"
git init -q "$dl"
mkdir "$dl/b" "$dl/other"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$dl/b/split.go"
printf 'clean\n' > "$dl/other/split.go"
git -C "$dl" add -A
rm -r "$dl/b"
ln -s other "$dl/b"
unread "a tracked file whose directory became a link to a clean one" "$dl" b/split.go
fc="$t/folded"
git init -q "$fc"
printf 'clean\n' > "$fc/lower.go"
git -C "$fc" add -A
h=$(printf 'n = "%s" + "%s"\n' "$p" "$q" | git -C "$fc" hash-object -w --stdin)
git -C "$fc" update-index --add --cacheinfo "100644,$h,Lower.go"
unread "a tracked file whose name another file's stands for (case folded or not)" "$fc" Lower.go
# An intent-to-add file has no blob yet, so only the working tree can be read for it: shut, it is
# read by neither scan, and the scan fails rather than pass clean over what it will hold.
ia="$t/intent"
git init -q "$ia"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$ia/new.go"
git -C "$ia" add -N new.go
chmod 000 "$ia/new.go"
if [ -r "$ia/new.go" ]; then
  fail "the intent-to-add case cannot shut a file here (run as root?), so it cannot check the scan"
elif residue "$ia" >/dev/null 2>&1; then
  fail "an intent-to-add file neither scan could read passed clean"
else
  ok "an intent-to-add file neither scan can read fails the scan"
fi
# A file named after an exempt path and a line number is its own file, not the exempt one.
e="$t/exempt"
git init -q "$e"
mkdir -p "$e/.claude/checks"
printf 'clean\n' > "$e/.claude/checks/name-residue.sh"
printf 'n = "%s" + "%s"\n' "$p" "$q" > "$e/.claude/checks/name-residue.sh:1:x"
git -C "$e" add -A
got=$(residue "$e" | tr '\n' ' ')
if [ "$got" = ".claude/checks/name-residue.sh:1:x " ]; then
  ok "a file named after an exempt path and a line number is not credited to the exempt path"
else
  fail "a file named '<exempt path>:1:x' was credited to the exempt path: found '$got'"
fi

if found=$(residue "$ROOT"); then
  if [ -z "$found" ]; then
    ok "no tracked file in $ROOT names the earlier name (split forms only in: $split_ok)"
  else
    fail "tracked files name the earlier name (rename it; only the two files above may build it from pieces):"
    printf '%s\n' "$found" | sed 's/^/     /'
  fi
else
  fail "could not read every tracked file in $ROOT (not a repository, git failed, or a tracked file is not in the working tree as itself: git status shows it)"
fi
finish
