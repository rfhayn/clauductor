export const meta = {
  name: 'build-change',
  description: 'Build an approved change group by group: the builder implements, the quick gate runs, an independent reviewer reviews, fixes repeat until severity converges, then one commit per group and a full gate receipt',
  whenToUse: 'After the owner has approved a change (changes/<id>/ on main) and its change/<id> branch is checked out clean in its OWN worktree (the main checkout stays on main), with the session entered into that worktree and STAYING there, editing nothing, until the run returns (each agent takes the session cwd when it starts). args: {change: "<id>", groups?: [numbers], maxRounds?: 3, resume?: <group>, changesDir?: "changes", branchPrefix?: "change/", gate?: "scripts/ci/gate.sh", quickFlags?: "--quick", attribution?: "<trailer or empty>"}. On a stop, send the returned report.notify with PushNotification; on success, push and run merge-pr. Without the Workflow tool, use the apply-change skill instead.',
  phases: [
    { title: 'Preflight', detail: 'branch, clean tree, open task groups' },
    { title: 'Build', detail: 'the builder agent implements one group' },
    { title: 'Gate', detail: 'the quick gate on the working tree' },
    { title: 'Review', detail: 'the reviewer agent, fix until peak severity converges' },
    { title: 'Commit', detail: 'one local commit per group' },
    { title: 'Receipt', detail: 'the full gate on HEAD' },
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

// ROLES and ATTRIBUTION_DEFAULT restate .claude/model-roles.json, which a workflow script cannot
// read; .claude/checks/model-roles.sh fails when they disagree. The mechanical steps run a script
// and quote its output, so they take the cheap role.
const ROLES = {
  mechanic: { model: "sonnet", effort: "low" },
};
const ATTRIBUTION_DEFAULT = 'Co-Authored-By: Claude <noreply@anthropic.com>'
const ATTRIBUTION = args.attribution == null ? ATTRIBUTION_DEFAULT : String(args.attribution)
const TRAILER = ATTRIBUTION ? `\n\n${ATTRIBUTION}` : ''
const CHEAP = ROLES.mechanic

const RANK = { none: 0, low: 1, medium: 2, high: 3, critical: 4 }
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
  required: ['branch', 'clean', 'groups', 'gitDir', 'commonDir', 'toplevel'],
}
const BUILD = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['done', 'design-issue', 'blocked'] },
    summary: { type: 'string' },
    tasksTicked: { type: 'array', items: { type: 'string' } },
    designIssue: { type: 'string' },
    disputed: { type: 'array', items: { type: 'string' } },
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
  },
  required: ['passed', 'evidence'],
}
const REVIEW = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          severity: { type: 'string', enum: ['critical', 'high', 'medium', 'low'] },
          file: { type: 'string' },
          line: { type: 'number' },
          summary: { type: 'string' },
          failure: { type: 'string', description: 'concrete input/state -> wrong result' },
          group: { type: 'number', description: "the task group whose code holds the defect's SOURCE (not where the symptom shows); 0 if the source predates this change" },
        },
        required: ['severity', 'file', 'summary', 'failure', 'group'],
      },
    },
  },
  required: ['findings'],
}
const COMMIT = {
  type: 'object',
  properties: { committed: { type: 'boolean' }, sha: { type: 'string' }, files: { type: 'array', items: { type: 'string' } }, problem: { type: 'string' } },
  required: ['committed'],
}

const report = { change, stoppedAt: null, reason: null, groups: [], receipt: null, notify: null, warnings: [] }
let ROOT = null

