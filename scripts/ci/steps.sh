# steps.sh: THE one definition of the gate's steps (GATE_STEPS in .claude/project.conf). Sourced by
# run-local.sh, which calls `gate_steps full|quick` in the directory being tested. The remote CI
# (.github/workflows/test.yml) runs the same commands: on Ubuntu for a pull request, on macOS and
# Ubuntu for a push to main (OPS-26).
#
# Each step is `step "<name>" <command> [args...]`: it prints `==> <name>` (the agent-facing
# filter keys on that), runs the command, and a non-zero exit fails the gate. `|| return 1` stops at
# the first failure; drop it on a step to run the rest anyway.
#
# No step needs an installed `clauductor` (ADR-0006, the no-clauductor invariant): the plugin
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
#   template/                internal/cmd (install/update tests read it via CLAUDUCTOR_FRAMEWORK;
#                            TestCommittedPluginIsCurrent), internal/plugin, internal/template
#   plugin/, .claude-plugin/ internal/cmd (TestCommittedPluginIsCurrent), internal/plugin
#   docs/panel.md            internal/template, internal/panel/{config,install,lease,metrics,web}
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
      template/*) _pkgs="$_pkgs ./internal/cmd ./internal/plugin ./internal/template ./internal/panel/lease" ;;
      plugin/*|.claude-plugin/*) _pkgs="$_pkgs ./internal/cmd ./internal/plugin" ;;
      docs/panel.md) _pkgs="$_pkgs ./internal/template ./internal/panel/config ./internal/panel/install ./internal/panel/lease ./internal/panel/metrics ./internal/panel/web" ;;
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

gate_steps() {
  mode=$1   # full | quick

  # This repo's own operating model: the process checks over its records, hooks and roles.
  step "process checks" sh .claude/checks/run.sh || return 1
  step "gofmt" gofmt_clean || return 1
  step "vet" sh -c 'cd framework && go vet ./...' || return 1
  # -count=1 on both test steps: go's test cache keys on what the test process itself opens, and
  # many tests run sh checks over template/ in subprocesses, so a cached pass can be stale evidence.
  step "test (short)" sh -c 'cd framework && go test -short -count=1 ./...' || return 1
  # The template's checks, run in the template (they hold the model every project receives).
  step "template checks" sh template/.claude/checks/run.sh || return 1
  # plugin/ is generated from template/; a template edit without a rebuild ships a stale plugin.
  step "plugin is current" sh -c 'root=$PWD; cd framework && go run ./cmd/clauductor plugin check --repo "$root"' || return 1

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
