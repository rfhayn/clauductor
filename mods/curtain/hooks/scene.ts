// The stage, as pure functions: one frame in, one grid of cells out. No `$`,
// so the hooks module calls it per frame and the tests and the preview script
// read frames directly.
//
// Pixel art in half blocks: the stage is a canvas of pixels two to a cell, one
// above the other, drawn as `▀` with the upper pixel as the foreground colour
// and the lower as the background (a Raster cell carries both). A stage crew of
// Clawds strikes the set while the work runs. Progress (the weighted travel,
// which never passes a step's boundary before the step completes) decides who
// is still on stage: each mover lifts its props overhead and carries them into
// the nearest wing, pushes a set piece out in front, and leaves with its last;
// the curtain comes down over the sky; one sweeper stays to sweep the last spot.
// The finale closes the curtain, one Clawd runs out and bows, then the fin.
//
// Sizes (rows): small 10, medium 12, full 16: the valance, the stage (7, 9 or
// 13 rows, 14, 18 or 26 pixels), the floor and the status. A band too short or
// too narrow for the size asked for gets the next one down, and below `small`
// a two-row bar (no sprites), so a sprite is never clipped.
//
// What reaches the terminal, for tmux inside xterm.js (the panel's lanes):
// - characters: `▀` `█`, the space and printable ASCII only (all in xterm.js's
//   own cell-exact drawing, none of ambiguous width).
// - colours: every RGB value is one of the xterm 256-colour palette's (the
//   6x6x6 cube or the grey ramp). The panel's tmux gives xterm-256color no RGB
//   feature and maps truecolour down to 256; palette colours pass unchanged.

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

/**
 * A sprite as drawn, in cells (x, y, w, h) and in pixels (px, py, pw, ph), so a
 * test can check nothing was clipped and nothing walked through anything.
 */
export type Placement = {
  kind: 'crew' | 'prop' | 'broom' | 'dust' | 'bow'
  pose?: string
  x: number
  y: number
  w: number
  h: number
  px: number
  py: number
  pw: number
  ph: number
}

export type Grid = { columns: number; rows: number; layout: SceneLayout; cells: Cell[]; placements: Placement[] }

export const DEFAULT_COLOR = 0x01000000
export const MAX_COLUMNS = 140
// A stage narrower than this draws the bar.
export const MIN_STAGE_COLUMNS = 30
export const ROWS: Record<SceneLayout, number> = { bar: 2, small: 10, medium: 12, full: 16 }
export const STAGE_ROWS: Record<StageSize, number> = { small: 7, medium: 9, full: 13 }
// Side drapes, in columns.
const WING = 2

// ------------------------------------------------------------------ palette
// xterm 256-colour palette values only (cube levels 00 5f 87 af d7 ff; greys 08 + 10n).

const C = {
  backdrop: 0x121212,
  velvet: [0x5f0000, 0x870000, 0xaf0000, 0xaf0000, 0x870000, 0x5f0000],
  wing: [0x5f0000, 0x870000],
  gold: 0xd7af00,
  goldDark: 0xaf8700,
  board: 0xaf875f,
  boardLight: 0xd7af87,
  grain: 0x875f00,
  text: 0xbcbcbc,
  alert: 0xffd700,
} as const
const FLOOR_EDGE = 0x875f00

// The legend the pixel art is drawn in: one character per pixel, `.` clear.
const LEGEND: Record<string, number> = {
  o: 0xd7875f, // Clawd: terracotta
  O: 0xaf5f5f, // Clawd: shade
  h: 0xffaf87, // Clawd: highlight
  e: 0x1c1c1c, // Clawd: eyes
  w: 0xaf875f, // wood
  W: 0x875f00, // wood, dark
  l: 0xd7af87, // wood, light
  g: 0x5faf00, // leaf
  G: 0x008700, // leaf, dark
  p: 0xaf5f00, // clay pot
  P: 0xd7875f, // pot rim
  k: 0x444444, // metal, dark
  m: 0x808080, // metal
  y: 0xffffaf, // lens
  Y: 0xffffff, // lens highlight
  c: 0x875f5f, // chair
  C: 0xaf5f5f, // chair seat
  r: 0xd7af5f, // rope
  R: 0xaf875f, // rope, shade
  s: 0xd7af87, // sandbag
  S: 0xaf875f, // sandbag, shade
  t: 0x875f00, // tie
  b: 0x5f87af, // painted flat
  B: 0x87afd7, // painted flat, light
  f: 0x875f00, // flat's frame
  x: 0xd7af5f, // bristles
  X: 0xaf875f, // bristles, shade
  H: 0x875f00, // broom handle
  d: 0xbcbcbc, // dust
  D: 0x808080, // dust, thin
}

