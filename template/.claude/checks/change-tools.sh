#!/bin/sh
# The scripts a change passes through, each falsified in a throwaway repo:
#   change-approval.sh  records an approval over the design's hash; a later edit to design.md or the
#                       Risk line voids it; --revoke puts it back to awaiting.
#   verify-change.sh    passes a finished change, and fails on an open task, an uncited scenario, a
#                       voided approval, and a ticked task whose named paths the diff never touched.
#   change-cost.sh      sums a branch's messages from transcripts (subagents included), once per
#                       message id, at .prices; ignores another branch and another repo; says OVER
#                       BUDGET past the proposal's budget; reports the budget's kind (hard, soft,
#                       unreadable); names a model it cannot price; and says CANNOT CHECK, never $0,
#                       when no transcript ran on the branch, still reporting the budget.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
EX="$ROOT/.claude/examples"
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/changes" "$R/specs" "$R/docs"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$ROOT/.claude/lib/usage.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/change-approval.sh" "$ROOT/.claude/verify-change.sh" "$ROOT/.claude/scenario-trace.sh" \
   "$ROOT/.claude/change-cost.sh" "$ROOT/.claude/model-roles.json" "$R/.claude/"
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
cp "$ROOT/.claude/lib/records.sh" "$R/.claude/lib/"
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
msg() {  # msg ID MODEL BRANCH CWD IN OUT [SESSION TIMESTAMP]: without a session, one per file
  st=""; [ -z "${7:-}" ] || st=$(printf '"sessionId":"%s","timestamp":"%s",' "$7" "$8")
  printf '{"type":"assistant",%s"gitBranch":"%s","cwd":"%s","message":{"id":"%s","model":"%s","usage":{"input_tokens":%s,"output_tokens":%s,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}\n' "$st" "$3" "$4" "$1" "$2" "$5" "$6"
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
# The budget's kind (OPS-85), which build-change acts on: the Budget line's own words, read by
# lib/change.sh proposal_budget. Unmarked is hard; `, soft` is soft; anything else is UNREADABLE.
budgetline() { sed "s/^\*\*Budget:\*\* .*/**Budget:** $1/" "$C/proposal.md" > "$d/p" && cp "$d/p" "$C/proposal.md"; }
cost --json; [ "$(jq -c '[.budgetUsd, .budgetKind, .over]' "$d/out" 2>/dev/null)" = '[15,"hard",true]' ] && ok "change-cost --json: an unmarked '\$15' is a hard budget (budgetKind hard)" || fail "change-cost, unmarked budget: $(cat "$d/out")"
budgetline '$15, soft'
cost --json; [ "$(jq -c '[.budgetUsd, .budgetKind, .over]' "$d/out" 2>/dev/null)" = '[15,"soft",true]' ] && ok "change-cost --json: '\$15, soft' is budgetKind soft, and over it is still over" || fail "change-cost, soft budget: $(cat "$d/out")"
cost; case "$(cat "$d/out")" in *"cost \$16 of soft budget \$15 — OVER BUDGET (soft: report it to the owner; the build goes on)"*) ok "change-cost: past a soft budget it says so, and that the build goes on" ;; *) fail "change-cost over a soft budget: $(cat "$d/out")" ;; esac
budgetline 'fifteen dollars, soft'
cost --json; [ "$(jq -c '[.budgetUsd, .budgetKind, .over]' "$d/out" 2>/dev/null)" = '[null,"unreadable",false]' ] && ok "change-cost --json: a Budget line it cannot read is budgetKind unreadable, neither hard nor soft" || fail "change-cost, unreadable budget: $(cat "$d/out")"
cost; case "$(cat "$d/out")" in *"budget UNREADABLE"*) ok "change-cost: ...and its one line says the budget is UNREADABLE" ;; *) fail "change-cost text, unreadable budget: $(cat "$d/out")" ;; esac
sed '/^\*\*Budget:\*\*/d' "$C/proposal.md" > "$d/p" && cp "$d/p" "$C/proposal.md"
cost --json; [ "$(jq -c '[.budgetUsd, .budgetKind, .over]' "$d/out" 2>/dev/null)" = '[null,"none",false]' ] && ok "change-cost --json: no Budget line is budgetKind none, with no figure" || fail "change-cost, no budget: $(cat "$d/out")"
cost; case "$(cat "$d/out")" in *"(no budget)"*) ok "change-cost: ...and its one line says (no budget)" ;; *) fail "change-cost text, no budget: $(cat "$d/out")" ;; esac
sed 's/^\*\*Risk:\*\*.*/&\
**Budget:** $15/' "$C/proposal.md" > "$d/p" && cp "$d/p" "$C/proposal.md"
grep -qx '\*\*Budget:\*\* \$15' "$C/proposal.md" || fail "change-tools: could not restore the fixture's '**Budget:** \$15'"
msg m7 mystery-model-1 "$B" "$main" 1000 0 >> "$P/$slug/s1.jsonl"
cost; case "$(cat "$d/out")" in *"NOT priced"*mystery-model-1*) ok "change-cost names a model .prices cannot price" ;; *) fail "unpriced model not named: $(cat "$d/out")" ;; esac
# OPS-74: an agent records its LEAD session's branch, often a stale `main`, never its own. Its cwd is
# what names the change: the lane worktree (m2 above recorded the branch there). A builder that
# started in the main checkout and cd'd into the worktree counts whole; one that never entered it,
# or entered a worktree whose name merely begins with the lane's, does not.
W="$P/$slug/s1/subagents/workflows/wf_1"; mkdir -p "$W"
{ msg b1 claude-opus-5-5 main "$main/.claude/worktrees/lane" 1000000 0      # $4.00
  msg b2 claude-opus-5-5 main "$main" 1000000 0; } > "$W/agent-b.jsonl"   # $4.00: the same agent, before its cd
