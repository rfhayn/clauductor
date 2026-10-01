#!/bin/sh
# review-lane.sh sends a PR to the lighter docs review only when every changed path is prose, and
# to the full reviewer otherwise, including for an empty diff and a last path with no newline.
. "$(dirname "$0")/lib.sh"
L="$ROOT/.claude/skills/merge-pr/review-lane.sh"
lane() { printf "$2" | sh "$L" | awk '{print $2}' | tr -d ';'; }
t() { [ "$(lane x "$2")" = "$1" ] && ok "$1: $3" || fail "want $1 for $3, got $(printf "$2" | sh "$L")"; }
t docs 'docs/a.md\nREADME.md\n' "Markdown only"
t full 'docs/a.md\nsrc/x.go\n' "Markdown plus code"
t full 'docs/a.md\nsrc/x.go' "a last code path with no trailing newline"
t full '' "an empty diff"
t full '.claude/skills/x/SKILL.md\n' "a SKILL.md (executable config)"
t full 'AGENTS.md\n' "AGENTS.md (every agent loads it)"
t full "$SPECS_DIR/auth/spec.md\n" "a living spec"
t full "$CHANGES_DIR/add-x/specs/auth/spec.md\n" "a change's spec delta"
t docs "$CHANGES_DIR/add-x/proposal.md\n" "a proposal's prose"
t full 'docs/data.json\n' "a JSON doc"
finish
