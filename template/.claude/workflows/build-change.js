export const meta = {
  name: 'build-change',
  description: 'Build an approved change group by group: the builder implements, the quick gate runs, an independent reviewer reviews, fixes repeat until severity converges, then one commit per group, a full gate receipt and the verify step',
  whenToUse: 'After the owner has approved a change (changes/<id>/ on main) and its change/<id> branch is checked out clean in its OWN worktree (the main checkout stays on main), with the session entered into that worktree and STAYING there, editing nothing, until the run returns (each agent takes the session cwd when it starts). args: {change: "<id>", groups?: [numbers], maxRounds?: 3, resume?: <group>, changesDir?: "changes", branchPrefix?: "change/", gate?: "scripts/ci/gate.sh", quickFlags?: "--quick", attribution?: "<trailer or empty>", session?: "<this session\'s id, for the Session: trailer>"}. On a stop, send the returned report.notify with PushNotification; on success, push and run merge-pr. Without the Workflow tool, use the apply-change skill instead.',
  phases: [
    { title: 'Preflight', detail: 'branch, clean tree, open task groups' },
    { title: 'Build', detail: 'the builder agent implements one group' },
    { title: 'Gate', detail: 'the quick gate on the working tree' },
    { title: 'Review', detail: 'the reviewer agent, fix until peak severity converges' },
    { title: 'Commit', detail: 'one local commit per group' },
    { title: 'Receipt', detail: 'the full gate on HEAD' },
    { title: 'Verify', detail: 'every task ticked, every scenario cited, the diff matches tasks.md' },
  ],
}

// The plan state is <changesDir>/<id>/tasks.md: builders tick it, nothing else holds progress, so a
// run stopped BETWEEN groups resumes by re-running with the same args. A run stopped MID-group
// leaves that group's work uncommitted (its tasks possibly all ticked): resume it with
// {change, resume: <n>}, which accepts the dirty tree and re-enters group n at the gate, so the work
// is still gated and reviewed. Never commit a half-group to get past preflight: its ticked tasks
// would make the group look finished and it would never be reviewed.
//
// The owner decides the proposal and design, ADRs, deploys and anything outside this repo; merging
// is NOT theirs: the session merges through merge-pr once the guard passes and review converged.
// Every stop below is an owner decision, an environment fault, a red gate, or a loop that is not
// converging. Nothing here pushes, merges or notifies: a script cannot call PushNotification, so
// every stop sets report.notify (ONE line) and the session that launched this sends it. On success
// notify stays null and the session carries on to push, PR, merge-pr. A THROWN error sets nothing:
// the session sees the throw itself and notifies as a block.

const change = args && args.change
if (!change) throw new Error('build-change needs args.change (the change id)')
const CHANGES = (args.changesDir || 'changes').replace(/\/+$/, '')
const BRANCH = `${args.branchPrefix || 'change/'}${change}`
const GATE_CMD = args.gate || 'scripts/ci/gate.sh'
const QUICK = args.quickFlags == null ? '--quick' : String(args.quickFlags)
const MAX_ROUNDS = args.maxRounds ?? 3
const RESUME = args.resume == null ? null : Number(args.resume)
const MAX_GATE_FIXES = 2

// ROLES, TIERS, ECONOMY, ECONOMY_FILE, PROVENANCE and ATTRIBUTION_DEFAULT restate
// .claude/model-roles.json, which a workflow script cannot read; .claude/checks/model-roles.sh fails
// when they disagree. The mechanical steps run a script and quote its output, so they take the
// cheap role. TIERS: the variant a role runs on for the proposal's **Risk:** tier (normal is the
// role itself). ECONOMY: the role one tier down while the panel's economy file says economy is on;
// the reviewer is never in it.
const ROLES = {
  builder: { model: "opus", effort: "high" },
  reviewer: { model: "opus", effort: "high" },
  mechanic: { model: "sonnet", effort: "low" },
};
const TIERS = {
  "builder.low": { model: "sonnet", effort: "high" },
  "builder.high": { model: "opus", effort: "xhigh" },
  "reviewer.high": { model: "opus", effort: "xhigh" },
};
const ECONOMY = {
  scribe: { model: "sonnet", effort: "medium" },
  orient: { model: "sonnet", effort: "low" },
  mechanic: { model: "haiku", effort: "low" },
};
const ECONOMY_FILE = '~/.clauductor/panel/economy.json'
const PROVENANCE = true
const ATTRIBUTION_DEFAULT = 'Co-Authored-By: Claude <noreply@anthropic.com>'
const ATTRIBUTION = args.attribution == null ? ATTRIBUTION_DEFAULT : String(args.attribution)
const SESSION = args.session ? String(args.session) : 'build-change'

