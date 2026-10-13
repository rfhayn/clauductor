// Package claudecall is how the panel lives beside Claude Code's login refresh
// (PANEL-28): it reads the refresh locks Claude Code takes, watches and logs them, and
// (group 2) gates every `claude` the panel spawns. Both the panel and the lane manager
// import it.
//
// Nothing in this package writes, touches or removes a lock or its owner record: a
// lock a live process holds, removed, would let two processes refresh one login at
// once, which is the very thing the lock prevents.
package claudecall

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The lock's names and timings, as Claude Code 2.1.288 has them, and 2.1.296 still
// does (proper-lockfile's options `stale: 60000, update: 5000`): the holder touches
// the lock every 5 s, and a lock untouched for 60 s is stale.
const (
	currentLockName = ".oauth_refresh.lock"
	ownerRecordName = ".oauth_refresh.lock.owner"
	// StaleAfter is how long a lock goes untouched before Claude Code counts it stale.
	StaleAfter = 60 * time.Second
	// FutureSkew is how far ahead of now a lock's mtime may be and still read live;
	// beyond it the lock is future-dated (#95739): Claude Code reads it as fresh until
	// real time passes it, so treating it as live would pause the panel for hours.
	FutureSkew = 5 * time.Second
	// ownerRecordMax is the most of the owner record read, as Claude Code reads it.
	ownerRecordMax = 4096
)

// State is a lock's state, from its mtime against now.
type State string

const (
	None   State = "none"   // no lock at the path
	Live   State = "live"   // modified less than StaleAfter ago, and no more than FutureSkew ahead
	Stale  State = "stale"  // modified StaleAfter or more ago
	Future State = "future" // modified more than FutureSkew in the future
)

// StateAt is the state of a lock modified at mod, read at now.
func StateAt(mod, now time.Time) State {
	age := now.Sub(mod)
	switch {
	case age < -FutureSkew:
		return Future
	case age >= StaleAfter:
		return Stale
	default:
		return Live
	}
}

// Which lock a reading is of.
const (
	Current = "current" // <config dir>/.oauth_refresh.lock
	Legacy  = "legacy"  // <config dir>.lock, beside the config dir
)

// Holder is the process the owner record names as a lock's holder.
type Holder struct {
	PID int
	// ProcStart is the record's procStart, as Claude Code wrote it ("" if absent).
	ProcStart string
	// Birth is the lock's birth time as the record states it (lockBirthtimeMs, or
	// legacyLockBirthtimeMs for the legacy lock). It comes from the record, never from
	// the filesystem, so the code is the same on darwin and Linux.
	Birth time.Time
}

// LockState is one lock as read at one moment.
type LockState struct {
	Which   string // Current or Legacy
	Path    string
	State   State
	ModTime time.Time // zero when State is None
	// Holder is the owner record's holder of THIS lock; nil when no record names it.
	Holder *Holder
	// RecordUnreadable: an owner record exists but could not be parsed (a new Claude
	// Code format, a partial write). The lock is still read by its mtime; its holder
	// is "unknown".
	RecordUnreadable bool
	// NotInRecord: a readable owner record exists but names no holder of THIS lock (a
	// legacy lock whose record carries no legacyLockBirthtimeMs). Its holder is unknown.
	NotInRecord bool
}

// ID is the lock's identity as far as the reading tells it: the record's birth time
// when the record names this lock's holder, else "" (the caller falls back on the
// path and an unbroken run of sightings).
func (s LockState) ID() string {
	if s.Holder == nil {
		return ""
	}
	return s.Which + "@" + s.Holder.Birth.UTC().Format(time.RFC3339Nano)
}

// Reading is both locks as read at one moment.
type Reading struct {
	Current, Legacy LockState
}

// Locks is the two locks, current first.
func (r Reading) Locks() []LockState { return []LockState{r.Current, r.Legacy} }

// Reader reads Claude Code's refresh locks for one config dir. It never writes.
type Reader struct {
	Dir string // the Claude Code config dir
}

// ConfigDir is the config dir Claude Code uses: $CLAUDE_CONFIG_DIR, else the
// home's .claude. getenv is os.Getenv in the panel; a blank value counts as unset.
func ConfigDir(getenv func(string) string, home string) string {
	if d := strings.TrimSpace(getenv("CLAUDE_CONFIG_DIR")); d != "" {
		return filepath.Clean(d)
	}
	return filepath.Join(home, ".claude")
}

