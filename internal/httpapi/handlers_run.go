package httpapi

import (
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func (s *Server) registerRunRoutes(api chi.Router) {
	api.Get("/api/issues/{id}/runs", s.listRuns)
	api.Get("/api/runs/{id}/events", s.listRunEvents)
	api.Get("/api/runs/{id}/log", s.runLog)
	api.Post("/api/runs/chain/{chain}/cancel", s.cancelChain)
	api.Post("/api/runs/chain/{chain}/retry", s.retryChain)
}

// retryChain finds the most recent failed run in the chain and enqueues a
// new run on the same agent with the same chain_id / chain_depth. Returns
// 404 when the chain has no failed run, 409 when the chain still has
// queued/running runs (the operator must cancel first).
func (s *Server) retryChain(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chain")
	run, err := s.store.RetryFailedRunInChain(r.Context(), chainID)
	respond(w, map[string]any{"run": run}, err, http.StatusCreated)
}

func (s *Server) cancelChain(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chain")
	// Wake the worker pool first for any in-flight run on this chain so the
	// process group exits before the store marks the row cancelled. Failures
	// here are non-fatal — the store sweep below still cancels the row.
	if s.runCanceller != nil {
		runs, listErr := s.store.ListRunsByChain(r.Context(), chainID)
		if listErr == nil {
			for _, run := range runs {
				if run.Status == "running" {
					s.runCanceller.CancelRun(run.ID)
				}
			}
		}
	}
	cancelled, err := s.store.CancelRunsByChain(r.Context(), chainID, store.CancelReasonInput{
		Message:        "user cancelled the chain",
		TerminalReason: store.TerminalReasonUserCancelled,
		CancelReason:   store.CancelReasonUser,
	})
	respond(w, map[string]any{"chain_id": chainID, "cancelled": cancelled}, err, http.StatusOK)
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
