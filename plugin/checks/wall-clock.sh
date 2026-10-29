#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Every clock read in a check says why load cannot outrun it (OPS-168). A check that times a case
# and asserts a bound guessed at a machine's pace passes on an idle laptop and fails a loaded gate
# with nothing wrong: living-visuals.sh's "the stopped command took 5s" under nine lanes. The bound
# a check asserts is the least the BROKEN behaviour must cost (a stopped 30 s command that took 30 s
# was not stopped), and a deadline it waits on is long and goes through lib.sh's `slow`; both are
# described in lib.sh, where checks/statusline.sh, living-visuals.sh and the rest take them from.
#
# One test per line, as checks/mktemp-in-tmpdir.sh does: a line that is not a whole-line comment
# and holds the word `date` and `+%s` passes only when it ends in the trailing comment
# `# wall-clock: <why>`, with no quote after its `#`, where a reviewer reads the reason: a fixture
# timestamp, or the broken floor its bound sits under.
#
# The set is every check run.sh runs, found the way run.sh finds them (a new file runs the moment it
# exists): the *.sh beside this file, and lib/modules.sh's extension checks (each enabled module's
# checks/*.sh and .claude/local/checks/*.sh). Known limits: only `date +%s` is a clock here, so
# bash's $SECONDS, or a clock read in another language a check calls, is not seen; and a disabled
# module's checks are read only once it is enabled, when run.sh starts running them.
# Falsified first on fixtures; they spell the clock @CLOCK@ until they are written, so this file's
# own scan does not read them as offences.
. "$(dirname "$0")/lib.sh"
need awk sed

# D and S are the two halves of a clock read, given by the caller so this file's lines hold neither.
scan_awk() { SCAN_AWK=$(cat); }
scan_awk <<'AWK'
function name(f) { sub(/^\.\//, "", f); return f }
/^[ \t]*#/ { next }
$0 ~ ("[ \t]#[ \t]*wall-clock:[ \t]*[^ \t'\"][^'\"]*$") { next }
$0 ~ ("(^|[^A-Za-z0-9_])" D "([^A-Za-z0-9_]|$)") && index($0, S) {
  l = $0; sub(/^[ \t]+/, "", l)
  print name(FILENAME) ":" FNR ": a clock read with no '# wall-clock: <why load cannot outrun it>': " l
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
t=$(@CLOCK@)  # wall-clock: 'quoted'
t=$(@CLOCK@)  # wall-clock:
t=$(@CLOCK@)
EOF
out=$(cd "$d" && scan ./bad.sh)
want=" $(grep -n '# case' "$d/bad.sh" | cut -d: -f1 | tr '\n' ' ') 6 7 8 "
for n in $want; do
  case $out in *"bad.sh:$n: "*) ok "falsified: bad.sh line $n is named" ;;
    *) fail "missed bad.sh line $n: $(sed -n "${n}p" "$d/bad.sh") (scan: $out)" ;; esac
done
extra=$(printf '%s\n' "$out" | grep . | while IFS= read -r l; do n=${l#bad.sh:}; n=${n%%:*}; case $want in (*" $n "*) ;; (*) printf '%s\n' "$l" ;; esac; done)
[ -z "$extra" ] && ok "...and bad.sh names no other line" || fail "bad.sh: the scan names lines that are not cases: $extra"

# ...and what is not a clock read, or says why, stays quiet.
fixture good.sh <<'EOF'
t0=$(@CLOCK@)  # wall-clock: a fixture timestamp, not a bound
# [ $(( $(@CLOCK@) - t0 )) -le 4 ] in a comment
day=$(date +%Y-%m-%d)
update "+%s" here
EOF
out=$(cd "$d" && scan ./good.sh)
[ -z "$out" ] && ok "a marked read, a comment, another format and a format with no command pass" \
  || fail "the scan names what is not an offence: $out"

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
