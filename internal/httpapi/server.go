package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

const Version = "0.1.0"

type Server struct {
	store     *store.Store
	cfg       config.Config
	startedAt time.Time
}

func New(st *store.Store, cfg config.Config) http.Handler {
	s := &Server{store: st, cfg: cfg, startedAt: time.Now()}
	r := chi.NewRouter()
	r.Use(s.cors)
	r.Get("/healthz", s.healthz)
	r.Group(func(api chi.Router) {
		api.Use(s.auth)
		api.Get("/api/settings", s.settings)
		api.Post("/api/system/backup", s.backup)
		api.Post("/api/system/vacuum", s.vacuum)
		api.Post("/api/system/cleanup-logs", s.cleanupLogs)

		api.Get("/api/workspaces", s.listWorkspaces)
		api.Post("/api/workspaces", s.createWorkspace)
		api.Get("/api/workspaces/{workspace}", s.getWorkspace)
		api.Put("/api/workspaces/{workspace}", s.updateWorkspace)
		api.Delete("/api/workspaces/{workspace}", s.deleteWorkspace)
		api.Get("/api/workspaces/{workspace}/agents", s.listAgents)
		api.Post("/api/workspaces/{workspace}/agents", s.createAgent)
		api.Get("/api/workspaces/{workspace}/issues", s.listIssues)
		api.Post("/api/workspaces/{workspace}/issues", s.createIssue)
		api.Get("/api/workspaces/{workspace}/issues/{issue}", s.getWorkspaceIssue)
		api.Get("/api/workspaces/{workspace}/autopilot", s.listAutopilot)
		api.Post("/api/workspaces/{workspace}/autopilot", s.createAutopilot)

		api.Get("/api/agents/{id}", s.getAgent)
		api.Put("/api/agents/{id}", s.updateAgent)
		api.Post("/api/agents/{id}/promote", s.promoteAgent)
		api.Delete("/api/agents/{id}", s.deleteAgent)

		api.Get("/api/issues/{id}", s.getIssue)
		api.Put("/api/issues/{id}", s.updateIssue)
		api.Post("/api/issues/{id}/rerun", s.rerunIssue)
		api.Post("/api/issues/{id}/cancel", s.cancelIssueRun)
		api.Delete("/api/issues/{id}", s.deleteIssue)
		api.Get("/api/issues/{id}/comments", s.listComments)
		api.Post("/api/issues/{id}/comments", s.addComment)
		api.Get("/api/issues/{id}/runs", s.listRuns)

		api.Delete("/api/comments/{id}", s.deleteComment)
		api.Get("/api/runs/{id}/log", s.runLog)
		api.Put("/api/autopilot/{id}", s.updateAutopilot)
		api.Delete("/api/autopilot/{id}", s.deleteAutopilot)
		api.Post("/api/autopilot/{id}/trigger", s.triggerAutopilot)
	})
	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range s.cfg.CORS {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowed[origin] || len(allowed) == 0) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token != "" {
			want := "Bearer " + s.cfg.Token
			if r.Header.Get("Authorization") != want {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.DB().PingContext(r.Context()) == nil
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime_seconds": int64(time.Since(s.startedAt).Seconds()), "db_ok": dbOK, "available_runtimes": availableRuntimeNames()})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": Version, "data_dir": s.cfg.DataDir, "available_runtimes": availableRuntimes(), "worker_pool_size": s.cfg.Workers, "auth_mode": s.cfg.AuthMode(), "timezone": s.cfg.Timezone})
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListWorkspaces(r.Context())
	respond(w, map[string]any{"workspaces": xs}, err, http.StatusOK)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string                 `json:"name"`
		Slug             string                 `json:"slug"`
		Description      string                 `json:"description"`
		IdentifierPrefix string                 `json:"identifier_prefix"`
		WorkingDir       string                 `json:"working_dir"`
		OutputDir        string                 `json:"output_dir"`
		MainAgent        store.CreateAgentInput `json:"main_agent"`
	}
	if !decode(w, r, &req) {
		return
	}
	ws, agent, err := s.store.CreateWorkspaceWithMainAgent(r.Context(), store.CreateWorkspaceInput{Name: req.Name, Slug: req.Slug, Description: req.Description, IdentifierPrefix: req.IdentifierPrefix, WorkingDir: req.WorkingDir, OutputDir: req.OutputDir, MainAgent: req.MainAgent})
	respond(w, map[string]any{"workspace": ws, "main_agent": agent}, err, http.StatusCreated)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	respond(w, map[string]any{"workspace": ws}, err, http.StatusOK)
}

