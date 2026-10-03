// Renders the curtain show to an HTML page, from the real scene module, so the
// owner can see the frames in colour before trying them in a session: each
// cell's character with its foreground and background, exactly what the
// terminal is handed.
//
//   node --experimental-strip-types tools/preview.ts            writes preview/curtain-v4.html
//   node --experimental-strip-types tools/preview.ts --text     prints the pixels as letters
//
// Not part of the hooks module: nothing in a session runs it.

import { writeFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { DEFAULT_COLOR, type Grid, type SceneInput, type StageSize, buildGrid, cast, census } from '../hooks/scene.ts'

const COLUMNS = 80
const BASE: SceneInput = { columns: COLUMNS, maxRows: 40, size: 'small', progress: 0, fall: 0, frame: 3, phase: 'running', phaseFrame: 0, status: '' }
const STATUS: Record<string, string> = {
  '0': '1/3 merge-pr 0:00 / ~8m',
  '25': '1/3 merge-pr 1:41 / ~8m',
  '50': '1/3 merge-pr 4:05 / ~8m',
  '75': '2/3 session-close 1:16 / ~4m',
  '95': '2/3 session-close 4:02 / ~4m',
}

type Frame = { title: string; grid: Grid }

function frames(size: StageSize): Frame[] {
  const out: Frame[] = []
  for (const pct of [0, 25, 50, 75, 95]) {
    const p = pct / 100
    const n = census(cast(COLUMNS), p)
    out.push({
      title: `${size}, ${pct}%: ${n.crew} crew, ${n.props} props`,
      grid: buildGrid({ ...BASE, size, progress: p, frame: 3 + pct, status: STATUS[String(pct)]! }),
    })
  }
  out.push({ title: `${size}, the bow`, grid: buildGrid({ ...BASE, size, phase: 'bow', phaseFrame: 9, progress: 0.99, fall: 1, status: 'Clawd takes a bow' }) })
  out.push({
    title: `${size}, the fin`,
    grid: buildGrid({ ...BASE, size, phase: 'fin', progress: 0.99, fall: 1, banner: ['~ fin ~', 'Thank you, goodnight'], status: 'fin - close lane lane-7 from the panel - /exit follows' }),
  })
  out.push({
    title: `${size}, INTERMISSION at 40%`,
    grid: buildGrid({ ...BASE, size, phase: 'halted', progress: 0.4, frame: 40, banner: ['INTERMISSION'], status: 'INTERMISSION - merge-pr stopped: PR #42 is OPEN, not MERGED' }),
  })
  return out
}

function hex(color: number, fallback: string): string {
  return color === DEFAULT_COLOR ? fallback : '#' + color.toString(16).padStart(6, '0')
}

function escape(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function html(all: Frame[]): string {
  const blocks = all.map(({ title, grid }) => {
    const rows: string[] = []
    for (let y = 0; y < grid.rows; y++) {
      let row = ''
      let run = ''
      let style = ''
      for (const cell of grid.cells.slice(y * grid.columns, (y + 1) * grid.columns)) {
        const s = `color:${hex(cell.fg, '#bcbcbc')};background:${hex(cell.bg, 'transparent')}`
        if (s !== style && run) {
          row += `<span style="${style}">${escape(run)}</span>`
          run = ''
        }
        style = s
        run += cell.ch
      }
      if (run) row += `<span style="${style}">${escape(run)}</span>`
      rows.push(row)
    }
    return `<h2>${escape(title)}</h2>\n<pre>${rows.join('\n')}</pre>`
  })
  return `<!doctype html>
<html><head><meta charset="utf-8"><title>Curtain v4 preview</title>
<style>
body { background: #0a0a0a; color: #bcbcbc; font-family: system-ui, sans-serif; margin: 24px; }
h1 { font-size: 18px; } h2 { font-size: 14px; font-weight: 500; margin: 24px 0 6px; color: #d7af00; }
pre { font-family: Menlo, "DejaVu Sans Mono", monospace; font-size: 15px; line-height: 1; margin: 0; display: inline-block; background: #121212; padding: 6px; }
</style></head><body>
<h1>Curtain v4: ${COLUMNS} columns, rendered from hooks/scene.ts</h1>
${blocks.join('\n')}
</body></html>
`
}

// Every distinct colour as a letter, for reading the pixels in a terminal.
function pixels(grid: Grid): string {
  const letters = new Map<number, string>()
  const pool = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  const letter = (c: number) => {
    if (c === 0x121212 || c === DEFAULT_COLOR) return ' '
    if (!letters.has(c)) letters.set(c, pool[letters.size % pool.length]!)
    return letters.get(c)!
  }
  const lines: string[] = []
  for (let y = 0; y < grid.rows; y++) {
    let top = ''
    let bottom = ''
    for (const cell of grid.cells.slice(y * grid.columns, (y + 1) * grid.columns)) {
      if (cell.ch === '▀') { top += letter(cell.fg); bottom += letter(cell.bg) }
      else if (cell.ch === '█') { top += letter(cell.fg); bottom += letter(cell.fg) }
      else if (cell.ch === ' ') { top += ' '; bottom += ' ' }
      else { top += cell.ch; bottom += cell.ch }
    }
    lines.push(top, bottom)
  }
  return lines.join('\n') + '\n' + [...letters].map(([c, l]) => l + '=' + c.toString(16)).join(' ')
}

const all = [...frames('small'), ...frames('medium')]
if (process.argv.includes('--text')) {
  for (const f of all) console.log('--- ' + f.title + '\n' + pixels(f.grid))
} else {
  const here = dirname(fileURLToPath(import.meta.url))
  const out = join(here, '..', 'preview', 'curtain-v4.html')
  mkdirSync(dirname(out), { recursive: true })
  writeFileSync(out, html(all))
  console.log(out)
}
