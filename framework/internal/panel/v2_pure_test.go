package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func v2Config(t *testing.T, extra string) *Config {
	t.Helper()
	c, err := ParseConfig([]byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}` + extra + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const tplJSON = `,"templates":[
 {"id":"build","title":"Build a change","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"Run build-change for {name}","model":"opus","effort":"high"},
 {"id":"fix","title":"Fix an issue","lane_type":"fix","first_prompt":"Fix issue {issue} on branch fix/{name}"}]`

func TestTemplateRendering(t *testing.T) {
	c := v2Config(t, tplJSON)
	r, err := c.RenderTemplate("build", "add-x", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Branch != "change/add-x" || r.FirstPrompt != "Run build-change for add-x" || r.Model != "opus" || r.LaneType != "build" {
		t.Fatalf("%+v", r)
	}
	r, err = c.RenderTemplate("fix", "login-bug", " #412 ")
	if err != nil {
		t.Fatal(err)
	}
	if r.Branch != "fix/login-bug" || r.FirstPrompt != "Fix issue #412 on branch fix/login-bug" {
		t.Fatalf("default prefix and trimmed issue: %+v", r)
	}
	bad := []struct{ tpl, name, issue, why string }{
		{"fix", "x", "412\n/exit", "a newline would submit early"},
		{"fix", "x", "4\r12", "an embedded carriage return"},
		{"fix", "x", "\x1b[2J", "an escape sequence drives the TUI"},
		{"fix", "x", "a‮b", "a bidi override hides text"},
		{"fix", "x", " ", "a line separator"},
		{"fix", "x", "", "the template needs an issue"},
		{"fix", "x", strings.Repeat("a", 201), "too long"},
		{"build", "x", "1", "the template takes no issue"},
		{"build", "Bad Name", "", "the name is a lane id"},
		{"build", "-x", "", "a leading dash"},
		{"nope", "x", "", "unknown template"},
	}
	for _, b := range bad {
		if _, err := c.RenderTemplate(b.tpl, b.name, b.issue); err == nil {
			t.Errorf("accepted %q/%q (%s)", b.name, b.issue, b.why)
		}
	}
}

func TestTemplateConfigValidation(t *testing.T) {
	bad := map[string]string{
		"unknown placeholder":   `,"templates":[{"id":"a","lane_type":"build","first_prompt":"do {thing}"}]`,
		"newline in the prompt": `,"templates":[{"id":"a","lane_type":"build","first_prompt":"line one\nline two"}]`,
		"escape in the prompt":  `,"templates":[{"id":"a","lane_type":"build","first_prompt":"x\u001b[31m"}]`,
		"unknown lane type":     `,"templates":[{"id":"a","lane_type":"nope","first_prompt":"x"}]`,
		"no {name} in pattern":  `,"templates":[{"id":"a","lane_type":"build","branch_pattern":"change/fixed","first_prompt":"x"}]`,
		"no prefix, no pattern": `,"templates":[{"id":"a","lane_type":"orchestrator","first_prompt":"x"}]`,
		"bad model":             `,"templates":[{"id":"a","lane_type":"build","first_prompt":"x","model":"--dangerously"}]`,
		"duplicate id":          `,"templates":[{"id":"a","lane_type":"build","first_prompt":"x"},{"id":"a","lane_type":"build","first_prompt":"y"}]`,
		"escaping lock":         `,"queues":[{"id":"g","lock":"../outside"}]`,
		"absolute lock":         `,"queues":[{"id":"g","lock":"/tmp/x"}]`,
		"empty command":         `,"queues":[{"id":"g","lock":"clauductor/gate.lock","command":[]}]`,
		"negative threshold":    `,"alerts":{"idle_minutes":-1}`,
		"guard above 100":       `,"quota_guard":{"five_hour_pct":101}`,
		"unknown alert key":     `,"alerts":{"idle":5}`,
		"future version":        `,"version":3`,
	}
	for why, extra := range bad {
		if _, err := ParseConfig([]byte(`{"name":"T","lanes":{"change/":"build","main":"orchestrator"}` + extra + `}`)); err == nil {
			t.Errorf("accepted: %s", why)
		}
	}
	c := v2Config(t, `,"version":2,"queues":[{"id":"gate","title":"Gate","lock":"clauductor/gate.lock","command":["bash","infra/ci/run-local.sh"]}],"alerts":{"idle_minutes":0.5,"context_pct":0},"quota_guard":{"five_hour_pct":0}`)
	th := c.AlertThresholds()
	if th.Idle != 30*time.Second || th.ContextPct != 0 || th.FiveHourPct != DefaultFiveHourPct || !th.Notify || th.GuardPct != 0 ||
		th.MinInterval != DefaultNotifyInterval*time.Second || th.Waiting != DefaultWaitingSeconds*time.Second {
		t.Fatalf("thresholds %+v", th)
	}
}

func TestFirstPromptDecision(t *testing.T) {
	start := t0
	tests := []struct {
		name string
		in   PromptInput
		at   time.Duration
		want string
	}{
		{"idle in claude agents: type it", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", Since: start}, time.Second, "send"},
		// Held at the trust dialog, the session is not listed at all (2.1.284).
		{"not listed yet: wait", PromptInput{State: "pending", Running: true, Since: start}, 5 * time.Second, "wait"},
		{"not listed for long: ask the human", PromptInput{State: "pending", Running: true, Since: start}, readyGrace, "stuck"},
		{"listed but busy: wait", PromptInput{State: "pending", Running: true, Listed: true, Status: "busy", Since: start}, time.Second, "wait"},
		{"listed but waiting (a dialog): wait", PromptInput{State: "pending", Running: true, Listed: true, Status: "waiting", Since: start}, time.Second, "wait"},
		{"you typed first: skip", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", Prompted: start, Since: start}, time.Second, "skip"},
		{"claude exited: stuck", PromptInput{State: "pending", Running: true, Dead: true, Since: start}, time.Second, "stuck"},
		{"lane down: wait", PromptInput{State: "pending", Since: start}, time.Hour, "wait"},
		{"panel died mid-typing: never again", PromptInput{State: "typing", Running: true, Listed: true, Status: "idle", Since: start}, time.Second, "stuck"},
		{"typed and submitted", PromptInput{State: "sent", Running: true, Prompted: start.Add(time.Second), PromptAt: start}, 2 * time.Second, "delivered"},
		{"typed, busy", PromptInput{State: "sent", Running: true, Listed: true, Status: "busy", PromptAt: start}, 2 * time.Second, "delivered"},
		{"typed, nothing yet", PromptInput{State: "sent", Running: true, Listed: true, Status: "idle", PromptAt: start}, 5 * time.Second, "wait"},
		{"typed, never landed", PromptInput{State: "sent", Running: true, Listed: true, Status: "idle", PromptAt: start}, confirmGrace, "stuck"},
		{"done", PromptInput{State: "delivered"}, 0, "none"},
	}
	for _, tc := range tests {
		if got := DecideFirstPrompt(tc.in, start.Add(tc.at)); got.Action != tc.want {
			t.Errorf("%s: got %s (%s), want %s", tc.name, got.Action, got.Why, tc.want)
		}
	}
}

func TestSelectRestorable(t *testing.T) {
	sid := func(n byte) string { return "11111111-1111-4111-8111-11111111111" + string(n) }
	recs := []LaneRecord{
		{ID: "a", SessionID: sid('a'), Path: "/ok"},
		{ID: "b", SessionID: sid('b'), Path: "/ok"},   // running: not lost
		{ID: "c", SessionID: sid('c'), Path: "/ok"},   // live in another process
		{ID: "d", SessionID: sid('a'), Path: "/ok"},   // same session as a
		{ID: "e", SessionID: sid('e'), Path: "/gone"}, // worktree removed
		{ID: "f", SessionID: "not-a-uuid", Path: "/ok"},
	}
	pick, skip := SelectRestorable(recs, map[string]bool{"b": true}, map[string]bool{sid('c'): true},
		func(p string) bool { return p == "/ok" })
	if len(pick) != 1 || pick[0].ID != "a" {
		t.Fatalf("pick %+v", pick)
	}
	why := map[string]string{}
	for _, s := range skip {
		why[s.ID] = s.Reason
	}
	for id, want := range map[string]string{"c": "already runs", "d": "restored once only", "e": "no longer exists", "f": "no valid session id"} {
		if !strings.Contains(why[id], want) {
			t.Errorf("%s: skip reason %q, want %q", id, why[id], want)
		}
	}
	if _, ok := why["b"]; ok {
		t.Error("a running lane is not a skip, it is not lost")
	}
}

// ---- alerts and the notifier ----

func alertModel(t *testing.T, cfgExtra string) *Model {
	t.Helper()
	m := NewModel(v2Config(t, cfgExtra), "/repo", t0)
	m.ApplyWorktrees([]Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}, nil, t0)
	return m
}

func alertKinds(v View) map[string]string {
	out := map[string]string{}
	for _, a := range v.Alerts {
		out[a.Kind] = a.Severity
	}
	return out
}

func TestAlertThresholds(t *testing.T) {
	m := alertModel(t, `,"alerts":{"idle_minutes":10,"context_pct":80,"five_hour_pct":90,"waiting_seconds":60}`)
	cwd := "/repo/w/x"
	m.ApplyAgents([]Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0)
	ctx, five := 79.0, 89.0
	p := StatusPayload{SessionID: "s", Cwd: cwd}
	p.ContextWindow.UsedPercentage = &ctx
	p.RateLimits.FiveHour = &RateLimit{UsedPercentage: &five}
	m.ApplyStatus(p, t0)
	// Just under every threshold: nothing.
	m.ApplyAgents([]Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(9*time.Minute))
	if v := m.Snapshot(t0.Add(9 * time.Minute)); len(v.Alerts) != 0 {
		t.Fatalf("under thresholds: %+v", v.Alerts)
	}
	ctx, five = 80, 90
	m.ApplyStatus(p, t0.Add(10*time.Minute))
	m.ApplyAgents([]Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(10*time.Minute))
	k := alertKinds(m.Snapshot(t0.Add(10 * time.Minute)))
	if k[AlertIdle] != SevInfo || k[AlertContext] != SevWarn || k[AlertQuota] != SevWarn {
		t.Fatalf("at thresholds: %+v", k)
	}
	// Waiting: a permission prompt older than 60 s.
	m.ApplyAgents([]Agent{{SessionID: "s", Cwd: cwd, Status: "waiting", WaitingFor: "permission prompt"}}, nil, t0.Add(11*time.Minute))
	if k := alertKinds(m.Snapshot(t0.Add(11*time.Minute + 59*time.Second))); k[AlertWaiting] != "" {
		t.Fatalf("waiting fired early: %+v", k)
	}
	if k := alertKinds(m.Snapshot(t0.Add(12 * time.Minute))); k[AlertWaiting] != SevBlock {
		t.Fatalf("waiting did not fire: %+v", k)
	}
	// A 0 threshold is off.
	off := alertModel(t, `,"alerts":{"idle_minutes":0,"context_pct":0,"five_hour_pct":0,"waiting_seconds":0}`)
	off.ApplyAgents([]Agent{{SessionID: "s", Cwd: cwd, Status: "waiting"}}, nil, t0)
	ctx, five = 99, 99
	off.ApplyStatus(p, t0)
	if v := off.Snapshot(t0.Add(time.Hour)); len(v.Alerts) != 0 {
		t.Fatalf("disabled alerts fired: %+v", v.Alerts)
	}
	// quota_auto_resume_stale: it will not continue by itself.
	m.ApplyHook(HookEvent{SessionID: "s", Cwd: cwd, Event: "Notification", NotificationType: "quota_auto_resume_stale"}, t0)
	if k := alertKinds(m.Snapshot(t0.Add(12 * time.Minute))); k[AlertNoAutoResume] != SevWarn {
		t.Fatalf("no auto-resume alert: %+v", k)
	}
}

func TestNotifierRateLimitGroupingFocusAndCounter(t *testing.T) {
	n := &Notifier{MinInterval: 5 * time.Minute, Project: "P"}
	a1 := AlertView{Key: "idle:s1", Kind: AlertIdle, Terminal: "lane-a", Name: "lane-a", Text: "idle for 10m"}
	a2 := AlertView{Key: "context:s1", Kind: AlertContext, Terminal: "lane-a", Name: "lane-a", Text: "context at 90%"}
	b1 := AlertView{Key: "waiting:s2", Kind: AlertWaiting, Terminal: "lane-b", Name: "lane-b", Text: "waiting"}
	// Two alerts of one lane: ONE notification.
	out := n.Process([]AlertView{a1, a2}, nil, t0)
	if len(out) != 1 || out[0].Title != "P · lane-a" || !strings.Contains(out[0].Body, "idle") || !strings.Contains(out[0].Body, "context") {
		t.Fatalf("grouping: %+v", out)
	}
	// The same alerts again: nothing (once per stretch).
	if out := n.Process([]AlertView{a1, a2}, nil, t0.Add(time.Minute)); len(out) != 0 {
		t.Fatalf("repeated: %+v", out)
	}
	// A new alert on the same lane inside the interval waits.
	a3 := AlertView{Key: "rate_limit:s1", Kind: AlertRateLimit, Terminal: "lane-a", Name: "lane-a", Text: "rate limit"}
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
	hostile := `x" & (do shell script "touch /tmp/pwned") & "`
	argv := NotifyArgv(hostile, hostile+"\n"+hostile)
	var script []string
	for i := 1; i < len(argv)-2; i += 2 {
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
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("/usr/bin/osascript"); err != nil {
		t.Skip("no osascript")
	}
	marker := filepath.Join(t.TempDir(), "pwned")
	hostile := `a" & (do shell script "touch ` + marker + `") & "b` + "\\\" ' ¬ «data» end run"
	argv := osascriptArgv([]string{"return item 2 of argv"}, "title", hostile)
	out, err := exec.Command(argv[0], argv[1:]...).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSuffix(string(out), "\n"); got != hostile {
		t.Fatalf("round trip:\n got %q\nwant %q", got, hostile)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the argument was executed")
	}
}

func TestConfigTrust(t *testing.T) {
	home, root := t.TempDir(), ResolvePath(t.TempDir())
	cfg := filepath.Join(root, "panel.json")
	tv, err := CheckTrust(home, root, cfg, "h1", false)
	if err != nil || !tv.Trusted || tv.Note == "" {
		t.Fatalf("first use is trusted and recorded: %+v %v", tv, err)
	}
	if tv, _ := CheckTrust(home, root, cfg, "h1", false); !tv.Trusted {
		t.Fatal("unchanged config untrusted")
	}
	tv, _ = CheckTrust(home, root, cfg, "h2", false)
	if tv.Trusted || tv.Prev != "h1" {
		t.Fatalf("a changed config must not be trusted silently: %+v", tv)
	}
	if trustedNow(home, root, cfg, "h2") {
		t.Fatal("an untrusted check recorded the new hash")
	}
	if tv, _ := CheckTrust(home, root, cfg, "h2", true); !tv.Trusted || !trustedNow(home, root, cfg, "h2") {
		t.Fatalf("--trust-config: %+v", tv)
	}
	fi, err := os.Stat(TrustPath(home, root))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("trust file mode: %v %v", fi, err)
	}
	// The hash covers the exact bytes.
	if ConfigHash([]byte("a")) == ConfigHash([]byte("a ")) {
		t.Fatal("hash ignores bytes")
	}
}
