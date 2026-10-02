#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# A model or an effort is chosen in ONE place: .claude/model-roles.json. Everything that cannot
# read that file restates it, and this check fails when any restatement disagrees:
#   - every skill's and every agent's frontmatter `model:` and `effort:`;
#   - .claude/settings.json `model`, `effortLevel` and CLAUDE_CODE_SUBAGENT_MODEL;
#   - .claude/workflows/build-change.js's ROLES, TIERS and ECONOMY tables and its ECONOMY_FILE,
#     when the workflow exists (its attribution and provenance are read at run time from
#     .claude/project-config.sh, so there is nothing to restate: checks/build-change.sh holds that);
#   - .clauductor/panel.json `lane_types`, when the panel config exists.
# The sets are enumerated from the filesystem, not from the JSON (*Enumerate the authority*):
# a skill directory the JSON forgot is a failure, and so is a JSON entry naming nothing.
#
# It also holds each agent's reach to its role: an agent needs a `tools:` line, a read-only
# agent's line has no write tool, and every tool name is one Claude Code knows (a misspelt tool
# is silently absent). Extend the known names with CHECK_EXTRA_TOOLS in project.conf.
. "$(dirname "$0")/lib.sh"
need jq

roles="$ROOT/.claude/model-roles.json"
jq -e . "$roles" >/dev/null 2>&1 || { fail "$roles is missing or not valid JSON"; finish; }
ok "model-roles.json parses"

# fm FILE KEY: a frontmatter value (between the first two --- lines), quotes stripped.
fm() {
  awk -v k="$2" 'NR==1 && $0!="---"{exit} NR>1 && $0=="---"{exit} NR>1 { i=index($0, ":"); if (i && substr($0,1,i-1)==k) { v=substr($0,i+1); sub(/^[ \t]+/,"",v); sub(/[ \t]+$/,"",v); gsub(/^"|"$/,"",v); print v; exit } }' "$1"
}
role_of() { jq -r --arg s "$2" ".$1[\$s] // empty" "$roles"; }
want() { jq -r --arg r "$1" --arg k "$2" '.roles[$r][$k] // empty' "$roles"; }

# match KIND NAME FILE ROLE: frontmatter model/effort against the role's.
match() {
  for key in model effort; do
    w=$(want "$4" "$key"); g=$(fm "$3" "$key")
    if [ -z "$w" ]; then
      [ -z "$g" ] || fail "$1 $2: sets $key '$g' but role '$4' chooses none"
    elif [ "$w" = "$g" ]; then ok "$1 $2: $key $g (role $4)"
    else fail "$1 $2: $key is '${g:-unset}', role '$4' says '$w'"; fi
  done
}

# ── Skills ─────────────────────────────────────────────────────────────────────────────
for d in "$CLAUDUCTOR_FW"/skills/*/ "$ROOT"/.claude/skills/*/; do
  [ -f "$d/SKILL.md" ] || continue
  s=$(basename "$d")
  r=$(role_of skills "$s")
  if [ -z "$r" ]; then fail "skill $s is not mapped to a role in model-roles.json .skills"; continue; fi
  jq -e --arg r "$r" '.roles[$r]' "$roles" >/dev/null || { fail "skill $s: role '$r' is not defined in .roles"; continue; }
  match skill "$s" "$d/SKILL.md" "$r"
done
for s in $(jq -r '.skills | keys[]' "$roles"); do
  [ -f "$CLAUDUCTOR_FW/skills/$s/SKILL.md" ] || [ -f "$ROOT/.claude/skills/$s/SKILL.md" ] || fail "model-roles.json maps skill '$s', which does not exist"
done

