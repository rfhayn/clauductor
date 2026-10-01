# scripts/ci/lib/steps.sh: the operating model's own gate steps, as a library. Sourced (never run)
# by the template's run-local.sh, and by a project's OWN runner that keeps its gate but wants the
# model's steps in it (GATE_RUN naming another script): source this file, then call the steps.
#
#   . scripts/ci/lib/steps.sh
#   model_steps                      the two every gate runs: scenario trace, then secrets
#   model_step_scenario_trace [SHA]  .claude/scenario-trace.sh --check (--rev SHA: at that commit)
#   model_step_secrets               gitleaks over the tracked files and the branch's commits
#   model_step_checks                the process checks (.claude/checks/run.sh)
#   model_step_no_clauductor         checks/no-clauductor.sh on its own (D10), where it exists
#   model_publish_status pass|fail SHA   after a FULL run: the ci-status module's commit status on
#                                    SHA, DISPLAY only (a no-op unless MODULES names ci-status)
#
# Each is a `step "<name>" …` call, so its output carries the `==> <name>` markers gate.sh filters
# on. The caller's own `step` function is used when it has one (run-local.sh's records failures for
# its verdict line); otherwise the minimal one below. Needs ROOT and MAIN_BRANCH (.claude/lib/conf.sh)
# and bash or POSIX sh; returns non-zero on a failed step. The framework tier: `clauductor update`
# refreshes this file, so a project edits its own runner, never this.

# step NAME COMMAND [ARGS...]: one gate step, marked for the agent-facing filter (unless the runner
# defined its own).
if ! command -v step >/dev/null 2>&1; then
  step() {
    _st_name=$1; shift
    echo "==> $_st_name"
    if "$@"; then echo "==> ok: $_st_name"; else _st_rc=$?; echo "==> FAIL: $_st_name (exit $_st_rc)"; return 1; fi
  }
fi

# model_script NAME [ARGS]: run the model's .claude/NAME: the repository's copy, else (a repository
# that runs the model from the clauductor plugin) through scripts/ci/clauductor-model.sh.
model_script() {
  _ms=$1; shift
  if [ -f "$ROOT/.claude/$_ms" ]; then sh "$ROOT/.claude/$_ms" "$@"
  elif [ -f "$ROOT/scripts/ci/clauductor-model.sh" ]; then sh "$ROOT/scripts/ci/clauductor-model.sh" "$_ms" "$@"
  else echo "FAIL: .claude/$_ms is missing, so this step cannot run (restore the operating model's files)"; return 1; fi
}

# The secret scan. Tracked files only (an ignored .env on this machine is not a leak), as they are in
# the tree under test, plus the commits this branch adds (a secret committed and then deleted is
# still in the history a push publishes). Without gitleaks: SKIPPED and why, locally; a FAILURE under
# CI (CI=true, as every hosted CI sets it), so a remote gate cannot pass without the scan.
model_secret_scan() {
  if ! command -v gitleaks >/dev/null 2>&1; then
    case "${CI:-}" in
      ''|false|0) echo "secrets: SKIPPED — gitleaks is not installed, so NO secret scan ran (brew install gitleaks, or see github.com/gitleaks/gitleaks). Under CI this fails."; return 0 ;;
      *) echo "secrets: FAIL — gitleaks is not installed, and CI=$CI requires the scan"; return 1 ;;
    esac
  fi
  _ss_src=. _ss_tmp=""
  if [ -d .git ] || [ -f .git ]; then
    _ss_tmp=$(mktemp -d "${TMPDIR:-/tmp}/secrets.XXXXXX")
    git ls-files -z | xargs -0 tar -cf - 2>/dev/null | tar -xf - -C "$_ss_tmp" 2>/dev/null
    _ss_src=$_ss_tmp
  fi
  _ss_rc=0
  if gitleaks dir --help >/dev/null 2>&1; then gitleaks dir --no-banner --redact "$_ss_src" || _ss_rc=$?
  else gitleaks detect --no-git --no-banner --redact --source "$_ss_src" || _ss_rc=$?; fi
  [ -n "$_ss_tmp" ] && rm -rf "$_ss_tmp"
  if [ "$_ss_rc" -eq 0 ] && { [ -d .git ] || [ -f .git ]; } && git rev-parse -q --verify "origin/${MAIN_BRANCH:-main}" >/dev/null 2>&1; then
    _ss_range="$(git merge-base "origin/${MAIN_BRANCH:-main}" HEAD 2>/dev/null)..HEAD"
    if gitleaks git --help >/dev/null 2>&1; then gitleaks git --no-banner --redact --log-opts="$_ss_range" . || _ss_rc=$?
    else gitleaks detect --no-banner --redact --log-opts="$_ss_range" --source . || _ss_rc=$?; fi
  fi
  return "$_ss_rc"
}

model_step_scenario_trace() {
  if [ -n "${1:-}" ]; then step "scenario trace" model_script scenario-trace.sh --check --rev "$1"
  else step "scenario trace" model_script scenario-trace.sh --check; fi
}
model_step_secrets() { step "secrets" model_secret_scan; }
model_step_checks() { step "process checks" model_script checks/run.sh; }
model_step_no_clauductor() {
  if [ -f "$ROOT/.claude/checks/no-clauductor.sh" ] || [ -f "$ROOT/scripts/ci/clauductor-model.sh" ]; then
    step "no clauductor" model_script checks/run.sh no-clauductor
  else
    echo "==> no clauductor: SKIPPED — .claude/checks/no-clauductor.sh is not in this repository"
  fi
}

# model_publish_status pass|fail SHA: draw a FULL run's verdict on the commit it tested, when the
# ci-status module is on (.claude/modules/ci-status). Display only: the merge guard ignores the
# contexts it posts (GATE_DISPLAY_CONTEXTS), so the receipt stays the only evidence. Never fails its
# caller, whatever the publisher does: a cosmetic reporter must not turn a verdict.
model_publish_status() {
  case " ${MODULES:-} " in *" ci-status "*) ;; *) return 0 ;; esac
  SHA=$2 model_script modules/ci-status/scripts/publish-status.sh local "$1" || true
}

# model_steps [SHA]: the steps every gate runs, before the project's own (GATE_STEPS). SHA, when
# given, is the commit a clean-room run tests: the trace reads that commit, not the working tree.
model_steps() {
  model_step_scenario_trace "${1:-}" || return 1
  model_step_secrets || return 1
}
