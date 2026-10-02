#!/bin/sh
# This project's side of the local panel's contract. The panel is clauductor's and is tested there;
# nothing in this project's gate would notice these promises breaking (ported from Standing Tee's
# control-panel-contract test; the status line's half is checks/statusline.sh):
#   1. The checked-in .claude/settings.json carries no panel hook: the panel installs its hooks in
#      the USER settings when it starts, so a machine that never ran the panel sends nothing.
#   2. Every card in .clauductor/panel.json runs a script that exists, with a well-formed refresh,
#      and runs EXACTLY what session-start's context.sh runs, stderr included: no panel-only copy
#      (one parser), and a card that dropped `2>&1` would show silence for the very failure the
#      session sees. Each card is run, and must exit 0 and print something.
#   3. Every lane template delegates to something that exists: each /command its first prompt names
#      is a skill or a workflow of that name, each repo path it names exists, its branch is under its
#      lane type's prefix, and the prompt is one typable line (a newline would submit it early).
#   4. The gate queue RUNs an executable run-local, and its lock is the one run-local takes.
# No panel config is a pass for 2-4: the panel is optional (nothing here needs it).
. "$(dirname "$0")/lib.sh"
need jq

# ── 1. No panel hook in the checked-in settings ───────────────────────────────────────────────────
# The panel's address, its two endpoints and the tag its installer writes. `/hook` and `/status`
# must END a path segment, so `.claude/hooks/...` and `status-write.sh` are not taken for them.
panel_hooks() {  # panel_hooks FILE: each hook entry (url or command) that targets the panel
  jq -r '(.hooks // {}) | to_entries[] | .value[]? | .hooks[]? | "\(.url // "") \(.command // "")"' "$1" \
    | grep -E '(127\.0\.0\.1|localhost):4393|/(hook|status)([?#"'"'"'[:space:]/]|$)|clauductor-panel' || true
}
# Installed as a plugin, the model's hooks are checked in too, in the plugin's hooks/hooks.json.
plugin=""
[ -n "${CLAUDUCTOR_FW:-}" ] && [ -f "$CLAUDUCTOR_FW/hooks/hooks.json" ] && plugin=1
S="$ROOT/.claude/settings.json"
if [ -f "$S" ]; then
  n=0
  for f in "$S" ${plugin:+"$CLAUDUCTOR_FW/hooks/hooks.json"}; do
    k=$(jq '[(.hooks // {}) | to_entries[] | .value[]? | .hooks[]?] | length' "$f" 2>/dev/null) || { fail "$f does not parse"; continue; }
    n=$((n + k))
    found=$(panel_hooks "$f")
    [ -z "$found" ] && ok "$(basename "$f") carries no panel hook (the panel installs its own in the user settings)" || fail "$f carries a panel hook, which every machine would then run: $found"
  done
  [ "$n" -ge 3 ] && ok "$n checked-in hook entries walked (non-vacuity)" || fail "only $n checked-in hook entries found: the walk would pass vacuously"
else
  fail "no .claude/settings.json"
fi
# A card that runs the model through scripts/ci/clauductor-model.sh finds the plugin this way.
[ -z "$plugin" ] || { CLAUDUCTOR_PLUGIN_ROOT=${CLAUDUCTOR_PLUGIN_ROOT:-$CLAUDUCTOR_FW}; export CLAUDUCTOR_PLUGIN_ROOT; }
# The detector, both ways (a check that could never fire passes vacuously).
F=$(scratch)/fx.json
for u in 'http://127.0.0.1:4393/hook?src=clauductor-panel' 'http://localhost:4393/status' 'http://127.0.0.1:5000/hook' 'http://127.0.0.1:9/x?src=clauductor-panel'; do
  jq -n --arg u "$u" '{hooks: {Stop: [{hooks: [{type: "http", url: $u}]}]}}' > "$F"
  [ -n "$(panel_hooks "$F")" ] && ok "the detector fires on $u" || fail "the detector missed $u"
done
D=.claude  # spelled through a variable: fixture commands, not paths this check uses
jq -n --arg a "sh \"\$CLAUDE_PROJECT_DIR\"/$D/hooks/x.sh" --arg b "sh $D/status-write.sh focus" \
  '{hooks: {Stop: [{hooks: [{type: "http", url: "https://hooks.example.com/notify"}]}], PreToolUse: [{hooks: [{type: "command", command: $a}]}, {hooks: [{type: "command", command: $b}]}]}}' > "$F"
