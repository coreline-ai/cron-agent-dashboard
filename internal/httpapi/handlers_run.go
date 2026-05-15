package httpapi

import (
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerRunRoutes(api chi.Router) {
	api.Get("/api/issues/{id}/runs", s.listRuns)
	api.Get("/api/runs/{id}/events", s.listRunEvents)
	api.Get("/api/runs/{id}/log", s.runLog)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListRuns(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"runs": xs}, err, http.StatusOK)
}

func (s *Server) listRunEvents(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListRunEvents(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"events": xs}, err, http.StatusOK)
}

func (s *Server) runLog(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetRunLogPath(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(p)+"\"")
	http.ServeFile(w, r, p)
}
