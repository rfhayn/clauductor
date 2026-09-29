package install

import (
	"context"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
)

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

// driven runs fn against a fake clock, advancing it by step each time fn waits on
// it, until fn returns. It returns fn's error and how far the clock moved: an
// install's launchctl polls are counted, not slept.
func driven(f *clock.Fake, step time.Duration, fn func() error) (error, time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { defer cancel(); done <- fn() }()
	start := f.Now()
	for f.BlockUntilContext(ctx, 1) == nil {
		f.Advance(step)
	}
	return <-done, f.Now().Sub(start)
}
