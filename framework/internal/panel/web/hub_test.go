package web

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// fakeClock advances by step on every read, so every snapshot has a different Now.
type fakeClock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

func TestHubPushesOnlyAChangedView(t *testing.T) {
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	clock := &fakeClock{t: t0, step: time.Second}
	h := NewHub(m, clock.now)
	h.TickEvery = 10 * time.Millisecond
	ch, cancel := h.subscribe()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)

	recv := func(within time.Duration) bool {
		select {
		case <-ch:
			return true
		case <-time.After(within):
			return false
		}
	}
	if !recv(time.Second) {
		t.Fatal("the first view was never pushed")
	}
	// 30 ticks, each at a later `now`, and nothing else changes: nothing is pushed.
	if recv(300 * time.Millisecond) {
		t.Fatalf("an unchanged view was pushed again (%d pushes)", h.Pushes())
	}
	// An update that changes nothing the page shows is not pushed either.
	h.Update(func(m *state.Model, now time.Time) {})
	if recv(400 * time.Millisecond) {
		t.Fatal("a no-op update was pushed")
	}
	// A real change is pushed, once.
	h.Update(func(m *state.Model, now time.Time) {
		m.ApplyPRs([]signals.PR{{Number: 7, Title: "Synthetic"}}, nil, now)
	})
	if !recv(time.Second) {
		t.Fatal("a changed view was not pushed")
	}
	if recv(300 * time.Millisecond) {
		t.Fatal("a changed view was pushed more than once")
	}
	if n := h.Pushes(); n != 2 {
		t.Fatalf("%d pushes, want 2", n)
	}
}

// Steady polling that changes nothing the page shows is pushed only on the tick
// (re-audit P2-4): 20 s of 2 s polls, scaled down 20 times, give at most one push
// per 5 s tick, not one per poll.
func TestSteadyPollingPushesOnlyOnTheTick(t *testing.T) {
	m := alertModel(t, "")
	h := NewHub(m, time.Now)
	h.Coalesce, h.TickEvery = 5*time.Millisecond, 250*time.Millisecond // 5 s → 250 ms
	ch, cancel := h.subscribe()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)
	pushes := 0
	deadline := time.After(time.Second)            // 20 s
	poll := time.NewTicker(100 * time.Millisecond) // 2 s
	defer poll.Stop()
	for n := int64(1); ; n++ {
		select {
		case <-ch:
			pushes++
			n--
			continue
		case <-poll.C:
			h.Update(func(m *state.Model, now time.Time) {
				m.ApplyAgentsTimed(waitingAgent("idle", ""), nil, 40*time.Millisecond, now)
				m.ApplyObs(state.Obs{AgentsPolls: n, AgentsPollMs: 30 + n})
			})
			continue
		case <-deadline:
		}
		break
	}
	// The ticks at 250, 500, 750 and 1000 ms, plus the first push.
	if pushes > 6 {
		t.Fatalf("%d pushes in 20 s (scaled) of polls that changed nothing, want at most 6", pushes)
	}
	if pushes < 2 {
		t.Fatalf("%d pushes: the bookkeeping never reached the page on the tick", pushes)
	}
}
