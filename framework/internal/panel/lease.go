package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The lease protocol serialises a shared resource (the full gate on port 3100)
// between processes that know nothing of each other or of the panel. It lives
// entirely on disk, so it survives a panel restart, and a POSIX shell can honour it
// without the panel running. macOS has no flock(1), so the lock is a DIRECTORY,
// because mkdir(2) is atomic everywhere:
//
//	<lock>/              held while it exists; mkdir creates it or fails with EEXIST
//	<lock>/owner.json    the holder: {v, nonce, pid, host, lane, cmd, started, renewed, ttl}
//	<lock>.waiters/      one <started>-<nonce>.json per waiter, same fields
//	<lock>.waiters/<nonce>.cancel   asks that waiter to give up (the panel's CANCEL)
//	<lock>.reclaim/      a short mutex taken only to remove a stale holder
//
// A holder is STALE, and may be removed by a waiter, when
//   - its pid is gone (same host; kill -0 answers ESRCH), or
//   - its lease expired: renewed + ttl is in the past (the holder renews every
//     ttl/3 while it runs, so an expired lease means a hung or reused pid), or
//   - owner.json is missing or unreadable and the directory is older than
//     ownerGrace (the holder died between mkdir and writing it).
//
// A live holder is NEVER removed or signalled, by the panel or by a waiter.
//
// Waiters queue FIFO by arrival (the waiter file's name). Only the first live waiter tries mkdir,
// so the queue is fair. A waiter whose pid is gone, or whose file has not been
// renewed for waiterTTL, is skipped and its file removed.

