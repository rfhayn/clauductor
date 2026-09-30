# lib.sh: sourced by each check. Sets ROOT (the project root) and small assertion helpers.
#
# ok MSG / fail MSG      record one result; `finish` exits 1 if any fail was recorded
# expect_rc WANT GOT MSG compare exit codes
# scratch                a fresh temp dir, removed on exit
# payload CMD [CWD]      a Bash PreToolUse payload for CMD, as JSON (needs jq)
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
trap 'rm -rf "$_scratch"' EXIT
trap 'rm -rf "$_scratch"; exit 1' INT TERM
scratch() { printf '%s' "$_scratch"; }

payload() {
  jq -cn --arg c "$1" --arg d "${2:-}" '{tool_name:"Bash", cwd:$d, tool_input:{command:$c}}'
}

# new_repo DIR: an empty git repo with one commit, identity set, quiet.
new_repo() {
  mkdir -p "$1"
  git -C "$1" init -q -b "${MAIN_BRANCH:-main}" 2>/dev/null || { git -C "$1" init -q && git -C "$1" checkout -q -b "${MAIN_BRANCH:-main}"; }
  git -C "$1" config user.email check@example.com
  git -C "$1" config user.name check
  git -C "$1" config commit.gpgsign false
}
