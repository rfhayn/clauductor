package panel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/state"
)

func TestHostAllowListIsExact(t *testing.T) {
	s, _ := newTestServer(t)
	s.HostNames = []string{"myproject.localhost"}
	p := strconv.Itoa(testPort)
	other := strconv.Itoa(testPort + 1)
	allowed := []string{
		"127.0.0.1:" + p, "localhost:" + p, "[::1]:" + p, "clauductor.localhost:" + p,
		"myproject.localhost:" + p, // configured
		// Host names compare case-insensitively (RFC 9110 §4.2.3).
		"CLAUDUCTOR.localhost:" + p, "Clauductor.LocalHost:" + p, "LOCALHOST:" + p,
	}
	refused := []string{
		"evil.localhost:" + p,                  // no wildcard
		"clauductor.localhost.evil.com:" + p,   // a suffix is not the name
		"x.clauductor.localhost:" + p,          // nor is a subdomain of it
		"clauductor.localhost:" + other,        // wrong port
		"127.0.0.1:" + other, "[::1]:" + other, // wrong port
		"clauductor.localhost", "127.0.0.1", // no port
		"clauductor.localhost.:" + p, // trailing dot
		"[::2]:" + p, "0.0.0.0:" + p, "127.0.0.2:" + p,
		"evil.com:" + p, "",
	}
	for _, h := range allowed {
		if w := do(s, "GET", "/healthz", "", withHost(h)); w.Code != 200 {
			t.Errorf("Host %q refused (%d)", h, w.Code)
		}
	}
	for _, h := range refused {
		if w := do(s, "GET", "/healthz", "", withHost(h)); w.Code != 403 {
			t.Errorf("Host %q allowed (%d)", h, w.Code)
		}
	}
}

// Origin must equal "http://" + THIS request's Host, on every POST and on the
// WebSocket upgrade: a page on one allowed name cannot drive the panel on another.
func TestOriginMustMatchThisRequestsHost(t *testing.T) {
	s, _ := newTestServer(t)
	p := strconv.Itoa(testPort)
	cases := []struct {
		host, origin string
		want         int
	}{
		{"clauductor.localhost:" + p, "http://clauductor.localhost:" + p, 204},
		{"[::1]:" + p, "http://[::1]:" + p, 204},
		{"CLAUDUCTOR.localhost:" + p, "http://clauductor.localhost:" + p, 204},
		{"clauductor.localhost:" + p, "http://localhost:" + p, 403},      // cross-host
		{"localhost:" + p, "http://clauductor.localhost:" + p, 403},      // cross-host
		{"clauductor.localhost:" + p, "http://127.0.0.1:" + p, 403},      // cross-host
		{"clauductor.localhost:" + p, "http://evil.localhost:" + p, 403}, // foreign
		{"clauductor.localhost:" + p, "https://clauductor.localhost:" + p, 403},
		{"clauductor.localhost:" + p, "", 403},
	}
	for _, c := range cases {
		w := do(s, "POST", "/api/refresh", "", withCookie(s), withHost(c.host), withHeader("Origin", c.origin))
		if w.Code != c.want {
			t.Errorf("POST Host %q Origin %q: %d, want %d", c.host, c.origin, w.Code, c.want)
		}
	}
	// A page on localhost opening a terminal WebSocket for clauductor.localhost.
	for _, origin := range []string{"http://localhost:" + p, "http://evil.localhost:" + p} {
		w := do(s, "GET", "/ws/term?lane=x", "", withCookie(s), withHost("clauductor.localhost:"+p),
			withHeader("Origin", origin), withHeader("Connection", "Upgrade"), withHeader("Upgrade", "websocket"))
		if w.Code != 403 || !strings.Contains(w.Body.String(), "origin") {
			t.Errorf("WS from %s: %d %q", origin, w.Code, w.Body.String())
		}
	}
}

func TestHostNamesConfig(t *testing.T) {
	for _, bad := range []string{"*.localhost", "evil.com", "a.b.localhost", "UPPER.localhost", "localhost", ".localhost", "-a.localhost", "a_b.localhost"} {
		if _, err := config.ParseConfig([]byte(`{"name":"T","host_names":["` + bad + `"]}`)); err == nil {
			t.Errorf("accepted host name %q", bad)
		}
	}
	if _, err := config.ParseConfig([]byte(`{"name":"T","host_names":["myproject.localhost","a1-b2.localhost"]}`)); err != nil {
		t.Fatal(err)
	}
}

// Both listeners serve one server, and clauductor.localhost reaches it.
func TestListenLoopbackServesBothAddresses(t *testing.T) {
	ln4, ln6, why, err := ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln4.Close()
	if ln6 == nil {
		t.Skip("no IPv6 loopback: " + why)
	}
	defer ln6.Close()
	port := ln4.Addr().(*net.TCPAddr).Port
	if ln6.Addr().(*net.TCPAddr).Port != port || !ln6.Addr().(*net.TCPAddr).IP.IsLoopback() {
		t.Fatalf("v6 listener %v", ln6.Addr())
	}
	s := &Server{Port: port, Token: "t", Hub: NewHub(state.NewModel(testConfig(t), "/repo", t0), time.Now)}
	srv := &http.Server{Handler: s.Handler()}
	go srv.Serve(ln4)
	go srv.Serve(ln6)
	defer srv.Close()
	for _, base := range []string{"http://127.0.0.1", "http://[::1]", "http://clauductor.localhost"} {
		resp, err := http.Get(fmt.Sprintf("%s:%d/healthz", base, port))
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(string(b), "ok pid=") {
			t.Fatalf("%s: %d %q", base, resp.StatusCode, b)
		}
	}
	// A second panel on the same port is refused on either address.
	if _, _, _, err := ListenLoopback(port); err == nil {
		t.Fatal("bound a taken port")
	}
}

// A process holding [::1]:<port> must be refused before the token is sent, since
// clauductor.localhost resolves to ::1 first.
func TestListenLoopbackRefusesAForeignV6Holder(t *testing.T) {
	foreign, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	defer foreign.Close()
	port := foreign.Addr().(*net.TCPAddr).Port
	ln4, _, _, err := ListenLoopback(port)
	if err == nil {
		ln4.Close()
		t.Fatal("started while another process holds [::1]:port")
	}
	if !strings.Contains(err.Error(), "[::1]") {
		t.Fatalf("error does not name ::1: %v", err)
	}
}

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