// LeaseOwner is the content of owner.json and of each waiter file.
type LeaseOwner struct {
	V       int    `json:"v"`
	Nonce   string `json:"nonce"`
	PID     int    `json:"pid"`
	Host    string `json:"host"`
	Lane    string `json:"lane,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	Started int64  `json:"started"` // unix seconds
	Renewed int64  `json:"renewed"` // unix seconds
	TTL     int64  `json:"ttl"`     // seconds; 0 = no expiry (pid liveness only)
}

const (
	ownerFileName = "owner.json"
	ownerGrace    = 10 * time.Second
	reclaimStale  = 30 * time.Second
	waiterTTL     = 60 * time.Second
	waiterRenew   = 10 * time.Second
	// DefaultLeaseTTL is lock-run's default lease: renewed every ttl/3.
	DefaultLeaseTTL = 10 * time.Minute
	// ExitCancelled is lock-run's exit code when its wait is cancelled (EX_TEMPFAIL).
	ExitCancelled = 75
)

var nonceRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func waitersDir(lock string) string { return lock + ".waiters" }
func reclaimDir(lock string) string { return lock + ".reclaim" }

func newNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// PIDAlive reports whether a process exists: kill(pid, 0) succeeds, or fails with
// EPERM (it exists but belongs to someone else).
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func hostName() string {
	h, _ := os.Hostname()
	return h
}

// LeaseStale reports whether a holder (or waiter) may be removed, and why. A pid on
// another host cannot be checked, so only the TTL applies to it.
func LeaseStale(o LeaseOwner, host string, now time.Time, alive func(int) bool) (bool, string) {
	if o.Host == host && o.PID > 0 && !alive(o.PID) {
		return true, fmt.Sprintf("pid %d is gone", o.PID)
	}
	if o.TTL > 0 && now.Unix() > o.Renewed+o.TTL {
		return true, fmt.Sprintf("its lease expired %ds ago (not renewed)", now.Unix()-o.Renewed-o.TTL)
	}
	return false, ""
}

func readLeaseFile(path string) (LeaseOwner, error) {
	var o LeaseOwner
	b, err := os.ReadFile(path)
	if err != nil {
		return o, err
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return o, fmt.Errorf("%s: %w", path, err)
	}
	return o, nil
}

// writeLeaseFile writes atomically (temp + rename in the same directory), so a
// reader never sees half a file.
func writeLeaseFile(path string, o LeaseOwner) error {
	b, _ := json.Marshal(o)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lease-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// holderState reads the lock directory: held?, the owner if readable, and whether
// the holder is stale (and why).
func holderState(lock, host string, now time.Time, alive func(int) bool) (held bool, o LeaseOwner, stale bool, why string) {
	fi, err := os.Stat(lock)
	if err != nil || !fi.IsDir() {
		return false, o, false, ""
	}
	o, err = readLeaseFile(filepath.Join(lock, ownerFileName))
	if err != nil {
		if now.Sub(fi.ModTime()) >= ownerGrace {
			return true, o, true, "it has no readable owner.json after " + ownerGrace.String()
		}
		return true, o, false, "its holder is starting"
	}
	stale, why = LeaseStale(o, host, now, alive)
	return true, o, stale, why
}

// waiterEntry is one waiter file.
type waiterEntry struct {
	LeaseOwner
	file      string
	cancelled bool
}

// listWaiters returns the waiters in queue order. Dead ones are returned separately
// so the caller can remove them.
func listWaiters(lock, host string, now time.Time, alive func(int) bool) (live, dead []waiterEntry) {
	dir := waitersDir(lock)
	ents, _ := os.ReadDir(dir)
	cancels := map[string]bool{}
	for _, e := range ents {
		if n, ok := strings.CutSuffix(e.Name(), ".cancel"); ok {
			cancels[n] = true
		}
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		o, err := readLeaseFile(p)
		if err != nil || !nonceRe.MatchString(o.Nonce) {
			continue // being written, or not ours to judge
		}
		w := waiterEntry{LeaseOwner: o, file: p, cancelled: cancels[o.Nonce]}
		wo := o
		wo.TTL = int64(waiterTTL / time.Second)
		if stale, _ := LeaseStale(wo, host, now, alive); stale {
			dead = append(dead, w)
			continue
		}
		live = append(live, w)
	}
	// A waiter file is named <arrival in unix ns>-<nonce>.json. The arrival is compared
	// as a number, so a writer that does not zero-pad it still queues in order.
	sort.Slice(live, func(i, j int) bool {
		ai, bi := arrival(live[i].file), arrival(live[j].file)
		if ai != bi {
			return ai < bi
		}
		return filepath.Base(live[i].file) < filepath.Base(live[j].file)
	})
	return live, dead
}

// arrival reads the arrival time from a waiter file name; an unparsable name sorts
// last.
func arrival(file string) int64 {
	head, _, _ := strings.Cut(filepath.Base(file), "-")
	n, err := strconv.ParseInt(head, 10, 64)
	if err != nil {
		return math.MaxInt64
	}
	return n
}

// reclaim removes a stale holder. It takes the reclaim mutex, then re-reads the
// holder and removes it only if it is still the SAME stale holder it judged (same
// nonce, still stale): between the judgement and the removal another waiter may
// already have reclaimed it and acquired the lock afresh.
func reclaim(lock string, judged LeaseOwner, host string, now time.Time, alive func(int) bool) (bool, error) {
	rd := reclaimDir(lock)
	if err := os.Mkdir(rd, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			// A reclaimer that died leaves the mutex behind; it is only ever held for
			// a few file operations, so an old one is dead.
			if fi, serr := os.Stat(rd); serr == nil && now.Sub(fi.ModTime()) >= reclaimStale {
				_ = os.Remove(rd)
			}
			return false, nil
		}
		return false, err
	}
	defer os.Remove(rd)
	held, cur, stale, _ := holderState(lock, host, now, alive)
	if !held || !stale || cur.Nonce != judged.Nonce {
		return false, nil
	}
	if err := os.RemoveAll(lock); err != nil {
		return false, err
	}
	return true, nil
}

// LockRunOptions configures one lock-run.
type LockRunOptions struct {
	Lock   string        // the lease directory
	Lane   string        // shown to others; default $CLAUDUCTOR_LANE
	TTL    time.Duration // the lease; default DefaultLeaseTTL
	Poll   time.Duration // how often a waiter looks; default 500 ms
	Argv   []string      // the command to run while holding the lease
	Stderr io.Writer
	Stdout io.Writer
	Stdin  io.Reader
	// Alive overrides the pid liveness check (tests).
	Alive func(int) bool
}

// LockRun waits its turn for the lease, runs Argv while holding it, and releases
// it. It returns the command's exit code: 128+n when a signal ended it, 75 when the
// wait was cancelled (from the panel), 130 when lock-run itself was interrupted
// while waiting. It never signals or removes a live holder.
func LockRun(ctx context.Context, o LockRunOptions) (int, error) {
	if len(o.Argv) == 0 {
		return 2, errors.New("lock-run: no command after --")
	}
	if o.TTL <= 0 {
		o.TTL = DefaultLeaseTTL
	}
	if o.Poll <= 0 {
		o.Poll = 500 * time.Millisecond
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Alive == nil {
		o.Alive = PIDAlive
	}
	if o.Lane == "" {
		o.Lane = os.Getenv("CLAUDUCTOR_LANE")
	}
	lock, err := filepath.Abs(o.Lock)
	if err != nil {
		return 2, err
	}
	lock = filepath.Clean(lock)
	// Re-entry: a gate script that re-runs itself through lock-run already holds it.
	if os.Getenv("CLAUDUCTOR_LOCK_HELD") == lock {
		return runChild(ctx, o, lock, nil)
	}
	if err := os.MkdirAll(waitersDir(lock), 0o755); err != nil {
		return 2, fmt.Errorf("lock-run: %w", err)
	}
	host := hostName()
	now := time.Now()
	me := LeaseOwner{V: 1, Nonce: newNonce(), PID: os.Getpid(), Host: host, Lane: o.Lane,
		Cmd: clip(strings.Join(o.Argv, " "), 200), Started: now.Unix(), Renewed: now.Unix(), TTL: int64(o.TTL / time.Second)}
	myWait := filepath.Join(waitersDir(lock), fmt.Sprintf("%020d-%s.json", now.UnixNano(), me.Nonce))
	if err := writeLeaseFile(myWait, me); err != nil {
		return 2, fmt.Errorf("lock-run: %w", err)
	}
	cancelFile := filepath.Join(waitersDir(lock), me.Nonce+".cancel")
	leaveQueue := func() { _ = os.Remove(myWait); _ = os.Remove(cancelFile) }

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	lastRenew, lastSay := now, ""
	say := func(s string) {
		if s != lastSay {
			fmt.Fprintln(o.Stderr, "lock-run: "+s)
			lastSay = s
		}
	}
	for {
		now = time.Now()
		if _, err := os.Stat(cancelFile); err == nil {
			leaveQueue()
			say("wait cancelled from the panel; not running " + me.Cmd)
			return ExitCancelled, nil
		}
		if now.Sub(lastRenew) >= waiterRenew {
			me.Renewed = now.Unix()
			_ = writeLeaseFile(myWait, me)
			lastRenew = now
		}
		live, dead := listWaiters(lock, host, now, o.Alive)
		for _, d := range dead {
			_ = os.Remove(d.file)
		}
		ahead := 0
		for _, w := range live {
			if w.Nonce == me.Nonce {
				break
			}
			ahead++
		}
		if ahead == 0 {
			if err := os.Mkdir(lock, 0o755); err == nil {
				held := me
				held.Started, held.Renewed = now.Unix(), now.Unix()
				if err := writeLeaseFile(filepath.Join(lock, ownerFileName), held); err != nil {
					_ = os.RemoveAll(lock)
					leaveQueue()
					return 2, fmt.Errorf("lock-run: writing owner.json: %w", err)
				}
				leaveQueue()
				return runChild(ctx, o, lock, &held)
			} else if !errors.Is(err, os.ErrExist) {
				leaveQueue()
				return 2, fmt.Errorf("lock-run: %w", err)
			}
			held, h, stale, why := holderState(lock, host, now, o.Alive)
			switch {
			case held && stale:
				say(fmt.Sprintf("reclaiming %s from %s: %s", lock, who(h), why))
				ok, err := reclaim(lock, h, host, now, o.Alive)
				if err != nil {
					say("reclaim failed: " + err.Error())
				}
				if ok {
					continue
				}
			case held:
				say(fmt.Sprintf("waiting for %s (held by %s since %s)", filepath.Base(lock), who(h), time.Unix(h.Started, 0).Format("15:04:05")))
			}
		} else {
			say(fmt.Sprintf("waiting for %s: %d ahead in the queue", filepath.Base(lock), ahead))
		}
		select {
		case <-ctx.Done():
			leaveQueue()
			return 130, nil
		case <-sigs:
			leaveQueue()
			return 130, nil
		case <-time.After(o.Poll):
		}
	}
}

func who(o LeaseOwner) string {
	if o.PID == 0 {
		return "an unknown holder"
	}
	if o.Lane != "" {
		return fmt.Sprintf("lane %s (pid %d)", o.Lane, o.PID)
	}
	return fmt.Sprintf("pid %d", o.PID)
}

// runChild runs the command. When held is set it holds the lease: it renews it every
// ttl/3 and releases it after the command exits, but only while owner.json still
// carries its own nonce (a lease it lost to a reclaim is not its to remove).
func runChild(ctx context.Context, o LockRunOptions, lock string, held *LeaseOwner) (int, error) {
	cmd := exec.Command(o.Argv[0], o.Argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Env = append(os.Environ(), "CLAUDUCTOR_LOCK_HELD="+lock)
	release := func() {
		if held == nil {
			return
		}
		ownerPath := filepath.Join(lock, ownerFileName)
		if cur, err := readLeaseFile(ownerPath); err == nil && cur.Nonce == held.Nonce {
			_ = os.Remove(ownerPath)
			_ = os.RemoveAll(lock)
		}
	}
	if err := cmd.Start(); err != nil {
		release()
		return 127, fmt.Errorf("lock-run: %w", err)
	}
	stop := make(chan struct{})
	defer close(stop)
	if held != nil {
		go func() {
			t := time.NewTicker(o.TTL / 3)
			defer t.Stop()
			ownerPath := filepath.Join(lock, ownerFileName)
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					cur, err := readLeaseFile(ownerPath)
					if err != nil || cur.Nonce != held.Nonce {
						fmt.Fprintln(o.Stderr, "lock-run: the lease was lost (reclaimed as stale); the command runs on unprotected")
						return
					}
					cur.Renewed = time.Now().Unix()
					_ = writeLeaseFile(ownerPath, cur)
				}
			}
		}()
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	for waiting := true; waiting; {
		select {
		case s := <-sigs:
			_ = cmd.Process.Signal(s) // forward: the command decides how to stop
		case <-ctx.Done():
			_ = cmd.Process.Signal(syscall.SIGTERM)
			ctx = context.Background()
		case werr = <-done:
			waiting = false
		}
	}
	release()
	if werr == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(werr, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return ee.ExitCode(), nil
	}
	return 1, werr
}

// ---- the panel's read-only view of a queue ----

// LeaseView is a holder or a waiter as the page shows it.
type LeaseView struct {
	Nonce      string `json:"nonce"`
	PID        int    `json:"pid"`
	Lane       string `json:"lane,omitempty"`
	Cmd        string `json:"cmd,omitempty"`
	Started    int64  `json:"started"` // unix ms
	Renewed    int64  `json:"renewed"` // unix ms
	TTL        int64  `json:"ttl"`
	Alive      bool   `json:"alive"`
	Stale      bool   `json:"stale"`
	StaleWhy   string `json:"staleWhy,omitempty"`
	Cancelling bool   `json:"cancelling,omitempty"`
}

// QueueView is one queue.
type QueueView struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Lock       string      `json:"lock"`
	HasCommand bool        `json:"hasCommand"`
	Held       bool        `json:"held"`
	Holder     *LeaseView  `json:"holder,omitempty"`
	HolderNote string      `json:"holderNote,omitempty"`
	Waiters    []LeaseView `json:"waiters"`
	Error      string      `json:"error,omitempty"`
	Run        *QueueRun   `json:"run,omitempty"` // the last RUN the panel started
}

func leaseView(o LeaseOwner, host string, now time.Time, alive func(int) bool) LeaseView {
	stale, why := LeaseStale(o, host, now, alive)
	return LeaseView{Nonce: o.Nonce, PID: o.PID, Lane: o.Lane, Cmd: o.Cmd, Started: o.Started * 1000, Renewed: o.Renewed * 1000,
		TTL: o.TTL, Alive: o.Host != host || alive(o.PID), Stale: stale, StaleWhy: why}
}

// ReadQueue reads one queue's lease and waiters. It only reads: removing stale
// entries is the waiters' job, and the panel never touches a holder.
func ReadQueue(q QueueConfig, lock string, now time.Time, alive func(int) bool) QueueView {
	host := hostName()
	v := QueueView{ID: q.ID, Title: q.Title, Lock: lock, HasCommand: len(q.Command) > 0, Waiters: []LeaseView{}}
	held, o, stale, why := holderState(lock, host, now, alive)
	v.Held = held
	if held {
		if o.Nonce != "" {
			lv := leaseView(o, host, now, alive)
			v.Holder = &lv
		}
		if stale {
			v.HolderNote = "stale: " + why + "; the next waiter reclaims it"
		} else if o.Nonce == "" {
			v.HolderNote = why
		}
	}
	live, _ := listWaiters(lock, host, now, alive)
	for _, w := range live {
		lv := leaseView(w.LeaseOwner, host, now, alive)
		lv.Stale, lv.StaleWhy = false, ""
		lv.Cancelling = w.cancelled
		v.Waiters = append(v.Waiters, lv)
	}
	return v
}

// CancelWait asks one waiter to give up, by writing its cancel file. It refuses the
// holder: the panel never stops a command that holds the lease.
func CancelWait(lock, nonce string) error {
	if !nonceRe.MatchString(nonce) {
		return errors.New("invalid waiter id")
	}
	if o, err := readLeaseFile(filepath.Join(lock, ownerFileName)); err == nil && o.Nonce == nonce {
		return errors.New("that is the holder, not a waiter; the panel never stops the holder")
	}
	host := hostName()
	live, _ := listWaiters(lock, host, time.Now(), PIDAlive)
	for _, w := range live {
		if w.Nonce == nonce {
			f, err := os.OpenFile(filepath.Join(waitersDir(lock), nonce+".cancel"), os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			return f.Close()
		}
	}
	return errors.New("no such waiter (it may have started or left already)")
}
