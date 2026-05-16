package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerAutopilotRoutes(api chi.Router) {
	api.Get("/api/workspaces/{workspace}/autopilot", s.listAutopilot)
	api.Post("/api/workspaces/{workspace}/autopilot", s.createAutopilot)
	api.Put("/api/autopilot/{id}", s.updateAutopilot)
	api.Delete("/api/autopilot/{id}", s.deleteAutopilot)
	api.Post("/api/autopilot/{id}/trigger", s.triggerAutopilot)
}

func (s *Server) listAutopilot(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	xs, err := s.store.ListAutopilotRules(r.Context(), ws.ID)
	respond(w, map[string]any{"rules": xs}, err, http.StatusOK)
}

func (s *Server) createAutopilot(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	var req store.UpsertAutopilotInput
	if !decode(w, r, &req) {
		return
	}
	rule, err := s.store.CreateAutopilotRule(r.Context(), ws.ID, req)
	if err == nil {
		s.reloadAutopilot(r.Context())
	}
	respond(w, map[string]any{"rule": rule}, err, http.StatusCreated)
}

func (s *Server) updateAutopilot(w http.ResponseWriter, r *http.Request) {
	var req store.UpsertAutopilotInput
	if !decode(w, r, &req) {
		return
	}
	rule, err := s.store.UpdateAutopilotRule(r.Context(), chi.URLParam(r, "id"), req)
	if err == nil {
		s.reloadAutopilot(r.Context())
	}
	respond(w, map[string]any{"rule": rule}, err, http.StatusOK)
}

func (s *Server) deleteAutopilot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteAutopilotRule(r.Context(), id)
	if err == nil {
		s.reloadAutopilot(r.Context())
	}
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}

func (s *Server) triggerAutopilot(w http.ResponseWriter, r *http.Request) {
	var result store.AutopilotTriggerResult
	var err error
	if s.autopilotManager != nil {
		result, err = s.autopilotManager.TriggerRuleResult(r.Context(), chi.URLParam(r, "id"))
	} else {
		result, err = s.store.TriggerAutopilotRuleResult(r.Context(), chi.URLParam(r, "id"))
	}
	if err != nil {
		if result.Rule.ID != "" && !errors.Is(err, store.ErrNotFound) {
			status := http.StatusInternalServerError
			if errors.Is(err, store.ErrValidation) {
				status = http.StatusBadRequest
			} else if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrState) {
				status = http.StatusConflict
			}
			writeError(w, status, "AUTOPILOT_TRIGGER_FAILED", store.AutopilotTriggerErrorMessage(err), map[string]any{"trigger_result": result})
			return
		}
		respond(w, nil, err, 0)
		return
	}
	respond(w, map[string]any{"trigger_result": result, "rule": result.Rule, "issue": result.Issue, "run": result.Run}, nil, http.StatusCreated)
}

func (s *Server) reloadAutopilot(ctx context.Context) {
	if s.autopilotManager != nil {
		_ = s.autopilotManager.Reload(ctx)
	}
}
