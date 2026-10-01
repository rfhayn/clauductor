#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# pr-merge-guard.sh, as a payload → exit-code table in both directions, against a throwaway repo
# whose origin is a local bare repo named like `owner/name` (so the foreign-repo rules run) and a
# stub `gh` on PATH (so no network and no GitHub are needed). Also: the receipt library across
# worktrees, the channel an advisory takes, and raw_command identical in both hooks that carry it.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/hooks/lib" "$R/.claude/lib" "$R/docs" "$d/bin"
cp "$CLAUDUCTOR_FW/hooks/pr-merge-guard.sh" "$R/.claude/hooks/"
cp "$CLAUDUCTOR_FW/hooks/lib/"* "$R/.claude/hooks/lib/"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/evals.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$R/.claude/"
cat > "$R/.claude/project.conf" <<'EOF'
GATE_DISPLAY_CONTEXTS="ci/local"
EOF
printf '# Journal\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" add -A && git -C "$R" commit -qm base
# origin: a bare repo INSIDE the checkout at acme/app.git, so `git remote get-url origin` reads
# `acme/app.git`, which the guard keys as acme/app, and fetches still work offline.
git init -q --bare "$R/acme/app.git"
echo "acme/" >> "$R/.git/info/exclude"
git -C "$R" remote add origin acme/app.git
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)

# A capability-change branch with a change directory, and its head.
git -C "$R" checkout -q -b change/add-x
mkdir -p "$R/changes/add-x"
printf '## 1. Do it\n- [x] 1.1 thing\n' > "$R/changes/add-x/tasks.md"
git -C "$R" add -A && git -C "$R" commit -qm "add-x: task group 1"
NOSLICE=$(git -C "$R" rev-parse HEAD)
printf -- '- [ ] Slice: a user can do x at /x\n' >> "$R/changes/add-x/tasks.md"
git -C "$R" commit -qam "add-x: slice"
SLICE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main

cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "pr checks") echo "${GH_CHECKS:-[]}" ;;
  "pr view")
    case "$*" in
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "${GH_BRANCH:-fix/1-x}" "$GH_HEAD" ;;
      *headRefOid*) echo "$GH_HEAD" ;;
    esac ;;
  "run list") echo "${GH_RUNS:-0}" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"

MAINSHA=$(git -C "$R" rev-parse HEAD)
receipt() { printf '%s\tfull\t%s\t%s\n' "$1" "${2:-clean}" "${3:-all}" > "$R/.git/ci-receipt"; }
guard() {  # guard WANT LABEL COMMAND [env assignments...]
  want=$1 label=$2 c=$3; shift 3
  rc=0
  out=$(payload "$c" "$R" | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="${GH_HEAD:-$MAINSHA}" "$@" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  expect_rc "$want" "$rc" "merge-guard $( [ "$want" = 2 ] && echo blocks || echo allows ): $label"
  [ "$rc" = "$want" ] || sed 's/^/       /' "$d/err" | head -3
}

guard 0 "a command that is not a merge" "git status"
guard 0 "gh pr view" "gh pr view 5"
guard 2 "--auto" "gh pr merge 5 --squash --auto"
guard 2 "--admin" "gh pr merge 5 --squash --admin"
rm -f "$R/.git/ci-receipt"
guard 2 "no evidence at all" "gh pr merge 5 --squash --delete-branch"
receipt "$MAINSHA"
guard 0 "a full clean receipt for the head" "gh pr merge 5 --squash --delete-branch"
case "$out" in *'"additionalContext"'*"passed in full"*) ok "merge-guard: its confirmation reaches Claude as additionalContext" ;; *) fail "merge-guard: no additionalContext on an allowed merge: $out" ;; esac
receipt "$MAINSHA" dirty
guard 2 "a dirty receipt" "gh pr merge 5 --squash"
receipt "$MAINSHA" clean quick
guard 2 "a partial receipt" "gh pr merge 5 --squash"
receipt 0000000000000000000000000000000000000000
guard 2 "a receipt for another commit" "gh pr merge 5 --squash"
rm -f "$R/.git/ci-receipt"
guard 2 "no remote workflow named, so a remote success does not count" "gh pr merge 5 --squash" GH_RUNS=1
echo 'GATE_REMOTE_WORKFLOW="ci.yml"' >> "$R/.claude/project.conf"
guard 0 "the named remote workflow succeeded" "gh pr merge 5 --squash" GH_RUNS=1
guard 2 "the named remote workflow has no success on the head" "gh pr merge 5 --squash" GH_RUNS=0

