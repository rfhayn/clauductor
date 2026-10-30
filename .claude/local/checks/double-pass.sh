#!/bin/sh
# The weekly double pass exists, keeps its parts, and cannot pass when a set fails (OPS-93, D5).
# Every gate runs the process checks once, inside the no-stridal environment (ADR-0006, amended
# 2026-10-18); the case that gives up, a check that fails only WITH stridal on PATH, is found by
# .github/workflows/checks-double-pass.yml alone. This repo's own layer holds it, since the workflow
# is this repo's (O5) and `stridal update` never touches .claude/local/. It fails when:
#   - the workflow is missing, or its `on:` has no `schedule:` with a cron (lib/health.sh's own
#     reading, the one scheduled-workflows.sh reports from);
#   - it has more than one job: `$GITHUB_PATH` is per job, so a set run in a second job would run
#     without the binary;
#   - it does not build stridal (`go build -o …/stridal ./cmd/stridal`), add it to
#     `$GITHUB_PATH`, and then, in a later step, `run: command -v stridal`, all before the first
#     set runs (that step fails the job if the directory added is not the one built into);
#   - it never runs a set (this repo's `.claude/checks/run.sh`, the template's), or runs one in any
#     form but the strict one below;
#   - a set's failure could be tolerated, its step skipped, or its path pointed elsewhere:
#     `continue-on-error:` or an `if:` other than `${{ !cancelled() }}`, `${{ always() }}` or bare
#     `always()`, on a set's step or on the job; a `shell:` or a `working-directory:` on a set's step;
#     a `shell:` or a `working-directory:` in a `defaults:`. Keys count quoted as well as bare (`"if":`). Setup steps may use
#     all of them;
#   - it sets `CI:` in an `env:` (the outer pass is meant to see GitHub's `CI=true`).
# THE STRICT FORM: every line that runs a set is `run: CHECKS_DOUBLE_PASS=1 sh <path>`, the step's
# entire command, with no continuation line. Then the set's exit status is the step's, and no
# spelling can swallow it: `|| true`, `; true`, a pipe (`| tee log`, whose status is tee's), a
# multi-line script ending in `exit 0`, a folded `|| true` on the next line. Equivalent forms are
# refused on purpose rather than parsed (CHECKS_DOUBLE_PASS in the step's `env:`, `| tee` under
# pipefail): a reader of shell-in-YAML is where such a check goes wrong. Comment lines never count.
# Falsified on copies of the real workflow, one per missing piece and one per way of swallowing a
# failure, each derived from it so a fixture that no longer changes anything fails too.
. "${CHECKS_LIB:?}"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/health.sh"   # health_on_trigger, health_crons, _health_workflows

WF="$ROOT/.github/workflows/checks-double-pass.yml"

# first_line FILE ERE: the number of the first non-comment line matching ERE; empty when none.
first_line() {
  RE="$2" awk '!/^[[:space:]]*#/ && $0 ~ ENVIRON["RE"] { print NR; exit }' "$1"
}

# set_problems FILE PATH: a line per way PATH's runs break the strict form, then `@first N`, the
# first line that runs it (or `never runs PATH`).
set_problems() {
  P=$(printf '%s' "$2" | sed 's/[.]/[.]/g') S="$2" awk '
    function ind(s) { match(s, /^[[:space:]]*(-[[:space:]]+)?/); return RLENGTH }
    BEGIN { P = ENVIRON["P"]; S = ENVIRON["S"] }
    # A plain scalar continues on any later line indented past its key: `run: … sh X` then
    # `  || true` is one command.
    pending && !/^[[:space:]]*$/ && !/^[[:space:]]*#/ {
      match($0, /^[[:space:]]*/)
      if (RLENGTH > key) print "continues the run: line of " S " on the next line (line " NR ")"
      pending = 0
    }
    /^[[:space:]]*#/ { next }
    # The path ends at anything a path cannot hold: `run.sh;`, `run.sh|` and `run.sh hooks` all run it.
    $0 ~ "(^|[[:space:]])sh " P "([^[:alnum:]_./-]|$)" {
      n++; if (!first) first = NR
      if ($0 !~ "CHECKS_DOUBLE_PASS=1 sh " P "([^[:alnum:]_./-]|$)")
        print "runs " S " without CHECKS_DOUBLE_PASS=1 (line " NR ")"
      else if ($0 !~ "^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*CHECKS_DOUBLE_PASS=1 sh " P "[[:space:]]*$")
        print "runs " S " other than as its step'"'"'s entire command, run: CHECKS_DOUBLE_PASS=1 sh " S " (line " NR ")"
      else { pending = 1; key = ind($0) }
    }
    END { if (!n) print "never runs " S; else print "@first " first }
  ' "$1"
}

