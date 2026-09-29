package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConfigExampleFixtureParses(t *testing.T) {
	cfg := testConfig(t)
	if cfg.Name != "My Project" || len(cfg.Cards) != 2 {
		t.Fatalf("%+v", cfg)
	}
}

func TestConfigRejects(t *testing.T) {
	tests := map[string]string{
		"missing name":        `{"lanes":{}}`,
		"unknown key":         `{"name":"x","lane":{}}`,
		"bad card id":         `{"name":"x","cards":[{"id":"Bad Id","command":["true"],"refresh":"interval:60"}]}`,
		"duplicate card id":   `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"interval:60"},{"id":"a","command":["true"],"refresh":"interval:60"}]}`,
		"empty command":       `{"name":"x","cards":[{"id":"a","command":[],"refresh":"interval:60"}]}`,
		"string command":      `{"name":"x","cards":[{"id":"a","command":"ls -la","refresh":"interval:60"}]}`,
		"bad refresh":         `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"every:60"}]}`,
		"watch escapes repo":  `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"watch:../x"}]}`,
		"watch absolute path": `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"watch:/etc/passwd"}]}`,
		"zero interval":       `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"interval:0"}]}`,
		"empty lane type":     `{"name":"x","lanes":{"fix/":""}}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig([]byte(raw)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestRefreshIntervalFloor(t *testing.T) {
	r, err := ParseRefresh("interval:1")
	if err != nil || r.Interval != minCardInterval {
		t.Fatalf("%v %v", r, err)
	}
	r, _ = ParseRefresh("watch:docs/./founder-queue.md")
	if r.WatchRel != "docs/founder-queue.md" {
		t.Fatalf("%q", r.WatchRel)
	}
}

func TestLaneFor(t *testing.T) {
	cfg := &Config{Name: "x", Lanes: map[string]string{
		"change/": "build", "change/propose-*": "propose", "fix/": "fix", "main": "orchestrator",
	}}
	tests := []struct{ branch, typ, name string }{
		{"change/add-x", "build", "add-x"},
		{"change/propose-2c24", "propose", "2c24"}, // longest rule wins
		{"fix/bug", "fix", "bug"},
		{"main", "orchestrator", "main"},
		{"maintenance", "other", "maintenance"}, // exact rules are not prefixes
		{"feature/y", "other", "feature/y"},
		{"", "detached", "(detached)"},
	}
	for _, tc := range tests {
		typ, name := cfg.LaneFor(tc.branch)
		if typ != tc.typ || name != tc.name {
			t.Errorf("%q → %s/%s, want %s/%s", tc.branch, typ, name, tc.typ, tc.name)
		}
	}
}

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
		if _, err := parseConfig([]byte(`{"name":"T","lanes":{"change/":"build","main":"orchestrator"}` + extra + `}`)); err == nil {
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

// The config name reaches notification titles: one line, no leading dash.
func TestConfigNameIsPlainText(t *testing.T) {
	for _, bad := range []string{`-eproperty p : 1`, " -x", "a\nb", "a‮b", "\x1b[31m"} {
		b, _ := json.Marshal(map[string]any{"name": bad})
		if _, err := parseConfig(b); err == nil {
			t.Errorf("accepted name %q", bad)
		}
	}
	if _, err := parseConfig([]byte(`{"name":"My Project · panel"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestHostNamesConfig(t *testing.T) {
	for _, bad := range []string{"*.localhost", "evil.com", "a.b.localhost", "UPPER.localhost", "localhost", ".localhost", "-a.localhost", "a_b.localhost"} {
		if _, err := parseConfig([]byte(`{"name":"T","host_names":["` + bad + `"]}`)); err == nil {
			t.Errorf("accepted host name %q", bad)
		}
	}
	if _, err := parseConfig([]byte(`{"name":"T","host_names":["myproject.localhost","a1-b2.localhost"]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRejectsBadLaneKeys(t *testing.T) {
	for _, body := range []string{
		`{"name":"x","tmux_socket":"a b"}`,
		`{"name":"x","tmux_socket":"../x"}`,
		`{"name":"x","base":"-x"}`,
		`{"name":"x","base":"a b"}`,
		`{"name":"x","worktree_dir":"../outside"}`,
		`{"name":"x","worktree_dir":"."}`,
		`{"name":"x","lane_types":{"build":{"model":"opus; rm -rf /"}}}`,
		`{"name":"x","lane_types":{"build":{"effort":"--dangerously-skip-permissions"}}}`,
		`{"name":"x","lane_types":{"build":{"modle":"opus"}}}`,
	} {
		if _, err := parseConfig([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if _, err := parseConfig([]byte(`{"name":"x","tmux_socket":"myproject","base":"origin/main","worktree_dir":"/abs/wt",
		"lane_types":{"build":{"model":"claude-opus-4-5[1m]","effort":"high"}}}`)); err != nil {
		t.Fatal(err)
	}
}
