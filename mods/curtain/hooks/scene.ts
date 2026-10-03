// The stage, as pure functions: one frame in, one grid of cells out. No `$`,
// so the hooks module calls it per frame and the tests read frames directly.
//
// Three layouts:
//   strip  (2 rows)  while the work runs, by default: a velvet valance with a
//                    gold hem fills the top row as the progress bar; the step,
//                    its time and a small Clawd sweeping share the second row
//   medium (6 rows)  valance, 3 rows of stage, floor, status
//   full   (9 rows)  valance, 6 rows of stage, floor, status; the finale's
//                    layout whatever the size, and the whole run's at `full`
// A band too narrow or too short for the layout asked for gets the next one down.

export type ScenePhase = 'running' | 'finale' | 'bow' | 'fin' | 'done' | 'halted'
export type SceneLayout = 'strip' | 'medium' | 'full'

export type SceneInput = {
  /** Cells the site gives the band (`e.props.bodyColumns`). */
  columns: number
  /** Rows the band may take (`e.props.maxRows`). */
  maxRows: number
  /** The layout asked for; a band too small for it gets a smaller one. */
  layout: SceneLayout
  /** How closed the curtain is drawn (or how full the strip's bar), 0 to 1. */
  drop: number
  /** The animation counter: Clawd's position and the dust come from it. */
  frame: number
  phase: ScenePhase
  /** Frames since the phase began: the bow's timing. */
  phaseFrame: number
  /** The status line (the strip's second row when it is held). */
  status: string
  /** Lines shown over the curtain (INTERMISSION, fin). */
  banner?: readonly string[]
  /** Step boundaries as fractions of the travel, marked on the strip's rail. */
  marks?: readonly number[]
}

/** What a non-terminal surface colours a cell by, in place of its RGB. */
export type Tone = 'curtain' | 'gold' | 'clawd' | 'broom' | 'dust' | 'floor' | 'text' | 'alert' | 'plain'

export type Cell = { ch: string; fg: number; bg: number; tone: Tone }

export type Grid = { columns: number; rows: number; layout: SceneLayout; cells: Cell[] }

export const DEFAULT_COLOR = 0x01000000
export const MAX_COLUMNS = 96
export const STRIP_ROWS = 2
export const MEDIUM_ROWS = 6
export const FULL_ROWS = 9
// A stage narrower than this draws the strip instead.
export const MIN_STAGE_COLUMNS = 28
// The strip's status column, at least; the rest of the second row is Clawd's
// lane, when it is at least STRIP_LANE_MIN wide.
const STRIP_STATUS_COLUMNS = 26
const STRIP_LANE_MIN = 12

const STAGE_TOP = 1

// Velvet: mirrored around the centre seam so the two curtains fold alike.
const FOLDS = [0x5c0912, 0x7a0e19, 0x991321, 0xb3192a, 0x991321, 0x7a0e19]
const GOLD = 0xd4a017
const VALANCE = 0x4a0710
const CLAWD = 0xd97757
const EYE = 0x000000
const HANDLE = 0x9a6a3a
const BRISTLE = [0xe3c46a, 0xc9a54c]
const DUST = [0xd0d0d0, 0xb0b0b0, 0x909090, 0x777777, 0x5f5f5f, 0x4a4a4a]
const BOARD = 0x8a5a2b
const BOARD_SHADE = 0x5e3b1b
const RAIL = 0x4a4a4a
const TEXT = 0xbdbdbd
const ALERT = 0xf0c040

// Clawd, the Claude Code mascot, three rows of nine cells. The cells marked in
// EYES are the quadrant blocks whose missing corner is an eye.
const CLAWD_UP = [' ▐▛███▜▌ ', '▝▜█████▛▘', '  ▘▘ ▝▝  ']
const CLAWD_STEP = [' ▐▛███▜▌ ', '▝▜█████▛▘', '  ▝▘ ▝▘  ']
const CLAWD_BOW = ['         ', '▗▟▀███▀▙▖', '  ▘▘ ▝▝  ']
const EYES = new Set(['0,2', '0,6'])
// Small Clawd for the strip: his head and eyes, bobbing as he sweeps.
const MINI = ['▐▛█▜▌', '▗▛█▜▖']
const MINI_EYES = new Set([1, 3])
const DUST_GLYPHS = ['·', '∘', '°', '˚', '·', '.']

