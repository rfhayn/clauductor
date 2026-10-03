// Curtain: the closing show for a lane's wrap-up.
//
// The `/curtain` skill (skills/curtain/SKILL.md) does the real work in one
// turn: /merge-pr, /session-close, the lane step. This module watches it and
// draws the show in the band above the prompt: a velvet curtain coming down
// over a stage that Clawd sweeps, one stretch of travel per step, each paced by
// how long that step has taken before. When the skill gives its done signal and
// the PR is confirmed merged, the curtain closes, Clawd bows, and the mod
// submits /exit on the skill's behalf (a skill cannot type /exit). Anything
// else (a step that stopped, a turn that ended without the signal, a cancel)
// freezes the show at INTERMISSION and sends nothing.
//
// Commands: /curtain-mod plays the closing show alone (no exit);
// /curtain-mod cancel holds a running show; /curtain-mod-demo [halt] plays the
// whole show with pretend steps (no exit, no prompts, no skills).

import type { EngineInterface, Register, Timer } from 'claude-code'
import {
  DEFAULT_EXPECTED_MS,
  DEMO_ACTUAL_MS,
  DEMO_EXPECTED_MS,
  STEPS,
  approach,
  clock,
  expectedFrom,
  target,
  withSample,
} from './timing.ts'
import { type SceneInput, type ScenePhase, type Tone, buildGrid, gridRuns, packCells } from './scene.ts'

type Api = EngineInterface
type Mode = 'real' | 'demo' | 'closing'
type Verdict = { ok: true; detail?: string } | { ok: false; reason: string }

type Show = {
  mode: Mode
  labels: readonly string[]
  expected: readonly number[]
  phase: ScenePhase
  /** Index of the running step; labels.length once all have completed. */
  stage: number
  stageStart: number[]
  completedAt: (number | null)[]
  checks: (Promise<Verdict> | null)[]
  startedAt: number
  now: number
  lastTick: number
  display: number
  frame: number
  phaseFrame: number
  phaseAt: number
  haltStep?: string
  haltReason?: string
  /** The PR the curtain rose on: what merge-pr has to merge. */
  pr: number | null
  prNote?: string
  /** The `gh pr view` that records `pr`, still running or done. */
  prRecorded?: Promise<void>
  lane?: string
  /** The skill's done signal arrived and every check passed. */
  isExitArmed: boolean
  isTurnEnded: boolean
  isExitSent: boolean
  timers: Timer[]
}

const TOOL_FULL = 'mcp__curtain__curtain_call'
const RASTER_KEY = 'curtain-stage'
// About 6.7 frames a second: smooth enough for a sweep, cheap enough to leave on.
const FRAME_MS = 150
const BOW_MS = 2_600
const FIN_MS = 3_600
const CLEAR_AFTER_MS = 4_000
// GitHub's clock and ours may disagree a little about when the close PR merged.
const SKEW_MS = 120_000
const DEMO_HALT_AT_MS = 9_000
const CLOSING_EXPECTED_MS = 6_000
const CLOSING_ACTUAL_MS = 4_500

const TONE_COLORS: Partial<Record<Tone, string>> = {
  curtain: 'red',
  gold: 'yellow',
  broom: 'yellow',
  dust: 'gray',
  floor: 'yellow',
  alert: 'yellow',
}

let show: Show | null = null
let ticker: Timer | null = null
let isTicking = false
let lastSurface = ''
let mounted: { requestId: string; columns: number; rows: number; bodyColumns: number; maxRows: number } | null = null

// ---------------------------------------------------------------- the frame

function stepName(s: Show): string {
  return s.labels[Math.min(s.stage, s.labels.length - 1)] ?? 'curtain'
}

