// The stage, as pure functions: one frame in, one grid of cells out. No `$`,
// so the hooks module calls it per frame and the tests read frames directly.
//
// A stage crew of small Clawds strikes the set while the work runs. Progress
// (the weighted travel, which never passes a step's boundary before the step
// completes) decides the scene: each mover carries its props into the wings and
// leaves with the last one, and the curtain comes down over the empty sky. One
// sweeper stays to the end and sweeps the last spot. The finale closes the
// curtain, one Clawd runs out in front of it and bows, then the fin.
//
// Sizes (rows): small 6, medium 9, full 12; each is a valance row, the stage
// (3, 6 or 9 rows), the floor and the status. A band too short or too narrow
// for the size asked for gets the next one down, and below `small` a two-row
// bar (no sprites), so a sprite is never clipped.
//
// What reaches the terminal, for tmux inside xterm.js (the panel's lanes):
// - characters: printable ASCII, box drawing and block elements (U+2500 to
//   U+259F) only. xterm.js draws that range itself, cell-exact. No emoji, no
//   symbols such as · ° ✓ ▸ whose width a font or tmux may count as two.
// - colours: every RGB value is one of the xterm 256-colour palette's (the 6x6x6
//   cube or the grey ramp). The panel's tmux gives xterm-256color no RGB
//   feature, so tmux maps truecolour down to 256 colours; colours already on
//   the palette survive that unchanged.

export type ScenePhase = 'running' | 'finale' | 'bow' | 'fin' | 'done' | 'halted'
export type StageSize = 'small' | 'medium' | 'full'
export type SceneLayout = 'bar' | StageSize

export type SceneInput = {
  /** Cells the site gives the band (`e.props.bodyColumns`). */
  columns: number
  /** Rows the band may take (`e.props.maxRows`). */
  maxRows: number
  /** The stage size asked for; a band too small for it gets a smaller one. */
  size: StageSize
  /** The weighted travel, 0 to 1. Decides who and what is still on stage. */
  progress: number
  /** The finale's last fall of the curtain, 0 to 1. */
  fall: number
  /** The animation counter: legs, brooms and dust come from it. */
  frame: number
  phase: ScenePhase
  /** Frames since the phase began: the bow's timing. */
  phaseFrame: number
  /** The status row. */
  status: string
  /** Lines shown over the stage (INTERMISSION, fin). */
  banner?: readonly string[]
}

/** What a non-terminal surface colours a cell by, in place of its RGB. */
export type Tone = 'curtain' | 'gold' | 'clawd' | 'broom' | 'dust' | 'floor' | 'prop' | 'text' | 'alert' | 'plain'

export type Cell = { ch: string; fg: number; bg: number; tone: Tone }

/** A sprite as drawn: where it sits, so a test can check nothing was clipped. */
export type Placement = { kind: 'crew' | 'prop' | 'broom' | 'dust'; x: number; y: number; w: number; h: number }

export type Grid = { columns: number; rows: number; layout: SceneLayout; cells: Cell[]; placements: Placement[] }

export const DEFAULT_COLOR = 0x01000000
export const MAX_COLUMNS = 96
// A stage narrower than this draws the bar.
export const MIN_STAGE_COLUMNS = 30
export const ROWS: Record<SceneLayout, number> = { bar: 2, small: 6, medium: 9, full: 12 }
const STAGE_ROWS: Record<StageSize, number> = { small: 3, medium: 6, full: 9 }
const STAGE_TOP = 1
const WING = 1

// xterm 256-colour palette values (cube levels 00 5f 87 af d7 ff; greys 08 + 10n).
const VELVET = [0x5f0000, 0x870000, 0xaf0000, 0x870000]
const VALANCE = 0x5f0000
const WINGS = 0x5f0000
const GOLD = 0xd7af00
const CLAWD = 0xd7875f
const EYE = 0x000000
const HANDLE = 0x875f00
const BRISTLE = [0xd7af5f, 0xaf875f]
const DUST = [0xbcbcbc, 0x8a8a8a, 0x6c6c6c, 0x4e4e4e]
const BOARD = 0xaf875f
const BOARD_SHADE = 0x875f00
const RAIL = 0x4e4e4e
const TEXT = 0xbcbcbc
const ALERT = 0xffd700

