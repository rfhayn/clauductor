// The real show's chaining, with every gh call stubbed: the skill's events
// move the curtain, the checks gate each boundary, and /exit goes out only on
// the /curtain done signal with the PR confirmed merged.

import { expect, test } from 'claude-code/testing'
import { MERGED, TOOL, bandText, endTurn, stage, startSession } from './stage.ts'

const SLOW = { timeoutMs: 30_000 }

async function raise($: any) {
  await startSession($)
  await $.skill.prompt({ skill: 'curtain', text: 'the curtain skill' })
}

test('all three steps succeed: the curtain falls, Clawd bows, and the mod submits /exit', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await raise($)
  expect(await bandText($)).toMatch(/▐▛███▜▌/)

  // merge-pr runs for five minutes; session-close starting is its boundary.
  await clock.advance(5 * 60_000)
  await $.skill.prompt({ skill: 'session-close', text: 'the session-close skill' })
  await clock.advance(3 * 60_000)

  const lane = await $.tool.call({ tool: TOOL, cue: 'lane' })
  expect(String(lane.result)).toMatch(/confirmed/)
  await clock.advance(15_000)

  const exit = await $.tool.call({ tool: TOOL, cue: 'exit' })
  expect(String(exit.result)).toMatch(/End your turn now/)
  // The signal alone sends nothing: the turn has not ended.
  await clock.advance(20_000)
  expect(rec.commands).toEqual([])

  await endTurn($)
  await clock.advance(1_000)
  expect(rec.commands).toEqual(['exit'])
  // Exactly once, however long the fin stays up.
  await clock.advance(20_000)
  expect(rec.commands).toEqual(['exit'])
  // Nothing was submitted as a prompt along the way.
  expect(rec.submits).toEqual([])
})

test('each completed step records its real duration for next time', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED, { 'samples:merge-pr': [400_000] })
  await raise($)
  await clock.advance(5 * 60_000)
  await $.skill.prompt({ skill: 'session-close', text: '' })
  await clock.advance(3 * 60_000)
  await $.tool.call({ tool: TOOL, cue: 'lane' })
  await clock.advance(10_000)
  await $.tool.call({ tool: TOOL, cue: 'exit' })
  const mergeRuns = rec.store.get('samples:merge-pr') as number[]
  expect(mergeRuns.length).toBe(2)
  expect(mergeRuns[0]).toBe(400_000)
  // Five minutes, give or take the frame the check settled in.
  expect(Math.abs(mergeRuns[1]! - 300_000) < 1_000).toBe(true)
  expect(Math.abs((rec.store.get('samples:session-close') as number[])[0]! - 180_000) < 1_000).toBe(true)
  expect(Math.abs((rec.store.get('samples:lane') as number[])[0]! - 10_000) < 1_000).toBe(true)
})

test('the curtain reads the stored timings: the band shows the learned estimate', SLOW, async ($, on) => {
  const { clock } = stage(on, MERGED, { 'samples:merge-pr': [120_000, 180_000] })
  await raise($)
  await clock.advance(1_000)
  expect(await bandText($)).toMatch(/of ~2:30/)
})

test('a PR that is not merged holds the show at INTERMISSION, and nothing exits', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, { ...MERGED, prState: 'OPEN' })
  await raise($)
  await clock.advance(60_000)
  const exit = await $.tool.call({ tool: TOOL, cue: 'exit' })
  expect(String(exit.result)).toMatch(/^Curtain: INTERMISSION\. merge-pr stopped: PR #42 is OPEN/)
  await endTurn($)
  await clock.advance(60_000)
  expect(rec.commands).toEqual([])
  expect(rec.status.some((s) => /^INTERMISSION: merge-pr stopped/.test(s ?? ''))).toBe(true)
  expect(await bandText($)).toMatch(/INTERMISSION/)
})

test('no session-close PR merged: the lane cue stops the show at session-close', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, { ...MERGED, closeMergedAt: null })
  await raise($)
  await clock.advance(60_000)
  await $.skill.prompt({ skill: 'session-close', text: '' })
  await clock.advance(60_000)
  const lane = await $.tool.call({ tool: TOOL, cue: 'lane' })
  expect(String(lane.result)).toMatch(/INTERMISSION\. session-close stopped/)
  const exit = await $.tool.call({ tool: TOOL, cue: 'exit' })
  expect(String(exit.result)).toMatch(/INTERMISSION/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
})

test('a turn that ends without the done signal halts at the running step', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await raise($)
  await clock.advance(90_000)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
  expect(rec.logs.some((l) => /INTERMISSION\. merge-pr stopped: the turn ended before \/curtain gave its done signal/.test(l))).toBe(true)
})

test('an interrupted turn halts too', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await raise($)
  await clock.advance(10_000)
  await endTurn($, true)
  expect(rec.logs.some((l) => /the turn was interrupted/.test(l))).toBe(true)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
})

test('/curtain-mod cancel freezes the curtain where it is, and a later done signal is refused', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await raise($)
  await clock.advance(4 * 60_000)
  const held = await $.command.run({ command: 'curtain-mod', args: 'cancel' })
  expect(String(held.text)).toMatch(/INTERMISSION/)
  const frozen = await bandText($)
  await clock.advance(60_000)
  // Frozen: the same frame a minute later.
  expect(await bandText($)).toBe(frozen)
  const exit = await $.tool.call({ tool: TOOL, cue: 'exit' })
  expect(String(exit.result)).toMatch(/INTERMISSION\. merge-pr stopped: cancelled/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
})

test('no PR when the curtain rises: the first check stops it', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, { ...MERGED, hasPr: false })
  await raise($)
  await clock.advance(1_000)
  await $.skill.prompt({ skill: 'session-close', text: '' })
  await clock.advance(1_000)
  expect(rec.logs.some((l) => /merge-pr stopped: no PR was found/.test(l))).toBe(true)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
})

test('the done signal with no show running says so, and exits nothing', SLOW, async ($, on) => {
  const { clock, rec } = stage(on, MERGED)
  await startSession($)
  const answer = await $.tool.call({ tool: TOOL, cue: 'exit' })
  expect(String(answer.result)).toMatch(/no \/curtain show is running/)
  await endTurn($)
  await clock.advance(30_000)
  expect(rec.commands).toEqual([])
})

test('the band draws text where there is no Raster', SLOW, async ($, on) => {
  const { clock } = stage(on, { ...MERGED, prState: 'OPEN' })
  await raise($)
  await clock.advance(1_000)
  const desktop = await $.ui.mount({ plugin: 'curtain', surface: 'desktop', component: 'AbovePrompt', props: { hasSurvey: false, isWorking: true, maxRows: 20, bodyColumns: 80, scroll: { offset: 0, bodyRows: 20 }, view: {} } })
  expect(await desktop.find({ type: 'Text', text: /merge-pr/ })).toBeDefined()
  await desktop.unmount()
  await $.tool.call({ tool: TOOL, cue: 'exit' })
  const again = await $.ui.mount({ plugin: 'curtain', surface: 'desktop', component: 'AbovePrompt', props: { hasSurvey: false, isWorking: false, maxRows: 20, bodyColumns: 80, scroll: { offset: 0, bodyRows: 20 }, view: {} } })
  expect(await again.find({ type: 'Text', text: /INTERMISSION/ })).toBeDefined()
})
