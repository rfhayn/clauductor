package claudecall

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/signals"
)

// The gate's numbers (design D2–D4). PANEL-25's retry reuses the waits and retries.
const (
	// PollWait is how long a poll waits for the slot, or for a live lock to clear,
	// before it gives up and retries.
	PollWait = 2 * time.Second
	// BusyRetry and PausedRetry are a poll's next wait after ErrBusy and ErrPaused.
	BusyRetry   = time.Second
	PausedRetry = 2 * time.Second
	// NoDeadlineWait bounds the wait of a call whose context has no deadline.
	NoDeadlineWait = 10 * time.Second
	// ShutdownGrace is the stop grace at the panel's shutdown, not ClaudeStopGrace's
	// 15 s: it keeps the exit inside `panel install`'s wait for the old job.
	ShutdownGrace = 3 * time.Second

	// lockRecheck is how often a waiting call stats the locks again.
	lockRecheck = 250 * time.Millisecond
	// The call history: the last 10 minutes of calls, at most 2,000 (the oldest
	// dropped). Every project's agents poll at its fastest is 30 a minute.
	historyKeep = 10 * time.Minute
	historyMax  = 2000
	// killAgain is how often Close repeats its kill of the calls still running.
	killAgain = 50 * time.Millisecond
)

var (
	// ErrBusy: another claude call of the panel's holds the slot. A poll applies no
	// update and retries in BusyRetry.
	ErrBusy = errors.New("busy: another of the panel's claude calls is running")
	// ErrPaused: a login refresh holds a lock. A poll applies no update but this note,
	// and retries in PausedRetry.
	ErrPaused = errors.New("paused: a login refresh is in progress")
	// errClosed: the panel is stopping.
	errClosed = errors.New("the panel is stopping")
)

// RetryAfter is a poll's next wait when err is the gate's own refusal (ErrBusy or
// ErrPaused), which callers tell apart from a failed call: ok is false otherwise.
func RetryAfter(err error) (wait time.Duration, ok bool) {
	switch {
	case errors.Is(err, ErrPaused):
		return PausedRetry, true
	case errors.Is(err, ErrBusy):
		return BusyRetry, true
	}
	return 0, false
}

// Gate is the one way the panel's `claude` calls run (PANEL-28): a Runner wrapper
// for argv whose program is claude (signals.ClaudeArgs). Every other command passes
// straight through. It never execs anything itself, so a Runner below it (PANEL-25's
// spawn counter) still sees every process that starts.
//
//   - The slot (D3): one claude at a time, held until the inner call returns, which is
//     when its process has been reaped and what was left of its group killed
//     (ExecRunner, #318).
//   - The lock pause (D4): no start while either refresh lock is live.
//   - The stop (D2): the caller gets its timeout at its deadline; the process runs on
//     under a context of the gate's own, which is cancelled then, so ExecRunner sends
//     SIGTERM, and SIGKILL only after the grace.
//   - The call history: which of the panel's calls a lock's holder was (Attribute).
type Gate struct {
	inner  signals.Runner
	reader Reader
	clock  clock.Clock
	log    *Log

	started time.Time // the panel's start: a lock born before it has an unknown holder
	slot    chan struct{}

	mu       sync.Mutex
	history  []*call // oldest first: only calls whose process started
	inflight sync.WaitGroup
	closed   bool
}

// call is one claude call that held the slot.
type call struct {
	name             string
	proc             *os.Process
	pid              int
	spawned, ended   time.Time // ended is zero while it runs
	stopped          bool      // its caller's context ended before it did
	stoppedAt        time.Time
	locks            []string // the locks present when it was stopped
	term             bool     // SIGTERM reached its process (ExecRunner's report)
	killedAtShutdown bool     // the claude itself got the shutdown's SIGKILL
	groupGone        bool     // nothing of its group was left to kill (signals.KillCall)
}

// NewGate wraps inner. r is the locks the pause reads; log receives a line per
// stopped call; clk dates the history and times the waits.
func NewGate(inner signals.Runner, r Reader, clk clock.Clock, log *Log) *Gate {
	return &Gate{inner: inner, reader: r, clock: clk, log: log, started: clk.Now(), slot: make(chan struct{}, 1)}
}

