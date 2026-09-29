package clock

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Fake is a Clock that moves only when a test calls Advance. Its timers, tickers,
// After and Sleep wait for Advance to carry the clock past their deadline, so a test
// drives a loop one iteration at a time instead of sleeping and hoping it ran.
//
// BlockUntil(n) waits until n waits are pending on the clock (an unfired Timer or
// After, a running Ticker, a Sleep). A loop that makes a new timer every iteration
// is idle, having finished its iteration, exactly when its timer is pending again:
//
//	f := clock.NewFake(t0)
//	go loop(ctx, f)   // polls, then waits f.NewTimer(interval)
//	f.BlockUntil(1)   // the first poll is done
//	f.Advance(interval)
//	f.BlockUntil(1)   // the second poll is done
//
// A Ticker stays pending while it runs, so BlockUntil cannot tell when a ticker's
// reader has finished an iteration; such a loop reports its progress another way.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	waits   []*fakeWait
	changed chan struct{} // closed and replaced whenever waits changes
}

// NewFake returns a Fake clock reading now.
func NewFake(now time.Time) *Fake {
	return &Fake{now: now, changed: make(chan struct{})}
}

type fakeWait struct {
	at     time.Time
	period time.Duration // > 0: a ticker
	ch     chan time.Time
}

// Now is the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the clock forward by d, firing every timer and ticker whose deadline
// it passes, in deadline order, each with the time it was due. A ticker that falls
// behind drops ticks, as a time.Ticker does.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	end := f.now.Add(d)
	for {
		sort.SliceStable(f.waits, func(i, j int) bool { return f.waits[i].at.Before(f.waits[j].at) })
		if len(f.waits) == 0 || f.waits[0].at.After(end) {
			break
		}
		w := f.waits[0]
		f.now = w.at
		select {
		case w.ch <- w.at:
		default: // a full channel drops the tick, as the time package does
		}
		if w.period > 0 {
			w.at = w.at.Add(w.period)
		} else {
			f.removeLocked(w)
		}
	}
	f.now = end
}

// BlockUntil waits until at least n waits are pending on the clock.
func (f *Fake) BlockUntil(n int) { _ = f.BlockUntilContext(context.Background(), n) }

// BlockUntilContext is BlockUntil that gives up, with ctx's error, when ctx ends.
func (f *Fake) BlockUntilContext(ctx context.Context, n int) error {
	for {
		f.mu.Lock()
		pending, changed := len(f.waits), f.changed
		f.mu.Unlock()
		if pending >= n {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Pending is the number of waits pending on the clock.
func (f *Fake) Pending() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.waits)
}

func (f *Fake) addLocked(w *fakeWait) {
	f.waits = append(f.waits, w)
	f.notifyLocked()
}

func (f *Fake) removeLocked(w *fakeWait) bool {
	for i, x := range f.waits {
		if x == w {
			f.waits = append(f.waits[:i], f.waits[i+1:]...)
			f.notifyLocked()
			return true
		}
	}
	return false
}

func (f *Fake) notifyLocked() {
	close(f.changed)
	f.changed = make(chan struct{})
}

// newWait registers a wait that fires after d (at once, on the next Advance, when d
// is not positive: a zero time.Timer fires at once too).
func (f *Fake) newWait(d, period time.Duration) *fakeWait {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := &fakeWait{at: f.now.Add(d), period: period, ch: make(chan time.Time, 1)}
	if d <= 0 && period <= 0 {
		w.ch <- f.now
		return w
	}
	f.addLocked(w)
	return w
}

// After is time.After on the fake clock.
func (f *Fake) After(d time.Duration) <-chan time.Time { return f.newWait(d, 0).ch }

// Sleep blocks until Advance carries the clock d past now.
func (f *Fake) Sleep(d time.Duration) { <-f.After(d) }

// NewTimer makes a timer on the fake clock.
func (f *Fake) NewTimer(d time.Duration) Timer {
	return &fakeTimer{f: f, w: f.newWait(d, 0)}
}

// NewTicker makes a ticker on the fake clock. d must be positive, as for
// time.NewTicker.
func (f *Fake) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		panic("clock: non-positive interval for NewTicker")
	}
	return &fakeTicker{f: f, w: f.newWait(d, d)}
}

type fakeTimer struct {
	f *Fake
	w *fakeWait
}

func (t *fakeTimer) C() <-chan time.Time { return t.w.ch }

// Stop reports whether it stopped the timer before it fired.
func (t *fakeTimer) Stop() bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	return t.f.removeLocked(t.w)
}

// Reset makes the timer fire d after now; it reports whether it was pending.
func (t *fakeTimer) Reset(d time.Duration) bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	was := t.f.removeLocked(t.w)
	t.w.at = t.f.now.Add(d)
	if d <= 0 {
		select {
		case t.w.ch <- t.f.now:
		default:
		}
		return was
	}
	t.f.addLocked(t.w)
	return was
}

type fakeTicker struct {
	f *Fake
	w *fakeWait
}

func (t *fakeTicker) C() <-chan time.Time { return t.w.ch }

func (t *fakeTicker) Stop() {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	t.f.removeLocked(t.w)
}
