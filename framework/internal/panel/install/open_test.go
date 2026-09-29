package install

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
)

func serveHealthz(t *testing.T, network, addr, body string) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err) // a skip here would hide the check
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, body) })}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func TestOpenURLChecksBothLoopbacks(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	os.MkdirAll(config.PanelDir(home), 0o700)
	os.WriteFile(filepath.Join(config.PanelDir(home), "pid"), []byte("4242\n"), 0o600)
	ctx := context.Background()

	// Ours on both: open clauductor.localhost.
	_, port := serveHealthz(t, "tcp4", "127.0.0.1:0", "ok pid=4242")
	serveHealthz(t, "tcp6", fmt.Sprintf("[::1]:%d", port), "ok pid=4242")
	u, err := openURLChecked(ctx, home, port, "tok")
	if err != nil || u != fmt.Sprintf("http://clauductor.localhost:%d/?t=tok", port) {
		t.Fatalf("both ours: %q %v", u, err)
	}

	// Something else on ::1: refused, the token goes nowhere.
	_, port2 := serveHealthz(t, "tcp4", "127.0.0.1:0", "ok pid=4242")
	serveHealthz(t, "tcp6", fmt.Sprintf("[::1]:%d", port2), "ok pid=1")
	if u, err := openURLChecked(ctx, home, port2, "tok"); err == nil || u != "" {
		t.Fatalf("a foreign ::1 holder got %q", u)
	}

	// Nothing on ::1: open the address that is ours, 127.0.0.1.
	_, port3 := serveHealthz(t, "tcp4", "127.0.0.1:0", "ok pid=4242")
	u, err = openURLChecked(ctx, home, port3, "tok")
	if err != nil || u != fmt.Sprintf("http://127.0.0.1:%d/?t=tok", port3) {
		t.Fatalf("v4 only: %q %v", u, err)
	}

	// Something else on 127.0.0.1: refused.
	_, port4 := serveHealthz(t, "tcp4", "127.0.0.1:0", "ok pid=7")
	if _, err := openURLChecked(ctx, home, port4, "tok"); err == nil {
		t.Fatal("a foreign v4 holder got the token")
	}
}

// F6: `clauductor panel open` sends the token only to the panel whose PID is in the
// pid file.
func TestOpenURLOnlyTrustsThePanelsPID(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := RotateToken(home); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok pid=999\n") }))
	defer ts.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(ts.URL, "http://127.0.0.1:"))
	pid := filepath.Join(config.PanelDir(home), "pid")
	os.WriteFile(pid, []byte("123\n"), 0o600)
	if u, err := OpenURL(context.Background(), home, port); err == nil || strings.Contains(u, "t=") {
		t.Fatalf("token sent to an impostor: %q %v", u, err)
	}
	os.WriteFile(pid, []byte("999\n"), 0o600)
	if u, err := OpenURL(context.Background(), home, port); err != nil || !strings.Contains(u, "/?t=") {
		t.Fatalf("the real panel: %q %v", u, err)
	}
	os.Remove(pid)
	if _, err := OpenURL(context.Background(), home, port); err == nil {
		t.Fatal("no pid file, yet the token was handed out")
	}
}
