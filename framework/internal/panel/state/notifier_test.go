package state

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

func TestNotifierRateLimitGroupingFocusAndCounter(t *testing.T) {
	t.Parallel()
	n := &Notifier{MinInterval: 5 * time.Minute, Project: "P"}
	a1 := AlertView{Key: "waiting:s1", Kind: AlertWaiting, Severity: signals.SevBlock, Terminal: "lane-a", Name: "lane-a", Text: "waiting on you for 3m"}
	a2 := AlertView{Key: "no_auto_resume:s1", Kind: AlertNoAutoResume, Severity: signals.SevWarn, Terminal: "lane-a", Name: "lane-a", Text: "will not auto-resume"}
	b1 := AlertView{Key: "waiting:s2", Kind: AlertWaiting, Severity: signals.SevBlock, Terminal: "lane-b", Name: "lane-b", Text: "waiting"}
	// Two alerts of one lane: ONE notification.
	out := n.Process([]AlertView{a1, a2}, nil, t0)
	if len(out) != 1 || out[0].Title != "P · lane-a" || !strings.Contains(out[0].Body, "waiting") || !strings.Contains(out[0].Body, "auto-resume") {
		t.Fatalf("grouping: %+v", out)
	}
	// The same alerts again: nothing (once per stretch).
	if out := n.Process([]AlertView{a1, a2}, nil, t0.Add(time.Minute)); len(out) != 0 {
		t.Fatalf("repeated: %+v", out)
	}
	// A new alert on the same lane inside the interval waits.
	a3 := AlertView{Key: "rate_limit:s1", Kind: AlertRateLimit, Severity: signals.SevBlock, Terminal: "lane-a", Name: "lane-a", Text: "rate limit"}
	if out := n.Process([]AlertView{a1, a2, a3}, nil, t0.Add(2*time.Minute)); len(out) != 0 || n.Stats().Deferred != 1 {
		t.Fatalf("rate limit: %+v %+v", out, n.Stats())
	}
	// ...and goes out once the interval has passed, if still active.
	if out := n.Process([]AlertView{a1, a2, a3}, nil, t0.Add(5*time.Minute)); len(out) != 1 || out[0].Body != "rate limit" {
		t.Fatalf("after interval: %+v", out)
	}
	// Another lane is not rate-limited by lane-a, but its terminal has focus: suppressed.
	if out := n.Process([]AlertView{b1}, map[string]bool{"lane-b": true}, t0.Add(5*time.Minute)); len(out) != 0 || n.Stats().Suppressed != 1 {
		t.Fatalf("focus: %+v %+v", out, n.Stats())
	}
	// Suppressed means seen: losing focus does not send it later.
	if out := n.Process([]AlertView{b1}, nil, t0.Add(6*time.Minute)); len(out) != 0 {
		t.Fatalf("sent after focus loss: %+v", out)
	}
	if n.Stats().Interrupts != 2 {
		t.Fatalf("interrupts %d", n.Stats().Interrupts)
	}
	// A condition that clears and comes back notifies again (after the interval).
	n.Process(nil, nil, t0.Add(20*time.Minute))
	if out := n.Process([]AlertView{a1}, nil, t0.Add(21*time.Minute)); len(out) != 1 {
		t.Fatalf("recurrence: %+v", out)
	}
	// The counter is per day.
	if n.Process(nil, nil, t0.Add(24*time.Hour)); n.Stats().Interrupts != 0 || n.Stats().Day != "2026-09-29" {
		t.Fatalf("day rollover: %+v", n.Stats())
	}
}

// The notification's AppleScript is fixed text; untrusted text only ever arrives as
// an argument after it.
func TestNotifyArgvKeepsTextOutOfTheScript(t *testing.T) {
	t.Parallel()
	hostile := `x" & (do shell script "touch /tmp/pwned") & "`
	argv := NotifyArgv(hostile, hostile+"\n"+hostile)
	var script []string
	if argv[len(argv)-3] != "--" {
		t.Fatalf("no -- between the script and the text: %q", argv)
	}
	for i := 1; i < len(argv)-3; i += 2 {
		if argv[i] != "-e" {
			t.Fatalf("argv[%d] = %q, want -e", i, argv[i])
		}
		script = append(script, argv[i+1])
	}
	for _, line := range script {
		if strings.Contains(line, "pwned") || strings.Contains(line, "x\"") {
			t.Fatalf("untrusted text reached the script: %q", line)
		}
	}
	if argv[len(argv)-2] != hostile || !strings.Contains(argv[len(argv)-1], "do shell script") {
		t.Fatalf("text is not passed as arguments: %q", argv[len(argv)-2:])
	}
}

