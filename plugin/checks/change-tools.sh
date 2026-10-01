#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The scripts a change passes through, each falsified in a throwaway repo:
#   change-approval.sh  records an approval over the design's hash; a later edit to design.md or the
#                       Risk line voids it; --revoke puts it back to awaiting.
#   verify-change.sh    passes a finished change, and fails on an open task, an uncited scenario, a
#                       voided approval, and a ticked task whose named paths the diff never touched.
#   change-cost.sh      sums a branch's messages from transcripts (subagents included), once per
#                       message id, at .prices; ignores another branch and another repo; says OVER
#                       BUDGET past the proposal's budget; names a model it cannot price; and says
#                       CANNOT CHECK, never $0, when no transcript ran on the branch.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
EX="$CLAUDUCTOR_FW/examples"
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/changes" "$R/specs" "$R/docs"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/usage.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/change-approval.sh" "$CLAUDUCTOR_FW/verify-change.sh" "$CLAUDUCTOR_FW/scenario-trace.sh" \
   "$CLAUDUCTOR_FW/change-cost.sh" "$ROOT/.claude/model-roles.json" "$R/.claude/"
cp -R "$EX/specs/greeting" "$R/specs/"
git -C "$R" add -A && git -C "$R" commit -qm base
git -C "$R" checkout -q -b change/add-greeting-name
cp -R "$EX/changes/add-greeting-name" "$R/changes/"
C="$R/changes/add-greeting-name"

# ── change-approval.sh ─────────────────────────────────────────────────────────────────────────
A() { (cd "$R" && APPROVAL_DATE=2026-01-02 sh .claude/change-approval.sh add-greeting-name "$@") > "$d/out" 2>&1; }
A; expect_rc 1 $? "change-approval: an unapproved change is not approved"
A --record Ana; expect_rc 0 $? "change-approval --record writes the Approved line"
grep -qE '^\*\*Approved:\*\* 2026-01-02 by Ana · design [0-9a-f]{12}$' "$C/proposal.md" && ok "the line reads '**Approved:** <date> by <owner> · design <hash>'" || fail "Approved line: $(grep Approved "$C/proposal.md")"
grep -q '^\*\*Status:' "$C/proposal.md" && fail "--record left the Status line" || ok "--record replaced the Status line"
A; expect_rc 0 $? "change-approval: the approval covers the design as written"
cp "$C/design.md" "$d/design.bak"; echo "- **D2** a late idea" >> "$C/design.md"
A; expect_rc 1 $? "change-approval: an edit to design.md after approval voids it"
grep -q CHANGED "$d/out" && ok "...and says the design CHANGED since approval" || fail "no CHANGED in: $(cat "$d/out")"
cp "$d/design.bak" "$C/design.md"
sed 's/^\*\*Risk:\*\* low/**Risk:** high/' "$C/proposal.md" > "$d/p" && cp "$d/p" "$C/proposal.md"
A; expect_rc 1 $? "change-approval: a changed Risk tier voids it"
sed 's/^\*\*Risk:\*\* high/**Risk:** low/' "$C/proposal.md" > "$d/p" && cp "$d/p" "$C/proposal.md"
A; expect_rc 0 $? "change-approval: restoring the design restores the approval"
A --revoke; expect_rc 0 $? "change-approval --revoke"
grep -qx '\*\*Status:\*\* awaiting approval' "$C/proposal.md" && ok "--revoke puts '**Status:** awaiting approval' back" || fail "--revoke: $(head -2 "$C/proposal.md")"
A --record Ana >/dev/null

# ── verify-change.sh ───────────────────────────────────────────────────────────────────────────
# Build the example change for real: the files its tasks name, a test citing its scenarios.
mkdir -p "$R/src/home" "$R/src/auth"
for f in src/home/greeting.ts src/home/greeting.test.ts src/auth/sign-out.ts src/auth/sign-out.test.ts; do echo "// $f" > "$R/$f"; done
printf '// GREETING-1-S1 GREETING-1-S2\n' >> "$R/src/home/greeting.test.ts"
printf '// FAREWELL-1-S1\n' >> "$R/src/auth/sign-out.test.ts"
sed 's/- \[ \]/- [x]/' "$C/tasks.md" > "$d/t" && cp "$d/t" "$C/tasks.md"
git -C "$R" add -A && git -C "$R" commit -qm "add-greeting-name: built"
V() { (cd "$R" && sh .claude/verify-change.sh add-greeting-name --base main) > "$d/out" 2>&1; }
V; rc=$?; expect_rc 0 "$rc" "verify-change passes a finished change"
[ "$rc" = 0 ] || sed 's/^/       /' "$d/out"
grep -q '^verify-change: add-greeting-name verified against main at [0-9a-f]' "$d/out" && ok "its verdict names the change, the base and the commit" || fail "verdict line: $(tail -1 "$d/out")"

