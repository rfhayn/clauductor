package panel

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lease"
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
//
// It pushes a snapshot at once only when what the view says changed (viewKey).
// Every age on the page is computed in the browser from a timestamp, and the
// bookkeeping of the polls (when each source was last read, the footer's counters,
// a lease's renewal) moves every 2 s while nothing happens: none of that is news.
// It still reaches the page, on the 5 s tick, if it is all that changed. The SSE
// handler sends its own heartbeat (HeartbeatEvery) so the page can still tell a
// quiet panel from a dead one.
type Hub struct {
	mu    sync.Mutex
	model *Model
	now   func() time.Time
	subs  map[chan []byte]struct{}
	dirty chan struct{}
	// last is the key (viewKey) of the last snapshot broadcast, and lastFull the
	// key of all of it (fullKey).
	last, lastFull [sha256.Size]byte
	sent           bool
	// Coalesce is how long an update waits for more before the push. Zero means 150 ms.
	Coalesce time.Duration
	// TickEvery is how often derived state (stale hooks, approximate readings) is
	// re-derived when nothing arrives. Zero means 5 s.
	TickEvery time.Duration
	// pushes counts broadcasts, for tests and the footer.
	pushes atomic.Int64
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
	b, _, _ := h.snapshotKeyed()
	return b
}

// snapshotKeyed returns the current view as JSON, its key and its full key.
func (h *Hub) snapshotKeyed() ([]byte, [sha256.Size]byte, [sha256.Size]byte) {
	h.mu.Lock()
	v := h.model.Snapshot(h.now())
	h.mu.Unlock()
	b, _ := json.Marshal(v)
	return b, viewKey(v), fullKey(v)
}

// fullKey identifies everything in a view but the clock it was taken at.
func fullKey(v View) [sha256.Size]byte {
	v.Now = 0
	b, _ := json.Marshal(v)
	return sha256.Sum256(b)
}

// viewKey identifies what a view says: fullKey without the polls' bookkeeping.
// Left out: when each source was last read (not whether it can be), when
// `claude agents` last answered, the footer's counters and the top bar's hook
// count, a card's run time, the quota's arrival time, and a lease's renewal.
func viewKey(v View) [sha256.Size]byte {
	v.Now, v.AgentsReadAt, v.HookEvents, v.StatusPosts, v.Dropped = 0, 0, 0, 0, 0
	v.Observe = ObsView{}
	src := make(map[string]SourceStatus, len(v.Sources))
	for k, s := range v.Sources {
		s.At = 0
		src[k] = s
	}
	v.Sources = src
	v.QueuesSrc.At = 0
	if v.Quota != nil {
		q := *v.Quota
		q.At = 0
		v.Quota = &q
	}
	cards := make([]CardState, len(v.Cards))
	for i, c := range v.Cards {
		c.Source.At = 0
		cards[i] = c
	}
	v.Cards = cards
	qs := make([]lease.QueueView, len(v.Queues))
	for i, q := range v.Queues {
		if q.Holder != nil {
			h := *q.Holder
			h.Renewed = 0
			q.Holder = &h
		}
		ws := make([]lease.LeaseView, len(q.Waiters))
		for j, w := range q.Waiters {
			w.Renewed = 0
			ws[j] = w
		}
		q.Waiters = ws
		qs[i] = q
	}
	v.Queues = qs
	return fullKey(v)
}

// Pushes is how many snapshots the hub has broadcast.
func (h *Hub) Pushes() int64 { return h.pushes.Load() }

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

// Run broadcasts coalesced updates, plus a periodic re-derivation so state that
// moves with time alone (the stale-hook banner, an approximate reading) still
// reaches the page. Either way a view equal to the last one pushed is not pushed.
func (h *Hub) Run(ctx context.Context) {
	every := h.TickEvery
	if every <= 0 {
		every = 5 * time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.dirty:
			wait := h.Coalesce
			if wait <= 0 {
				wait = 150 * time.Millisecond
			}
			time.Sleep(wait) // coalesce bursts
			h.broadcast(false)
		case <-tick.C:
			h.broadcast(true)
		}
	}
}

