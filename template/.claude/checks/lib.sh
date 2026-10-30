# lib.sh: sourced by each check. Sets ROOT (the project root) and small assertion helpers.
#
# ok MSG / fail MSG      record one result; `finish` exits 1 if any fail was recorded
# expect_rc WANT GOT MSG compare exit codes
# scratch                a fresh temp dir, removed on exit
# payload CMD [CWD]      a Bash PreToolUse payload for CMD, as JSON (needs jq)
# bare_env; bare CMD     build the no-stridal environment (ADR-0006: BARE_DIR, BARE_PATH,
#                        BARE_HOME), then run CMD inside it (bare_exec CMD: the same, replacing
#                        this shell)
# bare_assert            0 inside that environment, 1 (and why) outside it under a whole-set run,
#                        2 run alone or under CHECKS_DOUBLE_PASS=1
# bare_report            bare_assert reported as ok or fail (returns 0 or 1); 2: the caller re-runs
# unmarked CMD           run CMD with CHECKS_BARE, CHECKS_SET and CHECKS_DOUBLE_PASS cleared
# slow SECONDS           SECONDS times STRIDAL_TEST_SLOW, rounded up: a deadline to wait on
# wait_for SECONDS CMD   run CMD until it succeeds, or `slow SECONDS` have passed (status 1)
#
# A check that cannot run what it checks must FAIL, never pass: a skipped check reads exactly
# like a passing one (*A control needs a named addressee*: a green result names its subject).

if [ -z "${ROOT:-}" ]; then
  ROOT=$(cd "$(dirname "$0")/../.." && pwd)
fi
. "$ROOT/.claude/lib/conf.sh"
# Context scripts run by a check must not fetch or call gh (fetch_main in conf.sh).
CONTEXT_OFFLINE=1; export CONTEXT_OFFLINE

_fails=0
ok() { echo "ok   $*"; }
fail() { echo "FAIL $*"; _fails=$((_fails + 1)); }
finish() { [ "$_fails" -eq 0 ]; exit $?; }
expect_rc() {
  if [ "$1" = "$2" ]; then ok "$3"; else fail "$3 (exit $2, want $1)"; fi
}
need() {
  for _t in "$@"; do
    command -v "$_t" >/dev/null 2>&1 || { fail "cannot run: $_t is not installed"; finish; }
  done
}

# Created here, in the check's own shell, not inside `$(scratch)`: a trap set in a command
# substitution's subshell fires when that subshell exits, deleting the directory at once.
# pwd -P: macOS reaches /var through a symlink, and git reports the resolved path.
_scratch=$(mktemp -d "${TMPDIR:-/tmp}/checks.XXXXXX") && _scratch=$(cd "$_scratch" && pwd -P)
# people.sh widens these two traps around its decoy and its #71 block, restoring them word for word: change all together.
# They read $_scratch when they fire: run.sh's inner run points it at the environment to remove.
trap 'rm -rf "$_scratch"' EXIT
trap 'rm -rf "$_scratch"; exit 1' INT TERM
scratch() { printf '%s' "$_scratch"; }

payload() {
  jq -cn --arg c "$1" --arg d "${2:-}" '{tool_name:"Bash", cwd:$d, tool_input:{command:$c}}'
}

# ── Time under load (OPS-168): the checks' half of internal/testwait ──
# A check waits on the condition it needs, never on a guessed sleep, and a deadline is only how long
# it may take before the check gives up: a passing case never reaches it, so it can be long. slow
# stretches one by STRIDAL_TEST_SLOW (a factor, default 1; unset, not a number, below 1 or
# infinite, it is 1), as testwait.Scale does. A duration a check ASSERTS is never scaled: its bound
# is the least the broken behaviour must cost (a stopped 30 s command that took 30 s was not
# stopped), so load cannot reach it and the case still fails when the behaviour breaks.
# checks/wall-clock.sh holds every clock read in a check to a reason.
# The factor must be a whole decimal number ("2x" is not 2, as ParseFloat refuses it), and the
# rounding up ignores a float's noise, so 1.1 times 50 is 55, not 56.
slow() {
  awk -v f="${STRIDAL_TEST_SLOW:-1}" -v s="$1" 'BEGIN {
    if (f !~ /^[+]?([0-9]+[.]?[0-9]*|[.][0-9]+)([eE][+-]?[0-9]+)?$/) f = 1; else f += 0
    if (!(f >= 1) || f == f + 1) f = 1
    v = s * f; c = int(v); if (v - c > 1e-9) c++; printf "%d", c }'
}
# One look every tenth of a second, and one more once the deadline has passed, so a condition that
# came true just as it passed still counts. Ticks, not the clock: under load they only run long.
wait_for() {
  _wft=$(( $(slow "$1") * 10 )) _wfi=0; shift
  until "$@"; do
    [ "$_wfi" -lt "$_wft" ] || return 1
    sleep 0.1; _wfi=$((_wfi + 1))
  done
}

