# steps.sh: THE one definition of the gate's steps (GATE_STEPS in .claude/project.conf). Sourced by
# run-local.sh, which calls `gate_steps full|quick|docs` in the directory being tested. The remote CI
# (.github/workflows/test.yml) runs the same commands: on Ubuntu for a pull request, on macOS and
# Ubuntu for a push to main (OPS-26).
#
# Each step is `step "<name>" <command> [args...]`: it prints `==> <name>` (the agent-facing
# filter keys on that), runs the command, and a non-zero exit fails the gate. `|| return 1` stops at
# the first failure; drop it on a step to run the rest anyway.
#
# No step needs an installed `stridal` (ADR-0006, the no-stridal invariant): the plugin
# check runs the CLI from this checkout's source with `go run`.

# gofmt -l prints the files it would change and exits 0 either way, so the output is the verdict.
gofmt_clean() {
  out=$(cd framework && gofmt -l .) || return 1
  [ -z "$out" ] && return 0
  echo "gofmt would change:"; printf '    %s\n' $out; return 1
}

# ── The race step's scope (OPS-19) ─────────────────────────────────────────────────────────────
# `go test -race ./...` is most of the full gate's time (internal/panel's tmux and browser tests),
# and most PRs never touch what those tests exercise. So this repo's local full gate races only the
# packages whose tests can see the change. CI (.github/workflows/test.yml) still races ./... on every
# PR and the merge guard blocks on a red check, so the whole suite stays the safety net. The receipt
# still says `full`: it names the gate's MODE (every step ran; run-local.sh documents it so); the
# race step's scope is the `==> test (race): ...` line in the log.
#
# The mapping is what the tests READ outside framework/ (each read is a grep away):
#   template/                internal/cmd (install/update tests read it via STRIDAL_FRAMEWORK;
#                            TestCommittedPluginIsCurrent), internal/plugin, internal/template
#   plugin/, .claude-plugin/ internal/cmd (TestCommittedPluginIsCurrent), internal/plugin
#   docs/panel.md            internal/template, internal/panel/{config,install,lease,metrics,state,web}
#   docs/panel.schema.json   internal/panel/config
#   docs/guide.md            internal/panel/web
#   install.sh               internal/cmd (installsh_test.go runs it)
# framework/, any go.mod or go.sum, .github/ and scripts/ci/ race everything. So does any doubt: no
# origin/main, a shallow clone, a git error, or no change at all to scope from. Nothing mapped still
# races RACE_BASELINE: the race step never disappears from a full gate.
# .claude/checks/race-scope.sh holds this table to the tests themselves: every package whose tests
# read outside framework/ must be selected by the path it reads.
RACE_BASELINE="./internal/plugin ./internal/template"

# race_changed_paths: every path that differs from the merge-base with the main branch, in the
# working tree (tracked and untracked), one per line. Fails when that cannot be computed.
# GATE_RACE_BASE overrides the ref compared against (the check uses it; so can a timing run).
race_changed_paths() {
  _ref=${GATE_RACE_BASE:-origin/${MAIN_BRANCH:-main}}
  [ "$(git rev-parse --is-shallow-repository 2>/dev/null)" = false ] || return 1
  git rev-parse -q --verify "$_ref^{commit}" >/dev/null 2>&1 || return 1
  _base=$(git merge-base "$_ref" HEAD 2>/dev/null) || return 1
  # --no-renames: a file moved out of framework/ must still name framework/.
  _tracked=$(git diff --name-only --no-renames "$_base" 2>/dev/null) || return 1
  _untracked=$(git ls-files --others --exclude-standard 2>/dev/null) || return 1
  printf '%s\n%s\n' "$_tracked" "$_untracked" | sed '/^$/d' | sort -u
}