// A crew member: Clawd at two rows by four. The body row's inner corners (the
// missing quarter of ▛ and ▜) are his eyes; the second row is his legs.
const CREW_W = 4
const CREW_UP = ['▐▛▜▌', '▝▘▝▘']
const CREW_STEP = ['▐▛▜▌', '▘▝▘▝']
const CREW_BOW = ['▗▄▄▖', '▝▘▝▘']
const CREW_EYES = new Set([1, 2])

type PropKind = 'crate' | 'chair' | 'plant' | 'flat' | 'ladder' | 'spotlight'
const PROP_ART: Record<PropKind, { rows: readonly string[]; colors: readonly number[] }> = {
  crate: { rows: ['▛▜', '▙▟'], colors: [0xaf875f, 0xaf875f] },
  chair: { rows: ['▌  ', '▛▀▌'], colors: [0x875f5f, 0x875f5f] },
  plant: { rows: ['▞▚', '▜▛'], colors: [0x5faf00, 0xaf5f00] },
  flat: { rows: ['▛▀▀▜', '▙▄▄▟'], colors: [0x5f87af, 0x5f87af] },
  ladder: { rows: ['├┤', '├┤'], colors: [0xbcbcbc, 0xbcbcbc] },
  spotlight: { rows: ['▟█', '▐ '], colors: [0xd7d7d7, 0x8a8a8a] },
}
// The order props are set out in; the flat is pushed, the rest are carried.
const PROP_ORDER: readonly PropKind[] = ['crate', 'chair', 'plant', 'flat', 'ladder', 'spotlight']
const DUST_GLYPHS = ['.', ':', "'", '`']

// The sweeper's slot: room to sweep three cells each way with the broom behind.
const SWEEP_AMP = 3
const SWEEP_SLOT = 16
// When the last prop is gone and only the sweeper is left.
const WORK_START = 0.04
const WORK_END = 0.88
const OVERLAP = 0.35

export function crewFor(columns: number): number {
  return columns >= 76 ? 5 : columns >= 56 ? 4 : columns >= 40 ? 3 : 2
}

export function propsFor(columns: number): number {
  return columns >= 76 ? 6 : columns >= 56 ? 4 : columns >= 40 ? 3 : 2
}

// The layout a band can hold: the size asked for, or the next one down.
export function layout(bodyColumns: number, maxRows: number, size: StageSize): { columns: number; rows: number; layout: SceneLayout } {
  const columns = Math.max(1, Math.min(MAX_COLUMNS, Math.floor(bodyColumns) || 1))
  const order: StageSize[] = size === 'full' ? ['full', 'medium', 'small'] : size === 'medium' ? ['medium', 'small'] : ['small']
  const fits = (l: StageSize) => columns >= MIN_STAGE_COLUMNS && maxRows >= ROWS[l] + 1
  const chosen: SceneLayout = order.find(fits) ?? 'bar'
  return { columns, rows: ROWS[chosen], layout: chosen }
}

// ------------------------------------------------------------------ the cast

type Prop = { kind: PropKind; x: number; w: number; owner: number; isPushed: boolean }
type Task = { prop: number; from: number; to: number }
type Mover = { home: number; side: -1 | 1; tasks: Task[]; exitAt: number }
export type Cast = { columns: number; props: Prop[]; movers: Mover[]; sweepSlot: number }