// ── pure: begin ── (no agent, no state: .claude/checks/build-change.sh loads this block into node
// and falsifies it, so keep everything the loop decides with here, and nothing that calls out).
const RANK = { none: 0, low: 1, medium: 2, high: 3, critical: 4 }
// THE GRADE of one review round, from the gate and the reviewer (borrowed from Kimchi Ferment's
// per-step judge, in a coarser scale than its A–F: three grades, each with a consequence):
//   pass     the quick gate is green and the review found nothing medium or worse: commit.
//   concern  the gate is green; the worst finding is medium: fix and review again.
//   fail     the gate is red, or a finding is high or critical: fix and review again.
const gradeRound = (gatePassed, findings) => {
  if (!gatePassed) return 'fail'
  const peak = findings.reduce((p, f) => Math.max(p, RANK[f.severity] || 0), 0)
  return peak >= RANK.high ? 'fail' : peak === RANK.medium ? 'concern' : 'pass'
}
// A finding's identity across rounds: its file and the first words of its summary, normalised.
// Line numbers move under a fix, and a reworded summary still starts the same way.
const findingKey = (f) => `${f.file}|${String(f.summary || '').toLowerCase().replace(/[^a-z0-9 ]+/g, ' ').trim().split(/\s+/).slice(0, 6).join(' ')}`
// THE STUCK-LOOP BREAKER (Ferment stops after three failures; this stops at the first sign the loop
// cannot converge, instead of burning rounds to MAX_ROUNDS). `rounds` holds each round so far:
// {peak, actionable, keys (actionable findings), disputedKeys (those the builder disputed after it),
// diffHash (the reviewed tree)}. Returns the reason the latest round shows a stuck loop, or null.
//   recurring    a finding the builder was asked to fix, and did not dispute, is back unchanged;
//   not falling  the peak severity did not fall and the count of actionable findings did not either;
//   oscillating  the reviewed diff is one already reviewed: the fix changed nothing, or reverted.
// (A peak that ROSE is its own stop, checked before this.)
const stuckReason = (rounds) => {
  const n = rounds.length
  if (n < 2) return null
  const cur = rounds[n - 1], prev = rounds[n - 2]
  const disputed = new Set(prev.disputedKeys || [])
  const back = (cur.keys || []).filter((k) => (prev.keys || []).includes(k) && !disputed.has(k))
  if (back.length) return `recurring: ${back.length} finding(s) fixed in round ${n - 1} are back unchanged (${back[0].replace('|', ': ')})`
  if (cur.diffHash && rounds.slice(0, -1).some((r) => r.diffHash === cur.diffHash)) {
    const k = rounds.findIndex((r) => r.diffHash === cur.diffHash) + 1
    return `oscillating: round ${n} reviewed the same diff as round ${k} (the fix changed nothing, or reverted to it)`
  }
  if (cur.actionable > 0 && RANK[cur.peak] >= RANK[prev.peak] && cur.actionable >= prev.actionable) {
    return `not falling: peak ${prev.peak} → ${cur.peak}, actionable ${prev.actionable} → ${cur.actionable}`
  }
  return null
}
// ── pure: end ──
// `git diff` omits untracked files and a clean-room gate archives tracked files only, so a new file
// would be invisible to BOTH the gate and the reviewer, and a deleted-unstaged one breaks an
// archive. Registering them makes all three see one set.
const REGISTER = `First make the index match the working tree without staging content: for each path from \`git ls-files -z --others --exclude-standard\` run \`git add -N -- <path>\` (intent-to-add), and for each path from \`git ls-files -z --deleted\` run \`git rm -q --cached -- <path>\`. Report nothing about this step unless it fails.`
const GATE_OUTPUT = 'It prints only stage markers, failure lines and the verdict tail, and names the full log: `grep` that log for more, never read it whole.'
const NO_AGENT = 'if this is the first run since .claude/agents/ changed, the builder/reviewer types register only at SESSION START: restart Claude Code, then resume this run'

