package panel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pclock "github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/lanes"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/state"
	"github.com/rfhayn/stridal/internal/panel/types"
	"github.com/rfhayn/stridal/internal/panel/web"
)

const autoCloseSHA = "0123456789abcdef0123456789abcdef01234567"

// [LANECLOSE-2-S3] Auto-close reads the verdict again under the lane lock: a lane idle
// on a current reading, its pull request merged and its branch merged, when the poll
// reads the view, whose claude starts work while its close is planned (the plan's git
// fetch changes the hub, as a poll or a hook applied meanwhile would), is not closed:
// its worktree and branch stay, and Needs you asks about it with the reason. So is one
// whose claude the view shows exited (dead) and tmux, read under the lock, shows
// running again: the owner restarted it while the close was planned.
func TestAutoCloseReadsTheVerdictAgainUnderTheLock(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, want string
		dead       bool // the view's last tmux poll shows the lane's claude exited
		change     func(m *state.Model, wt string, now time.Time)
	}{
		{"claude turns busy", "claude is working", false, func(m *state.Model, wt string, now time.Time) {
			m.ApplyAgents([]signals.Agent{{SessionID: "s-x", Cwd: wt, Status: "busy", PID: 100}}, nil, now)
		}},
		{"its turn ends on a background shell", "its turn ended on background work: go test ./...", false, func(m *state.Model, wt string, now time.Time) {
			m.ApplyHook(mustHook(t, `{"hook_event_name":"Stop","session_id":"s-x","cwd":"`+wt+
				`","background_tasks":[{"id":"b1","type":"shell","status":"running","description":"go test ./..."}]}`), now)
		}},
		{"its exited claude is restarted", state.CloseRunsAgainWords, true, func(*state.Model, string, time.Time) {}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","change/":"build"},"tmux_socket":"sock","lanes_auto_close":"on_merge"}`))
			if err != nil {
				t.Fatal(err)
			}
			reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
			if err != nil {
				t.Fatal(err)
			}
			clk := pclock.NewFake(t0)
			lm := &lanes.LaneManager{Clock: clk, TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
				Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
			wt := filepath.Join(cfg.WorktreeRoot("/repo"), "x")
			if err := reg.Put(types.LaneRecord{ID: "x", SessionID: "s-x", Path: wt, Type: "build", Branch: "change/x", Action: "start", ActionDone: true}); err != nil {
				t.Fatal(err)
			}
			m := state.NewModel(cfg, "/repo", t0)
			m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main", Head: autoCloseSHA}, {Path: wt, Branch: "change/x", Head: autoCloseSHA}}, nil, t0)
			m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: wt, Created: 1, Dead: c.dead}}, reg.List(), "", nil, t0)
			m.ApplyAgents([]signals.Agent{{SessionID: "s-x", Cwd: wt, Status: "idle", PID: 100}}, nil, t0)
			m.ApplyHook(mustHook(t, `{"hook_event_name":"Stop","session_id":"s-x","cwd":"`+wt+`","background_tasks":[]}`), t0)
			hub := web.NewHub(m, clk)
			if held := hub.LaneCloseHeld("x", nil); held != "" {
				t.Fatalf("before the poll: CloseHeld %q, want none", held)
			}

			var mu sync.Mutex
			var ran []string
			stopped := false // a stop has begun: the lane's claude is gone from here on
			lm.Exec = func(_ context.Context, argv []string) ([]byte, error) {
				cmd := strings.Join(argv, " ")
				mu.Lock()
				defer mu.Unlock()
				ran = append(ran, cmd)
				switch {
				case strings.Contains(cmd, " list-panes "):
					if stopped {
						// A close that wrongly went ahead finds the lane gone, so its wait for
						// the death returns under the fake clock and the test's own
						// assertions fail, rather than a hang until go test's timeout.
						return nil, nil
					}
					// tmux now: the lane's claude runs (whatever the view's poll showed).
					return []byte("x\t" + wt + "\t" + wt + "\t0\t\t1\t0\tbuild\n"), nil
				case strings.Contains(cmd, " send-keys "), strings.Contains(cmd, " kill-session "):
					stopped = true
					return nil, nil
				}
				return nil, errors.New("unexpected tmux call " + cmd)
			}
			lm.Run = func(_ context.Context, _ string, argv []string) ([]byte, error) {
				cmd := strings.Join(argv, " ")
				mu.Lock()
				ran = append(ran, cmd)
				mu.Unlock()
				switch {
				case cmd == "git fetch --quiet":
					hub.Update(func(m *state.Model, now time.Time) { c.change(m, wt, now) })
					return nil, nil
				case cmd == "git worktree list --porcelain":
					return []byte("worktree /repo\nHEAD " + autoCloseSHA + "\nbranch refs/heads/main\n\nworktree " + wt + "\nHEAD " + autoCloseSHA + "\nbranch refs/heads/change/x\n\n"), nil
				case strings.HasPrefix(cmd, "git rev-parse --verify --quiet refs/heads/"), strings.HasPrefix(cmd, "git for-each-ref --merged="):
					return []byte(autoCloseSHA + "\n"), nil
				case argv[0] == "gh":
					return []byte("[]"), nil
				case argv[0] == "git":
					return nil, nil // status (clean), config
				}
				return nil, errors.New("unexpected command " + cmd)
			}
			run := func(_ context.Context, _ string, argv []string) ([]byte, error) {
				if strings.Join(argv[:3], " ") == "gh pr list" {
					return []byte(`[{"number":7}]`), nil
				}
				return nil, errors.New("unexpected command " + strings.Join(argv, " "))
			}
			var notices []state.Notice
			r := &Runtime{hub: hub, run: run, root: "/repo", clock: clk, cfg: cfg, lanes: lm,
				o: Options{Notify: func(n state.Notice) error { notices = append(notices, n); return nil }}}
			r.trust.Store(true)

			up, _ := r.pollAutoClose(t.Context(), t0)
			if up == nil {
				t.Fatal("pollAutoClose returned no update")
			}
			hub.Update(up)

			mu.Lock()
			calls := append([]string(nil), ran...)
			mu.Unlock()
			for _, cmd := range calls {
				if strings.HasPrefix(cmd, "git worktree remove") || strings.HasPrefix(cmd, "git update-ref -d") || strings.HasPrefix(cmd, "git branch -D") {
					t.Errorf("the close ran %q", cmd)
				}
			}
			if _, ok := reg.Get("x"); !ok {
				t.Error("the lane's record is gone")
			}
			v := hub.View()
			asked := false
			for _, n := range v.NeedsYou {
				if n.Kind == "merged" && n.Terminal == "x" && strings.Contains(n.Text, "#7 merged") && strings.Contains(n.Text, c.want) &&
					!strings.Contains(n.Text, "Close failed") && !strings.Contains(n.Text, "is not closed") {
					asked = true
				}
			}
			if !asked {
				t.Errorf("Needs you does not ask about lane x with %q alone: %+v", c.want, v.NeedsYou)
			}
			// The notification gives the verdict's words too, not the refused close's.
			if len(notices) != 1 || !strings.HasSuffix(notices[0].Body, "merged, but "+c.want+".") {
				t.Errorf("notices %+v, want one ending %q", notices, "merged, but "+c.want+".")
			}
			for _, n := range v.Terminals {
				if n.ID == "x" && n.Worktree == "" {
					t.Error("lane x lost its worktree")
				}
			}
		})
	}
}

func mustHook(t *testing.T, body string) signals.HookEvent {
	t.Helper()
	ev, err := signals.ParseHook([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}
