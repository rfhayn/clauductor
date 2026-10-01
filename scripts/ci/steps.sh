# steps.sh: THE one definition of the gate's steps (GATE_STEPS in .claude/project.conf). Sourced by
# run-local.sh, which calls `gate_steps full|quick` in the directory being tested. The remote CI
# (.github/workflows/test.yml) runs the same commands on macOS and Ubuntu.
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

gate_steps() {
  mode=$1   # full | quick

  # This repo's own operating model: the process checks over its records, hooks and roles.
  step "process checks" sh .claude/checks/run.sh || return 1
  step "gofmt" gofmt_clean || return 1
  step "vet" sh -c 'cd framework && go vet ./...' || return 1
  step "test (short)" sh -c 'cd framework && go test -short ./...' || return 1
  # The template's checks, run in the template (they hold the model every project receives).
  step "template checks" sh template/.claude/checks/run.sh || return 1
  # plugin/ is generated from template/; a template edit without a rebuild ships a stale plugin.
  step "plugin is current" sh -c 'root=$PWD; cd framework && go run ./cmd/clauductor plugin check --repo "$root"' || return 1

  [ "$mode" = full ] || return 0

  # The full suite with the race detector, as CI runs it (it includes the panel's browser tests
  # and the template-checks tests, which need node and the OpenSpec CLI for full coverage).
  step "test (race)" sh -c 'cd framework && go test -race ./...' || return 1
  return 0
}
