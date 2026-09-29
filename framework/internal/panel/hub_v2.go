package panel

import (
	"time"

	"github.com/clauductor/clauductor/internal/panel/state"
)

// Read runs fn on the model under the hub's lock without scheduling a broadcast.
// fn must only read.
func (h *Hub) Read(fn func(m *state.Model, now time.Time)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn(h.model, h.now())
}

// View returns the current view as a value.
func (h *Hub) View() state.View {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.model.Snapshot(h.now())
}
