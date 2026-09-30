# conf.sh: sourced (never run) by the operating model's scripts. Sets ROOT and the defaults,
# then reads the project's .claude/project.conf over them.
#
# ROOT comes from this file's own location when the caller passes it (CONF_FROM), else from
# git: a hook runs from wherever the session happens to be, and a context script may be run
# from a subdirectory. A caller that already knows ROOT sets it first; conf.sh keeps it.
#
# Usage from .claude/<dir>/x.sh:   ROOT=$(cd "$(dirname "$0")/../.." && pwd); . "$ROOT/.claude/lib/conf.sh"

if [ -z "${ROOT:-}" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || ROOT=$(pwd)
fi

PROJECT_NAME="Project"
PROJECT_SLUG="project"
STATUS_MARK="◆"
OWNER_ROLE="owner"
OWNER_NAME=""
MAIN_BRANCH="main"
BRANCH_CHANGE="change/"
BRANCH_FIX="fix/"
BRANCH_OPS="ops/"
JOURNAL="docs/development-journal.md"
INSIGHTS="docs/insights-log.md"
ROADMAP="docs/roadmap.md"
OWNER_QUEUE="docs/owner-queue.md"
ADR_DIR="docs/adr"
INSIGHT_AREAS=""
PROPOSALS="markdown"
CHANGES_DIR="changes"
SPECS_DIR="specs"
REVIEW_PAGE="none"
GATE_RUN="scripts/ci/run-local.sh"
GATE="scripts/ci/gate.sh"
GATE_STEPS="scripts/ci/steps.sh"
GATE_CLEAN_ROOM="none"
GATE_REMOTE_WORKFLOW=""
GATE_DISPLAY_CONTEXTS=""
GATE_FAIL_PATTERN=""
MQ_ORPHAN_SHAPES="*@WT@/*/.claude/hooks/*"
MQ_DOCKER="0"
MQ_COLIMA="0"
FORMAT_CMD=""
FORMAT_EXT=""

# shellcheck disable=SC1091
[ -f "$ROOT/.claude/project.conf" ] && . "$ROOT/.claude/project.conf"

# focus_file BRANCH: the per-branch focus file the status line reads and status-write.sh
# writes. Keyed by project AND branch, so two repos, or two lanes of one, never show each
# other's focus.
focus_file() {
  printf '%s/.claude/%s-status-%s.txt' "$HOME" "$PROJECT_SLUG" "$(printf '%s' "$1" | tr '/' '-')"
}

# fetch_main: bring origin/<main> up to date for a context script; 0 when origin/<main> is then
# readable. With CONTEXT_OFFLINE=1 (the checks set it) it fetches nothing and reports only
# whether a ref is already there, so a check never touches the network or the remote refs.
fetch_main() {
  [ "${CONTEXT_OFFLINE:-}" = 1 ] || git fetch -q origin "$MAIN_BRANCH" 2>/dev/null || return 1
  git rev-parse -q --verify "origin/$MAIN_BRANCH" >/dev/null 2>&1
}

# count_rows ERE FILE: how many lines match, or CANNOT CHECK when the file is missing. (`grep -c`
# prints 0 AND exits 1 on no match, so `$(grep -c … || echo 0)` prints "0" twice.)
count_rows() {
  if [ -f "$2" ]; then grep -cE "$1" "$2"; else echo "CANNOT CHECK ($2 is missing)"; fi
}

# hook_note EVENT TEXT: allow, and tell Claude. On exit 0 a hook's plain stdout and stderr
# reach only the debug log; `hookSpecificOutput.additionalContext` is the channel that reaches
# the model. One line, JSON-escaped. The same text goes to stderr for the debug log.
hook_note() {
  echo "$2" >&2
  _esc=$(printf '%s' "$2" | sed 's/\\/\\\\/g; s/"/\\"/g' | tr '\n\t' '  ')
  printf '{"hookSpecificOutput":{"hookEventName":"%s","additionalContext":"%s"}}\n' "$1" "$_esc"
}
