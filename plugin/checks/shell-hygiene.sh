#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Two shapes in a shell script that work in a terminal and fail inside Claude Code (OPS-150).
#
# 1. A bare `mktemp` (or `mktemp -d`, `-t`, `-p`: anything without a template operand). On macOS it
#    ignores $TMPDIR and creates under the `confstr` user temp directory (/var/folders/…/T), which
#    Claude Code's sandbox refuses, so a session-start script read the change queue as UNKNOWN on
#    2026-10-28. An explicit template, `mktemp -d "${TMPDIR:-/tmp}/<name>.XXXXXX"`, lands where
#    the sandbox allows, on every OS.
# 2. `printf` or `echo` piped into an awk that can exit before its input ends (`{ print …; exit }`
#    outside END). Once awk is gone the rest of the write fails; where SIGPIPE is ignored, as under
#    Claude Code, the shell prints `write error: Broken pipe` on stderr, into whatever reads it.
#    Have awk read all its input instead: `!got && <cond> { print …; got = 1 }`.
#
# The set is the repository's own file list (`git ls-files`: every tracked *.sh and .claude/bin/*),
# never a list typed here. Text inside single quotes is not a call (a fixture's script written by
# printf), nor is a comment. Falsified first on fixtures that hold each shape; they spell the words
# @MK@ and @AWK@ until they are written, so this file's own scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need git awk sed xargs

# The program is a here-doc handed to a plain command, not one inside $( ): a shell parsing a
# here-doc within a command substitution trips on its quotes and parentheses.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
# bare(a): 1 when the words after `mktemp` give it no template operand.
function bare(a) {
  while (1) {
    sub(/^[ \t]+/, "", a)
    if (a ~ /^-[dqu]+([ \t);|&`]|$)/) { sub(/^-[dqu]+/, "", a); continue }
    if (a ~ /^--([ \t]|$)/) { sub(/^--/, "", a); continue }
    if (a ~ /^[0-9]*[<>]/) { sub(/^[0-9]*[<>]+&?[^ \t);|&`]*/, "", a); continue }
    return a == "" || a ~ /^[-);|&`]/
  }
}
# early(p): the awk program p can exit before its input ends (an exit outside END).
function early(p) {
  sub(/END[ \t]*\{.*/, "", p)
  return p ~ /(^|[^A-Za-z0-9_])exit([^A-Za-z0-9_]|$)/
}
function say(what,   t) { t = head; sub(/^[ \t]+/, "", t); print FILENAME ":" start ": " what ": " t }
FNR == 1 { open = 0 }
# A program left open on the line of its pipe ends at the next single quote.
open {
  if (index($0, "'")) { s = $0; sub(/'.*/, "", s); open = 0; if (early(prog "\n" s)) say(PIPE) }
  else prog = prog "\n" $0
  next
}
/^[ \t]*#/ { next }
{
  head = $0; start = FNR
  l = $0; gsub(/'[^']*'/, "''", l); sub(/[ \t]+#.*/, "", l)
  r = l
  while (match(r, /mktemp/)) {
    pre = substr(r, 1, RSTART - 1); r = substr(r, RSTART + 6)
    if (r ~ /^[A-Za-z0-9_.\/-]/) continue
    if (pre !~ /(^|[;&|(`!{]|\$\(|(^|[ \t])(then|do|else|command))[ \t]*$/) continue
    if (bare(r)) say("mktemp with no template, so not under $TMPDIR on macOS")
  }
  # A pipe from printf or echo into awk: the program is the first single-quoted word after awk that
  # holds a brace, or one left open to later lines.
  r = $0
  while (!open && match(r, /\|[ \t]*awk([ \t]|$)/)) {
    pre = substr(r, 1, RSTART - 1); r = substr(r, RSTART + RLENGTH)
    gsub(/'[^']*'/, "", pre); sub(/.*\|/, "", pre)
    if (pre !~ /(^|[^A-Za-z0-9_-])(printf|echo)[ \t]/) continue
    w = r
    while (match(w, /'/)) {
      w = substr(w, RSTART + 1)
      if (!index(w, "'")) { prog = w; open = 1; break }
      p = w; sub(/'.*/, "", p); w = substr(w, length(p) + 2)
      if (index(p, "{")) { if (early(p)) say(PIPE); break }
    }
  }
}
AWK
# sh_scan FILE...: one line per offence, `file:line: what: text`.
sh_scan() { awk -v PIPE="printf into an awk that exits early" "$SCAN_AWK" "$@"; }

d=$(scratch)
# fixture NAME: the lines on stdin, with @MK@ and @AWK@ spelled out, as $d/NAME.
fixture() { sed -e 's/@MK@/mktemp/g' -e 's/@AWK@/awk/g' > "$d/$1"; }

# ── Falsified first: every shape below is a fixture the scan must name ──────────────────────────
fixture bad.sh <<'EOF'
t=$(@MK@) || exit 1
d=$(@MK@ -d)
q=$(@MK@ -d 2>/dev/null) || q=
p=$(@MK@ -t probe)
b=`@MK@`
if true; then @MK@ -q; fi
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '$4 == a { print; exit }')
echo "$x" | @AWK@ '/^## / { on = 1; next } on && /^- / { exit } on { print }'
printf '%s\n' "$big" | @AWK@ '
  NR > 2 { exit }
  { print }'
