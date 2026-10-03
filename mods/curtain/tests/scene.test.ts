// The stage in half-block pixel art: sizes and their rows, the crew and props
// thinning as progress rises, nothing clipped at any width, nobody walking
// through anything, the walk cycle, the bow only in its phase, and only
// characters and colours that survive tmux inside xterm.js.

import { expect, test } from 'claude-code/testing'
import {
  DEFAULT_COLOR,
  ROWS,
  STAGE_ROWS,
  type SceneInput,
  type StageSize,
  WALK,
  buildGrid,
  cast,
  census,
  crewFor,
  layout,
  packCells,
  propsFor,
  runningDrop,
} from '../hooks/scene.js'

const BASE: SceneInput = {
  columns: 80,
  maxRows: 40,
  size: 'small',
  progress: 0,
  fall: 0,
  frame: 3,
  phase: 'running',
  phaseFrame: 0,
  status: '1/3 merge-pr 3:12 / ~8m',
}
const SIZES: readonly StageSize[] = ['small', 'medium', 'full']
const PHASES = ['running', 'finale', 'bow', 'fin', 'halted'] as const
const crewOf = (input: SceneInput) => buildGrid(input).placements.filter((s) => s.kind === 'crew')

test('each size has its rows, and a band too small for it steps down rather than clip', async () => {
  expect(ROWS).toEqual({ bar: 2, small: 10, medium: 12, full: 16 })
  expect(layout(80, 40, 'small')).toEqual({ columns: 80, rows: 10, layout: 'small' })
  expect(layout(80, 40, 'medium').rows).toBe(12)
  expect(layout(80, 40, 'full').rows).toBe(16)
  expect(layout(80, 16, 'full').layout).toBe('medium')
  expect(layout(80, 12, 'full').layout).toBe('small')
  expect(layout(80, 10, 'small').layout).toBe('bar')
  expect(layout(29, 40, 'full').layout).toBe('bar')
  expect(layout(40, 40, 'full').layout).toBe('full')
  for (const size of SIZES) expect(buildGrid({ ...BASE, size }).rows).toBe(ROWS[size])
})

test('the crew and props start as wide as the stage allows and only ever thin out', async () => {
  expect(census(cast(80), 0)).toEqual({ crew: 3, props: 4 })
  expect(census(cast(60), 0)).toEqual({ crew: 2, props: 3 })
  expect(census(cast(40), 0)).toEqual({ crew: 1, props: 0 })
  expect(census(cast(140), 0).props).toBe(8)
  for (const columns of [140, 120, 96, 80, 70, 60, 50, 40, 30]) {
    const c = cast(columns)
    expect(census(c, 0)).toEqual({ crew: crewFor(columns), props: propsFor(columns) })
    let before = census(c, 0)
    for (let p = 0; p < 1; p += 0.001) {
      const now = census(c, p)
      expect(now.crew <= before.crew && now.props <= before.props, columns + ' columns at ' + p.toFixed(3)).toBe(true)
      before = now
    }
  }
})

test('the drawing matches the census, and the stage is never clear before the work completes', async () => {
  for (const columns of [120, 80, 60, 40]) {
    for (let p = 0; p < 1; p += 0.01) {
      const grid = buildGrid({ ...BASE, columns, progress: p })
      const drawn = grid.placements.filter((s) => s.kind === 'crew').length
      expect(drawn, columns + ' columns at ' + p.toFixed(2)).toBe(census(cast(columns), p).crew)
      expect(drawn >= 1).toBe(true)
    }
    // At the end, one Clawd sweeps the last spot.
    expect(census(cast(columns), 0.95)).toEqual({ crew: 1, props: 0 })
    const end = buildGrid({ ...BASE, columns, progress: 0.95 })
    expect(end.placements.filter((s) => s.kind === 'crew').map((s) => s.pose)).toEqual(['sweep'])
    expect(end.placements.some((s) => s.kind === 'dust')).toBe(true)
  }
})

test('no sprite is clipped at 80, 60 or 40 columns, in any size, phase or frame', async () => {
  for (const size of SIZES) {
    for (const columns of [80, 60, 40]) {
      for (const phase of PHASES) {
        for (let p = 0; p <= 1; p += 0.05) {
          for (const frame of [0, 1, 2, 7, 16]) {
            const grid = buildGrid({ ...BASE, size, columns, phase, progress: p, fall: p, frame, phaseFrame: frame })
            expect(grid.rows < BASE.maxRows).toBe(true)
            for (const s of grid.placements) {
              const where = size + ' ' + columns + ' ' + phase + ' p=' + p.toFixed(2) + ' ' + JSON.stringify(s)
              expect(s.x >= 0 && s.y >= 0 && s.x + s.w <= grid.columns && s.y + s.h <= grid.rows, where).toBe(true)
              // In pixels too: inside the stage, above the status row.
              expect(s.py >= 0 && s.py + s.ph <= (grid.rows - 1) * 2, where).toBe(true)
            }
          }
        }
      }
    }
  }
})