const PREFLIGHT = {
  type: 'object',
  properties: {
    branch: { type: 'string' },
    clean: { type: 'boolean' },
    gitDir: { type: 'string', description: 'verbatim output of git rev-parse --absolute-git-dir' },
    risk: { type: 'string', description: 'the value of proposal.md\'s **Risk:** line, verbatim; empty if there is none' },
    budgetUsd: { type: ['number', 'null'], description: 'the number in proposal.md\'s **Budget:** $N line; null if there is none' },
    costUsd: { type: ['number', 'null'], description: 'costUsd from change-cost.sh --json; null if it could not check' },
    economy: { type: 'boolean', description: 'true only if the economy file exists and parses with "economy": true' },
    today: { type: 'string', description: 'verbatim output of date +%Y-%m-%d' },
    commonDir: { type: 'string', description: 'verbatim output of git rev-parse --path-format=absolute --git-common-dir' },
    toplevel: { type: 'string', description: 'verbatim output of git rev-parse --show-toplevel' },
    dirtyFiles: { type: 'array', items: { type: 'string' } },
    groups: {
      type: 'array',
      items: {
        type: 'object',
        properties: { n: { type: 'number' }, title: { type: 'string' }, openTasks: { type: 'number' } },
        required: ['n', 'title', 'openTasks'],
      },
    },
  },
  required: ['branch', 'clean', 'groups', 'gitDir', 'commonDir', 'toplevel', 'risk', 'budgetUsd', 'costUsd', 'economy', 'today'],
}
const BUILD = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['done', 'design-issue', 'blocked'] },
    summary: { type: 'string' },
    tasksTicked: { type: 'array', items: { type: 'string' } },
    designIssue: { type: 'string' },
    disputed: { type: 'array', items: { type: 'string' } },
    disputedItems: { type: 'array', items: { type: 'number' }, description: 'the numbers, in the list you were given, of the findings you dispute' },
    notes: { type: 'string' },
  },
  required: ['status', 'summary'],
}
const GATE = {
  type: 'object',
  properties: {
    passed: { type: 'boolean' },
    evidence: { type: 'string', description: 'the output line(s) that show the verdict' },
    failures: { type: 'string', description: 'failing checks and first relevant error lines, <=60 lines' },
    environmentFault: { type: 'boolean', description: 'true if the tool could not run (a service or VM down), not a code failure' },
    diffHash: { type: 'string', description: 'verbatim output of: git diff HEAD | git hash-object --stdin' },
  },
  required: ['passed', 'evidence'],
}
// THE REVIEW, as the reviewer receives it: its prompt, its findings schema, the agent type and the
// spawn itself. With the reviewer agent's file and its model, this is what the reviewer IS.
// pr-merge-guard rule 13 hashes the lines between these markers, and between the review-call
// markers in the review loop (model-roles.json .evals.triggers.reviewer). An edit between them
// needs a new eval receipt; an edit anywhere else in this file does not. So everything that shapes
// the review stays inside, and checks/model-roles.sh fails if a reviewer spawn or the REVIEW schema
// appears outside. .claude/evals/run.sh READS REVIEW_PROMPT and REVIEW from here (one line each,
// JSON), so a receipt's hash of this section is a hash of what its eval actually sent.
// <review-prompt>
const REVIEW_PROMPT = "Review task group {n} (\"{title}\") of change \"{change}\" ({changes}/{change}/). The group's work is the current UNCOMMITTED working-tree diff."
const REVIEW_DISPUTED = "\n\nThe builder DISPUTES these earlier findings. For each, re-check the code: re-raise it only if the builder's reason is wrong, and say why in the finding.\n"
const REVIEW = {"type":"object","properties":{"findings":{"type":"array","items":{"type":"object","properties":{"severity":{"type":"string","enum":["critical","high","medium","low"]},"file":{"type":"string"},"line":{"type":"number"},"summary":{"type":"string"},"failure":{"type":"string","description":"concrete input/state -> wrong result"},"group":{"type":"number","description":"the task group whose code holds the defect's SOURCE (not where the symptom shows); 0 if the source predates this change"}},"required":["severity","file","summary","failure","group"]}}},"required":["findings"]}
const reviewPrompt = (g, change, disputed) =>
  REVIEW_PROMPT.replace(/\{(n|title|changes|change)\}/g, (_, k) => ({ n: String(g.n), title: g.title, changes: CHANGES, change })[k])
  + (disputed.length ? REVIEW_DISPUTED + disputed.map((d) => `- ${d}`).join('\n') : '')
