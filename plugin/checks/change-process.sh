#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The change process's steps are wired where they run, not only described (AGENTS.md rule 4: a
# delegation target is a named thing). Each line below is a step a skill, a workflow or a lane
# template must carry; removing it from there fails this check. The steps' own behaviour is
# falsified elsewhere (changes.sh, scenarios.sh, change-tools.sh, merge-guard.sh, gate.sh).
. "$(dirname "$0")/lib.sh"
need jq

has() {  # has FILE FIXED-TEXT WHAT
  _f="$ROOT/$1"; case $1 in .claude/*) [ -e "$_f" ] || _f="$CLAUDUCTOR_FW/${1#.claude/}" ;; esac
  if grep -qF -- "$2" "$_f" 2>/dev/null; then ok "$3 ($1)"; else fail "$1 no longer carries: $3 (looked for '$2')"; fi
}

# The fast path (D5): a one-sentence diff gets no proposal.
has .claude/skills/propose/SKILL.md "## The fast path" "propose states the fast path"
has "$PLAYBOOK" "fast path" "the playbook states the fast path"
for t in fix ops; do
  title=$(jq -r --arg t "$t" '.templates[] | select(.id == $t) | .title' "$ROOT/.clauductor/panel.json" 2>/dev/null)
  case "$title" in *"fast path"*) ok "the $t lane template's title offers the fast path" ;; *) fail "the $t lane template's title does not mention the fast path: '$title'" ;; esac
done

# Scenario IDs (D3), in every place a scenario is written.
for f in changes/README.md specs/README.md .claude/skills/propose/SKILL.md .claude/modules/openspec/config.yaml; do
  has "$f" "[CAP-n-Sn]" "the scenario ID grammar is given"
done

# The log (D6): build-change has the builder keep it and writes Progress per group.
has .claude/workflows/build-change.js '## Decision log' "build-change has the builder keep the Decision log"
has .claude/workflows/build-change.js '## Progress' "build-change writes Progress per committed group"
# The gate it runs is the project's (P1.7): GATE and GATE_QUICK_FLAGS, read ONCE, through
# project-config.sh --json (a second read could replace one with the other; checks/build-change.sh).
has .claude/project-config.sh '--arg quick "$GATE_QUICK_FLAGS"' "project-config.sh --json carries the project's GATE_QUICK_FLAGS"
has .claude/workflows/build-change.js 'GATE_CMD = CFG.gate' "build-change runs the project's GATE (project-config.sh) when no arg names one"
has .claude/workflows/build-change.js 'QUICK = CFG.quickFlags' "build-change runs the project's GATE_QUICK_FLAGS (project-config.sh) when no arg names them"
has .claude/agents/builder.md '## Decision log' "the builder agent keeps the Decision log"

# Verify (D7): in build-change and in merge-pr.
has .claude/workflows/build-change.js 'clauductor-model verify-change.sh' "build-change runs the verify step"
has .claude/skills/merge-pr/SKILL.md 'verify-change' "merge-pr runs the verify step"

# Approval (D8): recorded with the design's hash.
has .claude/skills/propose/SKILL.md 'change-approval.sh <id> --record' "propose records approval over the design hash"

# Budget, outcome and provenance at archive and merge (items 12–14).
has .claude/skills/archive-change/SKILL.md 'change-cost.sh' "archive records the actual cost"
has .claude/skills/archive-change/SKILL.md 'check-outcome-<id>' "archive queues the outcome check"
has .claude/skills/merge-pr/SKILL.md 'Agent-Role:' "merge-pr writes the provenance trailers"
has .claude/workflows/build-change.js "stop('budget'" "build-change stops over budget"

# The reviewer checks that a citing test asserts the THEN (D4).
has .claude/agents/reviewer.md 'assert the THEN' "the reviewer checks that each citing test asserts the THEN"
finish
