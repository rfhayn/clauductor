package claudecall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/lease"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/types"
)

// LogName is the login-lock log's file name in the panel's directory, beside quota.json.
const LogName = "login-lock.jsonl"

// LogMax caps the log's size. Past it, the oldest lines go and the newest half stays:
// a lock line is a few hundred bytes and a refresh is rare, so this holds far more
// than the 14 days the outcome check reads.
const LogMax = 1 << 20

// WatchEvery is how often the watch stats the locks: a stat spawns nothing.
const WatchEvery = 2 * time.Second

// Log is the append-only, size-capped login-lock log.
type Log struct {
	Path string
	Max  int64 // 0 is LogMax

	mu sync.Mutex
}

// NewLog is the log in the panel's directory of home.
func NewLog(home string) *Log {
	return &Log{Path: filepath.Join(config.PanelDir(home), LogName)}
}

// Append writes v as one JSON line.
func (l *Log) Append(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := config.EnsurePrivateDir(filepath.Dir(l.Path)); err != nil {
		return err
	}
	max := l.Max
	if max <= 0 {
		max = LogMax
	}
	if fi, err := os.Stat(l.Path); err == nil && fi.Size()+int64(len(line)) > max {
		if err := l.trim(max / 2); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// trim keeps the newest whole lines that fit in keep bytes.
func (l *Log) trim(keep int64) error {
	b, err := os.ReadFile(l.Path)
	if err != nil {
		return err
	}
	if cut := int64(len(b)) - keep; cut > 0 {
		// A cut that lands just after a newline is already at a whole line.
		mid := b[cut-1] != '\n'
		b = b[cut:]
		if mid {
			if i := bytes.IndexByte(b, '\n'); i >= 0 {
				b = b[i+1:]
			} else {
				b = nil
			}
		}
	}
	return config.WriteAtomic(l.Path, b, 0o600)
}

// How a lock's sighting ended.
const (
	EndGone     = "gone"          // no longer there
	EndReplaced = "replaced"      // another lock (another birth time) at the same path
	EndStopped  = "panel stopped" // still there when the watch stopped
)

// LockLine is the log's one line per lock seen, written when the sighting ends.
type LockLine struct {
	Kind  string `json:"kind"` // "lock"
	Which string `json:"which"`
	Path  string `json:"path"`
	// Holder says whose lock it is: "own" (the panel's own call, named in Call),
	// "other", or "unknown: <why>".
	Holder string `json:"holder"`
	Own    bool   `json:"own"`
	Call   string `json:"call,omitempty"`
	PID    int    `json:"pid,omitempty"`
	// Command is `ps -o command=` of the holder at first sight; "" with
	// HolderRunning false when no such process ran by then.
	Command       string `json:"command,omitempty"`
	HolderRunning bool   `json:"holder_running"`
	// HolderState is the holder as last checked: HolderAlive, HolderGone or
	// HolderPIDReused ("" when no record names one).
	HolderState  string     `json:"holder_state,omitempty"`
	Birth        *time.Time `json:"birth,omitempty"` // the owner record's birth time
	FirstSeen    time.Time  `json:"first_seen"`
	LastModified time.Time  `json:"last_modified"`
	LastState    State      `json:"last_state"`
	Ended        string     `json:"ended"`
	EndedAt      time.Time  `json:"ended_at"`
}

// Attribution is whose a lock is, decided once, at the sighting that first names
// its holder. The zero value is "not the panel's own".
type Attribution struct {
	Own  bool
	Call string // which of the panel's calls, when Own
	// Unknown, when set, is why the holder can't be told ("before this panel started").
	Unknown string
}

// A holder as the watch last checked it (LockLine.HolderState, types.LoginLock.HolderState).
const (
	HolderAlive = "running"
	HolderGone  = "gone"
	// HolderPIDReused: its pid runs, but another process: `ps -o lstart=` differs
	// from the record's procStart, or the pid came back after it was seen gone.
	HolderPIDReused = "gone (pid reused)"
)

// SettleWait is how long a lock that has lost its identity (the record that named
// it gone, the new one not yet written) is held to its old sighting before it counts
// as another lock whose holder wrote no record. Claude Code writes the record just
// after it takes the lock, so a retaken lock settles within a tick or two.
const SettleWait = 10 * time.Second

// liveGapTicks is how many watch intervals may pass between two ticks before a live
// run is broken: the panel's ticks can be late by a busy moment, never by minutes
// unless the machine slept.
const liveGapTicks = 3

// Watch stats both locks every WatchEvery and logs each lock it sees.
type Watch struct {
	Reader Reader
	Log    *Log
	Clock  clock.Clock
	Every  time.Duration // 0 is WatchEvery
	// Run runs `ps -o command=` for a holder (signals.ExecRunner in the panel).
	Run signals.Runner
	// Attribute decides whether a holder is one of the panel's own calls (the gate's
	// call history supplies it). nil logs the holder "unknown: no call history", never
	// "other": a lock the panel stranded must not read as someone else's.
	Attribute func(h Holder) Attribution
	// Out receives a failed log write, once per distinct error, so a log that
	// cannot be written is said rather than silently empty.
	Out io.Writer
	// Alive says whether a holder's pid exists, spawning nothing (nil is
	// lease.PIDAlive).
	Alive func(pid int) bool
	// OnChange receives the locks present, each time what the watch reads of them
	// changes: the panel's alerts are derived from them (state.Model.ApplyLoginLocks).
	OnChange func([]types.LoginLock)

	seen    map[string]*sighting // by Which
	lastErr string
	last    []types.LoginLock
	// lastTick is when Tick last finished: a gap of more than liveGapTicks ticks
	// breaks the live runs.
	lastTick time.Time
}

// sighting is one lock's unbroken run of sightings. It is attributed once its line
// has the record's birth time (line.Birth), and never again.
type sighting struct {
	id   string // the record's identity; "" while it is the path's unbroken run
	line LockLine
	// liveSince starts this identity's current unbroken run of live readings.
	liveSince time.Time
	// pendingSince is when the lock was first read without the identity this
	// sighting has (released and retaken between ticks, or its record rewritten).
	pendingSince time.Time
	// procStart is the record's; checked says it has been compared with the pid's
	// start time since the pid was last seen alive.
	procStart string
	checked   bool
}

// Loop runs the watch until ctx ends, then logs each lock still present as
// "panel stopped".
func (w *Watch) Loop(ctx context.Context) {
	every := w.Every
	if every <= 0 {
		every = WatchEvery
	}
	for {
		w.Tick(ctx)
		t := w.Clock.NewTimer(every)
		select {
		case <-ctx.Done():
			t.Stop()
			w.Stop()
			return
		case <-t.C():
		}
	}
}

// Tick reads both locks once and logs every sighting that ended.
func (w *Watch) Tick(ctx context.Context) {
	if w.seen == nil {
		w.seen = map[string]*sighting{}
	}
	now := w.Clock.Now()
	// A gap of several ticks (the machine asleep) breaks every live run: the watch
	// cannot say the lock stayed live while it did not look.
	every := w.Every
	if every <= 0 {
		every = WatchEvery
	}
	gap := longGap(w.lastTick, now, every)
	var views []types.LoginLock
	for _, s := range w.Reader.Read(now).Locks() {
		cur := w.seen[s.Which]
		if s.State == None {
			if cur != nil {
				w.end(cur, EndGone, now)
				delete(w.seen, s.Which)
			}
			continue
		}
		id := s.ID()
		switch {
		case cur != nil && cur.id != "" && id == "":
			// The lock names no holder now: released and retaken between two ticks
			// before the new holder wrote its record, or its record being rewritten.
			// The old line stays as it was until the identity settles.
			if cur.pendingSince.IsZero() {
				cur.pendingSince = now
			}
			if now.Sub(cur.pendingSince) < SettleWait {
				views = append(views, w.view(cur, s))
				continue
			}
			// It never settled: another lock, whose holder wrote no record.
			cur = w.replace(cur, s, cur.pendingSince)
		case cur != nil && cur.id != "" && id != cur.id:
			first := now
			if !cur.pendingSince.IsZero() {
				first = cur.pendingSince // first seen when it lost the old identity
			}
			cur = w.replace(cur, s, first)
		}
		if cur == nil {
			cur = w.start(s, now)
		}
		cur.pendingSince = time.Time{}
		if cur.id == "" && id != "" {
			// The record names this lock's holder now (the holder writes it just
			// after it takes the lock): the same unbroken run, now with its identity.
			cur.id = id
		}
		if s.ModTime.After(cur.line.LastModified) {
			cur.line.LastModified = s.ModTime
		}
		cur.line.LastState = s.State
		switch {
		case s.State != Live:
			cur.liveSince = time.Time{}
		case cur.liveSince.IsZero() || gap:
			cur.liveSince = now
		}
		if cur.line.Birth == nil {
			w.attribute(ctx, cur, s)
		}
		w.checkHolder(ctx, cur)
		views = append(views, w.view(cur, s))
	}
	w.emit(views)
	// Read at the end: a slow tick (a ps of up to 5 s) is no gap before the next.
	w.lastTick = w.Clock.Now()
}

// longGap reports whether a tick at now comes more than liveGapTicks intervals after
// the last one ended, at prev. By the wall clock: Go's time.Sub would use the
// monotonic reading, which stops while the machine sleeps (darwin's
// mach_absolute_time, Linux's CLOCK_MONOTONIC), and the alerts compare wall times.
func longGap(prev, now time.Time, every time.Duration) bool {
	return !prev.IsZero() && now.Round(0).Sub(prev.Round(0)) > liveGapTicks*every
}

// start is a new sighting of s, first seen at first.
func (w *Watch) start(s LockState, first time.Time) *sighting {
	cur := &sighting{line: LockLine{Kind: "lock", Which: s.Which, Path: s.Path, FirstSeen: first,
		Holder: "unknown: no owner record"}}
	w.seen[s.Which] = cur
	return cur
}

// replace ends cur as replaced, at first (when its successor was first seen), and
// starts the successor's sighting from then.
func (w *Watch) replace(cur *sighting, s LockState, first time.Time) *sighting {
	w.end(cur, EndReplaced, first)
	return w.start(s, first)
}

// view is the lock s as the sighting cur knows it.
func (w *Watch) view(cur *sighting, s LockState) types.LoginLock {
	v := types.LoginLock{Which: s.Which, Path: s.Path, State: string(s.State), ModTime: s.ModTime.UnixMilli(), PID: cur.line.PID,
		Command: cur.line.Command, HolderState: cur.line.HolderState, Holder: cur.line.Holder, Call: cur.line.Call}
	if cur.line.Birth != nil {
		v.Birth = cur.line.Birth.UnixMilli()
	}
	switch {
	case s.State == Live && !cur.liveSince.IsZero():
		v.LiveSince = cur.liveSince.UnixMilli()
	case s.State == Stale:
		v.StaleSince = s.ModTime.Add(StaleAfter).UnixMilli()
	}
	return v
}

// emit hands OnChange the locks when they differ from the last it was handed.
func (w *Watch) emit(views []types.LoginLock) {
	if reflect.DeepEqual(views, w.last) {
		return
	}
	w.last = views
	if w.OnChange != nil {
		w.OnChange(append([]types.LoginLock(nil), views...))
	}
}

// checkHolder updates the holder's state, spawning nothing while it is unchanged:
// its pid's existence is kill(pid, 0), and its start time (one ps) is compared with
// the record's procStart once each time the pid is seen alive anew. Gone is final,
// and a pid seen alive after it was seen gone names another process.
func (w *Watch) checkHolder(ctx context.Context, cur *sighting) {
	pid := cur.line.PID
	if pid <= 0 || cur.line.HolderState == HolderPIDReused {
		return
	}
	alive := lease.PIDAlive
	if w.Alive != nil {
		alive = w.Alive
	}
	switch {
	case !alive(pid):
		cur.line.HolderState, cur.checked = HolderGone, false
	case cur.line.HolderState == HolderGone:
		cur.line.HolderState = HolderPIDReused
	case !cur.checked:
		cur.checked = true
		cur.line.HolderState = HolderAlive
		if w.otherStart(ctx, pid, cur.procStart) {
			cur.line.HolderState = HolderPIDReused
		}
	}
}

// otherStart reports whether pid now names a process other than the one whose start
// time Claude Code recorded: procStart is `LC_ALL=C TZ=UTC ps -o lstart= -p <pid>`
// as Claude Code 2.1.296 reads it of itself. No procStart, or a ps that tells
// nothing, is no evidence.
func (w *Watch) otherStart(ctx context.Context, pid int, procStart string) bool {
	if w.Run == nil || strings.TrimSpace(procStart) == "" {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := w.Run(cctx, "", []string{"/usr/bin/env", "LC_ALL=C", "TZ=UTC", "ps", "-o", "lstart=", "-p", strconv.Itoa(pid)})
	now := strings.Join(strings.Fields(string(out)), " ")
	return err == nil && now != "" && now != strings.Join(strings.Fields(procStart), " ")
}

// attribute fills the holder at the first sighting that names one, and never again:
// a lock stranded for hours keeps what it got while its call was still known.
func (w *Watch) attribute(ctx context.Context, cur *sighting, s LockState) {
	if s.Holder == nil {
		switch {
		case s.RecordUnreadable:
			cur.line.Holder = "unknown: owner record unreadable"
		case s.NotInRecord:
			cur.line.Holder = "unknown: the record names no " + s.Which + " lock"
		}
		return
	}
	h := *s.Holder
	birth := h.Birth
	cur.line.PID, cur.line.Birth, cur.procStart = h.PID, &birth, h.ProcStart
	// Its command only when the pid is still the holder's: a reused pid's command
	// is another process's.
	w.checkHolder(ctx, cur)
	if cur.line.HolderState == HolderAlive {
		cur.line.Command, cur.line.HolderRunning = w.command(ctx, h.PID)
	}
	a := Attribution{Unknown: "no call history"}
	if w.Attribute != nil {
		a = w.Attribute(h)
	}
	switch {
	case a.Unknown != "":
		cur.line.Holder = "unknown: " + a.Unknown
	case a.Own:
		cur.line.Holder, cur.line.Own, cur.line.Call = "own", true, a.Call
	default:
		cur.line.Holder = "other"
	}
}

// command is the holder's `ps -o command=`, and whether ps found the process.
func (w *Watch) command(ctx context.Context, pid int) (string, bool) {
	if w.Run == nil || pid <= 0 {
		return "", false
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := w.Run(cctx, "", []string{"ps", "-o", "command=", "-p", strconv.Itoa(pid)})
	cmd := strings.TrimSpace(string(out))
	if err != nil || cmd == "" {
		return "", false
	}
	return cmd, true
}

// Stop logs each lock still present as "panel stopped".
func (w *Watch) Stop() {
	now := w.Clock.Now()
	for _, which := range []string{Current, Legacy} {
		if cur := w.seen[which]; cur != nil {
			w.end(cur, EndStopped, now)
			delete(w.seen, which)
		}
	}
}

func (w *Watch) end(cur *sighting, how string, now time.Time) {
	cur.line.Ended, cur.line.EndedAt = how, now
	err := w.Log.Append(cur.line)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg != w.lastErr && msg != "" && w.Out != nil {
		fmt.Fprintf(w.Out, "login-lock log: %v\n", err)
	}
	w.lastErr = msg
}
