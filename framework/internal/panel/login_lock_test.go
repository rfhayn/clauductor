package panel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/claudecall"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/state"
)

// Run itself watches the login refresh locks (PANEL-28), in the config dir its
// Options.LookupEnv names: a lock there while the panel runs is in login-lock.jsonl
// once it stops. The config dir is a temp one, never the process's CLAUDE_CONFIG_DIR.
func TestRunWatchesTheLoginLocks(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	cfg := filepath.Join(t.TempDir(), "claude-config")
	lookup := func(k string) (string, bool) {
		if k == "CLAUDE_CONFIG_DIR" {
			return cfg, true
		}
		return testEnv(k)
	}
	r := claudecall.NewReader(lookup, home)
	if err := os.MkdirAll(r.CurrentPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	// Its owner record names a lock born before this panel: Run's watch asks the gate's
	// call history (PANEL-28 group 2), which can say so; a watch with no history
	// would say "unknown: no call history".
	born := time.Now().Add(-time.Hour).UnixMilli()
	if err := os.WriteFile(r.OwnerPath(), []byte(fmt.Sprintf(`{"pid":4242,"lockBirthtimeMs":%d}`, born)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, LookupEnv: lookup, Runner: fakeRunner(root)})
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
	if len(lines) != 1 || lines[0].Path != filepath.Join(cfg, ".oauth_refresh.lock") || lines[0].Ended != claudecall.EndStopped ||
		lines[0].Holder != "unknown: before this panel started" || lines[0].PID != 4242 {
		t.Fatalf("lines: %+v, want the current lock in %s, ended %q, its holder unknown: before this panel started",
			lines, cfg, claudecall.EndStopped)
	}
}

// Run feeds the watch's reading to every project's model (PANEL-28 group 3): a lock
// stranded in the config dir is a warning on the page, naming its path and its gone
// holder, with the remedy.
func TestRunShowsAStrandedLock(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	r := claudecall.NewReader(testEnv, home)
	if err := os.MkdirAll(r.CurrentPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	gone := exec.Command("true")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	mod := time.Now().Add(-5 * time.Minute)
	rec := fmt.Sprintf(`{"pid":%d,"lockBirthtimeMs":%d}`, gone.Process.Pid, mod.UnixMilli())
	if err := os.WriteFile(r.OwnerPath(), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(r.CurrentPath(), mod, mod); err != nil {
		t.Fatal(err)
	}
	_, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	defer stop()
	var got state.AlertView
	waitFor(t, "the stranded-lock warning", func() bool {
		for _, a := range c.state(t).Alerts {
			if a.Kind == state.AlertStrandedLock {
				got = a
				return true
			}
		}
		return false
	})
	for _, want := range []string{r.CurrentPath(), "is gone", "remove " + r.CurrentPath() + " and run /login"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("warning %q lacks %q", got.Text, want)
		}
	}
}