# ── Agents ─────────────────────────────────────────────────────────────────────────────
known=" Read Grep Glob Edit Write MultiEdit NotebookEdit Bash BashOutput KillShell Skill Agent Task WebFetch WebSearch TodoWrite ReportFindings ${CHECK_EXTRA_TOOLS:-} "
for f in "$CLAUDUCTOR_FW"/agents/*.md; do
  [ -f "$f" ] || continue
  a=$(basename "$f" .md)
  r=$(role_of agents "$a")
  if [ -z "$r" ]; then fail "agent $a is not mapped to a role in model-roles.json .agents"; continue; fi
  match agent "$a" "$f" "$r"
  tools=$(fm "$f" tools)
  if [ -z "$tools" ]; then fail "agent $a has no tools: line; without one it inherits every tool"; continue; fi
  for t in $(printf '%s' "$tools" | tr ',' ' '); do
    case "$t" in mcp__*) continue ;; esac
    case "$known" in *" $t "*) ;; *) fail "agent $a: tool '$t' is not a known tool name (typo? else add it to CHECK_EXTRA_TOOLS)" ;; esac
  done
  if jq -e --arg a "$a" '(.read_only_agents // []) | index($a)' "$roles" >/dev/null; then
    bad=""
    for t in $(printf '%s' "$tools" | tr ',' ' '); do case "$t" in Edit|Write|MultiEdit|NotebookEdit) bad="$bad $t" ;; esac; done
    if [ -z "$bad" ]; then ok "agent $a is read-only by its tools: line"; else fail "agent $a is read-only but its tools: line grants$bad"; fi
  fi
done
for a in $(jq -r '.agents | keys[]' "$roles"); do
  [ -f "$CLAUDUCTOR_FW/agents/$a.md" ] || fail "model-roles.json maps agent '$a', which does not exist"
done

# ── Settings ───────────────────────────────────────────────────────────────────────────
st="$ROOT/.claude/settings.json"
if jq -e . "$st" >/dev/null 2>&1; then
  for pair in "model:model" "effortLevel:effort"; do
    k=${pair%%:*}; rk=${pair#*:}
    g=$(jq -r ".$k // empty" "$st"); w=$(want session "$rk")
    if [ "$g" = "$w" ]; then ok "settings.json $k $g (role session)"; else fail "settings.json $k is '${g:-unset}', role session says '$w'"; fi
  done
  g=$(jq -r '.env.CLAUDE_CODE_SUBAGENT_MODEL // empty' "$st"); w=$(want helper model)
  if [ "$g" = "$w" ]; then ok "CLAUDE_CODE_SUBAGENT_MODEL $g (role helper)"; else fail "settings.json env.CLAUDE_CODE_SUBAGENT_MODEL is '${g:-unset}', role helper says '$w'"; fi
else
  fail "$st is missing or not valid JSON"
fi

# ── The build-change workflow's ROLES table ────────────────────────────────────────────
wf="$CLAUDUCTOR_FW/workflows/build-change.js"
if [ -f "$wf" ]; then
  # Lines of the form:   <role>: { model: "<m>", effort: "<e>" },
  rows=$(sed -n '/^const ROLES = {/,/^};/p' "$wf" | sed -nE 's/^[[:space:]]*"?([a-z-]+)"?:[[:space:]]*\{[[:space:]]*model:[[:space:]]*"([^"]*)",[[:space:]]*effort:[[:space:]]*"([^"]*)".*/\1 \2 \3/p')
  [ -n "$rows" ] || fail "build-change.js: no 'const ROLES = {' table found (the check reads it)"
  printf '%s\n' "$rows" | while read -r r m e; do
    [ -n "$r" ] || continue
    wm=$(want "$r" model); we=$(want "$r" effort)
    if [ "$m" = "$wm" ] && [ "$e" = "$we" ]; then echo "ok   build-change.js role $r: $m/$e"; else echo "FAIL build-change.js role $r is $m/$e, model-roles.json says ${wm:-?}/${we:-?}"; fi
  done > "$(scratch)/wf"
  cat "$(scratch)/wf"
  n=$(grep -c '^FAIL' "$(scratch)/wf"); _fails=$((_fails + n))
fi