sed 's/- \[x\] 1.2/- [ ] 1.2/' "$C/tasks.md" > "$d/t" && cp "$d/t" "$C/tasks.md"
V; expect_rc 1 $? "verify-change fails on an open task"
sed 's/- \[ \] 1.2/- [x] 1.2/' "$C/tasks.md" > "$d/t" && cp "$d/t" "$C/tasks.md"

sed 's/ FAREWELL-1-S1//; s|// FAREWELL-1-S1||' "$R/src/auth/sign-out.test.ts" > "$d/t" && cp "$d/t" "$R/src/auth/sign-out.test.ts"
git -C "$R" commit -qam "drop a citation"
V; expect_rc 1 $? "verify-change fails when an added scenario is cited by no test"
grep -q 'MISSING  FAREWELL-1-S1' "$d/out" && ok "...and names FAREWELL-1-S1" || fail "no MISSING line: $(cat "$d/out")"
printf '// FAREWELL-1-S1\n' >> "$R/src/auth/sign-out.test.ts"; git -C "$R" commit -qam "cite it again"

git -C "$R" rm -q src/auth/sign-out.ts src/auth/sign-out.test.ts
git -C "$R" commit -qm "lose group 2's files"
printf '// FAREWELL-1-S1\n' > "$R/src/home/farewell.test.ts"; git -C "$R" add -A; git -C "$R" commit -qm "cite elsewhere"
V; expect_rc 1 $? "verify-change fails when a ticked task's named paths are not in the diff"
grep -q '^FAIL task claims src/auth/sign-out.ts' "$d/out" && ok "...and names the task and its paths" || fail "no 'task claims' line: $(cat "$d/out")"
git -C "$R" revert -q --no-edit HEAD~1 >/dev/null 2>&1 || true

echo "- **D3** edited after approval" >> "$C/design.md"; git -C "$R" commit -qam "edit design"
V; expect_rc 1 $? "verify-change fails when the design changed after approval"
# CHANGES_LEGACY: a grandfathered change keeps the format it was approved in, so its approval line
# and its scenarios are not judged; its tasks still are.
cp "$CLAUDUCTOR_FW/lib/records.sh" "$R/.claude/lib/"
[ -f "$R/.claude/project.conf" ] && cp "$R/.claude/project.conf" "$d/pc.bak" || : > "$d/pc.bak"
{ cat "$d/pc.bak"; echo 'CHANGES_LEGACY="add-greeting-name"'; } > "$R/.claude/project.conf"
# (group 2's files back, as the cases above may have left them; no test cites FAREWELL-1-S1)
mkdir -p "$R/src/auth"; echo "// src/auth/sign-out.ts" > "$R/src/auth/sign-out.ts"; echo "// src/auth/sign-out.test.ts" > "$R/src/auth/sign-out.test.ts"
for f in src/auth/sign-out.test.ts src/home/farewell.test.ts; do
  [ -f "$R/$f" ] && sed 's/FAREWELL-1-S1//g' "$R/$f" > "$d/t" && cp "$d/t" "$R/$f"
done
git -C "$R" add -A; git -C "$R" commit -qm "legacy: voided approval, an uncited scenario"
V; rc=$?; expect_rc 0 "$rc" "verify-change passes a grandfathered change whose approval and scenarios predate the format"
[ "$rc" = 0 ] || sed 's/^/       /' "$d/out"
grep -q 'approval: grandfathered' "$d/out" && grep -q 'scenarios: grandfathered' "$d/out" && ok "...and says why it did not judge them" || fail "legacy verify output: $(cat "$d/out")"
sed 's/- \[x\] 1.2/- [ ] 1.2/' "$C/tasks.md" > "$d/t" && cp "$d/t" "$C/tasks.md"
V; expect_rc 1 $? "verify-change still fails a grandfathered change with an open task"
sed 's/- \[ \] 1.2/- [x] 1.2/' "$C/tasks.md" > "$d/t" && cp "$d/t" "$C/tasks.md"
cp "$d/pc.bak" "$R/.claude/project.conf"

