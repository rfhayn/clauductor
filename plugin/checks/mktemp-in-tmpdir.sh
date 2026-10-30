#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# Every temporary file a script makes lands under $TMPDIR (OPS-150). On macOS a bare `mktemp` (and
# `-d`, `-t`, `-p`) ignores $TMPDIR and creates under the `confstr` user temp directory
# (/var/folders/…/T), which Claude Code's sandbox refuses, so a session-start script read the change
# queue as UNKNOWN on 2026-10-28. `mktemp -d "${TMPDIR:-/tmp}/<name>.XXXXXX"` lands where the
# sandbox allows, on every OS.
#
# One test per line, never a parse of the shell (a parser in regular expressions kept losing to the
# next form, three review rounds running): a line that holds the word `mktemp`, and is not a
# whole-line comment, passes only when the same line, outside any trailing comment, holds
# `${TMPDIR:-`, or when it ends in the trailing comment `# mktemp-in-tmpdir: <why>` with no quote
# after its `#`, where a reviewer reads the exemption.
#
# The set is the repository's own file list (`git ls-files -z`), never a list typed here: every
# tracked *.sh, and every tracked file with no extension whose first line is `#!…sh`. Each reaches
# awk as `./<path>`, so a name like `a=b.sh` is read as a file, not as an assignment. An unreadable
# *.sh fails; any other unreadable file (a dangling symlink) is not a script and is skipped.
#
# The reach is the whole repository by default: a project's own scripts run in the same macOS
# sandbox as the model's, so its bare call fails the same way. MKTEMP_CHECK_PATHS in
# .claude/project.conf (read through lib/conf.sh) narrows it to the git pathspecs it names, e.g.
# ".claude scripts/ci", for a project with vendored scripts it does not edit; a narrowed list that
# names no tracked file fails rather than passing empty.
# Falsified first on fixtures; they spell the word @MK@ until they are written, so this file's own
# scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need git awk sed xargs tr ln

