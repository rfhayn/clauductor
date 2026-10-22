#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
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
# And the project's settings: build-change takes its branch prefix, changes directory, gate,
# attribution trailer and provenance from .claude/project-config.sh at run time, never from a
# constant a project would have to edit (OPS-8 rehearsal: turning attribution off meant editing
# this framework file, which `clauductor update` then flagged forever). project-config.sh is run
# against a project with attribution and provenance off and other branch keys, and its output is
# fed to the workflow's own projectSettings and trailerBlock.
# Then the driver (below): the whole script, run with a stubbed agent, for review routing and the
# main-checkout stop.
#
# node runs workflow scripts, so a project using build-change has it; without it this says SKIPPED
# and why. NODE_REQUIRED=1 makes that a failure (clauductor's CI sets it).
. "$(dirname "$0")/lib.sh"

wf="$CLAUDUCTOR_FW/workflows/build-change.js"
[ -f "$wf" ] || { ok "no build-change workflow in this project"; finish; }
d=$(scratch)

grep -q 'const stuck = stuckReason(entry.rounds)' "$wf" && ok "the review loop consults the stuck-loop breaker every round" || fail "build-change.js no longer calls stuckReason(entry.rounds) in the review loop"
grep -q "return stop(\`group \${g.n} review\`, \`\${stuck}; grades \${grades(entry)}\${disputes}\`, 'stuck')" "$wf" && ok "a stuck loop stops with the distinct kind 'stuck'" || fail "build-change.js does not stop a stuck loop with kind 'stuck'"
grep -q "\${kind === 'stuck' ? 'STUCK' : 'STOPPED'}" "$wf" && ok "the notify line says STUCK for a stuck loop" || fail "the notify line does not distinguish a stuck loop"
grep -q 'grades ${grades(entry)}"' "$wf" && ok "the Progress line records each round's grade" || fail "the commit step's Progress line does not record the grades"

# Nothing a project configures is a constant in the workflow.
for c in 'const PROVENANCE =' 'const ATTRIBUTION' "|| 'change/'" "args.branchPrefix || "; do
  grep -qF "$c" "$wf" && fail "build-change.js hard-codes a project setting ($c): read it from project-config.sh (CFG)" || ok "build-change.js has no '$c'"
done
grep -qF 'project-config.sh --json' "$wf" && grep -qF 'CFG = projectSettings(settings.output, args)' "$wf" \
  && ok "build-change.js reads the project's settings at run time (project-config.sh --json, projectSettings)" \
  || fail "build-change.js does not read its settings from .claude/project-config.sh --json"

# project-config.sh against a project that turned attribution and provenance off and moved its branches.
pc="$CLAUDUCTOR_FW/project-config.sh"
if [ -f "$pc" ] && command -v jq >/dev/null 2>&1; then
  mkdir -p "$d/p/.claude/lib"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$d/p/.claude/lib/"; cp "$pc" "$d/p/.claude/"
  conf='BRANCH_CHANGE="feature/"\nCHANGES_DIR="openspec/changes"\nGATE="infra/ci/gate.sh"\nGATE_QUICK_FLAGS="--fast"\n'
  printf '{"attribution": {"enabled": false, "trailer": "Co-Authored-By: X <x@y>"}, "provenance": {"enabled": false}}\n' > "$d/p/.claude/model-roles.json"
  # gate-wait.sh is where install put it (Mapping): beside the template's runner, moved or not, and
  # at scripts/ci/ beside a project's own runner (OPS-72).
  for _gw in 'scripts/ci/run-local.sh scripts/ci/gate-wait.sh' './ci/run-local.sh ci/gate-wait.sh' 'bin/ci.sh scripts/ci/gate-wait.sh' \
    'run-local.sh ./gate-wait.sh' './run-local.sh ./gate-wait.sh' 'scripts/ci//run-local.sh scripts/ci/gate-wait.sh' \
    'ci/../run-local.sh ./gate-wait.sh' 'ci/run-local.sh/ ci/gate-wait.sh'; do
    set -- $_gw
    printf "${conf}GATE_RUN=\"%s\"\n" "$1" > "$d/p/.claude/project.conf"
    _got=$(ROOT= sh "$d/p/.claude/project-config.sh" --json 2>&1 | jq -r .gateWait 2>&1)
    [ "$_got" = "$2" ] && ok "project-config.sh --json: GATE_RUN=$1 puts gate-wait at $2" || fail "project-config.sh --json: GATE_RUN=$1 gave gateWait '$_got', want $2 (where install puts it)"
  done
  # The gate the workflow runs is GATE cleaned as install writes it into the sandbox exclusion.
  # A gate at the repository root keeps ./ (a bare name is looked up on PATH); an empty GATE stays
  # empty, so the workflow runs its default.
  printf 'GATE=""\n' > "$d/p/.claude/project.conf"
  _got=$(ROOT= sh "$d/p/.claude/project-config.sh" --json 2>&1 | jq -r .gate 2>&1)
  [ -z "$_got" ] && ok "project-config.sh --json: an empty GATE gives an empty gate (the workflow's default applies)" || fail "project-config.sh --json: an empty GATE gave gate '$_got', want empty"
  for _gw in './infra/ci/gate.sh infra/ci/gate.sh' 'infra//ci/gate.sh infra/ci/gate.sh' 'x/../infra/ci/gate.sh/ infra/ci/gate.sh' \
    './gate.sh ./gate.sh' 'gate.sh ./gate.sh'; do
    set -- $_gw
    printf 'GATE="%s"\n' "$1" > "$d/p/.claude/project.conf"
    _got=$(ROOT= sh "$d/p/.claude/project-config.sh" --json 2>&1 | jq -r .gate 2>&1)
    [ "$_got" = "$2" ] && ok "project-config.sh --json: GATE=$1 gives gate $2, as install writes it" || fail "project-config.sh --json: GATE=$1 gave gate '$_got', want $2 (the path install excludes from the sandbox)"
  done
  printf "${conf}GATE_RUN=\"infra/ci/run-local.sh\"\n" > "$d/p/.claude/project.conf"
  ROOT= sh "$d/p/.claude/project-config.sh" --json > "$d/off.json" 2>&1
  printf '{"attribution": {"enabled": true, "trailer": "Co-Authored-By: X <x@y>"}, "provenance": {"enabled": true}}\n' > "$d/p/.claude/model-roles.json"
  ROOT= sh "$d/p/.claude/project-config.sh" --json > "$d/on.json" 2>&1
  [ "$(ROOT= sh "$d/p/.claude/project-config.sh" branch change add-x)" = "feature/add-x" ] && ok "project-config.sh branch change add-x follows BRANCH_CHANGE" || fail "project-config.sh branch does not follow BRANCH_CHANGE"
  jq -e '.branch.change == "feature/" and .changesDir == "openspec/changes" and .gate == "infra/ci/gate.sh" and .gateWait == "infra/ci/gate-wait.sh" and .quickFlags == "--fast" and .attribution == "" and .provenance == false' "$d/off.json" >/dev/null \
    && ok "project-config.sh --json reads project.conf and model-roles.json (attribution and provenance off)" || fail "project-config.sh --json printed: $(cat "$d/off.json")"
