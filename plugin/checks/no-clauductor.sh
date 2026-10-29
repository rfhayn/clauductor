#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# A repository works without clauductor installed (the no-clauductor invariant). Everything a
# contributor needs runs in Claude Code alone; the clauductor binary, its panel and the plugin are
# conveniences on top, never dependencies. With `clauductor` absent from PATH (every other tool
# kept) and no panel (a HOME with no ~/.clauductor), this runs:
#   1. the gate: run-local.sh and gate.sh in a throwaway repo, through lease.sh;
#   2. every other process check. A whole-set checks/run.sh already runs the whole set inside this
#      environment, once (lib.sh's bare_env, the same helper as here), so under it this part only
#      asserts the environment from the real state (lib.sh's bare_assert): clauductor off PATH,
#      HOME the environment's, no ~/.clauductor. It fails when a whole-set run (CHECKS_SET=whole)
#      ran it with no environment (no CHECKS_BARE). Run alone, or under CHECKS_DOUBLE_PASS=1, it
#      runs this directory's other checks again in here, through $dir/run.sh: the double pass;
#   3. a representative skill context script (session-start's), in this project;
#   4. the status line;
# and fails if any of them errors, or asks for the binary or the panel. The project's own gate
# steps (GATE_STEPS) are scanned for a bare `clauductor` call, since running them here would run
# the whole gate inside a check. Falsified in fixtures: a gate step that calls clauductor fails the
# gate and is named by the scan, and a context script that calls it is caught. The single pass's
# fixtures: a check that calls clauductor fails a whole-set run; a run.sh with its wrap removed
# fails the assertion, and so does a stale CHECKS_BARE, by each part of the real state; a check that
# fails only with clauductor on PATH passes the single pass and fails the double pass (the gap a
# scheduled CHECKS_DOUBLE_PASS=1 run covers); a stale CHECKS_BARE does not earn the summary's
# "without clauductor"; run.sh does not pass lib.sh's CONTEXT_OFFLINE=1 on; an INT or a TERM stops a
# whole-set run and its running check, and the run dies of it (the INT half is skipped, by name, when
# this check was itself started with INT ignored).
. "$(dirname "$0")/lib.sh"
need git bash jq

d=$(scratch)
dir=$(cd "$(dirname "$0")" && pwd)

# ── The environment: PATH without clauductor, and a HOME with no panel (lib.sh's bare_env) ──────
bare_env

if bare sh -c 'command -v clauductor' >/dev/null 2>&1; then
  fail "cannot check: clauductor is still on the stripped PATH"; finish
fi
ok "clauductor is absent from PATH, and HOME has no panel"

# asks FILE: the lines where something errored for want of clauductor, or asked for it or the panel.
ASKS='clauductor: (command )?not found|command not found: clauductor|clauductor: No such file|clauductor[^a-z].*(is required|not installed|is missing|needed)|install clauductor|(please )?start the panel|panel (is )?required|start (it|the panel)? ?with:? .?clauductor|run .?clauductor panel'
asks() { grep -iE "$ASKS" "$1" | head -3; }

# ── 0. What the model's scripts print: no line tells the user to run clauductor ─────────────────
# A missing panel is a normal state, so a script may mention the panel as optional ("If installed:
# clauductor panel") but never as a step to take ("Panel: down (start it with: clauductor panel)").
fw=$(cd "$dir/.." && pwd)   # the model's files: .claude/ here, the plugin root in the plugin
said() {  # said FILE...: echo/printf lines that mention clauductor and ask for it
  grep -nHE '(echo|printf)[^#]*clauductor' "$@" 2>/dev/null | grep -iE "$ASKS" | head -3
}
printf 'echo "- Panel: down (start it with: clauductor panel)"\n' > "$d/b12.sh"
printf 'echo "- Panel: not running (optional; If installed: clauductor panel)"\n' > "$d/neutral.sh"
[ -n "$(said "$d/b12.sh")" ] && ok "falsified: a line telling the user to start clauductor is caught" || fail "the output scan missed: $(cat "$d/b12.sh")"
[ -z "$(said "$d/neutral.sh")" ] && ok "...and a line naming the panel as optional passes" || fail "the output scan flags a neutral line: $(said "$d/neutral.sh")"
s=$(find "$fw" "$ROOT/scripts/ci" \( -name worktrees -o -name checks \) -prune -o -name '*.sh' -print 2>/dev/null | while read -r f; do said "$f"; done)
[ -z "$s" ] && ok "no script of the model tells the user to run clauductor" || fail "a script asks for clauductor: $s"

