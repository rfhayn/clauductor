package panel

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testwait"
)

// PANEL-51 group 4 (D7): the owner's push command, push.json.

// pushStandIn writes a stand-in push program that records each run's arguments, one
// per line, in a file of its own under dir/runs; then it runs tail (shell text: an
// exit, a sleep). It returns the program's absolute path. The record is written
// beside runs and renamed in, whole: a test waits for it to appear, and under load
// one written in place was read before printf filled it (OPS-122).
func pushStandIn(t *testing.T, tail string) (prog, runs string) {
	t.Helper()
	dir := t.TempDir()
	runs = filepath.Join(dir, "runs")
	if err := os.Mkdir(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	prog = filepath.Join(dir, "push")
	testbin.Write(t, prog, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+dir+"/run.$$.tmp && mv "+dir+"/run.$$.tmp "+runs+"/run.$$\n"+tail+"\n")
	return prog, runs
}

// pushRuns lists the arguments of each run the stand-in recorded.
func pushRuns(t *testing.T, runs string) [][]string {
	t.Helper()
	es, err := os.ReadDir(runs)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Name() < es[j].Name() })
	var out [][]string
	for _, e := range es {
		b, err := os.ReadFile(filepath.Join(runs, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"))
	}
	return out
}

// writePush writes push.json, 0600, in home's panel folder (0700), holding body. A
// test that wants other modes sets them itself, each a literal (testbin's guard reads
// every chmod's mode).
func writePush(t *testing.T, home, body string) string {
	t.Helper()
	dir := config.PanelDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := pushPath(home)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o600); err != nil { // past the umask
		t.Fatal(err)
	}
	return p
}

func argvJSON(argv ...string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = fmt.Sprintf("%q", a)
	}
	return `{"argv":[` + strings.Join(q, ",") + `]}`
}

// [STALL-3-S4] A push file that is unsafe or malformed is refused, with a warning
// naming the file and why, and has no argv to run; with no file at all, there is
// neither a command nor a warning.
func TestReadPushRefusesByD7(t *testing.T) {
	t.Parallel()
	uid := os.Getuid()
	var none func(p string) error
	for _, c := range []struct {
		name, body string
		prep       func(p string) error // the modes, each a literal
		owner      int
		why        string // "" : accepted
	}{
		{"valid", argvJSON("/usr/bin/true", "-t", "topic"), none, uid, ""},
		{"valid, read-only to others", argvJSON("/usr/bin/true"), func(p string) error {
			if err := os.Chmod(p, 0o644); err != nil {
				return err
			}
			dir := filepath.Dir(p)
			if err := os.MkdirAll(dir, 0o700); err != nil { // it is: the guard sees a folder
				return err
			}
			return os.Chmod(dir, 0o755)
		}, uid, ""},
		{"an unknown key beside argv", `{"argv":["/usr/bin/true"],"note":"ntfy"}`, none, uid, ""},
		{"writable by group", argvJSON("/usr/bin/true"), func(p string) error { return os.Chmod(p, 0o620) }, uid, "writable by group or others"},
		{"writable by others", argvJSON("/usr/bin/true"), func(p string) error { return os.Chmod(p, 0o602) }, uid, "writable by group or others"},
		{"in a folder writable by group", argvJSON("/usr/bin/true"), func(p string) error {
			dir := filepath.Dir(p)
			if err := os.MkdirAll(dir, 0o700); err != nil { // it is: the guard sees a folder
				return err
			}
			return os.Chmod(dir, 0o770)
		}, uid, "folder"},
		{"not the user's", argvJSON("/usr/bin/true"), none, uid + 1, "not owned by you"},
		{"a relative program", argvJSON("ntfy", "publish", "topic"), none, uid, "not an absolute path"},
		{"not a JSON object", `["/usr/bin/true"]`, none, uid, "not one JSON object"},
		{"not JSON", `argv=/usr/bin/true`, none, uid, "not one JSON object"},
		{"two objects", argvJSON("/usr/bin/true") + argvJSON("/bin/sh"), none, uid, "more than one JSON value"},
		{"an empty argv", `{"argv":[]}`, none, uid, "non-empty list of strings"},
		{"no argv", `{"cmd":"/usr/bin/true"}`, none, uid, "non-empty list of strings"},
		{"an argv not of strings", `{"argv":["/usr/bin/true",1]}`, none, uid, "non-empty list of strings"},
	} {
		home := t.TempDir()
		p := writePush(t, home, c.body)
		if c.prep != nil {
			if err := c.prep(p); err != nil {
				t.Fatal(err)
			}
		}
		got := readPush(p, c.owner)
		switch {
		case c.why == "" && (got.refused != "" || len(got.argv) == 0):
			t.Errorf("%s: %+v, want accepted", c.name, got)
		case c.why != "" && len(got.argv) > 0:
			t.Errorf("%s: argv %q, want none to run", c.name, got.argv)
		case c.why != "" && (!strings.Contains(got.refused, c.why) || !strings.Contains(got.refused, p)):
			t.Errorf("%s: refused %q, want it to name %s and %q", c.name, got.refused, p, c.why)
		}
	}
	if got := readPush(pushPath(t.TempDir()), uid); got.refused != "" || len(got.argv) != 0 {
		t.Errorf("no push.json: %+v, want no command and no refusal", got)
	}
}

