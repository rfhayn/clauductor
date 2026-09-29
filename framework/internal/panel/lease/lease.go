// Package lease is the on-disk queue lease (a shared gate, say) that `clauductor
// lock-run` holds and the panel only reads. Of the panel it imports only clock and
// types, which have no dependencies, so lock-run links nothing else of the panel.
package lease

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
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// The lease protocol serialises a shared resource (a full test gate that binds a
// fixed port, say) between processes that know nothing of each other or of the panel. It lives
// entirely on disk, so it survives a panel restart, and a POSIX shell can honour it
// without the panel running. macOS has no flock(1), so the lock is a DIRECTORY,
// because mkdir(2) is atomic everywhere:
//
//	<lock>/              held while it exists; mkdir creates it or fails with EEXIST
//	<lock>/owner.json    the holder: {v, nonce, pid, pstart, host, lane, cmd, started, renewed, ttl}
//	<lock>.waiters/      one <started>-<nonce>.json per waiter, same fields
//	<lock>.waiters/<nonce>.cancel   asks that waiter to give up (the panel's CANCEL)
//	<lock>.reclaim/      a short mutex taken only to remove a stale holder
//
// pstart is the holder's process start time as `LC_ALL=C ps -o lstart= -p <pid>`
// prints it, whitespace collapsed, recorded at acquire. It is what tells a live
// holder from a reused pid, in Go and in plain shell alike.
//
// A holder on THIS host is judged by its process, never by the clock:
//   - its pid is gone (kill -0 answers ESRCH): stale;
//   - its pid is alive and its start time matches pstart: LIVE, however long it has
//     been silent. A stopped (SIGSTOP) or sleeping holder is still the holder;
//     expiring it would run two gates at once;
//   - its pid is alive with another start time: the pid was reused, stale.
//
//   - its pid is alive and its start time cannot be verified (no pstart recorded,
//     none readable, or the two from different sources, ps and /proc): LIVE.
//
// The TTL (renewed + ttl in the past; ttl 0 never expires) applies only to a record
// whose processes mean nothing here: one from another host, or with no pid.
//
// A record is VALID when checkRecord accepts it: one flat JSON object of strings,
// integers, true, false or null, its fields of their types, and a nonce of 16
// lower-case hex digits. lease.sh's lease_valid checks exactly the same.
//   - owner.json missing or invalid, in a lock directory older than ownerGrace (the
//     holder died between mkdir and a complete write), is stale; younger, its holder
//     is starting, and waiters wait.
//   - an invalid waiter file is not a waiter: it holds no place in the queue, and
//     nobody removes it (it is not ours to judge).
//
// A live holder is NEVER removed or signalled, by the panel or by a waiter.
// Liveness is decided ONLY by the holder's and its command's pid and start time, the
// same rule in Go and in shell, so the two sides always agree. lock-run also holds
// flock(2) on the lease directory while it runs; that never makes a record live (an
// orphan that inherited the fd would outlive the gate), and the panel only shows it.
//
// Waiters queue FIFO by arrival (the waiter file's name). Only the first live waiter
// tries mkdir, so the queue is fair. A waiter is judged by the holder's rule with a
// TTL of waiterTTL: on this host by its process, elsewhere by its renewals. A dead
// one is skipped and its file removed.

// LeaseOwner is the content of owner.json and of each waiter file.
type LeaseOwner struct {
	V      int    `json:"v"`
	Nonce  string `json:"nonce"`
	PID    int    `json:"pid"`
	PStart string `json:"pstart,omitempty"` // process start time (see above)
	// ChildPID and ChildPStart name the command lock-run runs. The record is live
	// while EITHER the holder or its command is: a lock-run killed with SIGKILL
	// leaves its gate running, and the gate still holds the lease.
	ChildPID    int    `json:"child_pid,omitempty"`
	ChildPStart string `json:"child_pstart,omitempty"`
	Host        string `json:"host"`
	Lane        string `json:"lane,omitempty"`
	Cmd         string `json:"cmd,omitempty"`
	Started     int64  `json:"started"` // unix seconds
	Renewed     int64  `json:"renewed"` // unix seconds
	TTL         int64  `json:"ttl"`     // seconds; 0 = no expiry (pid liveness only)
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
	// ExitLeaseLost is lock-run's exit code when it finds its lease taken away while
	// its command ran; it stops the command first (EX_SOFTWARE).
	ExitLeaseLost = 70
)

// ProcCheck reports whether a pid is alive and, if it can tell, its start time.
type ProcCheck func(pid int) (alive bool, start string)