# ── 1. The gate, in a throwaway repo ────────────────────────────────────────────────────────────
R="$d/repo"; mkdir -p "$R"
git -C "$R" init -q -b main 2>/dev/null || git -C "$R" init -q
mkdir -p "$R/scripts/ci/lib" "$R/.claude/lib"
cp "$ROOT"/scripts/ci/run-local.sh "$ROOT"/scripts/ci/gate.sh "$ROOT"/scripts/ci/lease.sh "$R/scripts/ci/"
cp "$ROOT/scripts/ci/lib/steps.sh" "$R/scripts/ci/lib/"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$R/.claude/"
steps() { printf 'gate_steps() {\n  step "unit" true || return 1\n%s  return 0\n}\n' "$1" > "$R/scripts/ci/steps.sh"; }
steps ""
(cd "$R" && bare git add -A && bare git commit -qm init) >/dev/null 2>&1
gate() { _g=$1; shift; (cd "$R" && bare CLAUDUCTOR_LANE=check bash "scripts/ci/$_g" "$@") > "$d/gate.out" 2>&1; }

gate run-local.sh; rc=$?
expect_rc 0 "$rc" "the full gate passes without clauductor"
[ -f "$R/.git/ci-receipt" ] && ok "...and writes its receipt" || fail "no receipt after a clean run without clauductor: $(tail -5 "$d/gate.out")"
a=$(asks "$d/gate.out"); [ -z "$a" ] && ok "...and asks for neither the binary nor the panel" || fail "the gate asked for clauductor: $a"
gate gate.sh --quick; rc=$?
expect_rc 0 "$rc" "gate.sh (the agent-facing wrapper) passes without clauductor"

# Falsify: a gate step that needs the binary must fail the gate, and the detector must name it.
steps '  step "hard call" clauductor version || return 1
'
(cd "$R" && bare git commit -qam "a hard clauductor call") >/dev/null 2>&1
gate run-local.sh; rc=$?
expect_rc 1 "$rc" "falsified: a gate step calling clauductor fails the gate"
a=$(asks "$d/gate.out"); [ -n "$a" ] && ok "...and the detector names it" || fail "the detector missed a missing clauductor: $(grep -i clauductor "$d/gate.out" | head -3)"

# The project's own gate steps: a bare clauductor command, not one guarded by `command -v`.
# (A path that ends in clauductor, as in `go run ./cmd/clauductor`, is not the installed binary.)
scan() {  # scan FILE: the offending lines
  grep -nE '(^|[[:space:];&|(`"'"'"'])clauductor([[:space:];&|)`"'"'"']|$)' "$1" 2>/dev/null \
    | grep -vE '^[0-9]+:[[:space:]]*#' | grep -v 'command -v clauductor' | head -3
}
cp "$R/scripts/ci/steps.sh" "$d/bad-steps.sh"
[ -n "$(scan "$d/bad-steps.sh")" ] && ok "the steps scan flags a bare clauductor call" || fail "the steps scan missed: $(cat "$d/bad-steps.sh")"
printf '# clauductor is optional\nstep "p" sh -c "cd framework && go run ./cmd/clauductor plugin check"\ncommand -v clauductor >/dev/null && clauductor x\n' > "$d/good-steps.sh"
[ -z "$(scan "$d/good-steps.sh")" ] && ok "...and passes a comment, a source path and a guarded call" || fail "the steps scan flags a legitimate line: $(scan "$d/good-steps.sh")"
if [ -f "$ROOT/$GATE_STEPS" ]; then
  s=$(scan "$ROOT/$GATE_STEPS")
  [ -z "$s" ] && ok "this project's gate steps ($GATE_STEPS) call no clauductor" || fail "$GATE_STEPS calls clauductor: $s"
else
  fail "GATE_STEPS $GATE_STEPS does not exist"
fi