// pushLinkedInto makes home's push.json a symbolic link to a safe push.json (0600) in
// a folder of its own, 0700, which it returns with the link's path.
func pushLinkedInto(t *testing.T, home string) (link, dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "push.json")
	if err := os.WriteFile(target, []byte(argvJSON("/usr/bin/true")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil { // past the umask
		t.Fatal(err)
	}
	link = writePush(t, home, "")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link, dir
}

// [STALL-3-S4] A push.json that is a symbolic link is judged by the folder of the file
// it names too: a safe file in a folder writable by group is refused, naming that
// folder (group 4's review round 1, L5).
func TestReadPushRefusesALinkIntoAnUnsafeFolder(t *testing.T) {
	t.Parallel()
	link, dir := pushLinkedInto(t, t.TempDir())
	if got := readPush(link, os.Getuid()); len(got.argv) == 0 {
		t.Fatalf("the premise: a link to a safe file in a safe folder is accepted: %+v", got)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil { // it is: the guard sees a folder
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatal(err)
	}
	got := readPush(link, os.Getuid())
	if len(got.argv) > 0 {
		t.Fatalf("a link into a folder writable by group: argv %q, want none to run", got.argv)
	}
	if !strings.Contains(got.refused, dir) || !strings.Contains(got.refused, "writable by group or others") {
		t.Errorf("refused %q, want it to name the link's target folder %s and why", got.refused, dir)
	}
}

// [STALL-3-S4] Something at push.json's path that is not a regular file is refused,
// and a FIFO there is refused without being read: a read would block until something
// writes to it (group 4's review round 1, L5).
func TestReadPushRefusesWhatIsNotAFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p := writePush(t, home, "")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := readPush(p, os.Getuid()); len(got.argv) > 0 || !strings.Contains(got.refused, "not a regular file") {
		t.Errorf("a folder at push.json's path: %+v, want it refused as not a regular file", got)
	}
	fifo := pushPath(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan pushCmd, 1)
	go func() { done <- readPush(fifo, os.Getuid()) }()
	select {
	case got := <-done:
		if len(got.argv) > 0 || !strings.Contains(got.refused, "not a regular file") {
			t.Errorf("a FIFO at push.json's path: %+v, want it refused as not a regular file", got)
		}
	case <-time.After(testwait.Scale(2 * time.Second)):
		// Unblock the read so the goroutine ends with the test.
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = w.Close()
		}
		t.Fatal("reading a FIFO at push.json's path blocked: it was read, not refused")
	}
}

// [STALL-3-S4] A chain of links is judged hop by hop: push.json links to a link in a
// second folder, which links to a safe file in a third; with the second folder writable
// by group, anyone in the group could point the chain at a file of theirs, so the file
// is refused, naming that folder, by readPush and, once the folder's mode changes after
// the start, by the push source (group 5's review round 1, L4).
func TestReadPushJudgesEveryHopOfALinkChain(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	link, safe := pushLinkedInto(t, home) // push.json -> safe/push.json
	mid := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(mid, 0o700); err != nil {
		t.Fatal(err)
	}
	hop := filepath.Join(mid, "push.json")
	if err := os.Symlink(filepath.Join(safe, "push.json"), hop); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hop, link); err != nil { // push.json -> shared/push.json -> safe/push.json
		t.Fatal(err)
	}
	w := newPushWatch(home)
	if got := readPush(link, os.Getuid()); len(got.argv) == 0 || len(w.current().argv) == 0 {
		t.Fatalf("the premise: a chain through safe folders is accepted: %+v, at start %+v", got, w.current())
	}
	if err := os.MkdirAll(mid, 0o700); err != nil { // it is: the guard sees a folder
		t.Fatal(err)
	}
	if err := os.Chmod(mid, 0o770); err != nil {
		t.Fatal(err)
	}
	got := readPush(link, os.Getuid())
	if len(got.argv) > 0 || !strings.Contains(got.refused, mid) {
		t.Errorf("a chain through a folder writable by group: %+v, want it refused naming %s", got, mid)
	}
	w.poll(context.Background(), time.Now())
	if c := w.current(); len(c.argv) > 0 || !strings.Contains(c.refused, mid) {
		t.Errorf("the push source after the middle folder was made writable by group: %+v, want it refused", c)
	}
}

// [STALL-3-S4] A relative link is followed from the folder it really sits in, as the
// OS opens it: with the panel folder itself a link to another place, push.json ->
// ../shared/push.json names the shared folder beside that place, and that folder is
// the one judged (group 5's review round 2, 1).
func TestReadPushFollowsARelativeLinkFromItsRealFolder(t *testing.T) {
	t.Parallel()
	home, real := t.TempDir(), t.TempDir()
	panelDir := config.PanelDir(home)
	for _, d := range []string{filepath.Dir(panelDir), filepath.Join(real, "panel")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(real, "panel"), panelDir); err != nil {
		t.Fatal(err)
	}
	// Made by name, so testbin's scan reads the Chmod below as a folder's, not an executable's.
	shared := filepath.Join(real, "shared")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "push.json"), []byte(argvJSON("/usr/bin/true")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(shared, "push.json"), 0o600); err != nil { // past the umask
		t.Fatal(err)
	}
	link := pushPath(home)
	if err := os.Symlink(filepath.Join("..", "shared", "push.json"), link); err != nil {
		t.Fatal(err)
	}
	if got := readPush(link, os.Getuid()); len(got.argv) == 0 {
		t.Fatalf("a relative link the OS opens through the linked panel folder: %+v, want accepted", got)
	}
	if err := os.Chmod(shared, 0o770); err != nil {
		t.Fatal(err)
	}
	if got := readPush(link, os.Getuid()); len(got.argv) > 0 || !strings.Contains(got.refused, "writable by group or others") {
		t.Errorf("the link's real target folder made writable by group: %+v, want it refused", got)
	}
}