// ProcStart is the start time of a process as the protocol records it:
// `LC_ALL=C ps -o lstart= -p <pid>` with whitespace collapsed; where there is no
// ps, "proc:" + field 22 (starttime) of /proc/<pid>/stat. "" if unknown. Values
// from the two sources are never compared with each other.
func ProcStart(pid int) string {
	if pid <= 0 {
		return ""
	}
	cmd := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if out, err := cmd.Output(); err == nil {
		if v := strings.Join(strings.Fields(string(out)), " "); v != "" {
			return v
		}
	}
	return procStatStart(pid)
}

// procStatStart reads field 22 of /proc/<pid>/stat. The command name (field 2) may
// hold spaces and parentheses, so fields are counted after its last ")".
func procStatStart(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	s := string(b)
	i := strings.LastIndex(s, ") ")
	if i < 0 {
		return ""
	}
	f := strings.Fields(s[i+2:])
	if len(f) < 20 {
		return ""
	}
	return "proc:" + f[19]
}

// SameSource: two start times come from the same source (ps, or /proc).
func SameSource(a, b string) bool {
	return strings.HasPrefix(a, "proc:") == strings.HasPrefix(b, "proc:")
}

// LiveProc is the real ProcCheck.
func LiveProc(pid int) (bool, string) {
	if !pidAlive(pid) {
		return false, ""
	}
	return true, ProcStart(pid)
}

// ProcCache is a ProcCheck that reads each pid's start time once (PANEL-7). A
// process's start time never changes, so while kill(pid, 0) keeps answering, the
// pid is the process it was: liveness is rechecked with kill every time, and ps
// runs again only for a pid that is new, or was seen gone since (a pid is reused
// only after its process is gone), and every Recheck anyway: a pid that died and
// was reused between two checks would otherwise keep the old start time for good.
// The panel's queue view uses it: the view only reads, and lock-run's waiters,
// which reclaim, judge with LiveProc every time.
type ProcCache struct {
	Alive   func(pid int) bool   // default PIDAlive
	Start   func(pid int) string // default ProcStart
	Now     func() time.Time     // default clock.System.Now (the panel passes its clock's)
	TTL     time.Duration        // an entry unused this long is dropped; default 1 min
	Recheck time.Duration        // a cached start time is read again this often; default 30 s

	mu sync.Mutex
	m  map[int]procCacheItem
}

type procCacheItem struct {
	start string
	read  time.Time
	used  time.Time
}