# ── 2. Every other process check ────────────────────────────────────────────────────────────────
# Under a whole-set run.sh they already run inside the environment, once: assert it from the real
# state. Run alone, or under CHECKS_DOUBLE_PASS=1, run them again in here (the double pass). The
# re-run is a named run of $dir/run.sh, with every marker set as under a wrapped run. lib.sh's
# bare_report reports the first two cases, so the fixtures below falsify this very code.
bare_report
if [ $? -eq 2 ]; then
  names=$(cd "$dir" && ls ./*.sh | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib\|no-clauductor')
  bare ROOT="$ROOT" CHECKS_SET= CHECKS_BARE="$BARE_DIR" CHECKS_DOUBLE_PASS= sh "$dir/run.sh" $names > "$d/checks.out" 2>&1; rc=$?
  expect_rc 0 "$rc" "the process checks pass without clauductor ($(tail -1 "$d/checks.out"))"
  [ "$rc" -eq 0 ] || grep -E '^(FAIL|    )' "$d/checks.out" | grep -v '^    ok   SKIPPED' | head -8 | sed 's/^/     /'
fi

# The single pass, falsified (D7 of OPS-93): fixture projects holding copies of run.sh, lib.sh and
# conf.sh, with a stub clauductor on the outer PATH. Every fixture run clears the markers first
# (unmarked) and sets only what its case needs, so it behaves the same inside the single pass and
# inside the double pass.
fx_project() {  # fx_project DIR: a fixture project with this directory's run.sh and lib.sh
  new_repo "$1"; mkdir -p "$1/.claude/checks" "$1/.claude/lib"
  cp "$dir/run.sh" "$dir/lib.sh" "$1/.claude/checks/"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$1/.claude/lib/"
}
F="$d/single"; fx_project "$F"; mkdir -p "$d/stub"
printf '#!/bin/sh\necho "clauductor stub"\n' > "$d/stub/clauductor"; chmod +x "$d/stub/clauductor"
fx_check() { printf '. "$(dirname "$0")/lib.sh"\n%s\nfinish\n' "$1" > "$F/.claude/checks/c.sh"; }
fx() { (cd "$F" && unmarked PATH="$d/stub:$BARE_PATH" "$@" sh .claude/checks/run.sh) > "$d/fx.out" 2>&1; }
fxn() { (cd "$F" && unmarked PATH="$BARE_PATH" "$@" sh .claude/checks/run.sh) > "$d/fx.out" 2>&1; }  # no stub
said_fx() { grep -q -- "$1" "$d/fx.out" && ok "...$2" || fail "the run did not say '$1': $(grep FAIL "$d/fx.out" | head -2)"; }
unwrapped() { sed '/# wrap: start/,/# wrap: end/d' "$dir/run.sh" > "$F/.claude/checks/run.sh"; }
grep -q '# wrap: start' "$dir/run.sh" && grep -q '# wrap: end' "$dir/run.sh" \
  && ok "run.sh marks its wrap, so the fixture can remove it" || fail "run.sh has no '# wrap: start' / '# wrap: end' markers"

fx_check 'clauductor version >/dev/null 2>&1 && ok "clauductor answered" || fail "this check needs clauductor"'
fx; expect_rc 1 $? "falsified: a check that calls clauductor fails a whole-set run.sh (the single pass)"
unwrapped; fx; expect_rc 0 $? "...and passes with run.sh's wrap removed: the stub is on the outer PATH"

fx_check 'bare_report; [ $? -ne 2 ] || ok "run alone"'
fx; expect_rc 1 $? "falsified: a whole-set run.sh with the wrap removed fails the environment assertion"
grep -q 'outside the no-clauductor environment' "$d/fx.out" && ok "...saying it ran outside the environment" || fail "the run did not report running outside the environment: $(grep FAIL "$d/fx.out" | head -2)"
cp "$dir/run.sh" "$F/.claude/checks/run.sh"
fx; expect_rc 0 $? "...and passes under the real run.sh"
fx CHECKS_DOUBLE_PASS=1; expect_rc 0 $? "...and passes under CHECKS_DOUBLE_PASS=1, where no-clauductor.sh re-runs the set itself"
# A stale CHECKS_BARE, each part of the real state on its own: clauductor on PATH; without the stub,
# a HOME with no ~/.clauductor that is not the environment's (a CI runner's case); a
# HOME that is the environment's but holds a ~/.clauductor.
fx CHECKS_BARE="$d/stale"; expect_rc 1 $? "falsified: a stale CHECKS_BARE with clauductor on PATH fails the assertion"
said_fx 'clauductor is on PATH' "saying clauductor is on PATH"
mkdir -p "$d/panel/home/.clauductor" "$d/nopanel/home" "$d/elsewhere"
fxn CHECKS_BARE="$d/stale" HOME="$d/elsewhere"; expect_rc 1 $? "falsified: a stale CHECKS_BARE with no clauductor and no ~/.clauductor fails the assertion"
said_fx "not the environment's" "saying HOME is not the environment's"
fxn CHECKS_BARE="$d/panel" HOME="$d/panel/home"; expect_rc 1 $? "falsified: CHECKS_BARE with its own HOME holding a ~/.clauductor fails the assertion"
said_fx '.clauductor exists' "saying ~/.clauductor exists"
fxn CHECKS_BARE="$d/nopanel" HOME="$d/nopanel/home"; expect_rc 0 $? "...and the same HOME without one passes"

# The summary's "without clauductor" comes from the real state, not the marker: a stale CHECKS_BARE
# over a check that passes anywhere must not earn it.
fx_check 'ok "passes anywhere"'
fx; grep -q 'passed (in .*, without clauductor)$' "$d/fx.out" && ok "a wrapped whole-set run's summary says it ran without clauductor" \
  || fail "a wrapped run's summary does not say it ran without clauductor: $(tail -1 "$d/fx.out")"
fx CHECKS_BARE="$d/stale"; expect_rc 0 $? "a whole-set run under a stale CHECKS_BARE, with a check that passes anywhere, passes"
grep -q 'without clauductor' "$d/fx.out" && fail "a stale CHECKS_BARE earned the summary 'without clauductor': $(tail -1 "$d/fx.out")" \
  || ok "falsified: ...but its summary does not say 'without clauductor', which the real state contradicts"
# run.sh sources lib.sh for its helpers only: a check that does not source lib.sh (as a module's
# check run by its own #! line need not) sees the caller's CONTEXT_OFFLINE, not lib.sh's 1.
printf '[ "${CONTEXT_OFFLINE:-}" != 1 ] && echo "ok   no CONTEXT_OFFLINE" || { echo "FAIL CONTEXT_OFFLINE=1 leaked from run.sh'\''s lib.sh"; exit 1; }\n' > "$F/.claude/checks/c.sh"
fx CONTEXT_OFFLINE=; expect_rc 0 $? "a whole-set run.sh passes the caller's CONTEXT_OFFLINE on, not lib.sh's 1"
# ...and a caller with none passes none on (unset in a POSIX `sh -c`, where `env -u` is not POSIX).
printf '[ -z "${CONTEXT_OFFLINE+x}" ] && echo "ok   CONTEXT_OFFLINE unset" || { echo "FAIL CONTEXT_OFFLINE is set (${CONTEXT_OFFLINE}) where the caller had none"; exit 1; }\n' > "$F/.claude/checks/c.sh"
fx sh -c 'unset CONTEXT_OFFLINE; exec "$@"' unset; expect_rc 0 $? "...and a caller with no CONTEXT_OFFLINE passes none on"
# A passing check's SKIPPED line (the INT fixture's, below) shows under its PASS line in run.sh's
# output, so a skip is never only a smaller count.
fx_check 'ok "runs"; ok "SKIPPED a part this check cannot run here"'
fx; grep -A1 '^PASS c (2)$' "$d/fx.out" | grep -q '^    ok   SKIPPED a part this check cannot run here$' \
  && ok "run.sh names a passing check's SKIPPED line under its PASS line" \
  || fail "run.sh swallowed a passing check's SKIPPED line: $(head -3 "$d/fx.out")"

fx_check 'command -v clauductor >/dev/null 2>&1 && fail "clauductor is on PATH" || ok "no clauductor on PATH"'
fx; expect_rc 0 $? "the gap: a check that fails only with clauductor on PATH passes the single pass"
fx CHECKS_DOUBLE_PASS=1; expect_rc 1 $? "...and fails under CHECKS_DOUBLE_PASS=1, the double pass"

# An INT (Ctrl-C) or a TERM reaches a whole-set run.sh and its running check: the check stops, the
# run stops with it, the next check never starts, and the environment is removed. A terminal signals
# its whole foreground process group; no sh can make a group without job control, so the sender
# stands in for the terminal and signals the run and the check by pid. The run under test is in
# this shell's foreground, so it keeps INT; the sender is in the background (a background command
# ignores INT, but can still send one), and waits for the check's marker before it sends.
sig_fx() {  # sig_fx SIG
  G="$d/sig-$1"; fx_project "$G"; mkdir -p "$G/tmp"
  cat > "$G/.claude/checks/a.sh" <<'EOF'
trap 'kill "$z" 2>/dev/null; touch "$ROOT/a.stopped"; exit 1' INT TERM
sleep 60 & z=$!
echo $$ > "$ROOT/a.started"
wait "$z"
touch "$ROOT/a.done"
EOF
  printf 'touch "$ROOT/b.started"\n' > "$G/.claude/checks/b.sh"
  # The sender signals only once the check has started (so the run is alive and its pid not reused),
  # and is stopped when the run returns.
  # The check runs 60 s unless stopped, and the sender waits on its marker (lib.sh wait_for), so a
  # loaded machine only makes both slower; a guessed 20 s poll and a 10 s check were bounds it could
  # outrun (OPS-168).
  (started() { [ -s "$G/a.started" ]; }; wait_for 30 started && kill -"$1" "$(cat "$G/run.pid")" "$(cat "$G/a.started")") 2>/dev/null &
  _sp=$!
  (cd "$G" && unmarked TMPDIR="$G/tmp" PATH="$BARE_PATH" sh -c 'echo $$ > "$1"; exec sh .claude/checks/run.sh' sig "$G/run.pid") > "$G/out" 2>&1; _rc=$?
  kill "$_sp" 2>/dev/null; wait "$_sp" 2>/dev/null
  # The run dies of the signal itself (128 + its number), so a caller stops too.
  case $1 in INT) _want=130 ;; *) _want=143 ;; esac
  [ "$_rc" -eq "$_want" ] && [ -f "$G/a.stopped" ] && [ ! -f "$G/a.done" ] \
    && ok "$1 to a whole-set run.sh reaches its running check, which stops, and the run dies of $1 with it (exit $_rc)" \
    || fail "$1 did not stop the running check and the run (exit $_rc, want $_want; check stopped: $([ -f "$G/a.stopped" ] && echo yes || echo no), finished: $([ -f "$G/a.done" ] && echo yes || echo no))"
  [ ! -f "$G/b.started" ] && ok "...and the set stops: the next check never starts" || fail "the set ran on after $1: the next check started"
  [ -z "$(ls -A "$G/tmp")" ] && ok "...and the environment is removed" || fail "$1 left $(ls -A "$G/tmp") behind"
}
# A shell started with INT ignored (`run.sh &` from a script, a wrapper with no job control) cannot
# catch one, and passes that on to every run it starts, so the INT fixture cannot run there. That
# is the launcher's state, not run.sh's: the TERM fixture still runs, and the skip is named in the
# check's output, never silent.
if [ "$(sh -c 'trap "echo caught" INT; kill -INT $$; :')" = caught ]; then
  sig_fx INT
else
  ok "SKIPPED the INT fixture: this check was started with INT ignored (in the background, with no job control), so no run it starts can receive INT; the TERM fixture runs"
fi
sig_fx TERM

# The exec window (OPS-121): a signal after run.sh's `exec` into the environment and before the inner
# run has its traps kills it with nothing to remove the environment. A stub `sh`, found only inside
# the environment, is that instant: it TERMs itself, with the run's pid, before any trap exists. The
# next whole-set run's start removes what it left, by the pid it recorded, and nothing else: not a
# live run's environment, not a scratch with no pid recorded.
W="$d/window"; fx_project "$W"; mkdir -p "$W/tmp" "$d/winsh"
printf '#!/bin/sh\nkill -TERM $$\n' > "$d/winsh/sh"; chmod +x "$d/winsh/sh"
# The check sees whether its own run still records a pid: a live run past its traps must not, since
# from another pid namespace (a Linux sandbox) its pid can read as gone.
cat > "$W/.claude/checks/c.sh" <<'EOF'
. "$(dirname "$0")/lib.sh"
[ -e "${CHECKS_BARE%/bare}/run.pid" ] && fail "a live run past its traps still records its pid, so a sweep could remove it" || ok "a live run records no pid"
finish
EOF
_sh=$(command -v sh)
(cd "$W" && unmarked TMPDIR="$W/tmp" PATH="$d/winsh:$BARE_PATH" "$_sh" .claude/checks/run.sh) > "$W/out" 2>&1
_left=$(ls -d "$W"/tmp/checks.*/bare 2>/dev/null)
[ -n "$_left" ] && ok "a TERM inside the exec window leaves the environment behind (${_left#"$W/"})" \
  || fail "the window fixture left no environment behind, so it did not signal inside the window: $(tail -2 "$W/out")"
