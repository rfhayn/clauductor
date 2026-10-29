#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# No unmarked `date +%s` clock read in a check (OPS-168). A check that times a case and asserts a
# bound guessed at a machine's pace passes on an idle laptop and fails a loaded gate with nothing
# wrong: living-visuals.sh's "the stopped command took 5s" under nine lanes. The bound a check
# asserts is the least the BROKEN behaviour must cost (a stopped 30 s command that took 30 s was not
# stopped), and a deadline it waits on is long and goes through lib.sh's `slow`; both are described
# in lib.sh. This file holds only the clock reads to a reason. A fixed-count poll or a guessed
# `sleep` before an assertion is the same class, and nothing here finds it: use lib.sh's `wait_for`.
#
# One test per line, as checks/mktemp-in-tmpdir.sh does: a line that is not a whole-line comment
# and holds the word `date` and `+%s` passes only when it ends in the trailing comment
# `# wall-clock: <why>`, where a reviewer reads the reason: a fixture timestamp, or the broken floor
# its bound sits under. So that a marker inside a string never counts, the reason holds no double
# quote and no clock read, and neither starts nor ends with a single quote (an apostrophe inside it,
# "the fake's sleep", is fine).
#
# The set is every check run.sh runs, found the way run.sh finds them (a new file runs the moment it
# exists): the *.sh beside this file, and lib/modules.sh's extension checks (each enabled module's
# checks/*.sh and .claude/local/checks/*.sh). Known limits: only a line holding `date` and `+%s` is
# a clock read here, so `date +"%s"`, a clock read through a helper function defined on another
# line, bash's $SECONDS, or a clock in another language a check calls, is not seen; and a disabled
# module's checks are read only once it is enabled, when run.sh starts running them.
# Falsified first on fixtures; they spell the clock @CLOCK@ until they are written, so this file's
# own scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need awk sed

