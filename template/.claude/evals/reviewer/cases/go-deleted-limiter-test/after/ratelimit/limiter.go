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
	cutoff := now.Add(-l.Window)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) > l.Max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}
