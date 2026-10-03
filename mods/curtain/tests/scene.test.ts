// The stage drawing: layouts by size, the curtain's travel, Clawd and his
// broom, the banners, and cells a Raster accepts.

import { expect, test } from 'claude-code/testing'
import { FULL_ROWS, type SceneInput, buildGrid, gridText, layout, packCells, sweepPosition } from '../hooks/scene.ts'

const BASE: SceneInput = { columns: 80, maxRows: 20, drop: 0, frame: 0, phase: 'running', phaseFrame: 0, status: 'CURTAIN · ▸ merge-pr' }

test('a roomy band gets the full stage, a narrow or short one the compact line', async () => {
  expect(layout(80, 20)).toEqual({ columns: 80, rows: FULL_ROWS, compact: false })
  expect(layout(200, 20).columns).toBe(96)
  expect(layout(20, 20).compact).toBe(true)
  expect(layout(80, 5).compact).toBe(true)
  expect(buildGrid({ ...BASE, columns: 12 }).rows).toBe(2)
  // Even a one-cell band draws something rather than nothing.
  expect(buildGrid({ ...BASE, columns: 1, maxRows: 1 }).columns).toBe(1)
})

test('open, Clawd sweeps the boards with his broom', async () => {
  const lines = gridText(buildGrid(BASE))
  expect(lines.some((l) => l.includes('▐▛███▜▌'))).toBe(true)
  expect(lines.some((l) => /[▒▓]/.test(l))).toBe(true)
  // He moves: a later frame puts him somewhere else.
  expect(sweepPosition(80, 0).x === sweepPosition(80, 10).x).toBe(false)
  // And turns at the ends.
  const dirs = new Set(Array.from({ length: 200 }, (_, f) => sweepPosition(80, f).dir))
  expect(dirs.size).toBe(2)
})

test('the curtain descends from the top as it drops', async () => {
  const curtainRows = (drop: number) =>
    gridText(buildGrid({ ...BASE, drop }))
      .slice(1, 7)
      .filter((l) => /^[█▀]+$/.test(l)).length
  expect(curtainRows(0)).toBe(0)
  expect(curtainRows(0.5)).toBe(3)
  expect(curtainRows(1)).toBe(6)
  // Fully closed, Clawd is hidden behind it.
  expect(gridText(buildGrid({ ...BASE, drop: 1 })).some((l) => l.includes('▐▛███▜▌'))).toBe(false)
})

test('the bow, the fin, and the intermission each have their beat', async () => {
  const bow = gridText(buildGrid({ ...BASE, phase: 'bow', phaseFrame: 6 }))
  expect(bow.some((l) => l.includes('▗▟▀███▀▙▖'))).toBe(true)
  const fin = gridText(buildGrid({ ...BASE, phase: 'fin', banner: ['~ fin ~', 'Thank you, goodnight'] }))
  expect(fin.some((l) => l.includes('~ fin ~'))).toBe(true)
  expect(fin.some((l) => l.includes('Thank you, goodnight'))).toBe(true)
  const held = gridText(buildGrid({ ...BASE, phase: 'halted', drop: 0.4, banner: ['INTERMISSION'], status: 'INTERMISSION · merge-pr stopped' }))
  expect(held.some((l) => l.includes('INTERMISSION'))).toBe(true)
  const narrow = gridText(buildGrid({ ...BASE, columns: 20, phase: 'halted', banner: ['INTERMISSION'] }))
  expect(narrow[0]).toMatch(/INTERMISSION/)
})

test('every cell is one width-1 BMP character, packed as the Raster expects', async () => {
  for (const phase of ['running', 'bow', 'fin', 'halted'] as const) {
    for (const columns of [80, 30, 20]) {
      const grid = buildGrid({ ...BASE, columns, phase, drop: 0.37, frame: 17, banner: ['INTERMISSION'] })
      for (const cell of grid.cells) {
        expect([...cell.ch].length).toBe(1)
        expect(cell.ch.codePointAt(0)! <= 0xffff).toBe(true)
      }
      const bytes = atob(packCells(grid)).length
      expect(bytes).toBe(grid.columns * grid.rows * 12)
    }
  }
})
