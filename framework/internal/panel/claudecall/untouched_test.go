package claudecall

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// fileState is what the panel must leave as it found: whether a path exists, its
// mode, its modification time, and its contents (a file's bytes, a directory's
// names).
type fileState struct {
	Exists  bool
	Mode    fs.FileMode
	ModTime time.Time
	Content string
	Names   []string
}

func stateOf(t *testing.T, path string) fileState {
	t.Helper()
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return fileState{}
	}
	if err != nil {
		t.Fatal(err)
	}
	s := fileState{Exists: true, Mode: fi.Mode(), ModTime: fi.ModTime()}
	if fi.IsDir() {
		es, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			s.Names = append(s.Names, e.Name())
		}
		sort.Strings(s.Names)
	} else {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s.Content = string(b)
	}
	return s
}

// [CLAUDECALLS-5-S3] The panel never touches a lock: with a live, a stale or a
// future-dated lock and its owner record, and a legacy lock, the reader, the watch,
// the gate's pause and the alerts all run, and every lock and record still exists
// with the same contents and modification time.
func TestNothingTouchesALockOrItsRecord(t *testing.T) {
	cfg, err := config.ParseConfig([]byte(`{"name":"T","lanes":{"main":"orchestrator"}}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, age := range map[string]time.Duration{"live": 3 * time.Second, "stale": 5 * time.Minute, "future": -2 * time.Hour} {
		t.Run(name, func(t *testing.T) {
			home, r := fakeConfig(t)
			now := time.Now()
			mod := now.Add(-age)
			makeLock(t, r.CurrentPath(), mod)
			// A lock directory holds what proper-lockfile may leave in it.
			if err := os.WriteFile(filepath.Join(r.CurrentPath(), "inside"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			touch(t, r.CurrentPath(), mod)
			writeRecord(t, r, recordStart(otherHolder(t), mod.Add(-time.Second), "Thu Oct  2 10:11:00 2026"))
			touch(t, r.OwnerPath(), mod)
			makeLock(t, r.LegacyPath(), mod.Add(-10*time.Minute))
			paths := []string{r.CurrentPath(), filepath.Join(r.CurrentPath(), "inside"), r.OwnerPath(), r.LegacyPath()}
			before := map[string]fileState{}
			for _, p := range paths {
				before[p] = stateOf(t, p)
				if !before[p].Exists {
					t.Fatalf("%s was not made", p)
				}
			}

			// The reader.
			r.Read(now)
			r.AnyLive(now)
			// The gate: a poll, paused while the lock is live; it runs no claude here.
			g := NewGate(func(context.Context, string, []string) ([]byte, error) { return []byte("[]"), nil }, r, clock.System, NewLog(home))
			ctx, cancel := context.WithTimeout(signals.WithPoll(context.Background()), 10*time.Second)
			g.Run(ctx, "", []string{"claude", "agents", "--json"})
			cancel()
			g.Close(ShutdownGrace)
			// The watch, with ps, its log and the alerts it feeds.
			var locks []types.LoginLock
			w := &Watch{Reader: r, Log: NewLog(home), Clock: clock.System, Run: testRunner, Attribute: g.Attribute,
				OnChange: func(ls []types.LoginLock) { locks = ls }}
			for i := 0; i < 3; i++ {
				w.Tick(context.Background())
			}
			if len(locks) != 2 {
				t.Fatalf("the watch saw %+v", locks)
			}
			m := state.NewModel(cfg, "/repo", now)
			m.ApplyLoginLocks(locks)
			for _, at := range []time.Duration{0, 3 * time.Minute, 3 * time.Hour} {
				m.Snapshot(now.Add(at))
			}
			w.Stop()

			for _, p := range paths {
				if after := stateOf(t, p); !reflect.DeepEqual(after, before[p]) {
					t.Errorf("%s changed:\n before %+v\n after  %+v", p, before[p], after)
				}
			}
		})
	}
}