# Pid 1 is a live process kill cannot signal ("Operation not permitted") unless this is root, where
# kill -0 succeeds: live either way.
mkdir -p "$W/tmp/checks.live" "$W/tmp/checks.eperm" "$W/tmp/checks.nopid"
echo $$ > "$W/tmp/checks.live/run.pid"; echo 1 > "$W/tmp/checks.eperm/run.pid"
(cd "$W" && unmarked TMPDIR="$W/tmp" PATH="$BARE_PATH" sh .claude/checks/run.sh) > "$W/out2" 2>&1
expect_rc 0 $? "...a whole-set run after it passes, its check seeing no pid recorded for its own run"
grep -q '^    FAIL' "$W/out2" && fail "$(grep '^    FAIL' "$W/out2" | head -1)"
[ -n "$_left" ] && [ ! -e "$_left" ] && ok "...and removes the environment the window left" \
  || fail "the environment the window left is still there: $(ls "$W/tmp")"
[ -d "$W/tmp/checks.live" ] && [ -d "$W/tmp/checks.eperm" ] && [ -d "$W/tmp/checks.nopid" ] \
  && ok "...but not a live run's environment (one kill can signal, one it cannot), nor a scratch with no pid recorded" \
  || fail "the sweep removed a live run's environment or an unrecorded scratch: $(ls "$W/tmp")"