// Check is the ProcCheck.
func (c *ProcCache) Check(pid int) (bool, string) {
	alive, start, now, ttl, recheck := c.Alive, c.Start, c.Now, c.TTL, c.Recheck
	if alive == nil {
		alive = pidAlive
	}
	if start == nil {
		start = ProcStart
	}
	if now == nil {
		now = clock.System.Now
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	if recheck <= 0 {
		recheck = 30 * time.Second
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[int]procCacheItem{}
	}
	t := now()
	for p, it := range c.m {
		if t.Sub(it.used) > ttl {
			delete(c.m, p)
		}
	}
	if !alive(pid) {
		delete(c.m, pid)
		return false, ""
	}
	it, ok := c.m[pid]
	if !ok || t.Sub(it.read) >= recheck {
		it.start, it.read = start(pid), t
	}
	it.used = t
	c.m[pid] = it
	return true, it.start
}

var nonceRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func waitersDir(lock string) string { return lock + ".waiters" }
func reclaimDir(lock string) string { return lock + ".reclaim" }

func newNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// pidAlive reports whether a process exists: kill(pid, 0) succeeds, or fails with
// EPERM (it exists but belongs to someone else).
func pidAlive(pid int) bool {
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

// procDead judges one recorded process on this host: dead when its pid is gone, or
// when its start time differs from the recorded one (from the same source): the pid
// was reused. An alive pid whose start time cannot be verified is LIVE: missing data
// never deletes somebody else's lease or waiter file.
func procDead(pid int, recorded string, proc ProcCheck) (bool, string) {
	alive, start := proc(pid)
	switch {
	case !alive:
		return true, fmt.Sprintf("pid %d is gone", pid)
	case recorded != "" && start != "" && SameSource(recorded, start) && start != recorded:
		return true, fmt.Sprintf("pid %d was reused (started %s, the record says %s)", pid, start, recorded)
	}
	return false, ""
}

// leaseStale reports whether a holder (or waiter) may be removed, and why. On this
// host it is judged by its processes, never by the clock: stale only when the holder
// AND its recorded command are both dead (or reused pids). The TTL applies only to a
// record from another host, whose pids mean nothing here.
func leaseStale(o LeaseOwner, host string, now time.Time, proc ProcCheck) (bool, string) {
	if o.Host == host && o.PID > 0 {
		dead, why := procDead(o.PID, o.PStart, proc)
		if !dead {
			return false, ""
		}
		if o.ChildPID > 0 {
			if cdead, cwhy := procDead(o.ChildPID, o.ChildPStart, proc); !cdead {
				return false, ""
			} else {
				why += "; its command " + cwhy
			}
		}
		return true, why
	}
	if o.TTL > 0 && now.Unix() > o.Renewed+o.TTL {
		return true, fmt.Sprintf("its lease from host %q expired %ds ago (not renewed)", o.Host, now.Unix()-o.Renewed-o.TTL)
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
	if err := checkRecord(b); err != nil {
		return LeaseOwner{}, fmt.Errorf("%s: %w", path, err)
	}
	return o, nil
}

var (
	jsonIntRe     = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	recordIntKeys = map[string]bool{"v": true, "pid": true, "child_pid": true, "started": true, "renewed": true, "ttl": true}
	recordStrKeys = map[string]bool{"nonce": true, "pstart": true, "child_pstart": true, "host": true, "lane": true, "cmd": true}
)

// checkRecord is the protocol's validity rule, the one lease.sh's lease_valid
// applies too: one flat JSON object whose values are strings, integers, true, false
// or null (no nested value, no fraction or exponent); the integer fields integers
// and the string fields strings; and a nonce of 16 lower-case hex digits.
func checkRecord(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for k, raw := range m {
		v := string(raw)
		isStr := strings.HasPrefix(v, `"`)
		if !isStr && !jsonIntRe.MatchString(v) && v != "true" && v != "false" && v != "null" {
			return fmt.Errorf("%s is not a string, an integer, true, false or null", k)
		}
		if recordIntKeys[k] && !jsonIntRe.MatchString(v) {
			return fmt.Errorf("%s is not an integer", k)
		}
		if recordStrKeys[k] && !isStr {
			return fmt.Errorf("%s is not a string", k)
		}
	}
	var n string
	if err := json.Unmarshal(m["nonce"], &n); err != nil || !nonceRe.MatchString(n) {
		return errors.New("no nonce of 16 lower-case hex digits")
	}
	return nil
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
func holderState(lock, host string, now time.Time, proc ProcCheck) (held bool, o LeaseOwner, stale bool, why string) {
	fi, err := os.Stat(lock)
	if err != nil || !fi.IsDir() {
		return false, o, false, ""
	}
	o, err = readLeaseFile(filepath.Join(lock, ownerFileName))
	if err == nil && !nonceRe.MatchString(o.Nonce) {
		err = errors.New("no valid nonce") // an invalid record is as good as none
	}
	if err != nil {
		o = LeaseOwner{}
		if now.Sub(fi.ModTime()) >= ownerGrace {
			return true, o, true, "it has no valid owner.json after " + ownerGrace.String()
		}
		return true, o, false, "its holder is starting"
	}
	stale, why = leaseStale(o, host, now, proc)
	return true, o, stale, why
}

// flocked reports whether another open file description holds flock(2) on the
// lease directory. Diagnostic only: it never decides liveness.
func flocked(lock string) bool {
	f, err := os.Open(lock)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.Is(err, syscall.EWOULDBLOCK)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

// holdFlock takes flock(2) on the lease directory and keeps it until release. It
// is not passed to the command: a daemon the gate leaves behind must not hold it.
func holdFlock(lock string) (*os.File, func()) {
	f, err := os.Open(lock)
	if err != nil {
		return nil, func() {}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, func() {}
	}
	return f, func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }
}

// waiterEntry is one waiter file.
type waiterEntry struct {
	LeaseOwner
	file      string
	cancelled bool
}

// listWaiters returns the waiters in queue order. Dead ones are returned separately
// so the caller can remove them.
func listWaiters(lock, host string, now time.Time, proc ProcCheck) (live, dead []waiterEntry) {
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
		if stale, _ := leaseStale(wo, host, now, proc); stale {
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
func reclaim(lock string, judged LeaseOwner, host string, now time.Time, proc ProcCheck) (bool, error) {
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
	held, cur, stale, _ := holderState(lock, host, now, proc)
	if !held || !stale || cur.Nonce != judged.Nonce {
		return false, nil
	}
	if err := os.RemoveAll(lock); err != nil {
		return false, err
	}
	return true, nil
}

// afterAcquire, when a test sets it, runs once lock-run holds the lease and before
// it starts the command: the window a signal must not be lost in.
var afterAcquire func()

// errReclaimBusy: the reclaim mutex stayed taken; the caller tries again later.
var errReclaimBusy = errors.New("the reclaim mutex is taken")

// updateOwner rewrites owner.json, only while it still carries nonce, under the
// reclaim mutex. A reclaimer holds that mutex from its judgement to its removal, so
// between this read and this write the record cannot be reclaimed and replaced by a
// new holder's: a plain read, then rename, could put this holder's record over the
// next one's. The write itself is atomic (temp file + rename). It reports whether
// the record was still ours.
func updateOwner(lock, nonce string, clk clock.Clock, mutate func(*LeaseOwner)) (bool, error) {
	rd := reclaimDir(lock)
	for tries := 0; ; tries++ {
		err := os.Mkdir(rd, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		// A reclaimer that died leaves the mutex behind; it is only ever held for a
		// few file operations, so an old one is dead.
		if fi, serr := os.Stat(rd); serr == nil && clk.Now().Sub(fi.ModTime()) >= reclaimStale {
			_ = os.Remove(rd)
			continue
		}
		if tries >= 20 {
			return true, errReclaimBusy
		}
		<-clk.After(10 * time.Millisecond)
	}
	defer os.Remove(rd)
	ownerPath := filepath.Join(lock, ownerFileName)
	cur, err := readLeaseFile(ownerPath)
	if err != nil || cur.Nonce != nonce {
		return false, nil
	}
	mutate(&cur)
	return true, writeLeaseFile(ownerPath, cur)
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
	// Proc overrides the process check (tests).
	Proc ProcCheck
	// Clock is the time lock-run waits, renews and stamps by. Required.
	Clock clock.Clock
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
	if o.Proc == nil {
		o.Proc = LiveProc
	}
	if o.Clock == nil {
		return 2, errors.New("lock-run: no clock")
	}
	if o.Lane == "" {
		o.Lane = os.Getenv("CLAUDUCTOR_LANE")
	}
	lock, err := filepath.Abs(o.Lock)
	if err != nil {
		return 2, err
	}
	lock = filepath.Clean(lock)
	// One channel for INT, TERM and HUP, from here to the exit. While lock-run waits,
	// a signal ends the wait; once it holds the lease, one is passed on to the
	// command, including one that arrived between winning the lease and starting it:
	// it waits in the channel until the command runs.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	// Re-entry: a gate script that re-runs itself through lock-run already holds it.
	if os.Getenv("CLAUDUCTOR_LOCK_HELD") == lock {
		return runChild(ctx, o, lock, nil, sigs)
	}
	if err := os.MkdirAll(waitersDir(lock), 0o755); err != nil {
		return 2, fmt.Errorf("lock-run: %w", err)
	}
	host := hostName()
	now := o.Clock.Now()
	me := LeaseOwner{V: 1, Nonce: newNonce(), PID: os.Getpid(), PStart: ProcStart(os.Getpid()), Host: host, Lane: o.Lane,
		Cmd: clip(strings.Join(o.Argv, " "), 200), Started: now.Unix(), Renewed: now.Unix(), TTL: int64(o.TTL / time.Second)}
	myWait := filepath.Join(waitersDir(lock), fmt.Sprintf("%020d-%s.json", now.UnixNano(), me.Nonce))
	if err := writeLeaseFile(myWait, me); err != nil {
		return 2, fmt.Errorf("lock-run: %w", err)
	}
	cancelFile := filepath.Join(waitersDir(lock), me.Nonce+".cancel")
	leaveQueue := func() { _ = os.Remove(myWait); _ = os.Remove(cancelFile) }

	lastRenew, lastSay := now, ""
	say := func(s string) {
		if s != lastSay {
			fmt.Fprintln(o.Stderr, "lock-run: "+s)
			lastSay = s
		}
	}
	for {
		now = o.Clock.Now()
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
		live, dead := listWaiters(lock, host, now, o.Proc)
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
				_, unflock := holdFlock(lock)
				defer unflock()
				held := me
				held.Started, held.Renewed = now.Unix(), now.Unix()
				if err := writeLeaseFile(filepath.Join(lock, ownerFileName), held); err != nil {
					_ = os.RemoveAll(lock)
					leaveQueue()
					return 2, fmt.Errorf("lock-run: writing owner.json: %w", err)
				}
				leaveQueue()
				if afterAcquire != nil {
					afterAcquire()
				}
				return runChild(ctx, o, lock, &held, sigs)
			} else if !errors.Is(err, os.ErrExist) {
				leaveQueue()
				return 2, fmt.Errorf("lock-run: %w", err)
			}
			held, h, stale, why := holderState(lock, host, now, o.Proc)
			switch {
			case held && stale:
				say(fmt.Sprintf("reclaiming %s from %s: %s", lock, who(h), why))
				ok, err := reclaim(lock, h, host, now, o.Proc)
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
		case <-o.Clock.After(o.Poll):
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
func runChild(ctx context.Context, o LockRunOptions, lock string, held *LeaseOwner, sigs <-chan os.Signal) (int, error) {
	cmd := exec.Command(o.Argv[0], o.Argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Env = append(childEnv(os.Environ()), "CLAUDUCTOR_LOCK_HELD="+lock)
	// The command gets a process group of its own. A Ctrl-C from the terminal then
	// reaches lock-run only, which passes it on ONCE; and lock-run can stop the whole
	// gate (its dev server and test runners too) if the lease is lost. The cost: the
	// command cannot read the terminal, which a gate never needs.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// On a terminal, the command's group must be the terminal's FOREGROUND group:
	// a background group that touches the terminal (stty, a prompt) is stopped with
	// SIGTTOU/SIGTTIN, forever, while it holds the lease. lock-run takes the
	// terminal back when the command ends. Only when lock-run is itself in the
	// foreground: a lock-run started with & must not steal the terminal.
	ttyFd := -1
	var saved *syscall.Termios
	if f, ok := cmd.Stdin.(*os.File); ok {
		if pg, err := tcgetpgrp(int(f.Fd())); err == nil && pg == syscall.Getpgrp() {
			ttyFd = int(f.Fd())
			cmd.SysProcAttr.Foreground, cmd.SysProcAttr.Ctty = true, 0 // fd 0 in the child
			// The command may leave the terminal raw or without echo (stty -echo, a
			// killed prompt); its settings are put back when lock-run takes it back.
			saved, _ = getTermios(ttyFd)
		}
	}
	takeTerminalBack := func() {
		if ttyFd >= 0 {
			signal.Ignore(syscall.SIGTTOU) // lock-run is in the background until this returns
			_ = tcsetpgrp(ttyFd, syscall.Getpgrp())
			if saved != nil {
				_ = setTermios(ttyFd, saved)
			}
		}
	}
	defer takeTerminalBack()
	group := func(sig syscall.Signal) { _ = syscall.Kill(-cmd.Process.Pid, sig) }
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
	// INT, TERM and HUP to lock-run go on to the command's group, once each, from the
	// channel LockRun has listened on since before it took the lease: one that
	// arrived before the start is passed on as soon as the command runs.
	if err := cmd.Start(); err != nil {
		release()
		return 127, fmt.Errorf("lock-run: %w", err)
	}
	if held != nil {
		// Record the command beside the holder: the lease stays live while either runs.
		childPID, childStart := cmd.Process.Pid, ProcStart(cmd.Process.Pid)
		_, _ = updateOwner(lock, held.Nonce, o.Clock, func(cur *LeaseOwner) { cur.ChildPID, cur.ChildPStart = childPID, childStart })
	}
	stop := make(chan struct{})
	defer close(stop)
	lost := make(chan struct{})
	if held != nil {
		// Once a second, check the lease is still ours; renew it every ttl/3 (only a
		// reader that cannot check this process uses the TTL).
		go func() {
			t := o.Clock.NewTicker(time.Second)
			defer t.Stop()
			ownerPath := filepath.Join(lock, ownerFileName)
			lastRenew := o.Clock.Now()
			for {
				select {
				case <-stop:
					return
				case <-t.C():
					cur, err := readLeaseFile(ownerPath)
					if err != nil || cur.Nonce != held.Nonce {
						close(lost)
						return
					}
					if now := o.Clock.Now(); now.Sub(lastRenew) >= o.TTL/3 {
						ours, err := updateOwner(lock, held.Nonce, o.Clock, func(cur *LeaseOwner) { cur.Renewed = now.Unix() })
						if !ours {
							close(lost)
							return
						}
						if err == nil {
							lastRenew = now // a busy mutex is tried again next tick
						}
					}
				}
			}
		}()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	for waiting := true; waiting; {
		select {
		case s := <-sigs:
			if sig, ok := s.(syscall.Signal); ok {
				group(sig)
			}
		case <-ctx.Done():
			group(syscall.SIGTERM)
			ctx = context.Background()
		case <-lost:
			// Someone removed the lease while the command ran: two gates may now
			// run at once. Stop this one rather than finish unprotected.
			fmt.Fprintln(o.Stderr, "lock-run: the lease was taken away while the command ran; stopping it")
			group(syscall.SIGTERM)
			select {
			case <-done:
			case <-o.Clock.After(5 * time.Second):
				group(syscall.SIGKILL)
				<-done
			}
			group(syscall.SIGKILL) // anything of the gate's that ignored TERM
			return ExitLeaseLost, nil
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

func leaseView(o LeaseOwner, host string, now time.Time, proc ProcCheck) types.LeaseView {
	stale, why := leaseStale(o, host, now, proc)
	alive := o.Host != host
	if !alive {
		alive, _ = proc(o.PID)
	}
	return types.LeaseView{Nonce: o.Nonce, PID: o.PID, Lane: o.Lane, Cmd: o.Cmd, Started: o.Started * 1000, Renewed: o.Renewed * 1000,
		TTL: o.TTL, Alive: alive, Stale: stale, StaleWhy: why}
}

// ReadQueue reads one queue's lease and waiters. It only reads: removing stale
// entries is the waiters' job, and the panel never touches a holder.
func ReadQueue(q types.QueueConfig, lock string, now time.Time, proc ProcCheck) types.QueueView {
	host := hostName()
	v := types.QueueView{ID: q.ID, Title: q.Title, Lock: lock, HasCommand: len(q.Command) > 0, Waiters: []types.LeaseView{}}
	held, o, stale, why := holderState(lock, host, now, proc)
	v.Held = held
	if held {
		if o.Nonce != "" {
			lv := leaseView(o, host, now, proc)
			v.Holder = &lv
		}
		if stale {
			v.HolderNote = "stale: " + why + "; the next waiter reclaims it"
			if flocked(lock) {
				v.HolderNote += " (an orphaned process still holds its flock; that does not keep it)"
			}
		} else if o.Nonce == "" {
			v.HolderNote = why
		}
	}
	live, _ := listWaiters(lock, host, now, proc)
	for _, w := range live {
		lv := leaseView(w.LeaseOwner, host, now, proc)
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
	live, _ := listWaiters(lock, host, clock.System.Now(), LiveProc)
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

// tcgetpgrp returns the foreground process group of the terminal on fd.
func tcgetpgrp(fd int) (int, error) {
	var pg int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGPGRP), uintptr(unsafe.Pointer(&pg))); e != 0 {
		return 0, e
	}
	return int(pg), nil
}

// tcsetpgrp makes pg the foreground process group of the terminal on fd.
func tcsetpgrp(fd, pg int) error {
	p := int32(pg)
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSPGRP), uintptr(unsafe.Pointer(&p))); e != 0 {
		return e
	}
	return nil
}

// childEnv undoes the startup TERM workaround for the command. bubbletea's init (it
// is linked into this binary for the HUD) asks the terminal for its background
// colour and waits up to 5 s on a pty that does not answer. A caller skips that by
// starting lock-run with TERM=dumb, the real value in CLAUDUCTOR_TERM, and
// CLAUDUCTOR_TERM_SET=1 if TERM was set at all (docs/panel.md's snippet does). The
// command gets the real TERM back: set (even to "") if it was set, unset if not.
func childEnv(env []string) []string {
	orig, ok, wasSet := "", false, false
	for _, kv := range env {
		if v, found := strings.CutPrefix(kv, "CLAUDUCTOR_TERM="); found {
			orig, ok = v, true
		}
		if kv == "CLAUDUCTOR_TERM_SET=1" {
			wasSet = true
		}
	}
	if !ok {
		return env
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDUCTOR_TERM=") || strings.HasPrefix(kv, "CLAUDUCTOR_TERM_SET=") || strings.HasPrefix(kv, "TERM=") {
			continue
		}
		out = append(out, kv)
	}
	if wasSet || orig != "" {
		out = append(out, "TERM="+orig)
	}
	return out
}

// getTermios reads a terminal's settings.
func getTermios(fd int) (*syscall.Termios, error) {
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&t))); e != 0 {
		return nil, e
	}
	return &t, nil
}

// setTermios writes a terminal's settings back.
func setTermios(fd int, t *syscall.Termios) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(t))); e != 0 {
		return e
	}
	return nil
}
