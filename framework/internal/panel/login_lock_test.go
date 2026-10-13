package panel

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/claudecall"
	"github.com/clauductor/clauductor/internal/panel/config"
)

// Run itself watches the login refresh locks (PANEL-28), in the config dir its
// Options.Getenv names: a lock there while the panel runs is in login-lock.jsonl once
// it stops. The config dir is a temp one, never the process's CLAUDE_CONFIG_DIR.
func TestRunWatchesTheLoginLocks(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	cfg := filepath.Join(t.TempDir(), "claude-config")
	getenv := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return cfg
		}
		return testEnv(k)
	}
	r := claudecall.NewReader(getenv, home)
	if err := os.MkdirAll(r.CurrentPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Getenv: getenv, Runner: fakeRunner(root)})
	stop()

	f, err := os.Open(filepath.Join(config.PanelDir(home), claudecall.LogName))
	if err != nil {
		t.Fatalf("no login-lock log after a run with a lock present: %v", err)
	}
	defer f.Close()
	var lines []claudecall.LockLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l claudecall.LockLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		lines = append(lines, l)
	}
	if len(lines) != 1 || lines[0].Path != filepath.Join(cfg, ".oauth_refresh.lock") || lines[0].Ended != claudecall.EndStopped {
		t.Fatalf("lines: %+v, want the current lock in %s, ended %q", lines, cfg, claudecall.EndStopped)
	}
}