func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		WorkingDir  string `json:"working_dir"`
		OutputDir   string `json:"output_dir"`
	}
	if !decode(w, r, &req) {
		return
	}
	ws, err := s.store.UpdateWorkspace(r.Context(), chi.URLParam(r, "workspace"), req.Name, req.Description, req.WorkingDir, req.OutputDir)
	respond(w, map[string]any{"workspace": ws}, err, http.StatusOK)
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "workspace")
	err := s.store.DeleteWorkspace(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
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

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	xs, err := s.store.ListIssues(r.Context(), ws.ID, store.ListIssuesFilter{Status: split(q.Get("status")), Execution: split(q.Get("execution")), Assignee: q.Get("assignee"), Query: q.Get("q"), Limit: limit})
	respond(w, map[string]any{"issues": xs, "next_cursor": nil}, err, http.StatusOK)
}
func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	var req struct {
		Title           string `json:"title"`
		Body            string `json:"body"`
		AssigneeAgentID string `json:"assignee_agent_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	issue, _, err := s.store.CreateIssueWithInitialRun(r.Context(), ws.ID, store.CreateIssueInput{Title: req.Title, Body: req.Body, AssigneeAgentID: req.AssigneeAgentID})
	respond(w, map[string]any{"issue": issue}, err, http.StatusCreated)
}
func (s *Server) getWorkspaceIssue(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	iss, err := s.store.LookupIssue(r.Context(), ws.ID, chi.URLParam(r, "issue"))
	respond(w, map[string]any{"issue": iss}, err, http.StatusOK)
}
func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	iss, err := s.store.GetIssue(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"issue": iss}, err, http.StatusOK)
}
func (s *Server) updateIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title           string `json:"title"`
		Body            string `json:"body"`
		AssigneeAgentID string `json:"assignee_agent_id"`
		Status          string `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	body := req.Body
	if body == "" {
		body = "\x00"
	}
	iss, err := s.store.UpdateIssue(r.Context(), chi.URLParam(r, "id"), req.Title, body, req.AssigneeAgentID, req.Status)
	respond(w, map[string]any{"issue": iss}, err, http.StatusOK)
}
func (s *Server) rerunIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	run, err := s.store.RerunIssue(r.Context(), chi.URLParam(r, "id"), req.AgentID)
	respond(w, map[string]any{"run": run}, err, http.StatusCreated)
}
func (s *Server) cancelIssueRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.CancelRunningRun(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"run": run}, err, http.StatusOK)
}
func (s *Server) deleteIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteIssue(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListComments(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"comments": xs}, err, http.StatusOK)
}
func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := s.store.AddUserComment(r.Context(), chi.URLParam(r, "id"), req.Content)
	respond(w, res, err, http.StatusCreated)
}
func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteComment(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}
func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListRuns(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"runs": xs}, err, http.StatusOK)
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
	respond(w, map[string]any{"rule": rule}, err, http.StatusCreated)
}
func (s *Server) updateAutopilot(w http.ResponseWriter, r *http.Request) {
	var req store.UpsertAutopilotInput
	if !decode(w, r, &req) {
		return
	}
	rule, err := s.store.UpdateAutopilotRule(r.Context(), chi.URLParam(r, "id"), req)
	respond(w, map[string]any{"rule": rule}, err, http.StatusOK)
}
func (s *Server) deleteAutopilot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteAutopilotRule(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}
func (s *Server) triggerAutopilot(w http.ResponseWriter, r *http.Request) {
	issue, _, err := s.store.TriggerAutopilotRule(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"issue": issue}, err, http.StatusCreated)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.To == "" {
		req.To = s.cfg.DBPath + "." + time.Now().UTC().Format("20060102T150405Z") + ".bak"
	}
	in, err := os.Open(s.cfg.DBPath)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	defer in.Close()
	out, err := os.Create(req.To)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	defer out.Close()
	n, err := io.Copy(out, in)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_path": req.To, "size_bytes": n})
}
func (s *Server) vacuum(w http.ResponseWriter, r *http.Request) {
	_, err := s.store.DB().ExecContext(r.Context(), `PRAGMA incremental_vacuum`)
	respond(w, map[string]any{"reclaimed_bytes": 0}, err, http.StatusOK)
}
func (s *Server) cleanupLogs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Days <= 0 {
		req.Days = 30
	}
	cutoff := time.Now().Add(-time.Duration(req.Days) * 24 * time.Hour)
	var deleted int
	var freed int64
	filepath.Walk(filepath.Join(s.cfg.DataDir, "runs"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.ModTime().Before(cutoff) {
			freed += info.Size()
			if os.Remove(p) == nil {
				deleted++
			}
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{"deleted_files": deleted, "freed_bytes": freed})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json", nil)
		return false
	}
	return true
}
func respond(w http.ResponseWriter, payload any, err error, success int) {
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if success == 0 {
		success = http.StatusOK
	}
	writeJSON(w, success, payload)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrValidation):
		writeError(w, 400, "VALIDATION_ERROR", err.Error(), nil)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, 404, "NOT_FOUND", "not found", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, 409, "CONFLICT", err.Error(), nil)
	case errors.Is(err, store.ErrState):
		writeError(w, 409, "STATE_ERROR", err.Error(), nil)
	default:
		writeError(w, 500, "INTERNAL_ERROR", err.Error(), nil)
	}
}
func writeError(w http.ResponseWriter, status int, code, msg string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": details}})
}
func split(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type RuntimeInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

func availableRuntimeNames() []string {
	infos := availableRuntimes()
	out := make([]string, 0, len(infos))
	for _, i := range infos {
		out = append(out, i.Name)
	}
	return out
}
func availableRuntimes() []RuntimeInfo {
	names := []string{"codex", "claude", "gemini"}
	out := []RuntimeInfo{}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			out = append(out, RuntimeInfo{Name: n, Path: p, Version: runtimeVersion(context.Background(), p)})
		}
	}
	return out
}
func runtimeVersion(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	b, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
