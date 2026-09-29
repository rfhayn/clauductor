package panel

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

func waitUntil(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// writeLeaseRecord writes an owner.json or waiter file as the lease protocol
// (docs/panel.md) lays it out on disk.
func writeLeaseRecord(t *testing.T, path string, o lease.LeaseOwner) {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The lane registry's file, as the tests read and write it (lanes.RegistryPath).
type registryFile struct {
	Version int                `json:"version"`
	Project string             `json:"project"`
	Lanes   []lanes.LaneRecord `json:"lanes"`
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

func testLaneManager(t *testing.T) *lanes.LaneManager {
	t.Helper()
	cfg, err := config.ParseConfig([]byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &lanes.LaneManager{TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

// goneGrace mirrors the reducer's grace for a template lane's missing tmux session
// (30 s) before it needs a RESTORE.
const goneGrace = 30 * time.Second

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("signals", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("config", "testdata", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ParseConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func fixtureWorktrees(t *testing.T) []signals.Worktree {
	t.Helper()
	wts, err := signals.ParseWorktreePorcelain(fixture(t, "worktrees-fixture.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

const xWT = "/repo/w/x"

func waitingAgent(status, waitingFor string) []signals.Agent {
	return []signals.Agent{{SessionID: "s1", Cwd: xWT, Status: status, WaitingFor: waitingFor}}
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ourHooks counts the panel's tagged hook objects per event in settings.json: a
// hook is the panel's when its URL carries src=<install.HookTag>.
func ourHooks(t *testing.T, home string) map[string]int {
	t.Helper()
	b, err := os.ReadFile(install.SettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	hooks, _ := settings["hooks"].(map[string]any)
	for ev, groups := range hooks {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				s, _ := h.(map[string]any)["url"].(string)
				if u, err := url.Parse(s); err == nil && s != "" && u.Query().Get("src") == install.HookTag {
					out[ev]++
				}
			}
		}
	}
	return out
}

func v2Config(t *testing.T, extra string) *config.Config {
	t.Helper()
	c, err := config.ParseConfig([]byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}` + extra + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const tplJSON = `,"templates":[
 {"id":"build","title":"Build a change","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"Run build-change for {name}","model":"opus","effort":"high"},
 {"id":"fix","title":"Fix an issue","lane_type":"fix","first_prompt":"Fix issue {issue} on branch fix/{name}"}]`

// ---- alerts and the notifier ----

func alertModel(t *testing.T, cfgExtra string) *state.Model {
	t.Helper()
	m := state.NewModel(v2Config(t, cfgExtra), "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}, nil, t0)
	return m
}

func alertKinds(v state.View) map[string]string {
	out := map[string]string{}
	for _, a := range v.Alerts {
		out[a.Kind] = a.Severity
	}
	return out
}
