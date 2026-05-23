package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// webhookView mirrors store.Webhook but exposes a `has_secret` boolean
// instead of the secret value so the API never echoes secrets back to
// clients. PUT callers must supply the secret again to keep it; sending an
// empty string clears it.
type webhookView struct {
	ID                   string   `json:"id"`
	WorkspaceID          string   `json:"workspace_id"`
	URL                  string   `json:"url"`
	HasSecret            bool     `json:"has_secret"`
	Events               []string `json:"events"`
	Enabled              bool     `json:"enabled"`
	MaskPII              bool     `json:"mask_pii"`
	FailedDeliveryCount  int      `json:"failed_delivery_count"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

func newWebhookView(w store.Webhook, failed int) webhookView {
	return webhookView{
		ID:                  w.ID,
		WorkspaceID:         w.WorkspaceID,
		URL:                 w.URL,
		HasSecret:           w.Secret != "",
		Events:              w.Events,
		Enabled:             w.Enabled,
		MaskPII:             w.MaskPII,
		FailedDeliveryCount: failed,
		CreatedAt:           w.CreatedAt,
		UpdatedAt:           w.UpdatedAt,
	}
}

func (s *Server) registerWebhookRoutes(api chi.Router) {
	api.Get("/api/workspaces/{workspace}/webhooks", s.listWebhooks)
	api.Post("/api/workspaces/{workspace}/webhooks", s.createWebhook)
	api.Get("/api/webhooks/{id}", s.getWebhook)
	api.Put("/api/webhooks/{id}", s.updateWebhook)
	api.Delete("/api/webhooks/{id}", s.deleteWebhook)
	api.Get("/api/webhooks/{id}/deliveries", s.listWebhookDeliveries)
	api.Post("/api/webhooks/{id}/deliveries/{delivery}/redeliver", s.redeliverWebhookDelivery)
}

// redeliverWebhookDelivery flips a dead-letter row back to 'pending' so the
// dispatcher retries it on the next poll. The `id` URL parameter is the
// webhook ID and `delivery` is the delivery row ID; we resolve both so
// the response can return the same shape listWebhookDeliveries already
// emits and the UI does not need a second round-trip.
func (s *Server) redeliverWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "delivery")
	if err := s.store.ResubmitWebhookDelivery(r.Context(), deliveryID); err != nil {
		respond(w, nil, err, 0)
		return
	}
	respond(w, map[string]any{"redelivered": true, "id": deliveryID}, nil, http.StatusOK)
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	hooks, err := s.store.ListWebhooks(r.Context(), ws.ID)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	out := make([]webhookView, 0, len(hooks))
	for _, h := range hooks {
		failed, _ := s.store.CountWebhookDeliveryFailed(r.Context(), h.ID)
		out = append(out, newWebhookView(h, failed))
	}
	respond(w, map[string]any{"webhooks": out}, nil, http.StatusOK)
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	var req store.UpsertWebhookInput
	if !decode(w, r, &req) {
		return
	}
	hook, err := s.store.CreateWebhook(r.Context(), ws.ID, req)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	failed, _ := s.store.CountWebhookDeliveryFailed(r.Context(), hook.ID)
	respond(w, map[string]any{"webhook": newWebhookView(hook, failed)}, nil, http.StatusCreated)
}

func (s *Server) getWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := s.store.GetWebhook(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	failed, _ := s.store.CountWebhookDeliveryFailed(r.Context(), hook.ID)
	respond(w, map[string]any{"webhook": newWebhookView(hook, failed)}, nil, http.StatusOK)
}

func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var req store.UpsertWebhookInput
	if !decode(w, r, &req) {
		return
	}
	hook, err := s.store.UpdateWebhook(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	failed, _ := s.store.CountWebhookDeliveryFailed(r.Context(), hook.ID)
	respond(w, map[string]any{"webhook": newWebhookView(hook, failed)}, nil, http.StatusOK)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteWebhook(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}

func (s *Server) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	deliveries, err := s.store.ListWebhookDeliveries(r.Context(), chi.URLParam(r, "id"), limit)
	respond(w, map[string]any{"deliveries": deliveries}, err, http.StatusOK)
}
