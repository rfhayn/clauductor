// The curtain's travel, as pure functions: no `$`, so the tests import them
// directly and the hooks module calls them on every frame.
//
// The travel from open (0) to closed (1) is split into one stretch per step,
// each as wide as its share of the expected total. Inside a stretch the curtain
// eases toward the stretch's end, but stops short of it until the step has
// actually completed: a long step reads as "nearly there", never as "done".

export const STEPS = ['merge-pr', 'session-close', 'lane'] as const
export type StepName = (typeof STEPS)[number]

// First-run expectations (ms): merge-pr runs the full gate; session-close lands
// a docs PR; the lane step is one line until PANEL-34 adds `clauductor lane close`.
export const DEFAULT_EXPECTED_MS: readonly number[] = [8 * 60_000, 4 * 60_000, 20_000]

// The demo's pretend expectations, and when each pretend step "really" ends:
// the first runs over its estimate, so the demo shows the curtain waiting at a
// boundary, and the second runs under, so it shows the glide.
export const DEMO_EXPECTED_MS: readonly number[] = [20_000, 12_000, 4_000]
export const DEMO_ACTUAL_MS: readonly number[] = [26_000, 9_000, 4_000]

// How close a running stretch may come to its boundary, as a fraction of the
// stretch. Below 1, so the boundary is crossed only by a completion.
export const MAX_WITHIN = 0.97

// How many past runs the moving average keeps per step.
export const SAMPLES_KEPT = 5

// The cumulative boundary after each stretch, e.g. [0.62, 0.98, 1].
export function boundaries(expected: readonly number[]): number[] {
  const safe = expected.map((ms) => (Number.isFinite(ms) && ms > 0 ? ms : 1))
  const total = safe.reduce((a, b) => a + b, 0)
  let sum = 0
  const out = safe.map((ms) => {
    sum += ms
    return sum / total
  })
  // Float sums can land a hair under 1; the last boundary is the closed curtain.
  out[out.length - 1] = 1
  return out
}

// How far through its own stretch a running step is drawn, 0 to MAX_WITHIN.
// 1 - e^(-2t/expected): about 86% of the way at the expected time, then a slow
// creep, so an overrun still moves but never arrives.
export function stretchProgress(elapsedMs: number, expectedMs: number): number {
  if (!(elapsedMs > 0)) return 0
  const tau = Math.max(expectedMs, 1) / 2
  return Math.min(MAX_WITHIN, MAX_WITHIN * (1 - Math.exp(-elapsedMs / tau)))
}

// Where the curtain should be: `stage` is the index of the running step, or
// expected.length once every step has completed (the curtain closes fully).
export function target(expected: readonly number[], stage: number, elapsedInStageMs: number): number {
  if (stage >= expected.length) return 1
  const b = boundaries(expected)
  const start = stage === 0 ? 0 : b[stage - 1]!
  const width = b[stage]! - start
  return start + width * stretchProgress(elapsedInStageMs, expected[stage]!)
}

// One frame of easing from where the curtain is drawn toward where it should be.
// Never moves backward, and never passes the target, so the drawn curtain obeys
// the same boundary rule the target does.
export function approach(display: number, goal: number, dtMs: number, tauMs = 280): number {
  if (!(goal > display)) return display
  if (!(dtMs > 0)) return display
  const next = display + (goal - display) * (1 - Math.exp(-dtMs / tauMs))
  return goal - next < 0.0005 ? goal : next
}

// The moving average of the kept samples, or the fallback when there are none.
export function expectedFrom(samples: unknown, fallbackMs: number): number {
  const valid = Array.isArray(samples)
    ? samples.filter((v): v is number => typeof v === 'number' && Number.isFinite(v) && v > 0).slice(-SAMPLES_KEPT)
    : []
  if (valid.length === 0) return fallbackMs
  return Math.round(valid.reduce((a, b) => a + b, 0) / valid.length)
}

// The sample list after one more run, keeping only the newest SAMPLES_KEPT.
export function withSample(samples: unknown, ms: number): number[] {
  const valid = Array.isArray(samples)
    ? samples.filter((v): v is number => typeof v === 'number' && Number.isFinite(v) && v > 0)
    : []
  return [...valid, Math.round(ms)].slice(-SAMPLES_KEPT)
}

// "1:05" for 65 seconds; minutes past an hour keep counting ("75:00").
export function clock(ms: number): string {
  const total = Math.max(0, Math.round(ms / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return m + ':' + String(s).padStart(2, '0')
}