# race_select: reads changed paths on stdin; prints `<packages>` TAB `<why>`, where <packages> is
# `./...` for the whole suite. It never prints an empty package list.
race_select() {
  _full="" _pkgs="" _areas="" _n=0
  while IFS= read -r _p; do
    [ -n "$_p" ] || continue
    _n=$((_n + 1))
    case $_p in
      framework/*|.github/*|scripts/ci/*|go.mod|go.sum|*/go.mod|*/go.sum)
        [ -n "$_full" ] || _full=$_p ;;
      template/*) _pkgs="$_pkgs ./internal/brand ./internal/cmd ./internal/plugin ./internal/template ./internal/panel/install ./internal/panel/lease" ;;
      plugin/*|.claude-plugin/*) _pkgs="$_pkgs ./internal/cmd ./internal/plugin" ;;
      docs/panel.md) _pkgs="$_pkgs ./internal/template ./internal/panel/config ./internal/panel/install ./internal/panel/lease ./internal/panel/metrics ./internal/panel/state ./internal/panel/web" ;;
      docs/panel.schema.json) _pkgs="$_pkgs ./internal/panel/config" ;;
      docs/guide.md) _pkgs="$_pkgs ./internal/panel/web" ;;
      install.sh) _pkgs="$_pkgs ./internal/cmd" ;;
    esac
    case $_p in */*) _a="${_p%%/*}/" ;; *) _a=$_p ;; esac
    case " $_areas " in *" $_a "*) ;; *) _areas="$_areas $_a" ;; esac
  done
  if [ -n "$_full" ]; then printf './...\t%s changed\n' "$_full"; return 0; fi
  if [ "$_n" -eq 0 ]; then printf './...\tno change against the merge-base to scope from\n'; return 0; fi
  # shellcheck disable=SC2086  # word splitting is the point: one package or area per line
  _pkgs=$(printf '%s\n' $RACE_BASELINE $_pkgs | sort -u | tr '\n' ' ' | sed 's/ $//')
  # shellcheck disable=SC2086
  _areas=$(printf '%s\n' $_areas | sort | tr '\n' ',' | sed 's/,$//; s/,/, /g')
  printf '%s\tonly %s changed\n' "$_pkgs" "$_areas"
}

# race_scope: the race step's scope for the tree in the current directory; the whole suite
# whenever the diff cannot be computed (fail safe).
race_scope() {
  if _paths=$(race_changed_paths); then
    printf '%s\n' "$_paths" | race_select
  else
    printf './...\tthe diff against %s could not be computed\n' "${GATE_RACE_BASE:-origin/${MAIN_BRANCH:-main}}"
  fi
}

# ── One run of each shared check (OPS-89) ──────────────────────────────────────────────────────
# The gate runs two check sets over one model: this repo's (.claude/checks, ROOT = this repo) and
# the template's (template/.claude/checks, ROOT = template/). The template's set always runs whole.
# The checks in CHECKS_ONCE read from ROOT only the model's framework files (they build their own
# fixtures from them) and project.conf values that are the same in both, so while both hold
# (framework_matches_template), the template's run of one is a run of exactly what this repo's would
# be, and this repo's set leaves it out (run.sh's CHECKS_SKIP). Every other check reads state that
# differs between the two ROOTs (the records, settings.json, model-roles.json, panel.json, steps.sh,
# .claude/local, git history) and runs in both. The reason for each is beside it;
# .claude/checks/check-sets.sh holds the rest: every check of either directory runs in at least one
# set, and only these are left out. A check added to the model runs in both sets until someone reads
# it and adds it here.
# Each reads from ROOT only model files it copies into a fixture or greps (2026-10-21, OPS-89):
#   archive-change   .claude/archive-change.sh and .claude/examples, run on fixture changes
#   artifacts        modules/artifacts, run on fixture repos (its guard rule, its context sections)
#   ci-status        modules/ci-status and the gate scripts, in a fixture gate
#   gate             the gate scripts (run-local, lease, lib/, gate-wait), in a fixture repo
#   health           extensions.sh, on a fixture project's health lines
#   ideas            modules/ideas, enabled in a fixture project
#   living-visuals   modules/living-visuals and its skills
#   machine-quiet    .claude/machine-quiet.sh, on fixture worktrees and processes
#   modules          lib/modules.sh, extensions.sh and the session skills' lines that call them
#   people           modules/people, on a fixture repo with a stand-in gh
#   premise-check    modules/premise-check's guard rule, on fixture PRs
#   report-back      the merge-pr and apply-change skills and build-change.js
#   review-lane      skills/merge-pr/review-lane.sh, on fixture diffs
#   risk-register    modules/risk-register, through extensions.sh in a fixture
#   run-parallel     checks/run.sh, lib.sh and lib/conf.sh, in fixture projects (OPS-175)
#   write-surfaces   modules/write-surfaces' guard rule, on fixture PRs
# Not here, though most of each is fixtures: merge-guard, evals, build-change, change-tools and
# metrics build fixtures from ROOT's model-roles.json; hooks, settings, skills and panel-contract read
# settings.json or AGENTS.md; statusline and quota-reading render ROOT's project.conf; session-context
# and health-lines read ROOT's records and workflows; the rest read the records themselves.
CHECKS_ONCE="archive-change artifacts ci-status gate health ideas living-visuals machine-quiet modules people premise-check report-back review-lane risk-register run-parallel write-surfaces"