// agentType last, so nothing a shared helper returns can turn the review into another agent's.
const reviewSpawn = (g, change, disputed, round) =>
  agent(PIN + reviewPrompt(g, change, disputed), { ...pick('reviewer'), label: `review:${g.n}#${round}`, phase: 'Review', schema: REVIEW, agentType: 'reviewer' })
// </review-prompt>
const COMMIT = {
  type: 'object',
  properties: {
    committed: { type: 'boolean' }, sha: { type: 'string' }, files: { type: 'array', items: { type: 'string' } }, problem: { type: 'string' },
    costUsd: { type: ['number', 'null'], description: 'costUsd from change-cost.sh --json after the commit; null if it could not check' },
  },
  required: ['committed', 'costUsd'],
}
const VERIFY = {
  type: 'object',
  properties: {
    passed: { type: 'boolean', description: 'true only if the script exited 0' },
    output: { type: 'string', description: 'the script\'s FAIL lines and its last line, verbatim' },
  },
  required: ['passed', 'output'],
}

const grades = (entry) => entry.rounds.map((r) => `R${r.round} ${r.grade}`).join(', ') || 'none'
const report = { change, stoppedAt: null, reason: null, stopKind: null, groups: [], receipt: null, verify: null, risk: null, economy: false, budgetUsd: null, costUsd: null, notify: null, warnings: [] }
let ROOT = null

// kind: 'stop' (an owner decision, an environment fault, a red gate, a limit) or 'stuck' (the
// stuck-loop breaker: the review loop shows it cannot converge). The notify line names it.
const stop = (where, reason, kind = 'stop') => {
  report.stoppedAt = where
  report.reason = reason
  report.stopKind = kind
  report.notify = `build-change ${change} ${kind === 'stuck' ? 'STUCK' : 'STOPPED'} at ${where}: ${String(reason).slice(0, 160)}`
  log(`STOP at ${where}: ${reason}; the session must send report.notify via PushNotification`)
  return report
}

// ── Preflight ──────────────────────────────────────────────────────────────────────────────────
phase('Preflight')
const pre = await agent(
  `Repo root is the cwd. Report, without changing anything:
1. \`git rev-parse --abbrev-ref HEAD\` → branch.
2. \`git status --porcelain\` → clean (true only if empty) and the dirty paths.
3. Parse ${CHANGES}/${change}/tasks.md: every heading of the form "## <n>. <title>" is a group; count its "- [ ]" lines as openTasks.
4. \`git rev-parse --absolute-git-dir\` → gitDir, \`git rev-parse --path-format=absolute --git-common-dir\` → commonDir, \`git rev-parse --show-toplevel\` → toplevel. Copy each output verbatim.
5. From ${CHANGES}/${change}/proposal.md: the value of the \`**Risk:**\` line → risk (verbatim, empty if absent); the dollars in the \`**Budget:** $N\` line → budgetUsd (null if absent).
6. \`sh .claude/change-cost.sh ${change} --json\` → its costUsd (null when it prints costUsd null or fails).
7. \`cat ${ECONOMY_FILE} 2>/dev/null\` → economy: true only if that prints JSON whose "economy" is true; false otherwise (no file included).
8. \`date +%Y-%m-%d\` → today.
Read the files; do not infer.`,
  { label: 'preflight', phase: 'Preflight', schema: PREFLIGHT, ...ROLES.mechanic },
)
if (!pre) return stop('preflight', 'preflight agent returned nothing')
if (pre.branch !== BRANCH) return stop('preflight', `on branch ${pre.branch}, expected ${BRANCH}`)
// The main checkout stays on main: every hook runs from "$CLAUDE_PROJECT_DIR", the main checkout,
// so a build holding it on a change branch leaves every worktree agent guarded by that branch's
// hooks, and worktree-hook-drift.sh then refuses every new worktree agent. The comparison is done
// HERE, on the two strings, not by trusting an agent's boolean. `allowMainCheckout` is honoured
// only with `resume`: it finishes a run already begun there, and cannot start a new one.
const norm = (p) => String(p || '').trim().replace(/\/+$/, '')
if (!norm(pre.gitDir) || !norm(pre.commonDir) || !norm(pre.toplevel)) return stop('preflight', 'preflight did not report gitDir/commonDir/toplevel')
const inMainCheckout = norm(pre.gitDir) === norm(pre.commonDir)
if (inMainCheckout && !(args.allowMainCheckout && RESUME != null)) return stop('preflight', `running in the MAIN checkout on ${pre.branch}; it must stay on main (hooks run from it). Run: git switch main here, git worktree add .claude/worktrees/build-${change} ${BRANCH}, EnterWorktree that path, re-run. Finishing a run already begun here: pass {resume: <group>, allowMainCheckout: true}`)
// EVERY LATER AGENT IS PINNED TO THIS ROOT. A workflow agent's cwd is the session's cwd when that
// agent STARTS, not when the run started: a session that leaves the worktree mid-run would send the
// next group's builder, gate and commit into whatever tree it is in by then.
ROOT = norm(pre.toplevel)
const PIN = `Repo root: ${ROOT}. Before anything else run \`cd ${ROOT} && git rev-parse --show-toplevel\`; if it does not print exactly ${ROOT}, change nothing and report that as the failure. Start every Bash command with \`cd ${ROOT} &&\`, and give Read/Edit/Write absolute paths under ${ROOT}.\n\n`
const spawn = (prompt, opts) => agent(PIN + prompt, opts)

