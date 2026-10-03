// The stage, as pure functions: one frame in, one grid of cells out. No `$`,
// so the hooks module calls it per frame and the tests read frames directly.
//
// Full layout (9 rows):         Compact layout (2 rows), for a narrow
//   valance (gold trim)          terminal or a short band:
//   6 rows of stage                curtain bar with Clawd sweeping
//   floor boards                   status line
//   status line

export type ScenePhase = 'running' | 'bow' | 'fin' | 'done' | 'halted'

export type SceneInput = {
  /** Cells the site gives the band (`e.props.bodyColumns`). */
  columns: number
  /** Rows the band may take (`e.props.maxRows`). */
  maxRows: number
  /** How closed the curtain is drawn, 0 (open) to 1 (closed). */
  drop: number
  /** The animation counter: Clawd's position and the dust come from it. */
  frame: number
  phase: ScenePhase
  /** Frames since the phase began: the bow's timing. */
  phaseFrame: number
  /** The line under the stage. */
  status: string
  /** Lines shown over the curtain (INTERMISSION, fin). */
  banner?: readonly string[]
}

/** What a non-terminal surface colours a cell by, in place of its RGB. */
export type Tone = 'curtain' | 'gold' | 'clawd' | 'broom' | 'dust' | 'floor' | 'text' | 'alert' | 'plain'

export type Cell = { ch: string; fg: number; bg: number; tone: Tone }

export type Grid = { columns: number; rows: number; compact: boolean; cells: Cell[] }

export const DEFAULT_COLOR = 0x01000000
export const FULL_ROWS = 9
export const COMPACT_ROWS = 2
export const MAX_COLUMNS = 96
// Narrower than this, or a band shorter than FULL_ROWS + 1, draws the compact layout.
export const MIN_FULL_COLUMNS = 28

const STAGE_TOP = 1
const STAGE_ROWS = 6
const FLOOR_ROW = STAGE_TOP + STAGE_ROWS
const STATUS_ROW = FLOOR_ROW + 1

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
const TEXT = 0xbdbdbd
const ALERT = 0xf0c040

// Clawd, the Claude Code mascot, three rows of nine cells. The cells marked
// `e` in EYES are the quadrant blocks whose missing corner is an eye.
const CLAWD_UP = [' ▐▛███▜▌ ', '▝▜█████▛▘', '  ▘▘ ▝▝  ']
const CLAWD_STEP = [' ▐▛███▜▌ ', '▝▜█████▛▘', '  ▝▘ ▝▘  ']
const CLAWD_BOW = ['         ', '▗▟▀███▀▙▖', '  ▘▘ ▝▝  ']
const EYES = new Set(['0,2', '0,6'])
const DUST_GLYPHS = ['·', '∘', '°', '˚', '·', '.']

