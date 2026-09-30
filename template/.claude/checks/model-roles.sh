#!/bin/sh
# A model or an effort is chosen in ONE place: .claude/model-roles.json. Everything that cannot
# read that file restates it, and this check fails when any restatement disagrees:
#   - every skill's and every agent's frontmatter `model:` and `effort:`;
#   - .claude/settings.json `model`, `effortLevel` and CLAUDE_CODE_SUBAGENT_MODEL;
#   - .claude/workflows/build-change.js's ROLES table, when the workflow exists;
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
for d in "$ROOT"/.claude/skills/*/; do
  [ -f "$d/SKILL.md" ] || continue
  s=$(basename "$d")
  r=$(role_of skills "$s")
  if [ -z "$r" ]; then fail "skill $s is not mapped to a role in model-roles.json .skills"; continue; fi
  jq -e --arg r "$r" '.roles[$r]' "$roles" >/dev/null || { fail "skill $s: role '$r' is not defined in .roles"; continue; }
  match skill "$s" "$d/SKILL.md" "$r"
done
for s in $(jq -r '.skills | keys[]' "$roles"); do
  [ -f "$ROOT/.claude/skills/$s/SKILL.md" ] || fail "model-roles.json maps skill '$s', which does not exist"
done

# ── Agents ─────────────────────────────────────────────────────────────────────────────
known=" Read Grep Glob Edit Write MultiEdit NotebookEdit Bash BashOutput KillShell Skill Agent Task WebFetch WebSearch TodoWrite ReportFindings ${CHECK_EXTRA_TOOLS:-} "
for f in "$ROOT"/.claude/agents/*.md; do
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
  [ -f "$ROOT/.claude/agents/$a.md" ] || fail "model-roles.json maps agent '$a', which does not exist"
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
wf="$ROOT/.claude/workflows/build-change.js"
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
  # Its default commit trailer restates attribution (empty when attribution is disabled).
  g=$(sed -n "s/^const ATTRIBUTION_DEFAULT = '\\(.*\\)'\$/\\1/p" "$wf")
  w=$(jq -r 'if .attribution.enabled then .attribution.trailer else "" end' "$roles")
  if grep -q '^const ATTRIBUTION_DEFAULT = ' "$wf" && [ "$g" = "$w" ]; then ok "build-change.js ATTRIBUTION_DEFAULT matches attribution ('${w:-disabled}')"
  else fail "build-change.js ATTRIBUTION_DEFAULT is '$g', model-roles.json attribution says '${w:-(disabled: empty)}'"; fi
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
# docs/playbook.md lists every skill with its role, between markers. Both directions: a skill the
# table forgot, a row naming a skill that does not exist, and a role that disagrees.
pb="$ROOT/docs/playbook.md"
if [ -f "$pb" ]; then
  tbl=$(sed -n '/<!-- skills-table begin -->/,/<!-- skills-table end -->/p' "$pb" | sed -nE 's/^\| `\/([a-z0-9-]+)` \|.*\| ([a-z-]+) \|$/\1 \2/p')
  [ -n "$tbl" ] || fail "docs/playbook.md has no skills table between its markers"
  printf '%s\n' "$tbl" | while read -r s r; do
    [ -n "$s" ] || continue
    w=$(role_of skills "$s")
    if [ -z "$w" ]; then echo "FAIL playbook lists /$s, which is not a skill in model-roles.json"
    elif [ "$w" = "$r" ]; then echo "ok   playbook /$s role $r"
    else echo "FAIL playbook says /$s runs as $r, model-roles.json says $w"; fi
  done > "$(scratch)/pb"
  for s in $(jq -r '.skills | keys[]' "$roles"); do
    printf '%s\n' "$tbl" | grep -q "^$s " || echo "FAIL skill $s is missing from docs/playbook.md's skills table" >> "$(scratch)/pb"
  done
  cat "$(scratch)/pb"; _fails=$((_fails + $(grep -c '^FAIL' "$(scratch)/pb")))
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
