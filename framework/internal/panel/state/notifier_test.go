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
