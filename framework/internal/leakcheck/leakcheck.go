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
// started (the test binary run again, or any process whose command line names this
// run's temp directory) is still alive. It reports and removes both, so a failing
// run does not leak either.
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
	return code
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

// processes lists, as "pid command", the processes other than this one that are
// its children (started and never reaped), or whose command line contains any of
// marks: the test binary run again as a helper, or a command naming a path in this
// run's temp directory, however far it was reparented.
func processes(parent int, marks ...string) []string {
	out, err := exec.Command("ps", "-A", "-ww", "-o", "pid=", "-o", "ppid=", "-o", "command=").Output()
	if err != nil {
		return nil
	}
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil || pid == parent {
			continue
		}
		cmd := strings.Join(f[2:], " ")
		if strings.HasPrefix(cmd, "ps -A -ww") { // this listing itself
			continue
		}
		hit := ppid == parent
		for _, m := range marks {
			hit = hit || (m != "" && strings.Contains(cmd, m))
		}
		if hit {
			found = append(found, strconv.Itoa(pid)+" "+cmd)
		}
	}
	sort.Strings(found)
	return found
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
