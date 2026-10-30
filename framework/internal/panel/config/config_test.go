package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// [CATCHUP-1-S4] A repository cannot turn catch-up on for itself: a panel.json that
// sets a catch_up key fails to load, and the error names the key. The switch is the
// machine's catch-up.json alone (OPS-118 D2).
func TestConfigRefusesCatchUpKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","catch_up":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatalf("loaded %+v, want the load to fail", cfg)
	}
	if !strings.Contains(err.Error(), `"catch_up"`) {
		t.Fatalf("error %q does not name the key", err)
	}
}

func TestConfigExampleFixtureParses(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	if cfg.Name != "My Project" || len(cfg.Cards) != 2 {
		t.Fatalf("%+v", cfg)
	}
}

func TestConfigRejects(t *testing.T) {
	t.Parallel()
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
		"suggest no command":  `{"name":"x","lanes":{"f/":"b"},"templates":[{"id":"a","lane_type":"b","first_prompt":"p","suggest":{"command":[],"refresh":"interval:60"}}]}`,
		"suggest bad refresh": `{"name":"x","lanes":{"f/":"b"},"templates":[{"id":"a","lane_type":"b","first_prompt":"p","suggest":{"command":["true"],"refresh":"watch:../x"}}]}`,
		"suggest unknown key": `{"name":"x","lanes":{"f/":"b"},"templates":[{"id":"a","lane_type":"b","first_prompt":"p","suggest":{"command":["true"],"refresh":"interval:60","shell":true}}]}`,
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
		"empty command":         `,"queues":[{"id":"g","lock":"stridal/gate.lock","command":[]}]`,
		"negative threshold":    `,"alerts":{"idle_minutes":-1}`,
		"guard above 100":       `,"quota_guard":{"five_hour_pct":101}`,
		"unknown alert key":     `,"alerts":{"idle":5}`,
		"future version":        `,"version":8`,
		"metrics refresh alone": `,"metrics":{"refresh":"interval:60"}`,
		"metrics empty command": `,"metrics":{"command":[]}`,
		"metrics bad refresh":   `,"metrics":{"command":["true"],"refresh":"interval:0"}`,
		"metrics unknown key":   `,"metrics":{"cmd":["true"]}`,
	}
	for why, extra := range bad {
		if _, err := parseConfig([]byte(`{"name":"T","lanes":{"change/":"build","main":"orchestrator"}` + extra + `}`)); err == nil {
			t.Errorf("accepted: %s", why)
		}
	}
	c := v2Config(t, `,"version":2,"queues":[{"id":"gate","title":"Gate","lock":"stridal/gate.lock","command":["bash","infra/ci/run-local.sh"]}],"alerts":{"idle_minutes":0.5,"context_pct":0},"quota_guard":{"five_hour_pct":0}`)
	th := c.AlertThresholds()
	if th.Idle != 30*time.Second || th.ContextPct != 0 || th.FiveHourPct != DefaultFiveHourPct || !th.Notify || th.GuardPct != 0 ||
		th.MinInterval != DefaultNotifyInterval*time.Second || th.Waiting != DefaultWaitingSeconds*time.Second {
		t.Fatalf("thresholds %+v", th)
	}
}

// [STALL-1-S8] The stall time is the project's: 30 minutes when alerts.stall_minutes
// is absent (alerts absent too), the number it sets, 0 for off; a negative is refused.
func TestStallMinutes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		extra string
		want  time.Duration
	}{
		{``, 30 * time.Minute},
		{`,"alerts":{"idle_minutes":5}`, 30 * time.Minute},
		{`,"alerts":{"stall_minutes":10}`, 10 * time.Minute},
		{`,"alerts":{"stall_minutes":0.5}`, 30 * time.Second},
		{`,"alerts":{"stall_minutes":0}`, 0},
	} {
		th := v2Config(t, c.extra).AlertThresholds()
		if th.Stall != c.want || th.StallMinutes != float64(c.want)/float64(time.Minute) {
			t.Errorf("%q: stall %v (%v minutes), want %v", c.extra, th.Stall, th.StallMinutes, c.want)
		}
	}
	_, err := parseConfig([]byte(`{"name":"T","alerts":{"stall_minutes":-1}}`))
	if err == nil || !strings.Contains(err.Error(), "alerts.stall_minutes must be 0 (off) or positive") {
		t.Errorf("a negative stall_minutes: %v, want it refused by name", err)
	}
}

// The config name reaches notification titles: one line, no leading dash.
func TestConfigNameIsPlainText(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// [LANECTX-1-S7] A context window outside Claude Code's range does not load, and the
// error names the key and the range 100000–1000000.
func TestContextWindowOutsideTheRangeDoesNotLoad(t *testing.T) {
	t.Parallel()
	for _, r := range []struct{ body, key string }{
		{`"launch":{"context_window":99999}`, "launch.context_window"},
		{`"lane_types":{"propose":{"context_window":1000001}}`, "lane_types.propose.context_window"},
		// An explicit 0 is a window outside the range, not "no window".
		{`"launch":{"context_window":0}`, "launch.context_window"},
		{`"lane_types":{"fix":{"context_window":-200000}}`, "lane_types.fix.context_window"},
	} {
		_, err := parseConfig([]byte(`{"name":"T","version":6,` + r.body + `}`))
		if err == nil || !strings.Contains(err.Error(), r.key) || !strings.Contains(err.Error(), "100000–1000000") {
			t.Errorf("%s: %v, want an error naming %s and 100000–1000000", r.body, err, r.key)
		}
	}
	// The range's own ends load, and a lane type's window overrides launch's.
	c, err := parseConfig([]byte(`{"name":"T","version":6,"launch":{"context_window":100000},"lane_types":{"propose":{"context_window":1000000}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.ContextWindowOf("fix") != 100000 || c.ContextWindowOf("propose") != 1000000 {
		t.Errorf("windows fix %d, propose %d", c.ContextWindowOf("fix"), c.ContextWindowOf("propose"))
	}
	// subagent_model becomes one value in the lane's settings, held to LaunchOptRe as model is.
	for _, bad := range []string{`"--dangerously"`, `"a b"`, `"opus\"}"`} {
		if _, err := parseConfig([]byte(`{"name":"T","version":6,"lane_types":{"fix":{"subagent_model":` + bad + `}}}`)); err == nil ||
			!strings.Contains(err.Error(), "lane_types.fix.subagent_model") {
			t.Errorf("subagent_model %s: %v", bad, err)
		}
	}
}

// A template's suggest command is trusted like a card's: RunList names it, and the
// dialog is told the template has a list.
func TestTemplateSuggest(t *testing.T) {
	t.Parallel()
	c, err := parseConfig([]byte(`{"name":"x","version":3,"lanes":{"f/":"b"},"cards":[{"id":"c","command":["true"],"refresh":"interval:60","pin":true}],
		"templates":[{"id":"a","lane_type":"b","first_prompt":"p","suggest":{"command":["next","--suggest"],"refresh":"watch:docs"}},{"id":"z","lane_type":"b","first_prompt":"p"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Cards[0].Pin {
		t.Error("pin not read")
	}
	tl := c.TemplateList()
	if !tl[0].Suggests || tl[1].Suggests {
		t.Errorf("suggests %v %v", tl[0].Suggests, tl[1].Suggests)
	}
	if !strings.Contains(strings.Join(c.RunList(), "\n"), `template a suggests from ["next" "--suggest"] (watch:docs)`) {
		t.Errorf("RunList %q", c.RunList())
	}
}
