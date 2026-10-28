#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
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
# Rule 1 is one test per line, never a parse of the shell (a parser in regular expressions keeps
# losing to the next form): a line that holds the word `mktemp`, and is not a whole-line comment,
# passes only when the same line, outside any trailing comment, holds `${TMPDIR:-`, or when it
# ends in a trailing comment `# shell-hygiene: <why>` with no quote after its `#`, where a
# reviewer reads the exemption. Rule 2 reads a pipe from printf or echo into awk on one line,
# whose program is the first quoted word after awk that holds a brace (it may run on over later
# lines), and judges every tracked *.awk file whole, for `awk -f`. A pipe continued onto the next
# line is not followed (#303's KNOWN LIMITS).
#
# The set is the repository's own file list (`git ls-files -z`), never a list typed here: every
# tracked *.sh and *.awk, and every tracked file with no extension whose first line is `#!…sh`. An
# unreadable *.sh or *.awk fails; any other unreadable file (a dangling symlink) is not a script
# and is skipped. Falsified first on fixtures that hold each shape; they spell the words @MK@ and
# @AWK@ until they are written, so this file's own scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need git awk sed xargs tr ln

# The program is a here-doc handed to a plain command, not one inside $( ): a shell parsing a
# here-doc within a command substitution trips on its quotes and parentheses.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
# The files that can be shell or awk, each read once for its first line; the rest leave ARGV.
BEGIN {
  WORD = "(^|[^A-Za-z0-9_])" W "([^A-Za-z0-9_]|$)"
  for (i = 1; i < ARGC; i++) {
    f = ARGV[i]; k = ""
    if (f ~ /\.awk$/) k = "awk"; else if (f ~ /\.sh$/) k = "sh"
    rc = (getline first < f); close(f)
    if (rc < 0) { if (k != "") print f ":0: unreadable, so it was not checked"; ARGV[i] = ""; continue }
    if (k == "" && f !~ /\.[^\/]*$/ && rc > 0 && first ~ /^#!.*sh([ \t]|$)/) k = "sh"
    if (k == "") ARGV[i] = ""; else { kindof[f] = k; kept++ }
  }
  # With every argument gone awk would read its stdin.
  if (!kept) ARGV[ARGC++] = "/dev/null"
}
# early(p): the awk program p can exit before its input ends (an exit outside END).
function early(p) {
  sub(/END[ \t]*\{.*/, "", p)
  return p ~ /(^|[^A-Za-z0-9_])exit([^A-Za-z0-9_]|$)/
}
function say(n, what, text) { sub(/^[ \t]+/, "", text); print FILENAME ":" n ": " what ": " text }
# marked(l): l ends in the trailing comment `# shell-hygiene: <why>`, with no quote after its `#`.
function marked(l) { return l ~ /[ \t]#[ \t]*shell-hygiene:[ \t]*[^ \t'"][^'"]*$/ }
# fed(pre): the command before a pipe is printf or echo; the cut is the last `|` outside quotes.
function fed(pre,   i, c, q, cut) {
  q = ""; cut = 0
  for (i = 1; i <= length(pre); i++) {
    c = substr(pre, i, 1)
    if (q == "") { if (c == "'" || c == "\"") q = c; else if (c == "|") cut = i }
    else if (c == q) q = ""
  }
  pre = substr(pre, cut + 1); gsub(/'[^']*'/, "", pre)
  return pre ~ /(^|[^A-Za-z0-9_-])(printf|echo)([ \t]|$)/
}
# program(r, text): judge the awk program in r, the words after `awk`: the first quoted word that
# holds a brace, or one left open, which the lines after it continue.
function program(r, text,   c, i, p) {
  if (r ~ /(^|[ \t])-f([ \t"']|$)/) return
  while (match(r, /['"]/)) {
    c = substr(r, RSTART, 1); r = substr(r, RSTART + 1)
    i = index(r, c)
    if (!i) { open = c; prog = r; ostart = FNR; ohead = text; return }
    p = substr(r, 1, i - 1); r = substr(r, i + 1)
    if (index(p, "{")) { if (early(p)) say(FNR, PIPE, text); return }
  }
}
# awkfile_end(): judge the *.awk file just read whole; called when the next file, or the input,
# starts or ends, so it names the file it read, not FILENAME.
function awkfile_end() {
  if (kind == "awk" && early(whole)) print afile ":1: an awk program that exits before its input ends, so a pipe into it can break"
  kind = ""
}
FNR == 1 { awkfile_end(); afile = FILENAME; kind = kindof[FILENAME]; open = ""; whole = "" }
kind == "awk" { whole = whole $0 "\n"; next }
open != "" {
  if (index($0, open)) { s = $0; sub(open ".*", "", s); if (early(prog "\n" s)) say(ostart, PIPE, ohead); open = "" }
  else prog = prog "\n" $0
  next
}
/^[ \t]*#/ { next }
marked($0) { next }
{
  code = $0; sub(/[ \t]#.*/, "", code)
  if ($0 ~ WORD && code !~ /\$\{TMPDIR:-/) say(FNR, W " with no ${TMPDIR:-/tmp} template on its line (macOS puts it in /var/folders)", $0)
  r = $0
  while (open == "" && match(r, /\|[ \t]*awk([ \t]|$)/)) {
    pre = substr(r, 1, RSTART - 1); r = substr(r, RSTART + RLENGTH)
    if (fed(pre)) program(r, $0)
  }
}
END { awkfile_end() }
AWK
# sh_scan FILE...: one line per offence, `file:line: what: text`. W is the word rule 1 looks for,
# spelled in two halves so this file's own lines do not hold it. LC_ALL=C: the scan matches bytes,
# and a tracked binary must not stop awk on a multibyte error.
sh_scan() { LC_ALL=C awk -v W="mk""temp" -v PIPE="printf into an awk that exits early" "$SCAN_AWK" "$@"; }

d=$(scratch)
# fixture NAME: the lines on stdin, with @MK@ and @AWK@ spelled out, as $d/NAME.
fixture() { mkdir -p "$(dirname "$d/$1")"; sed -e 's/@MK@/mk''temp/g' -e 's/@AWK@/awk/g' > "$d/$1"; }
# expect NAME [LINES]: every line of fixture NAME that ends `# case`, and each of LINES (a case that
# cannot end in a comment: one inside a quote), is named by the scan, and no other.
expect() {
  out=$(cd "$d" && sh_scan "$1")
  want=" $(grep -n '# case$' "$d/$1" | cut -d: -f1 | tr '\n' ' ') ${2:-} "
  for n in $want; do
    case $out in *"$1:$n: "*) ok "falsified: $1 line $n is named" ;;
      *) fail "missed $1 line $n: $(sed -n "${n}p" "$d/$1") (scan: $out)" ;; esac
  done
  extra=$(printf '%s\n' "$out" | grep . | while IFS= read -r l; do n=${l#"$1":}; n=${n%%:*}; case $want in (*" $n "*) ;; (*) printf '%s\n' "$l" ;; esac; done)
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
s=$(@MK@ "$MY_TMPDIR/x.XXXXXX")  # case
s=$(@MK@ "$TMPDIR/x.XXXXXX")  # case
s=$(@MK@ -td x.XXXXXX)  # case
s=$(@MK@ "$x/a.XXXXXX")  # case: not ${TMPDIR:-/tmp}
echo "# shell-hygiene: x"; e=$(@MK@)  # case
@MK@ -d  # case
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '$4 == a { print; exit }')  # case
echo "$x" | @AWK@ '/^## / { on = 1; next } on && /^- / { exit } on { print }'  # case
echo "a|b" | @AWK@ '{ print; exit }'  # case
printf '%s\n' "$x" | @AWK@ "\$1 == 1 { print; exit }"  # case
printf '%s\n' "$big" | @AWK@ '
  NR > 2 { exit }
  { print }'
EOF
# 18 is named though it ends in a comment: the line's own case marker is not a `# case` line end.
# 25: the program left open over three lines.
expect bad.sh "18 25"
fixture bin/tool <<'EOF'
#!/bin/sh
t=$(@MK@)
EOF
fixture bad.awk <<'EOF'
$1 == "x" { print $2; exit }
END { print "none" }
EOF
ln -s "$d/nowhere" "$d/gone.sh"
out=$(cd "$d" && sh_scan bin/tool bad.awk gone.sh)
case $out in *"bin/tool:2: "*) ok "falsified: a script with no extension and a #!/bin/sh line is read" ;;
  *) fail "missed the bare call in an extensionless #!/bin/sh script (scan: $out)" ;; esac
case $out in *"bad.awk:1: an awk program that exits before"*) ok "falsified: an awk program file that exits early is named, whatever feeds it" ;;
  *) fail "missed the awk program file that exits early (scan: $out)" ;; esac
case $out in *"gone.sh:0: unreadable"*) ok "falsified: a *.sh that cannot be read fails, never passes unread" ;;
  *) fail "an unreadable *.sh passed unread (scan: $out)" ;; esac

# ...and what is not an offence stays quiet: a ${TMPDIR:-} template, a marked line, a comment, an
# awk that reads all its input, a producer that is not printf, and a dangling symlink that is not a
# script.
fixture good.sh <<'EOF'
t=$(@MK@ "${TMPDIR:-/tmp}/x.XXXXXX") || exit 1
q=$(@MK@ -d 2>/dev/null "${TMPDIR:-/tmp}/q.XXXXXX") || die "@MK@ failed"
d=$(@MK@ -d "$d/case.XXXXXX")  # shell-hygiene: under the scratch directory, made in TMPDIR
for t in sh git @MK@ rm; do command -v "$t"; done  # shell-hygiene: a list of tools, not a call
# a bare @MK@ ignores $TMPDIR: t=$(@MK@)
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '!got && $4 == a { print; got = 1 }')
printf '%s\n' "$x" | @AWK@ '$1 == "a" { f = 1 } END { exit !f }'
printf '%s\n' "$x" | @AWK@ -f "$prog"
echo "a|b" | @AWK@ '{ n++ } END { print n }'
git show HEAD:x | @AWK@ '{ print; exit }'
EOF
fixture good.awk <<'EOF'
{ src = src $0 "\n" }
END { if (src == "") exit 1; print src; exit }
EOF
ln -s "$d/nowhere" "$d/gone"
out=$(cd "$d" && sh_scan good.sh good.awk gone)
[ -z "$out" ] && ok "a \${TMPDIR:-} template, a marked line, a comment, an awk that reads to the end and a dangling non-script pass" \
  || fail "the scan names what is not an offence: $out"

# ── This project ────────────────────────────────────────────────────────────────────────────────
git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { fail "cannot check: $ROOT is not a git checkout"; finish; }
# Fail closed: a listing git could not produce, or an awk that did not finish, is never clean. The
# list is NUL-separated (-z), so a path git would quote reaches awk as it is, and git's stderr is
# kept out of it.
list="$d/files"
git -C "$ROOT" ls-files -z > "$list" 2> "$d/ls.err" || { fail "git ls-files failed in $ROOT, so no script was checked: $(cat "$d/ls.err")"; finish; }
[ -s "$list" ] || { fail "git ls-files found no file in $ROOT, so nothing was checked"; finish; }
out=$(cd "$ROOT" && LC_ALL=C xargs -0 awk -v W="mk""temp" -v PIPE="printf into an awk that exits early" "$SCAN_AWK" < "$list" 2> "$d/scan.err") \
  || { fail "the scan did not finish in $ROOT: $(head -3 "$d/scan.err")"; finish; }
n=$(cd "$ROOT" && LC_ALL=C xargs -0 awk 'BEGIN { for (i = 1; i < ARGC; i++) { f = ARGV[i]; rc = (getline l < f); close(f); if (f ~ /\.(sh|awk)$/ || (rc > 0 && f !~ /\.[^\/]*$/ && l ~ /^#!.*sh([ \t]|$)/)) n++ }; print n + 0; exit }' < "$list" | awk '{ s += $1 } END { print s + 0 }')
if [ -z "$out" ]; then
  ok "no temporary file outside \$TMPDIR and no printf into an early-exit awk in $ROOT's $n tracked shell and awk files"
else
  printf '%s\n' "$out" | sed -n '1,20s/^/FAIL /p'
  fail "shell hygiene in $ROOT: $(printf '%s\n' "$out" | wc -l | tr -d ' ') offence(s), the first 20 above"
fi
finish
