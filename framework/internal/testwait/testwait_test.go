package testwait

import (
	"testing"
	"time"
)

// The factor only ever stretches a deadline: anything that is not a number of at
// least 1 reads as 1, so a typo cannot shrink every wait in the suite.
func TestParse(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]float64{"": 1, "2": 2, "1.5": 1.5, "0.5": 1, "-3": 1, "slow": 1, "NaN": 1, "Inf": 1} {
		if got := parse(in); got != want {
			t.Errorf("parse(%q) = %g, want %g", in, got, want)
		}
	}
}

// For returns once the condition holds, and looks at least once more after the
// deadline before it gives up.
func TestForLooksAfterTheDeadline(t *testing.T) {
	t.Parallel()
	n := 0
	For(t, "the third look", time.Second, func() bool { n++; return n == 3 })
	start := time.Now()
	For(t, "a condition true only once the deadline has passed", 50*time.Millisecond, func() bool {
		return time.Since(start) > Scale(50*time.Millisecond)
	})
}