// The risk tier the owner approved picks each role's variant; economy mode drops the roles it lists.
const RISK = pre.risk
if (!['low', 'normal', 'high'].includes(RISK)) return stop('preflight', `proposal.md's **Risk:** is '${RISK}', not low, normal or high; checks/changes.sh should have refused it`)
const ECONOMY_ON = pre.economy === true
const pick = (role) => {
  let c = TIERS[`${role}.${RISK}`] || ROLES[role]
  if (ECONOMY_ON && ECONOMY[role]) c = ECONOMY[role]
  return { model: c.model, effort: c.effort }
}
const CHEAP = pick('mechanic')
const BUILDER = { agentType: 'builder', ...pick('builder') }
const REVIEWER = pick('reviewer') // for the log line only: the review itself is spawned by reviewSpawn
Object.assign(report, { risk: RISK, economy: ECONOMY_ON, budgetUsd: pre.budgetUsd, costUsd: pre.costUsd })
log(`Risk ${RISK}: builder ${BUILDER.model}/${BUILDER.effort}, reviewer ${REVIEWER.model}/${REVIEWER.effort}, mechanic ${CHEAP.model}/${CHEAP.effort}${ECONOMY_ON ? ' (economy mode)' : ''}`)
// The budget (Shape Up's appetite): a change over it stops for the owner, who raises it or cuts scope.
const overBudget = (cost) => pre.budgetUsd != null && cost != null && cost > pre.budgetUsd
if (overBudget(pre.costUsd)) return stop('budget', `already $${pre.costUsd} against a budget of $${pre.budgetUsd}; the owner raises the budget (roadmap row and proposal.md) or cuts scope`)
if (pre.budgetUsd != null && pre.costUsd == null) report.warnings.push('budget set, but change-cost.sh could not read the cost: the budget is NOT being enforced on this run')
// Provenance trailers (model-roles.json provenance) and the attribution trailer, as ONE trailer block.
const trailers = (role) => {
  const t = PROVENANCE ? [`Change: ${change}`, `Agent-Role: ${role}`, `Model: ${pick(role).model}`, `Session: ${SESSION}`] : []
  if (ATTRIBUTION) t.push(ATTRIBUTION)
  return t.length ? `\n\n${t.join('\n')}` : ''
}
if (!pre.clean && RESUME == null) return stop('preflight', `working tree not clean: ${(pre.dirtyFiles || []).join(', ')}; the reviewer reviews the uncommitted diff, so it must start empty. If a previous run stopped mid-group, re-run with {resume: <group>}`)
if (RESUME != null && !pre.groups.some((g) => g.n === RESUME)) return stop('preflight', `resume group ${RESUME} is not a group in tasks.md`)
const groupsArg = args.groups == null ? [] : [].concat(args.groups)
const wanted = groupsArg.length ? new Set(groupsArg.map(Number)) : null
const unknown = wanted ? [...wanted].filter((n) => !pre.groups.some((g) => g.n === n)) : []
if (unknown.length) return stop('preflight', `args.groups names ${unknown.join(', ')}, which tasks.md does not have`)
// The resumed group runs even with zero open tasks: its builder may have ticked them all before the stop.
const todo = pre.groups.filter((g) => g.n === RESUME || (g.openTasks > 0 && (!wanted || wanted.has(g.n)) && (RESUME == null || g.n > RESUME)))
const skipped = pre.groups.filter((g) => g.openTasks > 0 && wanted && !wanted.has(g.n))
if (skipped.length) log(`Not running open groups ${skipped.map((g) => g.n).join(', ')} (not in args.groups)`)
const before = RESUME == null ? [] : pre.groups.filter((g) => g.n < RESUME && g.openTasks > 0)
if (before.length) log(`Not running open groups ${before.map((g) => g.n).join(', ')} (before resume group ${RESUME}); run them separately`)
if (!todo.length) return stop('preflight', 'nothing to build: no open task groups matched')

