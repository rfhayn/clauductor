// Package leakcheck is test support: it owns the throwaway tmux sockets the panel's
// tests start servers on, and checks, around a package's whole run, that the run left
// no tmux server and no helper process behind. Only tests import it.
//
// A socket is this process's when its name starts with Prefix and this process's
// pid. Main only ever kills or removes those: a suite someone else runs at the same
// time has its own pid in its names and is never touched.
package leakcheck

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/testbin"
)

// Prefix begins the name of every tmux socket a test makes.
const Prefix = "clauductor-test-"

// own is the name prefix of this process's sockets.
func own() string { return Prefix + strconv.Itoa(os.Getpid()) + "-" }

func newName(kind string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return own() + kind + hex.EncodeToString(b)
}

// SocketDir is where tmux keeps its sockets: $TMUX_TMPDIR (or /tmp)/tmux-<uid>.
func SocketDir() string {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()))
}

// TmuxSocket returns tmux and a socket of the test's own. Its cleanup, registered
// before the test can start a server on it, kills the server and removes the socket
// file, whether the test passes, fails or panics. It skips the test without tmux,
// and under -short.
func TmuxSocket(t testing.TB) (tmux, sock string) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: drives a real tmux server")
	}
	return SecurityTmuxSocket(t)
}

// SecurityTmuxSocket is TmuxSocket for a SECURITY test (a token rotation, an idle
// or untrusted close): it runs under -short too, and skips only without tmux.
func SecurityTmuxSocket(t testing.TB) (tmux, sock string) {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	sock = newName("")
	t.Cleanup(func() { kill(tmux, sock) })
	return tmux, sock
}

// NoServerSocket is a socket name of this process's that no test starts a server on,
// for a panel that runs no lane: it never reaches the machine's real panel socket.
// Main reports it if a server appears on it anyway.
func NoServerSocket() string { return newName("nosrv-") }

// kill ends sock's server, if any, and removes its file (kill-server leaves it).
func kill(tmux, sock string) {
	_ = exec.Command(tmux, "-L", sock, "kill-server").Run()
	_ = os.Remove(filepath.Join(SocketDir(), sock))
}

func live(tmux, sock string) bool {
	return exec.Command(tmux, "-L", sock, "list-sessions").Run() == nil
}

