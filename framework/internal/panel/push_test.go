package panel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/testbin"
)

// PANEL-51 group 4 (D7): the owner's push command, push.json.

// pushStandIn writes a stand-in push program that records each run's arguments, one
// per line, in a file of its own under dir/runs; then it runs tail (shell text: an
// exit, a sleep). It returns the program's absolute path.
func pushStandIn(t *testing.T, tail string) (prog, runs string) {
	t.Helper()
	dir := t.TempDir()
	runs = filepath.Join(dir, "runs")
	if err := os.Mkdir(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	prog = filepath.Join(dir, "push")
	testbin.Write(t, prog, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+runs+"/run.$$\n"+tail+"\n")
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

// [STALL-3-S5] A push that fails names what it printed; one that does not answer is
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
	slow, _ := pushStandIn(t, "sleep 5")
	start := time.Now()
	err = runPush(context.Background(), 200*time.Millisecond, []string{slow}, "T", "B")
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("a push that never answers took %s, want it stopped at its timeout", took)
	}
	if err == nil || !strings.Contains(err.Error(), "no answer within") {
		t.Fatalf("a push past its timeout: %v, want a timeout error", err)
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

// [STALL-3-S4] End to end: a push.json writable by group never runs, the macOS
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

// [STALL-1-S25] A stall that begins across a restart pushes nothing while it is
// approximate, and once when the panel has watched for the stall time: the lane's
// session is busy at the first panel's stop and idle when the second starts, its
// pull request open throughout.
func TestStallAcrossRestartPushesOnceWatched(t *testing.T) {
	t.Parallel()
	root, wt := stallRepoAt(t, "0.05", "a") // 3 s: the second panel's window
	const down = 2500 * time.Millisecond
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
		t.Fatalf("the restart window (3 s) ended before the notifier ticked twice on the approximate stall: %+v", s)
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