// NewReader is the Reader for the config dir ConfigDir names.
func NewReader(getenv func(string) string, home string) Reader {
	return Reader{Dir: ConfigDir(getenv, home)}
}

// CurrentPath is <config dir>/.oauth_refresh.lock.
func (r Reader) CurrentPath() string { return filepath.Join(r.Dir, currentLockName) }

// OwnerPath is the owner record, <config dir>/.oauth_refresh.lock.owner.
func (r Reader) OwnerPath() string { return filepath.Join(r.Dir, ownerRecordName) }

// LegacyPath is the legacy lock beside the config dir. Claude Code names it after
// the config dir's real path (realpath + ".lock"), so a symlinked config dir is
// resolved the same way; a dir that does not resolve is used as it is, as Claude Code
// does.
func (r Reader) LegacyPath() string {
	dir := filepath.Clean(r.Dir)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return dir + ".lock"
}

// Read is both locks' state at now: a stat of each and a read of the owner record.
func (r Reader) Read(now time.Time) Reading {
	rec, recOK, recErr := readOwnerRecord(r.OwnerPath())
	unreadable := recErr != nil
	cur := statLock(Current, r.CurrentPath(), now)
	leg := statLock(Legacy, r.LegacyPath(), now)
	if cur.State != None {
		cur.RecordUnreadable = unreadable
		if recOK && rec.LockBirthtimeMs != nil {
			cur.Holder = &Holder{PID: rec.PID, ProcStart: rec.ProcStart, Birth: msTime(*rec.LockBirthtimeMs)}
		}
	}
	if leg.State != None {
		leg.RecordUnreadable = unreadable
		// The record names the legacy lock's holder only when it carries that lock's
		// birth time: a holder that took no legacy lock leaves it out, and a legacy
		// lock without it may be another process's.
		if recOK && rec.LegacyLockBirthtimeMs != nil {
			leg.Holder = &Holder{PID: rec.PID, ProcStart: rec.ProcStart, Birth: msTime(*rec.LegacyLockBirthtimeMs)}
		} else if recOK {
			leg.NotInRecord = true
		}
	}
	return Reading{Current: cur, Legacy: leg}
}

func statLock(which, path string, now time.Time) LockState {
	s := LockState{Which: which, Path: path, State: None}
	fi, err := os.Lstat(path)
	if err != nil {
		return s
	}
	s.ModTime = fi.ModTime()
	s.State = StateAt(s.ModTime, now)
	return s
}

// ownerRecord is Claude Code's owner record. Only the fields the panel uses are
// kept; others (pidDomain, pidSpace) are ignored.
type ownerRecord struct {
	PID                   int      `json:"pid"`
	ProcStart             string   `json:"procStart"`
	LockBirthtimeMs       *float64 `json:"lockBirthtimeMs"`
	LegacyLockBirthtimeMs *float64 `json:"legacyLockBirthtimeMs"`
}

var errUnreadable = errors.New("owner record unreadable")

// readOwnerRecord reads the record defensively. ok is false with a nil error when
// there is no record; an error means one exists and could not be used.
func readOwnerRecord(path string) (rec ownerRecord, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, errUnreadable
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, ownerRecordMax+1))
	if err != nil || len(b) > ownerRecordMax {
		return rec, false, errUnreadable
	}
	if json.Unmarshal(b, &rec) != nil || rec.PID <= 0 || !validMs(rec.LockBirthtimeMs) {
		return ownerRecord{}, false, errUnreadable
	}
	if rec.LegacyLockBirthtimeMs != nil && !validMs(rec.LegacyLockBirthtimeMs) {
		rec.LegacyLockBirthtimeMs = nil
	}
	return rec, true, nil
}

// validMs: a positive, finite millisecond time (Claude Code writes the filesystem's
// birthtimeMs, which may carry a fraction).
func validMs(v *float64) bool {
	return v != nil && *v > 0 && !math.IsInf(*v, 0) && !math.IsNaN(*v) && *v < 1e15
}

func msTime(ms float64) time.Time {
	whole := math.Floor(ms)
	return time.UnixMilli(int64(whole)).Add(time.Duration((ms - whole) * float64(time.Millisecond)))
}
