#!/bin/sh
# A repository works without clauductor installed (the no-clauductor invariant). Everything a
# contributor needs runs in Claude Code alone; the clauductor binary, its panel and the plugin are
# conveniences on top, never dependencies. With `clauductor` absent from PATH (every other tool
# kept) and no panel (a HOME with no ~/.clauductor), this runs:
#   1. the gate: run-local.sh and gate.sh in a throwaway repo, through lease.sh;
#   2. every other process check in this directory;
#   3. a representative skill context script (session-start's), in this project;
#   4. the status line;
# and fails if any of them errors, or asks for the binary or the panel. The project's own gate
# steps (GATE_STEPS) are scanned for a bare `clauductor` call, since running them here would run
# the whole gate inside a check. Falsified in fixtures: a gate step that calls clauductor fails the
# gate and is named by the scan, and a context script that calls it is caught.
. "$(dirname "$0")/lib.sh"
need git bash jq

d=$(scratch)
dir=$(cd "$(dirname "$0")" && pwd)

# ── The environment: PATH without clauductor, and a HOME with no panel ──────────────────────────
# A directory that holds clauductor is replaced by a copy of it made of links to everything else,
# so dropping the binary never drops jq, git or node installed beside it.
np=""; i=0; IFS_OLD=$IFS; IFS=:
for p in $PATH; do
  if [ -n "$p" ] && [ -e "$p/clauductor" ]; then
    i=$((i + 1)); f="$d/path$i"; mkdir -p "$f"
    for x in "$p"/* "$p"/.[!.]*; do
      [ -e "$x" ] || continue
      [ "${x##*/}" = clauductor ] || ln -s "$x" "$f/${x##*/}" 2>/dev/null
    done
    p=$f
  fi
  np="$np${np:+:}$p"
done
IFS=$IFS_OLD
H="$d/home"; mkdir -p "$H"
# A contributor without clauductor still has a git identity.
printf '[user]\n\tname = check\n\temail = check@example.com\n[commit]\n\tgpgsign = false\n[init]\n\tdefaultBranch = main\n' > "$H/.gitconfig"
bare() { env HOME="$H" PATH="$np" CI= CLAUDUCTOR_LOCK_HELD= "$@"; }

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
mkdir -p "$R/scripts/ci" "$R/.claude/lib"
cp "$ROOT"/scripts/ci/run-local.sh "$ROOT"/scripts/ci/gate.sh "$ROOT"/scripts/ci/lease.sh "$R/scripts/ci/"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$R/.claude/"
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
names=$(cd "$dir" && ls ./*.sh | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib\|no-clauductor')
bare ROOT="$ROOT" sh "$ROOT/.claude/checks/run.sh" $names > "$d/checks.out" 2>&1; rc=$?
expect_rc 0 "$rc" "the process checks pass without clauductor ($(tail -1 "$d/checks.out"))"
[ "$rc" -eq 0 ] || grep -E '^(FAIL|    )' "$d/checks.out" | head -8 | sed 's/^/     /'

# ── 3. A skill context script ───────────────────────────────────────────────────────────────────
ctx() {  # ctx SCRIPT: rc 0, no error on stderr, nothing asking for clauductor or the panel
  (cd "$ROOT" && bare sh "$1") > "$d/ctx.out" 2> "$d/ctx.err"; _rc=$?
  cat "$d/ctx.err" >> "$d/ctx.out"
  [ "$_rc" -eq 0 ] && [ -z "$(asks "$d/ctx.out")" ] && ! grep -qiE 'clauductor|not found|No such file' "$d/ctx.err"
}
if ctx "$ROOT/.claude/skills/session-start/context.sh"; then
  ok "session-start's context runs without clauductor or the panel"
else
  fail "session-start's context needs clauductor or the panel: $(asks "$d/ctx.out") $(head -3 "$d/ctx.err")"
fi
printf 'echo "- Panel:"\nclauductor panel status\n' > "$d/hard-ctx.sh"
if ctx "$d/hard-ctx.sh"; then fail "falsified: a context script calling clauductor passed"
else ok "falsified: a context script calling clauductor is caught"; fi

# ── 4. The status line ──────────────────────────────────────────────────────────────────────────
in=$(jq -cn --arg c "$ROOT" '{session_id:"s", cwd:$c, context_window:{used_percentage:12}}')
line=$(printf '%s' "$in" | bare sh "$ROOT/.claude/statusline.sh" 2> "$d/sl.err"); rc=$?
expect_rc 0 "$rc" "the status line exits 0 without clauductor or the panel"
[ -n "$line" ] && [ ! -s "$d/sl.err" ] && ok "...prints its line, and nothing on stderr" || fail "status line: '$line', stderr: $(head -3 "$d/sl.err")"
finish