// Who stands where and when each job runs, for a stage `columns` wide. The
// same width always gives the same cast, so a frame is a pure function of
// progress and the animation counter.
export function cast(columns: number): Cast {
  const crew = crewFor(columns)
  const movers = crew - 1
  const props: Prop[] = PROP_ORDER.slice(0, propsFor(columns)).map((kind, i) => ({
    kind,
    x: 0,
    w: PROP_ART[kind].rows[0]!.length,
    owner: i % movers,
    isPushed: kind === 'flat',
  }))
  // Movers alternate sides; the first to go stands nearest its wing.
  const side = (m: number): -1 | 1 => (m % 2 === 0 ? -1 : 1)
  const owned = (m: number) => props.map((p, i) => ({ p, i })).filter(({ p }) => p.owner === m).map(({ i }) => i)
  const pushed = (m: number) => owned(m).filter((i) => props[i]!.isPushed)
  const carried = (m: number) => owned(m).filter((i) => !props[i]!.isPushed)
  // Each mover stands between its pushed set piece (wing side) and its carried
  // props (stage side): it pushes the piece straight out, then fetches each
  // carried prop nearest first and walks out ahead of it. No walk crosses a
  // prop still on stage. Left to right: left groups outermost first, the
  // sweeper's slot, right groups innermost first.
  type Item = { w: number; place: (x: number) => void }
  const items: Item[] = []
  const left = Array.from({ length: movers }, (_, m) => m).filter((m) => side(m) === -1)
  const right = Array.from({ length: movers }, (_, m) => m).filter((m) => side(m) === 1).reverse()
  const homes: number[] = Array.from({ length: movers }, () => 0)
  const prop = (i: number): Item => ({ w: props[i]!.w, place: (x) => (props[i]!.x = x) })
  for (const m of left) {
    for (const i of pushed(m)) items.push(prop(i))
    items.push({ w: CREW_W, place: (x) => (homes[m] = x) })
    for (const i of carried(m)) items.push(prop(i))
  }
  let sweepSlot = 0
  items.push({ w: SWEEP_SLOT, place: (x) => (sweepSlot = x) })
  for (const m of right) {
    for (const i of carried(m).reverse()) items.push(prop(i))
    items.push({ w: CREW_W, place: (x) => (homes[m] = x) })
    for (const i of pushed(m)) items.push(prop(i))
  }
  const lo = WING + 1
  const room = columns - WING - 1 - lo
  const used = items.reduce((a, it) => a + it.w, 0)
  const gap = Math.max(0, Math.floor((room - used) / (items.length + 1)))
  let x = lo + gap + Math.max(0, Math.floor((room - used - gap * (items.length + 1)) / 2))
  for (const it of items) {
    it.place(x)
    x += it.w + gap
  }
  // Each prop gets an equal share of the work, in the order the movers go; a
  // mover's own props run back to back and movers overlap a little.
  const unit = (WORK_END - WORK_START) / Math.max(1, props.length)
  let index = 0
  const movers_: Mover[] = []
  for (let m = 0; m < movers; m++) {
    // The set piece first, then the carried props, the one nearest the wing first.
    const nearest = carried(m).sort((a, b) => (side(m) === -1 ? props[a]!.x - props[b]!.x : props[b]!.x - props[a]!.x))
    const mine = [...pushed(m), ...nearest]
    const start = Math.max(0.01, WORK_START + index * unit - (m === 0 ? 0 : OVERLAP * unit))
    const end = Math.min(WORK_END, WORK_START + (index + mine.length) * unit)
    const span = (end - start) / Math.max(1, mine.length)
    const tasks = mine.map((prop, k) => ({ prop, from: start + k * span, to: start + (k + 1) * span }))
    index += mine.length
    movers_.push({ home: homes[m]!, side: side(m), tasks, exitAt: tasks.length ? tasks[tasks.length - 1]!.to : start })
  }
  return { columns, props, movers: movers_, sweepSlot }
}

// A pushed set piece leads its mover to the wing; a carried prop trails it.
// Whether the prop is on the mover's left, for a mover heading to `side`.
function propOnLeft(mover: Mover, prop: Prop): boolean {
  return prop.isPushed ? mover.side === -1 : mover.side === 1
}

// Where the mover stands to take the prop: beside it, on the side it pulls or pushes from.
function pickupX(c: Cast, mover: Mover, prop: Prop): number {
  return propOnLeft(mover, prop) ? prop.x + prop.w : prop.x - CREW_W
}

// Where the mover stands when mover and prop together reach the wing.
function exitX(c: Cast, mover: Mover, prop: Prop): number {
  const group = CREW_W + prop.w
  const left = mover.side === -1 ? WING : c.columns - WING - group
  return propOnLeft(mover, prop) ? left + prop.w : left
}