// Run through real osascript: the argument comes back byte for byte and nothing it
// contains is executed. The script returns item 2 of argv instead of displaying it,
// so the test shows no notification.
func TestOsascriptArgvRoundTrip(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("-short: runs osascript")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("/usr/bin/osascript"); err != nil {
		t.Skip("no osascript")
	}
	marker := filepath.Join(t.TempDir(), "pwned")
	hostile := `a" & (do shell script "touch ` + marker + `") & "b` + "\\\" ' ¬ «data» end run"
	// osascript parses options among its arguments: a title starting with "-e" is
	// more script unless "--" ends the options (the reviewer's probe, 2026-09-28).
	dash := `-eproperty p : (do shell script "touch ` + marker + `") --`
	for _, args := range [][2]string{{"title", hostile}, {dash, hostile}, {"-e", "-l"}} {
		for i, want := range args {
			argv := osascriptArgv([]string{fmt.Sprintf("return item %d of argv", i+1)}, args[0], args[1])
			out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
			if err != nil {
				t.Fatalf("%q: %v %s", args, err, out)
			}
			if got := strings.TrimSuffix(string(out), "\n"); got != want {
				t.Fatalf("round trip of item %d:\n got %q\nwant %q", i+1, got, want)
			}
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("an argument was executed")
	}
	// And the notification's own argv puts "--" before the text.
	argv := NotifyArgv(dash, "x")
	if argv[len(argv)-3] != "--" {
		t.Fatalf("no -- before the data: %q", argv)
	}
}

// NotifyLinesArgv keeps a body's lines and cuts it only at max (OPS-118 D6), where
// NotifyArgv flattens it and cuts at signals.DetailMax; its text still comes after "--".
func TestNotifyLinesArgvKeepsTheBodysLines(t *testing.T) {
	t.Parallel()
	body := "head " + strings.Repeat("h", 150) + "\n+ added line\n− removed line"
	argv := NotifyLinesArgv(strings.Repeat("T", 90), body, 600)
	if argv[len(argv)-3] != "--" || argv[len(argv)-1] != body || len([]rune(argv[len(argv)-2])) != 80 {
		t.Fatalf("argv %q: want --, the title cut at 80 runes, then the body as it is", argv)
	}
	if got := NotifyLinesArgv("t", body, 20)[len(argv)-1]; got != body[:19]+"…" {
		t.Fatalf("a body over max: %q", got)
	}
	if got := NotifyArgv("t", body)[len(argv)-1]; strings.Contains(got, "removed line") {
		t.Fatalf("NotifyArgv kept what follows DetailMax (%q): the test no longer shows the difference", got)
	}
}

// Review 2026-09-28: only what blocks you interrupts, and a restart never
// re-notifies an alert that is still active.
func TestNotifierInterruptsOnlyForBlockingAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	n := &Notifier{MinInterval: time.Minute}
	pageOnly := []AlertView{
		{Key: "idle:s", Kind: AlertIdle, Severity: signals.SevInfo, Terminal: "a", Text: "idle"},
		{Key: "context:s", Kind: AlertContext, Severity: signals.SevWarn, Terminal: "a", Text: "ctx"},
		{Key: "quota:global", Kind: AlertQuota, Severity: signals.SevWarn, Text: "quota"},
		{Key: "stop_failure:s", Kind: AlertStopFailure, Severity: signals.SevWarn, Terminal: "a", Text: "failed"},
	}
	if out := n.Process(pageOnly, nil, t0); len(out) != 0 {
		t.Fatalf("page-only alerts interrupted: %+v", out)
	}
	block := AlertView{Key: "waiting:s", Kind: AlertWaiting, Severity: signals.SevBlock, Terminal: "a", Text: "waiting", Since: t0.UnixMilli()}
	if out := n.Process(append(pageOnly, block), nil, t0); len(out) != 1 {
		t.Fatalf("a blocking alert did not interrupt: %+v", out)
	}
	// Restart: a new notifier restored from the saved state, the alert still active.
	b, _ := json.Marshal(n.State())
	var st NotifierState
	json.Unmarshal(b, &st)
	n2 := &Notifier{MinInterval: time.Minute}
	n2.Restore(st)
	if out := n2.Process([]AlertView{block}, nil, t0.Add(2*time.Minute)); len(out) != 0 {
		t.Fatalf("re-notified after a restart: %+v", out)
	}
	if n2.Stats().Interrupts != 1 {
		t.Fatalf("interrupt count lost across the restart: %+v", n2.Stats())
	}
	// Round 2: the saved state keeps each key's Since. The same condition in a NEW
	// stretch (it cleared while the panel was down, then came back) notifies again.
	n3 := &Notifier{MinInterval: time.Minute}
	n3.Restore(st)
	again := block
	again.Since = t0.Add(10 * time.Minute).UnixMilli()
	if out := n3.Process([]AlertView{again}, nil, t0.Add(11*time.Minute)); len(out) != 1 {
		t.Fatalf("a new stretch after a restart did not notify: %+v", out)
	}
	// Without the saved state it would have notified again (the test's premise).
	if out := (&Notifier{MinInterval: time.Minute}).Process([]AlertView{block}, nil, t0); len(out) != 1 {
		t.Fatal("premise: a fresh notifier notifies")
	}
}

