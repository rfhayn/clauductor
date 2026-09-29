package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// Hub owns the model and fans snapshots out to SSE subscribers.
//
// It pushes a snapshot at once only when what the view says changed (viewKey).
// Every age on the page is computed in the browser from a timestamp, and the
// bookkeeping of the polls (when each source was last read, the footer's counters,
// a lease's renewal) moves every 2 s while nothing happens: none of that is news.
// It still reaches the page, on the 5 s tick, if it is all that changed. The SSE
// handler sends its own heartbeat (HeartbeatEvery) so the page can still tell a
// quiet panel from a dead one.
type Hub struct {
	mu    sync.Mutex
	model *state.Model
	clock clock.Clock
	subs  map[chan []byte]struct{}
	dirty chan struct{}
	// last is the key (viewKey) of the last snapshot broadcast, and lastFull the
	// key of all of it (fullKey).
	last, lastFull [sha256.Size]byte
	sent           bool
	// Coalesce is how long an update waits for more before the push. Zero means 150 ms.
	Coalesce time.Duration
	// TickEvery is how often derived state (stale hooks, approximate readings) is
	// re-derived when nothing arrives. Zero means 5 s.
	TickEvery time.Duration
	// pushes counts broadcasts, for tests and the footer.
	pushes atomic.Int64
}

// NewHub wraps a model.
func NewHub(m *state.Model, c clock.Clock) *Hub {
	return &Hub{model: m, clock: clock.Or(c), subs: map[chan []byte]struct{}{}, dirty: make(chan struct{}, 1)}
}

// Update applies fn to the model under the lock and schedules a broadcast.
func (h *Hub) Update(fn func(m *state.Model, now time.Time)) {
	h.mu.Lock()
	fn(h.model, h.clock.Now())
	h.mu.Unlock()
	select {
	case h.dirty <- struct{}{}:
	default:
	}
}

// Snapshot returns the current view as JSON.
func (h *Hub) Snapshot() []byte {
	b, _, _ := h.snapshotKeyed()
	return b
}

// snapshotKeyed returns the current view as JSON, its key and its full key.
func (h *Hub) snapshotKeyed() ([]byte, [sha256.Size]byte, [sha256.Size]byte) {
	h.mu.Lock()
	v := h.model.Snapshot(h.clock.Now())
	h.mu.Unlock()
	b, _ := json.Marshal(v)
	return b, state.ViewKey(v), state.FullKey(v)
}

// Pushes is how many snapshots the hub has broadcast.
func (h *Hub) Pushes() int64 { return h.pushes.Load() }

func (h *Hub) subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Run broadcasts coalesced updates, plus a periodic re-derivation so state that
// moves with time alone (the stale-hook banner, an approximate reading) still
// reaches the page. Either way a view equal to the last one pushed is not pushed.
func (h *Hub) Run(ctx context.Context) {
	every := h.TickEvery
	if every <= 0 {
		every = 5 * time.Second
	}
	tick := h.clock.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.dirty:
			wait := h.Coalesce
			if wait <= 0 {
				wait = 150 * time.Millisecond
			}
			h.clock.Sleep(wait) // coalesce bursts
			h.broadcast(false)
		case <-tick.C():
			h.broadcast(true)
		}
	}
}

// broadcast pushes the current view to every subscriber if what it says changed,
// or, on a tick, if anything in it changed.
func (h *Hub) broadcast(tick bool) {
	snap, key, full := h.snapshotKeyed()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sent && key == h.last && (!tick || full == h.lastFull) {
		return
	}
	h.last, h.lastFull, h.sent = key, full, true
	h.pushes.Add(1)
	for ch := range h.subs {
		select { // keep only the latest snapshot per slow subscriber
		case <-ch:
		default:
		}
		ch <- snap
	}
}

// Read runs fn on the model under the hub's lock without scheduling a broadcast.
// fn must only read.
func (h *Hub) Read(fn func(m *state.Model, now time.Time)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn(h.model, h.clock.Now())
}

// View returns the current view as a value.
func (h *Hub) View() state.View {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.model.Snapshot(h.clock.Now())
}