msg b3 claude-haiku-4-5-20251001 main "$main/.claude/worktrees/lane/sub" 1000000 0 >> "$P/$slug/s1.jsonl"  # $1.00: the lead, in the lane
msg c1 claude-opus-5-5 main "$main" 1000000 0 > "$W/agent-c.jsonl"                                       # never in the lane
msg c2 claude-opus-5-5 main "$main/.claude/worktrees/lane-2" 1000000 0 > "$W/agent-d.jsonl"               # another lane
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 25 ] && ok "change-cost: an agent recording main counts by its cwd in the change's worktree, whole; the main checkout and another lane do not" || fail "change-cost, agents recording main: $(cat "$d/out")"
# A LEAD records a stale branch too: a session closing in lane `other` recorded $B there, then its
# own ops/other. A lane whose sessions all recorded another branch LAST there is not the change's:
# neither its messages on $B, nor its other branch, nor an agent that ran in it counts.
{ msg x1 claude-opus-5-5 "$B" "$main/.claude/worktrees/other" 1000000 0
  msg x2 claude-opus-5-5 "ops/other" "$main/.claude/worktrees/other/framework" 1000000 0; } >> "$P/$slug/s1.jsonl"
msg x3 claude-opus-5-5 main "$main/.claude/worktrees/other" 1000000 0 > "$W/agent-x.jsonl"
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 25 ] && ok "change-cost: a lane where a session recorded another branch last is not the change's, though a stale lead recorded the branch there" || fail "change-cost, stale lead in another lane: $(cat "$d/out")"
[ "$(jq -c .rejectedLanes "$d/out" 2>/dev/null)" = "[\"$main/.claude/worktrees/other\"]" ] && ok "change-cost --json names the lane it rejected" || fail "change-cost rejectedLanes: $(cat "$d/out")"
# The other way round: a lead hopping in from another lane carries that lane's branch into its first
# message here, then records $B. The lane is the change's, the hop's record and its agents included.
{ msg h1 claude-haiku-4-5-20251001 "ops/previous" "$main/.claude/worktrees/hop" 1000000 0      # $1.00
  msg h2 claude-haiku-4-5-20251001 "$B" "$main/.claude/worktrees/hop" 1000000 0; } > "$P/$slug/s4.jsonl"  # $1.00