# The project.conf keys that may differ between the two ROOTs: no CHECKS_ONCE check reads them.
# Nothing local proves it (a key read through a file a check runs is not in its source): CI's
# process-checks step (test.yml) is the run of this repo's whole set over its own project.conf,
# and check-sets.sh holds it in place (OPS-171). It enforces this only while GATE_PR_CHECKS names
# its job (OPS-98 sets it); with GATE_PR_CHECKS empty, Nothing. You. (check-sets prints which).
# Every other key must be the same in both.
# GATE_PR_CHECKS among them: this repo's is "test", the template's empty (OPS-98).
# APPROVAL_DELEGATED and GATE_DOCS: this repo's own are "1" (OPS-89, OPS-127). Only
# change-approval.sh and run-local.sh read them; no CHECKS_ONCE check runs change-approval.sh, gate
# and ci-status run run-local.sh only in fixtures whose project.conf they write, and conf.sh sets
# both defaults before it reads one (OPS-170). BATCH_REGENERATE: this repo's rebuilds plugin/; only
# scripts/ci/batch.sh reads it, and checks/batch.sh runs it in a fixture whose project.conf it writes.
CONF_NOT_READ="PROJECT_NAME PROJECT_SLUG STATUS_MARK OWNER_NAME INSIGHT_AREAS TEST_GLOBS FORMAT_CMD FORMAT_EXT GATE_PR_CHECKS APPROVAL_DELEGATED GATE_DOCS BATCH_REGENERATE"
# The compared keys this repo may set apart from the template's, accepting that every gate then runs
# CHECKS_ONCE in both sets. check-sets.sh fails any other key that differs (OPS-170).
CONF_DOUBLE_RUN_OK=""