EOF
out=$(sh_scan "$d/bad.sh")
for n in 1 2 3 4 5 6; do
  case $out in *"bad.sh:$n: mktemp with no template"*) ok "falsified: line $n's bare mktemp is named ($(sed -n "${n}p" "$d/bad.sh"))" ;;
    *) fail "missed the bare mktemp on line $n: $(sed -n "${n}p" "$d/bad.sh") (scan: $out)" ;; esac
done
for n in 7 8 9; do
  case $out in *"bad.sh:$n: printf into an awk that exits early"*) ok "falsified: line $n's pipe into an awk that exits early is named" ;;
    *) fail "missed the early-exit awk on line $n: $(sed -n "${n}p" "$d/bad.sh") (scan: $out)" ;; esac
done

# ...and what is not an offence stays quiet: a template, text in single quotes, a word in a list, a
# path, a comment, an awk that reads all its input, and a producer that is not printf or echo.
fixture good.sh <<'EOF'
t=$(@MK@ "${TMPDIR:-/tmp}/x.XXXXXX") || exit 1
d=$(@MK@ -d "$d/case.XXXXXX")
q=$(@MK@ -d 2>/dev/null "${TMPDIR:-/tmp}/q.XXXXXX")
printf 'm=$(@MK@ -d)\n' > "$d/mut.sh"
for t in sh git @MK@ rm; do command -v "$t"; done
printf '#!/bin/sh\nexec %s "$@"\n' "$(command -v @MK@)" > "$d/slowbin/@MK@"
# a bare @MK@ ignores $TMPDIR: t=$(@MK@)
x=1  # then @MK@
row=$(printf '%s\n' "$tsv" | @AWK@ -F'\t' -v a="$id" '!got && $4 == a { print; got = 1 }')
printf '%s\n' "$x" | @AWK@ '$1 == "a" { f = 1 } END { exit !f }'
git show HEAD:x | @AWK@ '{ print; exit }'
EOF
out=$(sh_scan "$d/good.sh")
[ -z "$out" ] && ok "a template, quoted text, a list word, a path, a comment and an awk that reads to the end pass" \
  || fail "the scan names what is not an offence: $out"

# ── This project ────────────────────────────────────────────────────────────────────────────────
git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { fail "cannot check: $ROOT is not a git checkout"; finish; }
# Fail closed: a listing git could not produce, or a file awk could not read, is never clean.
files=$(git -C "$ROOT" ls-files -- '*.sh' '.claude/bin/*' 2>&1) || { fail "git ls-files failed in $ROOT, so no script was checked: $files"; finish; }
n=$(printf '%s\n' "$files" | grep -c .)
[ "$n" -gt 0 ] || { fail "git ls-files found no shell script in $ROOT, so nothing was checked"; finish; }
out=$(cd "$ROOT" && printf '%s\n' "$files" | tr '\n' '\0' | xargs -0 awk -v PIPE="printf into an awk that exits early" "$SCAN_AWK" 2>&1) \
  || { fail "the scan could not read every script in $ROOT: $(printf '%s\n' "$out" | grep -v '^[^:]*:[0-9]*: ' | head -3)"; finish; }
if [ -z "$out" ]; then
  ok "no bare mktemp and no printf into an early-exit awk in $ROOT's $n tracked shell scripts"
else
  printf '%s\n' "$out" | sed -n '1,20s/^/FAIL /p'
  fail "shell hygiene in $ROOT: $(printf '%s\n' "$out" | wc -l | tr -d ' ') offence(s), the first 20 above"
fi
finish