const ORCHESTRATOR_TASKS = `Tasks that call for a FULL gate run on HEAD, or a review of this group, are run by the orchestrating workflow after you: leave those unticked and do not run them.`
const LOG = `Keep ${CHANGES}/${change}/tasks.md's log current, so a resumed run continues from the file: under "## Decision log" add one line (${pre.today}, group, the choice, the alternative, why) for each decision you took that design.md does not settle; if you stop before the group is done, add a line under "## Progress" saying what is finished and what is left. A decision that would CHANGE design.md is not yours: return design-issue.`

async function gateUntilGreen(g, entry) {
  for (let attempt = 0; ; attempt++) {
    const gate = await spawn(
      `${REGISTER} Then run \`${GATE_CMD} ${QUICK}\` from the repo root in the foreground (allow up to 15 minutes). ${GATE_OUTPUT} Judge by the tool's OUTPUT, never the exit code alone: passed=true only if the output shows every step completing with zero failures. Quote the verdict line(s) as evidence. If it could not run at all (a service or VM down, killed before the steps start), set environmentFault=true. Last, run \`git diff HEAD | git hash-object --stdin\` and copy its output as diffHash.`,
      { label: `gate:${g.n}#${attempt + 1}`, phase: 'Gate', schema: GATE, ...CHEAP },
    )
    if (!gate) return 'gate agent returned nothing'
    entry.gates.push({ passed: gate.passed, evidence: gate.evidence, diffHash: gate.diffHash || '' })
    if (gate.passed) return null
    if (gate.environmentFault) return `environment fault: ${gate.evidence}`
    if (attempt >= MAX_GATE_FIXES) return `gate still red after ${MAX_GATE_FIXES} fix attempts: ${gate.failures || gate.evidence}`
    const fix = await spawn(
      `Change "${change}", task group ${g.n} ("${g.title}"). The quick gate failed on your uncommitted work:\n\n${gate.failures || gate.evidence}\n\nFix the cause at its source. Stay inside group ${g.n}. Leave the work uncommitted.`,
      { label: `gate-fix:${g.n}#${attempt + 1}`, phase: 'Gate', schema: BUILD, ...BUILDER },
    )
    if (!fix) return 'builder returned nothing while fixing the gate'
    if (fix.status !== 'done') return `${fix.status}: ${fix.designIssue || fix.summary}`
  }
}

