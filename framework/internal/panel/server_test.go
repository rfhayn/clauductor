package panel

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testPort = 4393

func newTestServer(t *testing.T) (*Server, chan []byte) {
	t.Helper()
	hooks := make(chan []byte, 8)
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	return &Server{Port: testPort, Token: "secret-token", Hub: NewHub(m, time.Now), Hooks: hooks, Status: make(chan []byte, 8)}, hooks
}

type reqOpt func(*http.Request)

func withCookie(s *Server) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.Token}) }
}
func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }
func withHost(h string) reqOpt      { return func(r *http.Request) { r.Host = h } }
func withRemote(a string) reqOpt    { return func(r *http.Request) { r.RemoteAddr = a } }

func do(s *Server, method, target, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	// A bounded context: if a guard regressed, /events would stream forever and the
	// test would hang instead of failing.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	r.Host = "127.0.0.1:" + strconv.Itoa(testPort)
	r.RemoteAddr = "127.0.0.1:50000"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestListenBindsLoopbackOnly(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	a := ln.Addr().(*net.TCPAddr)
	if a.IP.String() != LoopbackHost {
		t.Fatalf("bound %v, want %s", a, LoopbackHost)
	}
	if ensureLoopback(&net.TCPAddr{IP: net.IPv4zero, Port: 4393}) == nil {
		t.Fatal("0.0.0.0 accepted as loopback")
	}
	if ensureLoopback(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 4393}) == nil {
		t.Fatal("LAN address accepted as loopback")
	}
}

func TestListenRefusesATakenPort(t *testing.T) {
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	ln, err := Listen(port)
	if err == nil {
		ln.Close()
		t.Fatalf("bound a taken port (got %v)", ln.Addr())
	}
	if !strings.Contains(err.Error(), "already in use") || !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Fatalf("unclear error: %v", err)
	}
}

func TestBrowserRoutesNeedTheToken(t *testing.T) {
	s, _ := newTestServer(t)
	tests := []struct {
		name   string
		method string
		target string
		opts   []reqOpt
		want   int
	}{
		{"page without cookie", "GET", "/", nil, 401},
		{"events without cookie", "GET", "/api/state", nil, 401},
		{"state without cookie", "GET", "/events", nil, 401},
		{"refresh without cookie", "POST", "/api/refresh", []reqOpt{withHeader("Origin", "http://127.0.0.1:4393")}, 401},
		{"wrong token", "GET", "/?t=nope", nil, 401},
		{"wrong cookie", "GET", "/", []reqOpt{func(r *http.Request) { r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: "nope"}) }}, 401},
		{"page with cookie", "GET", "/", []reqOpt{withCookie(s)}, 200},
		{"state with cookie", "GET", "/api/state", []reqOpt{withCookie(s)}, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if w := do(s, tc.method, tc.target, "", tc.opts...); w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestTokenExchangeSetsAStrictCookie(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(s, "GET", "/?t="+s.Token, "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("got %d → %q", w.Code, w.Header().Get("Location"))
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Value != s.Token || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", c)
	}
	page := do(s, "GET", "/", "", withCookie(s))
	if !strings.Contains(page.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || page.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("security headers missing")
	}
}

func TestForeignHostRefused(t *testing.T) {
	s, _ := newTestServer(t)
	for _, host := range []string{"evil.example:4393", "127.0.0.1:4394", "localhost", "[::1]:4393", "127.0.0.1.nip.io:4393"} {
		for _, target := range []string{"/", "/hook", "/events"} {
			method := "GET"
			if target == "/hook" {
				method = "POST"
			}
			if w := do(s, method, target, "{}", withCookie(s), withHost(host)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s with Host %s: got %d", method, target, host, w.Code)
			}
		}
	}
	if w := do(s, "GET", "/", "", withCookie(s), withHost("localhost:4393")); w.Code != 200 {
		t.Fatalf("localhost:port refused: %d", w.Code)
	}
}

func TestStateChangingRequestsNeedOurOrigin(t *testing.T) {
	s, _ := newTestServer(t)
	tests := []struct {
		origin string
		want   int
	}{
		{"http://evil.example", 403},
		{"", 403},
		{"null", 403},
		{"http://127.0.0.1:4394", 403},
		{"http://127.0.0.1:4393", 204},
		{"http://localhost:4393", 204},
	}
	for _, tc := range tests {
		opts := []reqOpt{withCookie(s)}
		if tc.origin != "" {
			opts = append(opts, withHeader("Origin", tc.origin))
		}
		if w := do(s, "POST", "/api/refresh", "", opts...); w.Code != tc.want {
			t.Errorf("Origin %q: got %d want %d", tc.origin, w.Code, tc.want)
		}
	}
}

func TestIngestEndpoints(t *testing.T) {
	s, hooks := newTestServer(t)
	w := do(s, "POST", "/hook", `{"hook_event_name":"Stop","cwd":"/repo"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("hook: %d", w.Code)
	}
	select {
	case b := <-hooks:
		if !strings.Contains(string(b), "Stop") {
			t.Fatal("wrong body forwarded")
		}
	default:
		t.Fatal("body not forwarded")
	}
	if w := do(s, "POST", "/status", `{}`); w.Code != http.StatusNoContent {
		t.Fatalf("status: %d", w.Code)
	}
	tests := []struct {
		name string
		opts []reqOpt
		body string
		meth string
		want int
	}{
		{"browser cross-site post", []reqOpt{withHeader("Origin", "http://evil.example")}, "{}", "POST", 403},
		{"fetch metadata", []reqOpt{withHeader("Sec-Fetch-Site", "cross-site")}, "{}", "POST", 403},
		{"non-loopback peer", []reqOpt{withRemote("192.168.1.10:5555")}, "{}", "POST", 403},
		{"GET", nil, "", "GET", 405},
		{"oversized body", nil, strings.Repeat("x", MaxIngestBody+1), "POST", 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if w := do(s, tc.meth, "/hook", tc.body, tc.opts...); w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
	if len(hooks) != 0 {
		t.Fatal("a refused request was forwarded")
	}
}

func TestEventsStreamsFullStateOnConnect(t *testing.T) {
	s, _ := newTestServer(t)
	ts := httptest.NewUnstartedServer(nil)
	ts.Start()
	defer ts.Close()
	s.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	ts.Config.Handler = s.Handler()

	req, _ := http.NewRequest("GET", ts.URL+"/events", nil)
	req.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(resp.Body)
	deadline := time.After(3 * time.Second)
	got := make(chan string, 1)
	go func() {
		var sb strings.Builder
		for {
			line, err := br.ReadString('\n')
			sb.WriteString(line)
			if err != nil || strings.HasPrefix(line, "data: ") {
				got <- sb.String()
				return
			}
		}
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, "event: state") || !strings.Contains(s, `"name":"Standing Tee"`) {
			t.Fatalf("first event: %s", s)
		}
	case <-deadline:
		t.Fatal("no state event")
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 0))
}
