// Package clock is the panel's one source of time. Every package reads the time,
// sleeps and waits through a Clock, so a test can pin `now` and the hub, the
// notifier, the lanes, the runtime and the queue view agree on it. This is the only
// package of the panel that calls the time package's clock functions
// (clock_check_test.go in the panel enforces it).
package clock

import "time"

// Clock tells the time and makes timers.
type Clock interface {
	Now() time.Time
	// After is time.After on this clock.
	After(d time.Duration) <-chan time.Time
	// Sleep is time.Sleep on this clock.
	Sleep(d time.Duration)
	NewTimer(d time.Duration) Timer
	NewTicker(d time.Duration) Ticker
}

// Timer is a *time.Timer, as a Clock makes it.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// Ticker is a *time.Ticker, as a Clock makes it.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// System is the real clock.
var System Clock = system{}

type system struct{}

func (system) Now() time.Time                         { return time.Now() }
func (system) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (system) Sleep(d time.Duration)                  { time.Sleep(d) }
func (system) NewTimer(d time.Duration) Timer         { return timer{time.NewTimer(d)} }
func (system) NewTicker(d time.Duration) Ticker       { return ticker{time.NewTicker(d)} }

type timer struct{ t *time.Timer }

func (t timer) C() <-chan time.Time        { return t.t.C }
func (t timer) Stop() bool                 { return t.t.Stop() }
func (t timer) Reset(d time.Duration) bool { return t.t.Reset(d) }

type ticker struct{ t *time.Ticker }

func (t ticker) C() <-chan time.Time { return t.t.C }
func (t ticker) Stop()               { t.t.Stop() }

// Func is a Clock whose Now is the function; its timers run on the real clock.
// Tests use it to pin `now` while waits still pass.
type Func func() time.Time

// Now calls f.
func (f Func) Now() time.Time { return f() }

// After waits on the real clock.
func (Func) After(d time.Duration) <-chan time.Time { return System.After(d) }

// Sleep sleeps on the real clock.
func (Func) Sleep(d time.Duration) { System.Sleep(d) }

// NewTimer makes a real timer.
func (Func) NewTimer(d time.Duration) Timer { return System.NewTimer(d) }

// NewTicker makes a real ticker.
func (Func) NewTicker(d time.Duration) Ticker { return System.NewTicker(d) }

// Or returns c, or System when c is nil.
func Or(c Clock) Clock {
	if c == nil {
		return System
	}
	return c
}