# ── change-cost.sh ─────────────────────────────────────────────────────────────────────────────
P="$d/projects"
main=$(cd "$R" && pwd -P)
slug=$(printf '%s' "$main" | sed 's/[^A-Za-z0-9]/-/g')
mkdir -p "$P/$slug/s1/subagents" "$P/$slug--claude-worktrees-lane" "$P/-elsewhere"
msg() {  # msg ID MODEL BRANCH CWD IN OUT
  printf '{"type":"assistant","gitBranch":"%s","cwd":"%s","message":{"id":"%s","model":"%s","usage":{"input_tokens":%s,"output_tokens":%s,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}\n' "$3" "$4" "$1" "$2" "$5" "$6"
}
B=change/add-greeting-name
{ msg m1 claude-opus-5-5 "$B" "$main" 1000000 0        # $4.00
  msg m1 claude-opus-5-5 "$B" "$main" 1000000 0        # the same message streamed again: once
  msg m2 claude-sonnet-5-5 "$B" "$main/.claude/worktrees/lane" 0 1000000; } > "$P/$slug/s1.jsonl"   # $10.00
msg m3 claude-haiku-4-5-20251001 "$B" "$main" 1000000 0 > "$P/$slug/s1/subagents/agent-a.jsonl"      # $1.00
msg m4 claude-opus-5-5 "change/other" "$main" 1000000 0 > "$P/$slug--claude-worktrees-lane/s2.jsonl"  # another branch
msg m5 claude-opus-5-5 "$B" "/somewhere/else" 1000000 0 > "$P/-elsewhere/s3.jsonl"                     # another repo
# The same two inside a file that DOES hold the branch, so the per-message filters are what excludes them.
{ msg m8 claude-opus-5-5 "change/other" "$main" 1000000 0; msg m9 claude-opus-5-5 "$B" "${main}-fork" 1000000 0; } >> "$P/$slug/s1.jsonl"
cost() { (cd "$R" && CLAUDE_PROJECTS_DIR="$P" sh .claude/change-cost.sh add-greeting-name "$@") > "$d/out" 2>&1; }
cost --json; rc=$?
expect_rc 0 "$rc" "change-cost reads the branch's transcripts"
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 15 ] && ok "change-cost: \$15 = opus 1M in + sonnet 1M out + a subagent's haiku 1M in; a repeated message once; another branch and another repo left out" || fail "change-cost: $(cat "$d/out")"
[ "$(jq -r .over "$d/out" 2>/dev/null)" = false ] && ok "change-cost: \$15 of a \$15 budget is not over" || fail "over: $(cat "$d/out")"
msg m6 claude-opus-5-5 "$B" "$main" 250000 0 >> "$P/$slug/s1.jsonl"
cost; case "$(cat "$d/out")" in *"cost \$16 of budget \$15 — OVER BUDGET"*) ok "change-cost: past the budget it says OVER BUDGET" ;; *) fail "change-cost over budget: $(cat "$d/out")" ;; esac
msg m7 mystery-model-1 "$B" "$main" 1000 0 >> "$P/$slug/s1.jsonl"
cost; case "$(cat "$d/out")" in *"NOT priced"*mystery-model-1*) ok "change-cost names a model .prices cannot price" ;; *) fail "unpriced model not named: $(cat "$d/out")" ;; esac
rm -rf "$P/$slug" "$P/$slug--claude-worktrees-lane"
cost; rc=$?
expect_rc 2 "$rc" "change-cost with no transcript on the branch exits 2"
case "$(cat "$d/out")" in "CANNOT CHECK"*) ok "...and says CANNOT CHECK, not \$0" ;; *) fail "no-transcript output: $(cat "$d/out")" ;; esac

# CHANGE_RECORD_EXTRA (P1.7): the project's own required sections, as checks/changes.sh and rule 9
# read them (lib/change.sh change_extra_missing).
X="$d/extra"; mkdir -p "$X"
printf '# P\n\n## Rollback\nRevert it.\n' > "$X/proposal.md"; printf '# D\n' > "$X/design.md"
out=$(CHANGE_RECORD_EXTRA="" sh -c '. "$1"; change_extra_missing "$2"' _ "$CLAUDUCTOR_FW/lib/change.sh" "$X")
[ -z "$out" ] && ok "CHANGE_RECORD_EXTRA empty: nothing extra is required" || fail "empty CHANGE_RECORD_EXTRA: $out"
out=$(CHANGE_RECORD_EXTRA="proposal.md:Rollback" sh -c '. "$1"; change_extra_missing "$2"' _ "$CLAUDUCTOR_FW/lib/change.sh" "$X")
[ -z "$out" ] && ok "CHANGE_RECORD_EXTRA: a record carrying the section passes" || fail "present section: $out"
out=$(CHANGE_RECORD_EXTRA="proposal.md:Rollback; design.md:Security review (STRIDE); notes.md:Owner" sh -c '. "$1"; change_extra_missing "$2"' _ "$CLAUDUCTOR_FW/lib/change.sh" "$X")
case "$out" in *"design.md has no '## Security review (STRIDE)' section"*"notes.md is missing"*) ok "CHANGE_RECORD_EXTRA: a missing section and a missing file are each named" ;; *) fail "missing sections: $out" ;; esac
finish