# ── Risk tiers, economy mode, provenance and prices ─────────────────────────────────────────────
# A role's tier variants (item 15) and its economy drop are model choices too, so they live in the
# JSON and build-change restates them. Both directions: a variant the table forgot, and a table row
# the JSON does not have.
models=" opus sonnet haiku fable "; efforts=" low medium high xhigh max "
rank_m() { case "$1" in haiku) echo 1 ;; sonnet) echo 2 ;; opus) echo 3 ;; fable) echo 4 ;; *) echo 0 ;; esac; }
rank_e() { case "$1" in low) echo 1 ;; medium) echo 2 ;; high) echo 3 ;; xhigh) echo 4 ;; max) echo 5 ;; *) echo 0 ;; esac; }
valid_pair() {  # valid_pair WHAT MODEL EFFORT
  case "$models" in *" $2 "*) ;; *) case "$2" in claude-*) ;; *) fail "$1: model '$2' is not opus, sonnet, haiku, fable or a claude-* id"; return 1 ;; esac ;; esac
  case "$efforts" in *" $3 "*) return 0 ;; esac
  fail "$1: effort '$3' is not one of$efforts"; return 1
}
levels=$(jq -r '(.tiers.levels // []) | join(" ")' "$roles")
[ "$levels" = "low normal high" ] && ok "risk tiers are low, normal, high" || fail "model-roles.json .tiers.levels must be [\"low\", \"normal\", \"high\"] (proposal.md's **Risk:** line), got '$levels'"
want_tiers=$(jq -r '.roles | to_entries[] | .key as $r | (.value.tiers // {}) | to_entries[] | "\($r).\(.key) \(.value.model // "") \(.value.effort // "")"' "$roles")
printf '%s\n' "$want_tiers" | while read -r rt m e; do
  [ -n "$rt" ] || continue
  t=${rt#*.}
  case " low high " in *" $t "*) ;; *) echo "FAIL role ${rt%%.*}: tier '$t' is not low or high (normal is the role itself)"; continue ;; esac
  (valid_pair "tier $rt" "$m" "$e") | sed 's/^FAIL/FAIL/' | grep . || echo "ok   tier $rt: $m/$e"
done > "$(scratch)/tiers"
cat "$(scratch)/tiers"; _fails=$((_fails + $(grep -c '^FAIL' "$(scratch)/tiers")))

eco_never=$(jq -r '(.economy.never // []) | sort | join(" ")' "$roles")
[ "$eco_never" = "planner reviewer" ] && ok "economy mode never drops the reviewer or the planner" || fail "model-roles.json .economy.never must be [\"reviewer\", \"planner\"], got '$eco_never'"
jq -r '(.economy.roles // {}) | to_entries[] | "\(.key) \(.value.model // "") \(.value.effort // "")"' "$roles" | while read -r r m e; do
  [ -n "$r" ] || continue
  case " reviewer planner " in *" $r "*) echo "FAIL economy mode drops role $r, which must never drop"; continue ;; esac
  bm=$(want "$r" model); be=$(want "$r" effort)
  [ -n "$bm" ] || { echo "FAIL economy names role $r, which .roles does not define"; continue; }
  (valid_pair "economy $r" "$m" "$e") | grep . && continue
  if [ "$(rank_m "$m")" -lt "$(rank_m "$bm")" ] || { [ "$m" = "$bm" ] && [ "$(rank_e "$e")" -lt "$(rank_e "$be")" ]; }; then
    echo "ok   economy $r: $bm/$be drops to $m/$e"
  else
    echo "FAIL economy $r: $m/$e is not a tier below the role's $bm/$be"
  fi
done > "$(scratch)/eco"
cat "$(scratch)/eco"; _fails=$((_fails + $(grep -c '^FAIL' "$(scratch)/eco")))

if jq -e '(.provenance.enabled | type == "boolean") and (.provenance.trailers == ["Change", "Agent-Role", "Model", "Session"])' "$roles" >/dev/null; then
  ok "provenance trailers Change, Agent-Role, Model, Session; enabled is $(jq -r .provenance.enabled "$roles")"
else
  fail "model-roles.json needs provenance.enabled (true or false) and provenance.trailers [\"Change\", \"Agent-Role\", \"Model\", \"Session\"]"
