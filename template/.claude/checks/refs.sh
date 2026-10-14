#!/bin/sh
# What checks/skills.sh, checks/hooks.sh and checks/agents-md-budget.sh read, held on fixtures.
# The three read their sources through the shared readers in .claude/lib/refs.sh (OPS-40, D11),
# and so does map.sh. A one-off comparison of the old and new output would prove nothing once the
# old copies were gone, so these cases run each REAL check with ROOT=<fixture>, before the swap
# and for good after it:
#   skills.sh            fails "named but missing" for a missing .claude/ path named in a SKILL.md
#                        (.sh), an agent (.js), AGENTS.md (.awk) and settings.json (.json); passes
#                        once each exists; passes when only the per-machine settings.local.json
#                        is named and absent
#   hooks.sh             fails when settings.json does not parse (its non-vacuity line, its
#                        expectation table and its scripts() table hold the rest, in hooks.sh)
#   agents-md-budget.sh  passes a 320-character row, fails a 321-character one, and fails when
#                        the table is missing
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
# Fixture paths are spelled through $D, so no line here is itself a path a check reads.
D=.claude

# fixture NAME CHECK…: a project at $d/NAME with the model's libraries and the checks named.
fixture() {
  F="$d/$1"; shift
  mkdir -p "$F/.claude/checks"
  cp -R "$ROOT/.claude/lib" "$F/.claude/"
  cp "$ROOT/.claude/checks/lib.sh" "$F/.claude/checks/"
  for c in "$@"; do cp "$ROOT/.claude/checks/$c.sh" "$F/.claude/checks/"; done
}
# crun CHECK: run the fixture's copy of CHECK against the fixture; its output is in $d/out.
crun() { rc=0; (ROOT="$F" sh "$F/.claude/checks/$1.sh") > "$d/out" 2>&1 || rc=$?; }
fails_with() {  # fails_with MSG TEXT: the last run exited non-zero with a FAIL line holding TEXT
  if [ "$rc" -ne 0 ] && grep '^FAIL' "$d/out" | grep -qF -- "$2"; then ok "$1"
  else fail "$1 (exit $rc, no FAIL line holding '$2'):"; grep '^FAIL' "$d/out" | head -5 | sed 's/^/       /'; fi
}
passes() {  # passes MSG: the last run exited 0
  if [ "$rc" -eq 0 ]; then ok "$1"; else fail "$1 (exit $rc):"; grep '^FAIL' "$d/out" | head -5 | sed 's/^/       /'; fi
}

# ── skills.sh: a named .claude/ path, per input kind and extension ──────────────────────────────
fixture skills skills
mkdir -p "$F/$D/skills/s" "$F/$D/agents"
printf -- '---\nname: s\ndescription: a fixture skill\n---\nRun `sh %s/gone-skill.sh` first.\n' "$D" > "$F/$D/skills/s/SKILL.md"
printf -- '---\nname: a\ndescription: a fixture agent\n---\nIt reads %s/gone-agent.js.\n' "$D" > "$F/$D/agents/a.md"
printf '# Working conventions\n\n| x | **`%s/gone-agents.awk`** |\n' "$D" > "$F/AGENTS.md"
printf '{"statusLine": {"command": "jq . %s/gone-settings.json"}}\n' "$D" > "$F/$D/settings.json"
crun skills
fails_with "skills.sh fails a missing path a SKILL.md names (.sh)" "named but missing: $D/gone-skill.sh"
fails_with "skills.sh fails a missing path an agent names (.js)" "named but missing: $D/gone-agent.js"
fails_with "skills.sh fails a missing path AGENTS.md names (.awk)" "named but missing: $D/gone-agents.awk"
fails_with "skills.sh fails a missing path settings.json names (.json)" "named but missing: $D/gone-settings.json"
for p in gone-skill.sh gone-agent.js gone-agents.awk gone-settings.json; do : > "$F/$D/$p"; done
crun skills
passes "skills.sh passes once each named path exists"

fixture skills-local skills
mkdir -p "$F/$D/skills/s"
printf -- '---\nname: s\ndescription: a fixture skill\n---\nLoosen it in `%s/settings.local.json`.\n' "$D" > "$F/$D/skills/s/SKILL.md"
crun skills
passes "skills.sh passes when only the per-machine settings.local.json is named, and absent"

# ── hooks.sh: a settings.json that does not parse ───────────────────────────────────────────────
# The hooks and what they source are copied, so the rows before the registration still run.
fixture hooks hooks
cp -R "$ROOT/.claude/hooks" "$F/.claude/"
cp "$ROOT/.claude/status-write.sh" "$F/.claude/"
printf '{"hooks": {"PreToolUse": [\n' > "$F/$D/settings.json"
crun hooks
fails_with "hooks.sh fails a settings.json that does not parse" "settings.json does not parse"

# ── agents-md-budget.sh: the row ceiling and the table anchor ───────────────────────────────────
# row N: a table row of exactly N characters.
row() { awk -v n="$1" 'BEGIN { s = "| a row of the length asked for | "; while (length(s) < n - 2) s = s "x"; print s " |" }'; }
# agents BODY: an AGENTS.md whose table holds BODY above its closing "Nothing. You." row.
agents() {
  printf '# Working conventions\n\n### What executes each rule\n\n| Rule / convention | What executes it |\n|---|---|\n%s\n| **Everything else** | **Nothing. You.** |\n\nRun every check.\n' "$1" > "$F/AGENTS.md"
}
fixture budget agents-md-budget
printf '@AGENTS.md\n' > "$F/CLAUDE.md"
[ "$(row 320 | awk '{ print length($0) }')" = 320 ] && [ "$(row 321 | awk '{ print length($0) }')" = 321 ] \
  && ok "the fixture rows are 320 and 321 characters" || fail "the fixture rows are not 320 and 321 characters"
agents "$(row 320)"
crun agents-md-budget
passes "agents-md-budget.sh passes a 320-character row"
agents "$(row 321)"
crun agents-md-budget
fails_with "agents-md-budget.sh fails a 321-character row" "rows over 320 characters"
printf '# Working conventions\n\n%s\n| **Everything else** | **Nothing. You.** |\n' "$(row 40)" > "$F/AGENTS.md"
crun agents-md-budget
fails_with "agents-md-budget.sh fails when the table is missing" "no 'What executes each rule' table found"
finish