// pushLinkChain makes push.json, in home, the head of a chain of n links in a folder of
// its own (0700), ending at a safe file there.
func pushLinkChain(t *testing.T, home string, n int) string {
	t.Helper()
	link, dir := pushLinkedInto(t, home) // push.json -> dir/push.json
	next := filepath.Join(dir, "push.json")
	for i := 1; i < n; i++ {
		hop := filepath.Join(dir, fmt.Sprintf("hop%d", i))
		if err := os.Symlink(next, hop); err != nil {
			t.Fatal(err)
		}
		next = hop
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(next, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// [STALL-3-S4] A chain of links is followed at most pushMaxHops links: 8 are read, 9
// are refused (group 5's review round 2, 3).
func TestReadPushBoundsALinkChain(t *testing.T) {
	t.Parallel()
	if got := readPush(pushLinkChain(t, t.TempDir(), pushMaxHops), os.Getuid()); len(got.argv) == 0 {
		t.Errorf("a chain of %d links: %+v, want accepted", pushMaxHops, got)
	}
	got := readPush(pushLinkChain(t, t.TempDir(), pushMaxHops+1), os.Getuid())
	if len(got.argv) > 0 || !strings.Contains(got.refused, "more than 8 symbolic links") {
		t.Errorf("a chain of %d links: %+v, want it refused", pushMaxHops+1, got)
	}
}

// The push source reads push.json again when its link is pointed at another file of
// the same size, time and folder: only the chain's hops tell the two apart (group 5's
// review round 2, 3).
func TestPushWatchSeesALinkRetargeted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	link, dir := pushLinkedInto(t, home)
	a, b := filepath.Join(dir, "push.json"), filepath.Join(dir, "other.json")
	when := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	for p, prog := range map[string]string{a: "/bin/push-a", b: "/bin/push-b"} {
		if err := os.WriteFile(p, []byte(argvJSON(prog)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o600); err != nil { // past the umask
			t.Fatal(err)
		}
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
	w := newPushWatch(home)
	if c := w.current(); len(c.argv) != 1 || c.argv[0] != "/bin/push-a" {
		t.Fatalf("the premise: the link names push-a at start: %+v", c)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, link); err != nil {
		t.Fatal(err)
	}
	w.poll(context.Background(), time.Now())
	if c := w.current(); len(c.argv) != 1 || c.argv[0] != "/bin/push-b" {
		t.Errorf("after the link was pointed at a file of the same size and time: %+v, want push-b read", c)
	}
}

// The push source reads push.json again when the folder of the file a link names
// changes mode: readPush judges that folder, so a chmod there makes the file unsafe
// (or safe) with nothing else about it changed (group 4's review round 1, L4).
func TestPushWatchSeesTheLinkTargetsFolder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	_, dir := pushLinkedInto(t, home)
	w := newPushWatch(home)
	if c := w.current(); len(c.argv) == 0 {
		t.Fatalf("the premise: the link is accepted at start: %+v", c)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil { // it is: the guard sees a folder
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatal(err)
	}
	w.poll(context.Background(), time.Now())
	if c := w.current(); len(c.argv) > 0 || !strings.Contains(c.refused, dir) {
		t.Fatalf("after the link's target folder was made writable by group: %+v, want it refused", c)
	}
}

// [STALL-3-S5] [STALL-3-S8] A push that fails names what it printed; one that does not answer is
// stopped at its timeout, even with a child of it still holding its output open.
func TestRunPushFailsAndTimesOut(t *testing.T) {
	t.Parallel()
	prog, runs := pushStandIn(t, "echo 'no route' >&2; exit 1")
	err := runPush(context.Background(), pushTimeout, []string{prog, "-x"}, "T", "B")
	if err == nil || !strings.Contains(err.Error(), "no route") {
		t.Fatalf("a push exiting 1 with \"no route\": %v, want an error naming it", err)
	}
	if got := pushRuns(t, runs); len(got) != 1 || strings.Join(got[0], "|") != "-x|T|B" {
		t.Fatalf("the push ran with %q, want its argv then the title and the body, as arguments", got)
	}
	// The stand-in outlasts the scaled bound many times over: the bound tells a stop
	// at the timeout from a wait for the stand-in, on a loaded machine too (OPS-122).
	// Its child holds the output open past the timeout's kill (what WaitDelay is for);
	// the test kills that one child, so it does not outlive the test.
	childPid := filepath.Join(t.TempDir(), "child.pid")
	slow, _ := pushStandIn(t, "sleep 60 & echo $! > "+childPid+"; wait")
	t.Cleanup(func() {
		if b, err := os.ReadFile(childPid); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	// The timeout gives the stand-in time to start its child first: a kill before the
	// fork would leave nothing for WaitDelay to cut short (OPS-122 review).
	timeout := testwait.Scale(time.Second)
	start := time.Now()
	err = runPush(context.Background(), timeout, []string{slow}, "T", "B")
	if took := time.Since(start); took > timeout+testwait.Scale(3*time.Second) {
		t.Fatalf("a push that never answers took %s, want it stopped at its timeout", took)
	}
	if err == nil || !strings.Contains(err.Error(), "no answer within") {
		t.Fatalf("a push past its timeout: %v, want a timeout error", err)
	}
}

// [STALL-3-S5] A failed push's error, which the panel logs, carries the last line of
// what it printed, clipped: a program run verbose can print its request, token
// included, before the line that says why it failed (group 4's review round 1, L7).
func TestRunPushErrorIsItsLastLine(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 500)
	prog, _ := pushStandIn(t, "echo '> Authorization: Bearer tk_secret'; echo 'no route to host' >&2; echo; exit 1")
	err := runPush(context.Background(), pushTimeout, []string{prog}, "T", "B")
	if err == nil || !strings.Contains(err.Error(), "no route to host") || strings.Contains(err.Error(), "tk_secret") {
		t.Fatalf("a push that printed a token, then why it failed: %v, want its last line alone", err)
	}
	prog, _ = pushStandIn(t, "echo "+long+"; exit 1")
	err = runPush(context.Background(), pushTimeout, []string{prog}, "T", "B")
	if err == nil || len(err.Error()) > 300 {
		t.Fatalf("a push whose last line is 500 bytes: %d bytes of error, want it clipped", len(fmt.Sprint(err)))
	}
}

// stalledAlertOf is lane a's stalled alert since since, as applyStalls raises it: its
// Text in the model's words, its own fields in fixed ones.
func stalledAlertOf(since time.Time) state.AlertView {
	return state.AlertView{Key: state.AlertStalled + ":/r/w/a", Kind: state.AlertStalled, Severity: signals.SevBlock, Lane: "/r/w/a",
		Name: "a", Terminal: "lane-a", Text: "stalled: its turn ended waiting on 1 shell: Run checks (running); open work: #152 open",
		Since: since.UnixMilli(), Wait: "a shell in flight", OpenWork: []string{"#152 open"}}
}

// [STALL-3-S1] [STALL-3-S6] The push rides on the notifier: a waiting alert alone is a
// macOS notification and no push; the stall is both, once per stretch; and the push's
// body is the stalled alert's own fields, never the notice's body, which holds the
// waiting alert's text when the two are sent together.
func TestPushRidesOnTheStallAlone(t *testing.T) {
	t.Parallel()
	prog, runs := pushStandIn(t, "")
	cmd := pushCmd{argv: []string{prog, "--topic", "x"}}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	wait := state.AlertView{Key: "waiting:s1", Kind: state.AlertWaiting, Severity: signals.SevBlock, Lane: "/r/w/a", Name: "a",
		Terminal: "lane-a", Text: "waiting on you: wants to use Bash", Since: day.UnixMilli()}
	stall := stalledAlertOf(day)
	send := func(n *state.Notifier, at time.Time, alerts ...state.AlertView) (notices int) {
		t.Helper()
		for _, nt := range n.Process(alerts, nil, at) {
			notices++
			if _, err := pushNotice(context.Background(), cmd, "Stalls", nt, at); err != nil {
				t.Fatal(err)
			}
		}
		return notices
	}

	// STALL-3-S6: the waiting alert at 00:02 alone, then the stall at 00:30.
	n := &state.Notifier{MinInterval: time.Minute, Project: "Stalls"}
	if got := send(n, day.Add(2*time.Minute), wait); got != 1 || len(pushRuns(t, runs)) != 0 {
		t.Fatalf("00:02, the waiting alert: %d notices and %d pushes, want 1 and 0", got, len(pushRuns(t, runs)))
	}
	if got := send(n, day.Add(30*time.Minute), wait, stall); got != 1 || len(pushRuns(t, runs)) != 1 {
		t.Fatalf("00:30, the stall: %d notices and %d pushes, want 1 and 1", got, len(pushRuns(t, runs)))
	}
	for _, m := range []time.Duration{31, 45} {
		if got := send(n, day.Add(m*time.Minute), wait, stall); got != 0 || len(pushRuns(t, runs)) != 1 {
			t.Fatalf("00:%d, still stalled: %d notices and %d pushes in all, want 0 and 1", m, got, len(pushRuns(t, runs)))
		}
	}
	run := pushRuns(t, runs)[0]
	if len(run) != 4 || run[0] != "--topic" || run[1] != "x" {
		t.Fatalf("the push's arguments %q, want its argv, then the title and the body", run)
	}
	title, body := run[2], run[3]
	if title != "Stalls · a" {
		t.Errorf("the push's title %q, want the project and the lane", title)
	}
	for _, want := range []string{"quiet for 30m", "#152", "a shell in flight"} {
		if !strings.Contains(body, want) {
			t.Errorf("the push's body %q does not name %q", body, want)
		}
	}
	for _, never := range []string{"Run checks", "wants to use Bash"} {
		if strings.Contains(body, never) {
			t.Errorf("the push's body %q carries %q", body, never)
		}
	}

	// STALL-3-S1: the stall and a new waiting alert in one notice.
	prog2, runs2 := pushStandIn(t, "")
	cmd = pushCmd{argv: []string{prog2}}
	n2 := &state.Notifier{MinInterval: time.Minute, Project: "Stalls"}
	ns := n2.Process([]state.AlertView{wait, stall}, nil, day.Add(30*time.Minute))
	if len(ns) != 1 || !strings.Contains(ns[0].Body, "wants to use Bash") {
		t.Fatalf("the premise: one notice whose body holds the waiting alert's text: %+v", ns)
	}
	if _, err := pushNotice(context.Background(), cmd, "Stalls", ns[0], day.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := pushRuns(t, runs2)
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("pushes %q, want one with the title and the body", got)
	}
	if strings.Contains(got[0][1], "wants to use Bash") || !strings.Contains(got[0][1], "#152") {
		t.Errorf("the push's body %q: want the stall's own fields, not the waiting alert's text", got[0][1])
	}
}

// pushTicks are stallTicks with push.json re-read every 100 ms.
func pushTicks() Ticks {
	t := stallTicks()
	t.Push = 100 * time.Millisecond
	return t
}

// stallOnShell has the lane's session end its turn on a shell in flight, with a last
// line, so the stall's own words would name both.
func stallOnShell(t *testing.T, p *stallPanel, wt string) {
	t.Helper()
	waitFor(t, "lane's session listed", func() bool {
		for _, l := range p.state(t, "").Lanes {
			for _, s := range l.Sessions {
				if s.ID == "sa" {
					return true
				}
			}
		}
		return false
	})
	p.c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"Stop","session_id":"sa","cwd":%q,"last_assistant_message":"Checks are running.",
		"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"Run checks","command":"sh .claude/checks/run.sh"}],"session_crons":[]}`, wt))
}

// [STALL-3-S1] End to end: a lane with #152 open stalls on a shell in flight, and
// the panel sends one macOS notification and one push, whose title is the
// notification's and whose body names the quiet time, #152 and the kind of wait, and
// none of the model's words; and none after.
func TestStallPushEndToEnd(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	writePush(t, home, argvJSON(prog, "--to", "phone"))
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, pushTicks())
	stallOnShell(t, p, wt["a"])
	waitFor(t, "lane a stalled, notified and pushed", func() bool {
		return len(log.stalled()) == 1 && len(pushRuns(t, runs)) == 1
	})
	p.polls.more(t, p.state(t, "").ProjectID+"/notify", 3)
	if n, got := len(log.stalled()), pushRuns(t, runs); n != 1 || len(got) != 1 {
		t.Fatalf("%d notifications and %d pushes, want one of each", n, len(got))
	}
	run := pushRuns(t, runs)[0]
	if len(run) != 4 || run[0] != "--to" || run[1] != "phone" {
		t.Fatalf("the push's arguments %q, want its argv, then the title and the body", run)
	}
	title := ""
	log.mu.Lock()
	for _, n := range log.ns {
		if n.Stall != nil {
			title = n.Title
		}
	}
	log.mu.Unlock()
	if run[2] != title || !strings.Contains(title, "Stalls") {
		t.Errorf("the push's title %q, want the notification's, %q, naming the project", run[2], title)
	}
	for _, want := range []string{"quiet for", "#152", "a shell in flight"} {
		if !strings.Contains(run[3], want) {
			t.Errorf("the push's body %q does not name %q", run[3], want)
		}
	}
	for _, never := range []string{"Run checks", "run.sh", "Checks are running"} {
		if strings.Contains(run[3], never) {
			t.Errorf("the push's body %q carries %q", run[3], never)
		}
	}
}

// [STALL-3-S4] [STALL-3-S9] End to end: a push.json writable by group never runs, the macOS
// notification still goes, and a warning on the page names the file and why; once
// the file is made safe the warning goes, and with the file removed none shows.
func TestStallPushRefusedEndToEnd(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	path := writePush(t, home, argvJSON(prog))
	if err := os.Chmod(path, 0o620); err != nil {
		t.Fatal(err)
	}
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, pushTicks())
	warned := func() string {
		for _, s := range p.state(t, "").Warnings {
			if strings.Contains(s, path) {
				return s
			}
		}
		return ""
	}
	waitFor(t, "lane a stalled and notified", func() bool { return len(log.stalled()) == 1 })
	p.polls.more(t, p.state(t, "").ProjectID+"/notify", 2)
	if got := pushRuns(t, runs); len(got) != 0 {
		t.Fatalf("a push.json writable by group ran: %q", got)
	}
	if s := warned(); !strings.Contains(s, "writable by group or others") {
		t.Fatalf("warnings %q, want one naming %s and why", p.state(t, "").Warnings, path)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the warning gone once push.json is safe", func() bool { return warned() == "" })
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	p.polls.more(t, "push", 2)
	p.polls.more(t, p.state(t, "").ProjectID+"/notify", 2)
	for _, s := range p.state(t, "").Warnings {
		if strings.Contains(strings.ToLower(s), "push") {
			t.Errorf("with no push.json, a warning shows: %q", s)
		}
	}
}

// [STALL-3-S5] End to end: a push that exits 1 with "no route" is counted in the
// notifier's failures with its error, and the macOS notification still goes.
func TestStallPushFailureCounted(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	prog, runs := pushStandIn(t, "echo 'no route' >&2; exit 1")
	writePush(t, home, argvJSON(prog))
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, pushTicks())
	waitFor(t, "lane a stalled, notified and its push failed", func() bool {
		o := p.state(t, "").Observe
		return len(log.stalled()) == 1 && len(pushRuns(t, runs)) == 1 && o.NotifyFailed == 1
	})
	o := p.state(t, "").Observe
	if !strings.Contains(o.NotifyError, "no route") {
		t.Errorf("the notifier's error %q, want it to name \"no route\"", o.NotifyError)
	}
	if o.NotifySent != 1 {
		t.Errorf("notifications sent %d, want 1: the macOS notification still goes", o.NotifySent)
	}
}

// [STALL-3-S10] End to end: with `alerts.notify` false, a lane with #152 open stalls
// and is shown stalled, yet neither a macOS notification nor a push goes out, though
// push.json is valid: the switch holds back the push as well as the notification.
func TestStallNotifyOffSendsNothing(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Stalls","version":7,"lanes":{"main":"orchestrator","change/":"build"},
		"base":"main","tmux_socket":"`+noServerSocket()+`","alerts":{"stall_minutes":`+stallMinutes+`,"notify":false}}`)
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	writePush(t, home, argvJSON(prog))
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, pushTicks())
	waitFor(t, "lane a shown stalled", func() bool { return laneStall(p.state(t, ""), wt["a"]) != nil })
	p.polls.more(t, p.state(t, "").ProjectID+"/notify", 3)
	if st := laneStall(p.state(t, ""), wt["a"]); st == nil {
		t.Fatal("lane a is no longer shown stalled")
	}
	log.mu.Lock()
	sent := len(log.ns)
	log.mu.Unlock()
	if got := pushRuns(t, runs); sent != 0 || len(got) != 0 {
		t.Fatalf("with alerts.notify false: %d notifications and %d pushes (%q), want none", sent, len(got), got)
	}
}

// [STALL-3-S11] End to end: a config that is not trusted names its project "Stalls",
// and a lane with #152 open stalls: the notification and the push go out, each titled
// "clauductor panel" and the lane, and neither title carries the untrusted name.
func TestStallPushUntrustedTitle(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	writePush(t, home, argvJSON(prog))
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTrust(t, root, home, w, log, pushTicks(), false)
	if p.state(t, "").Trust.Trusted {
		t.Fatal("the config is trusted; the test needs one that is not")
	}
	waitFor(t, "lane a stalled, notified and pushed", func() bool {
		return len(log.stalled()) == 1 && len(pushRuns(t, runs)) == 1
	})
	title := ""
	log.mu.Lock()
	for _, n := range log.ns {
		if n.Stall != nil {
			title = n.Title
		}
	}
	log.mu.Unlock()
	run := pushRuns(t, runs)[0]
	if len(run) != 2 {
		t.Fatalf("the push's arguments %q, want the title and the body", run)
	}
	for what, got := range map[string]string{"the notification's": title, "the push's": run[0]} {
		if got != "clauductor panel · a" {
			t.Errorf("%s title %q, want \"clauductor panel\" and the lane, never the untrusted name", what, got)
		}
	}
}

// [STALL-3-S8] [STALL-3-S9] The 10 s of STALL-3: a push is stopped past 10 s, and
// push.json is read again within 10 s of a change.
func TestPushBoundsAreTenSeconds(t *testing.T) {
	t.Parallel()
	if pushTimeout != 10*time.Second {
		t.Errorf("pushTimeout %s, want 10s", pushTimeout)
	}
	if got := DefaultTicks().Push; got != 10*time.Second {
		t.Errorf("the default push.json re-read %s, want 10s", got)
	}
}

// [STALL-1-S25] A stall that begins across a restart pushes nothing while it is
// approximate, and once when the panel has watched for the stall time: the lane's
// session is busy at the first panel's stop and idle when the second starts, its
// pull request open throughout.
func TestStallAcrossRestartPushesOnceWatched(t *testing.T) {
	t.Parallel()
	// The stall time, 3 s (scaled by CLAUDUCTOR_TEST_SLOW), is the second panel's
	// window; the first watches the lane quiet for most of it.
	window := testwait.Scale(3 * time.Second)
	root, wt := stallRepoAt(t, strconv.FormatFloat(window.Minutes(), 'f', -1, 64), "a")
	down := window * 5 / 6
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	writePush(t, home, argvJSON(prog))
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "busy", "")}
		w.open["change/a"] = 152
	})
	first := &noticeLog{}
	ticks := pushTicks()
	ticks.StallSave = time.Hour // the clean stop's save alone
	p := runStallPanelTicks(t, root, home, w, first, ticks)
	waitFor(t, "lane a's session busy and #152 listed", func() bool {
		v := p.state(t, "")
		for _, l := range v.Lanes {
			for _, s := range l.Sessions {
				if s.ID == "sa" && listsPR(v, 152) {
					return true
				}
			}
		}
		return false
	})
	// The first panel watches the lane quiet (its session busy, which is working) for
	// most of the stall time before it stops. A restart carries a lane not stalled at
	// the save back by the quiet seen before it (D1), so the lane is past the stall
	// time early in the second panel's window, and stays approximate until its end.
	time.Sleep(down)
	p.stop()
	w.set(func(w *stallWorld) { w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")} })
	again := &noticeLog{}
	p2 := runStallPanelTicks(t, root, home, w, again, pushTicks())
	waitFor(t, "lane a stalled, approximate", func() bool {
		s := laneStall(p2.state(t, ""), wt["a"])
		return s != nil && s.Approx
	})
	id := p2.state(t, "").ProjectID
	p2.polls.more(t, id+"/notify", 2)
	if s := laneStall(p2.state(t, ""), wt["a"]); s == nil || !s.Approx {
		t.Fatalf("the restart window (%s) ended before the notifier ticked twice on the approximate stall: %+v", window, s)
	}
	if n, got := len(again.stalled()), pushRuns(t, runs); n != 0 || len(got) != 0 {
		t.Fatalf("an approximate stall: %d notifications and %d pushes, want none", n, len(got))
	}
	waitFor(t, "lane a's stall exact, notified and pushed", func() bool {
		s := laneStall(p2.state(t, ""), wt["a"])
		return s != nil && !s.Approx && len(again.stalled()) == 1 && len(pushRuns(t, runs)) == 1
	})
	p2.polls.more(t, id+"/notify", 3)
	if n, got := len(again.stalled()), pushRuns(t, runs); n != 1 || len(got) != 1 {
		t.Fatalf("after the window: %d notifications and %d pushes, want one of each", n, len(got))
	}
}

// OPS-118 D6: a catch-up notice is a notification and a push; a waiting one (a config
// left for the owner) only while the loaded config's alerts.notify is on.
func TestSendCatchUpPushesItsNotice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	prog, runs := pushStandIn(t, "")
	cmd := pushCmd{argv: []string{prog, "--topic", "x"}}
	var sent []state.Notice
	notify := func(_ context.Context, n state.Notice) error { sent = append(sent, n); return nil }
	var out strings.Builder

	waits := catchUpNotice{root: "/r", key: "catch-up\x00/r\x00arrival\x00c", body: "did not trust #7", waits: true}
	if sendCatchUp(ctx, notify, cmd, &out, waits, false) || len(sent) != 0 || len(pushRuns(t, runs)) != 0 {
		t.Fatalf("a waiting notice with alerts.notify false went: %d notices, %d pushes", len(sent), len(pushRuns(t, runs)))
	}
	// alerts.notify false withholds only the notice that waits for the owner: a refusal
	// or a failed rebuild still reaches the owner.
	refusal := catchUpNotice{root: "/r", key: "catch-up\x00/r\x00refused", body: "Catch-up left /r as it was."}
	if !sendCatchUp(ctx, notify, cmd, &out, refusal, false) {
		t.Fatal("a refusal was withheld with alerts.notify false")
	}
	trusted := catchUpNotice{root: "/r", key: "catch-up\x00/r\x00arrival\x00d", body: "Catch-up trusted the new config on arrival from #7.",
		trusted: strings.Repeat("P", 90)}
	if !sendCatchUp(ctx, notify, cmd, &out, trusted, true) {
		t.Fatal("a trust notice was withheld")
	}
	if len(sent) != 2 || sent[0].Title != catchUpTitle || sent[1].Title != trusted.trusted {
		t.Fatalf("notices %+v: want a refusal titled %q, then the trust notice titled with the config's name", sent, catchUpTitle)
	}
	got := pushRuns(t, runs)
	if len(got) != 2 || len(got[1]) != 4 || got[1][0] != "--topic" || got[1][3] != trusted.body {
		t.Fatalf("pushes %q, want two, each its argv, then the title and the body", got)
	}
	if got[0][2] != catchUpTitle || got[1][2] != signals.Clip(trusted.trusted, 80) {
		t.Errorf("push titles %q and %q: want %q, then the name clipped to 80", got[0][2], got[1][2], catchUpTitle)
	}
	if out.Len() != 0 {
		t.Errorf("the panel log: %q", out.String())
	}
}

// On the Mac, with no push.json the owner's one channel, a catch-up notice arrives
// whole. It builds its bodies by hand and drives no fast-forward, so it cites no
// scenario: the CATCHUP-3 tests in catchup_trust_integration_test.go drive them and
// read each notice through catchUpNotifyArgv, as the Mac shows it. Review
// 2026-10-22 found the trust notice of a checkout at /Users/rich/Development/clauductor
// reaching osascript flattened and cut at 140 runes, its − line gone and its + line cut
// mid-command, and a notice that waits, or names a load error, without its reason. The
// catch-up source shows its notices with notifyCatchUp, never SendNotice's summary.
func TestCatchUpNoticeReachesTheMacWhole(t *testing.T) {
	t.Parallel()
	c := newCatchUp(Options{}, nil, nil, nil)
	if reflect.ValueOf(c.notify).Pointer() != reflect.ValueOf(notifyCatchUp).Pointer() {
		t.Fatal("with no test recording its notices, catch-up does not show them with notifyCatchUp")
	}
	root := "/Users/rich/Development/clauductor"
	x, y := `card x runs ["sh" ".claude/x.sh"] (interval:60)`, `card y runs ["sh" ".claude/y.sh"] (interval:60)`
	trust := catchUpNotice{root: root, key: "catch-up\x00k1", trusted: "Clauductor", body: trustBody(
		"Catch-up trusted the new .claude/panel.json in "+root+" on arrival from #7, merged by rhayn.", []string{x}, []string{y})}
	reason := "the loaded config was not the file the fast-forward started from (0123abc); the new config turns notices off (alerts.notify is false)"
	waits := catchUpNotice{root: root, key: "catch-up\x00k2", waits: true, body: catchUpBody("Catch-up moved " + root +
		" to abc1234 (#7, merged by rhayn), and did not trust its new .claude/panel.json: " + reason +
		". The config waits for the owner: Trust config… on the page, or `clauductor panel trust`.")}
	loadErr := "panel config: card x: unknown field \"intervall\""
	noLoad := catchUpNotice{root: root, key: "catch-up\x00k3", body: catchUpBody("Catch-up moved " + root +
		" to abc1234 (#7, merged by rhayn), and its new .claude/panel.json does not load, so the panel keeps serving the config it had loaded: " + loadErr + ".")}
	var sent []state.Notice
	notify := func(_ context.Context, n state.Notice) error { sent = append(sent, n); return nil }
	for _, n := range []catchUpNotice{trust, waits, noLoad} {
		if !sendCatchUp(context.Background(), notify, pushCmd{}, io.Discard, n, true) {
			t.Fatalf("%q was withheld", n.body)
		}
	}
	mac := func(i int) (title, body string) {
		argv := catchUpNotifyArgv(sent[i])
		return argv[len(argv)-2], argv[len(argv)-1]
	}
	title, body := mac(0)
	if title != "Clauductor" || body != trust.body {
		t.Fatalf("the trust notice on the Mac: %q, %q; want the config's name and the whole notice %q", title, body, trust.body)
	}
	if !strings.Contains(body, "#7") || !strings.Contains(body, "rhayn") || !strings.Contains(body, "\n+ "+x) || !strings.Contains(body, "\n− "+y) {
		t.Fatalf("the trust notice on the Mac lacks #7, the login, or a whole + or − line:\n%s", body)
	}
	if title, body = mac(1); title != catchUpTitle || !strings.Contains(body, reason) || !strings.Contains(body, "waits for the owner") {
		t.Fatalf("the notice that waits on the Mac, %q: %q lacks its reason or that it waits for the owner", title, body)
	}
	if _, body = mac(2); !strings.Contains(body, loadErr) {
		t.Fatalf("the notice of a config that does not load on the Mac: %q lacks the load's error", body)
	}
	// Past catchUpBodyMax the one text both ways carry is cut there, its lines kept.
	long := catchUpNotice{root: root, key: "catch-up\x00k4", body: "head\n" + strings.Repeat("x", 2*catchUpBodyMax)}
	sendCatchUp(context.Background(), notify, pushCmd{}, io.Discard, long, true)
	if _, body = mac(3); len([]rune(body)) != catchUpBodyMax || !strings.HasPrefix(body, "head\nxxx") || !strings.HasSuffix(body, "…") {
		t.Fatalf("a body over %d runes reached the Mac as %d runes: %.40q", catchUpBodyMax, len([]rune(body)), body)
	}
}

// [STALL-3-S12] With push.json's argv a stand-in, a project named "Panel" with catch-up
// on and a trusted config, and a session waiting on a permission prompt past
// waiting_seconds: the owner's #7, changing panel.json, goes out once as a notification
// and once as a push, titled "Panel", naming #7; the waiting alert goes out as a
// notification and no push.
func TestCatchUpNoticeIsPushedAndAWaitingAlertIsNot(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	cfg := cardCfg("Panel", true, ".claude/x.sh")
	w := newTrustWorld(t, cardCfg("Panel", true, ".claude/y.sh"), true)
	prog, runs := pushStandIn(t, "")
	writePush(t, w.home, argvJSON(prog))
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v\n%s", len(ns), ns, log.String())
	}
	noticeSays(t, ns[0], "Panel", "#7")
	got := pushRuns(t, runs)
	// The stand-in records an argument a line, and the body has lines of its own (+, −).
	if len(got) != 1 || len(got[0]) < 2 || got[0][0] != "Panel" || strings.Join(got[0][1:], "\n") != ns[0].Body {
		t.Fatalf("pushes %q, want one titled Panel, its body the notice's\n%s", got, log.String())
	}

	// The waiting alert, past waiting_seconds, through the notifier and the push the
	// panel runs on each notice it sends.
	at := time.Now()
	wait := state.AlertView{Key: "waiting:s1", Kind: state.AlertWaiting, Severity: signals.SevBlock, Lane: "/r/w/a", Name: "a",
		Terminal: "lane-a", Text: "waiting on you: wants to use Bash", Since: at.Add(-2 * time.Minute).UnixMilli()}
	n := &state.Notifier{MinInterval: time.Minute, Project: "Panel"}
	sent := n.Process([]state.AlertView{wait}, nil, at)
	if len(sent) != 1 {
		t.Fatalf("the waiting alert: %d notifications, want 1", len(sent))
	}
	if pushed, err := pushNotice(context.Background(), pushCmd{argv: []string{prog}}, "Panel", sent[0], at); pushed || err != nil {
		t.Fatalf("the waiting alert was pushed (%v, %v)", pushed, err)
	}
	if got := pushRuns(t, runs); len(got) != 1 {
		t.Fatalf("pushes %q, want the catch-up notice's alone", got)
	}
}