export function layout(bodyColumns: number, maxRows: number): { columns: number; rows: number; compact: boolean } {
  const columns = Math.max(1, Math.min(MAX_COLUMNS, Math.floor(bodyColumns) || 1))
  const compact = columns < MIN_FULL_COLUMNS || !(maxRows >= FULL_ROWS + 1)
  return { columns, rows: compact ? COMPACT_ROWS : FULL_ROWS, compact }
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

function text(grid: Grid, x: number, y: number, s: string, fg: number, bg: number, tone: Tone): void {
  let i = 0
  for (const ch of s) put(grid, x + i++, y, ch, fg, bg, tone)
}

function centred(grid: Grid, y: number, s: string, fg: number, bg: number, tone: Tone): void {
  const chars = [...s].slice(0, grid.columns)
  text(grid, Math.floor((grid.columns - chars.length) / 2), y, chars.join(''), fg, bg, tone)
}

function fold(grid: Grid, x: number): number {
  const seam = (grid.columns - 1) / 2
  return FOLDS[Math.floor(Math.abs(x - seam)) % FOLDS.length]!
}

// Clawd's left edge and heading for a frame: back and forth across the boards,
// one cell a frame, with room for the broom trailing on either side.
export function sweepPosition(columns: number, frame: number): { x: number; dir: 1 | -1 } {
  const lo = 3
  const hi = Math.max(lo + 1, columns - 12)
  const span = hi - lo
  const p = ((frame % (2 * span)) + 2 * span) % (2 * span)
  return p < span ? { x: lo + p, dir: 1 } : { x: hi - (p - span), dir: -1 }
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

function sweeper(grid: Grid, frame: number): void {
  const { x, dir } = sweepPosition(grid.columns, frame)
  const top = FLOOR_ROW - 3
  const bristles = BRISTLE[frame % 2]!
  // The broom trails behind Clawd: handle up to his side, bristles on the boards.
  const handleX = dir === 1 ? x - 1 : x + 9
  const brushX = dir === 1 ? x - 3 : x + 10
  put(grid, handleX, top + 1, dir === 1 ? '╱' : '╲', HANDLE, DEFAULT_COLOR, 'broom')
  put(grid, brushX, top + 2, frame % 2 ? '▓' : '▒', bristles, DEFAULT_COLOR, 'broom')
  put(grid, brushX + 1, top + 2, frame % 2 ? '▒' : '▓', bristles, DEFAULT_COLOR, 'broom')
  // Three puffs of dust kicked up behind the broom, drifting away and up.
  for (let i = 0; i < 3; i++) {
    const age = (frame + i * 2) % DUST_GLYPHS.length
    const dx = dir === 1 ? brushX - 1 - age : brushX + 2 + age
    const dy = age < 3 ? top + 2 : top + 1
    put(grid, dx, dy, DUST_GLYPHS[age]!, DUST[age]!, DEFAULT_COLOR, 'dust')
  }
  sprite(grid, frame % 4 < 2 ? CLAWD_UP : CLAWD_STEP, x, top)
}

function curtain(grid: Grid, drop: number): void {
  // Half-row resolution: the hem moves half a cell at a time.
  const halves = STAGE_ROWS * 2
  const covered = Math.max(0, Math.min(halves, Math.round(drop * halves)))
  for (let r = 0; r < STAGE_ROWS; r++) {
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

function frameStage(grid: Grid): void {
  for (let x = 0; x < grid.columns; x++) {
    put(grid, x, 0, x % 6 === 0 ? '▀' : '▀', VALANCE, x % 6 === 3 ? VALANCE : GOLD, 'gold')
    put(grid, x, FLOOR_ROW, '▀', x % 9 === 0 ? BOARD_SHADE : BOARD, BOARD_SHADE, 'floor')
  }
}

function full(input: SceneInput, columns: number): Grid {
  const grid: Grid = { columns, rows: FULL_ROWS, compact: false, cells: blank(columns, FULL_ROWS) }
  const behind = input.phase === 'running' || input.phase === 'halted'
  if (behind) sweeper(grid, input.frame)
  curtain(grid, input.phase === 'running' || input.phase === 'halted' ? input.drop : 1)
  if (input.phase === 'bow') {
    // In front of the closed curtain, centre stage: up, a bow, up again.
    const bowing = input.phaseFrame >= 4 && input.phaseFrame < 12
    sprite(grid, bowing ? CLAWD_BOW : CLAWD_UP, Math.floor((columns - 9) / 2), FLOOR_ROW - 3)
  }
  const banner = input.banner ?? []
  banner.forEach((line, i) => {
    const y = STAGE_TOP + 1 + i
    const bg = at(grid, Math.floor(columns / 2), y)?.tone === 'curtain' ? fold(grid, Math.floor(columns / 2)) : DEFAULT_COLOR
    centred(grid, y, line, ALERT, bg, input.phase === 'halted' ? 'alert' : 'gold')
  })
  frameStage(grid)
  const tone: Tone = input.phase === 'halted' ? 'alert' : 'text'
  text(grid, 0, STATUS_ROW, [...input.status].slice(0, columns).join(''), input.phase === 'halted' ? ALERT : TEXT, DEFAULT_COLOR, tone)
  return grid
}

function compact(input: SceneInput, columns: number): Grid {
  const grid: Grid = { columns, rows: COMPACT_ROWS, compact: true, cells: blank(columns, COMPACT_ROWS) }
  const drop = input.phase === 'running' || input.phase === 'halted' ? input.drop : 1
  const filled = Math.round(drop * columns)
  for (let x = 0; x < columns; x++) {
    if (x < filled) put(grid, x, 0, '█', fold(grid, x), DEFAULT_COLOR, 'curtain')
    else put(grid, x, 0, '▁', BOARD, DEFAULT_COLOR, 'floor')
  }
  if (input.phase === 'running' && filled < columns) {
    // Clawd shrinks to one glyph and sweeps the boards the curtain has not reached.
    const room = columns - filled
    const p = input.frame % Math.max(1, 2 * room)
    const x = filled + (p < room ? p : 2 * room - 1 - p)
    put(grid, x, 0, '▟', CLAWD, DEFAULT_COLOR, 'clawd')
  }
  const banner = input.banner?.[0]
  if (banner) centred(grid, 0, banner, ALERT, fold(grid, Math.floor(columns / 2)), input.phase === 'halted' ? 'alert' : 'gold')
  text(grid, 0, 1, [...input.status].slice(0, columns).join(''), input.phase === 'halted' ? ALERT : TEXT, DEFAULT_COLOR, input.phase === 'halted' ? 'alert' : 'text')
  return grid
}

export function buildGrid(input: SceneInput): Grid {
  const shape = layout(input.columns, input.maxRows)
  return shape.compact ? compact(input, shape.columns) : full(input, shape.columns)
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
