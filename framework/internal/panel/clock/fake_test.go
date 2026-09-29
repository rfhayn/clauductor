package clock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

var _ Clock = (*Fake)(nil)

var t0 = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)

// fired reports whether c has a value ready, without waiting.
func fired(c <-chan time.Time) (time.Time, bool) {
	select {
	case v := <-c:
		return v, true
	default:
		return time.Time{}, false
	}
}

func TestFakeNowMovesOnlyOnAdvance(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	if !f.Now().Equal(t0) {
		t.Fatalf("now %v", f.Now())
	}
	f.Advance(90 * time.Second)
	if want := t0.Add(90 * time.Second); !f.Now().Equal(want) {
		t.Fatalf("now %v, want %v", f.Now(), want)
	}
}

func TestFakeTimerFiresOnceItsDeadlinePasses(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	tm := f.NewTimer(10 * time.Second)
	f.Advance(9 * time.Second)
	if _, ok := fired(tm.C()); ok {
		t.Fatal("fired before its deadline")
	}
	f.Advance(time.Second)
	v, ok := fired(tm.C())
	if !ok || !v.Equal(t0.Add(10*time.Second)) {
		t.Fatalf("fired %v %v, want once at t0+10s", ok, v)
	}
	f.Advance(time.Hour)
	if _, ok := fired(tm.C()); ok {
		t.Fatal("a timer fired twice")
	}
	if tm.Stop() {
		t.Fatal("Stop of a fired timer reports it stopped it")
	}
	if f.Pending() != 0 {
		t.Fatalf("%d pending after the timer fired", f.Pending())
	}
}

func TestFakeTimerStopAndReset(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	tm := f.NewTimer(time.Second)
	if !tm.Stop() {
		t.Fatal("Stop of a pending timer reports it was not pending")
	}
	f.Advance(time.Minute)
	if _, ok := fired(tm.C()); ok {
		t.Fatal("a stopped timer fired")
	}
	if tm.Reset(5 * time.Second) {
		t.Fatal("Reset of a stopped timer reports it was pending")
	}
	f.Advance(4 * time.Second)
	if _, ok := fired(tm.C()); ok {
		t.Fatal("a reset timer fired early")
	}
	f.Advance(time.Second)
	if v, ok := fired(tm.C()); !ok || !v.Equal(t0.Add(time.Minute+5*time.Second)) {
		t.Fatalf("reset timer: %v %v", ok, v)
	}
	// A zero wait fires at once, as with the time package.
	if _, ok := fired(f.NewTimer(0).C()); !ok {
		t.Fatal("a zero timer did not fire at once")
	}
	if _, ok := fired(f.After(-time.Second)); !ok {
		t.Fatal("a negative After did not fire at once")
	}
}

func TestFakeTickerTicksEveryPeriodAndDropsWhatIsNotRead(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	tk := f.NewTicker(2 * time.Second)
	for i := 1; i <= 3; i++ {
		f.Advance(2 * time.Second)
		if v, ok := fired(tk.C()); !ok || !v.Equal(t0.Add(time.Duration(2*i)*time.Second)) {
			t.Fatalf("tick %d: %v %v", i, ok, v)
		}
	}
	// Ten periods with nobody reading: one tick is buffered, the rest dropped.
	f.Advance(20 * time.Second)
	if _, ok := fired(tk.C()); !ok {
		t.Fatal("no tick buffered")
	}
	if _, ok := fired(tk.C()); ok {
		t.Fatal("more than one tick buffered")
	}
	tk.Stop()
	f.Advance(time.Minute)
	if _, ok := fired(tk.C()); ok {
		t.Fatal("a stopped ticker ticked")
	}
	if f.Pending() != 0 {
		t.Fatalf("%d pending after Stop", f.Pending())
	}
}

func TestFakeAdvanceFiresInDeadlineOrder(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	late, early := f.NewTimer(3*time.Second), f.NewTimer(time.Second)
	tk := f.NewTicker(2 * time.Second)
	f.Advance(3 * time.Second)
	e, _ := fired(early.C())
	k, _ := fired(tk.C())
	l, _ := fired(late.C())
	if !e.Equal(t0.Add(time.Second)) || !k.Equal(t0.Add(2*time.Second)) || !l.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("fired at %v, %v, %v; want t0+1s, +2s, +3s", e, k, l)
	}
}

func TestFakeSleepAndAfterWaitForAdvance(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	woke := make(chan time.Time, 1)
	go func() {
		f.Sleep(5 * time.Second)
		woke <- f.Now()
	}()
	f.BlockUntil(1) // the goroutine is asleep
	f.Advance(4 * time.Second)
	select {
	case <-woke:
		t.Fatal("Sleep returned before the clock passed its deadline")
	default:
	}
	f.Advance(time.Second)
	if got := <-woke; !got.Equal(t0.Add(5 * time.Second)) {
		t.Fatalf("woke at %v", got)
	}
}

// The pattern the panel's tests use: a loop that waits a new timer each iteration is
// driven one iteration per Advance, and BlockUntil says when an iteration is done.
func TestFakeDrivesALoopOneIterationAtATime(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var polls atomic.Int32
	go func() {
		for {
			polls.Add(1)
			tm := f.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				tm.Stop()
				return
			case <-tm.C():
			}
		}
	}()
	for i := int32(1); i <= 5; i++ {
		f.BlockUntil(1)
		if got := polls.Load(); got != i {
			t.Fatalf("after %d advances, %d polls", i-1, got)
		}
		f.Advance(time.Second)
	}
	f.BlockUntil(1)
	if got := polls.Load(); got != 6 {
		t.Fatalf("%d polls, want 6", got)
	}
}

func TestFakeBlockUntilContextGivesUp(t *testing.T) {
	t.Parallel()
	f := NewFake(t0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.BlockUntilContext(ctx, 1); err == nil {
		t.Fatal("BlockUntilContext returned without a pending wait or an error")
	}
	f.NewTimer(time.Second)
	if err := f.BlockUntilContext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}