for (const g of todo) {
  const entry = { n: g.n, title: g.title, build: null, gates: [], rounds: [], disputed: [], residual: [], commit: null }
  report.groups.push(entry)
  log(`Group ${g.n}: ${g.title} (${g.openTasks} open tasks)`)

  if (g.n === RESUME && g.openTasks === 0) {
    log(`Group ${g.n}: resuming at the gate with the uncommitted work already on disk`)
    entry.build = { status: 'resumed', summary: 'resumed mid-group; every task already ticked, build step skipped', tasksTicked: [], notes: '' }
  } else {
    // ── Build ────────────────────────────────────────────────────────────────────────────────
    const partial = g.n === RESUME ? ' A previous run stopped partway through this group: its uncommitted work is already on disk. Read it (`git diff HEAD`, `git status`) and CONTINUE from it; do not redo or discard it.' : ''
    const built = await spawn(
      `Implement task group ${g.n} ("${g.title}") of change "${change}". Load its context from ${CHANGES}/${change}/ (proposal.md, design.md, any specs/ deltas, tasks.md), then implement ONLY group ${g.n}.${partial} ${ORCHESTRATOR_TASKS} ${LOG}`,
      { label: `build:${g.n}`, phase: 'Build', schema: BUILD, ...BUILDER },
    )
    if (!built) return stop(`group ${g.n} build`, `builder returned nothing; ${NO_AGENT}`)
    entry.build = { status: built.status, summary: built.summary, tasksTicked: built.tasksTicked || [], notes: built.notes || '' }
    if (built.status !== 'done') return stop(`group ${g.n} build`, `${built.status}: ${built.designIssue || built.summary}`)
  }

  // ── Gate ─────────────────────────────────────────────────────────────────────────────────────
  let why = await gateUntilGreen(g, entry)
  if (why) return stop(`group ${g.n} gate`, why)

  // ── Review until peak severity converges ─────────────────────────────────────────────────────
  let prevPeak = null
  for (let round = 1; ; round++) {
    // The one place a round's review is taken (rule 13 hashes it with the review-prompt section).
    // <review-call>
    const rev = await reviewSpawn(g, change, entry.disputed, round)
    // </review-call>
    if (!rev) return stop(`group ${g.n} review`, `reviewer returned nothing; ${NO_AGENT}`)
    const peak = rev.findings.reduce((p, f) => (RANK[f.severity] > RANK[p] ? f.severity : p), 'none')
    const actionable = rev.findings.filter((f) => RANK[f.severity] >= RANK.medium)
    // A finding sourced in an EARLIER group's committed code, or in code that predates the change,
    // has no exit under "stay inside group N": the fixer works around it or disputes it, the
    // reviewer re-raises it because the defect is real, and rounds burn to the cap. So the boundary
    // lifts for exactly those, and the fix lands in this group's diff, where this group's review
    // checks it. A LATER group's finding keeps the boundary: building its tasks early is the creep
    // the boundary prevents.
    const outside = actionable.filter((f) => typeof f.group === 'number' && f.group < g.n)
    const grade = gradeRound(true, rev.findings)
    const last = entry.gates[entry.gates.length - 1] || {}
    entry.rounds.push({ round, grade, peak, count: rev.findings.length, actionable: actionable.length, outside: outside.length, keys: actionable.map(findingKey), disputedKeys: [], diffHash: last.diffHash || '' })
    log(`Group ${g.n} review round ${round}: ${grade}, peak ${peak}, ${actionable.length} actionable of ${rev.findings.length}`)
    if (!actionable.length) {
      entry.residual = rev.findings
      break
    }
    if (prevPeak && RANK[peak] > RANK[prevPeak]) {
      entry.residual = rev.findings
      return stop(`group ${g.n} review`, `severity ROSE ${prevPeak} → ${peak}: fixes are introducing worse defects than they remove`)
    }
    const stuck = stuckReason(entry.rounds)
    if (stuck) {
      entry.residual = rev.findings
      const disputes = entry.disputed.length ? `; builder disputes, which you may need to rule on: ${entry.disputed.join(' | ')}` : ''
      return stop(`group ${g.n} review`, `${stuck}; grades ${grades(entry)}${disputes}`, 'stuck')
    }
    if (round >= MAX_ROUNDS) {
      entry.residual = rev.findings
      const disputes = entry.disputed.length ? `; builder disputes, which you may need to rule on: ${entry.disputed.join(' | ')}` : ''
      return stop(`group ${g.n} review`, `not converged after ${MAX_ROUNDS} rounds (peak ${peak})${disputes}`)
    }
    const source = (f) => (!outside.includes(f) ? '' : f.group === 0 ? ' (source: predates this change)' : ` (source: group ${f.group}, already committed)`)
    const list = actionable.map((f, i) => `${i + 1}. [${f.severity}] ${f.file}${f.line ? `:${f.line}` : ''}${source(f)}: ${f.summary}\n   failure: ${f.failure}`).join('\n')
    const scope = outside.length
      ? `Findings marked with a source live in code group ${g.n} did not write: fix those at that source anyway, in this uncommitted diff, where this group's review will check the fix; the group boundary does not apply to them. For everything else, stay inside group ${g.n}.`
      : `Stay inside group ${g.n}.`
    const fix = await spawn(
      `Change "${change}", task group ${g.n} ("${g.title}"). An independent review of your uncommitted work found:\n\n${list}\n\nFix each at its source, or return it in \`disputed\` with the reason and its number in \`disputedItems\`. ${scope} Leave the work uncommitted.`,
      { label: `review-fix:${g.n}#${round}`, phase: 'Review', schema: BUILD, ...BUILDER },
    )
    if (!fix) return stop(`group ${g.n} review`, 'builder returned nothing while fixing findings')
    entry.disputed.push(...(fix.disputed || []))
    entry.rounds[entry.rounds.length - 1].disputedKeys = (fix.disputedItems || []).map((i) => actionable[i - 1]).filter(Boolean).map(findingKey)
    if (fix.status !== 'done') return stop(`group ${g.n} review`, `${fix.status}: ${fix.designIssue || fix.summary}`)
    why = await gateUntilGreen(g, entry)
    if (why) return stop(`group ${g.n} gate after review fixes`, why)
    prevPeak = peak
  }

  // ── Commit (local only, never push) ──────────────────────────────────────────────────────────
  const c = await spawn(
    `Commit the uncommitted work for task group ${g.n} of change "${change}". First: independent review of this group converged in ${entry.rounds.length} round(s), so if ${CHANGES}/${change}/tasks.md has a task UNDER GROUP ${g.n}'s OWN HEADING asking for a review of the group, tick it (never tick a task under another group's heading). Then append ONE line under "## Progress" in that tasks.md: "- ${pre.today} group ${g.n} (${g.title}) built and reviewed: converged in ${entry.rounds.length} round(s), peak ${(entry.rounds[entry.rounds.length - 1] || {}).peak || 'none'}; grades ${grades(entry)}". Then: ${REGISTER} Now every path the group touched is tracked, so \`git diff HEAD --name-status\` is the complete list of what will be committed: refuse and report if any path looks like a secret (.env*, *credentials*, *.pem, *.key). Stage with \`git add -u\` (tracked paths only, deletions included; never \`git add -A\` or \`.\`, never by name: a staged deletion's path no longer exists). If \`git diff HEAD\` is empty the group produced no file change: do not commit, and return committed=true with the current sha and problem="no changes". Commit message, imperative, via heredoc:\n\n${change}: task group ${g.n} — ${g.title}${trailers('builder')}\n\nDo NOT push. Report the new short sha from \`git rev-parse --short HEAD\`. Last, run \`sh .claude/change-cost.sh ${change} --json\` and report its costUsd (null if it is null or the script fails).`,
    { label: `commit:${g.n}`, phase: 'Commit', schema: COMMIT, ...CHEAP },
  )
  if (!c || !c.committed) return stop(`group ${g.n} commit`, (c && c.problem) || 'commit agent returned nothing')
  entry.commit = c.sha
  entry.costUsd = c.costUsd
  if (c.costUsd != null) report.costUsd = c.costUsd
  if (overBudget(c.costUsd)) return stop(`group ${g.n} budget`, `$${c.costUsd} spent against a budget of $${pre.budgetUsd} with group ${g.n} committed; the owner raises the budget or cuts scope, then re-run`)
}

