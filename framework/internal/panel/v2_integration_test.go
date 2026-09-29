package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// v2Runner runs git and plain commands for real and fakes claude and gh. `claude
// agents` lists every registered lane whose tmux session exists, as idle, unless
// hidden is set: then it lists none, which is what a session held at the
// workspace-trust dialog looks like (verified on 2.1.284).
func v2Runner(tmux, sock, home, root string, hidden *atomic.Bool, waiting ...*atomic.Bool) signals.Runner {
	return func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		switch {
		case argv[0] == "gh":
			return []byte("[]"), nil
		case argv[0] == "claude" && len(argv) > 1 && argv[1] == "--version":
			return []byte("2.1.284 (Claude Code)\n"), nil
		case argv[0] == "claude" && len(argv) > 1 && argv[1] == "agents":
			if hidden.Load() {
				return []byte("[]"), nil
			}
			var f registryFile
			b, _ := os.ReadFile(lanes.RegistryPath(home, root))
			_ = json.Unmarshal(b, &f)
			var out []signals.Agent
			for _, l := range f.Lanes {
				if exec.Command(tmux, "-L", sock, "has-session", "-t", "="+l.ID).Run() == nil {
					a := signals.Agent{PID: 1, Cwd: l.Path, Kind: "interactive", SessionID: l.SessionID, Name: l.ID, Status: "idle"}
					if len(waiting) > 0 && waiting[0].Load() {
						a.Status, a.WaitingFor = "waiting", "permission prompt"
					}
					out = append(out, a)
				}
			}
			j, _ := json.Marshal(out)
			return j, nil
		case argv[0] == "claude":
			return nil, fmt.Errorf("unexpected %v", argv)
		}
		return signals.ExecRunner(ctx, dir, argv)
	}
}

func v2Project(t *testing.T, cfg string) (root, home string) {
	t.Helper()
	root = signals.ResolvePath(t.TempDir())
	home = t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), cfg)
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	gitRun(t, root, "remote", "add", "origin", filepath.Join(root, "no-such-remote"))
	return root, home
}

func lineCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(b)))
}

func termByID(v View, id string) *TermLaneView {
	for i := range v.Terminals {
		if v.Terminals[i].ID == id {
			return &v.Terminals[i]
		}
	}
	return nil
}