# A receipt written in a linked worktree's git dir counts.
git -C "$R" worktree add -q "$d/lane" 2>/dev/null
printf '%s\tfull\tclean\tall\n' "$MAINSHA" > "$(git -C "$d/lane" rev-parse --absolute-git-dir)/ci-receipt"
guard 0 "a receipt in a linked worktree's git dir" "gh pr merge 5 --squash"
guard 2 "a red check" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"lint","state":"FAILURE","bucket":"fail"}]'
guard 0 "a red DISPLAY context (GATE_DISPLAY_CONTEXTS)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"FAILURE","bucket":"fail"}]'

# Rule 3: the slice line, read at the head from the PR's own change directory.
printf '%s\tfull\tclean\tall\n' "$NOSLICE" > "$R/.git/ci-receipt"
guard 2 "a change PR whose tasks.md has no Slice line" "gh pr merge 5 --squash" GH_HEAD="$NOSLICE" GH_BRANCH=change/add-x
printf '%s\tfull\tclean\tall\n' "$SLICE" > "$R/.git/ci-receipt"
guard 0 "a change PR that states its slice" "gh pr merge 5 --squash" GH_HEAD="$SLICE" GH_BRANCH=change/add-x

# Rule 7: a journal session number merged elsewhere after this branch was cut.
git -C "$R" checkout -q -b ops/close main
sed -i.bak 's/^## Session 1/## Session 2 — 2026-01-02 — b — mine\n\n## Session 1/' "$R/docs/development-journal.md" 2>/dev/null
printf '# Journal\n\n## Session 2 — 2026-01-02 — b — mine\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
rm -f "$R/docs/development-journal.md.bak"
git -C "$R" commit -qam "close: session 2"
MINE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main
printf '# Journal\n\n## Session 2 — 2026-01-02 — c — theirs\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" commit -qam "their close: session 2"
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
printf '%s\tfull\tclean\tall\n' "$MINE" > "$R/.git/ci-receipt"
guard 2 "a journal session number another merged session took" "gh pr merge 5 --squash" GH_HEAD="$MINE" GH_BRANCH=ops/close
receipt "$MAINSHA"

# Rules 9–12: the change process, read at the head (lib/change-guard.sh).
# head_of BRANCH: commit what the caller staged on BRANCH (made from main), echo the sha, back to main.
on() { git -C "$R" checkout -q -B "$1" main; }
head_of() { git -C "$R" add -A && git -C "$R" commit -qm "$1" && h=$(git -C "$R" rev-parse HEAD) && git -C "$R" checkout -q main && printf '%s\n' "$h" > "$d/head"; }
at() { printf '%s\tfull\tclean\tall\n' "$(cat "$d/head")" > "$R/.git/ci-receipt"; }
H() { cat "$d/head"; }

on change/add-y; mkdir -p "$R/changes/add-y" "$R/src"
printf '## 1. Do it\n- [x] 1.1 thing\n- [ ] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
echo 'code' > "$R/src/y.txt"; head_of "build with an open task"; at
guard 2 "rule 9: a build PR whose change still has an open task" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y"
printf '## 1. Do it\n- [ ] 1.1 thing\n- [ ] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
printf '# Roadmap\n' > "$R/docs/roadmap.md"
head_of "the proposal, and its roadmap note"; at
guard 0 "rule 9: a proposal PR (the change's own files and the roadmap, every task open)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y"
printf '## 1. Do it\n- [ ] 1.1 thing\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
printf '# Guide\n' > "$R/docs/guide.md"
head_of "a docs-only build"; at
guard 2 "rule 9: a build that only edits docs (not the roadmap) is still a build" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y" "$R/src"
printf '## 1. Do it\n- [x] 1.1 thing\n- [x] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
echo 'code' > "$R/src/y.txt"; head_of "build, every task done"; at
guard 0 "rule 9: a build PR whose every task is ticked" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y