// Where a held prop sits against its mover.
function heldX(mover: Mover, prop: Prop, crewX: number): number {
  return propOnLeft(mover, prop) ? crewX - prop.w : crewX + CREW_W
}

type Pose = { x: number; holding: number | null; isWalking: boolean; dir: -1 | 1 }

// A mover at `progress`: null once it has left. Constant speed along each leg.
function poseOf(c: Cast, mover: Mover, progress: number): Pose | null {
  if (progress >= mover.exitAt) return null
  let x = mover.home
  for (const task of mover.tasks) {
    const prop = c.props[task.prop]!
    const pick = pickupX(c, mover, prop)
    const exit = exitX(c, mover, prop)
    if (progress < task.from) return { x, holding: null, isWalking: false, dir: mover.side }
    if (progress < task.to) {
      const q = (progress - task.from) / (task.to - task.from)
      const walk = Math.abs(pick - x)
      const carry = Math.abs(exit - pick)
      const total = Math.max(1, walk + carry)
      const d = q * total
      if (d < walk) return { x: Math.round(x + Math.sign(pick - x) * d), holding: null, isWalking: true, dir: pick >= x ? 1 : -1 }
      return { x: Math.round(pick + Math.sign(exit - pick) * (d - walk)), holding: task.prop, isWalking: true, dir: mover.side }
    }
    x = exit
  }
  return { x, holding: null, isWalking: false, dir: mover.side }
}

// How many crew and props are on stage at `progress`. The sweeper stays to the end.
export function census(c: Cast, progress: number): { crew: number; props: number } {
  const crew = 1 + c.movers.filter((m) => progress < m.exitAt).length
  const props = c.props.filter((_, i) => {
    const mover = c.movers[c.props[i]!.owner]!
    const task = mover.tasks.find((t) => t.prop === i)
    return !task || progress < task.to
  }).length
  return { crew, props }
}

// ------------------------------------------------------------------ drawing

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

function centred(grid: Grid, y: number, s: string, fg: number, tone: Tone): void {
  const chars = [...s].slice(0, grid.columns)
  const x0 = Math.floor((grid.columns - chars.length) / 2)
  chars.forEach((ch, i) => put(grid, x0 + i, y, ch, fg, at(grid, x0 + i, y)?.bg ?? DEFAULT_COLOR, tone))
}

function velvet(grid: Grid, x: number): number {
  const seam = (grid.columns - 1) / 2
  return VELVET[Math.floor(Math.abs(x - seam)) % VELVET.length]!
}

function art(grid: Grid, kind: Placement['kind'], rows: readonly string[], colors: readonly number[], x: number, y: number, tone: Tone, eyes?: Set<number>): void {
  grid.placements.push({ kind, x, y, w: rows[0]!.length, h: rows.length })
  rows.forEach((row, r) => {
    let c = 0
    for (const ch of row) {
      if (ch !== ' ') {
        const behind = at(grid, x + c, y + r)?.bg ?? DEFAULT_COLOR
        put(grid, x + c, y + r, ch, colors[r] ?? colors[0]!, r === 0 && eyes?.has(c) ? EYE : behind, tone)
      }
      c++
    }
  })
}

function crewArt(grid: Grid, x: number, top: number, rows: readonly string[]): void {
  art(grid, 'crew', rows, [CLAWD, CLAWD], x, top, 'clawd', CREW_EYES)
}

function propArt(grid: Grid, prop: Prop, x: number, top: number): void {
  const a = PROP_ART[prop.kind]
  art(grid, 'prop', a.rows, a.colors, x, top, 'prop')
}

function walking(frame: number): readonly string[] {
  return frame % 2 === 0 ? CREW_UP : CREW_STEP
}