function statusLine(s: Show): string {
  const prefix = s.mode === 'demo' ? 'DEMO · ' : s.mode === 'closing' ? 'CURTAIN CALL · ' : 'CURTAIN · '
  if (s.phase === 'halted') {
    return 'INTERMISSION · ' + (s.haltStep ?? stepName(s)) + ' stopped' + (s.haltReason ? ': ' + s.haltReason : '')
  }
  if (s.phase === 'bow') return prefix + 'Clawd takes a bow'
  if (s.phase === 'fin' || s.phase === 'done') {
    if (s.mode === 'demo') return prefix + 'fin · a demo: nothing was run, nothing exits'
    if (s.mode === 'closing') return prefix + 'fin · this closing show does not exit'
    const lane = s.lane ? 'close lane ' + s.lane + ' from the panel · ' : ''
    return prefix + 'fin · ' + lane + (s.isExitArmed ? '/exit follows' : 'type /exit')
  }
  const steps = s.labels
    .map((label, i) => (i < s.stage ? '✓ ' : i === s.stage ? '▸ ' : '· ') + label)
    .join('  ')
  if (s.stage >= s.labels.length) return prefix + steps
  const elapsed = s.now - (s.stageStart[s.stage] ?? s.now)
  return prefix + steps + '   ' + clock(elapsed) + ' of ~' + clock(s.expected[s.stage] ?? 0)
}

function sceneFor(s: Show, columns: number, maxRows: number): SceneInput {
  const banner =
    s.phase === 'halted' ? ['INTERMISSION'] : s.phase === 'fin' || s.phase === 'done' ? ['~ fin ~', 'Thank you, goodnight'] : undefined
  return {
    columns,
    maxRows,
    drop: s.display,
    frame: s.frame,
    phase: s.phase,
    phaseFrame: s.phaseFrame,
    status: statusLine(s),
    ...(banner ? { banner } : {}),
  }
}

// ---------------------------------------------------------------- the show

function stopTicker(): void {
  ticker?.cancel()
  ticker = null
}

function cancelTimers(s: Show): void {
  for (const t of s.timers) t.cancel()
  s.timers = []
}

async function startShow($: Api, mode: Mode, labels: readonly string[], expected: readonly number[]): Promise<Show> {
  if (show) cancelTimers(show)
  stopTicker()
  const now = await $.clock.now()
  const s: Show = {
    mode,
    labels,
    expected,
    phase: 'running',
    stage: 0,
    stageStart: [now],
    completedAt: labels.map(() => null),
    checks: labels.map(() => null),
    startedAt: now,
    now,
    lastTick: now,
    display: 0,
    frame: 0,
    phaseFrame: 0,
    phaseAt: now,
    pr: null,
    isExitArmed: false,
    isTurnEnded: false,
    isExitSent: false,
    timers: [],
  }
  show = s
  $.ui.status(undefined)
  ticker = $.clock.every(FRAME_MS, () => {
    void tick($)
  })
  $.ui.invalidate('ui.render')
  return s
}

async function tick($: Api): Promise<void> {
  const s = show
  if (!s || isTicking) return
  isTicking = true
  try {
    const now = await $.clock.now()
    if (show !== s || s.phase === 'halted') return
    const dt = now - s.lastTick
    s.lastTick = now
    s.now = now
    s.frame += 1
    s.phaseFrame += 1
    if (s.phase === 'running') {
      const goal = s.stage >= s.labels.length ? 1 : target(s.expected, s.stage, now - (s.stageStart[s.stage] ?? now))
      s.display = approach(s.display, goal, dt)
      if (s.stage >= s.labels.length && s.display >= 1) enterPhase(s, 'bow', now)
    } else if (s.phase === 'bow' && now - s.phaseAt >= BOW_MS) {
      enterPhase(s, 'fin', now)
    } else if (s.phase === 'fin' && now - s.phaseAt >= FIN_MS) {
      enterPhase(s, 'done', now)
      if (s.mode === 'real' && s.isExitArmed && s.isTurnEnded) void exitSession($, s)
    } else if (s.phase === 'done' && now - s.phaseAt >= CLEAR_AFTER_MS) {
      // A real show waiting for its turn to end keeps the fin up until it does.
      if (!(s.mode === 'real' && s.isExitArmed && !s.isExitSent)) clearShow($)
      return
    }
    await paint($, s)
  } finally {
    isTicking = false
  }
}