else
  ok "SKIPPED: no .claude/project-config.sh or no jq here"
fi

if ! command -v node >/dev/null 2>&1; then
  if [ "${NODE_REQUIRED:-}" = 1 ]; then fail "node is not installed (NODE_REQUIRED=1), so the loop's logic cannot be exercised"
  else ok "SKIPPED: node is not installed, so build-change's grading, stuck-loop, review-routing and checkout logic is not exercised here"; fi
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
// The reviewer names each finding's class (OPS-97) in the schema's `class` field: the key never reads
// it, so two findings of one class in one file are two findings, and a recurrence is one however its
// class is reworded.
const FC = (summary, cls) => ({ ...F('medium', 'src/a.ts', summary), class: cls })
// A class with no marker word, so the summary fallback below cannot hide a key that reads the field.
t('key: the class field is not the finding', findingKey(FC('Null check missing', 'a control that silently does nothing')), 'src/a.ts|null check missing')
t('key: a recurrence whose class field changes is one key', findingKey(FC('Null check missing on the user id path', 'one more unchecked id; the class is a control that silently does nothing')), k1)
t('key: two findings sharing a class field differ', findingKey(FC('The signature check only logs a mismatch', 'class X, first seen')) === findingKey(FC('The retry loop swallows every error', 'class X, first seen')), false)
// With the field filled, a marker word in the summary is the finding's own: no fallback drops it.
t('key: with a class field, "This class …" is the finding', findingKey(FC('This class never closes its DB connection. The fix is a finally block.', 'class X, first seen')) === findingKey(FC('This class swallows every parse error. The fix is a finally block.', 'class X, first seen')), false)
t('key: a blank class field is no class field', findingKey(FC('Class: unchecked id, first seen. Null check missing on the user id path', ' ')), k1)
// The fallback, for a summary that still carries its class: one finding is one key however its
// class clause is worded.
const kc = (s) => findingKey(F('medium', 'src/a.ts', s))
t('key: a leading class clause is not the finding', kc('Class: a control that silently does nothing (first seen). Null check missing on the user id path'), k1)
t('key: a class clause ended by a comma and "first seen"', kc('Class: a control that silently does nothing, first seen. Null check missing on the user id path'), k1)
t('key: "one more X; the class is Y" is a class clause too', kc('One more unchecked id; the class is a control that silently does nothing. Null check missing on the user id path'), k1)
t('key: "class Y, first seen" with no colon', kc('class a control that silently does nothing, first seen. Null check missing on the user id path'), k1)
t('key: two findings of one class in one file differ', kc('Class: a control that silently does nothing (first seen). The signature check only logs a mismatch') === kc('Class: a control that silently does nothing (first seen). The retry loop swallows every error'), false)
// The class's only marker sits before its dot: a clause cut at "package." would leave "json drift" in the key.
t('key: a dot inside the class (package.json) does not end it', kc('Class: package.json drift. Null check missing on the user id path'), k1)
t('key: ...nor a "Class: X:" class that ends at its colon', kc('Class: package.json drift: Null check missing on the user id path'), k1)
// A stray ")" must not take the depth below zero, where no clause would ever end again.
t('key: a stray close paren does not swallow the class clause after it', kc('Stray paren) in the loop. Class: unchecked id, first seen. Null check missing on the user id path'), kc('Stray paren) in the loop. Null check missing on the user id path'))
// "one more" marks a clause only where it leads one; inside a sentence it is the finding's own words.
t('key: a "one more" leading a later clause is dropped', kc('Null check missing; one more unchecked id. On the user id path'), k1)
t('key: a "one more" inside a sentence keeps it', kc('The loop needs one more bound check. It reads past the end.'), 'src/a.ts|the loop needs one more bound')
t('key: a summary that is only a class clause keys on itself', kc('Class: a control that silently does nothing (first seen).') === `src/a.ts|`, false)
// A reviewer that writes the class in the summary anyway may put it anywhere, worded any way: the
// clause or aside that carries a class marker is dropped, not a list of phrasings.
t('key: "This class (…) is first seen." leading', kc('This class (a control that silently does nothing) is first seen. Null check missing on the user id path'), k1)
t('key: two "This class … is first seen." findings differ', kc('This class (a control that silently does nothing) is first seen. The signature check only logs a mismatch') === kc('This class (a control that silently does nothing) is first seen. The retry loop swallows every error'), false)
t('key: "Another of this class (…)." with no other marker', kc('Another of this class (a control that silently does nothing). Null check missing on the user id path'), k1)
t('key: "First seen: X." leading', kc('First seen: a control that silently does nothing. Null check missing on the user id path'), k1)
t('key: a class aside after a dash, early', kc('Null check missing — class: unchecked id, first seen. On the user id path'), k1)
t('key: a nested aside is one aside', kc('Null check missing (class: an input guard (refunds.ts validates it), first seen) on the user id path'), k1)
t('key: a nested-aside recurrence reworded "one more" is one key', kc('Missing guard (class: an input guard (refunds.ts validates it), first seen). The total is unchecked'), kc('Missing guard (class: an input guard (refunds.ts validates it), one more). The total is unchecked'))
t('key: "class Y, one more." keys as "class Y, first seen." does', kc('class a control that silently does nothing, one more. Null check missing on the user id path'), k1)
t('key: a "(class: …)" aside is dropped', kc('Null check missing (class: a control that silently does nothing, first seen) on the user id path'), k1)
t('key: a "; the class is …" clause is dropped', kc('Null check missing; the class is a control that silently does nothing. On the user id path'), k1)
t('key: a class clause ended by a colon', kc('Class: a control that silently does nothing (first seen): null check missing on the user id path'), k1)
t('key: two findings of one class, the class mid-summary, differ', kc('Task untested (class: named behaviour with no test, first seen). Task 1.2 has no test') === kc('Task untested (class: named behaviour with no test, first seen). Task 1.3 has no test'), false)
// No marker, no strip: a summary that only starts with a word like "class" keeps every word.
t('key: "Classes …." keeps its first sentence', kc('Classes of user ids are unchecked. The handler trusts them.'), 'src/a.ts|classes of user ids are unchecked')
t('key: "Classify …." keeps its first sentence', kc('Classify the input first. It drops every id.'), 'src/a.ts|classify the input first it drops')
t('key: the noun "Class" with no marker keeps its sentence', kc('Class UserRepo does not close its connection. The pool runs dry.'), 'src/a.ts|class userrepo does not close its')
t('key: "Class-level …." keeps its sentence', kc('Class-level lock is never released. The handler leaks it.'), 'src/a.ts|class level lock is never released')
const R = (peak, actionable, keys, diffHash, disputedKeys) => ({ peak, actionable, keys, diffHash, disputedKeys })
t('stuck: never in round 1', stuckReason([R('high', 2, [k1], 'h1')]), null)
t('stuck: a fixed, undisputed finding back unchanged is recurring', (stuckReason([R('medium', 1, [k1], 'h1'), R('medium', 1, [k1], 'h2')]) || '').split(':')[0], 'recurring')
t('stuck: the same finding back after the builder DISPUTED it is not recurring', stuckReason([R('medium', 1, [k1], 'h1', [k1]), R('medium', 1, [k1], 'h2')]) === null || !String(stuckReason([R('medium', 1, [k1], 'h1', [k1]), R('medium', 1, [k1], 'h2')])).startsWith('recurring'), true)
t('stuck: the same diff as an earlier round is oscillating', (stuckReason([R('high', 2, ['a'], 'h1'), R('medium', 1, ['b'], 'h2'), R('low', 1, ['c'], 'h1')]) || '').split(':')[0], 'oscillating')
t('stuck: peak and count both not falling is not falling', (stuckReason([R('medium', 2, ['a', 'b'], 'h1'), R('medium', 2, ['c', 'd'], 'h2')]) || '').split(':')[0], 'not falling')
t('stuck: the peak falling is converging', stuckReason([R('high', 2, ['a', 'b'], 'h1'), R('medium', 3, ['c', 'd', 'e'], 'h2')]), null)
t('stuck: the count falling at the same peak is converging', stuckReason([R('medium', 3, ['a', 'b', 'c'], 'h1'), R('medium', 1, ['d'], 'h2')]), null)
const fs = require('fs')
const read = (f) => { try { return fs.readFileSync(f, 'utf8') } catch (e) { return null } }
const off = read(process.env.D + '/off.json'), on = read(process.env.D + '/on.json')
if (off != null) {
  const c = projectSettings(off, {})
  t('settings: the branch prefix is the project\'s', c.branchPrefix, 'feature/')
  t('settings: the changes directory is the project\'s', c.changesDir, 'openspec/changes')
  t('settings: the gate is the project\'s', c.gate, 'infra/ci/gate.sh')
  t('settings: gate-wait is where install put it, beside the project\'s runner', c.gateWait, 'infra/ci/gate-wait.sh')
  t('settings: the quick flags are the project\'s', c.quickFlags, '--fast')
  t('settings: a quickFlags arg wins, even an empty one', projectSettings(off, { quickFlags: '' }).quickFlags, '')
  t('settings: attribution and provenance off give a commit no trailer', trailerBlock(c, 'add-x', 'builder', 'opus', 's1'), '')
  const c2 = projectSettings(on, {})
  t('settings: provenance and attribution on give both trailers', trailerBlock(c2, 'add-x', 'builder', 'opus', 's1'), '\n\nChange: add-x\nAgent-Role: builder\nModel: opus\nSession: s1\nCo-Authored-By: X <x@y>')
  t('settings: an args override wins for one run', projectSettings(off, { branchPrefix: 'x/', attribution: 'A: b' }).branchPrefix + projectSettings(off, { attribution: 'A: b' }).attribution, 'x/A: b')
}
t('settings: output that is not the script\'s JSON stops the run', Boolean(projectSettings('jq: command not found', {}).error), true)
console.log(out.join('\n'))
EOF
D="$d" node "$d/pure.js" > "$d/out" 2>&1 || echo "FAIL node could not run the pure block: $(tail -3 "$d/out")" >> "$d/out"
cat "$d/out"; _fails=$((_fails + $(grep -c '^FAIL' "$d/out")))