on ops/spec; mkdir -p "$R/specs/cap" "$R/tests"
printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$R/specs/cap/spec.md"
head_of "a living scenario no test cites"; at
guard 2 "rule 10: a living scenario no test cites at the head" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/spec
on ops/spec; mkdir -p "$R/specs/cap" "$R/tests"
printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$R/specs/cap/spec.md"
echo '# CAP-1-S1' > "$R/tests/cap.sh"; head_of "a living scenario a test cites"; at
guard 0 "rule 10: a living scenario a test cites at the head" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/spec

A=changes/archive/2026-01-05-add-z
archive() {  # archive TASKS [SPEC|skip|none] [KNOW] [ROW]
  on ops/archive; mkdir -p "$R/$A"
  printf '%b' "$1" > "$R/$A/tasks.md"
  printf '# p\n' > "$R/$A/proposal.md"
  case "$2" in spec) mkdir -p "$R/$A/specs/z"; printf '## ADDED Requirements\n' > "$R/$A/specs/z/spec.md" ;; skip) printf 'skip_specs: true\n' > "$R/$A/.openspec.yaml" ;; esac
  [ -n "${3:-}" ] && printf "## How we'll know\n- **Signal:** s\n" >> "$R/$A/proposal.md"
  [ -n "${4:-}" ] && printf '| o.1 | `ops/check-outcome-add-z` — check the outcome of add-z (due 2026-02-05) | s | — | ⬜ queued |\n' > "$R/docs/roadmap.md"
  head_of "archive add-z"; at
}
DONE='- [x] 1.1 a\n\n## Progress\n- 2026-01-05 archived: actual cost $3.10 of budget $15\n'
archive "$DONE" spec
guard 0 "rule 11: archiving a finished change (tasks ticked, a delta, its cost recorded)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive '- [ ] 1.1 a\n- actual cost unknown (built on another machine)\n' spec
guard 2 "rule 11: archiving a change with an open task" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive '- [x] 1.1 a\n' spec
guard 2 "rule 11: archiving a change with no recorded actual cost" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" none
guard 2 "rule 11: archiving a change with no spec delta and no skip_specs" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" skip
guard 0 "rule 11: archiving a no-delta change that declares skip_specs: true" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" spec know
guard 2 "rule 11: archiving a change that says how we'll know, with no outcome check queued" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" spec know row
guard 0 "rule 11: ...and with its outcome check queued in the roadmap" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
git -C "$R" checkout -q main; receipt "$MAINSHA"

# Rule 12: provenance trailers, while model-roles.json enables them.
# The template ships it on; an installed project (it has an install or plugin marker) may turn it
# off, as clauductor's own repo does, so the fixture forces it on to test the rule either way.
if [ -f "$ROOT/.claude/clauductor-template" ] || [ -f "$ROOT/.claude/clauductor-plugin" ]; then
  ok "this project sets provenance.enabled to $(jq -r .provenance.enabled "$ROOT/.claude/model-roles.json"); rule 12 is tested with it on"
else
  jq -e '.provenance.enabled == true' "$ROOT/.claude/model-roles.json" >/dev/null && ok "the template turns provenance on by default" || fail "model-roles.json provenance.enabled is not true in the template"