// The sweeper: back and forth within his slot, broom trailing, dust behind.
// Near the end he closes in on the last spot, a little pile that shrinks.
function sweeper(grid: Grid, c: Cast, progress: number, frame: number, crewTop: number): void {
  const slot = c.sweepSlot
  const centre = slot + Math.floor((SWEEP_SLOT - CREW_W) / 2)
  const amp = progress < 0.85 ? SWEEP_AMP : 1
  const span = 2 * amp
  const p = frame % (2 * span)
  const offset = p < span ? p - amp : amp - (p - span)
  const dir: -1 | 1 = p < span ? 1 : -1
  const x = centre + offset
  if (progress >= 0.85) {
    const pile = Math.max(1, Math.round(((1 - progress) / 0.15) * 3))
    const pileX = centre + (dir === 1 ? -3 : CREW_W + 1)
    grid.placements.push({ kind: 'dust', x: pileX, y: crewTop + 1, w: pile, h: 1 })
    for (let i = 0; i < pile; i++) put(grid, pileX + i, crewTop + 1, i % 2 ? '.' : ':', DUST[1]!, DEFAULT_COLOR, 'dust')
  }
  const handleX = dir === 1 ? x - 1 : x + CREW_W
  const brushX = dir === 1 ? x - 2 : x + CREW_W
  grid.placements.push({ kind: 'broom', x: Math.min(handleX, brushX), y: crewTop, w: 2, h: 2 })
  put(grid, handleX, crewTop, dir === 1 ? '/' : '\\', HANDLE, DEFAULT_COLOR, 'broom')
  put(grid, brushX, crewTop + 1, frame % 2 ? '▓' : '▒', BRISTLE[frame % 2]!, DEFAULT_COLOR, 'broom')
  put(grid, brushX + 1, crewTop + 1, frame % 2 ? '▒' : '▓', BRISTLE[(frame + 1) % 2]!, DEFAULT_COLOR, 'broom')
  for (let i = 0; i < 2; i++) {
    const age = (frame + i * 2) % DUST_GLYPHS.length
    const dx = dir === 1 ? brushX - 1 - age : brushX + 2 + age
    if (dx < slot || dx >= slot + SWEEP_SLOT) continue
    grid.placements.push({ kind: 'dust', x: dx, y: crewTop, w: 1, h: 1 })
    put(grid, dx, crewTop + (age < 2 ? 1 : 0), DUST_GLYPHS[age]!, DUST[age]!, DEFAULT_COLOR, 'dust')
  }
  crewArt(grid, x, crewTop, walking(frame))
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
      const v = velvet(grid, x)
      const topColor = 2 * r === covered - 1 ? GOLD : v
      const y = STAGE_TOP + r
      if (bottomHalf) {
        const bottomColor = 2 * r + 1 === covered - 1 ? GOLD : v
        if (bottomColor === topColor) put(grid, x, y, '█', topColor, DEFAULT_COLOR, 'curtain')
        else put(grid, x, y, '▀', topColor, bottomColor, 'curtain')
      } else {
        put(grid, x, y, '▀', topColor, at(grid, x, y)?.bg ?? DEFAULT_COLOR, topColor === GOLD ? 'gold' : 'curtain')
      }
    }
  }
}

// How far the curtain comes down while the work runs: over the sky only,
// never over the crew's rows (or a lifted prop's).
export function runningDrop(stageRows: number, progress: number): number {
  const reach = stageRows >= 6 ? (stageRows - 3) / stageRows : (stageRows - 2) / stageRows
  return Math.max(0, Math.min(1, progress)) * reach
}

