package panel

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

// LoopbackHost is the only address the panel ever binds.
const LoopbackHost = "127.0.0.1"

// MaxIngestBody caps a hook or status-line body.
const MaxIngestBody = 256 << 10

// Listen binds 127.0.0.1:port and nothing else. A taken port is an error, never a
// reason to try another: the hooks in ~/.claude/settings.json post to a fixed URL.
func Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp4", net.JoinHostPort(LoopbackHost, strconv.Itoa(port)))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("port %d on %s is already in use (another panel? see ~/.clauductor/panel/port). "+
				"The panel does not fall back to another port, because its hooks post to a fixed URL; free the port or pass --port", port, LoopbackHost)
		}
		return nil, err
	}
	if err := ensureLoopback(ln.Addr()); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func ensureLoopback(a net.Addr) error {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() {
		return fmt.Errorf("refusing to serve on %v: the panel binds loopback only", a)
	}
	return nil
}

// NewToken returns a fresh per-launch secret (32 random bytes, hex).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Hub owns the model and fans snapshots out to SSE subscribers.
type Hub struct {
	mu    sync.Mutex
	model *Model
	now   func() time.Time
	subs  map[chan []byte]struct{}
	dirty chan struct{}
}

// NewHub wraps a model.
func NewHub(m *Model, now func() time.Time) *Hub {
	return &Hub{model: m, now: now, subs: map[chan []byte]struct{}{}, dirty: make(chan struct{}, 1)}
}

// Update applies fn to the model under the lock and schedules a broadcast.
func (h *Hub) Update(fn func(m *Model, now time.Time)) {
	h.mu.Lock()
	fn(h.model, h.now())
	h.mu.Unlock()
	select {
	case h.dirty <- struct{}{}:
	default:
	}
}

// Snapshot returns the current view as JSON.
func (h *Hub) Snapshot() []byte {
	h.mu.Lock()
	v := h.model.Snapshot(h.now())
	h.mu.Unlock()
	b, _ := json.Marshal(v)
	return b
}

func (h *Hub) subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Run broadcasts coalesced updates, plus a periodic tick so derived state (ages, the
// stale-hook banner) moves even when nothing arrives.
func (h *Hub) Run(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.dirty:
			time.Sleep(150 * time.Millisecond) // coalesce bursts
		case <-tick.C:
		}
		snap := h.Snapshot()
		h.mu.Lock()
		for ch := range h.subs {
			select { // keep only the latest snapshot per slow subscriber
			case <-ch:
			default:
			}
			ch <- snap
		}
		h.mu.Unlock()
	}
}

// Server is the panel's HTTP surface.
type Server struct {
	Port    int
	Token   string
	Hub     *Hub
	Hooks   chan<- []byte // raw hook bodies, processed after the 204
	Status  chan<- []byte // raw status-line bodies
	Refresh func()        // re-poll every source now
	// Lanes controls lanes (v1). Nil when tmux is unavailable: the panel still watches.
	Lanes *LaneManager
	// CookieMaxAge, when > 0, makes the session cookie persistent (seconds). Used when
	// the panel runs under launchd with a persistent token.
	CookieMaxAge int

	termMu  sync.Mutex
	tickets map[string]termTicket               // single-use WebSocket tickets
	viewers map[string]map[*termViewer]struct{} // open terminals by lane id
}

func (s *Server) cookieName() string { return "clauductor_panel_" + strconv.Itoa(s.Port) }

func (s *Server) hostOK(host string) bool {
	p := strconv.Itoa(s.Port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

func (s *Server) originOK(origin string) bool {
	p := strconv.Itoa(s.Port)
	return origin == "http://127.0.0.1:"+p || origin == "http://localhost:"+p
}

func remoteIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.Token)) == 1
}

// Handler returns the full HTTP handler with every guard applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.ingest(s.Hooks))
	mux.HandleFunc("/status", s.ingest(s.Status))
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/events", s.requireAuth(s.events))
	mux.HandleFunc("/api/state", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.Hub.Snapshot())
	}))
	mux.HandleFunc("/api/refresh", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.Refresh != nil {
			s.Refresh()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	s.laneRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// DNS rebinding: a hostile name resolving to 127.0.0.1 still carries its own Host.
		if !s.hostOK(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !remoteIsLoopback(r) {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		// Every state-changing browser request must come from the panel's own page.
		if r.URL.Path != "/hook" && r.URL.Path != "/status" &&
			r.Method != http.MethodGet && r.Method != http.MethodHead && !s.originOK(r.Header.Get("Origin")) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			http.Error(w, "unauthorized: open the URL `clauductor panel` printed", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// ingest serves /hook and /status. They carry no token (a session cannot know it), so
// they are loopback-only, write-only, size-capped, and closed to browsers: Claude
// Code sends no Origin, while any cross-site browser POST does.
func (s *Server) ingest(out chan<- []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			http.Error(w, "not for browsers", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxIngestBody))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Answer first: a hook must never wait on the panel's own processing.
		w.WriteHeader(http.StatusNoContent)
		if out == nil {
			return
		}
		select {
		case out <- body:
		default: // the processor is behind; dropping one event beats blocking a session
		}
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if t := r.URL.Query().Get("t"); t != "" {
		if subtle.ConstantTimeCompare([]byte(t), []byte(s.Token)) != 1 {
			http.Error(w, "unauthorized: stale or wrong token", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.Token, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: s.CookieMaxAge})
		// Drop the token from the address bar and history.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.authed(r) {
		http.Error(w, "unauthorized: open the URL `clauductor panel` printed", http.StatusUnauthorized)
		return
	}
	page, _ := webFS.ReadFile("web/index.html")
	// No inline script or style is allowed, and nothing outside this origin. The one
	// exception is a per-response nonce for the <style> elements xterm.js creates at
	// run time; panel.js stamps it on them.
	nonce, err := NewToken()
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}
	nonce = nonce[:32]
	page = bytes.Replace(page, []byte("{{STYLE_NONCE}}"), []byte(nonce), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'nonce-"+nonce+"'; "+
		"font-src 'self'; connect-src 'self' ws://"+r.Host+"; img-src 'self' data:; "+
		"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Write(page)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Connection", "keep-alive")
	ch, cancel := s.Hub.subscribe()
	defer cancel()
	send := func(b []byte) error {
		_, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b)
		fl.Flush()
		return err
	}
	fmt.Fprint(w, "retry: 2000\n\n")
	if send(s.Hub.Snapshot()) != nil {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			if send(b) != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
