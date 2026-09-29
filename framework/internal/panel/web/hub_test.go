package web

import (
	"context"
	"testing"
	"time"

	pclock "github.com/clauductor/clauductor/internal/panel/clock"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// hubRounds waits until the hub has decided whether to push n times in all.
func hubRounds(t *testing.T, h *Hub, n int64) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); h.rounds.Load() < n; {
		if time.Now().After(deadline) {
			t.Fatalf("the hub made %d push decisions, want %d", h.rounds.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// pushed reports whether a snapshot is waiting on ch, without waiting: the hub has
// already made its decision (hubRounds).
func pushed(ch chan []byte) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// startHub runs h on a fake clock and returns once its ticker is set.
func startHub(t *testing.T, h *Hub, f *pclock.Fake) {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go h.Run(ctx)
	f.BlockUntil(1)
}

func TestHubPushesOnlyAChangedView(t *testing.T) {
	t.Parallel()
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	f := pclock.NewFake(t0)
	h := NewHub(m, f)
	h.TickEvery, h.Coalesce = 5*time.Second, 150*time.Millisecond
	ch, cancel := h.subscribe()
	defer cancel()
	startHub(t, h, f)
	rounds := int64(0)
	tick := func() { f.Advance(h.TickEvery); rounds++; hubRounds(t, h, rounds) }
	// An update waits out the coalescing sleep (a second wait on the clock) first.
	update := func(fn func(m *state.Model, now time.Time)) {
		h.Update(fn)
		f.BlockUntil(2)
		f.Advance(h.Coalesce)
		rounds++
		hubRounds(t, h, rounds)
	}
	tick()
	if !pushed(ch) {
		t.Fatal("the first view was never pushed")
	}
	// 30 ticks, each at a later `now`, and nothing else changes: nothing is pushed.
	for i := 0; i < 30; i++ {
		tick()
		if pushed(ch) {
			t.Fatalf("tick %d: an unchanged view was pushed again (%d pushes)", i+1, h.Pushes())
		}
	}
	// An update that changes nothing the page shows is not pushed either.
	update(func(m *state.Model, now time.Time) {})
	if pushed(ch) {
		t.Fatal("a no-op update was pushed")
	}
	// A real change is pushed, once.
	update(func(m *state.Model, now time.Time) {
		m.ApplyPRs([]signals.PR{{Number: 7, Title: "Synthetic"}}, nil, now)
	})
	if !pushed(ch) {
		t.Fatal("a changed view was not pushed")
	}
	for i := 0; i < 3; i++ {
		tick()
		if pushed(ch) {
			t.Fatal("a changed view was pushed more than once")
		}
	}
	if n := h.Pushes(); n != 2 {
		t.Fatalf("%d pushes, want 2", n)
	}
}

// Steady polling that changes nothing the page shows is pushed only on the tick
// (re-audit P2-4): 20 s of 2 s polls give the first push and one per 5 s tick, not
// one per poll.
func TestSteadyPollingPushesOnlyOnTheTick(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	f := pclock.NewFake(t0)
	h := NewHub(m, f)
	h.Coalesce, h.TickEvery = 150*time.Millisecond, 5*time.Second
	ch, cancel := h.subscribe()
	defer cancel()
	startHub(t, h, f)
	pushes, rounds := 0, int64(0)
	elapsed, nextTick := time.Duration(0), h.TickEvery
	advance := func(d time.Duration) {
		f.Advance(d)
		elapsed += d
		for ; nextTick <= elapsed; nextTick += h.TickEvery {
			rounds++
		}
		hubRounds(t, h, rounds)
		if pushed(ch) {
			pushes++
		}
	}
	for n := int64(1); n <= 10; n++ { // a poll every 2 s for 20 s
		h.Update(func(m *state.Model, now time.Time) {
			m.ApplyAgentsTimed(waitingAgent("idle", ""), nil, 40*time.Millisecond, now)
			m.ApplyObs(state.Obs{AgentsPolls: n, AgentsPollMs: 30 + n})
		})
		f.BlockUntil(2) // the hub coalesces
		rounds++
		advance(h.Coalesce)
		advance(2*time.Second - h.Coalesce)
	}
	// The first push, then the ticks at 5, 10, 15 and 20 s.
	if pushes != 5 || h.Pushes() != 5 {
		t.Fatalf("%d pushes (%d counted) in 20 s of polls that changed nothing, want 5: the first and one per tick", pushes, h.Pushes())
	}
}
