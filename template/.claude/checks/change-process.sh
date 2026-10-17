#!/bin/sh
# The change process's steps are wired where they run, not only described (AGENTS.md rule 4: a
# delegation target is a named thing). Each line below is a step a skill, a workflow or a lane
# template must carry; removing it from there fails this check. The steps' own behaviour is
# falsified elsewhere (changes.sh, scenarios.sh, change-tools.sh, merge-guard.sh, gate.sh).
. "$(dirname "$0")/lib.sh"
need jq

has() {  # has FILE FIXED-TEXT WHAT
  if grep -qF -- "$2" "$ROOT/$1" 2>/dev/null; then ok "$3 ($1)"; else fail "$1 no longer carries: $3 (looked for '$2')"; fi
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
has .claude/workflows/build-change.js 'sh .claude/verify-change.sh' "build-change runs the verify step"
has .claude/skills/merge-pr/SKILL.md 'verify-change' "merge-pr runs the verify step"

# Approval (D8): recorded with the design's hash.
has .claude/skills/propose/SKILL.md 'change-approval.sh <id> --record' "propose records approval over the design hash"
# Review before approval (OPS-76): the rounds are spent before the owner reads it, so a finding does
# not void an approval; merge-pr then reviews only what changed since. This holds the steps in
# place; nothing checks that the review ran or that its PR-body line is true.
has .claude/skills/propose/SKILL.md '2b. **Review it to convergence BEFORE the owner reads it.**' "propose reviews the proposal before the owner reads it"
has .claude/skills/propose/SKILL.md 'an untested scenario is not a finding here' "propose tells its reviewer that tests are the build's, so a proposal can converge"
has .claude/skills/propose/SKILL.md 'end your turn with no final report and without SubagentHandback' "propose waits for its reviewer by ending its turn"
has .claude/skills/propose/SKILL.md '`Review: NOT converged after <n> round(s)' "propose writes a distinct line for a review that did not converge"
has .claude/skills/merge-pr/SKILL.md '`propose` step 2b reviewed it' "merge-pr reviews a proposal PR only since propose's review"
has .claude/skills/merge-pr/SKILL.md 'Only a body line starting `Review: converged in`' "merge-pr skips review only on a converged line"
has .claude/skills/merge-pr/SKILL.md 'the Approved line names that design hash' "merge-pr skips review only when the approval covers the reviewed design"

# Budget, outcome and provenance at archive and merge (items 12–14).
has .claude/skills/archive-change/SKILL.md 'change-cost.sh' "archive records the actual cost"
has .claude/skills/archive-change/SKILL.md 'check-outcome-<id>' "archive queues the outcome check"
has .claude/skills/merge-pr/SKILL.md 'Agent-Role:' "merge-pr writes the provenance trailers"
has .claude/workflows/build-change.js "stop('budget'" "build-change stops over budget"

# The reviewer checks that a citing test asserts the THEN (D4).
has .claude/agents/reviewer.md 'assert the THEN' "the reviewer checks that each citing test asserts the THEN"
finish
