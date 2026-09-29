package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
)

// laneRoutes adds the v1 routes. Every one except /healthz needs the auth cookie; the
// POSTs also pass the global Origin check, and the WebSocket checks Origin itself.
func (s *Server) laneRoutes(mux *http.ServeMux) {
	// /healthz tells `clauductor panel open` and a person with curl that the panel is
	// up. It answers "ok" and nothing else, so it needs no token.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// The PID lets `clauductor panel open` check it is talking to THIS panel (the
		// one in ~/.clauductor/panel/pid) before it sends the token anywhere.
		fmt.Fprintf(w, "ok pid=%d\n", os.Getpid())
	})
	files := http.FileServerFS(webFS)
	mux.HandleFunc("GET /vendor/", s.requireAuth(files.ServeHTTP))
	mux.HandleFunc("GET /static/", s.requireAuth(files.ServeHTTP))
	mux.HandleFunc("POST /api/lanes/{id}/ticket", s.requireAuth(s.issueTicketHandler))
	// The terminal checks the cookie itself, after taking the rotation generation.
	mux.HandleFunc("GET /ws/term", s.terminal)
	mux.HandleFunc("POST /api/lanes", s.requireAuth(s.startLane))
	mux.HandleFunc("POST /api/lanes/{id}/{action}", s.requireAuth(s.laneAction))
	s.orchRoutes(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// laneErr is an expected failure the HTTP layer reports in the lanes' vocabulary.
func laneErr(status int, code, format string, a ...any) *lanes.LaneError {
	return &lanes.LaneError{Status: status, Code: code, Msg: fmt.Sprintf(format, a...)}
}

func writeLaneErr(w http.ResponseWriter, e *lanes.LaneError) {
	writeJSON(w, e.Status, map[string]any{"ok": false, "error": e.Msg, "code": e.Code})
}

func (s *Server) noLanes(w http.ResponseWriter) bool {
	if s.Lanes == nil {
		writeLaneErr(w, laneErr(http.StatusServiceUnavailable, "no-tmux", "lanes are unavailable: tmux was not found on the panel's PATH"))
		return true
	}
	return false
}

func (s *Server) startLane(w http.ResponseWriter, r *http.Request) {
	if s.noLanes(w) {
		return
	}
	var req lanes.StartRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "malformed-request", "body must be {type, mode, name, worktree?}: %v", err))
		return
	}
	res, lerr := s.Lanes.StartLane(r.Context(), req, s.Orch.startGate())
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "lane": res})
}

func (s *Server) laneAction(w http.ResponseWriter, r *http.Request) {
	if s.noLanes(w) {
		return
	}
	id := r.PathValue("id")
	if !config.ValidLaneID(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid lane id"))
		return
	}
	var lerr *lanes.LaneError
	switch r.PathValue("action") {
	case "stop":
		lerr = s.Lanes.Stop(r.Context(), id)
	case "interrupt":
		lerr = s.Lanes.Interrupt(r.Context(), id)
	case "restart":
		lerr = s.Lanes.Restart(r.Context(), id)
	case "resume":
		if lerr = s.Lanes.Resume(r.Context(), id); lerr == nil {
			s.Lanes.MarkRestored(id)
		}
	case "forget":
		lerr = s.Lanes.Forget(r.Context(), id)
	case "terminal-app":
		lerr = s.Lanes.OpenInTerminalApp(r.Context(), id)
	default:
		lerr = laneErr(http.StatusNotFound, "not-found", "unknown lane action")
	}
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "at": time.Now().UnixMilli()})
}

// orchRoutes adds the v2 routes. All need the cookie and, as POSTs, pass the global
// Origin check. Each is a fixed verb on a validated id; none takes a command.
func (s *Server) orchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/lanes/restore-all", s.requireAuth(s.restoreAll))
	mux.HandleFunc("POST /api/queues/{id}/cancel", s.requireAuth(s.queueCancel))
	mux.HandleFunc("POST /api/queues/{id}/run", s.requireAuth(s.queueRun))
}

func decodeStrict(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "malformed-request", "bad body: %v", err))
		return false
	}
	return true
}

func (s *Server) restoreAll(w http.ResponseWriter, r *http.Request) {
	if s.noLanes(w) {
		return
	}
	var req struct {
		OverrideQuota bool `json:"overrideQuota"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	var guard func() string
	if s.Orch != nil {
		guard = s.Orch.QuotaGuard
	}
	res, lerr := s.Lanes.RestoreAll(r.Context(), guard, req.OverrideQuota)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}

func (s *Server) queueCancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Waiter string `json:"waiter"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if s.Orch == nil || s.Orch.CancelWait == nil {
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no queues"))
		return
	}
	if err := s.Orch.CancelWait(r.PathValue("id"), req.Waiter); err != nil {
		writeLaneErr(w, laneErr(http.StatusConflict, "cancel", "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) queueRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Worktree string `json:"worktree"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if s.Orch == nil || s.Orch.RunQueue == nil {
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no queues"))
		return
	}
	run, err := s.Orch.RunQueue(r.Context(), r.PathValue("id"), req.Worktree)
	if err != nil {
		writeLaneErr(w, laneErr(http.StatusConflict, "run", "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run": run})
}

// Orchestration is what the HTTP layer needs from the v2 runtime.
type Orchestration struct {
	Trusted    func() bool
	QuotaGuard func() string
	CancelWait func(queue, nonce string) error
	RunQueue   func(ctx context.Context, queue, worktree string) (*lease.QueueRun, error)
}

func (o *Orchestration) startGate() lanes.StartGate {
	if o == nil {
		return lanes.StartGate{}
	}
	return lanes.StartGate{Trusted: o.Trusted, QuotaGuard: o.QuotaGuard}
}