# ── The reviewer as it actually runs (OPS-16) ─────────────────────────────────────────────────
# Rule 13 hashes the marked review sections and the eval runner reads REVIEW_PROMPT and REVIEW from
# their const LINES. Two things could still make the reviewer that runs differ from the one an eval
# measured, and only executing the code shows them:
#   - pick(), outside the sections, chooses the reviewer's model and effort at run time: it must
#     give exactly model-roles.json's reviewer at every Risk tier, and never an economy drop;
#   - the section's code, not its const lines, is what is sent: an ASI continuation after a const,
#     or a reviewPrompt body that ignores REVIEW_PROMPT, would send something else.
# So the tables, pick() and the section are loaded into node with `agent` stubbed, and held to
# model-roles.json and to the const lines. Each attack is replayed on a copy and must fail.
roles_json="$ROOT/.claude/model-roles.json"
want=$(jq -c '.roles.reviewer as $r | {normal: {model: $r.model, effort: $r.effort},
  low: ($r.tiers.low // $r | {model, effort}), high: ($r.tiers.high // $r | {model, effort})}' "$roles_json" 2>/dev/null)
cat > "$d/contract.js" <<'EOF'
const fs = require('fs')
const [wfPath, wantJson] = process.argv.slice(2)
const src = fs.readFileSync(wfPath, 'utf8')
const out = []
const t = (label, ok, why) => out.push(`${ok ? 'ok  ' : 'FAIL'} ${label}${ok ? '' : ` (${why})`}`)
const between = (a, b) => { const i = src.indexOf(a); const j = src.indexOf(b, i + 1); return i < 0 || j < 0 ? null : src.slice(i, j) }
const deq = (a, b) => JSON.stringify(a) === JSON.stringify(b)
const prefixRun = async (risk, econ) => {
  // The WHOLE script up to the per-group loop, executed for real with agent, phase and log
  // stubbed and the settings and preflight answered: any statement before the loop that mutates
  // ROLES, TIERS or ECONOMY, or redefines pick, has run by the time pick('reviewer') is asked.
  const cut = src.indexOf('\nfor (const g of todo) {')
  if (cut < 0) throw new Error("cannot find the review loop ('for (const g of todo) {') in build-change.js")
  const prefix = src.slice(0, cut).replace(/^export const meta/m, 'const meta')
  const settings = { output: JSON.stringify({ branch: { change: 'change/' }, changesDir: 'changes', gate: 'infra/ci/gate.sh', quickFlags: '--fast', attribution: '', provenance: false }) }
  // The preflight answer also carries a gate that disagrees: the project's gate is read ONCE, from
  // the settings, and nothing the preflight agent reports may replace it.
  const pre = { branch: 'change/x', clean: true, dirtyFiles: [], groups: [{ n: 1, title: 't', openTasks: 1 }],
    gitDir: '/r/.git/worktrees/x', commonDir: '/r/.git', toplevel: '/r', risk, budgetUsd: null, costUsd: null, economy: econ, today: '2026-01-01',
    gate: 'scripts/ci/gate.sh', quickFlags: '--quick' }
  const agent = async (p, o) => (o && o.label === 'preflight' ? pre : o && o.label === 'settings' ? settings : null)
  const body = `return (async () => {\n${prefix}\nreturn { __ran: true, pick, gate: GATE_CMD, quick: QUICK }\n})()`
  return new Function('args', 'agent', 'phase', 'log', body)({ change: 'x' }, agent, () => {}, () => {})
}
;(async () => { try {
  const want = JSON.parse(wantJson)
  for (const risk of ['low', 'normal', 'high']) for (const econ of [false, true]) {
    const r = await prefixRun(risk, econ)
    if (!r || !r.__ran) { t(`the script up to the review loop runs (Risk ${risk})`, false, `it stopped early: ${JSON.stringify(r && (r.reason || r))}`); continue }
    if (risk === 'normal' && !econ) t("the gate and quick flags the run uses are project-config.sh's, read once",
      r.gate === 'infra/ci/gate.sh' && r.quick === '--fast', `got ${JSON.stringify({ gate: r.gate, quick: r.quick })}`)
    const got = r.pick('reviewer')
    t(`after the whole prefix ran, pick('reviewer') at Risk ${risk}${econ ? ', economy on' : ''} is model-roles.json's reviewer ${JSON.stringify(want[risk])}`,
      deq({ model: got.model, effort: got.effort }, want[risk]) && Object.keys(got).every((k) => k === 'model' || k === 'effort'),
      `got ${JSON.stringify(got)}`)
  }
  const sec = between('// <review-prompt>', '// </review-prompt>')
  if (!sec) throw new Error('no review-prompt section')
  const line = (name) => { const m = sec.match(new RegExp(`^const ${name} = (.*)$`, 'm')); return m ? JSON.parse(m[1]) : undefined }
  const P = line('REVIEW_PROMPT'), S = line('REVIEW')
  let sent = null
  const pickStub = () => ({ model: 'm', effort: 'e' })
  const m = new Function('CHANGES', 'PIN', 'pick', 'agent', `${sec}\nreturn { REVIEW_PROMPT, REVIEW, reviewPrompt, reviewSpawn }`)(
    'changes', 'PIN|', pickStub, (prompt, opts) => { sent = { prompt, opts }; return 'r' })
  const fill = (p, g, change) => p.replace(/\{(n|title|changes|change)\}/g, (_, k) => ({ n: String(g.n), title: g.title, changes: 'changes', change })[k])
  const g = { n: 'N', title: 'T {change}' }
  t('REVIEW_PROMPT as evaluated equals its const line (what the eval runner reads)', m.REVIEW_PROMPT === P, 'a continuation or reassignment changes it')
  t('REVIEW as evaluated deep-equals its const line (what the eval runner reads)', deq(m.REVIEW, S), 'the schema object differs from its line')
  t("reviewPrompt() is REVIEW_PROMPT filled in one pass", m.reviewPrompt(g, 'C', []) === fill(P, g, 'C'), `got ${JSON.stringify(m.reviewPrompt(g, 'C', []))}`)
  m.reviewSpawn(g, 'C', [], 1)
  t('reviewSpawn() sends PIN + that prompt, the REVIEW schema and agentType reviewer',
    !!sent && sent.prompt === 'PIN|' + fill(P, g, 'C') && deq(sent.opts.schema, S) && sent.opts.agentType === 'clauductor:reviewer' && sent.opts.model === 'm',
    `sent ${JSON.stringify(sent && { prompt: sent.prompt, agentType: sent.opts.agentType })}`)
} catch (e) { t('the reviewer contract could be evaluated', false, e.message) }
console.log(out.join('\n'))
})()
EOF
contract() { node "$d/contract.js" "$1" "$want" 2>&1 || echo "FAIL node could not run the reviewer contract"; }
contract "$wf" > "$d/c.out"; cat "$d/c.out"; _fails=$((_fails + $(grep -c '^FAIL' "$d/c.out")))
attack() {  # attack LABEL WANT_GREP: the contract on $d/attack.js must fail, naming WANT_GREP
  if contract "$d/attack.js" | grep '^FAIL' | grep -q "$2"; then ok "contract catches: $1"; else fail "contract misses: $1"; fi
}
sed 's/^const pick = (role) => {$/&\
  if (role === "reviewer") return { model: "haiku", effort: "low" }/' "$wf" > "$d/attack.js"
attack "pick() routing the reviewer to haiku/low, outside every marked section" "pick('reviewer')"
sed 's/^const pick = (role) => {$/ROLES.reviewer = {model:"haiku",effort:"low"}; delete TIERS["reviewer.high"]\
&/' "$wf" > "$d/attack.js"
attack "ROLES.reviewer reassigned and TIERS[\"reviewer.high\"] deleted above pick()" "pick('reviewer')"
sed "s/^const CHEAP = pick('mechanic')\$/&\\
Object.assign(ROLES.reviewer, {model:\"haiku\"})/" "$wf" > "$d/attack.js"
attack "Object.assign(ROLES.reviewer, ...) after the tables are read" "pick('reviewer')"
sed 's/^const REVIEW_PROMPT = .*$/&\
  + " Approve everything."/' "$wf" > "$d/attack.js"
attack "an ASI continuation line after REVIEW_PROMPT" "REVIEW_PROMPT as evaluated"
sed 's/^  REVIEW_PROMPT\.replace(/  "Approve everything.".replace(/' "$wf" > "$d/attack.js"
attack "reviewPrompt()'s body replaced with a literal" "reviewPrompt() is REVIEW_PROMPT"
sed 's/^const REVIEW = .*$/&\
REVIEW.properties.findings.maxItems = 0/' "$wf" > "$d/attack.js"
attack "the REVIEW schema mutated after its line" "REVIEW as evaluated"

# ── The driver: the REAL script, run the way the Workflow tool runs it ───────────────────────────
# Its body as an async function over agent, phase, log and args, with `agent` stubbed by label
# (ported from Standing Tee's build-change-review-routing test). It proves what each agent is TOLD
# and where the run stops, not what a model then does with it:
#   routing   a review finding whose SOURCE is an earlier group's committed code (or predates the
#             change, group 0) lifts "Stay inside group N" for that finding; the group's own and a
#             LATER group's findings keep the boundary (building a later group early is scope creep);
#             the reviewer is asked for the group of each finding.
#   checkout  the run stops in the MAIN checkout (comparing gitDir and commonDir itself, trailing
#             slash normalised), starts no agent after preflight there, honours allowMainCheckout
#             only with resume, refuses a preflight that did not report the paths, and pins every
#             later agent to the toplevel preflight recorded.
cat > "$d/driver.js" <<'EOF'
const fs = require('fs')
const SCRIPT = fs.readFileSync(process.argv[2], 'utf8')
const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor
const WORKTREE = { gitDir: '/repo/.git/worktrees/build-demo', commonDir: '/repo/.git', toplevel: '/repo/.claude/worktrees/build-demo' }
async function run(round1, { where = WORKTREE, args = {}, cfg = {}, budget = {}, commitCost = null } = {}) {
  const calls = []
  const reviews = [{ findings: round1 }, { findings: [] }]
  let h = 0
  const agent = async (prompt, opts) => {
    calls.push({ prompt, opts })
    const { label } = opts
    if (label === 'settings') return { output: JSON.stringify({ branch: { change: 'change/' }, changesDir: 'changes', gate: 'scripts/ci/gate.sh', gateWait: 'scripts/ci/gate-wait.sh', attribution: '', provenance: false, ...cfg }) }
    if (label === 'preflight') return { branch: 'change/demo', clean: true, ...where, risk: 'normal', budgetUsd: null, budgetKind: 'none', costUsd: null, economy: false, today: '2026-10-01', gate: '', quickFlags: '', ...budget,
      groups: [{ n: 1, title: 'Substrate', openTasks: 0 }, { n: 2, title: 'Service and API', openTasks: 0 }, { n: 3, title: 'Screens', openTasks: 2 }, { n: 4, title: 'Close-out', openTasks: 0 }] }
    if (label.startsWith('build:') || label.startsWith('review-fix:') || label.startsWith('gate-fix:')) return { status: 'done', summary: 'ok', disputed: [] }
    if (label.startsWith('gate:') || label === 'receipt') return { passed: true, evidence: 'ok', diffHash: `h${++h}` }
    if (label.startsWith('review:')) return reviews.shift()
    if (label.startsWith('commit:')) return { committed: true, sha: 'abc1234', costUsd: commitCost }
    if (label === 'verify') return { passed: true, output: 'ok' }
    throw new Error(`unexpected agent label ${label}`)
  }
  const body = SCRIPT.replace(/^export const meta\b/m, 'const meta')
  const fn = new AsyncFunction('agent', 'phase', 'log', 'args', body)
  const report = await fn(agent, () => {}, () => {}, { change: 'demo', ...args })
  const fix = calls.find((c) => c.opts.label === 'review-fix:3#1')
  const review = calls.find((c) => c.opts.label === 'review:3#1')
  return { report, fix: fix ? fix.prompt : '', review, calls }
}
const finding = (group, file = 'apps/web/app/leagues/LinkOnVisit.tsx') => ({ severity: 'medium', file, line: 49, group, summary: 'a partial link is neither announced nor listed', failure: 'league A commits, league B faults, the route answers 500' })
const LIFTED = 'the group boundary does not apply to them'
const out = []
const t = (label, cond, got) => out.push(`${cond ? 'ok  ' : 'FAIL'} driver: ${label}${cond ? '' : ` (got ${JSON.stringify(got)})`}`)
;(async () => {
  let r = await run([finding(3)])
  const items = r.review && r.review.opts.schema.properties.findings.items
  t('the reviewer is asked which group holds each finding\'s source', !!items && items.required.includes('group') && !!items.properties.group, items && items.required)
  t('the reviewer is asked for each finding\'s class in its own field (OPS-97)', !!items && items.required.includes('class') && !!items.properties.class, items && items.required)
  r = await run([{ ...finding(3), class: 'one more unannounced link; the class is a partial write with no report' }])
  t('a finding\'s class reaches the fixer, with what to do with a second instance', r.fix.includes('\n   class: one more unannounced link; the class is a partial write with no report') && r.fix.includes('close the class') && r.fix.includes('to the Decision log in changes/demo/tasks.md') && r.fix.includes('return design-issue naming it'), r.fix)
  r = await run([finding(3)])
  t('a finding of the group\'s own keeps "Stay inside group 3." and does not lift it', r.fix.includes('Stay inside group 3.') && !r.fix.includes(LIFTED) && r.report.groups[0].rounds[0].outside === 0, r.fix)
  r = await run([finding(2, 'packages/service/src/leagues.ts')])
  t('a finding sourced in an EARLIER group lifts the boundary for it (source: group 2), and the run finishes', r.report.stoppedAt === null && r.fix.includes('source: group 2') && r.fix.includes(LIFTED) && r.report.groups[0].rounds[0].outside === 1, [r.report.stoppedAt, r.fix])
  r = await run([finding(0, 'packages/service/src/roster.ts')])
  t('a finding whose source predates the change (group 0) lifts it too', r.fix.includes('source: predates this change') && r.fix.includes(LIFTED), r.fix)
  r = await run([finding(4)])
  t('a finding a LATER group owns keeps the boundary', r.fix.includes('Stay inside group 3.') && !r.fix.includes(LIFTED), r.fix)
  const MAIN = { gitDir: '/repo/.git', commonDir: '/repo/.git/', toplevel: '/repo' }
  // The settings agent (project-config.sh) runs before preflight; "after preflight" counts from it.
  const afterPreflight = (calls) => { const i = calls.findIndex((c) => c.opts.label === 'preflight'); return i < 0 ? null : calls.slice(i + 1) }
  r = await run([], { where: MAIN })
  t('it stops in the MAIN checkout (trailing slash normalised), starting no agent but settings and preflight', r.report.stoppedAt === 'preflight' && /MAIN checkout/.test(r.report.reason) && r.calls.map((c) => c.opts.label).join() === 'settings,preflight', [r.report.reason, r.calls.map((c) => c.opts.label)])
  r = await run([], { where: MAIN, args: { allowMainCheckout: true } })
  t('allowMainCheckout alone does not start a run in the main checkout', /MAIN checkout/.test(r.report.reason || ''), r.report.reason)
  r = await run([], { where: MAIN, args: { allowMainCheckout: true, resume: 3 } })
  t('...allowMainCheckout with resume does', !/MAIN checkout/.test(r.report.reason || ''), r.report.reason)
  r = await run([], { where: { gitDir: '', commonDir: '', toplevel: '' } })
  t('a preflight that did not report the paths is refused', /did not report gitDir/.test(r.report.reason || ''), r.report.reason)
  r = await run([])
  const later = afterPreflight(r.calls) || []
  t('every agent after preflight is pinned to the recorded root', r.report.stoppedAt === null && later.length > 3 && later.every((c) => /^Repo root: \/repo\/\.claude\/worktrees\/build-demo\./.test(c.prompt)), [r.report.stoppedAt, later.map((c) => c.prompt.slice(0, 60))])
  // The gate runs as a LONE command (OPS-72): settings.json's sandbox exclusion for the gate and
  // gate-wait applies only to the script run alone, so PIN's `cd <root> &&` before it ran the gate
  // sandboxed, where its process checks fail whatever the code did (PANEL-48's build, 2026-10-17).
  const chained = /(&&|;|\|)\s*(\S*\/)?gate(-wait)?\.sh|gate(-wait)?\.sh[^`\n]*(&&|;|\||>)/
  // gate-wait is named, as a LONE command and in the backgrounded-gate note, at the path the
  // project's settings give (gateWait), and at no other.
  const lone = (c, cmd, wait = 'scripts/ci/gate-wait.sh') => c.prompt.includes(`\`${cmd}\` as a LONE Bash command`) && c.prompt.includes(`\`${wait}\` as a LONE Bash command`)
    && c.prompt.includes(`call \`${wait}\` in the foreground`) && (c.prompt.match(/\S*gate-wait\.sh/g) || []).every((p) => p.replace(/^`/, '') === wait)
    && /the one exception to the `cd \/repo\/\.claude\/worktrees\/build-demo &&` rule/.test(c.prompt) && !chained.test(c.prompt)
  const gate = later.find((c) => c.opts.label === 'gate:3#1'), receipt = later.find((c) => c.opts.label === 'receipt')
  t('the quick gate agent runs `scripts/ci/gate.sh --quick` and gate-wait as LONE commands, exempt from PIN\'s cd rule, chained nowhere', !!gate && lone(gate, 'scripts/ci/gate.sh --quick'), gate && gate.prompt)
  t('the receipt agent runs `scripts/ci/gate.sh` and gate-wait the same way', !!receipt && lone(receipt, 'scripts/ci/gate.sh'), receipt && receipt.prompt)
  // A project whose template runner moved (GATE_RUN=ci/run-local.sh) has gate-wait at ci/ (OPS-72).
  r = await run([], { cfg: { gate: 'ci/gate.sh', gateWait: 'ci/gate-wait.sh' } })
  const g2 = r.calls.find((c) => c.opts.label === 'gate:3#1'), r2 = r.calls.find((c) => c.opts.label === 'receipt')
  t('with gateWait ci/gate-wait.sh, both gate agents name gate-wait there and only there', r.report.stoppedAt === null && !!g2 && !!r2 && lone(g2, 'ci/gate.sh --quick', 'ci/gate-wait.sh') && lone(r2, 'ci/gate.sh', 'ci/gate-wait.sh'), g2 && g2.prompt)
  // reportTo (OPS-43): the script cannot SendMessage, so the session it reports to rides back in
  // the report, for the session that launched it to send the outcome there.
  t('a run given no reportTo names none in its report', r.report.reportTo === null, r.report.reportTo)
  r = await run([], { args: { reportTo: ' lead ' } })
  t('a finished run hands args.reportTo back in its report, trimmed', r.report.stoppedAt === null && r.report.reportTo === 'lead', [r.report.stoppedAt, r.report.reportTo])
  r = await run([], { where: MAIN, args: { reportTo: 'lead' } })
  t('...and so does a stopped run, beside its notify line', r.report.stoppedAt === 'preflight' && r.report.reportTo === 'lead' && !!r.report.notify, [r.report.stoppedAt, r.report.reportTo, r.report.notify])
  // The budget (OPS-85). change-cost.sh reads the proposal's **Budget:** line ONE way and reports its
  // kind: a SOFT budget's overrun is reported in report.warnings and the build goes on; a hard one
  // (`$N`, or `$N, hard`) stops as before; a line it cannot read stops the run at preflight.
  const warned = (r, ...words) => r.report.warnings.some((w) => words.every((x) => w.includes(x)))
  const built = (r) => r.calls.some((c) => c.opts.label.startsWith('build:'))
  r = await run([], { budget: { budgetUsd: 10, budgetKind: 'soft', costUsd: 12 } })
  t('a SOFT budget already over at preflight warns, naming the spend, the budget and that it is soft, and the build goes on', r.report.stoppedAt === null && warned(r, '$12', '$10', 'soft'), [r.report.stoppedAt, r.report.reason, r.report.warnings])
  r = await run([], { budget: { budgetUsd: 10, budgetKind: 'soft', costUsd: 4 }, commitCost: 15 })
  t('a SOFT budget a group takes over warns the same way, and the build goes on to the receipt', r.report.stoppedAt === null && warned(r, '$15', '$10', 'soft', 'group 3') && !!r.report.receipt, [r.report.stoppedAt, r.report.reason, r.report.warnings])
  r = await run([], { budget: { budgetUsd: 10, budgetKind: 'hard', costUsd: 12 } })
  t('a HARD budget already over stops at preflight, as before', r.report.stoppedAt === 'budget' && /already \$12 against a budget of \$10/.test(r.report.reason) && !built(r), [r.report.stoppedAt, r.report.reason])
  r = await run([], { budget: { budgetUsd: 10, budgetKind: 'hard', costUsd: 4 }, commitCost: 15 })
  t('a HARD budget a group takes over stops after that group\'s commit, as before', r.report.stoppedAt === 'group 3 budget' && /\$15 spent against a budget of \$10/.test(r.report.reason) && !r.report.receipt, [r.report.stoppedAt, r.report.reason])
  r = await run([], { budget: { budgetUsd: 10, budgetKind: 'hard', costUsd: 4 }, commitCost: 9 })
  t('a HARD budget not reached neither stops nor warns', r.report.stoppedAt === null && !r.report.warnings.some((w) => /budget/i.test(w)), [r.report.stoppedAt, r.report.warnings])
  r = await run([], { budget: { budgetUsd: null, budgetKind: 'unreadable', costUsd: 4 } })
  t('a **Budget:** line change-cost.sh cannot read stops at preflight and says so, rather than being taken as hard or soft', r.report.stoppedAt === 'budget' && /cannot be read/.test(r.report.reason) && !built(r), [r.report.stoppedAt, r.report.reason])
  r = await run([], { budget: { budgetUsd: null, budgetKind: null, costUsd: null } })
  t('change-cost.sh not reporting the budget at all is a warning that no budget is enforced, not silence', r.report.stoppedAt === null && warned(r, 'NOT enforced'), [r.report.stoppedAt, r.report.warnings])
  r = await run([], { budget: { budgetUsd: 10, budgetKind: null, costUsd: 12 } })
  t('a budget whose kind was not reported is treated as hard, and the warning says so', r.report.stoppedAt === 'budget' && warned(r, '$10', 'HARD'), [r.report.stoppedAt, r.report.warnings])
  r = await run([], { budget: { budgetUsd: null, budgetKind: 'none', costUsd: 4 } })
  t('no **Budget:** line: no stop and no budget warning', r.report.stoppedAt === null && !r.report.warnings.some((w) => /budget/i.test(w)), [r.report.stoppedAt, r.report.warnings])
  // The stub above answers budgetKind itself, so what makes it ONE reading is the preflight prompt:
  // the budget comes from change-cost.sh's JSON, and the agent is told not to parse proposal.md.
  const pf = (r.calls.find((c) => c.opts.label === 'preflight') || {}).prompt || ''
  t('the preflight agent takes budgetUsd and budgetKind from change-cost.sh --json, never from proposal.md\'s **Budget:** line', /change-cost\.sh demo --json/.test(pf) && /budgetKind/.test(pf) && !/\*\*Budget:\*\*/.test(pf) && /Do not read the budget from proposal\.md/.test(pf), pf)
  console.log(out.join('\n'))
})().catch((e) => { console.log(`FAIL driver: the script threw: ${e && e.message}`) })
EOF
node "$d/driver.js" "$wf" > "$d/drv" 2>&1 || echo "FAIL driver: node exited non-zero: $(tail -3 "$d/drv")" >> "$d/drv"
grep -q . "$d/drv" || echo "FAIL driver: printed nothing" >> "$d/drv"
cat "$d/drv"; _fails=$((_fails + $(grep -c '^FAIL' "$d/drv")))
finish