# ── The no-stridal environment (ADR-0006): PATH without stridal, and a HOME with no panel ──
# One helper for checks/run.sh (a whole-set run builds it once and runs every check inside it) and
# checks/no-stridal.sh (run alone, or under CHECKS_DOUBLE_PASS=1, it re-runs the set inside it),
# so the two cannot drift. bare_env builds it in this script's scratch: BARE_DIR is its directory
# (CHECKS_BARE's value), BARE_HOME ($BARE_DIR/home) its HOME, BARE_PATH its PATH; BARE_* names, so a
# caller's own np, H or p is never overwritten. bare overrides only HOME, PATH, CI and
# STRIDAL_LOCK_HELD; every other exported variable passes through, the CHECKS_* markers included.
bare_env() {
  BARE_DIR="$(scratch)/bare"; mkdir -p "$BARE_DIR"
  # A directory that holds stridal is replaced by a copy of it made of links to everything else,
  # so dropping the binary never drops jq, git or node installed beside it.
  BARE_PATH=""; _bi=0; _bifs=$IFS; IFS=:
  for _bp in $PATH; do
    if [ -n "$_bp" ] && [ -e "$_bp/stridal" ]; then
      _bi=$((_bi + 1)); _bf="$BARE_DIR/path$_bi"; mkdir -p "$_bf"
      for _bx in "$_bp"/* "$_bp"/.[!.]*; do
        [ -e "$_bx" ] || continue
        [ "${_bx##*/}" = stridal ] || ln -s "$_bx" "$_bf/${_bx##*/}" 2>/dev/null
      done
      _bp=$_bf
    fi
    BARE_PATH="$BARE_PATH${BARE_PATH:+:}$_bp"
  done
  IFS=$_bifs
  BARE_HOME="$BARE_DIR/home"; mkdir -p "$BARE_HOME"
  # A contributor without stridal still has a git identity.
  printf '[user]\n\tname = check\n\temail = check@example.com\n[commit]\n\tgpgsign = false\n[init]\n\tdefaultBranch = main\n' > "$BARE_HOME/.gitconfig"
}
# bare_exec replaces the calling shell with CMD (run.sh `exec`s itself into the environment); bare
# is the same in a subshell, for every other caller.
bare_exec() { exec env HOME="$BARE_HOME" PATH="$BARE_PATH" CI= STRIDAL_LOCK_HELD= "$@"; }
bare() { (bare_exec "$@"); }

# bare_assert: where this check runs, from the real state, never the marker alone (a stale or
# exported CHECKS_BARE must not pass a run that stripped nothing). With CHECKS_BARE set: 0 when
# stridal is off PATH, HOME is the environment's, and HOME has no ~/.stridal; else 1. Under a
# whole-set run with no environment (CHECKS_SET=whole, no CHECKS_BARE, no CHECKS_DOUBLE_PASS=1): 1.
# Otherwise (run alone, or the double pass): 2. A 1 prints why.
bare_assert() {
  if [ -n "${CHECKS_BARE:-}" ]; then
    if command -v stridal >/dev/null 2>&1; then
      echo "CHECKS_BARE is set, but stridal is on PATH ($(command -v stridal))"; return 1
    fi
    if [ "${HOME:-}" != "$CHECKS_BARE/home" ]; then
      echo "CHECKS_BARE is set, but HOME is ${HOME:-unset}, not the environment's $CHECKS_BARE/home"; return 1
    fi
    if [ -e "$HOME/.stridal" ]; then
      echo "CHECKS_BARE is set, but $HOME/.stridal exists"; return 1
    fi
    return 0
  fi
  if [ "${CHECKS_SET:-}" = whole ] && [ "${CHECKS_DOUBLE_PASS:-}" != 1 ]; then
    echo "a whole-set run.sh ran the checks outside the no-stridal environment (CHECKS_SET=whole, no CHECKS_BARE)"; return 1
  fi
  return 2
}
# bare_report: bare_assert's answer as a result, for no-stridal.sh's part 2 and the fixture that
# falsifies it, so both run this code: 0 reported ok (inside the environment), 1 reported fail (and
# why), 2 reported nothing (run alone or the double pass: the caller re-runs the set).
bare_report() {
  _bw=$(bare_assert); _brc=$?
  case $_brc in
    0) ok "the process checks run inside the no-stridal environment ($CHECKS_BARE), started by a whole-set run.sh: not run again" ;;
    1) fail "$_bw" ;;
  esac
  return $_brc
}

# unmarked CMD: CMD with the run markers cleared (empty reads as unset everywhere), for a check that
# starts a whole-set run.sh or a no-stridal.sh of its own: it sets the ones its case needs after
# this, and never inherits the run it is inside.
unmarked() { env CHECKS_BARE= CHECKS_SET= CHECKS_DOUBLE_PASS= "$@"; }

# new_repo DIR: an empty git repo with one commit, identity set, quiet.
new_repo() {
  mkdir -p "$1"
  git -C "$1" init -q -b "${MAIN_BRANCH:-main}" 2>/dev/null || { git -C "$1" init -q && git -C "$1" checkout -q -b "${MAIN_BRANCH:-main}"; }
  git -C "$1" config user.email check@example.com
  git -C "$1" config user.name check
  git -C "$1" config commit.gpgsign false
}