// ── Receipt: the full gate on the committed HEAD, which is what pr-merge-guard rule 2 accepts ────
phase('Receipt')
const receipt = await spawn(
  `The receipt is keyed to the HEAD sha, so any commit AFTER the run voids it (pr-merge-guard rule 2). Order matters:
1. FIRST tick any task in ${CHANGES}/${change}/tasks.md that asks for a full gate run, and commit only that file. Skip if there is none. Its message, via heredoc:\n\n${change}: full gate receipt${trailers('mechanic')}\n
2. THEN run \`${GATE_CMD}\` with no flags (the full gate on committed HEAD) from the repo root in the foreground; allow up to 30 minutes. ${GATE_OUTPUT} Judge by OUTPUT: passed=true only if every step completes with zero failures and the output says the receipt was written and is clean. Quote the verdict line(s) as evidence.
3. If it FAILED and you ticked in step 1, untick it and commit that revert, so tasks.md never claims a run that did not pass.
Do NOT push.`,
  { label: 'receipt', phase: 'Receipt', schema: GATE, ...CHEAP },
)
report.receipt = receipt ? { passed: receipt.passed, evidence: receipt.evidence, failures: receipt.failures || '' } : null
if (!receipt || !receipt.passed) return stop('receipt', receipt ? receipt.failures || receipt.evidence : 'receipt agent returned nothing; tasks.md may carry a "full gate" tick committed before a run that never finished: check `git log -1` and untick it')

// ── Verify (D7): read-only, so the receipt stays valid ────────────────────────────────────────────
phase('Verify')
const v = await spawn(
  `Run \`sh .claude/verify-change.sh ${change}\` from the repo root. Change nothing. passed=true only if it exits 0. Quote its FAIL lines and its last line as output.`,
  { label: 'verify', phase: 'Verify', schema: VERIFY, ...CHEAP },
)
report.verify = v ? { passed: v.passed, output: v.output } : null
if (!v || !v.passed) return stop('verify', v ? v.output : 'verify agent returned nothing')
log('All groups committed; full gate green; verified. Next: push, gh pr create, merge-pr.')
return report
