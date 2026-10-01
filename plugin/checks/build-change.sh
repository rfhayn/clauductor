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
  printf 'BRANCH_CHANGE="feature/"\nCHANGES_DIR="openspec/changes"\nGATE="infra/ci/gate.sh"\n' > "$d/p/.claude/project.conf"
  printf '{"attribution": {"enabled": false, "trailer": "Co-Authored-By: X <x@y>"}, "provenance": {"enabled": false}}\n' > "$d/p/.claude/model-roles.json"
  ROOT= sh "$d/p/.claude/project-config.sh" --json > "$d/off.json" 2>&1
  printf '{"attribution": {"enabled": true, "trailer": "Co-Authored-By: X <x@y>"}, "provenance": {"enabled": true}}\n' > "$d/p/.claude/model-roles.json"
  ROOT= sh "$d/p/.claude/project-config.sh" --json > "$d/on.json" 2>&1
  [ "$(ROOT= sh "$d/p/.claude/project-config.sh" branch change add-x)" = "feature/add-x" ] && ok "project-config.sh branch change add-x follows BRANCH_CHANGE" || fail "project-config.sh branch does not follow BRANCH_CHANGE"
  jq -e '.branch.change == "feature/" and .changesDir == "openspec/changes" and .gate == "infra/ci/gate.sh" and .attribution == "" and .provenance == false' "$d/off.json" >/dev/null \
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
  const settings = { output: JSON.stringify({ branch: { change: 'change/' }, changesDir: 'changes', gate: 'scripts/ci/gate.sh', attribution: '', provenance: false }) }
  const pre = { branch: 'change/x', clean: true, dirtyFiles: [], groups: [{ n: 1, title: 't', openTasks: 1 }],
    gitDir: '/r/.git/worktrees/x', commonDir: '/r/.git', toplevel: '/r', risk, budgetUsd: null, costUsd: null, economy: econ, today: '2026-01-01' }
  const agent = async (p, o) => (o && o.label === 'preflight' ? pre : o && o.label === 'settings' ? settings : null)
  const body = `return (async () => {\n${prefix}\nreturn { __ran: true, pick }\n})()`
  return new Function('args', 'agent', 'phase', 'log', body)({ change: 'x' }, agent, () => {}, () => {})
}
;(async () => { try {
  const want = JSON.parse(wantJson)
  for (const risk of ['low', 'normal', 'high']) for (const econ of [false, true]) {
    const r = await prefixRun(risk, econ)
    if (!r || !r.__ran) { t(`the script up to the review loop runs (Risk ${risk})`, false, `it stopped early: ${JSON.stringify(r && (r.reason || r))}`); continue }
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
async function run(round1, { where = WORKTREE, args = {} } = {}) {
  const calls = []
  const reviews = [{ findings: round1 }, { findings: [] }]
  let h = 0
  const agent = async (prompt, opts) => {
    calls.push({ prompt, opts })
    const { label } = opts
    if (label === 'settings') return { output: JSON.stringify({ branch: { change: 'change/' }, changesDir: 'changes', gate: 'scripts/ci/gate.sh', attribution: '', provenance: false }) }
    if (label === 'preflight') return { branch: 'change/demo', clean: true, ...where, risk: 'normal', budgetUsd: null, costUsd: null, economy: false, today: '2026-10-01', gate: '', quickFlags: '',
      groups: [{ n: 1, title: 'Substrate', openTasks: 0 }, { n: 2, title: 'Service and API', openTasks: 0 }, { n: 3, title: 'Screens', openTasks: 2 }, { n: 4, title: 'Close-out', openTasks: 0 }] }
    if (label.startsWith('build:') || label.startsWith('review-fix:') || label.startsWith('gate-fix:')) return { status: 'done', summary: 'ok', disputed: [] }
    if (label.startsWith('gate:') || label === 'receipt') return { passed: true, evidence: 'ok', diffHash: `h${++h}` }
    if (label.startsWith('review:')) return reviews.shift()
    if (label.startsWith('commit:')) return { committed: true, sha: 'abc1234', costUsd: null }
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
  console.log(out.join('\n'))
})().catch((e) => { console.log(`FAIL driver: the script threw: ${e && e.message}`) })
EOF
node "$d/driver.js" "$wf" > "$d/drv" 2>&1 || echo "FAIL driver: node exited non-zero: $(tail -3 "$d/drv")" >> "$d/drv"
grep -q . "$d/drv" || echo "FAIL driver: printed nothing" >> "$d/drv"
cat "$d/drv"; _fails=$((_fails + $(grep -c '^FAIL' "$d/drv")))
finish
