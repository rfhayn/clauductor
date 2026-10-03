package claudecall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
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
	if int64(len(b)) > keep {
		b = b[int64(len(b))-keep:]
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		} else {
			b = nil
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
	Command       string     `json:"command,omitempty"`
	HolderRunning bool       `json:"holder_running"`
	Birth         *time.Time `json:"birth,omitempty"` // the owner record's birth time
	FirstSeen     time.Time  `json:"first_seen"`
	LastModified  time.Time  `json:"last_modified"`
	LastState     State      `json:"last_state"`
	Ended         string     `json:"ended"`
	EndedAt       time.Time  `json:"ended_at"`
}

// Attribution is whose a lock is, decided once, at the sighting that first names
// its holder.
type Attribution struct {
	Own  bool
	Call string // which of the panel's calls, when Own
	// Unknown, when set, is why the holder can't be told ("before this panel started").
	Unknown string
}

// Watch stats both locks every WatchEvery and logs each lock it sees.
type Watch struct {
	Reader Reader
	Log    *Log
	Clock  clock.Clock
	Every  time.Duration // 0 is WatchEvery
	// Run runs `ps -o command=` for a holder (signals.ExecRunner in the panel).
	Run signals.Runner
	// Attribute decides whether a holder is one of the panel's own calls; nil is "not
	// the panel's" (the gate's call history supplies it).
	Attribute func(h Holder) Attribution
	// Out receives a failed log write, once per distinct error, so a log that
	// cannot be written is said rather than silently empty.
	Out io.Writer

	seen    map[string]*sighting // by Which
	lastErr string
}

type sighting struct {
	id         string // the record's identity; "" while it is the path's unbroken run
	line       LockLine
	attributed bool
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
		if cur != nil && cur.id != "" && id != "" && id != cur.id {
			w.end(cur, EndReplaced, now)
			cur = nil
		}
		if cur == nil {
			cur = &sighting{line: LockLine{Kind: "lock", Which: s.Which, Path: s.Path, FirstSeen: now,
				Holder: "unknown: no owner record"}}
			w.seen[s.Which] = cur
		}
		if cur.id == "" && id != "" {
			// The record names this lock's holder now (the holder writes it just
			// after it takes the lock): the same unbroken run, now with its identity.
			cur.id = id
		}
		if s.ModTime.After(cur.line.LastModified) {
			cur.line.LastModified = s.ModTime
		}
		cur.line.LastState = s.State
		if !cur.attributed {
			w.attribute(ctx, cur, s)
		}
	}
}

// attribute fills the holder at the first sighting that names one, and never again:
// a lock stranded for hours keeps what it got while its call was still known.
func (w *Watch) attribute(ctx context.Context, cur *sighting, s LockState) {
	if s.Holder == nil {
		if s.RecordUnreadable {
			cur.line.Holder = "unknown: owner record unreadable"
		}
		return
	}
	cur.attributed = true
	h := *s.Holder
	birth := h.Birth
	cur.line.PID, cur.line.Birth = h.PID, &birth
	cur.line.Command, cur.line.HolderRunning = w.command(ctx, h.PID)
	a := Attribution{}
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