// Run is the gated Runner.
func (g *Gate) Run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	args, ok := signals.ClaudeArgs(argv)
	if !ok {
		return g.inner(ctx, dir, argv)
	}
	if err := g.acquire(ctx); err != nil {
		return nil, err
	}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		<-g.slot
		return nil, errClosed
	}
	g.inflight.Add(1)
	g.mu.Unlock()
	// Only a call that will run is timed: one refused at the close never ran.
	if f, _ := ctx.Value(acquiredKey{}).(func()); f != nil {
		f()
	}

	c := &call{name: callName(args)}
	// The call runs under a context of its own, so the process can be stopped with a
	// grace while the caller returns at its deadline; WithoutCancel keeps the caller's
	// values (PANEL-25's project tag).
	gctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	gctx = signals.WithProcessStart(gctx, func(p *os.Process) { g.spawned(c, p) })
	gctx = signals.WithStopSignal(gctx, func(os.Signal) { g.termSent(c) })
	stopWatch := context.AfterFunc(ctx, func() {
		g.stop(c)
		cancel()
	})
	if ctx.Err() != nil {
		cancel() // ended since acquire: now, not when AfterFunc's goroutine runs, so nothing starts
	}
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer g.inflight.Done()
		out, err := g.inner(gctx, dir, argv)
		cancel()
		g.end(c, err)
		<-g.slot // held until the process is reaped: never two claude at once
		done <- result{out, err}
	}()
	select {
	case res := <-done:
		stopWatch()
		return res.out, res.err
	case <-ctx.Done():
		select {
		case res := <-done: // it ended as the caller's context did
			return res.out, res.err
		default:
		}
		return nil, fmt.Errorf("claude: %w", ctx.Err())
	}
}

type acquiredKey struct{}

// WithAcquired asks the gate to call f when a claude call under ctx has taken the
// slot, in the caller's goroutine, just before the call runs: a caller that times
// the call (the agents poll's AgentsPollAvgMs, D3's live figure) times the call, not
// its wait for the slot or a lock.
func WithAcquired(ctx context.Context, f func()) context.Context {
	return context.WithValue(ctx, acquiredKey{}, f)
}

// acquire takes the slot once no lock is live. A poll waits at most PollWait; any
// other call within its context, or NoDeadlineWait when that has no deadline.
//
// A caller whose context has already ended never takes the slot: the call would run
// under the gate's own context, which does not end with the caller's, so its claude
// would start only to be stopped.
func (g *Gate) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	var bound time.Duration
	if signals.IsPoll(ctx) {
		bound = PollWait
	} else if _, ok := ctx.Deadline(); !ok {
		bound = NoDeadlineWait
	}
	var expired <-chan time.Time
	if bound > 0 {
		t := g.clock.NewTimer(bound)
		defer t.Stop()
		expired = t.C()
	}
	for {
		if g.isClosed() {
			return errClosed
		}
		if g.reader.AnyLive(g.clock.Now()) {
			t := g.clock.NewTimer(lockRecheck)
			select {
			case <-t.C():
				continue
			case <-expired:
				t.Stop()
				return ErrPaused
			case <-ctx.Done():
				t.Stop()
				return fmt.Errorf("%w: %w", ErrPaused, ctx.Err())
			}
		}
		select {
		case g.slot <- struct{}{}:
			// select picks at random among ready cases: the slot may win over a
			// context that ended meanwhile.
			if err := ctx.Err(); err != nil {
				<-g.slot
				return fmt.Errorf("claude: %w", err)
			}
			if !g.reader.AnyLive(g.clock.Now()) {
				return nil
			}
			<-g.slot // a lock went live while it waited: wait for that instead
		case <-expired:
			return ErrBusy
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrBusy, ctx.Err())
		}
	}
}

func (g *Gate) isClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

// Quiesce takes the slot once no claude call of the panel's runs, waiting at most max
// on the gate's clock, and keeps it: no call starts after it, since the panel's restart
// after rebuilding itself (OPS-118 D4) follows. It says whether it took the slot: false
// when max passed with a call still running, or ctx ended first; the restart goes
// ahead either way, and the shutdown's Close stops what still runs. Close needs no
// slot, so a kept one holds nothing up.
func (g *Gate) Quiesce(ctx context.Context, max time.Duration) bool {
	t := g.clock.NewTimer(max)
	defer t.Stop()
	select {
	case g.slot <- struct{}{}:
		return true
	case <-t.C():
		return false
	case <-ctx.Done():
		return false
	}
}

// spawned is ExecRunner's report of the started process: the call enters the
// history, dated by the panel's clock now (no ps per call).
func (g *Gate) spawned(c *call, p *os.Process) {
	now := g.clock.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	c.proc, c.pid, c.spawned = p, p.Pid, now
	g.prune(now)
	g.history = append(g.history, c)
	if over := len(g.history) - historyMax; over > 0 {
		g.history = append([]*call(nil), g.history[over:]...)
	}
}

// prune drops the calls that ended more than historyKeep ago. g.mu is held.
func (g *Gate) prune(now time.Time) {
	kept := g.history[:0]
	for _, c := range g.history {
		if c.ended.IsZero() || now.Sub(c.ended) <= historyKeep {
			kept = append(kept, c)
		}
	}
	for i := len(kept); i < len(g.history); i++ {
		g.history[i] = nil
	}
	g.history = kept
}

