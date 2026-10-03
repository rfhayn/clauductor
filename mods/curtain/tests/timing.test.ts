// The stretch and boundary math: each step's stretch is weighted by its
// expected duration, and the curtain never crosses a step's boundary until
// that step has completed, however long it overruns.

import { expect, test } from 'claude-code/testing'
import {
  DEFAULT_EXPECTED_MS,
  MAX_WITHIN,
  SAMPLES_KEPT,
  approach,
  boundaries,
  expectedFrom,
  stretchProgress,
  target,
  withSample,
} from '../hooks/timing.js'

test('each stretch is weighted by its expected duration', async () => {
  const b = boundaries([8 * 60_000, 4 * 60_000, 20_000])
  // 480 s, 240 s and 20 s of 740 s in all.
  expect(Math.abs(b[0]! - 480 / 740) < 1e-9).toBe(true)
  expect(Math.abs(b[1]! - 720 / 740) < 1e-9).toBe(true)
  expect(b[2]).toBe(1)
  expect(boundaries([1, 1])).toEqual([0.5, 1])
})

test('a running step never reaches its boundary, however long it overruns', async () => {
  const expected = DEFAULT_EXPECTED_MS
  const b = boundaries(expected)
  for (let stage = 0; stage < expected.length; stage++) {
    const start = stage === 0 ? 0 : b[stage - 1]!
    let previous = -1
    for (const factor of [0, 0.01, 0.5, 1, 2, 3, 10, 100, 1e6]) {
      const p = target(expected, stage, expected[stage]! * factor)
      expect(p >= start, 'stage ' + stage + ' at ' + factor + 'x starts at its stretch').toBe(true)
      expect(p < b[stage]!, 'stage ' + stage + ' at ' + factor + 'x stays short of its boundary').toBe(true)
      expect(p >= previous, 'stage ' + stage + ' never moves back').toBe(true)
      previous = p
    }
  }
})

test('the ease is paced by the expected duration', async () => {
  // Most of the way at the expected time, nowhere near it early on.
  expect(stretchProgress(60_000, 60_000) > 0.8).toBe(true)
  expect(stretchProgress(6_000, 60_000) < 0.2).toBe(true)
  expect(stretchProgress(1e12, 60_000)).toBe(MAX_WITHIN)
  expect(stretchProgress(0, 60_000)).toBe(0)
})

test('a completed step puts the next one at exactly its boundary, and the last closes the curtain', async () => {
  const expected = [20_000, 12_000, 4_000]
  const b = boundaries(expected)
  expect(target(expected, 1, 0)).toBe(b[0])
  expect(target(expected, 2, 0)).toBe(b[1])
  expect(target(expected, 3, 0)).toBe(1)
})

test('the drawn curtain glides toward its target without passing it or moving back', async () => {
  let display = 0
  const goal = 0.6
  let previous = 0
  for (let frame = 0; frame < 200; frame++) {
    display = approach(display, goal, 150)
    expect(display <= goal).toBe(true)
    expect(display >= previous).toBe(true)
    previous = display
  }
  expect(display).toBe(goal)
  // A lower target (never produced, but guarded) does not pull it back.
  expect(approach(0.7, 0.5, 150)).toBe(0.7)
})

test('expected durations are the moving average of the last runs', async () => {
  expect(expectedFrom(undefined, 480_000)).toBe(480_000)
  expect(expectedFrom([], 480_000)).toBe(480_000)
  expect(expectedFrom([100_000, 300_000], 480_000)).toBe(200_000)
  expect(expectedFrom(['x', -5, 300_000], 1)).toBe(300_000)
  let samples: number[] = []
  for (let i = 1; i <= SAMPLES_KEPT + 3; i++) samples = withSample(samples, i * 1000)
  expect(samples.length).toBe(SAMPLES_KEPT)
  expect(samples[samples.length - 1]).toBe((SAMPLES_KEPT + 3) * 1000)
  // The oldest runs fall out of the average.
  expect(expectedFrom(samples, 1)).toBe(((4 + 5 + 6 + 7 + 8) * 1000) / 5)
})
