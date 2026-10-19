package lanes

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// #102: tmux under load can leave an exited pane dead with neither pane_dead_status nor
// pane_dead_signal. fallbackTmux stands in for a lane socket that does that to every
// `--resume` launch, as claude on a session with no conversation exits 1 at once; a
// `--session-id` launch stays up. It keeps each command it is sent, in order. Nothing
// here starts a tmux server.
type fallbackTmux struct {
	mu       sync.Mutex
	paths    map[string]string     // lane id -> its pane's directory
	dead     map[string]bool       // lane id -> its pane exited, with no status recorded
	launches map[string][][]string // lane id -> each launch's lane command
	cmds     []string              // every command, its first four words
	allDie   bool                  // a --session-id launch dies the same way too
}

func newFallbackTmux() *fallbackTmux {
	return &fallbackTmux{paths: map[string]string{}, dead: map[string]bool{}, launches: map[string][][]string{}}
}

func (f *fallbackTmux) exec(_ context.Context, argv []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	args := argv[4:] // after -L <socket> -f /dev/null
	f.cmds = append(f.cmds, strings.Join(args[:min(len(args), 4)], " "))
	target := func(a string) string { return strings.TrimSuffix(strings.TrimPrefix(a, "="), ":") }
	switch args[0] {
	case "new-session", "respawn-pane": // new-session -d -s <id> -c <path> …; respawn-pane -k -t =<id>: -c <path> …
		id := target(args[3])
		if _, ok := f.paths[id]; args[0] == "respawn-pane" && !ok {
			return nil, errors.New("tmux: can't find session")
		}
		f.paths[id] = args[5]
		for i, a := range args {
			if a == "/usr/bin/env" {
				end := i
				for end < len(args) && args[end] != ";" {
					end++
				}
				cmd := append([]string(nil), args[i:end]...)
				f.launches[id] = append(f.launches[id], cmd)
				f.dead[id] = f.allDie || strings.Contains(strings.Join(cmd, " "), "--resume")
				break
			}
		}
	case "has-session":
		if _, ok := f.paths[target(args[2])]; !ok {
			return nil, errors.New("tmux: can't find session")
		}
	case "list-panes":
		var b strings.Builder
		for id, p := range f.paths {
			dead := "0"
			if f.dead[id] {
				dead = "1"
			}
			b.WriteString(id + "\t" + p + "\t" + p + "\t" + dead + "\t\t0\t0\tfix\t\n")
		}
		return []byte(b.String()), nil
	case "send-keys": // Escape: the claude exits, and the pane with it
		if args[len(args)-1] == "Escape" {
			delete(f.paths, target(args[2]))
		}
	case "kill-session":
		delete(f.paths, target(args[2]))
	}
	return nil, nil
}

func (f *fallbackTmux) snapshot(id string) (launches [][]string, cmds []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.launches[id]...), append([]string(nil), f.cmds...)
}

// [#102] A restart whose `--resume` claude dies with no status recorded falls back to
// `--session-id` in the same pane, and the server is kept from emptying meanwhile.
func TestRestartFallsBackWhenTmuxRecordsNoStatus(t *testing.T) {
	r := newCloseRepo(t)
	tm := newFallbackTmux()
	r.m.Exec = tm.exec
	r.m.LookupEnv = func(string) (string, bool) { return "", false }
	r.m.FastExit = 300 * time.Millisecond
	ctx := context.Background()
	res, lerr := r.m.Start(ctx, StartRequest{Type: "fix", Mode: "root", Name: "fx"})
	if lerr != nil {
		t.Fatal(lerr)
	}
	// The registry thinks the session has a conversation, so the restart tries --resume.
	if _, err := r.m.Registry.MarkConversation(res.SessionID); err != nil {
		t.Fatal(err)
	}
	_, before := tm.snapshot("fx")
	if lerr := r.m.Restart(ctx, "fx"); lerr != nil {
		t.Fatalf("restart: %v", lerr)
	}
	launches, cmds := tm.snapshot("fx")
	cmds = cmds[len(before):]
	if len(launches) != 3 {
		t.Fatalf("%d launches, want 3 (the start, the restart's --resume, its --session-id fallback): a dead pane with no status was read as a start", len(launches))
	}
	for i, want := range []string{"--resume", "--session-id"} {
		if tail := launches[i+1][len(launches[i+1])-2:]; !reflect.DeepEqual(tail, []string{want, res.SessionID}) {
			t.Fatalf("restart launch %d ends %q, want %s %s", i+1, tail, want, res.SessionID)
		}
	}
	if rec, _ := r.m.Registry.Get("fx"); rec.Conversation || !rec.ActionDone {
		t.Fatalf("registry after the fallback: %+v", rec)
	}
	// The order on the socket: exit-empty off before the stop's kill-session, and not
	// on again until the lane's new session exists; the fallback respawns the pane
	// rather than killing its session; exit-empty back on last.
	off, kill, launch, respawn, on := -1, -1, -1, -1, -1
	for i, c := range cmds {
		switch {
		case c == "set-option -g exit-empty off" && off < 0:
			off = i
		case strings.HasPrefix(c, "kill-session"):
			if launch >= 0 {
				t.Fatalf("the fallback killed the lane's session (%q at %d), which can empty the server: %q", c, i, cmds)
			}
			kill = i
		case strings.HasPrefix(c, "new-session") && launch < 0:
			launch = i
		case strings.HasPrefix(c, "respawn-pane"):
			respawn = i
		case c == "set-option -g exit-empty on":
			if launch < 0 {
				t.Fatalf("exit-empty on at %d, before the lane's new session: the server can exit there: %q", i, cmds)
			}
			on = i
		}
	}
	if off < 0 || kill < off || launch < kill || respawn < launch || on < respawn {
		t.Fatalf("exit-empty off %d, kill-session %d, new-session %d, respawn-pane %d, exit-empty on %d; want them in that order: %q", off, kill, launch, respawn, on, cmds)
	}
}

// [#102] When both flags die with no status recorded, the restart is refused as a
// failed launch, saying the status is unknown, and the dead pane is left to show why.
func TestRestartWhoseLaunchesBothDieWithNoStatusFails(t *testing.T) {
	r := newCloseRepo(t)
	tm := newFallbackTmux()
	r.m.Exec = tm.exec
	r.m.LookupEnv = func(string) (string, bool) { return "", false }
	r.m.FastExit = 300 * time.Millisecond
	ctx := context.Background()
	res, lerr := r.m.Start(ctx, StartRequest{Type: "fix", Mode: "root", Name: "fx"})
	if lerr != nil {
		t.Fatal(lerr)
	}
	tm.mu.Lock()
	tm.allDie = true
	tm.mu.Unlock()
	lerr = r.m.Restart(ctx, "fx")
	if lerr == nil || lerr.Status != 409 || lerr.Code != "launch-failed" {
		t.Fatalf("restart: %v, want a 409 launch-failed", lerr)
	}
	for _, want := range []string{"--resume or with --session-id " + res.SessionID, "no exit status"} {
		if !strings.Contains(lerr.Error(), want) {
			t.Errorf("the refusal %q does not say %q", lerr.Error(), want)
		}
	}
	if launches, _ := tm.snapshot("fx"); len(launches) != 3 {
		t.Fatalf("%d launches, want 3 (the start and both flags)", len(launches))
	}
	if !r.m.Exists(ctx, "fx") {
		t.Fatal("the dead pane was not left on screen")
	}
}