// OPS-122: a lane's alert carries its terminal once tmux reports the terminal running,
// and only its worktree before. After a restart, a loaded machine's tmux poll can
// answer after `claude agents` does: the alert, in a new stretch (its Since restarts
// with the panel), comes in under the worktree alone. The lane was notified inside
// MinInterval under its terminal, so it waits all the same.
func TestNotifierIntervalHoldsWhileTheLaneIsUnbound(t *testing.T) {
	t.Parallel()
	bound := AlertView{Key: "waiting:s", Kind: AlertWaiting, Severity: signals.SevBlock, Lane: "/repo/.wt/bug", Name: "bug",
		Terminal: "bug", Text: "waiting", Since: t0.UnixMilli()}
	n := &Notifier{MinInterval: 10 * time.Minute}
	if out := n.Process([]AlertView{bound}, nil, t0); len(out) != 1 {
		t.Fatalf("the bound alert: %+v, want one notice", out)
	}
	unbound := bound
	unbound.Terminal, unbound.Since = "", t0.Add(2*time.Second).UnixMilli()
	n2 := &Notifier{MinInterval: 10 * time.Minute}
	n2.Restore(n.State())
	if out := n2.Process([]AlertView{unbound}, nil, t0.Add(3*time.Second)); len(out) != 0 {
		t.Fatalf("the restart re-notified the lane before tmux bound it: %+v", out)
	}
	// And the other way: a notice sent while unbound holds the lane once bound.
	n3 := &Notifier{MinInterval: 10 * time.Minute}
	if out := n3.Process([]AlertView{unbound}, nil, t0); len(out) != 1 {
		t.Fatalf("the unbound alert: %+v, want one notice", out)
	}
	rebound := bound
	rebound.Since = t0.Add(5 * time.Second).UnixMilli()
	if out := n3.Process([]AlertView{rebound}, nil, t0.Add(6*time.Second)); len(out) != 0 {
		t.Fatalf("the lane, once bound, was notified again inside MinInterval: %+v", out)
	}
	// The premise: past MinInterval the new stretch notifies.
	if out := n3.Process([]AlertView{rebound}, nil, t0.Add(11*time.Minute)); len(out) != 1 {
		t.Fatalf("past MinInterval: %+v, want one notice", out)
	}
}

