// The demo and the hand-typed closing show: both play to the end with
// nothing run, nothing submitted, nothing recorded, and no /exit. The demo
// plays the whole production at a pace: the crew strike the set, the curtain
// falls, one Clawd bows.

import { expect, test } from 'claude-code/testing'
import { MERGED, band, bandText, crewOnStage, endTurn, runCommand, stage, startSession } from './stage.js'

const SLOW = { timeoutMs: 30_000 }
const status = (lines: string[]) => (lines[lines.length - 1] ?? '').trimEnd()

test('/curtain-mod-demo: the crew thins out, the curtain falls, one Clawd bows, no exit, under a minute', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($, { command: 'curtain-mod-demo', args: '' })
  expect(String(said.text)).toMatch(/Curtain demo \(small\).*Nothing is run/)
  await clock.advance(1_000)
  let lines = await band($)
  expect(lines.length).toBe(10)
  expect(status(lines).startsWith('demo 1/3 merge-pr 0:01 / ~20s')).toBe(true)
  const counts = [await crewOnStage($)]
  expect(counts[0]).toBe(3)

  // The first pretend step overruns its 20 s estimate: the scene waits short of the boundary.
  await clock.advance(24_000)
  lines = await band($)
  expect(status(lines)).toMatch(/^demo 1\/3 merge-pr 0:25/)
  counts.push(await crewOnStage($))
  await clock.advance(2_500)
  expect(status(await band($))).toMatch(/^demo 2\/3 session-close/)
  await clock.advance(9_000)
  counts.push(await crewOnStage($))
  // Fewer hands as the work nears its end, never more.
  expect(counts[1]! <= counts[0]! && counts[2]! <= counts[1]! && counts[2]! < counts[0]!).toBe(true)

  // The last step ends at 39 s: the curtain falls, then the bow and the fin.
  await clock.advance(4_000)
  expect(status(await band($))).toBe('demo the curtain falls')
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

test('/curtain-mod-demo halt freezes the scene at INTERMISSION, nobody bows, and nothing exits', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  await runCommand($, { command: 'curtain-mod-demo', args: 'halt' })
  await clock.advance(10_000)
  const held = await band($)
  expect(held.length).toBe(10)
  expect(status(held).startsWith('INTERMISSION - merge-pr stopped: review did not converge')).toBe(true)
  expect(held.join('\n')).toMatch(/INTERMISSION/)
  await clock.advance(60_000)
  expect(await band($)).toEqual(held)
  expect(held.join('\n')).not.toMatch(/Clawd takes a bow/)
  expect(rec.commands).toEqual([])
  // /curtain-mod cancel on a held show clears it.
  await runCommand($, { command: 'curtain-mod', args: 'cancel' })
  expect(await band($)).toEqual([])
})

test('the demo takes a size and halt, in either order, and refuses anything else', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  await runCommand($, { command: 'curtain-mod-demo', args: 'full' })
  await clock.advance(1_000)
  expect((await band($)).length).toBe(16)

  await runCommand($, { command: 'curtain-mod-demo', args: 'halt medium' })
  await clock.advance(10_000)
  const held = await band($)
  expect(held.length).toBe(12)
  expect(held.join('\n')).toMatch(/INTERMISSION/)

  await runCommand($, { command: 'curtain-mod-demo', args: 'small halt' })
  await clock.advance(1_000)
  expect((await band($)).length).toBe(10)

  const off = await runCommand($, { command: 'curtain-mod-demo', args: 'off' })
  expect(String(off.text)).toMatch(/The curtain show is off/)

  const refused = await runCommand($, { command: 'curtain-mod-demo', args: 'huge' })
  expect(String(refused.text)).toMatch(/^Usage: \/curtain-mod-demo \[off\|small\|medium\|full\] \[halt\]\. Not understood: huge/)
  const tooMany = await runCommand($, { command: 'curtain-mod-demo', args: 'full medium halt' })
  expect(String(tooMany.text)).toMatch(/^Usage/)
})

test('the configured size is the demo default', { ...SLOW, options: { size: 'medium' } }, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($, { command: 'curtain-mod-demo', args: '' })
  expect(String(said.text)).toMatch(/Curtain demo \(medium\)/)
  await clock.advance(1_000)
  expect((await band($)).length).toBe(12)
})

test('with the size off, the demo and the closing show say so and draw nothing', { ...SLOW, options: { size: 'off' } }, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const demo = await runCommand($, { command: 'curtain-mod-demo', args: '' })
  expect(String(demo.text)).toMatch(/The curtain show is off/)
  const closing = await runCommand($, { command: 'curtain-mod', args: '' })
  expect(String(closing.text)).toMatch(/The curtain show is off/)
  await clock.advance(10_000)
  expect(await band($)).toEqual([])
  // An explicit size still plays the demo.
  await runCommand($, { command: 'curtain-mod-demo', args: 'small' })
  await clock.advance(1_000)
  expect((await band($)).length).toBe(10)
  expect(rec.commands).toEqual([])
})

test('/curtain-mod alone plays the closing show, ends with a bow, and does not exit', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await runCommand($, { command: 'curtain-mod', args: '' })
  expect(String(said.text)).toMatch(/does not exit/)
  await clock.advance(1_000)
  expect((await band($)).length).toBe(10)
  await clock.advance(7_500)
  expect(await bandText($)).toMatch(/Clawd takes a bow/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.runs).toEqual([])
})

test('a real /curtain takes over from a demo', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  await runCommand($, { command: 'curtain-mod-demo', args: '' })
  await clock.advance(5_000)
  await $.skill.prompt({ skill: 'curtain:curtain', text: '' })
  await clock.advance(1_000)
  expect(status(await band($)).startsWith('1/3 merge-pr 0:01')).toBe(true)
  // And the demo cannot start over a real show.
  const refused = await runCommand($, { command: 'curtain-mod-demo', args: '' })
  expect(String(refused.text)).toMatch(/real \/curtain show is in progress/)
})