# problems FILE: one line per missing piece or way to pass a failure; nothing when the workflow is whole.
problems() {
  [ -f "$1" ] || { echo "the workflow is missing"; return; }
  { health_on_trigger "$1" schedule && [ -n "$(health_crons "$1")" ]; } || echo "no schedule: (with a cron) under on:"
  first=
  for s in .claude/checks/run.sh template/.claude/checks/run.sh; do
    out=$(set_problems "$1" "$s")
    printf '%s\n' "$out" | grep -v '^@first '
    n=$(printf '%s\n' "$out" | sed -n 's/^@first //p')
    if [ -n "$n" ] && { [ -z "$first" ] || [ "$n" -lt "$first" ]; }; then first=$n; fi
  done
  shape_problems "$1"
  b=$(first_line "$1" 'go build .*-o [^ ]*/stridal"? .*[.]/cmd/stridal')
  p=$(first_line "$1" '>> *"?[$]GITHUB_PATH')
  c=$(first_line "$1" '^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*command -v stridal[[:space:]]*$')
  [ -n "$b" ] || echo "does not build stridal (go build -o …/stridal ./cmd/stridal)"
  [ -n "$p" ] || echo "does not put the built stridal on PATH (>> \$GITHUB_PATH)"
  [ -n "$c" ] || echo "never checks that stridal resolves (a step whose run: is command -v stridal)"
  if [ -n "$first" ] && [ -n "$b" ] && [ -n "$p" ] && { [ "$b" -gt "$first" ] || [ "$p" -gt "$first" ]; }; then
    echo "builds stridal or puts it on PATH after the first set runs"
  fi
  if [ -n "$first" ] && [ -n "$p" ] && [ -n "$c" ] && { [ "$c" -gt "$first" ] || [ "$c" -lt "$p" ]; }; then
    echo "checks command -v stridal after the first set runs, or before PATH has it"
  fi
}

