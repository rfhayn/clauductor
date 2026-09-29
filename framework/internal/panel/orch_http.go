package panel

import (
	"encoding/json"
	"net/http"
)

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
