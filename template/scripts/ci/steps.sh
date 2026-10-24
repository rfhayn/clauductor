# steps.sh: THE one definition of the gate's steps (GATE_STEPS in .claude/project.conf). Sourced by
# run-local.sh, which calls `gate_steps full|quick|docs` in the directory being tested. If you also
# run a remote CI, have it call this file too, so the local and remote gates cannot drift apart.
#
# `docs` is the docs gate (run-local.sh --docs), for a diff run-local.sh has already found to be
# prose only (.claude/lib/docs-gate.sh). Run what reads the records (the process checks) and return.
# If any of your tests parse a Markdown file, fail the docs mode when the diff touches that file.
#
# Each step is `step "<name>" <command> [args...]`: it prints `==> <name>` (the agent-facing
# filter keys on that), runs the command, and a non-zero exit fails the gate. `|| return 1` stops at
# the first failure; drop it on a step to run the rest anyway.
#
# CONFIGURE: replace the examples with your project's commands. `/start-project` asks for them.

gate_steps() {
  mode=$1   # full | quick | docs

  # The operating model's own checks: they hold the hooks, the roadmap, the ADRs and the model
  # roles to what AGENTS.md says. Plain sh + git + jq, so they run whatever your language.
  step "process checks" sh .claude/checks/run.sh || return 1
  [ "$mode" != docs ] || return 0

  # step "lint"      <your linter>        || return 1   # e.g. npm run lint · go vet ./... · ruff check .
  # step "typecheck" <your type checker>  || return 1   # e.g. npx tsc --noEmit · mypy .
  # step "test"      <your unit tests>    || return 1   # e.g. npm test · go test ./... · pytest

  [ "$mode" = full ] || return 0

  # Slow steps only the full gate runs (integration, end-to-end, builds):
  # step "e2e"       <your e2e suite>     || return 1
  return 0
}