fi
if jq -e '(.prices | type == "object") and ([.prices | to_entries[] | select(.key != "_why") | .value | (.input | type) == "number" and (.output | type) == "number" and (.cache_read | type) == "number"] | length > 0 and all)' "$roles" >/dev/null; then
  ok "prices: $(jq -r '[.prices | keys[] | select(. != "_why")] | length' "$roles") model prefixes, each with input, output and cache_read"
else
  fail "model-roles.json .prices needs '<model prefix>': {input, output, cache_read} in dollars per million tokens (change-cost.sh reads it)"
fi

if [ -f "$wf" ]; then
  # TIERS rows:   "<role>.<tier>": { model: "<m>", effort: "<e>" },   and ECONOMY rows the same by role.
  table() { sed -n "/^const $1 = {/,/^};/p" "$wf" | sed -nE 's/^[[:space:]]*"?([a-z.-]+)"?:[[:space:]]*\{[[:space:]]*model:[[:space:]]*"([^"]*)",[[:space:]]*effort:[[:space:]]*"([^"]*)".*/\1 \2 \3/p' | sort; }
  got=$(table TIERS); want=$(printf '%s\n' "$want_tiers" | grep . | sort)
  if grep -q '^const TIERS = {' "$wf" && [ "$got" = "$want" ]; then ok "build-change.js TIERS matches every role's tier variants"
  else fail "build-change.js TIERS disagrees with model-roles.json's tier variants: has [$(printf '%s' "$got" | tr '\n' ';')], want [$(printf '%s' "$want" | tr '\n' ';')]"; fi
  got=$(table ECONOMY); want=$(jq -r '(.economy.roles // {}) | to_entries[] | "\(.key) \(.value.model) \(.value.effort)"' "$roles" | sort)
  if grep -q '^const ECONOMY = {' "$wf" && [ "$got" = "$want" ]; then ok "build-change.js ECONOMY matches .economy.roles"
  else fail "build-change.js ECONOMY disagrees with .economy.roles: has [$(printf '%s' "$got" | tr '\n' ';')], want [$(printf '%s' "$want" | tr '\n' ';')]"; fi
  g=$(sed -n "s/^const ECONOMY_FILE = '\\(.*\\)'\$/\\1/p" "$wf"); w=$(jq -r '.economy.file // empty' "$roles")
  [ -n "$w" ] && [ "$g" = "$w" ] && ok "build-change.js ECONOMY_FILE is $w" || fail "build-change.js ECONOMY_FILE is '$g', .economy.file says '$w'"
fi

# ── The panel's lane types ─────────────────────────────────────────────────────────────
pj="$ROOT/.clauductor/panel.json"
if [ -f "$pj" ]; then
  for lane in $(jq -r '.lanes | to_entries[] | select(.key | startswith("_") | not) | .key' "$roles"); do
    r=$(jq -r --arg l "$lane" '.lanes[$l]' "$roles")
    for key in model effort; do
      g=$(jq -r --arg l "$lane" --arg k "$key" '.lane_types[$l][$k] // empty' "$pj"); w=$(want "$r" "$key")
      if [ "$g" = "$w" ]; then ok "panel lane_type $lane: $key $g (role $r)"; else fail "panel.json lane_types.$lane.$key is '${g:-unset}', role $r says '$w'"; fi
    done
  done
fi

