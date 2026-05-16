package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerIssueRoutes(api chi.Router) {
	api.Get("/api/workspaces/{workspace}/issues", s.listIssues)
	api.Post("/api/workspaces/{workspace}/issues", s.createIssue)
	api.Get("/api/workspaces/{workspace}/issues/{issue}", s.getWorkspaceIssue)
	api.Get("/api/issues/{id}", s.getIssue)
	api.Put("/api/issues/{id}", s.updateIssue)
	api.Post("/api/issues/{id}/rerun", s.rerunIssue)
	api.Post("/api/issues/{id}/cancel", s.cancelIssueRun)
	api.Delete("/api/issues/{id}", s.deleteIssue)
	api.Get("/api/issues/{id}/subissues", s.listSubIssues)
	api.Post("/api/issues/{id}/subissues", s.createSubIssue)
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
	issue, run, err := s.store.CreateIssueWithInitialRun(r.Context(), ws.ID, store.CreateIssueInput{Title: req.Title, Body: req.Body, AssigneeAgentID: req.AssigneeAgentID})
	respond(w, map[string]any{"issue": issue, "run": run}, err, http.StatusCreated)
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
	var req store.UpdateIssueInput
	if !decode(w, r, &req) {
		return
	}
	iss, err := s.store.UpdateIssue(r.Context(), chi.URLParam(r, "id"), req)
	respond(w, map[string]any{"issue": iss}, err, http.StatusOK)
}

func (s *Server) rerunIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if !decodeOptional(w, r, &req) {
		return
	}
	run, err := s.store.RerunIssue(r.Context(), chi.URLParam(r, "id"), req.AgentID)
	respond(w, map[string]any{"run": run}, err, http.StatusCreated)
}

func (s *Server) cancelIssueRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetActiveRunByIssue(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	if _, err := s.store.AppendRunEvent(r.Context(), store.RunEventInput{
		RunID:     run.ID,
		IssueID:   run.IssueID,
		EventType: store.RunEventCancelRequest,
		Message:   "Cancel requested",
		Details: map[string]any{
			"cancel_reason": store.CancelReasonUser,
		},
	}); err != nil {
		respond(w, nil, err, 0)
		return
	}
	processCancelRequested := false
	if s.runCanceller != nil {
		processCancelRequested = s.runCanceller.CancelRun(run.ID)
	}
	if processCancelRequested {
		respond(w, map[string]any{"run": run, "cancel_requested": true}, nil, http.StatusOK)
		return
	}
	cancelled, err := s.store.CancelRunWithReason(r.Context(), run.ID, store.CancelReasonInput{
		Message:        "user cancelled",
		TerminalReason: store.TerminalReasonUserCancelled,
		CancelReason:   store.CancelReasonUser,
	})
	if err == nil {
		s.forgetPendingRunCancelIfUnclaimed(cancelled)
	} else {
		s.forgetPendingRunCancelIfCurrentRunUnclaimed(r.Context(), run.ID)
	}
	respond(w, map[string]any{"run": cancelled, "cancel_requested": false}, err, http.StatusOK)
}

func (s *Server) forgetPendingRunCancelIfCurrentRunUnclaimed(ctx context.Context, runID string) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	s.forgetPendingRunCancelIfUnclaimed(run)
}

func (s *Server) forgetPendingRunCancelIfUnclaimed(run store.Run) {
	if s.runCanceller == nil || run.ClaimedAt != "" || run.StartedAt != "" {
		return
	}
	cleaner, ok := s.runCanceller.(PendingRunCancelCleaner)
	if !ok {
		return
	}
	cleaner.ForgetPendingCancel(run.ID)
}

func (s *Server) deleteIssue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteIssue(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}

func (s *Server) listSubIssues(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListSubIssues(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"issues": xs}, err, http.StatusOK)
}

func (s *Server) createSubIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title           string `json:"title"`
		Body            string `json:"body"`
		AssigneeAgentID string `json:"assignee_agent_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	issue, run, err := s.store.CreateSubIssue(r.Context(), chi.URLParam(r, "id"), store.CreateIssueInput{Title: req.Title, Body: req.Body, AssigneeAgentID: req.AssigneeAgentID})
	respond(w, map[string]any{"issue": issue, "run": run}, err, http.StatusCreated)
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