type Art = readonly string[]

// Clawd, the Claude Code mascot: a wide terracotta body with two dark eyes,
// short arms and four legs. 11 by 7 pixels.
const CREW_W = 11
const CREW_H = 7
const BODY: Art = ['..ohooooo..', '..oeoooeO..', '..oeoooeO..', 'oooooooooOO', '..OOOOOOO..']
// The walk cycle: one pair of legs down while the other lifts.
const LEGS_A: Art = ['..O.O.O.O..', '..O...O....']
const LEGS_B: Art = ['..O.O.O.O..', '....O...O..']
export const WALK: readonly Art[] = [[...BODY, ...LEGS_A], [...BODY, ...LEGS_B]]
// Carrying: arms up along the body to hold the prop overhead.
const CARRY_BODY: Art = ['.oohoooooo.', '.ooeoooeOo.', '.ooeoooeOo.', '.oooooooOo.', '..OOOOOOO..']
export const CARRY: readonly Art[] = [[...CARRY_BODY, ...LEGS_A], [...CARRY_BODY, ...LEGS_B]]
// Sweeping: the front arm out to the broom, the back arm in.
const SWEEP_BODY: Art = ['..ohooooo..', '..oeoooeO..', '..oeoooeO..', '..oooooooOO', '..OOOOOOO..']
// The bow: head down, eyes to the floor.
export const BOW: Art = ['...........', '...........', '..ohooooo..', '.oooooooOO.', '..oeoooeO..', '..OOOOOOO..', '..O.O.O.O..']

type PropKind = 'crate' | 'ladder' | 'plant' | 'spotlight' | 'sandbag' | 'rope' | 'chair' | 'flat'
// Each prop as it stands, and as it is carried overhead (a ladder goes flat).
const PROPS: Record<PropKind, { stand: Art; carried: Art }> = {
  crate: {
    stand: ['WWWWWWWW', 'WlllllWW', 'WWWWWWWW', 'WwlwwlwW', 'WWWWWWWW', 'WllwwllW'],
    carried: ['WWWWWWWW', 'WlllllWW', 'WWWWWWWW', 'WwlwwlwW', 'WWWWWWWW', 'WllwwllW'],
  },
  ladder: {
    stand: ['w....w', 'wllllw', 'w....w', 'w....w', 'wllllw', 'w....w', 'w....w', 'wllllw', 'w....w', 'w....w', 'wllllw', 'w....w'],
    carried: ['wwwwwwwwwww', '.l..l..l..l', 'wwwwwwwwwww'],
  },
  plant: {
    stand: ['.g.G.g.', 'gGgggGg', '.GgGgG.', '..gGg..', 'PPPPPPP', '.ppppp.', '..ppp..'],
    carried: ['.g.G.g.', 'gGgggGg', '.GgGgG.', 'PPPPPPP', '.ppppp.', '..ppp..'],
  },
  spotlight: {
    stand: ['.kkkk..', 'kmmmmyY', 'kmmmmyy', '.kkkk..', '...m...', '...m...', '.mmmmm.'],
    carried: ['.kkkk..', 'kmmmmyY', 'kmmmmyy', '.kkkk..', '...m...', '.mmmmm.'],
  },
  sandbag: {
    stand: ['..tt..', '.ssss.', 'sSssSs', 'ssssss'],
    carried: ['..tt..', '.ssss.', 'sSssSs', 'ssssss'],
  },
  rope: {
    stand: ['.rRrRr.', 'rR...Rr', 'Rr.r.rR', '.rRrRr.'],
    carried: ['.rRrRr.', 'rR...Rr', 'Rr.r.rR', '.rRrRr.'],
  },
  chair: {
    stand: ['c.....', 'c.....', 'c.....', 'cCCCCC', 'c....c', 'c....c'],
    carried: ['c.....', 'c.....', 'cCCCCC', 'c....c', 'c....c', 'c....c'],
  },
  flat: {
    stand: ['ffffffffff', 'fbbbbbbbbf', 'fbbBbbbbbf', 'fbBBBbbbbf', 'fbbBbbbBbf', 'fbbbbbBBBf', 'fbbbbbbBbf', 'fbbbbbbbbf', 'ffffffffff', '.k......k.'],
    carried: ['ffffffffff'],
  },
}
// The order props are set out in, as the stage widens. The flat is pushed.
const PROP_ORDER: readonly PropKind[] = ['crate', 'ladder', 'plant', 'spotlight', 'sandbag', 'rope', 'chair', 'flat']
const CARRIED_MAX_H = 6