msg h3 claude-haiku-4-5-20251001 main "$main/.claude/worktrees/hop" 1000000 0 > "$W/agent-h.jsonl"     # $1.00
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 28 ] && ok "change-cost: a lane a lead hopped into, carrying its previous branch for one message, is still the change's" || fail "change-cost, hop-in: $(cat "$d/out")"
# The lane is the directory under .claude/worktrees/, not the cwd that recorded the branch: a
# recording in lane `deep`'s subdirectory makes `deep` the root, so an agent at `deep` itself counts.
msg y1 claude-haiku-4-5-20251001 "$B" "$main/.claude/worktrees/deep/framework" 1000000 0 >> "$P/$slug/s1.jsonl"  # $1.00
msg y2 claude-haiku-4-5-20251001 main "$main/.claude/worktrees/deep" 1000000 0 > "$W/agent-y.jsonl"              # $1.00
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 30 ] && ok "change-cost: a branch recorded in a lane's subdirectory makes the whole lane the change's" || fail "change-cost, lane subdirectory: $(cat "$d/out")"
# A detached HEAD, or no branch at all, recorded last in the lane is not another branch.
{ msg z1 claude-haiku-4-5-20251001 HEAD "$main/.claude/worktrees/deep" 1000000 0                  # $1.00
  msg z2 claude-haiku-4-5-20251001 "" "$main/.claude/worktrees/deep" 1000000 0; } >> "$P/$slug/s1.jsonl"  # $1.00
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 32 ] && ok "change-cost: a detached HEAD or an empty branch recorded in the lane does not reject it" || fail "change-cost, HEAD and empty: $(cat "$d/out")"
# A worktree on the branch that no transcript has recorded yet: git names it while it is live, and
# keeps naming it though a session recorded another branch there last.
git -C "$R" worktree add -q -f "$main/.claude/worktrees/live" "$B" 2>/dev/null
msg e1 claude-haiku-4-5-20251001 main "$main/.claude/worktrees/live" 1000000 0 > "$W/agent-e.jsonl"       # $1.00
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 33 ] && ok "change-cost: an agent in a live worktree on the branch counts though no message recorded the branch there" || fail "change-cost, live worktree: $(cat "$d/out")"
{ msg e2 claude-haiku-4-5-20251001 "$B" "$main/.claude/worktrees/live" 1000000 0                 # $1.00
  msg e3 claude-haiku-4-5-20251001 "ops/foreign" "$main/.claude/worktrees/live" 1000000 0; } > "$P/$slug/s5.jsonl"  # $1.00
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 35 ] && [ "$(jq -c .rejectedLanes "$d/out" 2>/dev/null)" = "[\"$main/.claude/worktrees/other\"]" ] && ok "change-cost: a live worktree on the branch counts, and is not called rejected, though a session recorded another branch there last" || fail "change-cost, live worktree with a foreign branch: $(cat "$d/out")"
git -C "$R" worktree remove -f "$main/.claude/worktrees/live" 2>/dev/null
# Votes by session (sessionId) and in time order, from the LEAD's records only. An agent records its
# lead's CURRENT branch: the lead left lane v1 for v1-next and ops/next, its agent still in v1 then
# recorded ops/next there. v1 stays the change's, and the agent's $1 counts.
V="$main/.claude/worktrees/v"; T=2026-10-13T10:00
{ msg v1 claude-haiku-4-5-20251001 "$B" "${V}1" 0 0 L1 "${T}:01Z"
  msg v2 claude-haiku-4-5-20251001 "ops/next" "${V}1-next" 0 0 L1 "${T}:02Z"; } > "$P/$slug/L1.jsonl"
mkdir -p "$P/$slug/L1/subagents"
msg v3 claude-haiku-4-5-20251001 "ops/next" "${V}1" 1000000 0 L1 "${T}:03Z" > "$P/$slug/L1/subagents/agent-v.jsonl"  # $1.00
# One session over two transcript files (a resume) ended on ops/two in v2: v2 is rejected.
msg v4 claude-haiku-4-5-20251001 "$B" "${V}2" 0 0 L2 "${T}:01Z" > "$P/$slug/L2a.jsonl"
msg v5 claude-haiku-4-5-20251001 "ops/two" "${V}2" 0 0 L2 "${T}:02Z" > "$P/$slug/L2b.jsonl"
# Two sessions in v3: one ended on the branch, one on ops/three. One vote for the branch takes it.
{ msg v6 claude-haiku-4-5-20251001 "ops/three" "${V}3" 0 0 L3 "${T}:01Z"
  msg v7 claude-haiku-4-5-20251001 "$B" "${V}3" 0 0 L3 "${T}:02Z"; } > "$P/$slug/L3.jsonl"
{ msg v8 claude-haiku-4-5-20251001 "$B" "${V}3" 0 0 L3b "${T}:03Z"
  msg v9 claude-haiku-4-5-20251001 "ops/three" "${V}3" 0 0 L3b "${T}:04Z"; } > "$P/$slug/L3b.jsonl"