// ownSockets lists this process's socket files.
func ownSockets() []string {
	entries, _ := os.ReadDir(SocketDir())
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), own()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// Main runs the package's tests and fails the run (exit 1) if, once they are done,
// a tmux socket of this process is still there, or a helper process this run
// started (a child, the test binary run again below it, or any process whose command
// line names this run's temp directory) is still alive. It reports and removes both,
// so a failing run does not leak either, and touches no process it cannot prove this
// run started (processes: #319). It fails the run too if a test launched `claude`
// from PATH (guardClaude: #93).
//
// TMPDIR is pointed at a directory of this run's own, so t.TempDir and every command
// line that names a path in it carry the run's mark. A timeout or an interrupt still
// kills this process's tmux servers first.
func Main(m *testing.M) int {
	flag.Parse()
	tmux, _ := exec.LookPath("tmux")
	warnStale(tmux)
	base := os.TempDir()
	mark, err := os.MkdirTemp(base, "leakcheck-"+strconv.Itoa(os.Getpid())+"-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "leakcheck:", err)
		return 1
	}
	os.Setenv("TMPDIR", mark)
	defer os.RemoveAll(mark)
	defer os.Setenv("TMPDIR", base)
	calls, err := guardClaude(mark)
	if err != nil {
		fmt.Fprintln(os.Stderr, "leakcheck:", err)
		return 1
	}
	killOwn := func() {
		if tmux != "" {
			for _, s := range ownSockets() {
				kill(tmux, s)
			}
		}
	}
	// A timed-out run panics without running any cleanup, and an interrupt ends it
	// at once: kill this process's servers just before either.
	if f := flag.Lookup("test.timeout"); f != nil {
		if d, err := time.ParseDuration(f.Value.String()); err == nil && d > 10*time.Second {
			timer := time.AfterFunc(d-5*time.Second, killOwn)
			defer timer.Stop()
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	go func() {
		if s, ok := <-sigs; ok {
			killOwn()
			fmt.Fprintf(os.Stderr, "leakcheck: %v: killed this run's tmux servers\n", s)
			os.Exit(1)
		}
	}()

	code := m.Run()

	if tmux != "" {
		if left := settle(func() []string { return ownSockets() }); len(left) > 0 {
			for _, s := range left {
				state := "a socket file left behind"
				if live(tmux, s) {
					state = "a LIVE tmux server"
				}
				fmt.Fprintf(os.Stderr, "leakcheck: tmux socket %s: %s; every socket needs leakcheck.TmuxSocket's cleanup\n", s, state)
				kill(tmux, s)
			}
			code = 1
		}
	}
	helpers := func() []string { return processes(os.Getpid(), os.Args[0], mark) }
	if left := settle(helpers); len(left) > 0 {
		for _, p := range left {
			fmt.Fprintf(os.Stderr, "leakcheck: helper process still running after the tests: %s\n", p)
			if pid, err := strconv.Atoi(strings.Fields(p)[0]); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		code = 1
	}
	// Read last: a lane or helper that called claude while it was being stopped is
	// still reported.
	if reportClaudeCalls(calls, os.Stderr) {
		code = 1
	}
	return code
}

// reportClaudeCalls writes one line to w for each call guardClaude's claude recorded
// in calls (one line each), and reports whether there was any.
func reportClaudeCalls(calls string, w io.Writer) bool {
	b, _ := os.ReadFile(calls)
	if len(b) == 0 {
		return false
	}
	lines := strings.Split(string(b), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // after the last call's newline; a call without arguments is an empty line too
	}
	for _, c := range lines {
		fmt.Fprintf(w, "leakcheck: a test launched the real claude (#93): %s; fake it in the test's runner\n", strings.TrimSuffix("claude "+c, " "))
	}
	return true
}

// guardClaude puts a directory of this run's first on PATH, holding a `claude` that
// records its arguments in the returned file, says why on stderr and exits 1. A test
// that reaches the real claude runs it with this user's login (`claude auth status`
// did, through a runner that refused only a bare argv[0] of "claude": #93), so a
// fake in the test's runner is the only claude a test may meet. The test binary run
// again as a helper, and a tmux server a test starts, inherit the PATH.
//
// It does not cover everything. A lane's PATH is rebuilt by lanes.SessionEnv, with
// the tool directories (git, gh, node) before the panel's PATH, so a bare claude in
// a lane can still resolve to a real one there: a test fakes a lane's claude through
// Options.LaneProgram. And a claude named by an absolute path is never caught.
func guardClaude(mark string) (string, error) {
	dir := filepath.Join(mark, "claude-guard")
	calls := filepath.Join(dir, "calls")
	q := "'" + strings.ReplaceAll(calls, "'", `'\''`) + "'"
	// printf, not echo: sh's echo reads \c in an argument, or a lone -n, as its own.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + q + "\necho 'a test launched the real claude (#93)' >&2\nexit 1\n"
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", err
	}
	// testbin's writer, though no test has forked yet in Main: a test that calls this
	// itself could race a parallel test's fork into "text file busy" (#78).
	if err := testbin.WriteFile(filepath.Join(dir, "claude"), script); err != nil {
		return "", err
	}
	return calls, os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// settle waits up to 3 s for list to come back empty (a killed server or process
// takes a moment to go), and returns what is left.
func settle(list func() []string) []string {
	var left []string
	for deadline := time.Now().Add(3 * time.Second); ; {
		if left = list(); len(left) == 0 || time.Now().After(deadline) {
			return left
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// processes lists, as "pid command", the processes this run provably started and
// left: its descendants that are its children (started and never reaped) or run this
// test binary (a helper), and any process whose command line names a path in this
// run's temp directory (mark), however far it was reparented.
//
// A process running this binary that is no descendant is someone else's: another
// run of the same binary path (two lanes, two scripts) is never listed, so Main never
// kills it (#319, ADR-0005). The cost, a known limit: a helper whose parent exited
// before it (`sh -c "bin ... &"`), and whose command line names no path in mark, is no
// longer found. Its environment still carries TMPDIR=mark, but reading another
// process's environment differs by OS (ps -E, /proc/<pid>/environ) and is not done.
func processes(self int, bin, mark string) []string {
	out, err := exec.Command("ps", "-A", "-ww", "-o", "pid=", "-o", "ppid=", "-o", "etime=", "-o", "command=").Output()
	if err != nil {
		return nil
	}
	return leftovers(string(out), self, bin, mark)
}

// leftovers is processes over ps's listing, one "pid ppid etime command" a line.
func leftovers(out string, self int, bin, mark string) []string {
	type proc struct {
		ppid int
		age  time.Duration
		cmd  string
	}
	all := map[int]proc{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		age, ok := elapsed(f[2])
		if err1 == nil && err2 == nil && ok {
			all[pid] = proc{ppid, age, strings.Join(f[3:], " ")}
		}
	}
	// descends walks pid's parents up to self. A parent younger than its child is a
	// reused pid (the real parent exited while ps read the table), not an ancestor:
	// etime has whole seconds, so a second of slack.
	descends := func(pid int) bool {
		for seen := map[int]bool{}; pid > 1 && !seen[pid]; {
			seen[pid] = true
			p, ok := all[pid]
			if !ok {
				return false
			}
			parent, ok := all[p.ppid]
			if !ok || parent.age+time.Second < p.age {
				return false
			}
			if p.ppid == self {
				return true
			}
			pid = p.ppid
		}
		return false
	}
	var found []string
	for pid, p := range all {
		if pid == self || strings.HasPrefix(p.cmd, "ps -A -ww") {
			continue
		}
		argv0 := strings.Fields(p.cmd)[0]
		ours := (p.ppid == self || argv0 == bin) && descends(pid)
		if ours || namesMark(p.cmd, mark) {
			found = append(found, strconv.Itoa(pid)+" "+p.cmd)
		}
	}
	sort.Strings(found)
	return found
}

// namesMark reports whether cmd names mark or a path in it. MkdirTemp ends mark with a
// decimal of varying length, so "leakcheck-100-7" must not match an earlier run's
// "leakcheck-100-71234" (the same pid, reused): a digit after mark is another mark.
func namesMark(cmd, mark string) bool {
	if mark == "" {
		return false
	}
	for rest := cmd; ; {
		i := strings.Index(rest, mark)
		if i < 0 {
			return false
		}
		rest = rest[i+len(mark):]
		if rest == "" || rest[0] < '0' || rest[0] > '9' {
			return true
		}
	}
}

// elapsed reads ps's etime, [[dd-]hh:]mm:ss, the same on macOS and Linux and in any
// locale.
func elapsed(s string) (time.Duration, bool) {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	secs := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0, false
		}
		secs = secs*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(secs)*time.Second, true
}

// warnStale names sockets that runs of this suite killed before their cleanup (a
// SIGKILL) left, for whoever ran them to remove. It never touches them: the pid in
// the name may be someone else's run.
func warnStale(tmux string) {
	if tmux == "" {
		return
	}
	entries, _ := os.ReadDir(SocketDir())
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), Prefix)
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(strings.SplitN(rest, "-", 2)[0])
		if err != nil || pid == os.Getpid() || syscall.Kill(pid, 0) == nil {
			continue
		}
		if live(tmux, e.Name()) {
			fmt.Fprintf(os.Stderr, "leakcheck: warning: tmux server on %s outlived its test run (pid %d is gone); `tmux -L %s kill-server` ends it\n",
				e.Name(), pid, e.Name())
		}
	}
}

// TableKeys is the bindings of one key table on sock, one `bind-key` line each,
// read from the full `list-keys`. tmux 3.7c's `list-keys -T <table>` prints nothing
// for a table that `unbind-key -a` emptied and a later bind-key filled again, though
// the binding is there and works (seen on the macOS runner).
func TableKeys(tmux, sock, table string) string {
	out, _ := exec.Command(tmux, "-L", sock, "list-keys").Output()
	var keep []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "-T" {
				if f[i+1] == table {
					keep = append(keep, strings.TrimSpace(line))
				}
				break
			}
		}
	}
	return strings.Join(keep, "\n")
}
