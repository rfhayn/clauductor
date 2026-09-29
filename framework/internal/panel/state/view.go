package state

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/clauductor/clauductor/internal/panel/lease"
)

// FullKey identifies everything in a view but the clock it was taken at.
func FullKey(v View) [sha256.Size]byte {
	v.Now = 0
	b, _ := json.Marshal(v)
	return sha256.Sum256(b)
}

// ViewKey identifies what a view says: fullKey without the polls' bookkeeping.
// Left out: when each source was last read (not whether it can be), when
// `claude agents` last answered, the footer's counters and the top bar's hook
// count, a card's run time, the quota's arrival time, and a lease's renewal.
func ViewKey(v View) [sha256.Size]byte {
	v.Now, v.AgentsReadAt, v.HookEvents, v.StatusPosts, v.Dropped = 0, 0, 0, 0, 0
	v.Observe = ObsView{}
	src := make(map[string]SourceStatus, len(v.Sources))
	for k, s := range v.Sources {
		s.At = 0
		src[k] = s
	}
	v.Sources = src
	v.QueuesSrc.At = 0
	if v.Quota != nil {
		q := *v.Quota
		q.At = 0
		v.Quota = &q
	}
	cards := make([]CardState, len(v.Cards))
	for i, c := range v.Cards {
		c.Source.At = 0
		cards[i] = c
	}
	v.Cards = cards
	qs := make([]lease.QueueView, len(v.Queues))
	for i, q := range v.Queues {
		if q.Holder != nil {
			h := *q.Holder
			h.Renewed = 0
			q.Holder = &h
		}
		ws := make([]lease.LeaseView, len(q.Waiters))
		for j, w := range q.Waiters {
			w.Renewed = 0
			ws[j] = w
		}
		q.Waiters = ws
		qs[i] = q
	}
	v.Queues = qs
	return FullKey(v)
}