[ -z "$(panel_hooks "$F")" ] && ok "the detector passes hooks that are not the panel's (.claude/hooks/, status-write.sh, another host)" || fail "the detector flagged a hook that is not the panel's: $(panel_hooks "$F")"

PJ="$ROOT/.clauductor/panel.json"
if [ ! -f "$PJ" ]; then ok "no .clauductor/panel.json: no panel contract to keep (the panel is optional)"; finish; fi
jq -e . "$PJ" >/dev/null 2>&1 || { fail ".clauductor/panel.json does not parse"; finish; }
CTX="$ROOT/.claude/skills/session-start/context.sh"
[ -f "$CTX" ] || { fail "session-start's context.sh is missing, so no card can be held to it"; finish; }

# ── 2. Every card runs what session-start runs ────────────────────────────────────────────────────
# lanes: every branch prefix this project uses is classified.
for pair in "$BRANCH_CHANGE:build" "$BRANCH_FIX:fix" "$BRANCH_OPS:ops" "$MAIN_BRANCH:orchestrator"; do
  k=${pair%:*}; v=${pair##*:}
  got=$(jq -r --arg k "$k" '.lanes[$k] // empty' "$PJ")
  [ "$got" = "$v" ] && ok "lanes: $k is a $v lane" || fail "lanes: '$k' should be '$v' (project.conf's branch prefixes), panel.json says '${got:-nothing}'"
done
nc=$(jq '.cards | length' "$PJ"); nu=$(jq '[.cards[].id] | unique | length' "$PJ")
[ "$nc" -ge 1 ] && [ "$nc" = "$nu" ] && ok "$nc card(s), each id once" || fail "cards: $nc card(s), $nu distinct id(s)"
# A context.sh line runs the queue through roadmap_queue (lib/conf.sh); a card can only name the
# script, whose front door hands every call to that same helper. They are one command; the run
# below holds them to the same output.
# Installed as a plugin, the same model script is spelled three ways (context.sh's
# `sh "$CLAUDUCTOR_FW"/x.sh`, a card's `sh scripts/ci/clauductor-model.sh x.sh`, the template's
# `sh .claude/x.sh`): canon() makes them one spelling before they are compared.
canon() { sed -e 's#roadmap_queue #sh @MODEL@/roadmap-queue.sh #g' -e 's#sh "\$CLAUDUCTOR_FW"/#sh @MODEL@/#g' -e 's#sh scripts/ci/clauductor-model\.sh #sh @MODEL@/#g' -e 's#sh [.]claude/#sh @MODEL@/#g'; }
ctx_lines=$(grep -v '^[[:space:]]*#' "$CTX" | canon)
jq -c '.cards[]' "$PJ" > "$(scratch)/cards"
while IFS= read -r card; do
  id=$(printf '%s' "$card" | jq -r .id)
  bin=$(printf '%s' "$card" | jq -r '.command[0]')
  inv=$(printf '%s' "$card" | jq -r 'if .command[1] == "-c" then .command[2:] | join(" ") else .command | join(" ") end')
  command -v "$bin" >/dev/null 2>&1 && ok "card $id: its interpreter $bin is on PATH" || fail "card $id: $bin is not on PATH"
  scripts=$(printf '%s\n' "$inv" | tr ' ' '\n' | grep -E '^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+\.(sh|mjs|js)$' || true)
  if [ -z "$scripts" ]; then fail "card $id names no repo script ('$inv'): a card is a view of a script session-start also runs"
  else for s in $scripts; do [ -f "$ROOT/$s" ] && ok "card $id: $s exists" || fail "card $id: $s does not exist"; done; fi
  refresh=$(printf '%s' "$card" | jq -r '.refresh // empty')
  case $refresh in
    watch:?*) [ -e "$ROOT/${refresh#watch:}" ] && ok "card $id: refresh $refresh watches something that exists" || fail "card $id: refresh $refresh watches nothing that exists" ;;
    interval:[1-9]*) printf '%s' "${refresh#interval:}" | grep -qE '^[0-9]+$' && ok "card $id: refresh $refresh" || fail "card $id: refresh '$refresh' is not interval:<seconds>" ;;
    *) fail "card $id: refresh '$refresh' is not watch:<path> or interval:<seconds>" ;;
  esac
  invc=$(printf '%s\n' "$inv" | canon)
  base=$(printf '%s' "$invc" | sed 's/[[:space:]]*2>&1[[:space:]]*$//')
  line=$(printf '%s\n' "$ctx_lines" | grep -F -- "$base" | head -1)
  if [ -z "$line" ]; then fail "card $id runs '$inv', which session-start's context.sh does not run (a panel-only command)"
  else
    want=$(printf '%s' "${line%%|*}" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
    [ "$invc" = "$want" ] && ok "card $id runs exactly what session-start runs, stderr included: $inv" || fail "card $id runs '$inv'; session-start runs '$want'"
  fi
  # A script that calls GitHub cannot be RUN here (no login in CI, and the answer would depend on
  # the network) unless it honours CONTEXT_OFFLINE, as the model's own scripts do: such a card is
  # held to existence and shape only, and at least one card must run for real.
  online=""
  for s in $scripts; do
    [ -f "$ROOT/$s" ] && grep -qE '(^|[[:space:];|&(`$])gh[[:space:]]+(api|run|pr|issue|repo|auth|workflow)([[:space:]]|$)' "$ROOT/$s" && ! grep -q CONTEXT_OFFLINE "$ROOT/$s" && online=$s
  done
  if [ -n "$online" ]; then ok "card $id: $online calls GitHub and ignores CONTEXT_OFFLINE, so it is checked for existence and shape, not run"; continue; fi
  argv=$(printf '%s' "$card" | jq -r '.command | @sh')
  out=$(cd "$ROOT" && eval "set -- $argv" && "$@" 2>&1); rc=$?
  [ "$rc" -eq 0 ] && [ -n "$out" ] && ok "card $id exits 0 from the project root and prints something" || fail "card $id: exit $rc, output '$(printf '%s' "$out" | head -2 | tr '\n' ' ')'"
  echo "$id" >> "$(scratch)/ran"
  # The queue card names the script; session-start calls the helper: run both, compare the output.
  case $invc in
    "sh @MODEL@/roadmap-queue.sh "*)
      qa=$(printf '%s' "${invc#sh @MODEL@/roadmap-queue.sh }" | sed 's/[[:space:]]*2>&1[[:space:]]*$//')
      # shellcheck disable=SC2086
      b=$(cd "$ROOT" && roadmap_queue $qa 2>&1)
      [ "$out" = "$b" ] && ok "card $id prints what session-start's roadmap_queue $qa prints" || fail "card $id and session-start's roadmap_queue $qa disagree, so the panel and the session show two queues: '$(printf '%s' "$out" | head -2 | tr '\n' ' ')' vs '$(printf '%s' "$b" | head -2 | tr '\n' ' ')'" ;;
  esac