// The broom, in front of the sweeper (heading right; mirrored heading left),
// in two positions of its swing.
const BROOMS: readonly Art[] = [
  ['...', '...', '...', 'H..', '.H.', '..H', 'xXx'],
  ['...', '...', '...', 'H..', 'H..', '.H.', 'XxX'],
]
const DUST: readonly Art[] = [['.d.', 'ddd'], ['d.D', '.D.'], ['.D.', '...']]

// The sweeper's slot: him and his broom on either side.
const SWEEP_SLOT = CREW_W + 2 * 3
const WORK_START = 0.04
const WORK_END = 0.88
const OVERLAP = 0.35

const TONES = new Map<number, Tone>([
  ...C.velvet.map((c) => [c, 'curtain'] as const),
  [C.gold, 'gold'],
  [LEGEND.o!, 'clawd'],
  [LEGEND.O!, 'clawd'],
  [LEGEND.h!, 'clawd'],
  [LEGEND.x!, 'broom'],
  [LEGEND.H!, 'broom'],
  [LEGEND.d!, 'dust'],
  [C.board, 'floor'],
  [C.text, 'text'],
  [C.alert, 'alert'],
])

function propWidth(kind: PropKind): number {
  return PROPS[kind].stand[0]!.length
}

// How many movers and props fit a stage `columns` wide, with a cell between
// neighbours: the most crew and props that fit, more crew first.
export function lineup(columns: number): { movers: number; props: PropKind[] } {
  const room = columns - 2 * WING
  let best = { movers: 0, props: [] as PropKind[], score: -1 }
  for (let m = 0; m <= 3; m++) {
    for (let n = m === 0 ? 0 : m; n <= (m === 0 ? 0 : PROP_ORDER.length); n++) {
      const props = PROP_ORDER.slice(0, n)
      const need = m * CREW_W + SWEEP_SLOT + props.reduce((a, k) => a + propWidth(k), 0) + (m + n + 2)
      if (need > room) continue
      const score = m + n
      if (score > best.score || (score === best.score && m > best.movers)) best = { movers: m, props, score }
    }
  }
  return { movers: best.movers, props: best.props }
}

export function crewFor(columns: number): number {
  return lineup(columns).movers + 1
}

export function propsFor(columns: number): number {
  return lineup(columns).props.length
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
  const plan = lineup(columns)
  const movers = plan.movers
  const props: Prop[] = plan.props.map((kind, i) => ({ kind, x: 0, w: propWidth(kind), owner: movers ? i % movers : 0, isPushed: kind === 'flat' }))
  // Movers alternate sides; the first to go stands nearest its wing.
  const side = (m: number): -1 | 1 => (m % 2 === 0 ? -1 : 1)
  const owned = (m: number) => props.map((p, i) => ({ p, i })).filter(({ p }) => p.owner === m).map(({ i }) => i)
  const pushed = (m: number) => owned(m).filter((i) => props[i]!.isPushed)
  const carried = (m: number) => owned(m).filter((i) => !props[i]!.isPushed)
  // Each mover stands between its pushed set piece (wing side) and its carried
  // props (stage side): it pushes the piece straight out, then fetches each
  // carried prop nearest first and carries it out overhead. No walk crosses a
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
  const lo = WING
  const room = columns - 2 * WING
  const used = items.reduce((a, it) => a + it.w, 0)
  const gap = Math.max(1, Math.floor((room - used) / (items.length + 1)))
  let x = lo + Math.max(0, Math.floor((room - used - gap * (items.length - 1)) / 2))
  for (const it of items) {
    it.place(x)
    x += it.w + gap
  }
  // Each prop gets an equal share of the work, in the order the movers go; a
  // mover's own props run back to back and movers overlap a little.
  const unit = (WORK_END - WORK_START) / Math.max(1, props.length)
  let index = 0
  const list: Mover[] = []
  for (let m = 0; m < movers; m++) {
    // The set piece first, then the carried props, the one nearest the wing first.
    const nearest = carried(m).sort((a, b) => (side(m) === -1 ? props[a]!.x - props[b]!.x : props[b]!.x - props[a]!.x))
    const mine = [...pushed(m), ...nearest]
    const start = Math.max(0.01, WORK_START + index * unit - (m === 0 ? 0 : OVERLAP * unit))
    const end = Math.min(WORK_END, WORK_START + (index + mine.length) * unit)
    const span = (end - start) / Math.max(1, mine.length)
    const tasks = mine.map((p, k) => ({ prop: p, from: start + k * span, to: start + (k + 1) * span }))
    index += mine.length
    list.push({ home: homes[m]!, side: side(m), tasks, exitAt: tasks.length ? tasks[tasks.length - 1]!.to : start })
  }
  return { columns, props, movers: list, sweepSlot }
}