function enterPhase(s: Show, phase: ScenePhase, now: number): void {
  s.phase = phase
  s.phaseAt = now
  s.phaseFrame = 0
}

// Repaint the stage. On the terminal the Raster is repainted in place with
// `$.ui.blit`, which skips the render pass; anywhere else, or when the size
// changed, the band is drawn again (and less often off the terminal).
async function paint($: Api, s: Show): Promise<void> {
  if (lastSurface === 'terminal' && mounted) {
    const grid = buildGrid(sceneFor(s, mounted.bodyColumns, mounted.maxRows))
    if (grid.columns === mounted.columns && grid.rows === mounted.rows) {
      const blitted = await $.ui.blit({
        requestId: mounted.requestId,
        key: RASTER_KEY,
        cells: packCells(grid),
        columns: grid.columns,
        rows: grid.rows,
      })
      if (!blitted.deny) return
    }
    $.ui.invalidate('ui.render')
    return
  }
  if (lastSurface === '' || s.frame % 3 === 0 || s.phaseFrame === 0) $.ui.invalidate('ui.render')
}

function halt($: Api, step: string, reason: string): void {
  const s = show
  if (!s || s.phase === 'halted' || s.phase === 'done') return
  s.phase = 'halted'
  s.haltStep = step
  s.haltReason = reason
  s.isExitArmed = false
  cancelTimers(s)
  // Frozen where it stands: no more frames, so no more CPU.
  stopTicker()
  const line = 'Curtain: INTERMISSION. ' + step + ' stopped' + (reason ? ': ' + reason : '') + '. Nothing more runs, and no /exit is sent.'
  $.ui.status('INTERMISSION: ' + step + ' stopped' + (reason ? ': ' + reason : ''))
  $.ui.log(line)
  $.ui.invalidate('ui.render')
}

function clearShow($: Api): void {
  if (show) cancelTimers(show)
  stopTicker()
  show = null
  mounted = null
  $.ui.status(undefined)
  $.ui.invalidate('ui.render')
}

async function completeStage($: Api, s: Show, i: number): Promise<void> {
  if (s.completedAt[i] != null) return
  const now = await $.clock.now()
  s.completedAt[i] = now
  s.stage = i + 1
  if (i + 1 < s.labels.length) s.stageStart[i + 1] = now
  if (s.mode === 'real') await saveSample($, STEPS[i] ?? 'lane', now - (s.stageStart[i] ?? now))
}

// ---------------------------------------------------------------- timings

async function loadExpected($: Api): Promise<number[]> {
  const out: number[] = []
  for (let i = 0; i < STEPS.length; i++) {
    let samples: unknown
    try {
      samples = await $.store.get('samples:' + STEPS[i])
    } catch {
      samples = undefined
    }
    out.push(expectedFrom(samples, DEFAULT_EXPECTED_MS[i]!))
  }
  return out
}

async function saveSample($: Api, step: string, ms: number): Promise<void> {
  // Read again right before the write: another session may have saved a run.
  try {
    const before = await $.store.get('samples:' + step)
    await $.store.set('samples:' + step, withSample(before, ms))
  } catch (err) {
    $.ui.log('Curtain: could not save the ' + step + ' timing: ' + String(err), { to: 'debug' })
  }
}

// ---------------------------------------------------------------- the checks

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

async function recordPr($: Api, s: Show): Promise<void> {
  try {
    const out = await $.process.run(['gh', 'pr', 'view', '--json', 'number,state'])
    const data = parseJson(out.stdout) as { number?: unknown } | undefined
    if (out.exitCode === 0 && typeof data?.number === 'number') s.pr = data.number
    else s.prNote = 'no PR was found for this branch when the curtain rose'
  } catch (err) {
    s.prNote = 'gh pr view could not run: ' + String(err)
  }
}

