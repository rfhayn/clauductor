// Package testwait is test support: the deadlines the panel's tests wait on, scaled for
// a loaded machine. Only tests import it.
//
// A test waits on the condition it needs, never on a sleep, and the deadline is only
// how long it may take before the test gives up. On a machine running several gates at
// once (or a slow CI runner) a deadline that is ample on an idle laptop expires while the
// condition is still on its way: the test fails, and nothing was wrong. So every deadline
// goes through Scale, which multiplies it by CLAUDUCTOR_TEST_SLOW (a factor, default 1;
// CI sets it). A passing test never waits for a deadline, so a larger one costs nothing.
package testwait

import (
	"math"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Env names the variable that scales every test deadline: CLAUDUCTOR_TEST_SLOW=2 doubles
// them. Unset, unparsable, infinite or below 1, the factor is 1.
const Env = "CLAUDUCTOR_TEST_SLOW"

var factor = sync.OnceValue(func() float64 { return parse(os.Getenv(Env)) })

func parse(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || !(f >= 1) || math.IsInf(f, 1) { // NaN fails f >= 1
		return 1
	}
	return f
}

// Factor is the deadline multiplier read from Env.
func Factor() float64 { return factor() }

// Scale is d stretched by the factor: use it for every deadline a test waits on, never
// for a duration whose length the test asserts.
func Scale(d time.Duration) time.Duration { return time.Duration(float64(d) * Factor()) }

// For polls cond every 20 ms until it holds, and fails the test once Scale(d) has passed
// without it. The look that fails it comes after the deadline, so a condition that came
// true just as the deadline passed still counts.
func For(t testing.TB, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(Scale(d))
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s (%s=%g)", Scale(d), what, Env, Factor())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
