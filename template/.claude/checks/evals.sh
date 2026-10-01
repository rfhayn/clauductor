#!/bin/sh
# The seeded-defect eval suite and its runner (OPS-10), with a FAKE reviewer: fake-claude.sh
# answers in place of `claude -p`, so no case spends a token and CI needs no Claude CLI.
#   1. every case of every suite is well formed: its planted defects point at real lines of real
#      files, its trees differ, and the reviewer suite covers Go, TypeScript, Python and shell with
#      clean controls among them;
#   2. the runner's scoring arithmetic, on canned findings whose right answers are worked out here;
#   3. the receipt's format and the hashes it names;
#   4. checks/model-roles.sh refuses a changed model without a matching passing receipt;
#   5. the marked sections a trigger names, and the trigger declaration's default (OPS-16);
#   6. the hash functions run.sh duplicates from lib/evals.sh have not drifted.
# pr-merge-guard rule 13 is exercised in checks/merge-guard.sh.
. "$(dirname "$0")/lib.sh"
need git jq

ev="$ROOT/.claude/evals"
[ -f "$ev/run.sh" ] || { fail "no eval runner at $ev/run.sh"; finish; }

# ── 1. The suites ──────────────────────────────────────────────────────────────────────────────
lines_of() { wc -l < "$1" | tr -d ' '; }
for s in "$ev"/*/cases; do
  [ -d "$s" ] || continue
  role=$(basename "$(dirname "$s")")
  nc=0; bad=0
  for c in "$s"/*/; do
    c=${c%/}; id=$(basename "$c"); nc=$((nc + 1))
    k="$c/case.json"
    if ! jq -e . "$k" >/dev/null 2>&1; then fail "$role/$id: case.json is missing or not JSON"; bad=1; continue; fi
    why=$(jq -r --arg id "$id" '
      [ (if .id != $id then "id is \(.id), not its directory name" else empty end),
        (if (.lang as $l | ["go", "ts", "py", "sh"] | index($l)) == null then "lang \(.lang) is not go, ts, py or sh" else empty end),
        (if (.kind as $k | ["defect", "clean"] | index($k)) == null then "kind \(.kind) is not defect or clean" else empty end),
        (if ((.title // "") | length) == 0 or ((.brief // "") | length) == 0 then "no title or brief" else empty end),
        (if (.tasks | type) != "array" or (.tasks | length) == 0 then "no tasks" else empty end),
        (if .kind == "defect" and ((.expected // []) | length) == 0 then "a defect case plants nothing" else empty end),
        (if .kind == "clean" and ((.expected // []) | length) > 0 then "a clean case plants a defect" else empty end),
        (if ([(.expected // [])[].id] | length) != ([(.expected // [])[].id] | unique | length) then "two defects share an id" else empty end),
        ((.expected // [])[] | select(
            (.severity as $v | ["critical", "high", "medium", "low"] | index($v)) == null
            or (.file | type) != "string"
            or (.lines | type) != "array" or (.lines | length) != 2 or .lines[0] < 1 or .lines[0] > .lines[1]
            or (.keywords | type) != "array" or (.keywords | length) == 0
            or ((.why // "") | length) == 0)
          | "defect \(.id // "?") needs a severity, a file, lines [lo, hi], keywords and a why")
      ] | .[]' "$k" 2>&1) || why="its fields could not be checked: $why"
    if [ -n "$why" ]; then fail "$role/$id: $(printf '%s' "$why" | tr '\n' ';')"; bad=1; continue; fi
    for p in $(jq -r '(.delete // [])[]' "$k"); do
      [ -f "$c/before/$p" ] || { fail "$role/$id deletes $p, which its before/ tree does not have"; bad=1; }
    done
    for row in $(jq -r '.expected[] | "\(.id)|\(.file)|\(.lines[1])"' "$k"); do
      did=${row%%|*}; rest=${row#*|}; file=${rest%%|*}; hi=${rest#*|}
      if [ -f "$c/after/$file" ]; then src="$c/after/$file"
      elif jq -e --arg f "$file" '(.delete // []) | index($f)' "$k" >/dev/null && [ -f "$c/before/$file" ]; then src="$c/before/$file"
      else fail "$role/$id $did names $file, which is neither in after/ nor a deleted file of before/"; bad=1; continue; fi
      [ "$hi" -le "$(lines_of "$src")" ] || { fail "$role/$id $did: line $hi is past the end of $file ($(lines_of "$src") lines)"; bad=1; }
    done
    if [ -d "$c/before" ] && diff -rq "$c/before" "$c/after" >/dev/null 2>&1 && [ "$(jq '(.delete // []) | length' "$k")" = 0 ]; then
      fail "$role/$id: before/ and after/ are identical, so there is no diff to review"; bad=1
    fi
  done
  [ "$bad" = 0 ] && ok "$role suite: all $nc cases well formed (defects at real lines of real files, a diff in each)"
done
s="$ev/reviewer/cases"
if [ -d "$s" ]; then
  n=$(ls "$s"/*/case.json | wc -l | tr -d ' ')
  clean=$(cat "$s"/*/case.json | jq -s '[.[] | select(.kind == "clean")] | length')
  langs=$(cat "$s"/*/case.json | jq -rs '[.[].lang] | unique | join(" ")')
  [ "$n" -ge 12 ] && [ "$n" -le 20 ] && ok "reviewer suite has $n cases (12 to 20)" || fail "reviewer suite has $n cases, want 12 to 20"
  [ "$clean" -ge 3 ] && ok "reviewer suite has $clean clean controls (false positives are measured)" || fail "reviewer suite has $clean clean cases, want at least 3"
  [ "$langs" = "go py sh ts" ] && ok "reviewer suite covers Go, Python, shell and TypeScript" || fail "reviewer suite languages are '$langs', want go py sh ts"
  for want in "wrong money" "cross-tenant access" "data loss" "a scenario with no test" "a test that passes without the code" "silent control failure" "portability"; do
    cat "$s"/*/case.json | jq -e --arg w "$want" -s 'any(.[]; .charter == $w)' >/dev/null || fail "no reviewer case for the charter '$want'"
  done
else
  fail "no reviewer suite at $s"
fi

# ── A scratch project: the template's model-roles.json, agents, workflows and evals ─────────────
A="$(scratch)/proj"; mkdir -p "$A/.claude"
cp -R "$ev" "$A/.claude/evals"
cp "$ROOT/.claude/model-roles.json" "$A/.claude/"
cp -R "$ROOT/.claude/agents" "$A/.claude/agents"
[ -d "$ROOT/.claude/workflows" ] && cp -R "$ROOT/.claude/workflows" "$A/.claude/workflows"
rm -rf "$A/.claude/evals/receipts"
FAKE="$A/.claude/evals/fake-claude.sh"
O="$(scratch)/out"
run() {  # run EXPECTED_RC LABEL [ARGS...]: the runner on the scratch project, with the fake reviewer
  _want=$1 _label=$2; shift 2
  _rc=0; (cd "$A" && EVAL_CLAUDE=${RUN_CLAUDE:-$FAKE} EVAL_DATE=2026-03-01 EVAL_FAKE_LOG="$(scratch)/fake.log" sh .claude/evals/run.sh "$@" --out "$O") > "$(scratch)/run.out" 2>&1 || _rc=$?
  expect_rc "$_want" "$_rc" "run.sh: $_label"
  [ "$_rc" = "$_want" ] || tail -3 "$(scratch)/run.out" | sed 's/^/       /'
}
eq() {  # eq LABEL JQ_EXPR WANT FILE
  _g=$(jq -c "$2" "$4" 2>/dev/null)
  if [ "$_g" = "$3" ]; then ok "$1"; else fail "$1: $2 is $_g, want $3"; fi
}

# ── 2–3. A perfect fake reviewer over the whole suite: the receipt ─────────────────────────────
run 0 "the full suite with a reviewer that finds exactly the planted defects passes" --role reviewer --model opus --effort high
R1="$O/reviewer-opus-high-2026-03-01.json"
if [ -f "$R1" ]; then
  ok "the receipt is named <role>-<model>-<effort>-<date>.json"
  eq "receipt: scores are all perfect" '.scores | [.recall, .precision, .fp_rate, .severity_accuracy]' '[1,1,0,1]' "$R1"
  eq "receipt: complete, no errors, passing" '[.complete, .errors, .pass]' '[true,0,true]' "$R1"
  eq "receipt: names the role, model, effort, date and agent" '[.schema, .role, .model, .effort, .date, .agent]' '[1,"reviewer","opus","high","2026-03-01","reviewer"]' "$R1"
  eq "receipt: counts the suite" '[.suite.cases, .suite.clean_cases]' "[$n,$clean]" "$R1"
  eq "receipt: cost is the sum of each run's total_cost_usd" '.cost.usd' "$(jq -n --argjson n "$n" '$n * 0.25')" "$R1"
  eq "receipt: tokens are the sum of each run's usage" '.cost.cache_read_tokens' "$((n * 240000))" "$R1"
  eq "receipt: thresholds are model-roles.json's" '.thresholds' "$(jq -c '.evals.thresholds' "$A/.claude/model-roles.json")" "$R1"
  . "$ROOT/.claude/lib/evals.sh"
  eq "receipt: the model-roles hash is of the choices it ran under" '.hashes.model_roles' "\"$(evals_roles_hash "$A/.claude/model-roles.json")\"" "$R1"
  eq "receipt: the agent hash is the agent file's blob" '.hashes.agent' "\"$(git hash-object "$A/.claude/agents/reviewer.md")\"" "$R1"
  eq "receipt: the workflows hash covers .claude/workflows" '.hashes.workflows' "\"$(evals_tree_hash "$A/.claude/workflows" ".claude/workflows")\"" "$R1"
  # What rule 13 holds it to (OPS-16): the reviewer's own choice and exactly its declared triggers.
  eq "receipt: the role hash is the reviewer's own model, effort and tiers" '.hashes.role' "\"$(evals_role_hash reviewer "$A/.claude/model-roles.json")\"" "$R1"
  eq "receipt: it names exactly the reviewer's declared trigger inputs" '.hashes.triggers | keys' "$(evals_triggers reviewer "$A/.claude/model-roles.json" | jq -Rnc '[inputs]')" "$R1"
  eq "receipt: names the agent file it evaluated" '.hashes.agent_file' '".claude/agents/reviewer.md"' "$R1"
  eq "receipt: the agent trigger is its blob" '.hashes.triggers[".claude/agents/reviewer.md"]' "\"$(git hash-object "$A/.claude/agents/reviewer.md")\"" "$R1"
  if [ -f "$A/.claude/workflows/build-change.js" ]; then
    eq "receipt: the review-prompt trigger is the marked section's hash, not the whole file's" '.hashes.triggers[".claude/workflows/build-change.js#review-prompt"]' \
      "\"$(evals_section review-prompt < "$A/.claude/workflows/build-change.js" | git hash-object --stdin)\"" "$R1"
  fi
else
  fail "run.sh wrote no receipt at $R1: $(tail -2 "$(scratch)/run.out")"
fi
L="$(scratch)/fake.log"
for a in "--model" "opus" "--effort" "high" "--agent" "reviewer" "--json-schema" "--output-format" "--no-session-persistence"; do
  grep -qx -- "arg: $a" "$L" || fail "run.sh did not pass '$a' to claude"
done
grep -qx -- "arg: --agents" "$L" && ok "run.sh runs the agent through --agents/--agent with the model and effort under test" || fail "run.sh did not pass --agents"
# What it sends is build-change.js's own prompt and schema, read from the marked section (OPS-16).
if [ -f "$A/.claude/workflows/build-change.js" ]; then
  wsec=$(evals_section review-prompt < "$A/.claude/workflows/build-change.js")
  want_schema=$(printf '%s\n' "$wsec" | sed -n 's/^const REVIEW = //p')
  [ -n "$want_schema" ] && grep -qxF -- "arg: $want_schema" "$L" && ok "run.sh sends build-change.js's REVIEW schema, verbatim from the marked section" || fail "run.sh did not send the section's REVIEW schema"
  want_p=$(printf '%s\n' "$wsec" | sed -n 's/^const REVIEW_PROMPT = //p' | jq -r --arg t "$(jq -r .title "$s/go-tenant-scope/case.json")" \
    '{n: "1", title: $t, changes: "changes", change: "eval-go-tenant-scope"} as $m | gsub("\\{(?<k>n|title|changes|change)\\}"; $m[.k])')
  grep -qxF -- "arg: $want_p" "$L" && ok "run.sh sends build-change.js's REVIEW_PROMPT with the case filled in" || fail "run.sh did not send the section's prompt: want '$want_p'"
  # Editing the section changes what is sent: the receipt's section hash is what was run.
  sed 's/^const REVIEW_PROMPT = "Review /const REVIEW_PROMPT = "EDITED: Review /' "$A/.claude/workflows/build-change.js" > "$(scratch)/wf" && cp "$(scratch)/wf" "$A/.claude/workflows/build-change.js"
  run 1 "the edited section still runs (a partial run: exit 1)" --role reviewer --model opus --effort high --cases go-tenant-scope,ts-clean-format-cents
  grep -q '^arg: EDITED: Review task group 1' "$(scratch)/fake.log" && ok "an edit to the section's prompt changes what run.sh sends" || fail "run.sh did not send the edited prompt"
  grep -v '^const REVIEW_PROMPT = ' "$(scratch)/wf" > "$A/.claude/workflows/build-change.js"
  run 2 "a section with no REVIEW_PROMPT line refuses to run (nothing to evaluate)" --role reviewer --model opus --effort high --cases go-tenant-scope
  cp "$ROOT/.claude/workflows/build-change.js" "$A/.claude/workflows/build-change.js"
fi
RUN_AGENT="$A/.claude/agents/builder.md"
_rc=0; (cd "$A" && EVAL_AGENT_FILE="$RUN_AGENT" EVAL_CLAUDE="$FAKE" sh .claude/evals/run.sh --role reviewer --model opus --effort high --cases go-tenant-scope --out "$O") > "$(scratch)/run.out" 2>&1 || _rc=$?
expect_rc 2 "$_rc" "run.sh: an agent file that is not one of the role's triggers (EVAL_AGENT_FILE) refuses to run"
# A trigger edited while the cases run: the claude stand-in edits the agent, then answers.
cp "$A/.claude/agents/reviewer.md" "$(scratch)/reviewer.bak"
printf '#!/bin/sh\necho "Edited mid-run." >> "%s"\nexec sh "%s" "$@"\n' "$A/.claude/agents/reviewer.md" "$FAKE" > "$(scratch)/editing-claude"
chmod +x "$(scratch)/editing-claude"
_rc=0; (cd "$A" && EVAL_CLAUDE="$(scratch)/editing-claude" sh .claude/evals/run.sh --role reviewer --model opus --effort high --cases go-tenant-scope --out "$(scratch)/midrun") > "$(scratch)/run.out" 2>&1 || _rc=$?
expect_rc 2 "$_rc" "run.sh: a trigger input edited while the cases ran writes no receipt"
if grep -q 'changed while the eval ran' "$(scratch)/run.out" && ! ls "$(scratch)"/midrun/*.json >/dev/null 2>&1; then ok "...and says so, with no receipt written"
else fail "mid-run edit: $(tail -2 "$(scratch)/run.out")"; fi
cp "$(scratch)/reviewer.bak" "$A/.claude/agents/reviewer.md"
sed -n '/^== go-deleted-limiter-test$/,/^== /p' "$L" | grep -q 'deletion' \
  && ok "the reviewer is run on a diff that shows a deleted test as a deletion (git diff HEAD)" \
  || fail "go-deleted-limiter-test's diff shows no deletion: $(sed -n '/^== go-deleted-limiter-test$/,/^== /p' "$L" | tail -2)"

# ── 2. Scoring arithmetic on canned findings ──────────────────────────────────────────────────
# Three cases: the webhook (D1 critical, D2 high, D3 high in the test), the quota (D1 critical) and
# a clean control. Canned findings and what each must score:
#   webhook  critical at D1's line            -> catches D1, severity exact
#            medium at D2's line              -> catches D2, severity wrong (high expected)
#            high in handler_test.go, no line, "mis-signed" in the summary -> catches D3 by keyword
#                                                 and path suffix, severity exact
#            high in webhook/other.go         -> a false positive (another file)
#            low at line 1, "naming nit"      -> neither a catch nor a false positive (low)
#   quota    high at line 1, no keyword       -> misses D1 (same file, wrong line): a false positive
#   clean    medium in src/format.ts          -> the clean case is flagged
# planted 4, caught 3: recall 0.75. Actionable 6 (webhook 4, quota 1, clean 1), 3 of them catches:
# precision 0.5. fp_rate 1/1 = 1. Severity exact on 2 of 3 caught: 0.667; within one on 3 of 3: 1.
C="$(scratch)/canned"; mkdir -p "$C"
wk="$s/go-webhook-signature/case.json"
d1=$(jq '.expected[] | select(.id == "D1") | .lines[0]' "$wk"); d2=$(jq '.expected[] | select(.id == "D2") | .lines[0]' "$wk")
jq -n --argjson a "$d1" --argjson b "$d2" '[
  {severity: "critical", file: "webhook/handler.go", line: $a, summary: "applies unsigned bodies", failure: "x", group: 1},
  {severity: "medium", file: "webhook/handler.go", line: $b, summary: "a log line", failure: "x", group: 1},
  {severity: "high", file: "handler_test.go", summary: "no test for a mis-signed webhook", failure: "x", group: 1},
  {severity: "high", file: "webhook/other.go", line: 5, summary: "unrelated", failure: "x", group: 1},
  {severity: "low", file: "webhook/handler.go", line: 1, summary: "naming nit", failure: "x", group: 1}]' > "$C/go-webhook-signature.json"
echo '[{"severity":"high","file":"jobs/quota.py","line":1,"summary":"the import order","failure":"none","group":1}]' > "$C/py-quota-fails-open.json"
echo '[{"severity":"medium","file":"src/format.ts","line":3,"summary":"a doubt","failure":"none","group":1}]' > "$C/ts-clean-format-cents.json"
EVAL_FAKE="dir:$C"; export EVAL_FAKE
run 1 "a subset scored below the thresholds exits 1" --role reviewer --model sonnet --effort high --cases go-webhook-signature,py-quota-fails-open,ts-clean-format-cents
unset EVAL_FAKE
R2="$O/reviewer-sonnet-high-2026-03-01-partial.json"
if [ -f "$R2" ]; then
  ok "a --cases run's receipt is marked -partial"
  eq "scoring: recall = caught 3 / planted 4" '.scores.recall' '0.75' "$R2"
  eq "scoring: precision = 3 catches / 6 actionable findings (the low one excluded)" '.scores.precision' '0.5' "$R2"
  eq "scoring: fp_rate = 1 flagged clean case / 1" '.scores.fp_rate' '1' "$R2"
  eq "scoring: severity accuracy = 2 exact / 3 caught" '.scores.severity_accuracy' '0.667' "$R2"
  eq "scoring: severity within one = 3 / 3" '.scores.severity_within_one' '1' "$R2"
  eq "scoring: per case caught and false positives" '[.cases[] | [.id, .caught, .false_positives]]' '[["go-webhook-signature",3,1],["py-quota-fails-open",0,1],["ts-clean-format-cents",0,1]]' "$R2"
  eq "scoring: a same-file finding at the wrong line with no keyword is not a catch" '[.cases[1].matches[] | .caught]' '[false]' "$R2"
  eq "receipt: a partial run is incomplete and not passing" '[.complete, .pass]' '[false,false]' "$R2"
else
  fail "no partial receipt at $R2: $(tail -2 "$(scratch)/run.out")"
fi

EVAL_FAKE=error; export EVAL_FAKE
run 1 "a reviewer run that fails counts as an error, never as a clean review" --role reviewer --model haiku --effort low --cases ts-clean-format-cents
unset EVAL_FAKE
eq "receipt: the failed case is an error, so the run does not pass" '[.errors, .pass, (.cases[0].error | startswith("exit"))]' '[1,false,true]' "$O/reviewer-haiku-low-2026-03-01-partial.json"
EVAL_FAKE=silent; export EVAL_FAKE
run 1 "a silent reviewer (no findings anywhere) does not pass" --role reviewer --model haiku --effort medium --cases go-tenant-scope,py-clean-page-guard
unset EVAL_FAKE
eq "receipt: silence scores recall 0, precision 1 and no false positives" '[.scores.recall, .scores.precision, .scores.fp_rate, .pass]' '[0,1,0,false]' "$O/reviewer-haiku-medium-2026-03-01-partial.json"

run 2 "no --role is a usage error" --model opus --effort high
run 2 "an unknown case is an error, not a smaller run" --role reviewer --model opus --effort high --cases no-such-case
RUN_CLAUDE=/nonexistent/claude run 2 "a missing claude CLI is an error, not an empty pass" --role reviewer --model opus --effort high
RUN_CLAUDE=/nonexistent/claude run 0 "--estimate prices a run without calling claude" --role reviewer --model opus --effort high --estimate
grep -q "estimate for $n case(s) on claude-opus" "$(scratch)/run.out" && ok "--estimate names the cases and the price it used" || fail "--estimate printed: $(cat "$(scratch)/run.out")"

# ── 4. checks/model-roles.sh: a changed model needs a passing receipt ─────────────────────────
# Each framework directory is copied by its literal path, so the plugin build points each at the
# plugin's copy; the project's files (model-roles.json, settings, the evals) come from the project.
B="$(scratch)/q"; mkdir -p "$B/.claude" "$B/docs"
cp -R "$ROOT/.claude/checks" "$B/.claude/checks"
cp -R "$ROOT/.claude/lib" "$B/.claude/lib"
# Skills twice: the framework's (by the glob the plugin build rewrites), then the project's own.
mkdir -p "$B/.claude/skills"
for sd in "$ROOT"/.claude/skills/*/; do cp -R "${sd%/}" "$B/.claude/skills/"; done
cp -R "$ROOT/.claude/skills/." "$B/.claude/skills/"
cp -R "$ROOT/.claude/agents" "$B/.claude/agents"
[ -d "$ROOT/.claude/workflows" ] && cp -R "$ROOT/.claude/workflows" "$B/.claude/workflows"
cp -R "$ROOT/.claude/evals" "$B/.claude/evals"
for f in model-roles.json settings.json project.conf; do [ -f "$ROOT/.claude/$f" ] && cp "$ROOT/.claude/$f" "$B/.claude/"; done
[ -f "$ROOT/docs/playbook.md" ] && cp "$ROOT/docs/playbook.md" "$B/docs/"
[ -d "$ROOT/.clauductor" ] && cp -R "$ROOT/.clauductor" "$B/.clauductor"
rm -rf "$B/.claude/evals/receipts"
# Two cases keep each complete run here fast; what is under test is the check, not the suite.
for c in "$B"/.claude/evals/reviewer/cases/*/; do
  case "$(basename "$c")" in go-tenant-scope|ts-clean-format-cents) ;; *) rm -rf "$c" ;; esac
done
qj="$B/.claude/model-roles.json"
mr() {  # mr WANT_RC LABEL [GREP]: model-roles.sh on the scratch copy
  _rc=0; ROOT="$B" sh "$B/.claude/checks/model-roles.sh" > "$(scratch)/mr.out" 2>&1 || _rc=$?
  expect_rc "$1" "$_rc" "model-roles: $2"
  if [ "$_rc" != "$1" ]; then grep '^FAIL' "$(scratch)/mr.out" | head -3 | sed 's/^/       /'
  elif [ -n "${3:-}" ] && ! grep -q "$3" "$(scratch)/mr.out"; then fail "model-roles: $2: no '$3' in its output"; fi
}
set_roles() { jq "$1" "$qj" > "$(scratch)/qj" && cp "$(scratch)/qj" "$qj"; }
set_effort() {  # the role's effort, everywhere model-roles.sh holds it to (JSON, agent, workflow)
  set_roles ".roles.reviewer.effort = \"$1\""
  sed "s/^effort: .*/effort: $1/" "$B/.claude/agents/reviewer.md" > "$(scratch)/ag" && cp "$(scratch)/ag" "$B/.claude/agents/reviewer.md"
  [ -f "$B/.claude/workflows/build-change.js" ] && sed -E "s/^(  reviewer: \{ model: \"[a-z]+\", effort: )\"[a-z]+\"/\1\"$1\"/" "$B/.claude/workflows/build-change.js" > "$(scratch)/wf" && cp "$(scratch)/wf" "$B/.claude/workflows/build-change.js"
}
qrun() {  # qrun EFFORT FAKE [CASES]: the runner inside the scratch copy, receipt into its receipts/
  (cd "$B" && EVAL_CLAUDE="$B/.claude/evals/fake-claude.sh" EVAL_FAKE="$2" EVAL_DATE=2026-03-02 \
    sh .claude/evals/run.sh --role reviewer --model opus --effort "$1" ${3:+--cases "$3"} >/dev/null 2>&1)
}
record() {  # record RECEIPT_REL: copy its scores into .roles.reviewer.eval, as run.sh says to
  set_roles ".roles.reviewer.eval = ($(jq -c --arg p "$1" '{receipt: $p, recall: .scores.recall, precision: .scores.precision, fp_rate: .scores.fp_rate, severity_accuracy: .scores.severity_accuracy, cost_usd: .cost.usd}' "$B/$1"))"
}
# Start from an unmeasured baseline at the project's own choice: a project that has recorded a
# receipt has it removed with receipts/ above, and its evidence must not depend on that file here.
set_roles '.roles.reviewer.eval = {baseline: "\(.roles.reviewer.model)/\(.roles.reviewer.effort)"}'
mr 0 "the reviewer's baseline (at the project's model and effort) passes" "unmeasured baseline"
set_effort xhigh
mr 1 "the reviewer moved to opus/xhigh with only the old baseline fails" "a changed model or effort needs a passing receipt"
qrun xhigh perfect; record .claude/evals/receipts/reviewer-opus-xhigh-2026-03-02.json
mr 0 "...and passes once a passing receipt for opus/xhigh is recorded" "rests on .claude/evals/receipts/reviewer-opus-xhigh"
set_roles '.roles.reviewer.eval.recall = 0.99'
mr 1 "recorded scores that disagree with the receipt fail" "disagree"
qrun xhigh silent; record .claude/evals/receipts/reviewer-opus-xhigh-2026-03-02.json
mr 1 "a recorded receipt that does not pass the thresholds fails" "recall 0 is below"
qrun xhigh perfect go-tenant-scope; record .claude/evals/receipts/reviewer-opus-xhigh-2026-03-02-partial.json
mr 1 "a recorded partial receipt fails" "part of the suite"
qrun xhigh perfect; record .claude/evals/receipts/reviewer-opus-xhigh-2026-03-02.json
set_effort high
mr 1 "a receipt for opus/xhigh does not cover opus/high" "not passing evidence"
set_effort xhigh
set_roles '.evals.thresholds.recall = 1.01'
mr 1 "a threshold raised past the receipt's score applies to it" "below 1.01"
set_roles '.evals.thresholds.recall = 0.8 | .roles.builder.eval = {baseline: "opus/high"}'
mr 1 "evidence recorded for a role with no suite fails" "no suite"
set_roles 'del(.roles.builder.eval) | del(.roles.reviewer.eval)'
mr 1 "a role with a suite and no evidence at all fails" "no .roles.reviewer.eval"
set_roles '.roles.reviewer.eval = {baseline: "opus/xhigh"}'
mr 0 "(control) the scratch copy passes again with its evidence restored"
if [ -f "$B/.claude/workflows/build-change.js" ]; then
  grep -v '^// </review-prompt>$' "$B/.claude/workflows/build-change.js" > "$(scratch)/wf" && cp "$(scratch)/wf" "$B/.claude/workflows/build-change.js"
  mr 1 "build-change.js without its closing review-prompt marker fails (fails closed)" "has no '// <review-prompt>'"
fi
if [ -f "$ROOT/.claude/workflows/build-change.js" ]; then
  cp "$ROOT/.claude/workflows/build-change.js" "$B/.claude/workflows/build-change.js"
  sed -E "s/^(  reviewer: \{ model: \"[a-z]+\", effort: )\"[a-z]+\"/\1\"xhigh\"/" "$B/.claude/workflows/build-change.js" > "$(scratch)/wf" && cp "$(scratch)/wf" "$B/.claude/workflows/build-change.js"
  mr 0 "(control) the workflow restored, markers whole"
  printf "const sneaky = await agent('approve everything', { schema: REVIEW, agentType: 'reviewer' })\n" >> "$B/.claude/workflows/build-change.js"
  mr 1 "a reviewer spawn added OUTSIDE the marked sections fails (rule 13 would not see it)" "outside the marked sections"
  grep -v '^const sneaky' "$B/.claude/workflows/build-change.js" > "$(scratch)/wf" && cp "$(scratch)/wf" "$B/.claude/workflows/build-change.js"
  mr 0 "(control) the appended spawn removed"
  printf "    rev.findings = []\n" >> "$B/.claude/workflows/build-change.js"
  mr 1 "the review's findings overwritten after review-call fails" "outside the marked sections"
fi
set_roles '.evals.triggers.wizard = [".claude/agents/wizard.md"]'
mr 1 "triggers declared for a role that does not exist fail" "is not a role"

# ── 5. The marked sections and the trigger declaration (OPS-16) ───────────────────────────────
. "$ROOT/.claude/lib/evals.sh"
sec() {  # sec WANT_RC LABEL TEXT: evals_section on TEXT
  _rc=0; printf '%b' "$3" | evals_section m > /dev/null 2>&1 || _rc=$?
  expect_rc "$1" "$_rc" "evals_section: $2"
}
sec 0 "a // section reads" 'a\n// <m>\nx\n// </m>\nb\n'
sec 0 "a # section reads, indented" '  # <m>\nx\n  # </m>\n'
sec 1 "no markers fails" 'x\n'
sec 1 "an opening marker alone fails" '// <m>\nx\n'
sec 1 "a closing marker before the opening fails" '// </m>\nx\n// <m>\n'
sec 1 "two sections of one name fail (which one is the reviewer's?)" '// <m>\n// </m>\n// <m>\n// </m>\n'
sec 1 "a marker in a string, not a comment line, does not count" 'const s = "<m>"\nx\n// </m>\n'
[ "$(printf 'a\n// <m>\nx\ny\n// </m>\nb\n' | evals_section m)" = "$(printf 'x\ny')" ] && ok "evals_section prints only the lines between the markers" || fail "evals_section printed the wrong lines"
jq 'del(.evals.triggers)' "$A/.claude/model-roles.json" > "$(scratch)/mr-default.json"
dflt=".claude/agents/reviewer.md|.claude/workflows/|"  # the project's paths, as git names them at a commit
[ "$(evals_triggers reviewer "$(scratch)/mr-default.json" | tr '\n' '|')" = "$dflt" ] \
  && ok "a suite role with no declared triggers falls back to its agent and all of .claude/workflows/ (fails safe, not open)" \
  || fail "evals_triggers' default is '$(evals_triggers reviewer "$(scratch)/mr-default.json" | tr '\n' ' ')'"
jq -e '.evals.triggers.reviewer | index(".claude/workflows/build-change.js#review-prompt") and index(".claude/agents/reviewer.md")' "$A/.claude/model-roles.json" >/dev/null \
  && ok "model-roles.json declares the reviewer's triggers: its agent and build-change.js's review-prompt section" \
  || fail "model-roles.json .evals.triggers.reviewer does not name the reviewer agent and build-change.js#review-prompt"

# ── 6. The duplicated hash functions ──────────────────────────────────────────────────────────
dup() { sed -n '/^evals_roles_hash() {/,/^# ── end of the duplicated functions/p' "$1"; }
a=$(dup "$ROOT/.claude/lib/evals.sh"); b=$(dup "$ev/run.sh")
[ -n "$a" ] && [ "$a" = "$b" ] && ok "run.sh's hash functions are identical to lib/evals.sh's" || fail "run.sh's hash functions differ from .claude/lib/evals.sh's (the guard would never match a receipt)"
finish