async function verifyMerged($: Api, s: Show): Promise<Verdict> {
  await s.prRecorded
  if (s.pr == null) return { ok: false, reason: s.prNote ?? 'no PR was recorded when the curtain rose' }
  try {
    const out = await $.process.run(['gh', 'pr', 'view', String(s.pr), '--json', 'state'])
    const data = parseJson(out.stdout) as { state?: unknown } | undefined
    if (out.exitCode !== 0) return { ok: false, reason: 'gh pr view ' + s.pr + ' failed: ' + out.stderr.trim().slice(0, 120) }
    if (data?.state === 'MERGED') return { ok: true, detail: '#' + s.pr + ' merged' }
    return { ok: false, reason: 'PR #' + s.pr + ' is ' + String(data?.state ?? 'in an unknown state') + ', not MERGED' }
  } catch (err) {
    return { ok: false, reason: 'gh pr view could not run: ' + String(err) }
  }
}

// session-close's last act is merging its own close PR, on a branch named
// `<BRANCH_OPS>session-<N>-close` (BRANCH_SESSION_CLOSE in project.conf). That
// merge, after the curtain rose, is the observable sign it finished.
async function verifySessionClose($: Api, s: Show): Promise<Verdict> {
  try {
    const out = await $.process.run(['gh', 'pr', 'list', '--state', 'merged', '--limit', '30', '--json', 'number,headRefName,mergedAt'])
    if (out.exitCode !== 0) return { ok: false, reason: 'gh pr list failed: ' + out.stderr.trim().slice(0, 120) }
    const rows = parseJson(out.stdout)
    const list = Array.isArray(rows) ? (rows as { number?: unknown; headRefName?: unknown; mergedAt?: unknown }[]) : []
    const close = list.find(
      (r) =>
        typeof r.headRefName === 'string' &&
        /session-\d+-close/.test(r.headRefName) &&
        typeof r.mergedAt === 'string' &&
        Date.parse(r.mergedAt) >= s.startedAt - SKEW_MS,
    )
    if (close) return { ok: true, detail: 'close PR #' + String(close.number) + ' merged' }
    return { ok: false, reason: 'no session-<N>-close PR has merged since the curtain rose' }
  } catch (err) {
    return { ok: false, reason: 'gh pr list could not run: ' + String(err) }
  }
}

async function verifyStep($: Api, s: Show, i: number): Promise<Verdict> {
  if (i === 1) return verifySessionClose($, s)
  // merge-pr's own check, and the backing check for the lane step and the exit.
  return verifyMerged($, s)
}

// Complete steps 0..i in order, each only once its check passes. Checks are
// shared, so a cue and a skill boundary that race run each check once.
async function ensureStage($: Api, s: Show, i: number): Promise<Verdict> {
  for (let j = 0; j <= i; j++) {
    if (show !== s || s.phase === 'halted') return { ok: false, reason: s.haltReason ?? 'the show was halted' }
    if (s.completedAt[j] != null) continue
    if (!s.checks[j]) s.checks[j] = verifyStep($, s, j)
    const verdict = await s.checks[j]!
    if (show !== s || s.phase === 'halted') return { ok: false, reason: s.haltReason ?? 'the show was halted' }
    if (!verdict.ok) {
      halt($, s.labels[j] ?? 'curtain', verdict.reason)
      return verdict
    }
    await completeStage($, s, j)
  }
  return { ok: true }
}

function intermissionText(s: Show): string {
  return (
    'Curtain: INTERMISSION. ' +
    (s.haltStep ?? stepName(s)) +
    ' stopped' +
    (s.haltReason ? ': ' + s.haltReason : '') +
    '. Stop here and report what stopped. Do not call curtain_call again; the session stays open.'
  )
}

async function exitSession($: Api, s: Show): Promise<void> {
  if (s.isExitSent || s.mode !== 'real' || !s.isExitArmed) return
  s.isExitSent = true
  try {
    await $.command.run({ command: 'exit' })
  } catch (err) {
    $.ui.status('Curtain: type /exit to leave')
    $.ui.log('Curtain: could not submit /exit (' + String(err) + '). Type /exit to leave.')
  }
}

// ---------------------------------------------------------------- the hooks