// #231: a waiting alert's Since is when this panel first saw the wait, so a restart
// gives a stretch still waiting a new one. Past MinInterval, with the lane still
// waiting, the restart sends nothing: before `claude agents` answers, while the alert
// waits out its threshold, or once it shows with its new Since. The wait ending and
// coming back after the restart is a new stretch, and notifies.
func TestNotifierRestartPastMinIntervalKeepsAWaitNotified(t *testing.T) {
	t.Parallel()
	const cfg = `,"alerts":{"waiting_seconds":60,"stall_minutes":0}`
	wts := []signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}
	waiting := []signals.Agent{{SessionID: "s", Cwd: "/repo/w/x", Status: "waiting", WaitingFor: "input needed", PID: 7}}
	waitAlert := func(v View) []AlertView {
		var out []AlertView
		for _, a := range v.Alerts {
			if a.Kind == AlertWaiting {
				out = append(out, a)
			}
		}
		return out
	}

	a := NewModel(v2Config(t, cfg), "/repo", t0)
	a.ApplyWorktrees(wts, nil, t0)
	a.ApplyAgents(waiting, nil, t0)
	a.ApplyAgents(waiting, nil, t0.Add(2*time.Minute))
	n := &Notifier{MinInterval: 5 * time.Minute}
	if out := n.ProcessView(a.Snapshot(t0.Add(2*time.Minute)), nil, t0.Add(2*time.Minute)); len(out) != 1 {
		t.Fatalf("before the restart: %d notices, want 1", len(out))
	}

	start := t0.Add(20 * time.Minute)
	b := NewModel(v2Config(t, cfg), "/repo", start)
	b.ApplyWorktrees(wts, nil, start)
	n2 := &Notifier{MinInterval: 5 * time.Minute}
	n2.Restore(n.State())
	at := func(d time.Duration) time.Time { return start.Add(d) }
	if out := n2.ProcessView(b.Snapshot(at(0)), nil, at(0)); len(out) != 0 {
		t.Fatalf("before claude agents answers: %+v", out)
	}
	b.ApplyAgents(waiting, nil, at(2*time.Second))
	if out := n2.ProcessView(b.Snapshot(at(3*time.Second)), nil, at(3*time.Second)); len(out) != 0 {
		t.Fatalf("inside the waiting threshold: %+v", out)
	}
	b.ApplyAgents(waiting, nil, at(70*time.Second))
	v := b.Snapshot(at(70 * time.Second))
	if w := waitAlert(v); len(w) != 1 || w[0].Since != at(2*time.Second).UnixMilli() {
		t.Fatalf("premise: the waiting alert after the restart is %+v, want one with the restart's Since", w)
	}
	if out := n2.ProcessView(v, nil, at(70*time.Second)); len(out) != 0 {
		t.Fatalf("the restart re-notified a wait still active past MinInterval: %+v", out)
	}

	// The wait ends and comes back: a new stretch, and it notifies.
	busy := []signals.Agent{{SessionID: "s", Cwd: "/repo/w/x", Status: "busy", PID: 7}}
	b.ApplyAgents(busy, nil, at(2*time.Minute))
	if out := n2.ProcessView(b.Snapshot(at(2*time.Minute)), nil, at(2*time.Minute)); len(out) != 0 {
		t.Fatalf("busy: %+v", out)
	}
	b.ApplyAgents(waiting, nil, at(3*time.Minute))
	b.ApplyAgents(waiting, nil, at(4*time.Minute+5*time.Second))
	if out := n2.ProcessView(b.Snapshot(at(4*time.Minute+5*time.Second)), nil, at(4*time.Minute+5*time.Second)); len(out) != 1 {
		t.Fatalf("a wait begun after the restart: %d notices, want 1", len(out))
	}
}

// stalledAt is a stalled alert of lane-a since since, as applyStalls raises it.
func stalledAt(since time.Time, approx bool) AlertView {
	return AlertView{Key: AlertStalled + ":/repo/w/a", Kind: AlertStalled, Severity: signals.SevBlock, Lane: "/repo/w/a",
		Name: "a", Terminal: "lane-a", Text: "stalled: its turn ended waiting on 1 shell: Run checks (running); open work: #152 open",
		Since: since.UnixMilli(), Approx: approx, Wait: "a shell in flight", OpenWork: []string{"#152 open"}}
}

// [STALL-3-S1] (the notice's part) A stalled alert goes out once per stretch, its
// notice carrying the stalled alert apart from a waiting alert grouped with it, and
// its line saying how long the lane has been quiet.
func TestNotifierCarriesTheStallApart(t *testing.T) {
	t.Parallel()
	n := &Notifier{MinInterval: time.Minute, Project: "P"}
	st := stalledAt(t0, false)
	wait := AlertView{Key: "waiting:s1", Kind: AlertWaiting, Severity: signals.SevBlock, Lane: "/repo/w/a", Name: "a", Terminal: "lane-a",
		Text: "waiting on you: wants to use Bash", Since: t0.UnixMilli()}
	out := n.Process([]AlertView{wait, st}, nil, t0.Add(30*time.Minute))
	if len(out) != 1 {
		t.Fatalf("a waiting and a stalled alert of one lane: %d notices, want 1", len(out))
	}
	if out[0].Stall == nil || out[0].Stall.Key != st.Key || out[0].Stall.Wait != "a shell in flight" {
		t.Fatalf("the notice's stall: %+v, want the stalled alert", out[0].Stall)
	}
	if !strings.Contains(out[0].Body, "stalled, quiet for 30m: its turn ended waiting on 1 shell") || !strings.Contains(out[0].Body, "wants to use Bash") {
		t.Errorf("the macOS body %q: want the stall's line with its quiet time, and the waiting alert's", out[0].Body)
	}
	for _, at := range []time.Duration{31 * time.Minute, 45 * time.Minute} {
		if got := n.Process([]AlertView{wait, st}, nil, t0.Add(at)); len(got) != 0 {
			t.Fatalf("still stalled at +%s: %+v, want nothing (once per stretch)", at, got)
		}
	}
	// A notice with no stall carries none.
	n2 := &Notifier{MinInterval: time.Minute}
	if got := n2.Process([]AlertView{wait}, nil, t0.Add(2*time.Minute)); len(got) != 1 || got[0].Stall != nil {
		t.Fatalf("a waiting alert alone: %+v, want one notice with no stall", got)
	}
}

