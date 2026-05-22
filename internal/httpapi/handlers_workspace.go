package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

// defaultRetryPolicyJSON is applied when a workspace/agent create request omits
// retry_policy_json. We default to 3 attempts with exponential backoff on
// timeout/executor_error so that transient environment issues self-recover.
const defaultRetryPolicyJSON = `{"max_attempts":3,"backoff_seconds":[10,60,300],"retry_on":["timeout","executor_error"]}`

// ensureWorkspaceWorkingDir returns a working_dir path that exists on disk.
// If the caller did not specify one, it is auto-populated to
// <data_dir>/workdirs/<slug> and the directory is created. This guards against
// codex/claude/gemini failures caused by empty CWD (see RFP-1 incident).
func (s *Server) ensureWorkspaceWorkingDir(workingDir, slug string) (string, error) {
	working := strings.TrimSpace(workingDir)
	if working == "" {
		if s.cfg.DataDir == "" || slug == "" {
			return working, nil
		}
		working = filepath.Join(s.cfg.DataDir, "workdirs", slug)
	}
	if err := os.MkdirAll(working, 0o755); err != nil {
		return "", err
	}
	return working, nil
}

func (s *Server) registerWorkspaceRoutes(api chi.Router) {
	api.Get("/api/workspaces", s.listWorkspaces)
	api.Post("/api/workspaces", s.createWorkspace)
	api.Get("/api/workspaces/{workspace}", s.getWorkspace)
	api.Put("/api/workspaces/{workspace}", s.updateWorkspace)
	api.Delete("/api/workspaces/{workspace}", s.deleteWorkspace)
	api.Get("/api/workspaces/{workspace}/export", s.exportWorkspace)
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
		AutoCloseOnRunDone       *bool                  `json:"auto_close_on_run_done"`
		MainAgent                store.CreateAgentInput `json:"main_agent"`
	}
	if !decode(w, r, &req) {
		return
	}
	workingDir, err := s.ensureWorkspaceWorkingDir(req.WorkingDir, req.Slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WORKING_DIR_SETUP_FAILED", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.MainAgent.RetryPolicyJSON) == "" {
		req.MainAgent.RetryPolicyJSON = defaultRetryPolicyJSON
	}
	ws, agent, err := s.store.CreateWorkspaceWithMainAgent(r.Context(), store.CreateWorkspaceInput{Name: req.Name, Slug: req.Slug, Description: req.Description, IdentifierPrefix: req.IdentifierPrefix, WorkingDir: workingDir, OutputDir: req.OutputDir, DefaultTimeoutSeconds: req.DefaultTimeoutSeconds, AutoChainEnabled: req.AutoChainEnabled, AutoChainMaxDepth: req.AutoChainMaxDepth, AutoChainDailyRunLimit: req.AutoChainDailyRunLimit, AutoChainDailyCostMicros: req.AutoChainDailyCostMicros, AutoChainDryRun: req.AutoChainDryRun, AutoCloseOnRunDone: req.AutoCloseOnRunDone, MainAgent: req.MainAgent})
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

// exportWorkspace streams the v2 JSON snapshot. `include_history=1` includes
// the issue/run/comment/attachment slices; `mask_pii=1` redacts email and
// phone fragments before writing them out. The response is streamed with a
// Content-Disposition attachment header so operators can save it directly
// from the browser.
func (s *Server) exportWorkspace(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "workspace")
	opts := app.ExportWorkspaceOptions{
		IncludeHistory: parseBool(r.URL.Query().Get("include_history")),
		MaskPII:        parseBool(r.URL.Query().Get("mask_pii")),
	}
	export, err := app.ExportWorkspaceWithOptions(r.Context(), s.store, slug, opts)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	filename := slug + ".workspace.json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(export)
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
