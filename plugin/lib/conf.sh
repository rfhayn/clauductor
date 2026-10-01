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
ROADMAP_PARSER=""
ROADMAP_NORMALIZER=""
OWNER_QUEUE="docs/owner-queue.md"
ADR_DIR="docs/adr"
INSIGHT_AREAS=""
INSIGHTS_TABLE_HEADING="## Log"
JOURNAL_HEADING='^## Session [0-9]+(\.[0-9]+)* — [0-9]{4}-[0-9]{2}-[0-9]{2}[^ ]* — [^ ].* — .+'
RECORDS_BASELINE=""
CHANGES_LEGACY=""
COMPOUND_MAX_SESSIONS="3"
PROPOSALS="markdown"
CHANGES_DIR="changes"
SPECS_DIR="specs"
REVIEW_PAGE="none"
SCENARIO_IDS="required"
TEST_GLOBS="*_test.go *.test.* *.spec.* test_*.py *_test.py tests/ test/ __tests__/ spec/"
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
MODULES=""
PLAYBOOK="docs/playbook.md"
AGENTS_MD_MAX_BYTES="16000"
AGENTS_MD_MAX_ROW="320"
CHANGE_RECORD_EXTRA=""
GATE_QUICK_FLAGS="--quick"

# shellcheck disable=SC1091
[ -f "$ROOT/.claude/project.conf" ] && . "$ROOT/.claude/project.conf"

# roadmap_queue MODE [ARGS] [FILE]: THE way a script reads the change queue. Every consumer calls
# this, never a parser by path (checks/roadmap.sh fails one that does), so a project that keeps its
# own parser sets ROADMAP_PARSER once and every reader follows (docs/roadmap.md, *Plugging in your
# own parser*, states the contract). Modes: --text, --check, --tsv, --queued [kind]; an explicit
# roadmap path may follow. Exit 0 = read; anything else = the queue is UNKNOWN (not empty).
#
# --queued is derived here from --tsv, so a plugged parser need not implement it. --tsv goes through
# ROADMAP_NORMALIZER when one is set (a filter from the parser's columns to the contract's), then is
# held to the contract: a row the consumers would misread is an error, never a silently wrong queue.
roadmap_queue() {
  case "${1:-}" in
    --queued)
      shift; _rq_k=""
      case "${1:-}" in change|fix|ops) _rq_k=$1; shift ;; esac
      _rq_t=$(roadmap_queue --tsv "$@") || { printf '%s\n' "$_rq_t"; return 1; }
      [ -z "$_rq_t" ] || printf '%s\n' "$_rq_t" | awk -F'\t' -v k="$_rq_k" '$7 == "queued" && (k == "" || $6 == k) { print $5 }'
      return 0 ;;
  esac
  # ROADMAP_BUILTIN=1 tells the template's parser it was called by this helper, not as the front
  # door, so a ROADMAP_PARSER naming it (or a wrapper around it) cannot loop back here.
  if [ -z "$ROADMAP_PARSER" ]; then
    _rq_o=$(ROADMAP_BUILTIN=1 sh "$CLAUDUCTOR_FW/roadmap-queue.sh" "$@"); _rq_rc=$?
  else
    _rq_o=$(cd "$ROOT" && ROADMAP_BUILTIN=1 && export ROOT ROADMAP ROADMAP_BUILTIN && eval "$ROADMAP_PARSER \"\$@\""); _rq_rc=$?
  fi
  if [ "$_rq_rc" -eq 0 ] && [ "${1:-}" = --tsv ]; then
    if [ -n "$ROADMAP_NORMALIZER" ] && [ -n "$_rq_o" ]; then
      _rq_o=$(printf '%s\n' "$_rq_o" | (cd "$ROOT" && export ROOT ROADMAP && eval "$ROADMAP_NORMALIZER")) \
        || { echo "ERROR: ROADMAP_NORMALIZER ($ROADMAP_NORMALIZER) failed, so the change queue is UNKNOWN"; return 1; }
    fi
    if ! _rq_e=$(printf '%s\n' "$_rq_o" | roadmap_tsv_errors); then
      printf 'ERROR: the roadmap parser (%s) broke the --tsv contract, so the change queue is UNKNOWN:\n%s\n' "${ROADMAP_PARSER:-.claude/roadmap-queue.sh}" "$_rq_e"
      return 1
    fi
  fi
  [ -z "$_rq_o" ] || printf '%s\n' "$_rq_o"
  return "$_rq_rc"
}

# roadmap_tsv_errors: stdin is --tsv output; prints one line per row that breaks the contract and
# exits 1 if any did. 12 tab-separated columns (more are ignored): line phase section id change kind
# state pr owner summary budget due. Blank input is an empty queue, not an error.
roadmap_tsv_errors() {
  awk -F'\t' '
    function bad(m) { printf "  tsv row %d (%s): %s\n", NR, substr($0, 1, 60), m; nb++ }
    $0 == "" { next }
    NF < 12 { bad("has " NF " column(s), the contract needs 12"); next }
    $1 !~ /^[0-9]+$/ { bad("column 1 (line) is not a line number") }
    $2 == "" { bad("column 2 (phase) is empty; a row outside every phase has \"-\"") }
    $4 !~ /^[A-Za-z0-9][A-Za-z0-9._-]*$/ { bad("column 4 (row id) \"" $4 "\" is empty or not [A-Za-z0-9._-]") }
    $5 == "" { bad("column 5 (change id) is empty") }
    $6 != "change" && $6 != "fix" && $6 != "ops" { bad("column 6 (kind) \"" $6 "\" is not change, fix or ops") }
    $7 != "queued" && $7 != "inflight" && $7 != "merged" && $7 != "cancelled" { bad("column 7 (state) \"" $7 "\" is not queued, inflight, merged or cancelled (a ROADMAP_NORMALIZER maps a parser'"'"'s own words)") }
    $8 != "" && $8 !~ /^[0-9]+$/ { bad("column 8 (PR) \"" $8 "\" is not a number") }
    $11 != "" && $11 !~ /^[0-9]+(\.[0-9][0-9]?)?$/ { bad("column 11 (budget) \"" $11 "\" is not dollars") }
    $12 != "" && $12 !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/ { bad("column 12 (due) \"" $12 "\" is not YYYY-MM-DD") }
    END { exit nb > 0 }'
}

# The enabled modules (MODULES) may carry default keys of their own (module.conf), applied only
# where project.conf set nothing. Only when a module is on, or a legacy switch names one: every
# hook sources this file, and most projects enable none.
if [ -n "$MODULES" ] || [ "$PROPOSALS" = openspec ] || [ "$REVIEW_PAGE" = artifact ]; then
  # shellcheck disable=SC1091
  [ -f "$CLAUDUCTOR_FW/lib/modules.sh" ] && . "$CLAUDUCTOR_FW/lib/modules.sh" && modules_defaults
fi

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
