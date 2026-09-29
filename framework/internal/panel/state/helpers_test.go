package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// agentsQuiet mirrors the runtime's quiet `claude agents` interval (15 s), the
// longest wait freshness must allow for.
const agentsQuiet = 15 * time.Second

func v2Config(t *testing.T, extra string) *config.Config {
	t.Helper()
	c, err := loadConfig(t, []byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}`+extra+`}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const tplJSON = `,"templates":[
 {"id":"build","title":"Build a change","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"Run build-change for {name}","model":"opus","effort":"high"},
 {"id":"fix","title":"Fix an issue","lane_type":"fix","first_prompt":"Fix issue {issue} on branch fix/{name}"}]`

func alertModel(t *testing.T, cfgExtra string) *Model {
	t.Helper()
	m := NewModel(v2Config(t, cfgExtra), "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}, nil, t0)
	return m
}

func alertKinds(v View) map[string]string {
	out := map[string]string{}
	for _, a := range v.Alerts {
		out[a.Kind] = a.Severity
	}
	return out
}

// readWeb reads one of the page's static files.
func readWeb(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "web", "static", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// loadConfig reads panel.json bytes the way the panel does: from a file.
func loadConfig(t *testing.T, b []byte) (*config.Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return config.LoadConfig(p)
}

// readWorktreesFrom reads `git worktree list --porcelain` output the way the panel
// does, through signals.ReadWorktrees.
func readWorktreesFrom(out []byte) ([]signals.Worktree, error) {
	return signals.ReadWorktrees(context.Background(), func(context.Context, string, []string) ([]byte, error) { return out, nil }, "/")
}