// stop marks the call stopped as its caller's context ends, with the locks present
// then. It does nothing for a call that already ended.
func (g *Gate) stop(c *call) {
	now := g.clock.Now()
	var locks []string
	if g.reader.Dir != "" {
		for _, l := range g.reader.Read(now).Locks() {
			if l.State != None {
				locks = append(locks, l.Which+" "+string(l.State))
			}
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !c.ended.IsZero() {
		return
	}
	c.stopped, c.stoppedAt, c.locks = true, now, locks
}

// end dates the call's end (the inner call returned: the process is reaped), and
// logs it if it was stopped with its process running.
//
// The line is written before the call is marked ended, so a reader that sees the end
// (the history, a test) sees the line too.
func (g *Gate) end(c *call, err error) {
	now := g.clock.Now()
	g.mu.Lock()
	line := StopLine{Kind: "stopped", Call: c.name, PID: c.pid, Spawned: c.spawned, StoppedAt: c.stoppedAt, Ended: now,
		Lock: len(c.locks) > 0, Locks: c.locks, Signals: []string{}}
	if c.stopped && c.term {
		line.Signals = append(line.Signals, "SIGTERM")
	}
	if c.stopped && c.proc != nil && (c.killedAtShutdown || killedBySIGKILL(err)) {
		line.Signals = append(line.Signals, "SIGKILL")
	}
	g.mu.Unlock()
	// Only a call the panel actually signalled was stopped: one that exited by itself
	// as its caller gave up (the SIGTERM found it gone) is no stopped call.
	if len(line.Signals) > 0 && g.log != nil {
		_ = g.log.Append(line)
	}
	g.mu.Lock()
	c.ended = now
	g.mu.Unlock()
}

// termSent records ExecRunner's report that SIGTERM reached the call's process.
func (g *Gate) termSent(c *call) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c.term = true
}

func killedBySIGKILL(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL
}

// StopLine is the log's line for a claude call the panel stopped (its caller's
// timeout, or the panel's shutdown).
type StopLine struct {
	Kind      string    `json:"kind"` // "stopped"
	Call      string    `json:"call"`
	PID       int       `json:"pid"`
	Spawned   time.Time `json:"spawned"`
	StoppedAt time.Time `json:"stopped_at"`
	Ended     time.Time `json:"ended"` // reaped
	// Signals are what the panel sent it: SIGTERM, then SIGKILL if it outlived the grace.
	Signals []string `json:"signals"`
	// Lock says whether a refresh lock existed when it was stopped; Locks names them.
	Lock  bool     `json:"lock"`
	Locks []string `json:"locks,omitempty"`
}

// Attribute says whether a lock's holder is one of the panel's own calls (the
// Watch's Attribute): the record's pid matches a call in the history, and the lock's
// birth falls within that call's spawn and end. A lock born after its pid's call
// ended is another process's that reused the pid. A lock born before the panel
// started has a holder the history cannot know.
func (g *Gate) Attribute(h Holder) Attribution {
	if h.Birth.Before(g.started) {
		return Attribution{Unknown: "before this panel started"}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(g.clock.Now())
	for i := len(g.history) - 1; i >= 0; i-- {
		c := g.history[i]
		if c.pid != h.PID || h.Birth.Before(c.spawned) {
			continue
		}
		if c.ended.IsZero() || !h.Birth.After(c.ended) {
			return Attribution{Own: true, Call: c.name}
		}
	}
	return Attribution{}
}

// Close is the panel's shutdown (Run, after its goroutines are done): no call starts
// from here on. The calls still running have had SIGTERM through their cancelled
// contexts; Close waits for them at most grace, then kills each process not yet
// reaped, and waits briefly for those reaps.
func (g *Gate) Close(grace time.Duration) {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	done := make(chan struct{})
	go func() { g.inflight.Wait(); close(done) }()
	t := g.clock.NewTimer(grace)
	defer t.Stop()
	select {
	case <-done:
		return
	case <-t.C():
	}
	g.killRunning()
	// A SIGKILLed call is reaped at once and its output closes. But a child forked
	// as the kill lands can miss it (macOS) and hold the output open, so the kill is
	// sent again until every call has ended. The bound covers an inner Runner that
	// ignores its context (no process to kill).
	r := g.clock.NewTimer(time.Second)
	defer r.Stop()
	for {
		again := g.clock.NewTimer(killAgain)
		select {
		case <-done:
			again.Stop()
			return
		case <-r.C():
			again.Stop()
			return
		case <-again.C():
			g.killRunning()
		}
	}
}

// killRunning SIGKILLs every call still running, whole (signals.KillCall): a child
// left running would hold the call's output open past Close's wait, and the call
// would end, and log its line, only after the panel had exited (#143).
func (g *Gate) killRunning() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, c := range g.history {
		if !c.ended.IsZero() || c.proc == nil || c.groupGone {
			continue
		}
		claude, err := signals.KillCall(c.proc)
		c.killedAtShutdown = c.killedAtShutdown || claude
		c.groupGone = errors.Is(err, os.ErrProcessDone)
	}
}

// callName names a call by claude's own arguments, for the log and the history.
func callName(args []string) string {
	return signals.Clip(strings.Join(append([]string{"claude"}, args...), " "), 160)
}