# shape_problems FILE: the workflow's structure, read by indentation: the number of jobs, what a set's
# step or the job may not carry, a `shell:` in any `defaults:`, and a `CI:` key anywhere.
shape_problems() {
  awk '
    function indent(s) { match(s, /^[[:space:]]*/); return RLENGTH }
    # key(s): the mapping key a line starts, bare or quoted (`if:`, `"if":`, `'"'"'if'"'"':`); "" if none.
    function key(s,   c, i, rest) {
      sub(/^[[:space:]]*(-[[:space:]]+)?/, "", s); c = substr(s, 1, 1)
      if (c == "\"" || c == Q) {
        s = substr(s, 2); i = index(s, c); if (!i) return ""
        rest = substr(s, i + 1); return rest ~ /^[[:space:]]*:/ ? substr(s, 1, i - 1) : ""
      }
      if (!match(s, /^[A-Za-z0-9_-]+[[:space:]]*:/)) return ""
      s = substr(s, 1, RLENGTH); sub(/[[:space:]]*:$/, "", s); return s
    }
    # An `if:` may only be `${{ !cancelled() }}`, `${{ always() }}` or bare `always()`: a bare
    # `!cancelled()` is a YAML tag, not an expression, and GitHub refuses the workflow.
    function bad_if(s,   v) {
      if (key(s) != "if") return 0
      v = s; sub(/^[^:]*:[[:space:]]*/, "", v)
      return v !~ /^(\$\{\{[[:space:]]*(!cancelled\(\)|always\(\))[[:space:]]*\}\}|always\(\))[[:space:]]*$/
    }
    function endstep(   i, k) {
      if (isset) for (i = 1; i <= ns; i++) {
        k = key(sl[i])
        if (k == "continue-on-error") print "tolerates a failure: continue-on-error on a set'"'"'s step (line " sn[i] ")"
        if (k == "shell") print "sets a shell: on a set'"'"'s step, which decides what its status is (line " sn[i] ")"
        if (k == "working-directory") print "sets a working-directory: on a set'"'"'s step, which changes which set its path runs (line " sn[i] ")"
        if (bad_if(sl[i])) print "an if: other than ${{ !cancelled() }} or always() can skip a set'"'"'s step (line " sn[i] ")"
      }
      ns = 0; isset = 0
    }
    BEGIN { Q = sprintf("%c", 39) }
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    { ind = indent($0); k = key($0) }
    k == "CI" { print "sets CI in an env: (line " NR "): the outer pass is meant to see GitHub'"'"'s CI=true" }
    indef && ind <= defind { indef = 0 }
    indef && k == "shell" { print "sets a default shell: (line " NR "), which decides what every step'"'"'s status is" }
    indef && k == "working-directory" { print "sets a default working-directory: (line " NR "), which changes which set a path runs" }
    k == "defaults" { indef = 1; defind = ind }
    # A step runs from its `- ` item to the next item at the same indentation.
    insteps && itind < 0 && /^[[:space:]]*- / { itind = ind }
    insteps && /^[[:space:]]*- / && ind == itind { endstep(); ns = 1; sl[1] = $0; sn[1] = NR; if ($0 ~ /[[:space:]]sh (template\/)?[.]claude\/checks\/run[.]sh/) isset = 1; next }
    insteps && ind > itind { ns++; sl[ns] = $0; sn[ns] = NR; if ($0 ~ /[[:space:]]sh (template\/)?[.]claude\/checks\/run[.]sh/) isset = 1; next }
    insteps { endstep(); insteps = 0 }
    ind == 0 { injobs = (k == "jobs"); jind = -1; next }
    injobs && jind < 0 { jind = ind }
    injobs && ind == jind { jobs++; kind = -1; next }
    injobs && ind > jind && kind < 0 { kind = ind }
    injobs && ind == kind {
      if (k == "continue-on-error") print "tolerates a failure: continue-on-error on the job (line " NR ")"
      if (bad_if($0)) print "an if: other than ${{ !cancelled() }} or always() can skip the job (line " NR ")"
      if (k == "steps") { insteps = 1; itind = -1 }
    }
    END { endstep(); if (jobs != 1) print "has " jobs + 0 " jobs: GITHUB_PATH is per job, so every set must run in the one job that builds stridal" }
  ' "$1"
}

# The real workflow, and the reader's view of it: scheduled-workflows.sh lists what _health_workflows
# finds, so a workflow it cannot see would have no reader.
out=$(problems "$WF")
if [ -z "$out" ]; then
  ok "checks-double-pass.yml runs both sets weekly in one job, CHECKS_DOUBLE_PASS=1, each its step's entire command, after a built stridal is on PATH and resolves"
else
  printf '%s\n' "$out" | while IFS= read -r line; do echo "FAIL checks-double-pass.yml: $line"; done
  fail "checks-double-pass.yml is not the weekly double pass"
  finish
fi
if _health_workflows schedule | grep -qx checks-double-pass.yml; then
  ok "scheduled-workflows.sh's reader lists checks-double-pass.yml"
else
  fail "scheduled-workflows.sh's reader (_health_workflows schedule) does not list checks-double-pass.yml"
fi

# The fixtures: each a copy of the real workflow missing one piece, or swallowing a failure one way;
# each must name what is wrong.
D=$(scratch)
fx() { # fx NAME WANT WHAT: problems of $D/NAME must include WANT; WHAT describes the fixture
  _o=$(problems "$D/$1")
  if printf '%s\n' "$_o" | grep -qF -- "$2"; then ok "falsified: a workflow $3 fails ($2)"
  else fail "falsified: a workflow $3 does not fail with '$2' (got: ${_o:-nothing})"; fi
}
changed() { # changed NAME: fail if $D/NAME is the real workflow (its derivation no longer matches)
  cmp -s "$WF" "$D/$1" && fail "fixture $1 is the real workflow: its derivation no longer matches"
}
derive() { sed "$2" "$WF" > "$D/$1"; changed "$1"; } # derive NAME SED-SCRIPT
after() { # after NAME ERE LINE: LINE inserted after the first line matching ERE
  RE="$2" L="$3" awk '{ print } !d && $0 ~ ENVIRON["RE"] { print ENVIRON["L"]; d = 1 }' "$WF" > "$D/$1"; changed "$1"
}
R='CHECKS_DOUBLE_PASS=1 sh [.]claude/checks/run[.]sh$'
T='CHECKS_DOUBLE_PASS=1 sh template/[.]claude/checks/run[.]sh$'

# The pieces.
fx missing.yml "the workflow is missing" "that does not exist"
derive no-schedule.yml '/^[[:space:]]*schedule:/d; /^[[:space:]]*- cron:/d'
fx no-schedule.yml "no schedule:" "with no schedule:"
derive no-double.yml 's/CHECKS_DOUBLE_PASS=1 //'
fx no-double.yml "runs .claude/checks/run.sh without CHECKS_DOUBLE_PASS=1" "that runs this repo's set without CHECKS_DOUBLE_PASS=1"
fx no-double.yml "runs template/.claude/checks/run.sh without CHECKS_DOUBLE_PASS=1" "that runs the template's set without CHECKS_DOUBLE_PASS=1"
derive no-build.yml '/go build /d'
fx no-build.yml "does not build stridal" "that never builds stridal"
derive no-path.yml '/GITHUB_PATH/d'
fx no-path.yml "does not put the built stridal on PATH" "that never puts it on PATH"
move_last() { # move_last NAME STEP [HEADER]: the step named STEP (its `- name:` to the next one) moved to the end, after HEADER lines
  N="- name: $2" H="${3:-}" awk 'index($0, ENVIRON["N"]) { hold = 1; moved = "" }
     hold && /- name:/ && !index($0, ENVIRON["N"]) { hold = 0 }
     hold { moved = moved $0 "\n"; next } { print }
     END { if (ENVIRON["H"] != "") print ENVIRON["H"]; printf "%s", moved }' "$WF" > "$D/$1"
  changed "$1"
}
move_last late.yml "Build stridal onto PATH"
fx late.yml "after the first set runs" "that builds stridal after the run"
derive no-cmdv.yml '/run: command -v stridal/d'
fx no-cmdv.yml "never checks that stridal resolves" "that never runs command -v stridal"
move_last cmdv-late.yml "stridal is on PATH"
fx cmdv-late.yml "checks command -v stridal after the first set runs" "that checks command -v stridal after the sets"
# The template's set moved into a second job of its own, with no build: GITHUB_PATH is per job.
move_last two-jobs.yml "The double pass, the template's set" "  template-set:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"
fx two-jobs.yml "has 2 jobs" "whose template set runs in a second job"
derive no-template.yml '/sh template\/\.claude\/checks\/run\.sh/d'
fx no-template.yml "never runs template/.claude/checks/run.sh" "that runs only this repo's set"
derive no-repo.yml '/ sh \.claude\/checks\/run\.sh/d'
fx no-repo.yml "never runs .claude/checks/run.sh" "that runs only the template's set"
derive named.yml 's#sh template/.claude/checks/run.sh$#& hooks#'
fx named.yml "runs template/.claude/checks/run.sh other than as its step's entire command" "that runs one named check, not the template's whole set"

# Each way of swallowing a set's failure.
derive or-true.yml "s#$T#& || true#"
fx or-true.yml "runs template/.claude/checks/run.sh other than as its step's entire command" "whose template set ends '|| true'"
derive semi-true.yml "s#$R#&; true#"
fx semi-true.yml "runs .claude/checks/run.sh other than as its step's entire command" "whose repo set ends '; true'"
derive tee.yml "s#$R#& | tee repo.log#"
fx tee.yml "runs .claude/checks/run.sh other than as its step's entire command" "that pipes the repo set into tee"
# A multi-line script: the set's status recorded, then `exit 0` (the shape this check first missed).
RE="run: $R" awk '$0 ~ ENVIRON["RE"] { match($0, /^[[:space:]]*/); i = substr($0, 1, RLENGTH)
    print i "run: |"; print i "  CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh || failed=1"; print i "  exit 0"; next } { print }' "$WF" > "$D/exit0.yml"
changed exit0.yml
fx exit0.yml "runs .claude/checks/run.sh other than as its step's entire command" "whose repo set is a script ending 'exit 0'"
RE="run: $R" awk '{ print } $0 ~ ENVIRON["RE"] { match($0, /^[[:space:]]*/); print substr($0, 1, RLENGTH) "  || true" }' "$WF" > "$D/folded.yml"
changed folded.yml
fx folded.yml "continues the run: line of .claude/checks/run.sh on the next line" "whose repo set's run: line folds in '|| true' on the next line"
after step-coe.yml "run: $T" "        continue-on-error: true"
fx step-coe.yml "tolerates a failure: continue-on-error" "with continue-on-error on the template set's step"
after job-coe.yml '^    runs-on:' "    continue-on-error: true"
fx job-coe.yml "tolerates a failure: continue-on-error" "with continue-on-error on the job"
derive if-false.yml 's/if: \${{ !cancelled() }}/if: false/'
fx if-false.yml "an if: other than" "that skips the template set's step (if: false)"
after job-if.yml '^    runs-on:' "    if: github.event_name == 'workflow_dispatch'"
fx job-if.yml "an if: other than" "whose job runs only when dispatched"
after shell.yml "run: $R" "        shell: sh -c 'sh {0} || true'"
fx shell.yml "sets a shell:" "whose repo set's step runs under a shell that swallows its status"
after defaults-shell.yml '^    runs-on:' "    defaults:
      run:
        shell: sh -c 'sh {0} || true' {0}"
fx defaults-shell.yml "sets a default shell:" "whose job's defaults give every step a shell that swallows its status"
after ci-empty.yml '^    runs-on:' '    env:
      CI: ""'
fx ci-empty.yml "sets CI in an env:" "that empties CI for the job"
# A set's path pointed elsewhere: from template/, `sh .claude/checks/run.sh` is the template's set.
after wd.yml "run: $R" "        working-directory: template"
fx wd.yml "sets a working-directory: on a set's step" "whose repo set's step runs in template/"
after defaults-wd.yml '^    runs-on:' "    defaults:
      run:
        working-directory: template"
fx defaults-wd.yml "sets a default working-directory:" "whose job's defaults run every step in template/"
# A banned key quoted is the same key.
derive quoted-if.yml "s/if: \${{ !cancelled() }}/'if': false/"
fx quoted-if.yml "an if: other than" "that skips the template set's step with a quoted 'if': false"
after quoted-coe.yml "run: $T" '        "continue-on-error": true'
fx quoted-coe.yml "tolerates a failure: continue-on-error" "with a quoted \"continue-on-error\" on the template set's step"
# A bare `!cancelled()` is a YAML tag, not an expression: GitHub refuses the workflow.
derive bare-cancelled.yml 's/if: \${{ !cancelled() }}/if: !cancelled()/'
fx bare-cancelled.yml "an if: other than" "whose template set's step has a bare if: !cancelled()"

# What a setup step may do: the bans are on a set's step and the job, not on every step.
fx_ok() { # fx_ok NAME WHAT: $D/NAME must have no problems
  _o=$(problems "$D/$1")
  if [ -z "$_o" ]; then ok "a workflow $2 passes"; else fail "a workflow $2 fails: $_o"; fi
}
after setup-shell.yml 'working-directory: framework' "        shell: bash"
fx_ok setup-shell.yml "whose build step sets shell: bash"
after setup-coe.yml 'npm install -g @fission-ai/openspec' "        continue-on-error: true"
fx_ok setup-coe.yml "whose OpenSpec step may fail (continue-on-error)"
derive always.yml 's/if: \${{ !cancelled() }}/if: always()/'
fx_ok always.yml "whose template set's step runs if: always()"
derive always-expr.yml 's/if: \${{ !cancelled() }}/if: ${{ always() }}/'
fx_ok always-expr.yml "whose template set's step runs if: \${{ always() }}"
finish
