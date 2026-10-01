package ratelimit

import "time"

// Limiter allows up to Max calls per Window for each key.
type Limiter struct {
	Max    int
	Window time.Duration
	hits   map[string][]time.Time
}

// New returns a Limiter of max calls per window.
func New(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, hits: map[string][]time.Time{}}
}

// Allow records a call for key at now and reports whether it is within the limit.
func (l *Limiter) Allow(key string, now time.Time) bool {
	cut := now.Add(-l.Window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.Max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