# The program is a here-doc handed to a plain command, not one inside $( ): a shell parsing a
# here-doc within a command substitution trips on its quotes and parentheses. W is the word, given
# in two halves by the caller so this file's own lines do not hold it.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
# The files that can be shell, each read once for its first line; the rest leave ARGV.
BEGIN {
  WORD = "(^|[^A-Za-z0-9_])" W "([^A-Za-z0-9_]|$)"
  MARK = "[ \t]#[ \t]*" W "-in-tmpdir:[ \t]*[^ \t'\"][^'\"]*$"
  for (i = 1; i < ARGC; i++) {
    f = ARGV[i]; sh = (f ~ /\.sh$/)
    rc = (getline first < f); close(f)
    if (rc < 0) { if (sh) print name(f) ":0: unreadable, so it was not checked"; ARGV[i] = ""; continue }
    if (!sh && f !~ /\.[^\/]*$/ && rc > 0 && first ~ /^#!.*sh([ \t]|$)/) sh = 1
    if (sh) kept++; else ARGV[i] = ""
  }
  # With every argument gone awk would read its stdin.
  if (!kept) ARGV[ARGC++] = "/dev/null"
}
function name(f) { sub(/^\.\//, "", f); return f }
/^[ \t]*#/ { next }
$0 ~ MARK { next }
{
  code = $0; sub(/[ \t]#.*/, "", code)
  if ($0 ~ WORD && code !~ /\$\{TMPDIR:-/) {
    l = $0; sub(/^[ \t]+/, "", l)
    print name(FILENAME) ":" FNR ": " W " with no ${TMPDIR:-/tmp} template on its line (macOS puts it in /var/folders): " l
  }
}
AWK
# scan FILE...: one line per offence, `file:line: what: text`. LC_ALL=C: the scan matches bytes, and
# a tracked binary must not stop awk on a multibyte error.
scan() { LC_ALL=C awk -v W="mk""temp" "$SCAN_AWK" "$@"; }
# scan_repo DIR [PATHSPECS]: scan every tracked file of the repository at DIR (under PATHSPECS, a
# space-separated list, when given), each as ./<path>; the list is NUL-separated (-z), so a path
# git would quote reaches awk as it is, and git's stderr is kept out of it. Fails closed: a listing
# git could not produce, an empty one, one with no shell script in it (a pathspec naming only docs),
# or an awk that did not finish, is never clean. set -f: a pathspec's `*` is git's to match, not the
# shell's. The count of scripts read is left in $d/count.
scan_repo() {
  (set -f; git -C "$1" ls-files -z -- ${2:-} > "$d/files" 2> "$d/ls.err") || { echo "git ls-files failed in $1: $(cat "$d/ls.err")"; return 2; }
  [ -s "$d/files" ] || { echo "git ls-files found no file in $1${2:+ under MKTEMP_CHECK_PATHS=\"$2\"}"; return 2; }
  (cd "$1" && xargs -0 sh -c 'for f do printf "./%s\000" "$f"; done' sh < "$d/files" > "$d/files.dot") \
    || { echo "the file list could not be prefixed in $1"; return 2; }
  (cd "$1" && LC_ALL=C xargs -0 awk 'BEGIN { for (i = 1; i < ARGC; i++) { f = ARGV[i]; rc = (getline l < f); close(f); if (f ~ /\.sh$/ || (rc > 0 && f !~ /\.[^\/]*$/ && l ~ /^#!.*sh([ \t]|$)/)) n++ }; print n + 0; exit }' < "$d/files.dot") \
    | awk '{ s += $1 } END { print s + 0 }' > "$d/count"
  [ "$(cat "$d/count")" -gt 0 ] || { echo "no tracked shell script in $1${2:+ under MKTEMP_CHECK_PATHS=\"$2\"}, so nothing was checked"; return 2; }
  (cd "$1" && LC_ALL=C xargs -0 awk -v W="mk""temp" "$SCAN_AWK" < "$d/files.dot" 2> "$d/scan.err") \
    || { echo "the scan did not finish in $1: $(head -3 "$d/scan.err")"; return 2; }
}

d=$(scratch)
# fixture NAME: the lines on stdin, with @MK@ spelled out, as $d/NAME.
fixture() { mkdir -p "$(dirname "$d/$1")"; sed -e 's/@MK@/mk''temp/g' > "$d/$1"; }

# ── Falsified first: every line of bad.sh ending `# case` must be named, and no other ────────────
fixture bad.sh <<'EOF'
t=$(@MK@) || exit 1  # case
d=$(@MK@ -d)  # case
q=$(@MK@ -d 2>/dev/null) || q=  # case
p=$(@MK@ -t probe.XXXXXX)  # case
b=`@MK@`  # case
if true; then @MK@ -q; fi  # case
if @MK@ -d >/dev/null; then :; fi  # case
x=$(TMPDIR=/x @MK@)  # case
exec @MK@  # case
t=$(env @MK@)  # case
echo "it's"; t=$(@MK@); echo "it's"  # case
x="a #b"; t=$(@MK@)  # case
sh -c 't=$(@MK@)'  # case
s=$(@MK@ "$d/x.XXXXXX")  # case
s=$(@MK@ "$MY_TMPDIR/x.XXXXXX")  # case
s=$(@MK@ "$TMPDIR/x.XXXXXX")  # case
s=$(@MK@ -td x.XXXXXX)  # case
echo "# @MK@-in-tmpdir: x"; e=$(@MK@)  # case
@MK@ -d  # case
awk -F\" '{ print $4 }' "$f"
tmp=$(@MK@ -d)  # case
s=$(@MK@ "$x/a.XXXXXX")  # case: not ${TMPDIR:-/tmp}
echo " # @MK@-in-tmpdir: x"; e=$(@MK@)  # case
t=$(@MK@) # @MK@-in-tmpdir:
EOF
out=$(cd "$d" && scan ./bad.sh)
# 24: a marker with no reason, which cannot also end in `# case`.
want=" $(grep -n '# case' "$d/bad.sh" | cut -d: -f1 | tr '\n' ' ') 24 "
for n in $want; do
  case $out in *"bad.sh:$n: "*) ok "falsified: bad.sh line $n is named" ;;
    *) fail "missed bad.sh line $n: $(sed -n "${n}p" "$d/bad.sh") (scan: $out)" ;; esac
done
extra=$(printf '%s\n' "$out" | grep . | while IFS= read -r l; do n=${l#bad.sh:}; n=${n%%:*}; case $want in (*" $n "*) ;; (*) printf '%s\n' "$l" ;; esac; done)
[ -z "$extra" ] && ok "...and bad.sh names no other line" || fail "bad.sh: the scan names lines that are not cases: $extra"

fixture bin/tool <<'EOF'
#!/bin/sh
t=$(@MK@)
EOF
ln -s "$d/nowhere" "$d/gone.sh"
out=$(cd "$d" && scan ./bin/tool ./gone.sh)
case $out in *"bin/tool:2: "*) ok "falsified: a script with no extension and a #!/bin/sh line is read" ;;
  *) fail "missed the bare call in an extensionless #!/bin/sh script (scan: $out)" ;; esac
case $out in *"gone.sh:0: unreadable"*) ok "falsified: a *.sh that cannot be read fails, never passes unread" ;;
  *) fail "an unreadable *.sh passed unread (scan: $out)" ;; esac

# ...and what is not an offence stays quiet: a ${TMPDIR:-} template, a marked line, a comment, a
# message, and a dangling symlink that is not a script.
fixture good.sh <<'EOF'
t=$(@MK@ "${TMPDIR:-/tmp}/x.XXXXXX") || exit 1
q=$(@MK@ -d 2>/dev/null "${TMPDIR:-/tmp}/q.XXXXXX") || die "@MK@ failed"
tmp=${TMPDIR:-/tmp}; w=$(@MK@ -d "${tmp%/}/w.XXXXXX")
d=$(@MK@ -d "$d/case.XXXXXX")  # @MK@-in-tmpdir: under the scratch directory, made in TMPDIR
for t in sh git @MK@ rm; do command -v "$t"; done  # @MK@-in-tmpdir: a list of tools, not a call
# a bare @MK@ ignores $TMPDIR: t=$(@MK@)
EOF
ln -s "$d/nowhere" "$d/gone"
out=$(cd "$d" && scan ./good.sh ./gone)
[ -z "$out" ] && ok "a \${TMPDIR:-} template, a marked line, a comment and a dangling non-script pass" \
  || fail "the scan names what is not an offence: $out"

# A tracked name awk would take for an assignment (`a=b.sh`) is scanned all the same.
r="$d/repo"; new_repo "$r"
printf 't=$(%s)\n' "mk""temp" > "$r/a=b.sh"
git -C "$r" add -A && git -C "$r" commit -qm init
out=$(scan_repo "$r")
case $out in *"a=b.sh:1: "*) ok "falsified: a tracked script named like an awk assignment (a=b.sh) is scanned" ;;
  *) fail "a tracked a=b.sh was not scanned (scan: $out)" ;; esac