# ── 3. A skill context script ───────────────────────────────────────────────────────────────────
ctx() {  # ctx SCRIPT: rc 0, no error on stderr, nothing asking for clauductor or the panel
  (cd "$ROOT" && bare sh "$1") > "$d/ctx.out" 2> "$d/ctx.err"; _rc=$?
  cat "$d/ctx.err" >> "$d/ctx.out"
  [ "$_rc" -eq 0 ] && [ -z "$(asks "$d/ctx.out")" ] && ! grep -qiE 'clauductor|not found|No such file' "$d/ctx.err"
}
if ctx "$CLAUDUCTOR_FW/skills/session-start/context.sh"; then
  ok "session-start's context runs without clauductor or the panel"
else
  fail "session-start's context needs clauductor or the panel: $(asks "$d/ctx.out") $(head -3 "$d/ctx.err")"
fi
printf 'echo "- Panel:"\nclauductor panel status\n' > "$d/hard-ctx.sh"
if ctx "$d/hard-ctx.sh"; then fail "falsified: a context script calling clauductor passed"
else ok "falsified: a context script calling clauductor is caught"; fi

# ── 4. The status line ──────────────────────────────────────────────────────────────────────────
in=$(jq -cn --arg c "$ROOT" '{session_id:"s", cwd:$c, context_window:{used_percentage:12}}')
line=$(printf '%s' "$in" | bare sh "$CLAUDUCTOR_FW/statusline.sh" 2> "$d/sl.err"); rc=$?
expect_rc 0 "$rc" "the status line exits 0 without clauductor or the panel"
[ -n "$line" ] && [ ! -s "$d/sl.err" ] && ok "...prints its line, and nothing on stderr" || fail "status line: '$line', stderr: $(head -3 "$d/sl.err")"
finish