// Where the mover stands to take a prop: a pushed piece from its stage side,
// a carried one from its wing side, before lifting it overhead.
function pickupX(mover: Mover, prop: Prop): number {
  const onLeft = prop.isPushed ? mover.side === -1 : mover.side === 1
  return onLeft ? prop.x + prop.w : prop.x - CREW_W
}

// Where the mover stands when it reaches the wing with the prop.
function exitX(c: Cast, mover: Mover, prop: Prop): number {
  if (!prop.isPushed) return mover.side === -1 ? WING : c.columns - WING - CREW_W
  return mover.side === -1 ? WING + prop.w : c.columns - WING - prop.w - CREW_W
}

type Pose = { x: number; holding: number | null; isWalking: boolean; dir: -1 | 1 }

// A mover at `progress`: null once it has left. Constant speed along each leg.
function poseOf(c: Cast, mover: Mover, progress: number): Pose | null {
  if (progress >= mover.exitAt) return null
  let x = mover.home
  for (const task of mover.tasks) {
    const prop = c.props[task.prop]!
    const pick = pickupX(mover, prop)
    const exit = exitX(c, mover, prop)
    if (progress < task.from) return { x, holding: null, isWalking: false, dir: mover.side }
    if (progress < task.to) {
      const q = (progress - task.from) / (task.to - task.from)
      const walk = Math.abs(pick - x)
      const carry = Math.abs(exit - pick)
      const d = q * Math.max(1, walk + carry)
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
  const props = c.props.filter((p, i) => {
    const task = c.movers[p.owner]?.tasks.find((t) => t.prop === i)
    return !task || progress < task.to
  }).length
  return { crew, props }
}

// How far the curtain comes down while the work runs: over the sky only,
// never over the crew or a prop held overhead.
export function runningDrop(stageRows: number, progress: number): number {
  const stagePx = stageRows * 2
  const sky = Math.max(0, stagePx - CREW_H - CARRIED_MAX_H)
  return (Math.max(0, Math.min(1, progress)) * sky) / stagePx
}

// ------------------------------------------------------------------ the canvas

type Canvas = { w: number; h: number; px: number[]; placements: Placement[] }

function canvas(w: number, h: number): Canvas {
  return { w, h, px: Array.from({ length: w * h }, () => C.backdrop), placements: [] }
}

function pset(cv: Canvas, x: number, y: number, color: number): void {
  if (x < 0 || y < 0 || x >= cv.w || y >= cv.h) return
  cv.px[y * cv.w + x] = color
}

function pget(cv: Canvas, x: number, y: number): number {
  return cv.px[y * cv.w + x] ?? C.backdrop
}

function mirror(art: Art): Art {
  return art.map((row) => [...row].reverse().join(''))
}

function draw(cv: Canvas, art: Art, x: number, y: number, kind: Placement['kind'], pose?: string): void {
  const pw = art[0]!.length
  const ph = art.length
  const top = Math.floor(y / 2)
  cv.placements.push({ kind, ...(pose ? { pose } : {}), x, y: top, w: pw, h: Math.ceil((y + ph) / 2) - top, px: x, py: y, pw, ph })
  art.forEach((row, r) => {
    let c = 0
    for (const ch of row) {
      const color = LEGEND[ch]
      if (color !== undefined) pset(cv, x + c, y + r, color)
      c++
    }
  })
}

// Velvet folds two pixels to a shade, dark into light and back, mirrored about
// the centre seam so the two curtains fold alike.
function velvet(w: number, x: number): number {
  const seam = (w - 1) / 2
  return C.velvet[Math.floor(Math.abs(x - seam) / 2) % C.velvet.length]!
}

function sweeper(cv: Canvas, c: Cast, frame: number, crewTop: number): void {
  // He faces one way, then the other, the broom swinging in front of him.
  const dir: -1 | 1 = Math.floor(frame / 16) % 2 === 0 ? 1 : -1
  const x = c.sweepSlot + 3
  const swing = frame % 2
  const broom = dir === 1 ? BROOMS[swing]! : mirror(BROOMS[swing]!)
  const bx = dir === 1 ? x + CREW_W : x - 3
  draw(cv, dir === 1 ? [...SWEEP_BODY, ...LEGS_A] : mirror([...SWEEP_BODY, ...LEGS_A]), x, crewTop, 'crew', 'sweep')
  draw(cv, broom, bx, crewTop, 'broom')
  // A small cloud of dust rising off the bristles.
  const age = frame % 3
  draw(cv, DUST[age]!, bx, crewTop + 3 - age, 'dust')
}

// Draw the stage into a pixel canvas: valance, stage, floor.
function paintStage(input: SceneInput, columns: number, size: StageSize): Canvas {
  const stageRows = STAGE_ROWS[size]
  const stagePx = stageRows * 2
  const h = 2 + stagePx + 2
  const cv = canvas(columns, h)
  const floorTop = 2 + stagePx
  const crewTop = floorTop - CREW_H
  const c = cast(columns)
  const isOpen = input.phase === 'running' || input.phase === 'halted' || input.phase === 'finale'
  // In the finale the last movers hurry off as the curtain falls.
  const progress = input.phase === 'finale' ? input.progress + (0.99 - input.progress) * Math.max(0, input.fall) : input.progress
  if (isOpen) {
    const poses = c.movers.map((m) => poseOf(c, m, progress))
    const held = new Set(poses.flatMap((p) => (p && p.holding !== null ? [p.holding] : [])))
    c.props.forEach((prop, i) => {
      if (held.has(i)) return
      const task = c.movers[prop.owner]?.tasks.find((t) => t.prop === i)
      if (task && progress >= task.to) return
      const art = PROPS[prop.kind].stand
      draw(cv, art, prop.x, floorTop - art.length, 'prop')
    })
    poses.forEach((pose, m) => {
      if (!pose) return
      const step = (input.frame + m) % 2
      const holding = pose.holding === null ? null : c.props[pose.holding]!
      if (holding && !holding.isPushed) {
        draw(cv, CARRY[step]!, pose.x, crewTop, 'crew', 'carry' + step)
        const art = PROPS[holding.kind].carried
        draw(cv, art, pose.x + Math.floor((CREW_W - art[0]!.length) / 2), crewTop - art.length, 'prop')
      } else {
        draw(cv, pose.isWalking ? WALK[step]! : WALK[0]!, pose.x, crewTop, 'crew', pose.isWalking ? 'walk' + step : 'stand')
        if (holding) {
          const art = PROPS[holding.kind].stand
          draw(cv, art, pose.dir === -1 ? pose.x - holding.w : pose.x + CREW_W, floorTop - art.length, 'prop')
        }
      }
    })
    sweeper(cv, c, input.frame, crewTop)
  }
  // The wings, the curtain over everything, the valance and the floor.
  for (let y = 2; y < floorTop; y++) {
    for (let x = 0; x < WING; x++) {
      pset(cv, x, y, C.wing[(x + y) % 2]!)
      pset(cv, columns - 1 - x, y, C.wing[(x + y + 1) % 2]!)
    }
  }
  const drop = isOpen ? runningDrop(stageRows, input.progress) + (input.phase === 'finale' ? (1 - runningDrop(stageRows, input.progress)) * input.fall : 0) : 1
  const covered = Math.max(0, Math.min(stagePx, Math.round(drop * stagePx)))
  for (let r = 0; r < covered; r++) {
    for (let x = 0; x < columns; x++) pset(cv, x, 2 + r, r === covered - 1 ? C.gold : velvet(columns, x))
  }
  if (input.phase === 'bow') {
    // One Clawd runs out from the wing to centre stage, bows, and stands.
    const centre = Math.floor((columns - CREW_W) / 2)
    const run = Math.min(1, input.phaseFrame / 6)
    const x = Math.round(WING + (centre - WING) * run)
    const bowing = input.phaseFrame >= 7 && input.phaseFrame < 13
    if (bowing) draw(cv, BOW, x, crewTop, 'bow', 'bow')
    else draw(cv, run < 1 ? WALK[input.phaseFrame % 2]! : WALK[0]!, x, crewTop, 'crew', 'run')
  }
  for (let x = 0; x < columns; x++) {
    pset(cv, x, 0, C.velvet[0]!)
    pset(cv, x, 1, x % 6 === 3 ? C.goldDark : C.gold)
    // Boards: a light top edge, grain here and there, and a joint every 12.
    const grain = (x * 37 + 11) % 13 < 2
    pset(cv, x, floorTop, x % 12 === 0 ? C.grain : grain ? C.grain : C.board)
    pset(cv, x, floorTop + 1, x % 12 === 0 ? C.grain : (x * 17) % 7 === 0 ? C.boardLight : FLOOR_EDGE)
  }
  return cv
}

function toneOf(color: number): Tone {
  return TONES.get(color) ?? 'plain'
}

// The canvas as cells: `▀` with the upper pixel as foreground and the lower as
// background; a cell of one colour is `█`, an empty one a space.
function cellsOf(cv: Canvas): Cell[] {
  const cells: Cell[] = []
  for (let r = 0; r < cv.h / 2; r++) {
    for (let x = 0; x < cv.w; x++) {
      const top = pget(cv, x, 2 * r)
      const bottom = pget(cv, x, 2 * r + 1)
      if (top === bottom && top === C.backdrop) cells.push({ ch: ' ', fg: C.backdrop, bg: C.backdrop, tone: 'plain' })
      else if (top === bottom) cells.push({ ch: '█', fg: top, bg: C.backdrop, tone: toneOf(top) })
      else cells.push({ ch: '▀', fg: top, bg: bottom, tone: toneOf(top) })
    }
  }
  return cells
}

function blankCells(n: number): Cell[] {
  return Array.from({ length: n }, () => ({ ch: ' ', fg: DEFAULT_COLOR, bg: DEFAULT_COLOR, tone: 'plain' as Tone }))
}

function text(grid: Grid, x: number, y: number, s: string, fg: number, tone: Tone, keepBg: boolean): void {
  let i = 0
  for (const ch of s) {
    const at = y * grid.columns + x + i
    if (x + i >= 0 && x + i < grid.columns && y >= 0 && y < grid.rows) {
      // Over the stage the letters sit on the upper pixel's colour.
      const behind = grid.cells[at]!
      const bg = keepBg ? (behind.ch === '▀' || behind.ch === '█' ? behind.fg : behind.bg) : DEFAULT_COLOR
      grid.cells[at] = { ch, fg, bg, tone }
    }
    i++
  }
}

function stage(input: SceneInput, columns: number, size: StageSize): Grid {
  const cv = paintStage(input, columns, size)
  const rows = ROWS[size]
  const grid: Grid = { columns, rows, layout: size, cells: [...cellsOf(cv), ...blankCells(columns)], placements: cv.placements }
  const isHeld = input.phase === 'halted'
  ;(input.banner ?? []).forEach((line, i) => {
    const chars = [...line].slice(0, columns)
    text(grid, Math.floor((columns - chars.length) / 2), 1 + i, chars.join(''), C.alert, isHeld ? 'alert' : 'gold', true)
  })
  text(grid, 0, rows - 1, [...input.status].slice(0, columns).join(''), isHeld ? C.alert : C.text, isHeld ? 'alert' : 'text', false)
  return grid
}

// Too small for any stage: the progress as a velvet bar, and the status.
function bar(input: SceneInput, columns: number): Grid {
  const grid: Grid = { columns, rows: ROWS.bar, layout: 'bar', cells: blankCells(columns * ROWS.bar), placements: [] }
  const closed = input.phase === 'running' || input.phase === 'halted' ? input.progress : input.phase === 'finale' ? input.progress + (1 - input.progress) * input.fall : 1
  const filled = Math.max(0, Math.min(columns, Math.round(closed * columns)))
  for (let x = 0; x < columns; x++) {
    grid.cells[x] = x < filled ? { ch: '▀', fg: velvet(columns, x), bg: C.gold, tone: 'curtain' } : { ch: '▀', fg: 0x444444, bg: DEFAULT_COLOR, tone: 'floor' }
  }
  const isHeld = input.phase === 'halted'
  text(grid, 0, 1, [...input.status].slice(0, columns).join(''), isHeld ? C.alert : C.text, isHeld ? 'alert' : 'text', false)
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