// [STALL-3-S2] A lane that stalls while its terminal has keyboard focus in a visible
// page: no notice (so no push: the push rides on the notice), and the stall counts as
// seen, so losing focus sends nothing later.
func TestNotifierFocusedStallIsSeen(t *testing.T) {
	t.Parallel()
	n := &Notifier{MinInterval: time.Minute}
	st := stalledAt(t0, false)
	if out := n.Process([]AlertView{st}, map[string]bool{"lane-a": true}, t0.Add(30*time.Minute)); len(out) != 0 {
		t.Fatalf("focused: %+v, want no notice", out)
	}
	if n.Stats().Suppressed != 1 {
		t.Errorf("suppressed %d, want 1", n.Stats().Suppressed)
	}
	if since, ok := n.State().Notified[st.Key]; !ok || since != st.Since {
		t.Errorf("the focused stall's mark is %d (held %v), want %d: seen", since, ok, st.Since)
	}
	if out := n.Process([]AlertView{st}, nil, t0.Add(31*time.Minute)); len(out) != 0 {
		t.Fatalf("after focus is lost: %+v, want nothing", out)
	}
}

// [STALL-3-S3] An approximate stall never interrupts: no notice, so no push.
func TestNotifierApproxStallNeverInterrupts(t *testing.T) {
	t.Parallel()
	n := &Notifier{MinInterval: time.Minute}
	st := stalledAt(t0, true)
	for _, at := range []time.Duration{30 * time.Minute, 40 * time.Minute} {
		if out := n.Process([]AlertView{st}, nil, t0.Add(at)); len(out) != 0 {
			t.Fatalf("an approximate stall at +%s: %+v, want no notice", at, out)
		}
	}
	if n.Stats().Interrupts != 0 {
		t.Errorf("interrupts %d, want 0", n.Stats().Interrupts)
	}
}

// [STALL-3-S7] A restart while a notified stall is still active: while the view says
// stalls are pending (its sources not yet answered), the stall is missing from the
// alerts and its mark is kept, not deleted as cleared; once judged with the same
// since, nothing is sent. With nothing pending, a stall gone from the alerts clears.
func TestNotifierKeepsStalledMarksWhilePending(t *testing.T) {
	t.Parallel()
	st := stalledAt(t0, false)
	first := &Notifier{MinInterval: time.Minute}
	if out := first.Process([]AlertView{st}, nil, t0.Add(30*time.Minute)); len(out) != 1 {
		t.Fatalf("the first panel: %d notices, want 1", len(out))
	}
	saved := first.State()

	n := &Notifier{MinInterval: time.Minute}
	n.Restore(saved)
	n.StallsPending = true
	for i := 0; i < 3; i++ {
		if out := n.Process(nil, nil, t0.Add(time.Hour+time.Duration(i)*2*time.Second)); len(out) != 0 {
			t.Fatalf("pending tick %d: %+v", i, out)
		}
		if since, ok := n.State().Notified[st.Key]; !ok || since != st.Since {
			t.Fatalf("pending tick %d: the stall's mark is %d (held %v), want %d", i, since, ok, st.Since)
		}
	}
	n.StallsPending = false
	if out := n.Process([]AlertView{st}, nil, t0.Add(time.Hour+time.Minute)); len(out) != 0 {
		t.Fatalf("judged again with the same since: %+v, want nothing sent", out)
	}
	// The premise: with nothing pending, the stall gone clears its mark.
	n.Process(nil, nil, t0.Add(time.Hour+2*time.Minute))
	if _, ok := n.State().Notified[st.Key]; ok {
		t.Fatal("with nothing pending, a stall gone from the alerts kept its mark")
	}
}