const stop = (where, reason) => {
  report.stoppedAt = where
  report.reason = reason
  report.notify = `build-change ${change} STOPPED at ${where}: ${String(reason).slice(0, 160)}`
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
Read the files; do not infer.`,
  { label: 'preflight', phase: 'Preflight', schema: PREFLIGHT, ...CHEAP },
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

async function gateUntilGreen(g, entry) {
  for (let attempt = 0; ; attempt++) {
    const gate = await spawn(
      `${REGISTER} Then run \`${GATE_CMD} ${QUICK}\` from the repo root in the foreground (allow up to 15 minutes). ${GATE_OUTPUT} Judge by the tool's OUTPUT, never the exit code alone: passed=true only if the output shows every step completing with zero failures. Quote the verdict line(s) as evidence. If it could not run at all (a service or VM down, killed before the steps start), set environmentFault=true.`,
      { label: `gate:${g.n}#${attempt + 1}`, phase: 'Gate', schema: GATE, ...CHEAP },
    )
    if (!gate) return 'gate agent returned nothing'
    entry.gates.push({ passed: gate.passed, evidence: gate.evidence })
    if (gate.passed) return null
    if (gate.environmentFault) return `environment fault: ${gate.evidence}`
    if (attempt >= MAX_GATE_FIXES) return `gate still red after ${MAX_GATE_FIXES} fix attempts: ${gate.failures || gate.evidence}`
    const fix = await spawn(
      `Change "${change}", task group ${g.n} ("${g.title}"). The quick gate failed on your uncommitted work:\n\n${gate.failures || gate.evidence}\n\nFix the cause at its source. Stay inside group ${g.n}. Leave the work uncommitted.`,
      { label: `gate-fix:${g.n}#${attempt + 1}`, phase: 'Gate', schema: BUILD, agentType: 'clauductor:builder' },
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
      `Implement task group ${g.n} ("${g.title}") of change "${change}". Load its context from ${CHANGES}/${change}/ (proposal.md, design.md, any specs/ deltas, tasks.md), then implement ONLY group ${g.n}.${partial} ${ORCHESTRATOR_TASKS}`,
      { label: `build:${g.n}`, phase: 'Build', schema: BUILD, agentType: 'clauductor:builder' },
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
    const rev = await spawn(
      `Review task group ${g.n} ("${g.title}") of change "${change}" (${CHANGES}/${change}/). The group's work is the current UNCOMMITTED working-tree diff.${entry.disputed.length ? `\n\nThe builder DISPUTES these earlier findings. For each, re-check the code: re-raise it only if the builder's reason is wrong, and say why in the finding.\n${entry.disputed.map((d) => `- ${d}`).join('\n')}` : ''}`,
      { label: `review:${g.n}#${round}`, phase: 'Review', schema: REVIEW, agentType: 'clauductor:reviewer' },
    )
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
    entry.rounds.push({ round, peak, count: rev.findings.length, actionable: actionable.length, outside: outside.length })
    log(`Group ${g.n} review round ${round}: peak ${peak}, ${actionable.length} actionable of ${rev.findings.length}`)
    if (!actionable.length) {
      entry.residual = rev.findings
      break
    }
    if (prevPeak && RANK[peak] > RANK[prevPeak]) {
      entry.residual = rev.findings
      return stop(`group ${g.n} review`, `severity ROSE ${prevPeak} → ${peak}: fixes are introducing worse defects than they remove`)
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
      `Change "${change}", task group ${g.n} ("${g.title}"). An independent review of your uncommitted work found:\n\n${list}\n\nFix each at its source, or return it in \`disputed\` with the reason. ${scope} Leave the work uncommitted.`,
      { label: `review-fix:${g.n}#${round}`, phase: 'Review', schema: BUILD, agentType: 'clauductor:builder' },
    )
    if (!fix) return stop(`group ${g.n} review`, 'builder returned nothing while fixing findings')
    entry.disputed.push(...(fix.disputed || []))
    if (fix.status !== 'done') return stop(`group ${g.n} review`, `${fix.status}: ${fix.designIssue || fix.summary}`)
    why = await gateUntilGreen(g, entry)
    if (why) return stop(`group ${g.n} gate after review fixes`, why)
    prevPeak = peak
  }

  // ── Commit (local only, never push) ──────────────────────────────────────────────────────────
  const c = await spawn(
    `Commit the uncommitted work for task group ${g.n} of change "${change}". First: independent review of this group converged in ${entry.rounds.length} round(s), so if ${CHANGES}/${change}/tasks.md has a task UNDER GROUP ${g.n}'s OWN HEADING asking for a review of the group, tick it (never tick a task under another group's heading). Then: ${REGISTER} Now every path the group touched is tracked, so \`git diff HEAD --name-status\` is the complete list of what will be committed: refuse and report if any path looks like a secret (.env*, *credentials*, *.pem, *.key). Stage with \`git add -u\` (tracked paths only, deletions included; never \`git add -A\` or \`.\`, never by name: a staged deletion's path no longer exists). If \`git diff HEAD\` is empty the group produced no file change: do not commit, and return committed=true with the current sha and problem="no changes". Commit message, imperative, via heredoc:\n\n${change}: task group ${g.n} — ${g.title}${TRAILER}\n\nDo NOT push. Report the new short sha from \`git rev-parse --short HEAD\`.`,
    { label: `commit:${g.n}`, phase: 'Commit', schema: COMMIT, ...CHEAP },
  )
  if (!c || !c.committed) return stop(`group ${g.n} commit`, (c && c.problem) || 'commit agent returned nothing')
  entry.commit = c.sha
}

// ── Receipt: the full gate on the committed HEAD, which is what pr-merge-guard rule 2 accepts ────
phase('Receipt')
const receipt = await spawn(
  `The receipt is keyed to the HEAD sha, so any commit AFTER the run voids it (pr-merge-guard rule 2). Order matters:
1. FIRST tick any task in ${CHANGES}/${change}/tasks.md that asks for a full gate run, and commit only that file ("${change}: full gate receipt"${ATTRIBUTION ? ` + blank line + "${ATTRIBUTION}"` : ''}). Skip if there is none.
2. THEN run \`${GATE_CMD}\` with no flags (the full gate on committed HEAD) from the repo root in the foreground; allow up to 30 minutes. ${GATE_OUTPUT} Judge by OUTPUT: passed=true only if every step completes with zero failures and the output says the receipt was written and is clean. Quote the verdict line(s) as evidence.
3. If it FAILED and you ticked in step 1, untick it and commit that revert, so tasks.md never claims a run that did not pass.
Do NOT push.`,
  { label: 'receipt', phase: 'Receipt', schema: GATE, ...CHEAP },
)
report.receipt = receipt ? { passed: receipt.passed, evidence: receipt.evidence, failures: receipt.failures || '' } : null
if (!receipt || !receipt.passed) return stop('receipt', receipt ? receipt.failures || receipt.evidence : 'receipt agent returned nothing; tasks.md may carry a "full gate" tick committed before a run that never finished: check `git log -1` and untick it')
log('All groups committed; full gate green. Next: push, gh pr create, merge-pr.')
return report