const ROWS: Record<SceneLayout, number> = { strip: STRIP_ROWS, medium: MEDIUM_ROWS, full: FULL_ROWS }

// The layout a band can hold: the one asked for, or the next one down.
export function layout(bodyColumns: number, maxRows: number, wanted: SceneLayout = 'full'): { columns: number; rows: number; layout: SceneLayout } {
  const columns = Math.max(1, Math.min(MAX_COLUMNS, Math.floor(bodyColumns) || 1))
  const order: SceneLayout[] = wanted === 'full' ? ['full', 'medium', 'strip'] : wanted === 'medium' ? ['medium', 'strip'] : ['strip']
  const fits = (l: SceneLayout) => l === 'strip' || (columns >= MIN_STAGE_COLUMNS && maxRows >= ROWS[l] + 1)
  const chosen = order.find(fits) ?? 'strip'
  return { columns, rows: ROWS[chosen], layout: chosen }
}

function blank(columns: number, rows: number): Cell[] {
  return Array.from({ length: columns * rows }, () => ({ ch: ' ', fg: DEFAULT_COLOR, bg: DEFAULT_COLOR, tone: 'plain' as Tone }))
}

function put(grid: Grid, x: number, y: number, ch: string, fg: number, bg: number, tone: Tone): void {
  if (x < 0 || y < 0 || x >= grid.columns || y >= grid.rows) return
  grid.cells[y * grid.columns + x] = { ch, fg, bg, tone }
}

function at(grid: Grid, x: number, y: number): Cell | undefined {
  if (x < 0 || y < 0 || x >= grid.columns || y >= grid.rows) return undefined
  return grid.cells[y * grid.columns + x]
}

function text(grid: Grid, x: number, y: number, s: string, fg: number, bg: number, tone: Tone, width = grid.columns): void {
  let i = 0
  for (const ch of s) {
    if (i >= width) break
    put(grid, x + i++, y, ch, fg, bg, tone)
  }
}

function centred(grid: Grid, y: number, s: string, fg: number, bg: number, tone: Tone): void {
  const chars = [...s].slice(0, grid.columns)
  text(grid, Math.floor((grid.columns - chars.length) / 2), y, chars.join(''), fg, bg, tone)
}

function fold(grid: Grid, x: number): number {
  const seam = (grid.columns - 1) / 2
  return FOLDS[Math.floor(Math.abs(x - seam)) % FOLDS.length]!
}

// Back and forth across a lane `lo..hi` (left edges), one cell a frame.
function bounce(lo: number, hi: number, frame: number): { x: number; dir: 1 | -1 } {
  const span = Math.max(1, hi - lo)
  const p = ((frame % (2 * span)) + 2 * span) % (2 * span)
  return p < span ? { x: lo + p, dir: 1 } : { x: hi - (p - span), dir: -1 }
}

// Clawd's left edge and heading on the stage, with room for the broom trailing.
export function sweepPosition(columns: number, frame: number): { x: number; dir: 1 | -1 } {
  return bounce(3, Math.max(4, columns - 12), frame)
}

function sprite(grid: Grid, rows: readonly string[], x: number, top: number): void {
  rows.forEach((row, r) => {
    let c = 0
    for (const ch of row) {
      if (ch !== ' ') {
        const behind = at(grid, x + c, top + r)?.bg ?? DEFAULT_COLOR
        put(grid, x + c, top + r, ch, CLAWD, EYES.has(r + ',' + c) ? EYE : behind, 'clawd')
      }
      c++
    }
  })
}

function dust(grid: Grid, frame: number, fromX: number, dir: 1 | -1, low: number, high: number, puffs: number): void {
  for (let i = 0; i < puffs; i++) {
    const age = (frame + i * 2) % DUST_GLYPHS.length
    const dx = dir === 1 ? fromX - 1 - age : fromX + 2 + age
    put(grid, dx, age < 3 ? low : high, DUST_GLYPHS[age]!, DUST[age]!, DEFAULT_COLOR, 'dust')
  }
}