done < "$(scratch)/cards"
[ -s "$(scratch)/ran" ] && ok "$(wc -l < "$(scratch)/ran" | tr -d ' ') card(s) were run for real (the GitHub exemption did not swallow them all)" || fail "no card was run: every one was exempted as calling GitHub"
m=$(jq -r '.metrics.command // empty | join(" ")' "$PJ")
if [ -n "$m" ]; then
  for s in $(printf '%s\n' "$m" | tr ' ' '\n' | grep -E '^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+\.sh$'); do [ -f "$ROOT/$s" ] && ok "metrics: $s exists" || fail "metrics: $s does not exist"; done
fi

# ── 3. Every lane template delegates to something that exists ─────────────────────────────────────
target_exists() {  # a skill (the project's, or the plugin's as /<plugin>:<skill>), or a workflow whose meta name is $1
  _n=${1#*:}
  { [ -f "$ROOT/.claude/skills/$_n/SKILL.md" ] || { [ -n "${CLAUDUCTOR_FW:-}" ] && [ -f "$CLAUDUCTOR_FW/skills/$_n/SKILL.md" ]; }; } && return 0
  [ -f "$ROOT/.claude/workflows/$_n.js" ] && grep -qE "name:[[:space:]]*['\"]$_n['\"]" "$ROOT/.claude/workflows/$_n.js"
}
commands_in() { printf '%s\n' "$1" | grep -oE '(^|[[:space:]])/[a-z][a-z0-9-]*(:[a-z][a-z0-9-]*)?([[:space:].,;)]|:[[:space:]]|$)' | sed 's|^[[:space:]]*/||; s|[[:space:]:.,;)]*$||'; }
# A repo path: a token whose first segment is a top-level directory of this project.
paths_in() {
  printf '%s\n' "$1" | grep -oE '(^|[[:space:]("])[A-Za-z0-9_.][A-Za-z0-9_.-]*/[A-Za-z0-9_./-]*[A-Za-z0-9_]' | sed 's|^[[:space:]("]*||' \
    | while IFS= read -r p; do [ -d "$ROOT/${p%%/*}" ] && printf '%s\n' "$p"; done
}
# The detectors, both ways.
[ "$(commands_in 'Fix it: run /premise-check first, then /merge-pr.' | tr '\n' ' ')" = "premise-check merge-pr " ] && [ -z "$(commands_in 'node infra/premise-check.mjs and a/b paths')" ] \
  && ok "the /command detector finds commands, and not paths" || fail "the /command detector: '$(commands_in 'Fix it: run /premise-check first, then /merge-pr.' | tr '\n' ' ')'"
D=.claude  # spelled through a variable: a fixture, not a path this check uses
pt="check with $D/checks/run.sh, then (sh $D/roadmap-queue.sh --text), not and/or nowhere/x.sh."
[ "$(paths_in "$pt" | tr '\n' ' ')" = "$D/checks/run.sh $D/roadmap-queue.sh " ] \
  && ok "the path detector finds repo paths (trailing punctuation dropped), and not prose like and/or" || fail "the path detector: '$(paths_in "$pt" | tr '\n' ' ')'"
[ "$(commands_in '/clauductor:propose {name}: its row' | tr '\n' ' ')" = "clauductor:propose " ] && ok "the /command detector reads a plugin's /<plugin>:<skill>" || fail "the /command detector on a plugin skill: '$(commands_in '/clauductor:propose {name}: its row')'"
target_exists premise-check && fail "target_exists found a skill that does not exist" || ok "target_exists refuses a /command that is no skill or workflow"
jq -c '.templates[]?' "$PJ" > "$(scratch)/templates"
while IFS= read -r t; do
  id=$(printf '%s' "$t" | jq -r .id); fp=$(printf '%s' "$t" | jq -r .first_prompt)
  for c in $(commands_in "$fp"); do target_exists "$c" && ok "template $id: /$c is a skill or workflow here" || fail "template $id: /$c is not a skill or workflow here (a lane would improvise it)"; done
  for p in $(paths_in "$fp"); do [ -e "$ROOT/$p" ] && ok "template $id: $p exists" || fail "template $id: its prompt names $p, which does not exist"; done
  lt=$(printf '%s' "$t" | jq -r .lane_type); bp=$(printf '%s' "$t" | jq -r .branch_pattern)
  pre=$(jq -r --arg v "$lt" '.lanes | to_entries[] | select((.key | endswith("/")) and .value == $v) | .key' "$PJ" | head -1)
  [ -n "$pre" ] && [ "$bp" = "${pre}{name}" ] && ok "template $id: branch $bp is under its $lt lane's prefix" || fail "template $id: branch '$bp' is not '<the $lt lane prefix>{name}' (prefix '${pre:-none}')"
  printf '%s' "$t" | jq -e '[.first_prompt | explode[] | select(. < 32 or . == 127)] | length == 0' >/dev/null && ok "template $id: its first prompt is one typable line" || fail "template $id: its first prompt has a control character (a newline submits it early)"
  bad=$(printf '%s\n' "$fp" | grep -oE '\{[A-Za-z_]+\}' | grep -vxE '\{(name|issue)\}' || true)
  [ -z "$bad" ] && ok "template $id: only {name} and {issue} placeholders" || fail "template $id: unknown placeholder(s) $bad"
done < "$(scratch)/templates"

# ── 4. The gate queue is the lease run-local takes ────────────────────────────────────────────────
q=$(jq -c '.queues[]? | select(.id == "gate")' "$PJ")
if [ -n "$q" ]; then
  cmd=$(printf '%s' "$q" | jq -r '.command[0] // empty'); lock=$(printf '%s' "$q" | jq -r '.lock // empty')
  [ -n "$cmd" ] && [ -x "$ROOT/$cmd" ] && ok "gate queue: RUN execs $cmd, which exists and is executable" || fail "gate queue: '$cmd' is missing or not executable (RUN execs it with no shell)"
  run="$ROOT/$GATE_RUN"
  took=$(sed -n 's|^[[:space:]]*lock="\$common/\(.*\)"[[:space:]]*$|\1|p' "$run" 2>/dev/null | head -1)
  if [ -z "$took" ]; then ok "gate queue: $GATE_RUN takes no lock this check can read (a project's own runner); not compared"
  elif [ "$took" = "$lock" ]; then ok "gate queue: its lock $lock is the one $GATE_RUN takes"
  else fail "gate queue: its lock is $lock, $GATE_RUN takes $took: the panel would show an empty queue while gates collide"; fi
fi
finish