func TestTemplateLaneGetsItsFirstPromptOnceWhenReady(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	marker := filepath.Join(t.TempDir(), "typed")
	cfg := `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},"base":"main","worktree_dir":".wt",
		"templates":[{"id":"fix","title":"Fix","lane_type":"fix","branch_pattern":"fix/{name}",
		"first_prompt":"echo prompt-{name}-{issue} >> ` + marker + `"}],
		"alerts":{"idle_minutes":0.02,"waiting_seconds":1,"min_interval_seconds":600},"quota_guard":{"five_hour_pct":90}}`
	root, home := v2Project(t, cfg)
	var hidden, waiting atomic.Bool
	hidden.Store(true) // claude "is at the trust dialog": not in claude agents yet
	var mu sync.Mutex
	var notices []Notice
	tweak := func(o *Options) {
		o.Runner = v2Runner(tmux, sock, home, root, &hidden, &waiting)
		o.Notify = func(n Notice) error { mu.Lock(); notices = append(notices, n); mu.Unlock(); return nil }
	}
	p := startPanelWith(t, root, home, sock, tweak)

	// Placeholders are validated server-side: a newline in the issue is refused.
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Template: "fix", Name: "bug", Issue: "7\n/exit"}); code != 400 {
		t.Fatalf("newline in the issue: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Template: "fix", Name: "bug", Issue: "7"}); code != 200 {
		t.Fatalf("start template lane: %d %v", code, body)
	}
	if b := gitRun(t, root, "branch", "--list", "fix/bug"); !strings.Contains(b, "fix/bug") {
		t.Fatalf("the template's branch was not created: %q", b)
	}
	// Not listed in claude agents: nothing may be typed, however long it takes.
	time.Sleep(2500 * time.Millisecond)
	if n := lineCount(marker); n != 0 {
		t.Fatalf("typed %d time(s) before claude was ready", n)
	}
	if tv := termByID(p.state(t), "bug"); tv == nil || tv.PromptState != "pending" || tv.Template != "fix" {
		t.Fatalf("pending prompt not shown: %+v", tv)
	}
	hidden.Store(false) // claude is idle now
	waitFor(t, "the first prompt to run", func() bool { return lineCount(marker) == 1 })
	b, _ := os.ReadFile(marker)
	if strings.TrimSpace(string(b)) != "prompt-bug-7" {
		t.Fatalf("typed %q", b)
	}
	waitFor(t, "state sent", func() bool { tv := termByID(p.state(t), "bug"); return tv != nil && tv.PromptState == "sent" })
	time.Sleep(3 * time.Second)
	if n := lineCount(marker); n != 1 {
		t.Fatalf("the first prompt was typed %d times", n)
	}

	// The idle alert (threshold 1.2 s) shows on the page but never interrupts.
	waitFor(t, "the idle alert", func() bool {
		for _, a := range p.state(t).Alerts {
			if a.Kind == AlertIdle {
				return true
			}
		}
		return false
	})
	time.Sleep(2500 * time.Millisecond)
	mu.Lock()
	if len(notices) != 0 {
		t.Fatalf("an idle alert interrupted: %+v", notices)
	}
	mu.Unlock()
	// A permission prompt older than 1 s blocks the lane: one notification.
	waiting.Store(true)
	waitUntil(t, "a waiting notification", 10*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return len(notices) > 0 })
	time.Sleep(3 * time.Second)
	mu.Lock()
	if len(notices) != 1 || !strings.Contains(notices[0].Body, "waiting") || !strings.Contains(notices[0].Title, "bug") {
		t.Fatalf("notices %+v", notices)
	}
	mu.Unlock()
	if v := p.state(t); v.Observe.Notifier.Interrupts != 1 || v.Observe.ClaudeVersion != "2.1.284" {
		t.Fatalf("observability: %+v", v.Observe)
	}
	// Restart the panel with the alert still active: no second notification.
	p.stop()
	p = startPanelWith(t, root, home, sock, tweak)
	waitFor(t, "the alert after the restart", func() bool {
		for _, a := range p.state(t).Alerts {
			if a.Kind == AlertWaiting {
				return true
			}
		}
		return false
	})
	time.Sleep(4 * time.Second)
	mu.Lock()
	if len(notices) != 1 {
		t.Fatalf("the restart re-notified: %+v", notices)
	}
	mu.Unlock()
	if v := p.state(t); v.Observe.Notifier.Interrupts != 1 {
		t.Fatalf("interrupt count after restart: %+v", v.Observe.Notifier)
	}
	waiting.Store(false)

	// Quota guard: at 95% (guard 90) a lane is refused unless overridden.
	hi, resets := 95.0, time.Now().Add(time.Hour).Unix()
	st := map[string]any{"session_id": "x", "cwd": root, "rate_limits": map[string]any{"five_hour": map[string]any{"used_percentage": hi, "resets_at": resets}}}
	sb, _ := json.Marshal(st)
	if code := (liveClient{base: p.base}).post(t, "/status", string(sb)); code != 204 {
		t.Fatalf("status post %d", code)
	}
	waitFor(t, "the quota to land", func() bool { return p.state(t).QuotaGuard != "" })
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 409 || body["code"] != "quota" {
		t.Fatalf("quota guard: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch", OverrideQuota: true}); code != 200 {
		t.Fatalf("override: %d %v", code, body)
	}

	// A reboot, in miniature: the tmux server dies. Both lanes become restorable, and
	// RESTORE ALL brings both back on their own session ids.
	exec.Command(tmux, "-L", sock, "kill-server").Run()
	waitFor(t, "restorable lanes", func() bool { return len(p.state(t).Restorable) == 2 })
	code, body := p.post(t, "/api/lanes/restore-all", map[string]any{"overrideQuota": true})
	if code != 200 {
		t.Fatalf("restore-all: %d %v", code, body)
	}
	res, _ := body["result"].(map[string]any)
	if r, _ := res["restored"].([]any); len(r) != 2 {
		t.Fatalf("restored %v", body)
	}
	if !hasSession(tmux, sock, "bug") || !hasSession(tmux, sock, "orch") {
		t.Fatal("lanes not back in tmux")
	}
	// Restoring again restores nothing: every lane is running.
	_, body = p.post(t, "/api/lanes/restore-all", map[string]any{"overrideQuota": true})
	if r, _ := body["result"].(map[string]any)["restored"].([]any); len(r) != 0 {
		t.Fatalf("second restore restored %v", body)
	}
	// And the restored template lane is never typed into again.
	time.Sleep(2 * time.Second)
	if n := lineCount(marker); n != 1 {
		t.Fatalf("restore retyped the first prompt (%d)", n)
	}
}

func TestUntrustedConfigRunsNoCommandsOrTemplates(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	cfg := `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},"base":"main","worktree_dir":".wt",
		"cards":[{"id":"c","command":["echo","card-ran"],"refresh":"interval:60"}],
		"templates":[{"id":"fix","lane_type":"fix","first_prompt":"hello"}]}`
	root, home := v2Project(t, cfg)
	var hidden atomic.Bool
	run := func() *panelRun {
		return startPanelWith(t, root, home, sock, func(o *Options) { o.Runner = v2Runner(tmux, sock, home, root, &hidden) })
	}
	cardOK := func(p *panelRun) (bool, string) {
		v := p.state(t)
		return v.Cards[0].Source.OK, v.Cards[0].Source.Error
	}
	// First run: trusted on first use, the card runs.
	p := run()
	waitFor(t, "the card", func() bool { ok, _ := cardOK(p); return ok })
	if !p.state(t).Trust.Trusted {
		t.Fatal("first use not trusted")
	}
	p.stop()
	// The config changes (a pull): the card and templates are off, loudly.
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), strings.Replace(cfg, "card-ran", "card-changed", 1))
	p = run()
	waitFor(t, "the untrusted card", func() bool { _, e := cardOK(p); return strings.Contains(e, "not run") })
	v := p.state(t)
	if v.Trust.Trusted || !strings.Contains(strings.Join(v.Banners, " "), "changed since you trusted it") {
		t.Fatalf("untrusted state: %+v %v", v.Trust, v.Banners)
	}
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Template: "fix", Name: "x"}); code != 409 || body["code"] != "untrusted-config" {
		t.Fatalf("template under an untrusted config: %d %v", code, body)
	}
	// `clauductor panel trust` lifts it in the running panel.
	if _, err := TrustConfig(home, root, ""); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the card after trust", 10*time.Second, func() bool { ok, _ := cardOK(p); return ok })
	if !p.state(t).Trust.Trusted {
		t.Fatal("still untrusted")
	}
}
