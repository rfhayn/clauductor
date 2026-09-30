package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// The spend ledger keeps what the status line says each session cost, a day at a
// time, broken down by lane type, model and branch, so the Metrics view has spend
// for 90 days and a change's budget can be checked against everything its lanes
// spent, across panel restarts. It is the project's own file,
// ~/.clauductor/panel/<project hash>/spend.json (0600).

// LedgerDays is how far back the ledger keeps spend: the longest window, and a month.
const LedgerDays = 120

// seenKeep is how long the ledger remembers a session's last total, so a session
// that posts again after a restart adds only what it spent since.
const seenKeep = 14 * 24 * time.Hour

// Spend is one line of a day: what sessions of one lane type, model and branch cost.
type Spend struct {
	Type   string  `json:"type,omitempty"`
	Model  string  `json:"model,omitempty"`
	Branch string  `json:"branch,omitempty"`
	USD    float64 `json:"usd"`
}

type seenCost struct {
	USD float64 `json:"usd"`
	At  int64   `json:"at"` // unix seconds
}

// Observation is a session's cumulative status-line cost as last posted, and where
// it ran. The model hands these over; the ledger turns them into spend.
type Observation struct {
	Session  string
	TotalUSD float64
	Day      string // the local day of the post, 2006-01-02
	Type     string
	Model    string
	Branch   string
	At       time.Time
}

type ledgerFile struct {
	Version int                 `json:"version"`
	Days    map[string][]Spend  `json:"days"`
	Seen    map[string]seenCost `json:"seen"`
}

// Ledger is the spend ledger. Its methods are safe for concurrent use.
type Ledger struct {
	path  string
	mu    sync.Mutex
	f     ledgerFile
	dirty bool
}

// OpenLedger reads the ledger at path; a missing or unreadable file starts empty
// (spend is a record, never a reason not to start).
func OpenLedger(path string) *Ledger {
	l := &Ledger{path: path, f: ledgerFile{Version: 1, Days: map[string][]Spend{}, Seen: map[string]seenCost{}}}
	if b, err := os.ReadFile(path); err == nil {
		var f ledgerFile
		if json.Unmarshal(b, &f) == nil && f.Version == 1 {
			if f.Days != nil {
				l.f.Days = f.Days
			}
			if f.Seen != nil {
				l.f.Seen = f.Seen
			}
		}
	}
	return l
}

// LedgerPath is where a project's ledger lives.
func LedgerPath(home, root string) string {
	return filepath.Join(config.ProjectDir(home, root), "spend.json")
}

// Observe adds what a session spent since its last observation. A session the
// ledger has not seen adds its whole total: that was spent, and nothing recorded it.
func (l *Ledger) Observe(o Observation) {
	if o.Session == "" || o.TotalUSD < 0 || o.Day == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delta := o.TotalUSD
	if prev, ok := l.f.Seen[o.Session]; ok {
		delta = o.TotalUSD - prev.USD
		if delta < 0 {
			// A total never falls; if one does, count nothing and keep the highest,
			// so rising back to it is not counted twice.
			return
		}
	}
	l.f.Seen[o.Session] = seenCost{USD: o.TotalUSD, At: o.At.Unix()}
	l.dirty = true
	if delta <= 0 {
		return
	}
	day := l.f.Days[o.Day]
	for i := range day {
		if day[i].Type == o.Type && day[i].Model == o.Model && day[i].Branch == o.Branch {
			day[i].USD += delta
			return
		}
	}
	l.f.Days[o.Day] = append(day, Spend{Type: o.Type, Model: o.Model, Branch: o.Branch, USD: delta})
}

// Save writes the ledger if it changed, first dropping days and sessions past what
// it keeps.
func (l *Ledger) Save(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return nil
	}
	cut := now.AddDate(0, 0, -LedgerDays).Format("2006-01-02")
	for d := range l.f.Days {
		if d < cut {
			delete(l.f.Days, d)
		}
	}
	old := now.Add(-seenKeep).Unix()
	for s, c := range l.f.Seen {
		if c.At < old {
			delete(l.f.Seen, s)
		}
	}
	b, err := json.Marshal(l.f)
	if err != nil {
		return err
	}
	if err := config.EnsurePrivateDir(filepath.Dir(l.path)); err != nil {
		return err
	}
	if err := config.WriteAtomic(l.path, b, 0o600); err != nil {
		return err
	}
	l.dirty = false
	return nil
}

// Days returns a copy of the ledger's days, keyed 2006-01-02.
func (l *Ledger) Days() map[string][]Spend {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string][]Spend, len(l.f.Days))
	for d, s := range l.f.Days {
		out[d] = append([]Spend(nil), s...)
	}
	return out
}

// FirstDay is the oldest day the ledger has, or "".
func FirstDay(days map[string][]Spend) string {
	ks := make([]string, 0, len(days))
	for d := range days {
		ks = append(ks, d)
	}
	sort.Strings(ks)
	if len(ks) == 0 {
		return ""
	}
	return ks[0]
}

// ByBranch sums the ledger's spend per branch, over every day it keeps.
func ByBranch(days map[string][]Spend) map[string]float64 {
	out := map[string]float64{}
	for _, d := range days {
		for _, s := range d {
			if s.Branch != "" {
				out[s.Branch] += s.USD
			}
		}
	}
	return out
}