# D and S are the two halves of a clock read, given by the caller so this file's lines hold neither.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
function name(f) { sub(/^\.\//, "", f); return f }
# marked LINE: it ends in `# wall-clock: <reason>`, the reason with no double quote, no clock read,
# and no single quote at either end.
function marked(l) {
  if (!match(l, /[ \t]#[ \t]*wall-clock:[ \t]*[^ \t'"]([^"]*[^ \t'"])?[ \t]*$/)) return 0
  return !index(substr(l, RSTART), S)
}
/^[ \t]*#/ { next }
marked($0) { next }
$0 ~ ("(^|[^A-Za-z0-9_])" D "([^A-Za-z0-9_]|$)") && index($0, S) {
  l = $0; sub(/^[ \t]+/, "", l)
  print name(FILENAME) ":" FNR ": a clock read with no trailing # wall-clock: <why load cannot outrun it> (the reason holds no double quote and no clock read, and no single quote at either end): " l
}
AWK
# scan FILE...: one line per offence, `file:line: what: text`.
scan() { LC_ALL=C awk -v D="da""te" -v S="+""%s" "$SCAN_AWK" "$@"; }

d=$(scratch)
# fixture NAME: the lines on stdin, with @CLOCK@ spelled out, as $d/NAME.
fixture() { sed -e 's/@CLOCK@/da''te +''%s/g' > "$d/$1"; }

# ── Falsified first: every line of bad.sh ending `# case` must be named, and no other ────────────
fixture bad.sh <<'EOF'
t0=$(@CLOCK@)  # case
[ $(( $(@CLOCK@) - t0 )) -le 4 ] && ok "fast"  # case
s=$(@CLOCK@); run; took=$(( $(@CLOCK@) - s ))  # case
now=`@CLOCK@`  # case
echo "# wall-clock: x"; t=$(@CLOCK@)  # case
echo '# wall-clock: x'; t=$(@CLOCK@)  # case
t=$(@CLOCK@)  # wall-clock: a string with a "quote"  # case
t=$(@CLOCK@)  # wall-clock: x; t=$(@CLOCK@)  # case
t=$(@CLOCK@)  # wall-clock: 'quoted'
t=$(@CLOCK@)  # wall-clock:
t=$(@CLOCK@)
t=$(@CLOCK@); echo '# wall-clock: x'
EOF
out=$(cd "$d" && scan ./bad.sh)
# 9-12: a quoted reason, none, no marker, a marker closing a single-quoted string; none can also
# end in `# case`.
want=" $(grep -n '# case' "$d/bad.sh" | cut -d: -f1 | tr '\n' ' ') 9 10 11 12 "
for n in $want; do
  case $out in *"bad.sh:$n: "*) ok "falsified: bad.sh line $n is named" ;;
    *) fail "missed bad.sh line $n: $(sed -n "${n}p" "$d/bad.sh") (scan: $out)" ;; esac
done
extra=$(printf '%s\n' "$out" | grep . | while IFS= read -r l; do n=${l#bad.sh:}; n=${n%%:*}; case $want in (*" $n "*) ;; (*) printf '%s\n' "$l" ;; esac; done)
[ -z "$extra" ] && ok "...and bad.sh names no other line" || fail "bad.sh: the scan names lines that are not cases: $extra"

# ...and what is not a clock read, or says why, stays quiet.
fixture good.sh <<'EOF'
t0=$(@CLOCK@)  # wall-clock: a fixture timestamp, not a bound
t1=$(@CLOCK@)  # wall-clock: the fake's sleep is the bound
# [ $(( $(@CLOCK@) - t0 )) -le 4 ] in a comment
day=$(date +%Y-%m-%d)
update "+%s" here
EOF
out=$(cd "$d" && scan ./good.sh)
[ -z "$out" ] && ok "a marked read (an apostrophe in its reason too), a comment, another format and a format with no command pass" \
  || fail "the scan names what is not an offence: $out"

# ── lib.sh's slow and wait_for, which the checks' deadlines go through ─────────────────────────
# slow reads the factor as testwait.Factor does: a number of at least 1, else 1. Rounded up, past a
# float's noise (1.1 times 50 is 55, not 56).
for c in "-:7:7" "1:7:7" "2:7:14" "1.5:7:11" "1.05:10:11" "1.1:50:55" "+2:7:14" "1e1:2:20" \
  "0.5:7:7" "-3:7:7" "0:7:7" "2x:7:7" "x2:7:7" "abc:7:7" ":7:7" "inf:7:7" "Inf:7:7" "nan:7:7" "1e400:7:7"; do
  f=${c%%:*} r=${c#*:}; s=${r%%:*} w=${r#*:}
  if [ "$f" = - ]; then got=$(unset CLAUDUCTOR_TEST_SLOW; slow "$s"); f=unset; else got=$(CLAUDUCTOR_TEST_SLOW=$f slow "$s"); fi
  [ "$got" = "$w" ] && ok "slow $s with CLAUDUCTOR_TEST_SLOW=$f is $w" || fail "slow $s with CLAUDUCTOR_TEST_SLOW=$f is '$got', want $w"
done
# wait_for looks once more after its deadline, as testwait.For does: a condition first true on that
# look counts, one true only on the look after it does not. Ten ticks at 1 s, so 11 looks.
looks() { _lk=$((_lk + 1)); [ "$_lk" -ge "$want_look" ]; }
for c in "11:0" "12:1"; do
  want_look=${c%%:*} _lk=0
  (unset CLAUDUCTOR_TEST_SLOW; wait_for 1 looks); rc=$?
  expect_rc "${c#*:}" "$rc" "wait_for 1 with the condition first true on look $want_look (the 11th is after the deadline)"
done
_lk=0 want_look=1; wait_for 1 looks && [ "$_lk" = 1 ] && ok "wait_for returns on the first look that holds, never waiting" || fail "wait_for took $_lk looks for a condition true on the first"

# The reach: a copy of this check in a project of its own names a clock read in that project's
# .claude/local/checks, an extension run.sh runs. Falsified by scanning only the directory beside
# this file: red. The copy runs with WALL_CLOCK_INNER=1, which skips this case, so it never recurses.
if [ -z "${WALL_CLOCK_INNER:-}" ]; then
  P="$d/proj"; mkdir -p "$P/.claude/checks" "$P/.claude/lib" "$P/.claude/local/checks"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$P/.claude/lib/"
  cp "$(dirname "$0")/lib.sh" "$0" "$P/.claude/checks/"
  printf 't=$(%s)\n' "da""te +""%s" > "$P/.claude/local/checks/late.sh"
  out=$(cd "$P" && WALL_CLOCK_INNER=1 ROOT="$P" sh ".claude/checks/${0##*/}" 2>&1)
  case $out in *"FAIL .claude/local/checks/late.sh:1: "*) ok "falsified: a clock read in an extension check (.claude/local/checks) is named" ;;
    *) fail "a clock read in .claude/local/checks/late.sh was not named: $(printf '%s\n' "$out" | tail -3)" ;; esac
fi

# ── This project: every check run.sh runs ───────────────────────────────────────────────────────
# run.sh's two lists, read the way it reads them: this directory's *.sh (a plugin's checks/ as much
# as a project's .claude/checks/), and ext_files' extension checks.
here=$(cd "$(dirname "$0")" && pwd)
set -- "$here"/*.sh
if [ -f "$CLAUDUCTOR_FW/lib/modules.sh" ]; then
  . "$CLAUDUCTOR_FW/lib/modules.sh"
  ext_files checks .sh | cut -f2 > "$d/ext"
  while IFS= read -r f; do set -- "$@" "$f"; done < "$d/ext"
fi
for f do shift; [ -f "$f" ] && set -- "$@" "$f"; done
n=$#
[ "$n" -gt 0 ] || { fail "cannot check: no check script found beside $0"; finish; }
# An awk that could not read a file did not check it: that fails, never passes.
out=$(scan "$@" 2> "$d/scan.err") || { fail "the scan did not finish: $(head -3 "$d/scan.err")"; finish; }
out=$(printf '%s\n' "$out" | sed "s|^$ROOT/||" | grep .)
if [ -z "$out" ]; then
  ok "every clock read in the $n checks run.sh runs ($here and the enabled extensions) says why load cannot outrun it"
else
  printf '%s\n' "$out" | sed -n '1,20s/^/FAIL /p'
  fail "clock reads with no reason in the checks run.sh runs: $(printf '%s\n' "$out" | wc -l | tr -d ' ') line(s), the first 20 above"
fi
finish