// broadcast pushes the current view to every subscriber if what it says changed,
// or, on a tick, if anything in it changed.
func (h *Hub) broadcast(tick bool) {
	snap, key, full := h.snapshotKeyed()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sent && key == h.last && (!tick || full == h.lastFull) {
		return
	}
	h.last, h.lastFull, h.sent = key, full, true
	h.pushes.Add(1)
	for ch := range h.subs {
		select { // keep only the latest snapshot per slow subscriber
		case <-ch:
		default:
		}
		ch <- snap
	}
}

// HeartbeatEvery is how often an open /events stream says it is alive, with the
// server's clock. The page treats three missed beats as a lost connection.
var HeartbeatEvery = 5 * time.Second

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

	// TermIdleTimeout closes a terminal whose page has sent nothing (not even its
	// once-a-minute "alive" while visible) for this long. Zero means 5 minutes.
	TermIdleTimeout time.Duration

	// beforeAddViewer, if set, runs between a terminal's auth and its registration
	// (tests use it to rotate the token in that window).
	beforeAddViewer func()

	tokenMu sync.RWMutex
	rotated chan struct{} // closed, and replaced, on each token rotation

	// HostNames are extra <label>.localhost names the panel answers to (hosts.go).
	HostNames []string

	// Orch carries the v2 orchestration (templates, quota guard, queues, restore).
	Orch *Orchestration

	// overflow counts ingest bodies dropped because the processor was behind; it is
	// shown apart from events dropped for a foreign cwd.
	overflow atomic.Int64

	termMu  sync.Mutex
	tickets map[string]termTicket               // single-use WebSocket tickets
	viewers map[string]map[*termViewer]struct{} // open terminals by lane id
}

func (s *Server) cookieName() string { return "clauductor_panel_" + strconv.Itoa(s.Port) }

// hostOK and originOK use the exact allow-list in hosts.go.
func (s *Server) hostOK(host string) bool { return s.hostAllowed(host) }

func (s *Server) originOK(origin, host string) bool { return s.originMatches(origin, host) }

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
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.currentToken())) == 1
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
			r.Method != http.MethodGet && r.Method != http.MethodHead && !s.originOK(r.Header.Get("Origin"), r.Host) {
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
			s.overflow.Add(1)
		}
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if t := r.URL.Query().Get("t"); t != "" {
		token := s.currentToken()
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) != 1 {
			http.Error(w, "unauthorized: stale or wrong token", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: token, Path: "/",
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
	rotated := s.rotation() // a token rotation ends this stream; the page's reconnect then gets 401
	send := func(b []byte) error {
		_, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b)
		fl.Flush()
		return err
	}
	fmt.Fprint(w, "retry: 2000\n\n")
	if send(s.Hub.Snapshot()) != nil {
		return
	}
	ping := time.NewTicker(HeartbeatEvery)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-rotated:
			return
		case b := <-ch:
			if send(b) != nil {
				return
			}
		case <-ping.C:
			// A named event, not a comment: EventSource hides comments from the page,
			// and the page needs the beat (and the server's clock) to know it is live.
			if _, err := fmt.Fprintf(w, "event: hb\ndata: {\"now\":%d}\n\n", s.Hub.now().UnixMilli()); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) currentToken() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.Token
}

// rotation returns a channel that is closed at the next token rotation.
func (s *Server) rotation() <-chan struct{} {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.rotated == nil {
		s.rotated = make(chan struct{})
	}
	return s.rotated
}

// Rotate replaces the token. Every cookie issued for the old one stops working at
// once, every open terminal is closed (code 4001) and every event stream ends, so
// nothing opened with the old token outlives it.
func (s *Server) Rotate(token string) {
	s.tokenMu.Lock()
	if token == s.Token {
		s.tokenMu.Unlock()
		return
	}
	s.Token = token
	if s.rotated != nil {
		close(s.rotated)
	}
	s.rotated = make(chan struct{})
	s.tokenMu.Unlock()
	s.termMu.Lock()
	s.tickets = nil // a ticket issued under the old token dies with it
	s.termMu.Unlock()
	s.closeAllTerminals(4001, "token rotated")
}
