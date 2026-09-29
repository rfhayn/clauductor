package panel

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
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
	web, _ := fs.Sub(webFS, "web")
	files := http.FileServerFS(web)
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

func writeLaneErr(w http.ResponseWriter, e *LaneError) {
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
	var req StartRequest
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
	var lerr *LaneError
	switch r.PathValue("action") {
	case "stop":
		lerr = s.Lanes.Stop(r.Context(), id)
	case "interrupt":
		lerr = s.Lanes.Interrupt(r.Context(), id)
	case "restart":
		lerr = s.Lanes.Restart(r.Context(), id)
	case "resume":
		if lerr = s.Lanes.Resume(r.Context(), id); lerr == nil {
			s.Lanes.markRestored(id)
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