# Time order, not file order: written last but recorded first, the branch is not v4's last vote.
{ msg va claude-haiku-4-5-20251001 "ops/four" "${V}4" 0 0 L4 "${T}:05Z"
  msg vb claude-haiku-4-5-20251001 "$B" "${V}4" 0 0 L4 "${T}:01Z"; } > "$P/$slug/L4.jsonl"
cost --json
[ "$(jq -r .costUsd "$d/out" 2>/dev/null)" = 33 ] && ok "change-cost: an agent left in the lane after its lead moved on does not take the lane from the change" || fail "change-cost, agent left behind: $(cat "$d/out")"
# `live`, now removed, is rejected as well: its one session ended there on ops/foreign (a known limit).
[ "$(jq -c .rejectedLanes "$d/out" 2>/dev/null)" = "[\"$main/.claude/worktrees/live\",\"$main/.claude/worktrees/other\",\"${V}2\",\"${V}4\"]" ] && ok "change-cost: a lane is voted by session across its files and in time order; one session's vote for the branch takes it" || fail "change-cost, session votes: $(cat "$d/out")"
cost; case "$(cat "$d/out")" in *"; rejected lanes: live, other, v2, v4 (another branch was recorded there last)"*) ok "change-cost names the rejected lanes in its one line" ;; *) fail "change-cost text, rejected lanes: $(cat "$d/out")" ;; esac
rm -rf "$P/$slug" "$P/$slug--claude-worktrees-lane"
cost; rc=$?
expect_rc 2 "$rc" "change-cost with no transcript on the branch exits 2"
case "$(cat "$d/out")" in "CANNOT CHECK"*) ok "...and says CANNOT CHECK, not \$0" ;; *) fail "no-transcript output: $(cat "$d/out")" ;; esac
cost --json
jq -e '.costUsd == null and .budgetUsd == 15 and .budgetKind == "hard" and (.cannot | length > 0)' "$d/out" >/dev/null 2>&1 \
  && ok "...and its --json still reports the budget and its kind, so build-change knows what it cannot enforce" || fail "change-cost --json, CANNOT CHECK: $(cat "$d/out")"

# CHANGE_RECORD_EXTRA (P1.7): the project's own required sections, as checks/changes.sh and rule 9
# read them (lib/change.sh change_extra_missing).
X="$d/extra"; mkdir -p "$X"
printf '# P\n\n## Rollback\nRevert it.\n' > "$X/proposal.md"; printf '# D\n' > "$X/design.md"
out=$(CHANGE_RECORD_EXTRA="" sh -c '. "$1"; change_extra_missing "$2"' _ "$ROOT/.claude/lib/change.sh" "$X")
[ -z "$out" ] && ok "CHANGE_RECORD_EXTRA empty: nothing extra is required" || fail "empty CHANGE_RECORD_EXTRA: $out"
out=$(CHANGE_RECORD_EXTRA="proposal.md:Rollback" sh -c '. "$1"; change_extra_missing "$2"' _ "$ROOT/.claude/lib/change.sh" "$X")
[ -z "$out" ] && ok "CHANGE_RECORD_EXTRA: a record carrying the section passes" || fail "present section: $out"
out=$(CHANGE_RECORD_EXTRA="proposal.md:Rollback; design.md:Security review (STRIDE); notes.md:Owner" sh -c '. "$1"; change_extra_missing "$2"' _ "$ROOT/.claude/lib/change.sh" "$X")
case "$out" in *"design.md has no '## Security review (STRIDE)' section"*"notes.md is missing"*) ok "CHANGE_RECORD_EXTRA: a missing section and a missing file are each named" ;; *) fail "missing sections: $out" ;; esac
finish
