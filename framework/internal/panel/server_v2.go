package panel

// Overflow returns how many ingest bodies were dropped because the processor was
// behind.
func (s *Server) Overflow() int64 { return s.overflow.Load() }

// FocusedLanes returns the lanes whose terminal has keyboard focus in a page now.
// The notifier sends no OS notification for them: you are looking at the lane.
func (s *Server) FocusedLanes() map[string]bool {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	out := map[string]bool{}
	for lane, vs := range s.viewers {
		for v := range vs {
			if v.focused.Load() {
				out[lane] = true
			}
		}
	}
	return out
}
