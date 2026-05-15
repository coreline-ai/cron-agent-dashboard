package httpapi

import (
	"net/http"

	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerAgentRoutes(api chi.Router) {
	api.Get("/api/workspaces/{workspace}/agents", s.listAgents)
	api.Post("/api/workspaces/{workspace}/agents", s.createAgent)
	api.Get("/api/agents/{id}", s.getAgent)
	api.Get("/api/agents/{id}/instructions", s.listAgentInstructions)
	api.Put("/api/agents/{id}", s.updateAgent)
	api.Post("/api/agents/{id}/promote", s.promoteAgent)
	api.Delete("/api/agents/{id}", s.deleteAgent)
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	xs, err := s.store.ListAgents(r.Context(), ws.ID)
	respond(w, map[string]any{"agents": xs}, err, http.StatusOK)
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	var req store.CreateAgentInput
	if !decode(w, r, &req) {
		return
	}
	a, err := s.store.CreateAgent(r.Context(), ws.ID, req)
	respond(w, map[string]any{"agent": a}, err, http.StatusCreated)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAgent(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"agent": a}, err, http.StatusOK)
}

func (s *Server) listAgentInstructions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.store.ListAgentInstructionVersions(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"versions": versions}, err, http.StatusOK)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	var req store.CreateAgentInput
	if !decode(w, r, &req) {
		return
	}
	a, err := s.store.UpdateAgent(r.Context(), chi.URLParam(r, "id"), req)
	respond(w, map[string]any{"agent": a}, err, http.StatusOK)
}

func (s *Server) promoteAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.PromoteAgent(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"agent": a}, err, http.StatusOK)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteAgent(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}
