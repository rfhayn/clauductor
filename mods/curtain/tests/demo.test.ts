// The demo and the hand-typed closing show: both play to the end with
// nothing run, nothing submitted, nothing recorded, and no /exit. The demo
// shows the real look: the strip while the pretend work runs, the full stage
// for the finale.

import { expect, test } from 'claude-code/testing'
import { MERGED, band, bandText, endTurn, runCommand, stage, startSession } from './stage.js'

const SLOW = { timeoutMs: 30_000 }

test('/curtain-mod-demo: the strip while it runs, the full stage for the finale, no exit, under 50 s', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($,{ command: 'curtain-mod-demo', args: '' })
  expect(String(said.text)).toMatch(/Curtain demo \(small\).*Nothing is run/)
  await clock.advance(1_000)
  let lines = await band($)
  expect(lines.length).toBe(2)
  expect(lines[1]!.startsWith('demo 1/3 merge-pr 0:01 / ~20s')).toBe(true)

  // The first pretend step overruns its 20 s estimate: the bar waits short of the boundary.
  await clock.advance(24_000)
  expect((await band($))[1]!).toMatch(/^demo 1\/3 merge-pr 0:25/)
  await clock.advance(2_500)
  expect((await band($))[1]!).toMatch(/^demo 2\/3 session-close/)
  expect((await band($)).length).toBe(2)

  // The last step ends at 39 s: the finale expands to the full stage.
  await clock.advance(13_000)
  lines = await band($)
  expect(lines.length).toBe(9)
  await clock.advance(3_500)
  expect(await bandText($)).toMatch(/Clawd takes a bow/)
  await clock.advance(3_000)
  expect(await bandText($)).toMatch(/Thank you, goodnight/)

  await endTurn($)
  // Cleared away by 53 s.
  await clock.advance(6_000)
  expect(await band($)).toEqual([])
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.submits).toEqual([])
  expect(rec.runs).toEqual([])
  // A demo teaches the timings nothing.
  expect([...rec.store.keys()]).toEqual([])
})

test('/curtain-mod-demo halt freezes the strip at INTERMISSION, never expands, and exits nothing', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  await runCommand($,{ command: 'curtain-mod-demo', args: 'halt' })
  await clock.advance(10_000)
  const held = await band($)
  expect(held.length).toBe(2)
  expect(held[1]!.startsWith('INTERMISSION · merge-pr stopped: review did not converge')).toBe(true)
  await clock.advance(60_000)
  expect(await band($)).toEqual(held)
  expect(rec.commands).toEqual([])
  // /curtain-mod cancel on a held show clears it.
  await runCommand($,{ command: 'curtain-mod', args: 'cancel' })
  expect(await band($)).toEqual([])
})

test('the demo takes a size and halt, in either order, and refuses anything else', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  await runCommand($,{ command: 'curtain-mod-demo', args: 'full' })
  await clock.advance(1_000)
  expect((await band($)).length).toBe(9)

  await runCommand($,{ command: 'curtain-mod-demo', args: 'halt medium' })
  await clock.advance(10_000)
  const held = await band($)
  expect(held.length).toBe(6)
  expect(held.join('\n')).toMatch(/INTERMISSION/)

  await runCommand($,{ command: 'curtain-mod-demo', args: 'small halt' })
  await clock.advance(1_000)
  expect((await band($)).length).toBe(2)

  const refused = await runCommand($,{ command: 'curtain-mod-demo', args: 'huge' })
  expect(String(refused.text)).toMatch(/^Usage: \/curtain-mod-demo \[small\|medium\|full\] \[halt\]\. Not understood: huge/)
  const tooMany = await runCommand($,{ command: 'curtain-mod-demo', args: 'full medium halt' })
  expect(String(tooMany.text)).toMatch(/^Usage/)
})

test('the configured size is the demo default', { ...SLOW, options: { size: 'medium' } }, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($,{ command: 'curtain-mod-demo', args: '' })
  expect(String(said.text)).toMatch(/Curtain demo \(medium\)/)
  await clock.advance(1_000)
  expect((await band($)).length).toBe(6)
})

test('/curtain-mod alone plays the closing show, expands for its finale, and does not exit', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($,{ command: 'curtain-mod', args: '' })
  expect(String(said.text)).toMatch(/does not exit/)
  await clock.advance(1_000)
  expect((await band($)).length).toBe(2)
  await clock.advance(5_000)
  expect((await band($)).length).toBe(9)
  await clock.advance(5_000)
  expect(await bandText($)).toMatch(/fin|bow/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.runs).toEqual([])
})

test('a real /curtain takes over from a demo', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  await runCommand($,{ command: 'curtain-mod-demo', args: '' })
  await clock.advance(5_000)
  await $.skill.prompt({ skill: 'curtain:curtain', text: '' })
  await clock.advance(1_000)
  expect((await band($))[1]!.startsWith('1/3 merge-pr 0:01')).toBe(true)
  // And the demo cannot start over a real show.
  const refused = await runCommand($,{ command: 'curtain-mod-demo', args: '' })
  expect(String(refused.text)).toMatch(/real \/curtain show is in progress/)
})
