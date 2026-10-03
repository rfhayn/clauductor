// The stage crew, drawn: sizes and their rows, the crew and props thinning as
// progress rises, nothing clipped at any width, the bow only in its phase, and
// only characters and colours that survive tmux inside xterm.js.

import { expect, test } from 'claude-code/testing'
import {
  DEFAULT_COLOR,
  ROWS,
  type SceneInput,
  type StageSize,
  buildGrid,
  cast,
  census,
  crewFor,
  gridText,
  layout,
  packCells,
  propsFor,
} from '../hooks/scene.js'

const BASE: SceneInput = {
  columns: 80,
  maxRows: 20,
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
const crewIn = (lines: string[]) => lines.join('\n').split('▐▛▜▌').length - 1

test('each size has its rows, and a band too small for it steps down rather than clip', async () => {
  expect(ROWS).toEqual({ bar: 2, small: 6, medium: 9, full: 12 })
  expect(layout(80, 20, 'small')).toEqual({ columns: 80, rows: 6, layout: 'small' })
  expect(layout(80, 20, 'medium').rows).toBe(9)
  expect(layout(80, 20, 'full').rows).toBe(12)
  expect(layout(80, 10, 'full').layout).toBe('medium')
  expect(layout(80, 9, 'full').layout).toBe('small')
  expect(layout(80, 6, 'small').layout).toBe('bar')
  expect(layout(29, 20, 'full').layout).toBe('bar')
  expect(layout(40, 20, 'full').layout).toBe('full')
  expect(layout(200, 20, 'small').columns).toBe(96)
  for (const size of SIZES) expect(buildGrid({ ...BASE, size }).rows).toBe(ROWS[size])
})

test('the crew and props start full and only ever thin out as progress rises', async () => {
  expect(census(cast(80), 0)).toEqual({ crew: 5, props: 6 })
  expect(census(cast(60), 0)).toEqual({ crew: 4, props: 4 })
  expect(census(cast(40), 0)).toEqual({ crew: 3, props: 3 })
  for (const columns of [96, 80, 70, 60, 50, 40, 30]) {
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

test('the stage is never clear before the work completes, and at the end one Clawd sweeps the last spot', async () => {
  for (const columns of [80, 60, 40]) {
    for (let p = 0; p < 1; p += 0.01) {
      const grid = buildGrid({ ...BASE, columns, progress: p })
      expect(grid.placements.some((s) => s.kind === 'crew'), columns + ' columns at ' + p.toFixed(2)).toBe(true)
    }
    const end = buildGrid({ ...BASE, columns, progress: 0.95 })
    expect(census(cast(columns), 0.95)).toEqual({ crew: 1, props: 0 })
    expect(end.placements.filter((s) => s.kind === 'crew').length).toBe(1)
    expect(end.placements.filter((s) => s.kind === 'prop').length).toBe(0)
    // The last spot: a little pile by his broom.
    expect(gridText(end).some((l) => /[:.]{1,3}/.test(l.replace(/^.\s*/, '')))).toBe(true)
  }
})

test('the drawing thins out with the census', async () => {
  const at = (p: number) => buildGrid({ ...BASE, progress: p })
  expect(crewIn(gridText(at(0)))).toBe(5)
  expect(at(0).placements.filter((s) => s.kind === 'prop').length).toBe(6)
  expect(crewIn(gridText(at(0.95)))).toBe(1)
})

test('no sprite is clipped at 80, 60 or 40 columns, in any size, phase or frame', async () => {
  for (const size of SIZES) {
    for (const columns of [80, 60, 40]) {
      for (const phase of PHASES) {
        for (let p = 0; p <= 1; p += 0.05) {
          for (const frame of [0, 1, 5, 7]) {
            const grid = buildGrid({ ...BASE, size, columns, phase, progress: p, fall: p, frame, phaseFrame: frame * 2 })
            expect(grid.rows < BASE.maxRows).toBe(true)
            for (const s of grid.placements) {
              const where = size + ' ' + columns + ' ' + phase + ' p=' + p.toFixed(2) + ' ' + JSON.stringify(s)
              expect(s.x >= 0 && s.y >= 0 && s.x + s.w <= grid.columns && s.y + s.h <= grid.rows, where).toBe(true)
            }
          }
        }
      }
    }
  }
})

test('no Clawd walks through a prop or another Clawd while the work runs', async () => {
  for (const size of SIZES) {
    for (const columns of [80, 60, 40]) {
      for (let p = 0; p < 1; p += 0.005) {
        const grid = buildGrid({ ...BASE, size, columns, progress: p })
        const things = grid.placements.filter((s) => s.kind === 'crew' || s.kind === 'prop')
        for (let i = 0; i < things.length; i++) {
          for (let j = i + 1; j < things.length; j++) {
            const a = things[i]!
            const b = things[j]!
            const across = Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x)
            const down = Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y)
            expect(across <= 0 || down <= 0, size + ' ' + columns + ' p=' + p.toFixed(3) + ' ' + JSON.stringify([a, b])).toBe(true)
          }
        }
      }
    }
  }
})

test('the curtain never covers the crew while the work runs', async () => {
  for (const size of SIZES) {
    const lines = gridText(buildGrid({ ...BASE, size, progress: 0.999 }))
    expect(crewIn(lines), size).toBe(1)
  }
})

test('the bow happens only in the bow phase, never on a halt', async () => {
  for (const phase of PHASES) {
    for (let f = 0; f < 20; f++) {
      const lines = gridText(buildGrid({ ...BASE, phase, progress: 0.97, fall: 1, phaseFrame: f, frame: f }))
      const bowing = lines.some((l) => l.includes('▗▄▄▖'))
      if (phase !== 'bow') expect(bowing, phase + ' frame ' + f).toBe(false)
    }
  }
  const bow = gridText(buildGrid({ ...BASE, phase: 'bow', phaseFrame: 9 }))
  expect(bow.some((l) => l.includes('▗▄▄▖'))).toBe(true)
  const fin = gridText(buildGrid({ ...BASE, phase: 'fin', banner: ['~ fin ~', 'Thank you, goodnight'] }))
  expect(fin.some((l) => l.includes('~ fin ~')) && fin.some((l) => l.includes('Thank you, goodnight'))).toBe(true)
  const held = gridText(buildGrid({ ...BASE, phase: 'halted', progress: 0.4, banner: ['INTERMISSION'], status: 'INTERMISSION - merge-pr stopped' }))
  expect(held.some((l) => l.includes('INTERMISSION'))).toBe(true)
})

// The xterm 256-colour palette: the 6x6x6 cube and the grey ramp.
function onPalette(color: number): boolean {
  if (color === DEFAULT_COLOR) return true
  const parts = [(color >> 16) & 0xff, (color >> 8) & 0xff, color & 0xff]
  const cube = [0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff]
  if (parts.every((v) => cube.includes(v))) return true
  return parts[0] === parts[1] && parts[1] === parts[2] && parts[0]! >= 8 && parts[0]! <= 238 && (parts[0]! - 8) % 10 === 0
}

test('only ASCII, box drawing and block elements, in colours on the 256-colour palette', async () => {
  for (const size of SIZES) {
    for (const columns of [80, 40, 20]) {
      for (const phase of PHASES) {
        // Every dust glyph and leg pose turns up within four frames.
        for (const [p, frame] of [0, 0.5, 0.95].flatMap((p) => [0, 1, 2, 3].map((f) => [p, f] as const))) {
          const grid = buildGrid({ ...BASE, size, columns, phase, progress: p, fall: p, frame, phaseFrame: 9 + frame, banner: ['INTERMISSION'] })
          for (const cell of grid.cells) {
            const code = cell.ch.codePointAt(0)!
            expect([...cell.ch].length).toBe(1)
            expect((code >= 0x20 && code <= 0x7e) || (code >= 0x2500 && code <= 0x259f), 'U+' + code.toString(16)).toBe(true)
            expect(onPalette(cell.fg) && onPalette(cell.bg), cell.fg.toString(16) + ' ' + cell.bg.toString(16)).toBe(true)
          }
          expect(atob(packCells(grid)).length).toBe(grid.columns * grid.rows * 12)
        }
      }
    }
  }
})
