#!/bin/sh
# Two shapes in a shell script that work in a terminal and fail inside Claude Code (OPS-150).
#
# 1. A temporary file not named under $TMPDIR. On macOS a bare `mktemp` (and `-d`, `-t`, `-p`)
#    ignores $TMPDIR and creates under the `confstr` user temp directory (/var/folders/…/T), which
#    Claude Code's sandbox refuses, so a session-start script read the change queue as UNKNOWN on
#    2026-10-28. `mktemp -d "${TMPDIR:-/tmp}/<name>.XXXXXX"` lands where the sandbox allows, on
#    every OS.
# 2. `printf` or `echo` piped into an awk that can exit before its input ends (`{ print …; exit }`
#    outside END). Once awk is gone the rest of the write fails; where SIGPIPE is ignored, as under
#    Claude Code, the shell prints `write error: Broken pipe` on stderr, into whatever reads it.
#    Have awk read all its input instead: `!got && <cond> { print …; got = 1 }`.
#
# Rule 1 does not parse the shell, which a regular expression would keep losing to the next form
# (`if mktemp …; then`, `env mktemp`, `$(TMPDIR=/x mktemp)`): EVERY word `mktemp` on a line that is
# not a whole-line comment is an offence unless the text that follows it, up to the end of its
# command, holds both `TMPDIR` (or a variable this file sets from it: `tmp=${TMPDIR:-/tmp}`) and an
# `XXX` template, and no `-t`, `-p` or `--tmpdir`, which choose
# the directory themselves; `<word> failed` is read as a message. A line that is not such a call, or whose template is somewhere else on
# purpose, says so where a reviewer reads it: `# shell-hygiene: <why>`, on that line. Rule 2 reads
# the pipe from printf or echo (on one line, or continued on the next), the program in single or
# double quotes; an awk program in a tracked *.awk file (`awk -f`) is judged as a whole.
#
# The set is the repository's own file list (`git ls-files`), never a list typed here: every
# tracked *.sh, every tracked file whose first line is a `#!…sh` line, and every tracked *.awk.
# Falsified first on fixtures that hold each shape; they spell the words @MK@ and @AWK@ until they
# are written, so this file's own scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need git awk sed xargs tr

