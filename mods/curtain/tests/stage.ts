// Shared stubs for the Curtain tests: Claude Code's answers, recorded, so a
// test can fire the skill's events and read back what the mod did. No `gh`,
// no store file, no clock: everything here answers from memory.

import { mock } from 'claude-code/testing'

export const NOW = Date.parse('2026-10-02T12:00:00Z')
export const TOOL = 'mcp__curtain__curtain_call'

/** What `gh` reports, per test. */
export type Gh = {
  /** Whether the branch has a PR when the curtain rises. */
  hasPr: boolean
  /** The PR's state when merge-pr's result is checked. */
  prState: string
  /** When the session-close PR merged, or null when none has. */
  closeMergedAt: string | null
}

export const MERGED: Gh = { hasPr: true, prState: 'MERGED', closeMergedAt: '2026-10-02T12:09:00Z' }

export type Record = {
  status: (string | undefined)[]
  logs: string[]
  commands: string[]
  submits: string[]
  runs: string[][]
  store: Map<string, unknown>
}

function ghAnswer(argv: readonly string[], gh: Gh): { exitCode: number; stdout: string; stderr: string } {
  const args = argv.join(' ')
  if (args === 'gh pr view --json number,state') {
    return gh.hasPr
      ? { exitCode: 0, stdout: JSON.stringify({ number: 42, state: 'OPEN' }), stderr: '' }
      : { exitCode: 1, stdout: '', stderr: 'no pull requests found for branch "ops/x"' }
  }
  if (args === 'gh pr view 42 --json state') return { exitCode: 0, stdout: JSON.stringify({ state: gh.prState }), stderr: '' }
  if (args.startsWith('gh pr list --state merged')) {
    const rows = [{ number: 40, headRefName: 'ops/session-11-close', mergedAt: '2026-10-01T18:00:00Z' }]
    if (gh.closeMergedAt) rows.unshift({ number: 43, headRefName: 'ops/session-12-close', mergedAt: gh.closeMergedAt })
    return { exitCode: 0, stdout: JSON.stringify(rows), stderr: '' }
  }
  return { exitCode: 127, stdout: '', stderr: 'unexpected command in a test: ' + args }
}

// Every stub the mod's calls need. Call it before the test's first call on `$`.
export function stage(on: any, gh: Gh = MERGED, store: { [key: string]: unknown } = {}) {
  const clock = mock.clock(on, { now: NOW })
  mock.env(on, { CLAUDUCTOR_LANE: 'lane-7' })
  const rec: Record = { status: [], logs: [], commands: [], submits: [], runs: [], store: new Map(Object.entries(store)) }
  on('session.start', () => ({ cwd: '/work' }))
  on('command.register', ($: any, e: any) => ({ value: { command: e.name } }))
  on('tool.register', ($: any, e: any) => ({ value: { tool: e.name } }))
  on('ui.status', ($: any, e: any) => {
    rec.status.push(e.text)
    return { value: undefined }
  })
  on('ui.log', ($: any, e: any) => {
    rec.logs.push(e.text)
    return { value: undefined }
  })
  on('ui.blit', () => ({ value: {} }))
  on('store.get', ($: any, e: any) => ({ value: rec.store.get(e.key) }))
  on('store.set', ($: any, e: any) => {
    rec.store.set(e.key, e.value)
    return { value: undefined }
  })
  on('process.run', ($: any, e: any) => {
    rec.runs.push([...e.argv])
    return { value: ghAnswer(e.argv, gh) }
  })
  on('skill.prompt', ($: any, e: any) => ({ text: e.text }))
  on('turn.complete', () => ({ text: '' }))
  // The mod's own $.command.run (the /exit) lands here; the mod's commands never do.
  on('command.run', ($: any, e: any) => {
    rec.commands.push(e.command)
    return { text: '' }
  })
  on('prompt.submit', ($: any, e: any) => {
    rec.submits.push(e.text)
    return { text: e.text }
  })
  on('tool.call', () => ({ result: 'answered by Claude Code' }))
  on('ui.render', () => ({ type: 'Text', props: {}, children: ['drawn by Claude Code'] }))
  return { clock, rec }
}

export async function startSession($: any): Promise<void> {
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
}

export async function endTurn($: any, isAborted = false): Promise<void> {
  await $.turn.complete({
    turnId: 't1',
    answer: 'Curtain: done',
    durationMs: 1000,
    isAborted,
    reason: isAborted ? 'aborted' : 'answer',
  })
}

/** The band, as Claude Code passes it to a ui.render hook. */
export const BAND = {
  component: 'AbovePrompt',
  requestId: 'above-prompt',
  viewport: { columns: 80, rows: 40, isFullscreen: false },
  props: {
    hasSurvey: false,
    isWorking: true,
    maxRows: 20,
    bodyColumns: 80,
    scroll: { offset: 0, bodyRows: 20 },
    view: {},
  },
} as const

/** The Raster's cells as text lines. */
export function rasterLines(props: { [key: string]: unknown }): string[] {
  const columns = Number(props.columns)
  const rows = Number(props.rows)
  const binary = atob(String(props.cells))
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0))
  const words = new Uint32Array(bytes.buffer)
  const lines: string[] = []
  for (let y = 0; y < rows; y++) {
    let line = ''
    for (let x = 0; x < columns; x++) line += String.fromCodePoint(words[(y * columns + x) * 3]!)
    lines.push(line)
  }
  return lines
}

/** What the band shows on the terminal now, as text. */
export async function bandText($: any): Promise<string> {
  const ui = await $.ui.mount({ plugin: 'curtain', surface: 'terminal', ...BAND })
  const raster = await ui.find({ type: 'Raster' })
  await ui.unmount()
  return raster ? rasterLines(raster.props).join('\n') : ''
}