# ── The playbook's skill table ─────────────────────────────────────────────────────────
# The playbook (PLAYBOOK in project.conf, default docs/playbook.md) lists every skill with its role,
# between `<!-- skills-table begin -->` and `<!-- skills-table end -->`. Both directions: a skill
# the table forgot, a row naming a skill that does not exist, and a role that disagrees. The rows
# are Markdown (| `/skill` | … | role |) or, in an HTML playbook, <tr><td>/skill</td>…<td>role</td>,
# read as the same cells.
pb_table() {  # pb_table FILE: "<skill> <role>" per row of FILE's skills table
  sed -n '/<!-- skills-table begin -->/,/<!-- skills-table end -->/p' "$1" \
    | awk '/^[ \t]*<tr>/ { s = $0; gsub(/<\/t[dh]>/, "\t", s); gsub(/<[^>]*>/, "", s); n = split(s, c, "\t")
        for (i = 1; i <= n; i++) { gsub(/^[ \t]+|[ \t]+$/, "", c[i]) }
        if (n >= 3) printf "| `%s` | | %s |\n", c[1], c[n - 1]; next } { print }' \
    | sed -nE 's/^\| `\/(clauductor:)?([a-z0-9-]+)` \|.*\| ([a-z-]+) \|$/\2 \3/p'
}
pb="$ROOT/$PLAYBOOK"
[ -f "$pb" ] || fail "PLAYBOOK=$PLAYBOOK (.claude/project.conf) does not exist, so its skills table cannot be checked"
if [ -f "$pb" ]; then
  tbl=$(pb_table "$pb")
  [ -n "$tbl" ] || fail "$PLAYBOOK has no skills table between its markers"
  # The same table as HTML rows reads the same (an adopting project's playbook may be a page).
  printf '<!-- skills-table begin -->\n<table>\n' > "$(scratch)/pb.html"
  printf '%s\n' "$tbl" | while read -r s r; do printf '  <tr><td><code>/%s</code></td><td>when</td><td>%s</td></tr>\n' "$s" "$r"; done >> "$(scratch)/pb.html"
  printf '</table>\n<!-- skills-table end -->\n' >> "$(scratch)/pb.html"
  [ "$(pb_table "$(scratch)/pb.html")" = "$tbl" ] && ok "the skills table reads the same from an HTML playbook" || fail "an HTML skills table reads differently: $(pb_table "$(scratch)/pb.html" | head -3 | tr '\n' ';')"
  printf '%s\n' "$tbl" | while read -r s r; do
    [ -n "$s" ] || continue
    w=$(role_of skills "$s")
    if [ -z "$w" ]; then echo "FAIL playbook lists /$s, which is not a skill in model-roles.json"
    elif [ "$w" = "$r" ]; then echo "ok   playbook /$s role $r"
    else echo "FAIL playbook says /$s runs as $r, model-roles.json says $w"; fi
  done > "$(scratch)/pb"
  for s in $(jq -r '.skills | keys[]' "$roles"); do
    printf '%s\n' "$tbl" | grep -q "^$s " || echo "FAIL skill $s is missing from $PLAYBOOK's skills table" >> "$(scratch)/pb"
  done
  cat "$(scratch)/pb"; _fails=$((_fails + $(grep -c '^FAIL' "$(scratch)/pb")))
fi

# ── Eval evidence (OPS-10) ─────────────────────────────────────────────────────────────
# A role with a seeded-defect suite records what its model and effort rest on: a passing receipt
# of .claude/evals/run.sh for exactly that choice, or the baseline it had before the suite existed.
# So changing the model or effort without a new receipt fails here. The roles are the suite
# directories (the authority), both directions: a suite whose role records nothing, and evidence
# for a role with no suite. The receipt's hashes are pr-merge-guard rule 13's to hold, not this
# check's: here a receipt is judged on its role, model, effort, completeness and thresholds.
evlib="$CLAUDUCTOR_FW/lib/evals.sh"
if [ ! -f "$evlib" ]; then
  fail "cannot find $evlib, so the eval evidence cannot be checked"