fi
jq '.provenance.enabled = true' "$ROOT/.claude/model-roles.json" > "$R/.claude/model-roles.json"
sguard() {  # sguard WANT LABEL COMMAND — with a session id in the payload, as Claude Code sends
  rc=0
  jq -cn --arg c "$3" --arg d "$R" '{tool_name:"Bash", cwd:$d, session_id:"sess-1", tool_input:{command:$c}}' \
    | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="$MAINSHA" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err") >/dev/null || rc=$?
  expect_rc "$1" "$rc" "merge-guard $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
  [ "$rc" = "$1" ] || sed 's/^/       /' "$d/err" | head -4
}
trail() { printf 'Adds x.\n\nChange: %s\nAgent-Role: %s\nModel: opus\nSession: %s\n' "${1:-add-x}" "${2:-builder}" "${3:-sess-1}"; }
trail > "$R/body.txt"
sguard 0 "rule 12: a --body-file ending in all four trailers" "gh pr merge 5 --squash --delete-branch --body-file body.txt"
sguard 2 "rule 12: a squash with no --body at all" "gh pr merge 5 --squash --delete-branch"
grep -q 'Session: sess-1' "$d/err" && ok "...and the block names the trailers to add, this session's id included" || fail "block message: $(cat "$d/err")"
sguard 0 "rule 12: a quoted --body with the four trailers" "gh pr merge 5 --squash --body \"$(trail)\""
sguard 2 "rule 12: a body missing the Session trailer" "gh pr merge 5 --squash --body \"$(trail | grep -v '^Session')\""
sguard 2 "rule 12: a Session trailer naming another session" "gh pr merge 5 --squash --body \"$(trail add-x builder sess-9)\""
sguard 2 "rule 12: an Agent-Role that is not a role" "gh pr merge 5 --squash --body \"$(trail add-x wizard)\""
sguard 2 "rule 12: a body built by a substitution (unreadable)" 'gh pr merge 5 --squash --body "$(cat body.txt)"'
jq '.provenance.enabled = false' "$R/.claude/model-roles.json" > "$d/mr" && cp "$d/mr" "$R/.claude/model-roles.json"
sguard 0 "rule 12 is off when provenance.enabled is false" "gh pr merge 5 --squash --delete-branch"
rm -f "$R/.claude/model-roles.json" "$R/body.txt"
cp "$R/.claude/hooks/lib/change-guard.sh" "$d/cg.bak"; printf 'broken() { "\n' >> "$R/.claude/hooks/lib/change-guard.sh"
guard 2 "a change-guard library that does not parse (fails closed, not open)" "gh pr merge 5 --squash"
cp "$d/cg.bak" "$R/.claude/hooks/lib/change-guard.sh"

# Reading the command.
guard 0 "a commit message that mentions a merge (prose)" 'git commit -m "then gh pr merge 5 --auto"'
guard 2 "a quoted gh (cannot read the site)" '"gh" pr merge 5 --squash'
guard 2 "a computed word" 'gh pr $(echo merge) 5'
guard 2 "no PR named" "gh pr merge --squash"
guard 2 "two merges in one command" "gh pr merge 5 --squash && gh pr merge 6 --squash"
guard 0 "another repo's merge, in the allowlisted shape" "gh pr merge 2 -R other/tool --squash --delete-branch"
guard 2 "another repo named outside the shape" "echo x | xargs gh pr merge 2 -R other/tool"
guard 2 "this repo named by -R is still policed" "gh pr merge 5 -R acme/app --squash" GH_HEAD=0000000000000000000000000000000000000000