test('no Clawd walks through a prop or another Clawd, and carried props ride above their carrier', async () => {
  for (const size of SIZES) {
    for (const columns of [120, 80, 60]) {
      for (let p = 0; p < 1; p += 0.004) {
        const things = buildGrid({ ...BASE, size, columns, progress: p }).placements.filter((s) => s.kind === 'crew' || s.kind === 'prop')
        for (let i = 0; i < things.length; i++) {
          for (let j = i + 1; j < things.length; j++) {
            const a = things[i]!
            const b = things[j]!
            const across = Math.min(a.px + a.pw, b.px + b.pw) - Math.max(a.px, b.px)
            const down = Math.min(a.py + a.ph, b.py + b.ph) - Math.max(a.py, b.py)
            expect(across <= 0 || down <= 0, size + ' ' + columns + ' p=' + p.toFixed(3) + ' ' + JSON.stringify([a, b])).toBe(true)
          }
        }
      }
    }
  }
  // Somewhere along the way a Clawd carries a prop: it sits on his raised hands.
  let carried = 0
  for (let p = 0; p < 0.9; p += 0.004) {
    const grid = buildGrid({ ...BASE, progress: p })
    for (const crew of grid.placements.filter((s) => s.pose?.startsWith('carry'))) {
      const above = grid.placements.find((s) => s.kind === 'prop' && s.py + s.ph === crew.py && s.px >= crew.px && s.px + s.pw <= crew.px + crew.pw)
      expect(above, 'carry at ' + p.toFixed(3)).toBeDefined()
      carried++
    }
  }
  expect(carried > 10).toBe(true)
})

test('the walk cycle alternates: one pair of legs down while the other lifts', async () => {
  const down = (art: readonly string[]) => new Set([...art[art.length - 1]!].flatMap((ch, i) => (ch === '.' ? [] : [i])))
  const a = down(WALK[0]!)
  const b = down(WALK[1]!)
  expect(a.size).toBe(2)
  expect(b.size).toBe(2)
  expect([...a].some((i) => b.has(i))).toBe(false)
  // A walking Clawd changes frame on every tick, and comes back to it on the next.
  let checked = 0
  for (let p = 0.02; p < 0.9 && checked < 20; p += 0.01) {
    const at = (frame: number) => crewOf({ ...BASE, progress: p, frame }).map((s) => s.pose)
    const [f0, f1, f2] = [at(4), at(5), at(6)]
    f0.forEach((pose, i) => {
      if (!pose || !/^(walk|carry)\d$/.test(pose)) return
      expect(f1[i]).not.toBe(pose)
      expect(f2[i]).toBe(pose)
      checked++
    })
  }
  expect(checked > 0).toBe(true)
})

test('the curtain never covers the crew or a carried prop while the work runs', async () => {
  for (const size of SIZES) {
    for (const p of [0.5, 0.9, 0.999]) {
      const covered = Math.round(runningDrop(STAGE_ROWS[size], p) * STAGE_ROWS[size] * 2)
      for (const s of buildGrid({ ...BASE, size, progress: p }).placements) {
        expect(s.py >= 2 + covered, size + ' ' + p + ' ' + JSON.stringify(s)).toBe(true)
      }
    }
  }
})

test('the bow happens only in the bow phase, never on a halt', async () => {
  for (const phase of PHASES) {
    for (let f = 0; f < 20; f++) {
      const bows = buildGrid({ ...BASE, phase, progress: 0.97, fall: 1, phaseFrame: f, frame: f }).placements.filter((s) => s.kind === 'bow')
      if (phase !== 'bow') expect(bows.length, phase + ' frame ' + f).toBe(0)
    }
  }
  expect(buildGrid({ ...BASE, phase: 'bow', phaseFrame: 9 }).placements.filter((s) => s.kind === 'bow').length).toBe(1)
  const fin = buildGrid({ ...BASE, phase: 'fin', banner: ['~ fin ~', 'Thank you, goodnight'] })
  const lines = Array.from({ length: fin.rows }, (_, y) => fin.cells.slice(y * fin.columns, (y + 1) * fin.columns).map((c) => c.ch).join(''))
  expect(lines.some((l) => l.includes('~ fin ~')) && lines.some((l) => l.includes('Thank you, goodnight'))).toBe(true)
})

// The xterm 256-colour palette: the 6x6x6 cube and the grey ramp.
function onPalette(color: number): boolean {
  if (color === DEFAULT_COLOR) return true
  const parts = [(color >> 16) & 0xff, (color >> 8) & 0xff, color & 0xff]
  const cube = [0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff]
  if (parts.every((v) => cube.includes(v))) return true
  return parts[0] === parts[1] && parts[1] === parts[2] && parts[0]! >= 8 && parts[0]! <= 238 && (parts[0]! - 8) % 10 === 0
}

test('only half blocks, block elements, spaces and ASCII text, in colours on the 256-colour palette', { timeoutMs: 60_000 }, async () => {
  for (const size of SIZES) {
    for (const columns of [140, 80, 40, 20]) {
      for (const phase of PHASES) {
        // Every dust puff, broom swing and heading turns up within these frames.
        for (const [p, frame] of [0, 0.3, 0.6, 0.95].flatMap((p) => [0, 1, 2, 16].map((f) => [p, f] as const))) {
          const grid = buildGrid({ ...BASE, size, columns, phase, progress: p, fall: p, frame, phaseFrame: 9 + frame, banner: ['INTERMISSION'] })
          const bad = grid.cells.filter((cell) => {
            const code = cell.ch.codePointAt(0)!
            const isSafe = [...cell.ch].length === 1 && ((code >= 0x20 && code <= 0x7e) || (code >= 0x2580 && code <= 0x259f))
            return !isSafe || !onPalette(cell.fg) || !onPalette(cell.bg)
          })
          const where = size + ' ' + columns + ' ' + phase + ' p=' + p + ' frame ' + frame
          expect(bad.map((c) => 'U+' + c.ch.codePointAt(0)!.toString(16) + ' ' + c.fg.toString(16) + '/' + c.bg.toString(16)), where).toEqual([])
          expect(atob(packCells(grid)).length).toBe(grid.columns * grid.rows * 12)
        }
      }
    }
  }
})