function sweeper(grid: Grid, frame: number, floorRow: number): void {
  const { x, dir } = sweepPosition(grid.columns, frame)
  const top = floorRow - 3
  const bristles = BRISTLE[frame % 2]!
  // The broom trails behind Clawd: handle up to his side, bristles on the boards.
  const handleX = dir === 1 ? x - 1 : x + 9
  const brushX = dir === 1 ? x - 3 : x + 10
  put(grid, handleX, top + 1, dir === 1 ? '╱' : '╲', HANDLE, DEFAULT_COLOR, 'broom')
  put(grid, brushX, top + 2, frame % 2 ? '▓' : '▒', bristles, DEFAULT_COLOR, 'broom')
  put(grid, brushX + 1, top + 2, frame % 2 ? '▒' : '▓', bristles, DEFAULT_COLOR, 'broom')
  dust(grid, frame, brushX, dir, top + 2, top + 1, 3)
  sprite(grid, frame % 4 < 2 ? CLAWD_UP : CLAWD_STEP, x, top)
}

function curtain(grid: Grid, drop: number, stageRows: number): void {
  // Half-row resolution: the hem moves half a cell at a time.
  const halves = stageRows * 2
  const covered = Math.max(0, Math.min(halves, Math.round(drop * halves)))
  for (let r = 0; r < stageRows; r++) {
    const topHalf = 2 * r < covered
    const bottomHalf = 2 * r + 1 < covered
    if (!topHalf) continue
    for (let x = 0; x < grid.columns; x++) {
      const velvet = fold(grid, x)
      const topColor = 2 * r === covered - 1 ? GOLD : velvet
      const y = STAGE_TOP + r
      if (bottomHalf) {
        const bottomColor = 2 * r + 1 === covered - 1 ? GOLD : velvet
        if (bottomColor === topColor) put(grid, x, y, '█', topColor, DEFAULT_COLOR, 'curtain')
        else put(grid, x, y, '▀', topColor, bottomColor, 'curtain')
      } else {
        // The hem: velvet over whatever is still showing in the lower half.
        put(grid, x, y, '▀', topColor, DEFAULT_COLOR, topColor === GOLD ? 'gold' : 'curtain')
      }
    }
  }
}

function stage(input: SceneInput, columns: number, stageRows: number, which: SceneLayout): Grid {
  const rows = stageRows + 3
  const floorRow = STAGE_TOP + stageRows
  const grid: Grid = { columns, rows, layout: which, cells: blank(columns, rows) }
  const isOpen = input.phase === 'running' || input.phase === 'halted' || input.phase === 'finale'
  if (isOpen) sweeper(grid, input.frame, floorRow)
  curtain(grid, isOpen ? input.drop : 1, stageRows)
  if (input.phase === 'bow') {
    // In front of the closed curtain, centre stage: up, a bow, up again.
    const bowing = input.phaseFrame >= 4 && input.phaseFrame < 12
    sprite(grid, bowing ? CLAWD_BOW : CLAWD_UP, Math.floor((columns - 9) / 2), floorRow - 3)
  }
  ;(input.banner ?? []).forEach((line, i) => {
    const y = Math.min(STAGE_TOP + (stageRows > 3 ? 1 : 0) + i, floorRow - 1)
    const mid = Math.floor(columns / 2)
    const bg = at(grid, mid, y)?.tone === 'curtain' ? fold(grid, mid) : DEFAULT_COLOR
    centred(grid, y, line, ALERT, bg, input.phase === 'halted' ? 'alert' : 'gold')
  })
  for (let x = 0; x < columns; x++) {
    put(grid, x, 0, '▀', VALANCE, x % 6 === 3 ? VALANCE : GOLD, 'gold')
    put(grid, x, floorRow, '▀', x % 9 === 0 ? BOARD_SHADE : BOARD, BOARD_SHADE, 'floor')
  }
  const isHeld = input.phase === 'halted'
  text(grid, 0, floorRow + 1, input.status, isHeld ? ALERT : TEXT, DEFAULT_COLOR, isHeld ? 'alert' : 'text')
  return grid
}

