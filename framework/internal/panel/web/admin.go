package web

import (
	"context"
	"net/http"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// PANEL-22: the project menu adds, trusts and removes projects. These routes change
// which repositories the panel serves and whose commands it may run, so each passes
// the guards every POST does (the Host allow-list, this page's Origin, the cookie),
// takes a strict JSON body (unknown fields refused, 16 KB at most) and names no
// command. Validating a path and previewing `panel init` read files and run git's
// read-only plumbing; the trust report renders the config. Nothing a repository names
// runs until "Trust and add" (or Trust config…) records its exact bytes, which the
// request must name by hash.

// ProjectAdmin is what the routes need from the panel's live project registry.
type ProjectAdmin interface {
	// Validate checks a path the page names and says what adding it would do.
	Validate(ctx context.Context, path string) (any, *lanes.LaneError)
	// InitPreview is what `panel init` would write there, and why. It writes nothing.
	InitPreview(ctx context.Context, path string) (any, *lanes.LaneError)
	// Init writes it, never over an existing file.
	Init(ctx context.Context, path string) (any, *lanes.LaneError)
	// Add registers the repository and serves it at once; with trust, it first
	// trusts the config, only if its bytes still hash to hash.
	Add(ctx context.Context, req AddProjectRequest) (any, *lanes.LaneError)
	// Remove unregisters a project and stops serving it, or with dryRun says what
	// that would do (and what refuses it: its lanes).
	Remove(ctx context.Context, id string, dryRun bool) (any, *lanes.LaneError)
	// Trust trusts a served project's config (only its bytes that hash to hash), or
	// with dryRun returns its trust report.
	Trust(ctx context.Context, id string, dryRun bool, hash string) (any, *lanes.LaneError)
}

// PathRequest names a repository by the path someone typed.
type PathRequest struct {
	Path string `json:"path"`
}

// AddProjectRequest is "Trust and add" (Trust, with the hash its report showed) or
// "Add without trusting".
type AddProjectRequest struct {
	Path  string `json:"path"`
	Trust bool   `json:"trust"`
	Hash  string `json:"hash"`
}

// RemoveProjectRequest is the confirmation's plan (DryRun) or the removal.
type RemoveProjectRequest struct {
	DryRun bool `json:"dryRun"`
}

// TrustProjectRequest is the trust report (DryRun) or the trust of its exact bytes.
type TrustProjectRequest struct {
	DryRun bool   `json:"dryRun"`
	Hash   string `json:"hash"`
}

func (s *Server) adminRoutes(mux *http.ServeMux) {
	path := func(do func(ProjectAdmin, context.Context, string) (any, *lanes.LaneError)) http.HandlerFunc {
		return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			var req PathRequest
			if !s.adminOK(w) || !decodeStrict(w, r, &req) {
				return
			}
			res, lerr := do(s.Admin, r.Context(), req.Path)
			answer(w, res, lerr)
		})
	}
	mux.HandleFunc("POST /api/projects/validate", path(ProjectAdmin.Validate))
	mux.HandleFunc("POST /api/projects/init-preview", path(ProjectAdmin.InitPreview))
	mux.HandleFunc("POST /api/projects/init", path(ProjectAdmin.Init))
	mux.HandleFunc("POST /api/projects/add", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		var req AddProjectRequest
		if !s.adminOK(w) || !decodeStrict(w, r, &req) {
			return
		}
		if req.Trust && !validHash(req.Hash) {
			writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "trust needs the hash of the config its report showed"))
			return
		}
		res, lerr := s.Admin.Add(r.Context(), req)
		answer(w, res, lerr)
	}))
	mux.HandleFunc("POST /api/projects/{id}/remove", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		var req RemoveProjectRequest
		id, ok := s.adminID(w, r)
		if !ok || !decodeStrict(w, r, &req) {
			return
		}
		res, lerr := s.Admin.Remove(r.Context(), id, req.DryRun)
		answer(w, res, lerr)
	}))
	mux.HandleFunc("POST /api/projects/{id}/trust", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		var req TrustProjectRequest
		id, ok := s.adminID(w, r)
		if !ok || !decodeStrict(w, r, &req) {
			return
		}
		if !req.DryRun && !validHash(req.Hash) {
			writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "trust needs the hash of the config its report showed"))
			return
		}
		res, lerr := s.Admin.Trust(r.Context(), id, req.DryRun, req.Hash)
		answer(w, res, lerr)
	}))
}

func (s *Server) adminOK(w http.ResponseWriter) bool {
	if s.Admin == nil {
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "this panel does not add or remove projects"))
		return false
	}
	return true
}

func (s *Server) adminID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.adminOK(w) {
		return "", false
	}
	id := r.PathValue("id")
	if !config.ProjectIDRe.MatchString(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid project id"))
		return "", false
	}
	return id, true
}

// validHash: a config's hex SHA-256.
func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func answer(w http.ResponseWriter, res any, lerr *lanes.LaneError) {
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}