# framework_verdict: reads `stridal diff --json` on stdin; passes when every framework file is the
# template's (an extra file passes only under .claude/checks/: a check of this repo's own, which this
# set runs), else prints the ones that are not and fails. Fails closed: no report, or one without
# its file list, fails.
framework_verdict() {
  # `dest` is the project's path (an extra file has only that); `path` the template's.
  _fv=$(jq -er 'if (.files | type) != "array" then error("no file list") else
    [.files[] | select(.tier == "framework")
      | (if (.dest // "") != "" then .dest else .path end) as $p
      | select(.status != "identical" and (.status != "extra" or ($p | startswith(".claude/checks/") | not)))
      | "\(.status) \($p)"] | join(", ") end' 2>/dev/null) || { echo "the framework diff gave no report"; return 1; }
  [ -z "$_fv" ] || { echo "$_fv"; return 1; }
}

# conf_values DIR: DIR's effective project.conf (over conf.sh's defaults), one KEY=value a line, but
# ROOT and CONF_NOT_READ. conf_differs DIR DIR: the keys whose values differ; fails closed, naming
# the directory, when either cannot be read.
conf_values() {
  _cv_root=$(cd "$1" 2>/dev/null && pwd) || return 1
  _cv_skip="ROOT|PWD|OLDPWD|PPID|SHLVL|RANDOM|SECONDS|LINENO|BASH[A-Z_]*"
  for _cv_k in $CONF_NOT_READ; do _cv_skip="$_cv_skip|$_cv_k"; done
  # An empty environment, so the caller's variables (this function's own included) are not read.
  env -i PATH="$PATH" HOME="${HOME:-/}" ROOT="$_cv_root" sh -c 'cd "$ROOT" && . ./.claude/lib/conf.sh >/dev/null 2>&1 && set' \
    | LC_ALL=C grep -aE '^[A-Z][A-Z0-9_]*=' | LC_ALL=C grep -avE "^($_cv_skip)="
}
conf_differs() {
  _cd_a=$(conf_values "$1") && [ -n "$_cd_a" ] || { echo "(unreadable in $1)"; return 0; }
  _cd_b=$(conf_values "$2") && [ -n "$_cd_b" ] || { echo "(unreadable in $2)"; return 0; }
  { printf '%s\n' "$_cd_a" | sed 's/^/a /'; printf '%s\n' "$_cd_b" | sed 's/^/b /'; } \
    | LC_ALL=C awk '{ s = substr($0, 1, 1); l = substr($0, 3); k = l; sub(/=.*/, "", k); v[s, k] = l; keys[k] = 1 }
        END { for (k in keys) if (v["a", k] != v["b", k]) print k }' | sort | tr '\n' ' ' | sed 's/ $//'
}

# framework_matches_template: every framework file of this repository is the template's, byte for
# byte, and the project.conf values a CHECKS_ONCE check can read are the same. The authority on the
# files is `stridal diff` (the tiers install and update use), built from this checkout's source,
# as the plugin step runs it. Prints what differs when they do not match.
framework_matches_template() {
  _ft=$(mktemp -d "${TMPDIR:-/tmp}/framework.XXXXXX") || { echo "no temp directory"; return 1; }
  if ! (cd framework && go build -o "$_ft/stridal" ./cmd/stridal) >/dev/null 2>&1; then
    rm -rf "$_ft"; echo "the CLI does not build from framework/"; return 1
  fi
  if ! _fj=$(env -u STRIDAL_FRAMEWORK "$_ft/stridal" diff --json --template-dir template 2>/dev/null); then
    rm -rf "$_ft"; echo "the framework diff failed"; return 1
  fi
  rm -rf "$_ft"
  _fd=$(printf '%s' "$_fj" | framework_verdict) || { echo "$_fd"; return 1; }
  _fc=$(conf_differs . template)
  [ -z "$_fc" ] || { echo "project.conf differs in $_fc"; return 1; }
}

# check_sets: sets CHECKS_LEFT, the checks this repo's set leaves to the template's, and says which.
check_sets() {
  CHECKS_LEFT=""
  if _why=$(framework_matches_template); then
    CHECKS_LEFT=$CHECKS_ONCE
    echo "==> check sets: the framework matches the template; this repo's set leaves to the template's:$(printf ' %s' $CHECKS_LEFT)"
  else
    echo "==> check sets: this repo's framework differs from the template ($_why); both sets run every check"
  fi
}

# docs_scope: the docs gate's guard for this repo (OPS-89). run-local.sh has found the diff prose
# only; this refuses it when the prose is a file a Go test reads (the race table above: docs/panel.md,
# docs/guide.md, template/, plugin/), which race_select maps to more than RACE_BASELINE.
docs_scope() {
  # Always against origin/main: a GATE_RACE_BASE that already holds the edit would hide it, and the
  # guard's own re-check knows prose, not this table.
  _ds=$(unset GATE_RACE_BASE; race_scope)
  [ "$(printf '%s' "$_ds" | cut -f1)" = "$RACE_BASELINE" ] && return 0
  echo "a docs gate cannot vouch for this diff: Go tests read it ($(printf '%s' "$_ds" | cut -f2)). Run the full gate."
  return 1
}

gate_steps() {
  mode=$1   # full | quick | docs

  [ "$mode" != docs ] || step "docs scope" docs_scope || return 1
  # This repo's own operating model: the process checks over its records, hooks and roles, but for
  # the ones the template's set below runs from the same files (CHECKS_ONCE).
  step "check sets" check_sets || return 1
  step "process checks" env CHECKS_SKIP="$CHECKS_LEFT" sh .claude/checks/run.sh || return 1
  # A prose-only diff changes nothing the remaining steps build or test (docs_scope holds the rest).
  [ "$mode" != docs ] || return 0
  step "gofmt" gofmt_clean || return 1
  step "vet" sh -c 'cd framework && go vet ./...' || return 1
  # -count=1 on both test steps: go's test cache keys on what the test process itself opens, and
  # many tests run sh checks over template/ in subprocesses, so a cached pass can be stale evidence.
  # -timeout 5m: the short suite's slowest run on record is 40 s (gate-runs.log, 2026-10-22 to
  # 10-24), so a hung test fails in minutes rather than at go test's 10-minute default (OPS-89).
  step "test (short)" sh -c 'cd framework && go test -short -count=1 -timeout 5m ./...' || return 1
  # The template's checks, run in the template (they hold the model every project receives): all of
  # them, CHECKS_ONCE included, and never a CHECKS_SKIP from the caller.
  step "template checks" env CHECKS_SKIP= sh template/.claude/checks/run.sh || return 1
  # plugin/ is generated from template/; a template edit without a rebuild ships a stale plugin.
  step "plugin is current" sh -c 'root=$PWD; cd framework && go run ./cmd/stridal plugin check --repo "$root"' || return 1

  [ "$mode" = full ] || return 0

  # The race detector over what the change can reach (race_scope above). `./...` is the full suite
  # as CI runs it (the panel's browser tests and the template-checks tests, which need node and
  # the OpenSpec CLI for full coverage).
  scope=$(race_scope)
  race_pkgs=$(printf '%s' "$scope" | cut -f1)
  if [ "$race_pkgs" = ./... ]; then shown="all packages"; else shown=$(printf '%s' "$race_pkgs" | sed 's|\./||g'); fi
  echo "==> test (race): $shown ($(printf '%s' "$scope" | cut -f2))"
  # shellcheck disable=SC2086  # the package list is split on purpose
  # -timeout 25m as CI sets it: internal/plugin, always in the scope, runs near go test's 10-minute
  # per-package default on a loaded machine.
  step "test (race)" sh -c 'cd framework && go test -race -count=1 -timeout 25m "$@"' race $race_pkgs || return 1
  return 0
}