function strip(input: SceneInput, columns: number): Grid {
  const grid: Grid = { columns, rows: STRIP_ROWS, layout: 'strip', cells: blank(columns, STRIP_ROWS) }
  // Row 0: the valance. Velvet over a gold hem as far as the work has come,
  // a thin rail after it, with each step's boundary picked out in gold.
  const filled = Math.max(0, Math.min(columns, Math.round(input.drop * columns)))
  const marks = new Set((input.marks ?? []).slice(0, -1).map((m) => Math.min(columns - 1, Math.round(m * columns))))
  for (let x = 0; x < columns; x++) {
    if (x < filled) put(grid, x, 0, '▀', fold(grid, x), GOLD, 'curtain')
    else put(grid, x, 0, '▔', marks.has(x) ? GOLD : RAIL, DEFAULT_COLOR, marks.has(x) ? 'gold' : 'floor')
  }
  // Row 1: held, the reason takes the whole row; running, the status on the
  // left and Clawd sweeping the rest.
  const isHeld = input.phase === 'halted'
  if (isHeld) {
    text(grid, 0, 1, input.status, ALERT, DEFAULT_COLOR, 'alert')
    return grid
  }
  // The status takes what it needs (at least STRIP_STATUS_COLUMNS, so Clawd's
  // lane does not jump as the clock ticks); Clawd gets the rest if it is enough.
  const statusColumns = Math.max(STRIP_STATUS_COLUMNS, [...input.status].length + 2)
  const lane = columns - statusColumns
  text(grid, 0, 1, input.status, TEXT, DEFAULT_COLOR, 'text')
  if (lane >= STRIP_LANE_MIN) {
    const lo = statusColumns + 3
    const hi = columns - 8
    const { x, dir } = bounce(lo, Math.max(lo + 1, hi), input.frame)
    const mini = MINI[input.frame % 4 < 2 ? 0 : 1]!
    const handleX = dir === 1 ? x - 1 : x + 5
    const brushX = dir === 1 ? x - 2 : x + 6
    put(grid, handleX, 1, dir === 1 ? '╱' : '╲', HANDLE, DEFAULT_COLOR, 'broom')
    put(grid, brushX, 1, input.frame % 2 ? '▓' : '▒', BRISTLE[input.frame % 2]!, DEFAULT_COLOR, 'broom')
    // A puff or two, kept inside Clawd's lane.
    for (let i = 0; i < 2; i++) {
      const age = (input.frame + i * 3) % DUST_GLYPHS.length
      const dx = dir === 1 ? brushX - 1 - age : brushX + 1 + age
      if (dx > statusColumns && dx < columns) put(grid, dx, 1, DUST_GLYPHS[age]!, DUST[age]!, DEFAULT_COLOR, 'dust')
    }
    let c = 0
    for (const ch of mini) {
      put(grid, x + c, 1, ch, CLAWD, MINI_EYES.has(c) ? EYE : DEFAULT_COLOR, 'clawd')
      c++
    }
  }
  return grid
}

export function buildGrid(input: SceneInput): Grid {
  const shape = layout(input.columns, input.maxRows, input.layout)
  if (shape.layout === 'strip') return strip(input, shape.columns)
  return stage(input, shape.columns, shape.layout === 'full' ? 6 : 3, shape.layout)
}

// The Raster's `cells`: base64 of little-endian u32 triplets [codePoint, fg, bg].
export function packCells(grid: Grid): string {
  const words = new Uint32Array(grid.cells.length * 3)
  grid.cells.forEach((cell, i) => {
    words[i * 3] = cell.ch.codePointAt(0) ?? 32
    words[i * 3 + 1] = cell.fg
    words[i * 3 + 2] = cell.bg
  })
  return (new Uint8Array(words.buffer) as Uint8Array & { toBase64(): string }).toBase64()
}

// The grid as plain lines, for reading a frame in a test.
export function gridText(grid: Grid): string[] {
  const lines: string[] = []
  for (let y = 0; y < grid.rows; y++) {
    lines.push(grid.cells.slice(y * grid.columns, (y + 1) * grid.columns).map((c) => c.ch).join(''))
  }
  return lines
}

// The grid as runs of one tone per row, for surfaces without a Raster.
export function gridRuns(grid: Grid): { text: string; tone: Tone }[][] {
  const rows: { text: string; tone: Tone }[][] = []
  for (let y = 0; y < grid.rows; y++) {
    const runs: { text: string; tone: Tone }[] = []
    for (const cell of grid.cells.slice(y * grid.columns, (y + 1) * grid.columns)) {
      const last = runs[runs.length - 1]
      if (last && last.tone === cell.tone) last.text += cell.ch
      else runs.push({ text: cell.ch, tone: cell.tone })
    }
    rows.push(runs)
  }
  return rows
}