else
  . "$evlib"
  if jq -e '(.evals.thresholds.recall | type) == "number" and (.evals.thresholds.fp_rate | type) == "number"' "$roles" >/dev/null; then
    ok "eval thresholds: $(jq -r '.evals.thresholds | to_entries | map("\(.key) \(.value)") | join(", ")' "$roles")"
  else
    fail "model-roles.json needs .evals.thresholds with a numeric recall and fp_rate (and optionally severity_accuracy)"
  fi
  suites=$(evals_suite_roles "$ROOT")
  # The trigger inputs rule 13 holds a receipt to (OPS-16): each declared role is a role, each
  # input is a string, and each marked section present in this tree is readable. A marked file
  # this project does not have (a plugin project's workflows live in the plugin) hashes as "none"
  # at both ends of a PR, so it is reported, not failed.
  if ! jq -e '(.evals.triggers // {}) | type == "object"' "$roles" >/dev/null; then
    fail "model-roles.json .evals.triggers must be an object of role -> [inputs]"
  else
    for r in $(jq -r '(.evals.triggers // {}) | keys[] | select(startswith("_") | not)' "$roles"); do
      if ! jq -e --arg r "$r" '.roles | has($r)' "$roles" >/dev/null; then fail "evals.triggers.$r: '$r' is not a role in .roles"; continue; fi
      if ! jq -e --arg r "$r" '.evals.triggers[$r] | type == "array" and length > 0 and all(.[]; type == "string" and length > 0)' "$roles" >/dev/null; then
        fail "evals.triggers.$r must be a non-empty list of inputs (path, dir/ or path#MARKER)"; continue
      fi
      for i in $(evals_triggers "$r" "$roles"); do
        h=$(evals_input_hash "$ROOT" "$i")
        case "$h" in
          markers-missing) fail "evals.triggers.$r: ${i%%#*} has no '// <${i#*#}>' ... '// </${i#*#}>' section (each marker once, in order, on lines of their own), so rule 13 cannot tell an edit inside it from one outside. Restore the markers" ;;
          none) ok "evals.triggers.$r: $i is not in this tree (it hashes as none at both ends of a PR)" ;;
          *) ok "evals.triggers.$r: $i is readable ($h)" ;;
        esac
      done
    done
  fi
  # The markers only protect what is between them. The review must not be reachable from outside:
  # a second reviewer spawn, the REVIEW schema or reviewSpawn used elsewhere in build-change.js would
  # change what the reviewer is in lines rule 13 does not hash.
  wfr="$CLAUDUCTOR_FW/workflows/build-change.js"
  if [ -f "$wfr" ]; then
    outside=$(awk '
      { t = $0; sub(/^[ \t]+/, "", t) }
      t == "// <review-prompt>" || t == "// <review-call>" { in_s = 1; next }
      t == "// </review-prompt>" || t == "// </review-call>" { in_s = 0; next }
      t ~ /^\/\// { next }
      !in_s && /agentType: *.([A-Za-z0-9_-]+:)?reviewer.|schema: *REVIEW[^_A-Za-z]|schema: *REVIEW$|reviewSpawn\(|reviewPrompt\(|REVIEW_PROMPT|[^.A-Za-z_]rev *= *[^=]|rev\.findings *= *[^=]|rev\.findings\.(push|pop|shift|unshift|splice|length *= *[^=])|Object\.assign\(rev[,)]/ { print NR ": " t }' "$wfr")
    if [ -z "$outside" ]; then ok "build-change.js spawns the reviewer only inside its marked review-prompt and review-call sections"
    else fail "build-change.js reaches the reviewer outside the marked sections rule 13 hashes, so an edit there would change the review with no eval: $(printf '%s' "$outside" | tr '\n' ';')"; fi
    # The model tables are restated literals (above) and must stay what was read: nothing in the
    # script may assign into, delete from or Object.assign onto ROLES, TIERS or ECONOMY. (They are
    # not frozen in the workflow itself: editing build-change.js would void the reviewer's receipt
    # under guards that hash the whole workflows tree. checks/build-change.sh also runs the script's
    # whole prefix and asks pick('reviewer') afterwards.)
    mut=$(awk '
      { t = $0; sub(/^[ \t]+/, "", t) }
      t ~ /^\/\// { next }
      /(^|[^A-Za-z0-9_$.])(ROLES|TIERS|ECONOMY)((\.[A-Za-z_$][A-Za-z0-9_$]*)|(\[[^]]*\]))+[ \t]*([-+*\/]?=)([^=]|$)/ \
        || /delete[ \t]+(ROLES|TIERS|ECONOMY)[.[]/ \
        || /Object\.(assign|defineProperty|defineProperties|setPrototypeOf)\([ \t]*(ROLES|TIERS|ECONOMY)([.[,)]|[ \t])/ { print NR ": " t }' "$wfr")
    if [ -z "$mut" ]; then ok "build-change.js never mutates its ROLES, TIERS or ECONOMY tables"
    else fail "build-change.js mutates a model table at run time, so pick() would not return what model-roles.json says: $(printf '%s' "$mut" | tr '\n' ';')"; fi
  fi
  for r in $suites; do
    m=$(want "$r" model); e=$(want "$r" effort)
    if [ -z "$m" ]; then fail "eval suite .claude/evals/$r/ is for role '$r', which .roles does not define"; continue; fi
    ev=$(jq -c --arg r "$r" '.roles[$r].eval // empty' "$roles")
    if [ -z "$ev" ]; then
      fail "role $r has an eval suite but no .roles.$r.eval: run sh .claude/evals/run.sh --role $r --model $m --effort $e and record its receipt"
      continue
    fi
    rc=$(printf '%s' "$ev" | jq -r '.receipt // empty'); base=$(printf '%s' "$ev" | jq -r '.baseline // empty')
    if [ -n "$rc" ]; then
      why=$(evals_verdict "$ROOT/$rc" "$r" "$m" "$e" "$roles")
      if [ -n "$why" ]; then
        fail "role $r is $m/$e, and its receipt $rc is not passing evidence for that: $(printf '%s' "$why" | tr '\n' ';' | sed 's/;/; /g'). Re-run: sh .claude/evals/run.sh --role $r --model $m --effort $e"
        continue
      fi
      drift=$(jq -r --slurpfile x "$ROOT/$rc" --arg r "$r" '.roles[$r].eval as $v | $x[0] as $x
        | [ ("recall", "precision", "fp_rate", "severity_accuracy") as $k | select($v[$k] != $x.scores[$k]) | "\($k) \($v[$k]) (receipt: \($x.scores[$k]))" ]
          + (if $v.cost_usd != $x.cost.usd then ["cost_usd \($v.cost_usd) (receipt: \($x.cost.usd))"] else [] end) | join(", ")' "$roles")
      if [ -z "$drift" ]; then
        ok "role $r: $m/$e rests on $rc ($(jq -r '"recall \(.scores.recall), fp_rate \(.scores.fp_rate), $\(.cost.usd)"' "$ROOT/$rc"))"
      else
        fail "role $r: the scores model-roles.json records disagree with $rc: $drift"
      fi
    elif [ -n "$base" ]; then
      if [ "$base" = "$m/$e" ]; then ok "role $r: $m/$e is its unmeasured baseline (a change of model or effort needs a passing receipt)"
      else fail "role $r is now $m/$e, but its evidence is the baseline $base: a changed model or effort needs a passing receipt. Run sh .claude/evals/run.sh --role $r --model $m --effort $e and record it"; fi
    else
      fail "role $r: .roles.$r.eval has neither a receipt nor a baseline"
    fi
  done
  for r in $(jq -r '.roles | to_entries[] | select(.value.eval != null) | .key' "$roles"); do
    printf '%s\n' "$suites" | grep -qx "$r" || fail "role $r records eval evidence, but there is no suite .claude/evals/$r/cases/ to have produced it"
  done
fi

# ── Attribution ────────────────────────────────────────────────────────────────────────
if jq -e '.attribution.enabled | type == "boolean"' "$roles" >/dev/null; then
  ok "attribution.enabled is $(jq -r '.attribution.enabled' "$roles")"
  if jq -e '.attribution.enabled == false or ((.attribution.trailer // "") | test("^[A-Za-z-]+: .+"))' "$roles" >/dev/null; then
    ok "attribution trailer is a git trailer"
  else
    fail "attribution.trailer must be a git trailer ('Key: value') when enabled"
  fi
else
  fail "model-roles.json needs attribution.enabled (true or false)"
fi
finish