# The program is a here-doc handed to a plain command, not one inside $( ): a shell parsing a
# here-doc within a command substitution trips on its quotes and parentheses.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
# early(p): the awk program p can exit before its input ends (an exit outside END).
function early(p) {
  sub(/END[ \t]*\{.*/, "", p)
  return p ~ /(^|[^A-Za-z0-9_])exit([^A-Za-z0-9_]|$)/
}
function say(n, what, text) { sub(/^[ \t]+/, "", text); print FILENAME ":" n ": " what ": " text }
function marked(text) { return text ~ /#[ \t]*shell-hygiene:[ \t]*[^ \t]/ }
# program(r, n, text): judge the awk program in r, the words after `awk`: the first quoted word
# that holds a brace, or one left open, which the lines after it continue.
function program(r, n, text,   c, i, p) {
  if (marked(text)) return
  if (r ~ /(^|[ \t])-f([ \t"']|$)/) return
  while (match(r, /['"]/)) {
    c = substr(r, RSTART, 1); r = substr(r, RSTART + 1)
    i = index(r, c)
    if (!i) { open = c; prog = r; ostart = n; ohead = text; return }
    p = substr(r, 1, i - 1); r = substr(r, i + 1)
    if (index(p, "{")) { if (early(p)) say(n, PIPE, text); return }
  }
}
# fed(seg): the last command of a pipeline segment is printf or echo.
function fed(seg) {
  gsub(/'[^']*'/, "", seg); sub(/.*\|/, "", seg)
  return seg ~ /(^|[^A-Za-z0-9_-])(printf|echo)([ \t]|$)/
}
# awkfile_end(): judge the *.awk file just read whole (called once the next file, or the input, starts
# or ends, so it names the file it read, not FILENAME).
function awkfile_end() {
  if (kind == "awk" && early(whole)) print afile ":1: an awk program that exits before its input ends, so a pipe into it can break"
  kind = ""
}
# undertmp(seg): the words after the call name $TMPDIR, or a variable set from it in this file.
function undertmp(seg,   v) {
  if (seg ~ /TMPDIR/) return 1
  for (v in tmpvar) if (seg ~ ("\\$\\{?" v "([^A-Za-z0-9_]|$)")) return 1
  return 0
}
FNR == 1 {
  awkfile_end(); afile = FILENAME; split("", tmpvar)
  open = ""; pend = 0; whole = ""
  if (FILENAME ~ /\.awk$/) kind = "awk"
  else if (FILENAME ~ /\.sh$/ || $0 ~ /^#!.*sh([ \t]|$)/) kind = "sh"
  else { kind = ""; nextfile }
}
kind == "awk" { whole = whole $0 "\n"; next }
open != "" {
  if (index($0, open)) { s = $0; sub(open ".*", "", s); if (early(prog "\n" s)) say(ostart, PIPE, ohead); open = "" }
  else prog = prog "\n" $0
  next
}
/^[ \t]*#/ { pend = 0; next }
{
  # A variable this file sets from $TMPDIR (`tmp=${TMPDIR:-/tmp}`) names it as well as TMPDIR does.
  if (match($0, /(^|[ \t;(])[A-Za-z_][A-Za-z0-9_]*="?\$\{?TMPDIR([^A-Za-z0-9_]|$)/)) {
    v = substr($0, RSTART, RLENGTH); sub(/^[ \t;(]/, "", v); sub(/=.*/, "", v); tmpvar[v] = 1
  }
  if (!marked($0)) {
    r = $0
    while (match(r, "(^|[^A-Za-z0-9_])" W "([^A-Za-z0-9_]|$)")) {
      # RSTART is the word itself at the start of the line, else the character before it.
      r = substr(r, RSTART + length(W) + (substr(r, RSTART, length(W)) != W))
      # `<word> failed` is a message (`|| die "<word> failed"`), not a call.
      if (r ~ /^[ \t]+failed/) continue
      seg = r; if (match(seg, /[);|&`]/)) seg = substr(seg, 1, RSTART - 1)
      if (!undertmp(seg) || seg !~ /XXX/ || seg ~ /(^|[ \t])(-[A-Za-z]*[tp]|--tmpdir)([ \t=]|$)/) {
        say(FNR, W " with no template under $TMPDIR (macOS puts it in /var/folders)", $0); break
      }
    }
  }
  r = $0
  # A pipe left open at the end of the last line feeds this one.
  if (pend && match(r, /^[ \t]*(\|[ \t]*)?awk([ \t]|$)/)) program(substr(r, RSTART + RLENGTH), pstart, phead)
  pend = 0
  while (open == "" && match(r, /\|[ \t]*awk([ \t]|$)/)) {
    pre = substr(r, 1, RSTART - 1); r = substr(r, RSTART + RLENGTH)
    if (fed(pre)) program(r, FNR, $0)
  }
  # This line ends in a pipe, or in a backslash before one, from printf or echo.
  t = $0; sub(/[ \t]+#[^'"]*$/, "", t); sub(/[ \t]+$/, "", t)
  if (open == "" && t ~ /(\||\\)$/) {
    sub(/[ \t]*(\||\\)$/, "", t)
    if (fed(t)) { pend = 1; pstart = FNR; phead = $0 }
  }
}
END { awkfile_end() }
AWK
# sh_scan FILE...: one line per offence, `file:line: what: text`. W is the word rule 1 looks for,
# spelled in two halves so this file's own lines do not hold it.
# LC_ALL=C: the scan matches bytes, and a tracked binary must not stop awk on a multibyte error.
sh_scan() { LC_ALL=C awk -v W="mk""temp" -v PIPE="printf into an awk that exits early" "$SCAN_AWK" "$@"; }

d=$(scratch)
# fixture NAME: the lines on stdin, with @MK@ and @AWK@ spelled out, as $d/NAME.
fixture() { sed -e 's/@MK@/mk''temp/g' -e 's/@AWK@/awk/g' > "$d/$1"; }
# expect NAME [LINES]: every line of fixture NAME that ends `# case`, and each of LINES (a case that
# cannot end in a comment: one inside a quote, or a backslash continuation), is named by the scan,
# and no other.
expect() {
  out=$(sh_scan "$d/$1")
  want=" $(grep -n '# case$' "$d/$1" | cut -d: -f1 | tr '\n' ' ') ${2:-} "
  for n in $want; do
    case $out in *"$1:$n: "*) ok "falsified: $1 line $n is named" ;;
      *) fail "missed $1 line $n: $(sed -n "${n}p" "$d/$1") (scan: $out)" ;; esac
  done
  extra=$(printf '%s\n' "$out" | grep . | while IFS= read -r l; do n=${l#*"$1":}; n=${n%%:*}; case $want in (*" $n "*) ;; (*) printf '%s\n' "$l" ;; esac; done)
  [ -z "$extra" ] && ok "...and $1 names no other line" || fail "$1: the scan names lines that are not cases: $extra"
}

# ── Falsified first: every shape below is a fixture the scan must name ──────────────────────────
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
u=$(@MK@ -d "${TMPDIR:-/tmp}/a.XXXXXX"; @MK@ -d)  # case
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '$4 == a { print; exit }')  # case
echo "$x" | @AWK@ '/^## / { on = 1; next } on && /^- / { exit } on { print }'  # case
printf '%s\n' "$big" | @AWK@ '
  NR > 2 { exit }
  { print }'
printf '%s\n' "$x" |  # case
  @AWK@ '{ print; exit }'
printf '%s\n' "$x" \
  | @AWK@ '{ print; exit }'
printf '%s\n' "$x" | @AWK@ "\$1 == 1 { print; exit }"  # case
tmp=${TMPDIR:-/tmp}; other=/var/tmp
w=$(@MK@ -d "$other/w.XXXXXX")  # case
w=$(@MK@ -d "$tmpx/w.XXXXXX")  # case
EOF
# 18: the program left open over three lines; 23: the pipe after a backslash.
expect bad.sh "18 23"
fixture bad.awk <<'EOF'
$1 == "x" { print $2; exit }
END { print "none" }
EOF
out=$(sh_scan "$d/bad.awk")
case $out in *"bad.awk:1: an awk program that exits before"*) ok "falsified: an awk program file that exits early is named, whatever feeds it" ;;
  *) fail "missed the awk program file that exits early (scan: $out)" ;; esac

# ...and what is not an offence stays quiet: a template under $TMPDIR, a marked line, a comment, an
# awk that reads all its input (here, over a continued pipe), and a producer that is not printf.
fixture good.sh <<'EOF'
t=$(@MK@ "${TMPDIR:-/tmp}/x.XXXXXX") || exit 1
q=$(@MK@ -d 2>/dev/null "$TMPDIR/q.XXXXXX")
tmp=${TMPDIR:-/tmp}
w=$(@MK@ -d "${tmp%/}/w.XXXXXX")
d=$(@MK@ -d "$d/case.XXXXXX")  # shell-hygiene: $d is this script's scratch, made under $TMPDIR
for t in sh git @MK@ rm; do command -v "$t"; done  # shell-hygiene: a list of tools, not a call
# a bare @MK@ ignores $TMPDIR: t=$(@MK@)
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '!got && $4 == a { print; got = 1 }')
printf '%s\n' "$x" | @AWK@ '$1 == "a" { f = 1 } END { exit !f }'
printf '%s\n' "$x" |
  @AWK@ '{ n++ } END { print n }'
printf '%s\n' "$x" | @AWK@ -f "$prog"
git show HEAD:x | @AWK@ '{ print; exit }'
EOF
fixture good.awk <<'EOF'
{ src = src $0 "\n" }
END { if (src == "") exit 1; print src; exit }
EOF
out=$(sh_scan "$d/good.sh" "$d/good.awk")
[ -z "$out" ] && ok "a template under \$TMPDIR, a marked line, a comment and an awk that reads to the end pass" \
  || fail "the scan names what is not an offence: $out"

# ── This project ────────────────────────────────────────────────────────────────────────────────
git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { fail "cannot check: $ROOT is not a git checkout"; finish; }
# Fail closed: a listing git could not produce, or a file awk could not read, is never clean. The
# list is NUL-separated (-z), so a path git would quote reaches awk as it is, and git's stderr is
# kept out of it.
list="$d/files"
git -C "$ROOT" ls-files -z > "$list" 2> "$d/ls.err" || { fail "git ls-files failed in $ROOT, so no script was checked: $(cat "$d/ls.err")"; finish; }
[ -s "$list" ] || { fail "git ls-files found no file in $ROOT, so nothing was checked"; finish; }
out=$(cd "$ROOT" && LC_ALL=C xargs -0 awk -v W="mk""temp" -v PIPE="printf into an awk that exits early" "$SCAN_AWK" < "$list" 2> "$d/scan.err") \
  || { fail "the scan could not read every file in $ROOT: $(head -3 "$d/scan.err")"; finish; }
n=$(cd "$ROOT" && LC_ALL=C xargs -0 awk 'FNR == 1 { if (FILENAME ~ /\.(sh|awk)$/ || $0 ~ /^#!.*sh([ \t]|$)/) n++; nextfile } END { print n + 0 }' < "$list" | awk '{ s += $1 } END { print s + 0 }')
if [ -z "$out" ]; then
  ok "no temporary file outside \$TMPDIR and no printf into an early-exit awk in $ROOT's $n tracked shell and awk files"
else
  printf '%s\n' "$out" | sed -n '1,20s/^/FAIL /p'
  fail "shell hygiene in $ROOT: $(printf '%s\n' "$out" | wc -l | tr -d ' ') offence(s), the first 20 above"
fi
finish