function stage(input: SceneInput, columns: number, size: StageSize): Grid {
  const stageRows = STAGE_ROWS[size]
  const rows = ROWS[size]
  const floorRow = STAGE_TOP + stageRows
  const crewTop = floorRow - 2
  const grid: Grid = { columns, rows, layout: size, cells: blank(columns, rows), placements: [] }
  const c = cast(columns)
  const isOpen = input.phase === 'running' || input.phase === 'halted' || input.phase === 'finale'
  // In the finale the last movers hurry off as the curtain falls.
  const progress = input.phase === 'finale' ? input.progress + (0.99 - input.progress) * Math.max(0, input.fall) : input.progress
  if (isOpen) {
    const poses = c.movers.map((m) => poseOf(c, m, progress))
    const held = new Set(poses.flatMap((p) => (p && p.holding !== null ? [p.holding] : [])))
    // Props still where they were set, then the crew, then what they carry.
    c.props.forEach((prop, i) => {
      if (held.has(i)) return
      const owner = c.movers[prop.owner]!
      const task = owner.tasks.find((t) => t.prop === i)
      if (task && progress >= task.to) return
      propArt(grid, prop, prop.x, crewTop)
    })
    const lift = stageRows >= 6 ? 1 : 0
    poses.forEach((pose, m) => {
      if (!pose) return
      const frames = pose.isWalking ? walking(input.frame + m) : (input.frame + m) % 12 < 9 ? CREW_UP : CREW_STEP
      crewArt(grid, pose.x, crewTop, frames)
      if (pose.holding !== null) {
        const prop = c.props[pose.holding]!
        propArt(grid, prop, heldX(c.movers[m]!, prop, pose.x), crewTop - (prop.isPushed ? 0 : lift))
      }
    })
    sweeper(grid, c, progress, input.frame, crewTop)
  }
  // The wings, then the curtain over everything.
  for (let y = STAGE_TOP; y < floorRow; y++) {
    put(grid, 0, y, '█', WINGS, DEFAULT_COLOR, 'curtain')
    put(grid, columns - 1, y, '█', WINGS, DEFAULT_COLOR, 'curtain')
  }
  const drop = isOpen ? runningDrop(stageRows, input.progress) + (input.phase === 'finale' ? (1 - runningDrop(stageRows, input.progress)) * input.fall : 0) : 1
  curtain(grid, drop, stageRows)
  if (input.phase === 'bow') {
    // One Clawd runs out from the wing to centre stage, bows, and stands.
    const centre = Math.floor((columns - CREW_W) / 2)
    const run = Math.min(1, input.phaseFrame / 6)
    const x = Math.round(WING + 1 + (centre - WING - 1) * run)
    const bowing = input.phaseFrame >= 7 && input.phaseFrame < 13
    crewArt(grid, x, crewTop, bowing ? CREW_BOW : run < 1 ? walking(input.phaseFrame) : CREW_UP)
  }
  ;(input.banner ?? []).forEach((line, i) => centred(grid, STAGE_TOP + i, line, ALERT, input.phase === 'halted' ? 'alert' : 'gold'))
  for (let x = 0; x < columns; x++) {
    put(grid, x, 0, '▀', VALANCE, x % 6 === 3 ? VALANCE : GOLD, 'gold')
    put(grid, x, floorRow, '▀', x % 9 === 0 ? BOARD_SHADE : BOARD, BOARD_SHADE, 'floor')
  }
  const isHeld = input.phase === 'halted'
  text(grid, 0, floorRow + 1, [...input.status].slice(0, columns).join(''), isHeld ? ALERT : TEXT, DEFAULT_COLOR, isHeld ? 'alert' : 'text')
  return grid
}

// Too small for any stage: the progress as a velvet bar, and the status.
function bar(input: SceneInput, columns: number): Grid {
  const grid: Grid = { columns, rows: ROWS.bar, layout: 'bar', cells: blank(columns, ROWS.bar), placements: [] }
  const closed = input.phase === 'running' || input.phase === 'halted' ? input.progress : input.phase === 'finale' ? input.progress + (1 - input.progress) * input.fall : 1
  const filled = Math.max(0, Math.min(columns, Math.round(closed * columns)))
  for (let x = 0; x < columns; x++) {
    if (x < filled) put(grid, x, 0, '▀', velvet(grid, x), GOLD, 'curtain')
    else put(grid, x, 0, '▔', RAIL, DEFAULT_COLOR, 'floor')
  }
  const isHeld = input.phase === 'halted'
  text(grid, 0, 1, [...input.status].slice(0, columns).join(''), isHeld ? ALERT : TEXT, DEFAULT_COLOR, isHeld ? 'alert' : 'text')
  return grid
}

export function buildGrid(input: SceneInput): Grid {
  const shape = layout(input.columns, input.maxRows, input.size)
  return shape.layout === 'bar' ? bar(input, shape.columns) : stage(input, shape.columns, shape.layout)
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

// The grid as plain lines, for reading a frame in a test or a preview.
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