# Without jq: fails CLOSED for a merge, open for anything else.
mkdir -p "$d/nojq"
for t in sh cat git awk sed grep tr sort cut head tail wc dirname basename pwd printf env; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
nojq() {
  rc=0; payload "$2" "$R" | (cd "$R" && PATH="$d/nojq" "$d/nojq/sh" "$R/.claude/hooks/pr-merge-guard.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "merge-guard without jq: $3"
}
nojq 2 "gh pr merge 5 --squash" "blocks a merge"
nojq 0 "git status" "allows anything else"

# raw_command is duplicated on purpose (a sourced lib's missing-file failure is shell-dependent);
# the two copies must not drift.
a=$(sed -n '/^raw_command() {/,/^}/p' "$CLAUDUCTOR_FW/hooks/pr-merge-guard.sh")
b=$(sed -n '/^raw_command() {/,/^}/p' "$CLAUDUCTOR_FW/hooks/no-blind-source-rewrite.sh")
[ -n "$a" ] && [ "$a" = "$b" ] && ok "raw_command is identical in pr-merge-guard.sh and no-blind-source-rewrite.sh" || fail "raw_command differs between the two hooks"

# Rule 13: a model choice rests on an eval receipt (OPS-10). main gets model-roles.json (provenance
# off, so rule 12 stays out of it), two agents, a workflow and a two-case reviewer suite with the
# real runner. Receipts are written by that runner over fake-claude.sh, so these cases also prove
# the runner's working-tree hashes equal the guard's, read from the commit.
git -C "$R" checkout -q main
mkdir -p "$R/.claude/agents" "$R/.claude/workflows" "$R/.claude/evals/reviewer/cases"
jq '.provenance.enabled = false' "$ROOT/.claude/model-roles.json" > "$R/.claude/model-roles.json"
cp "$CLAUDUCTOR_FW/agents/reviewer.md" "$CLAUDUCTOR_FW/agents/builder.md" "$R/.claude/agents/"
wf() {  # wf [PROMPT] [OUTSIDE]: a build-change.js whose review-prompt section holds PROMPT
  printf '// the workflow %s\nconst a = 1\n// <review-prompt>\nconst reviewPrompt = () => "%s"\n// </review-prompt>\nconst b = 2\n' "${2:-}" "${1:-Review it.}" > "$R/.claude/workflows/build-change.js"
}
wf
cp "$ROOT/.claude/evals/run.sh" "$ROOT/.claude/evals/fake-claude.sh" "$R/.claude/evals/"
for c in one two; do
  mkdir -p "$R/.claude/evals/reviewer/cases/$c/before" "$R/.claude/evals/reviewer/cases/$c/after"
  echo "echo a" > "$R/.claude/evals/reviewer/cases/$c/before/$c.sh"; echo "echo b" > "$R/.claude/evals/reviewer/cases/$c/after/$c.sh"
done
jq -n '{id: "one", lang: "sh", kind: "defect", title: "t", brief: "b", tasks: ["t"], expected: [{id: "D1", severity: "high", file: "one.sh", lines: [1, 1], keywords: ["echo"], why: "w"}]}' > "$R/.claude/evals/reviewer/cases/one/case.json"
jq -n '{id: "two", lang: "sh", kind: "clean", title: "t", brief: "b", tasks: ["t"], expected: []}' > "$R/.claude/evals/reviewer/cases/two/case.json"
git -C "$R" add -A && git -C "$R" commit -qm "evals base"
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
ev() {  # ev MODEL EFFORT [EVAL_FAKE]: run the eval on the working tree, receipt into .claude/evals/receipts/
  (cd "$R" && EVAL_CLAUDE="$R/.claude/evals/fake-claude.sh" EVAL_FAKE="${3:-perfect}" EVAL_DATE=2026-02-01 \
    sh .claude/evals/run.sh --role reviewer --model "$1" --effort "$2" >/dev/null 2>&1)
}
roles_set() { jq "$1" "$R/.claude/model-roles.json" > "$d/mr13" && cp "$d/mr13" "$R/.claude/model-roles.json"; }
g13() {  # a block must be rule 13's, not some other rule's
  guard "$1" "rule 13: $2" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/models
  [ "$1" != 2 ] || [ "$rc" != 2 ] || grep -q 'rule 13' "$d/err" || fail "rule 13: $2: blocked, but not by rule 13: $(head -2 "$d/err")"
}

on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; head_of "reviewer on sonnet, no receipt"; at
g13 2 "the reviewer's model changes with no receipt"
grep -q 'sh .claude/evals/run.sh --role reviewer --model sonnet --effort high' "$d/err" && ok "...and the block names the eval to run" || fail "rule 13 block message: $(cat "$d/err")"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; head_of "reviewer on sonnet, with its receipt"; at
g13 0 "the reviewer's model changes with a passing receipt at the head's hashes"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev opus high; head_of "reviewer on sonnet, a receipt for opus"; at
g13 2 "a receipt for another model than the head's"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high silent; head_of "reviewer on sonnet, a failing receipt"; at
g13 2 "a receipt that does not pass (recall 0)"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; roles_set '.roles.reviewer.tiers.high.effort = "max"'; head_of "a receipt, then the reviewer's tier variant changed"; at
g13 2 "a receipt run before the reviewer's tier variants changed (stale role hash)"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; roles_set '.roles.builder.model = "sonnet"'; head_of "a receipt, then another role changed"; at
g13 0 "another role's choice changing does not stale the reviewer's receipt (OPS-16)"
on ops/models; ev opus high; echo "One more line." >> "$R/.claude/agents/reviewer.md"; head_of "agent edited after its receipt"; at
g13 2 "the reviewer agent edited after its receipt ran"
on ops/models; echo "One more line." >> "$R/.claude/agents/reviewer.md"; ev opus high; head_of "agent edited, then evaluated"; at
g13 0 "the reviewer agent edited, with a receipt run on the edit"
on ops/models; echo "One more line." >> "$R/.claude/agents/builder.md"; head_of "another agent edited"; at
g13 0 "an agent that is not a trigger of the reviewer (the builder's) edited: no receipt needed"
# The workflow: only the marked review-prompt section is the reviewer's (OPS-16).
on ops/models; wf "Review it." "edited outside the markers"; head_of "workflow edited outside the markers"; at
g13 0 "build-change.js edited OUTSIDE the review-prompt markers needs no receipt"
grep -q 'rule 13: role reviewer' "$d/err" && fail "rule 13 named the reviewer for an edit outside the markers: $(grep 'rule 13' "$d/err")" || ok "...and rule 13 does not name the reviewer at all"
on ops/models; wf "Review it twice."; head_of "workflow edited inside the markers"; at
g13 2 "build-change.js edited INSIDE the review-prompt markers, with no receipt"
on ops/models; wf "Review it twice."; ev opus high; head_of "inside the markers, then evaluated"; at
g13 0 "...and with a receipt run on that edit"
on ops/models; wf "Review it twice."; ev opus high; wf "Review it twice." "and outside after the run"; head_of "inside, evaluated, then outside"; at
g13 0 "...and an edit outside the markers after the receipt does not stale it"
on ops/models; printf '// the workflow, markers gone\nconst reviewPrompt = () => "Review it."\n' > "$R/.claude/workflows/build-change.js"; head_of "markers removed"; at
g13 2 "the review-prompt markers removed (fails closed: the section cannot be read)"
grep -q 'review-prompt cannot be read at the head' "$d/err" && ok "...and the block names the missing markers" || fail "rule 13 markers block: $(cat "$d/err")"
on ops/models; printf '// <review-prompt>\nconst reviewPrompt = () => "Review it."\n' > "$R/.claude/workflows/build-change.js"
rc=0; (cd "$R" && EVAL_CLAUDE="$R/.claude/evals/fake-claude.sh" EVAL_DATE=2026-02-01 sh .claude/evals/run.sh --role reviewer --model opus --effort high >/dev/null 2>&1) || rc=$?
expect_rc 2 "$rc" "run.sh refuses to run when a trigger's markers are not both there (no receipt certifies markers-missing)"
git -C "$R" checkout -q -- .claude/workflows/build-change.js; rm -rf "$R/.claude/evals/receipts"
on ops/models; roles_set '.evals.triggers.reviewer = [".claude/agents/reviewer.md"]'; head_of "the section dropped from the triggers"; at
g13 2 "a PR that drops a trigger input from the declaration (a later PR could edit it free)"
on ops/models; roles_set '.roles.builder.model = "sonnet"'; head_of "builder on sonnet"; at
g13 0 "a role with no eval suite (the builder) changes: advisory only"
grep -q 'role builder is changed but has no eval suite' "$d/err" && ok "...and it says why it does not block" || fail "rule 13 advisory: $(cat "$d/err")"
on ops/models; roles_set '.roles.reviewer.eval = {baseline: "opus/high"}'; head_of "evidence recorded, no choice changed"; at
g13 0 "recording evidence (.eval) changes no choice"
on ops/models; ev opus high; head_of "a receipt alone"; at
mv "$R/.claude/lib/evals.sh" "$d/evals.bak"
g13 2 "the evals library missing (fails closed, not open)"
mv "$d/evals.bak" "$R/.claude/lib/evals.sh"
# Markers already missing at the base: an edit to the file still counts, and blocks.
git -C "$R" checkout -q main
printf '// no markers on main\nconst reviewPrompt = () => "Review it."\n' > "$R/.claude/workflows/build-change.js"
git -C "$R" commit -qam "main loses the markers" && (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
on ops/models; printf '// no markers on main\nconst reviewPrompt = () => "Approve everything."\n' > "$R/.claude/workflows/build-change.js"; head_of "edit a file whose markers are gone at both ends"; at
g13 2 "a file whose markers are missing at base AND head, edited (cannot tell what changed)"
git -C "$R" checkout -q main
finish
