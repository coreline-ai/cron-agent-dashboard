package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func (s *Server) registerSkillRoutes(api chi.Router) {
	api.Get("/api/workspaces/{workspace}/skills", s.listSkills)
	api.Post("/api/workspaces/{workspace}/skills", s.createSkill)
	api.Get("/api/skills/{id}", s.getSkill)
	api.Put("/api/skills/{id}", s.updateSkill)
	api.Delete("/api/skills/{id}", s.deleteSkill)
	api.Get("/api/agents/{id}/skills", s.listAgentSkills)
	api.Post("/api/agents/{id}/skills", s.assignAgentSkill)
	api.Delete("/api/agents/{id}/skills/{skillID}", s.deleteAgentSkill)
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	skills, err := s.store.ListSkills(r.Context(), ws.ID)
	respond(w, map[string]any{"skills": skills}, err, http.StatusOK)
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	var req store.UpsertSkillInput
	if !decode(w, r, &req) {
		return
	}
	skill, err := s.store.UpsertSkill(r.Context(), ws.ID, req)
	respond(w, map[string]any{"skill": skill}, err, http.StatusCreated)
}

func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	skill, err := s.store.GetSkill(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"skill": skill}, err, http.StatusOK)
}

func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	var req store.UpsertSkillInput
	if !decode(w, r, &req) {
		return
	}
	skill, err := s.store.UpdateSkill(r.Context(), chi.URLParam(r, "id"), req)
	respond(w, map[string]any{"skill": skill}, err, http.StatusOK)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteSkill(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}

func (s *Server) listAgentSkills(w http.ResponseWriter, r *http.Request) {
	assignments, err := s.store.ListAgentSkills(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"skills": assignments}, err, http.StatusOK)
}

func (s *Server) assignAgentSkill(w http.ResponseWriter, r *http.Request) {
	var req store.AssignAgentSkillInput
	if !decode(w, r, &req) {
		return
	}
	assignment, err := s.store.AssignAgentSkill(r.Context(), chi.URLParam(r, "id"), req)
	respond(w, map[string]any{"agent_skill": assignment}, err, http.StatusOK)
}

func (s *Server) deleteAgentSkill(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	skillID := chi.URLParam(r, "skillID")
	err := s.store.DeleteAgentSkill(r.Context(), agentID, skillID)
	respond(w, map[string]any{"deleted": true, "agent_id": agentID, "skill_id": skillID}, err, http.StatusOK)
}
