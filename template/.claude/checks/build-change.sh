#!/bin/sh
# build-change's review loop decides with pure functions (the `pure` block of
# .claude/workflows/build-change.js): the grade of each round and the stuck-loop breaker. This loads
# that block into node and falsifies it both ways:
#   grades       pass (gate green, nothing medium or worse), concern (worst is medium), fail (gate
#                red, or a high or critical finding);
#   stuck        a fixed, undisputed finding back unchanged; the peak and the count both not
#                falling; the reviewed diff equal to an earlier round's; and NOT stuck when the loop
#                is converging, when the recurring finding was disputed, or in round 1.
# Then, without node, that the loop calls the breaker, stops with the distinct kind 'stuck', and
# records the grades in the Progress line.
#
# node runs workflow scripts, so a project using build-change has it; without it this says SKIPPED
# and why. NODE_REQUIRED=1 makes that a failure (clauductor's CI sets it).
. "$(dirname "$0")/lib.sh"

wf="$ROOT/.claude/workflows/build-change.js"
[ -f "$wf" ] || { ok "no build-change workflow in this project"; finish; }
d=$(scratch)

grep -q 'const stuck = stuckReason(entry.rounds)' "$wf" && ok "the review loop consults the stuck-loop breaker every round" || fail "build-change.js no longer calls stuckReason(entry.rounds) in the review loop"
grep -q "return stop(\`group \${g.n} review\`, \`\${stuck}; grades \${grades(entry)}\${disputes}\`, 'stuck')" "$wf" && ok "a stuck loop stops with the distinct kind 'stuck'" || fail "build-change.js does not stop a stuck loop with kind 'stuck'"
grep -q "\${kind === 'stuck' ? 'STUCK' : 'STOPPED'}" "$wf" && ok "the notify line says STUCK for a stuck loop" || fail "the notify line does not distinguish a stuck loop"
grep -q 'grades ${grades(entry)}"' "$wf" && ok "the Progress line records each round's grade" || fail "the commit step's Progress line does not record the grades"

if ! command -v node >/dev/null 2>&1; then
  if [ "${NODE_REQUIRED:-}" = 1 ]; then fail "node is not installed (NODE_REQUIRED=1), so the loop's logic cannot be exercised"
  else ok "SKIPPED: node is not installed, so build-change's grading and stuck-loop logic is not exercised here"; fi
  finish
fi
sed -n '/^\/\/ ── pure: begin/,/^\/\/ ── pure: end/p' "$wf" > "$d/pure.js"
[ -s "$d/pure.js" ] || { fail "build-change.js has no '// ── pure: begin' … '// ── pure: end' block"; finish; }
cat >> "$d/pure.js" <<'EOF'
const out = []
const t = (label, got, want) => out.push(`${got === want ? 'ok  ' : 'FAIL'} ${label}${got === want ? '' : ` (got ${JSON.stringify(got)}, want ${JSON.stringify(want)})`}`)
const F = (severity, file, summary) => ({ severity, file, summary })
t('grade: gate red is fail', gradeRound(false, []), 'fail')
t('grade: a high finding is fail', gradeRound(true, [F('high', 'a.ts', 'x')]), 'fail')
t('grade: a critical finding is fail', gradeRound(true, [F('critical', 'a.ts', 'x')]), 'fail')
t('grade: worst finding medium is concern', gradeRound(true, [F('medium', 'a.ts', 'x'), F('low', 'b', 'y')]), 'concern')
t('grade: only low findings is pass', gradeRound(true, [F('low', 'a.ts', 'x')]), 'pass')
t('grade: no findings is pass', gradeRound(true, []), 'pass')
const k1 = findingKey(F('medium', 'src/a.ts', 'Null check missing on the user id path'))
t('key: line numbers and punctuation do not matter', findingKey(F('medium', 'src/a.ts', 'Null-check missing on the user id path, again!')), k1)
t('key: another file is another finding', findingKey(F('medium', 'src/b.ts', 'Null check missing on the user id path')) === k1, false)
const R = (peak, actionable, keys, diffHash, disputedKeys) => ({ peak, actionable, keys, diffHash, disputedKeys })
t('stuck: never in round 1', stuckReason([R('high', 2, [k1], 'h1')]), null)
t('stuck: a fixed, undisputed finding back unchanged is recurring', (stuckReason([R('medium', 1, [k1], 'h1'), R('medium', 1, [k1], 'h2')]) || '').split(':')[0], 'recurring')
t('stuck: the same finding back after the builder DISPUTED it is not recurring', stuckReason([R('medium', 1, [k1], 'h1', [k1]), R('medium', 1, [k1], 'h2')]) === null || !String(stuckReason([R('medium', 1, [k1], 'h1', [k1]), R('medium', 1, [k1], 'h2')])).startsWith('recurring'), true)
t('stuck: the same diff as an earlier round is oscillating', (stuckReason([R('high', 2, ['a'], 'h1'), R('medium', 1, ['b'], 'h2'), R('low', 1, ['c'], 'h1')]) || '').split(':')[0], 'oscillating')
t('stuck: peak and count both not falling is not falling', (stuckReason([R('medium', 2, ['a', 'b'], 'h1'), R('medium', 2, ['c', 'd'], 'h2')]) || '').split(':')[0], 'not falling')
t('stuck: the peak falling is converging', stuckReason([R('high', 2, ['a', 'b'], 'h1'), R('medium', 3, ['c', 'd', 'e'], 'h2')]), null)
t('stuck: the count falling at the same peak is converging', stuckReason([R('medium', 3, ['a', 'b', 'c'], 'h1'), R('medium', 1, ['d'], 'h2')]), null)
console.log(out.join('\n'))
EOF
node "$d/pure.js" > "$d/out" 2>&1 || echo "FAIL node could not run the pure block: $(tail -3 "$d/out")" >> "$d/out"
cat "$d/out"; _fails=$((_fails + $(grep -c '^FAIL' "$d/out")))
finish
