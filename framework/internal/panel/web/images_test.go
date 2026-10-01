package web

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/leakcheck"
	"github.com/clauductor/clauductor/internal/panel/lanes"
)

const pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

// PANEL-15b: the drop route is a lane action like the others: the cookie, this
// page's Origin, a valid lane; and it keeps only an image, by its bytes, under the
// size cap. Nothing refused leaves a file behind.
func TestDroppedImageRouteRefuses(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.Socket = leakcheck.NoServerSocket() // never the machine's own panel socket
	uploads := t.TempDir()
	s.Lanes.UploadDir = uploads
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	for _, c := range []struct {
		name, target, body string
		opts               []reqOpt
		want               int
	}{
		{"no cookie", "/api/lanes/lane-a/image", pngBytes, []reqOpt{origin}, 401},
		{"no Origin", "/api/lanes/lane-a/image", pngBytes, []reqOpt{withCookie(s)}, 403},
		{"another port's Origin", "/api/lanes/lane-a/image", pngBytes, []reqOpt{withCookie(s), withHeader("Origin", "http://127.0.0.1:3000")}, 403},
		{"not an image", "/api/lanes/lane-a/image", "#!/bin/sh\nrm -rf ~\n", []reqOpt{withCookie(s), origin, withHeader("X-Filename", "cat.png")}, 415},
		{"a name is not a type", "/api/lanes/lane-a/image", "GIF", []reqOpt{withCookie(s), origin, withHeader("Content-Type", "image/gif")}, 415},
		{"too large", "/api/lanes/lane-a/image", pngBytes + strings.Repeat("x", lanes.MaxImageBytes), []reqOpt{withCookie(s), origin}, 413},
		{"bad lane id", "/api/lanes/Bad.Lane/image", pngBytes, []reqOpt{withCookie(s), origin}, 400},
		// The socket has no server here: an image for a lane that is not running.
		{"no such lane", "/api/lanes/lane-a/image", pngBytes, []reqOpt{withCookie(s), origin}, 404},
	} {
		if w := do(s, "POST", c.target, c.body, c.opts...); w.Code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, w.Code, w.Body, c.want)
		}
	}
	if ents, _ := os.ReadDir(uploads); len(ents) != 0 {
		t.Fatalf("a refused drop left %d entries in %s", len(ents), uploads)
	}
}

// A dropped image is kept 0600 in the panel's state directory, never the worktree,
// and its path reaches the lane as a terminal's drop does: a bracketed paste and a
// space, never Enter.
func TestDroppedImageIsTypedWithoutEnter(t *testing.T) {
	t.Parallel()
	tmux, sock := leakcheck.TmuxSocket(t)
	wt := t.TempDir()
	typed := filepath.Join(t.TempDir(), "typed")
	// The lane asks for bracketed paste, as claude does, then records its raw input.
	prog := "printf '\\033[?2004h'; stty raw -echo; exec cat > " + shq(typed)
	if out, err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "lane-a", "-c", wt, "sh", "-c", prog).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.TmuxPath, s.Lanes.Socket = tmux, sock
	uploads := filepath.Join(t.TempDir(), "uploads")
	s.Lanes.UploadDir = uploads
	// Wait until the lane has asked for bracketed paste (tmux pastes plainly before):
	// cat creating the file comes after the request.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(typed); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	w := do(s, "POST", "/api/lanes/lane-a/image", pngBytes, withCookie(s), withHeader("Origin", "http://127.0.0.1:4393"),
		withHeader("X-Filename", "..%2F..%2Fmy%20screen%20shot.jpeg"))
	if w.Code != 200 {
		t.Fatalf("drop: %d %s", w.Code, w.Body)
	}
	var res struct{ Path string }
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if filepath.Dir(res.Path) != filepath.Join(uploads, "lane-a") || !strings.HasSuffix(res.Path, "-my-screen-shot.png") {
		t.Fatalf("kept at %s: want %s/lane-a/<ms>-my-screen-shot.png (the name sanitised, the extension the bytes')", res.Path, uploads)
	}
	if fi, err := os.Stat(res.Path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the kept image: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(res.Path); string(b) != pngBytes {
		t.Fatal("the kept image differs from what was dropped")
	}
	want := "\x1b[200~" + res.Path + "\x1b[201~ "
	var got []byte
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ = os.ReadFile(typed); len(got) >= len(want) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if string(got) != want {
		t.Fatalf("the lane got %q, want %q", got, want)
	}
	if bytes.ContainsAny(got, "\r\n") {
		t.Fatal("the drop pressed Enter")
	}
	_ = filepath.Walk(wt, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			t.Errorf("the drop wrote %s into the worktree", p)
		}
		return nil
	})
}
