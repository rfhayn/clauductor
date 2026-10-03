// The demo and the hand-typed closing show: both play to the end with
// nothing run, nothing submitted, nothing recorded, and no /exit.

import { expect, test } from 'claude-code/testing'
import { MERGED, bandText, endTurn, stage, startSession } from './stage.ts'

const SLOW = { timeoutMs: 30_000 }

test('/curtain-mod-demo plays the whole show and exits nothing', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await $.command.run({ command: 'curtain-mod-demo', args: '' })
  expect(String(said.text)).toMatch(/Nothing is run/)
  await clock.advance(1_000)
  expect(await bandText($)).toMatch(/DEMO · ▸ merge-pr/)

  // The first pretend step overruns its 20 s estimate: the curtain waits short of the boundary.
  await clock.advance(24_000)
  expect(await bandText($)).toMatch(/▸ merge-pr/)
  await clock.advance(2_500)
  expect(await bandText($)).toMatch(/✓ merge-pr {2}▸ session-close/)

  await clock.advance(13_000)
  // Closed, bowing, then the fin.
  await clock.advance(3_000)
  expect(await bandText($)).toMatch(/~ fin ~|Clawd takes a bow/)
  await clock.advance(3_000)
  expect(await bandText($)).toMatch(/Thank you, goodnight/)

  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.submits).toEqual([])
  expect(rec.runs).toEqual([])
  // A demo teaches the timings nothing.
  expect([...rec.store.keys()]).toEqual([])
  // And it clears itself away.
  expect(await bandText($)).toBe('')
})

test('/curtain-mod-demo halt freezes at INTERMISSION and exits nothing', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  await $.command.run({ command: 'curtain-mod-demo', args: 'halt' })
  await clock.advance(10_000)
  const held = await bandText($)
  expect(held).toMatch(/INTERMISSION/)
  expect(held).toMatch(/merge-pr stopped: review did not converge/)
  await clock.advance(60_000)
  expect(await bandText($)).toBe(held)
  expect(rec.commands).toEqual([])
  // /curtain-mod cancel on a held show clears it.
  await $.command.run({ command: 'curtain-mod', args: 'cancel' })
  expect(await bandText($)).toBe('')
})

test('/curtain-mod alone plays the closing show and does not exit', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const said = await $.command.run({ command: 'curtain-mod', args: '' })
  expect(String(said.text)).toMatch(/does not exit/)
  await clock.advance(1_000)
  expect(await bandText($)).toMatch(/CURTAIN CALL/)
  await clock.advance(8_000)
  expect(await bandText($)).toMatch(/fin|bow/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.runs).toEqual([])
})

test('a real /curtain takes over from a demo', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED)
  await startSession($)
  await $.command.run({ command: 'curtain-mod-demo', args: '' })
  await clock.advance(5_000)
  await $.skill.prompt({ skill: 'curtain:curtain', text: '' })
  await clock.advance(1_000)
  expect(await bandText($)).toMatch(/^[\s\S]*CURTAIN · ▸ merge-pr/)
  // And the demo cannot start over a real show.
  const refused = await $.command.run({ command: 'curtain-mod-demo', args: '' })
  expect(String(refused.text)).toMatch(/real \/curtain show is in progress/)
})