export const register: Register = (on) => {
  on('session.start', async ($, e, next) => {
    try {
      await $.tool.register({
        name: 'curtain_call',
        description:
          "The Curtain mod's done signal for the /curtain skill. Call it ONLY when the curtain skill tells you to: " +
          'cue "lane" once session-close has succeeded, and cue "exit" as the skill\'s last step. ' +
          'Its result says whether to go on or stop.',
        inputSchema: {
          type: 'object',
          properties: { cue: { type: 'string', enum: ['lane', 'exit'] } },
          required: ['cue'],
        },
      })
      await $.command.register({
        name: 'curtain-mod',
        description: 'Play the closing show (no exit), or hold a running one: /curtain-mod cancel',
        argumentHint: '[cancel]',
        immediate: true,
      })
      await $.command.register({
        name: 'curtain-mod-demo',
        description: 'Watch the whole curtain show with pretend steps; nothing runs, nothing exits',
        argumentHint: '[halt]',
        immediate: true,
      })
    } catch (err) {
      $.ui.log('Curtain: could not register its tool or commands: ' + String(err))
    }
    return next(e)
  })

  // Keep the done signal in the model's tool list, not behind ToolSearch, so
  // the skill's instruction to call it lands on a tool Claude can see.
  on('tool.describe', { tool: 'mcp__curtain__curtain_call' }, async ($, e, next) => {
    const described = await next(e)
    return { ...described, isDeferred: false }
  })

  // The curtain rises when the /curtain skill is expanded (typed, or via the Skill tool).
  on('skill.prompt', { skill: /^(curtain:)?curtain$/ }, async ($, e, next) => {
    const running = show && show.mode === 'real' && show.phase !== 'halted' && show.phase !== 'done'
    if (!running) {
      const expected = await loadExpected($)
      const s = await startShow($, 'real', STEPS, expected)
      s.lane = (await $.env.get('CLAUDUCTOR_LANE')) || undefined
      // Off the skill's path: the turn starts while gh answers.
      s.prRecorded = recordPr($, s)
    }
    return next(e)
  })

  // session-close starting means merge-pr is behind us: confirm it, then cross.
  on('skill.prompt', { skill: /(^|:)session-close$/ }, async ($, e, next) => {
    const s = show
    if (s && s.mode === 'real' && s.phase === 'running' && s.completedAt[0] == null) void ensureStage($, s, 0)
    return next(e)
  })

  on('tool.call', { tool: 'mcp__curtain__curtain_call' }, async ($, e) => {
    const cue = (e as unknown as { cue?: unknown }).cue
    const s = show
    if (!s || s.mode !== 'real') {
      return { result: 'Curtain: no /curtain show is running in this session, so the mod will not exit it. Tell the user to type /exit.' }
    }
    if (s.phase === 'halted') return { result: intermissionText(s) }
    if (cue === 'lane') {
      const verdict = await ensureStage($, s, 1)
      return { result: verdict.ok ? 'Curtain: merge-pr and session-close confirmed. Go on to the lane step.' : intermissionText(s) }
    }
    if (cue === 'exit') {
      const verdict = await ensureStage($, s, 2)
      if (!verdict.ok) return { result: intermissionText(s) }
      s.isExitArmed = true
      return {
        result:
          'Curtain: all three steps confirmed. End your turn now with a one-line summary. ' +
          'The curtain mod closes the curtain, plays the bow, and then submits /exit for you.',
      }
    }
    return { result: 'Curtain: unknown cue ' + JSON.stringify(cue) + '; use "lane" or "exit".' }
  })

  // The main loop's turn ending: the exit goes ahead only if the signal came.
  on('turn.complete', async ($, e, next) => {
    const s = show
    if (!e.agentId && s && s.mode === 'real') {
      s.isTurnEnded = true
      if (s.isExitArmed) {
        if (s.phase === 'done') void exitSession($, s)
      } else if (s.phase !== 'halted' && s.phase !== 'done') {
        const why = e.isAborted
          ? 'the turn was interrupted'
          : e.reason === 'error'
            ? 'the turn ended on an API error'
            : e.reason === 'refusal'
              ? 'the model refused'
              : 'the turn ended before /curtain gave its done signal'
        halt($, stepName(s), why)
      }
    }
    return next(e)
  })

  on('command.run', { command: 'curtain-mod' }, async ($, e) => {
    const arg = e.args.trim()
    const s = show
    if (arg === 'cancel') {
      if (!s) return { text: 'No curtain show is running.' }
      if (s.phase === 'halted' || s.phase === 'done') {
        clearShow($)
        return { text: 'Curtain cleared.' }
      }
      halt($, stepName(s), 'cancelled with /curtain-mod cancel')
      return { text: 'Curtain held at INTERMISSION. Nothing more runs, and no /exit is sent. /curtain-mod cancel again clears it.' }
    }
    if (arg !== '') return { text: 'Usage: /curtain-mod (play the closing show) or /curtain-mod cancel.' }
    if (s && s.mode === 'real' && s.phase !== 'halted' && s.phase !== 'done') {
      return { text: 'A /curtain show is in progress (' + stepName(s) + '). /curtain-mod cancel holds it.' }
    }
    const closing = await startShow($, 'closing', ['curtain call'], [CLOSING_EXPECTED_MS])
    closing.timers.push(
      $.clock.after(CLOSING_ACTUAL_MS, () => {
        if (show === closing && closing.phase === 'running') void completeStage($, closing, 0)
      }),
    )
    return { text: 'Curtain call: the closing show plays above the prompt. It does not exit (the /curtain skill does).' }
  })

  on('command.run', { command: 'curtain-mod-demo' }, async ($, e) => {
    const s = show
    if (s && s.mode === 'real' && s.phase !== 'halted' && s.phase !== 'done') {
      return { text: 'A real /curtain show is in progress; the demo waits for it.' }
    }
    const isHalt = e.args.trim() === 'halt'
    const demo = await startShow($, 'demo', STEPS, DEMO_EXPECTED_MS)
    if (isHalt) {
      demo.timers.push(
        $.clock.after(DEMO_HALT_AT_MS, () => {
          if (show === demo) halt($, STEPS[0], 'review did not converge (pretend: /curtain-mod-demo halt)')
        }),
      )
    } else {
      let at = 0
      DEMO_ACTUAL_MS.forEach((ms, i) => {
        at += ms
        demo.timers.push(
          $.clock.after(at, () => {
            if (show === demo && demo.phase === 'running') void completeStage($, demo, i)
          }),
        )
      })
    }
    return {
      text:
        'Curtain demo: pretend steps of ' +
        DEMO_ACTUAL_MS.map((ms) => Math.round(ms / 1000) + ' s').join(', ') +
        (isHalt ? ', halting in the first' : '') +
        '. Nothing is run, nothing is submitted, nothing exits.',
    }
  })

  // The band above the prompt: the stage while a show is on, nothing otherwise.
  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const s = show
    if (!s) return next(e)
    const props = e.props as { bodyColumns?: number; maxRows?: number }
    const bodyColumns = props.bodyColumns ?? e.viewport?.columns ?? 80
    const maxRows = props.maxRows ?? 12
    const grid = buildGrid(sceneFor(s, bodyColumns, maxRows))
    lastSurface = e.surface
    if (e.surface === 'terminal') {
      const { Box, Raster } = $.ui.resolve(e)
      mounted = { requestId: String(e.requestId), columns: grid.columns, rows: grid.rows, bodyColumns, maxRows }
      return Box({
        flexDirection: 'column',
        children: [Raster({ key: RASTER_KEY, columns: grid.columns, rows: grid.rows, cells: packCells(grid) })],
      })
    }
    mounted = null
    const { Box, Text } = $.ui.resolve(e)
    return Box({
      flexDirection: 'column',
      children: gridRuns(grid).map((runs) =>
        Box({
          flexDirection: 'row',
          children: runs.map((run) => {
            const color = TONE_COLORS[run.tone]
            return color ? Text({ color, children: [run.text] }) : Text({ children: [run.text] })
          }),
        }),
      ),
    })
  })
}
