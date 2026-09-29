package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/coder/websocket"
)

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "config", "testdata", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(t, b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func fixtureWorktrees(t *testing.T) []signals.Worktree {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "signals", "testdata", "worktrees-fixture.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	wts, err := readWorktreesFrom(b)
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

func v2Config(t *testing.T, extra string) *config.Config {
	t.Helper()
	c, err := loadConfig(t, []byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}`+extra+`}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func alertModel(t *testing.T, cfgExtra string) *state.Model {
	t.Helper()
	m := state.NewModel(v2Config(t, cfgExtra), "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}, nil, t0)
	return m
}

const xWT = "/repo/w/x"

func waitingAgent(status, waitingFor string) []signals.Agent {
	return []signals.Agent{{SessionID: "s1", Cwd: xWT, Status: status, WaitingFor: waitingFor}}
}

func testLaneManager(t *testing.T) *lanes.LaneManager {
	t.Helper()
	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))

	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &lanes.LaneManager{Clock: clock.System, TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}

// throwawaySocket is a tmux socket of the test's own, killed at cleanup.
func throwawaySocket(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: drives a real tmux server")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	b := make([]byte, 4)
	rand.Read(b)
	sock := "clauductor-test-" + strconv.Itoa(os.Getpid()) + "-" + hex.EncodeToString(b)
	t.Cleanup(func() {
		exec.Command(tmux, "-L", sock, "kill-server").Run()
		// kill-server leaves the socket file; tmux keeps it in $TMUX_TMPDIR (or /tmp)/tmux-<uid>.
		dir := os.Getenv("TMUX_TMPDIR")
		if dir == "" {
			dir = "/tmp"
		}
		os.Remove(filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), sock))
	})
	return tmux, sock
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func closeCode(c *websocket.Conn, within time.Duration) websocket.StatusCode {
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
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