# MKTEMP_CHECK_PATHS narrows the reach: a bare call under the paths it names fails, one outside
# passes, and a list that names no tracked file fails rather than passing empty.
r="$d/narrow"; new_repo "$r"; mkdir -p "$r/.claude" "$r/vendor"
printf 't=$(%s)\n' "mk""temp" > "$r/.claude/x.sh"; cp "$r/.claude/x.sh" "$r/vendor/y.sh"
git -C "$r" add -A && git -C "$r" commit -qm init
out=$(scan_repo "$r" ".claude scripts/ci")
case $out in *".claude/x.sh:1: "*) ok "MKTEMP_CHECK_PATHS: a bare call under a named path still fails" ;;
  *) fail "MKTEMP_CHECK_PATHS: the bare call under .claude/ was not named (scan: $out)" ;; esac
case $out in *"vendor/y.sh"*) fail "falsified: MKTEMP_CHECK_PATHS did not narrow the scan; vendor/y.sh was read (scan: $out)" ;;
  *) ok "falsified: MKTEMP_CHECK_PATHS narrows the scan, so a bare call outside its paths passes" ;; esac
out=$(scan_repo "$r" "nowhere")
case $? in 2) ok "MKTEMP_CHECK_PATHS naming no tracked file fails, never passes empty ($out)" ;;
  *) fail "MKTEMP_CHECK_PATHS naming no tracked file passed: $out" ;; esac
printf '# notes\n' > "$r/README.md"; git -C "$r" add -A && git -C "$r" commit -qm docs
out=$(scan_repo "$r" "README.md")
case $? in 2) ok "falsified: MKTEMP_CHECK_PATHS naming files but no shell script fails, never passes with 0 read ($out)" ;;
  *) fail "MKTEMP_CHECK_PATHS naming only a README passed with no script read: $out" ;; esac
# A pathspec's `*` is git's: "*.sh" reaches sub/deep.sh, which the shell's own glob, run from the
# repository's top, would have narrowed to top.sh.
printf 't=$(%s)\n' "mk""temp" > "$r/top.sh"; mkdir -p "$r/sub"; cp "$r/top.sh" "$r/sub/deep.sh"
git -C "$r" add -A && git -C "$r" commit -qm glob
out=$(cd "$r" && scan_repo "$r" "*.sh")
case $out in *"sub/deep.sh:1: "*) ok "falsified: a pathspec's * is matched by git, not expanded by the shell" ;;
  *) fail "the pathspec *.sh did not reach sub/deep.sh (scan: $out)" ;; esac

# ── This project ────────────────────────────────────────────────────────────────────────────────
git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { fail "cannot check: $ROOT is not a git checkout"; finish; }
# A value of only spaces is no value.
case ${MKTEMP_CHECK_PATHS:-} in *[![:space:]]*) ;; *) MKTEMP_CHECK_PATHS="" ;; esac
out=$(scan_repo "$ROOT" "$MKTEMP_CHECK_PATHS") || { fail "$out"; finish; }
n=$(cat "$d/count")
reach="the whole repository"; [ -z "${MKTEMP_CHECK_PATHS:-}" ] || reach="MKTEMP_CHECK_PATHS=\"$MKTEMP_CHECK_PATHS\""
if [ -z "$out" ]; then
  ok "every temporary file is named under \$TMPDIR in $ROOT's $n tracked shell scripts ($reach)"
else
  printf '%s\n' "$out" | sed -n '1,20s/^/FAIL /p'
  fail "temporary files outside \$TMPDIR in $ROOT: $(printf '%s\n' "$out" | wc -l | tr -d ' ') line(s), the first 20 above"
fi
finish
