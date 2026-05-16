package httpapi

import (
	"net/http"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerWorkspaceRoutes(api chi.Router) {
	api.Get("/api/workspaces", s.listWorkspaces)
	api.Post("/api/workspaces", s.createWorkspace)
	api.Get("/api/workspaces/{workspace}", s.getWorkspace)
	api.Put("/api/workspaces/{workspace}", s.updateWorkspace)
	api.Delete("/api/workspaces/{workspace}", s.deleteWorkspace)
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListWorkspaces(r.Context())
	respond(w, map[string]any{"workspaces": xs}, err, http.StatusOK)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                     string                 `json:"name"`
		Slug                     string                 `json:"slug"`
		Description              string                 `json:"description"`
		IdentifierPrefix         string                 `json:"identifier_prefix"`
		WorkingDir               string                 `json:"working_dir"`
		OutputDir                string                 `json:"output_dir"`
		DefaultTimeoutSeconds    int                    `json:"default_timeout_seconds"`
		AutoChainEnabled         bool                   `json:"auto_chain_enabled"`
		AutoChainMaxDepth        int                    `json:"auto_chain_max_depth"`
		AutoChainDailyRunLimit   *int                   `json:"auto_chain_daily_run_limit"`
		AutoChainDailyCostMicros int64                  `json:"auto_chain_daily_cost_micros"`
		AutoChainDryRun          bool                   `json:"auto_chain_dry_run"`
		MainAgent                store.CreateAgentInput `json:"main_agent"`
	}
	if !decode(w, r, &req) {
		return
	}
	ws, agent, err := s.store.CreateWorkspaceWithMainAgent(r.Context(), store.CreateWorkspaceInput{Name: req.Name, Slug: req.Slug, Description: req.Description, IdentifierPrefix: req.IdentifierPrefix, WorkingDir: req.WorkingDir, OutputDir: req.OutputDir, DefaultTimeoutSeconds: req.DefaultTimeoutSeconds, AutoChainEnabled: req.AutoChainEnabled, AutoChainMaxDepth: req.AutoChainMaxDepth, AutoChainDailyRunLimit: req.AutoChainDailyRunLimit, AutoChainDailyCostMicros: req.AutoChainDailyCostMicros, AutoChainDryRun: req.AutoChainDryRun, MainAgent: req.MainAgent})
	respond(w, map[string]any{"workspace": ws, "main_agent": agent}, err, http.StatusCreated)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	respond(w, map[string]any{"workspace": ws}, err, http.StatusOK)
}

func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req store.UpdateWorkspaceInput
	if !decode(w, r, &req) {
		return
	}
	ws, err := s.store.UpdateWorkspace(r.Context(), chi.URLParam(r, "workspace"), req)
	respond(w, map[string]any{"workspace": ws}, err, http.StatusOK)
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "workspace")
	err := s.store.DeleteWorkspace(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}
