package store

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/jmoiron/sqlx"
)

// KnownWebhookEvents lists every lifecycle event the dispatcher knows how to
// emit. Callers that pass an unknown event into UpsertWebhookInput.Events
// trigger ErrValidation. Keep this list in sync with where
// EnqueueWebhookDeliveries is invoked (Phase 3).
var KnownWebhookEvents = []string{
	"run.completed",
	"run.failed",
	"issue.done",
	"issue.cancelled",
}

const webhookSelectBase = `SELECT id,workspace_id,url,secret,events_json,enabled,created_at,updated_at FROM webhook`

// CreateWebhook registers a new webhook subscription for a workspace.
func (s *Store) CreateWebhook(ctx context.Context, workspaceID string, in UpsertWebhookInput) (Webhook, error) {
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return Webhook{}, err
	}
	normalizedURL, eventsJSON, enabled, err := normalizeWebhookInput(in)
	if err != nil {
		return Webhook{}, err
	}
	t := now()
	w := Webhook{
		ID:          newID(),
		WorkspaceID: workspaceID,
		URL:         normalizedURL,
		Secret:      in.Secret,
		EventsJSON:  eventsJSON,
		Enabled:     enabled,
		CreatedAt:   t,
		UpdatedAt:   t,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO webhook(id,workspace_id,url,secret,events_json,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		w.ID, w.WorkspaceID, w.URL, w.Secret, w.EventsJSON, boolInt(w.Enabled), t, t,
	); err != nil {
		return Webhook{}, normalizeErr(err)
	}
	return s.GetWebhook(ctx, w.ID)
}

// ListWebhooks returns the webhooks registered for a workspace, ordered with
// enabled rows first (so the UI shows actionable items at the top).
func (s *Store) ListWebhooks(ctx context.Context, workspaceID string) ([]Webhook, error) {
	var rows []Webhook
	if err := s.db.SelectContext(ctx, &rows, webhookSelectBase+` WHERE workspace_id=? ORDER BY enabled DESC, created_at DESC`, workspaceID); err != nil {
		return nil, normalizeErr(err)
	}
	for i := range rows {
		if err := decodeWebhookEvents(&rows[i]); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *Store) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	var w Webhook
	if err := s.db.GetContext(ctx, &w, webhookSelectBase+` WHERE id=?`, id); err != nil {
		return Webhook{}, normalizeErr(err)
	}
	if err := decodeWebhookEvents(&w); err != nil {
		return Webhook{}, err
	}
	return w, nil
}

// UpdateWebhook replaces every mutable field of an existing webhook with the
// values in the input. Mirrors PUT /api/agents/:id's full-replace contract so
// the HTTP layer stays predictable.
func (s *Store) UpdateWebhook(ctx context.Context, id string, in UpsertWebhookInput) (Webhook, error) {
	existing, err := s.GetWebhook(ctx, id)
	if err != nil {
		return Webhook{}, err
	}
	normalizedURL, eventsJSON, enabled, err := normalizeWebhookInput(in)
	if err != nil {
		return Webhook{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE webhook SET url=?,secret=?,events_json=?,enabled=?,updated_at=? WHERE id=?`,
		normalizedURL, in.Secret, eventsJSON, boolInt(enabled), now(), existing.ID,
	); err != nil {
		return Webhook{}, normalizeErr(err)
	}
	return s.GetWebhook(ctx, existing.ID)
}

func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhook WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

// ListWebhookDeliveries returns the most recent delivery attempts (success or
// failure) for a given webhook, newest first.
func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []WebhookDelivery
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id,webhook_id,event_type,payload_json,status,status_code,response_body,error_message,attempt,next_attempt_at,delivered_at,created_at FROM webhook_delivery WHERE webhook_id=? ORDER BY created_at DESC LIMIT ?`,
		webhookID, limit,
	); err != nil {
		return nil, normalizeErr(err)
	}
	return rows, nil
}

// EnqueueWebhookDeliveries inserts a 'pending' delivery row for every enabled
// webhook on the workspace whose Events filter matches the given event (an
// empty Events array matches everything). Intended to be called inside the
// existing lifecycle transactions so a webhook fan-out and the lifecycle
// change land or roll back together.
func (s *Store) EnqueueWebhookDeliveries(ctx context.Context, tx *sqlx.Tx, workspaceID, eventType string, payload []byte) (int, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(eventType) == "" {
		return 0, ErrValidation
	}
	if len(payload) == 0 {
		return 0, ErrValidation
	}
	var hooks []Webhook
	if err := tx.SelectContext(ctx, &hooks,
		webhookSelectBase+` WHERE workspace_id=? AND enabled=1`, workspaceID,
	); err != nil {
		return 0, normalizeErr(err)
	}
	queued := 0
	t := now()
	for i := range hooks {
		if err := decodeWebhookEvents(&hooks[i]); err != nil {
			return queued, err
		}
		if !webhookEventMatches(hooks[i].Events, eventType) {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO webhook_delivery(id,webhook_id,event_type,payload_json,status,next_attempt_at,created_at) VALUES(?,?,?,?, 'pending', ?, ?)`,
			newID(), hooks[i].ID, eventType, string(payload), t, t,
		); err != nil {
			return queued, normalizeErr(err)
		}
		queued++
	}
	return queued, nil
}

func webhookEventMatches(filter []string, event string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, e := range filter {
		if e == event {
			return true
		}
	}
	return false
}

func normalizeWebhookInput(in UpsertWebhookInput) (string, string, bool, error) {
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return "", "", false, ErrValidation
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", false, ErrValidation
	}
	known := map[string]struct{}{}
	for _, e := range KnownWebhookEvents {
		known[e] = struct{}{}
	}
	for _, e := range in.Events {
		if _, ok := known[strings.TrimSpace(e)]; !ok {
			return "", "", false, ErrValidation
		}
	}
	eventsJSON := "[]"
	if len(in.Events) > 0 {
		b, err := json.Marshal(in.Events)
		if err != nil {
			return "", "", false, err
		}
		eventsJSON = string(b)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return rawURL, eventsJSON, enabled, nil
}

func decodeWebhookEvents(w *Webhook) error {
	raw := strings.TrimSpace(w.EventsJSON)
	if raw == "" || raw == "[]" {
		w.Events = nil
		return nil
	}
	var events []string
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return err
	}
	w.Events = events
	return nil
}
