// The stage drawing: the strip, the half stage and the full stage, the
// curtain's travel, Clawd and his broom, the banners, and cells a Raster accepts.

import { expect, test } from 'claude-code/testing'
import { FULL_ROWS, MEDIUM_ROWS, STRIP_ROWS, type SceneInput, buildGrid, gridText, layout, packCells, sweepPosition } from '../hooks/scene.js'

const STAGE: SceneInput = {
  columns: 80,
  maxRows: 20,
  layout: 'full',
  drop: 0,
  frame: 0,
  phase: 'running',
  phaseFrame: 0,
  status: 'CURTAIN · ▸ merge-pr',
}
const STRIP: SceneInput = { ...STAGE, layout: 'strip', drop: 0.31, frame: 7, status: '1/3 merge-pr 3:12 / ~8m', marks: [0.648, 0.973, 1] }

test('each layout gets its rows, and a band too small for one gets the next down', async () => {
  expect(layout(80, 20, 'full')).toEqual({ columns: 80, rows: FULL_ROWS, layout: 'full' })
  expect(layout(80, 20, 'medium')).toEqual({ columns: 80, rows: MEDIUM_ROWS, layout: 'medium' })
  expect(layout(80, 20, 'strip')).toEqual({ columns: 80, rows: STRIP_ROWS, layout: 'strip' })
  expect(layout(80, 8, 'full').layout).toBe('medium')
  expect(layout(80, 5, 'full').layout).toBe('strip')
  expect(layout(20, 20, 'full').layout).toBe('strip')
  expect(layout(200, 20, 'full').columns).toBe(96)
  // Even a one-cell band draws something rather than nothing.
  expect(buildGrid({ ...STAGE, columns: 1, maxRows: 1 }).columns).toBe(1)
})

test('the strip is at most two rows at 80 and 40 columns, with the step, its time and Clawd', async () => {
  for (const columns of [80, 40]) {
    const grid = buildGrid({ ...STRIP, columns })
    expect(grid.rows <= 2).toBe(true)
    expect(grid.columns).toBe(columns)
    const [bar, row] = gridText(grid)
    // The valance: velvet as far as the work has come, the rail after it.
    expect(bar!.startsWith('▀'.repeat(Math.round(0.31 * columns)))).toBe(true)
    expect(bar!.endsWith('▔')).toBe(true)
    expect(row!.startsWith('1/3 merge-pr 3:12 / ~8m')).toBe(true)
    expect(row!).toMatch(/[▐▗]▛█▜[▌▖]/)
  }
  // Clawd sweeps his lane, both ways, and never over the status.
  const xs = Array.from({ length: 120 }, (_, f) => gridText(buildGrid({ ...STRIP, frame: f }))[1]!.search(/[▐▗]▛/))
  expect(Math.min(...xs) > '1/3 merge-pr 3:12 / ~8m'.length).toBe(true)
  expect(new Set(xs).size > 20).toBe(true)
  // Too narrow for his lane, the status keeps the row to itself.
  expect(gridText(buildGrid({ ...STRIP, columns: 24 }))[1]).toBe('1/3 merge-pr 3:12 / ~8m ')
})

test('a held strip stays two rows and gives its second row to the reason', async () => {
  const held = gridText(buildGrid({ ...STRIP, phase: 'halted', status: 'INTERMISSION · merge-pr stopped: PR #42 is OPEN' }))
  expect(held.length).toBe(2)
  expect(held[1]!.startsWith('INTERMISSION · merge-pr stopped: PR #42 is OPEN')).toBe(true)
  expect(held[1]!).not.toMatch(/▛█▜/)
})

test('on the stage, Clawd sweeps the boards with his broom', async () => {
  const lines = gridText(buildGrid(STAGE))
  expect(lines.some((l) => l.includes('▐▛███▜▌'))).toBe(true)
  expect(lines.some((l) => /[▒▓]/.test(l))).toBe(true)
  expect(sweepPosition(80, 0).x === sweepPosition(80, 10).x).toBe(false)
  const dirs = new Set(Array.from({ length: 200 }, (_, f) => sweepPosition(80, f).dir))
  expect(dirs.size).toBe(2)
  // The half stage has him too.
  expect(gridText(buildGrid({ ...STAGE, layout: 'medium' })).some((l) => l.includes('▝▜█████▛▘'))).toBe(true)
})

test('the curtain descends from the top as it drops', async () => {
  const curtainRows = (drop: number) =>
    gridText(buildGrid({ ...STAGE, drop }))
      .slice(1, 7)
      .filter((l) => /^[█▀]+$/.test(l)).length
  expect(curtainRows(0)).toBe(0)
  expect(curtainRows(0.5)).toBe(3)
  expect(curtainRows(1)).toBe(6)
  expect(gridText(buildGrid({ ...STAGE, drop: 1 })).some((l) => l.includes('▐▛███▜▌'))).toBe(false)
})

test('the bow, the fin, and the intermission each have their beat', async () => {
  const bow = gridText(buildGrid({ ...STAGE, phase: 'bow', phaseFrame: 6 }))
  expect(bow.some((l) => l.includes('▗▟▀███▀▙▖'))).toBe(true)
  const fin = gridText(buildGrid({ ...STAGE, phase: 'fin', banner: ['~ fin ~', 'Thank you, goodnight'] }))
  expect(fin.some((l) => l.includes('~ fin ~'))).toBe(true)
  expect(fin.some((l) => l.includes('Thank you, goodnight'))).toBe(true)
  const held = gridText(buildGrid({ ...STAGE, phase: 'halted', drop: 0.4, banner: ['INTERMISSION'] }))
  expect(held.some((l) => l.includes('INTERMISSION'))).toBe(true)
  // The finale's fall still shows Clawd sweeping beneath the curtain.
  expect(gridText(buildGrid({ ...STAGE, phase: 'finale', drop: 0.5 })).some((l) => l.includes('▝▜█████▛▘'))).toBe(true)
})

test('every cell is one width-1 BMP character, packed as the Raster expects', async () => {
  for (const which of ['strip', 'medium', 'full'] as const) {
    for (const phase of ['running', 'finale', 'bow', 'fin', 'halted'] as const) {
      for (const columns of [80, 40, 20]) {
        const grid = buildGrid({ ...STRIP, layout: which, columns, phase, drop: 0.37, frame: 17, banner: ['INTERMISSION'] })
        for (const cell of grid.cells) {
          expect([...cell.ch].length).toBe(1)
          expect(cell.ch.codePointAt(0)! <= 0xffff).toBe(true)
        }
        expect(atob(packCells(grid)).length).toBe(grid.columns * grid.rows * 12)
      }
    }
  }
})
