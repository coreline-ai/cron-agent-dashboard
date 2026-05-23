package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

const webhookSelectBase = `SELECT id,workspace_id,url,secret,events_json,enabled,mask_pii,created_at,updated_at FROM webhook`

// CreateWebhook registers a new webhook subscription for a workspace.
func (s *Store) CreateWebhook(ctx context.Context, workspaceID string, in UpsertWebhookInput) (Webhook, error) {
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return Webhook{}, err
	}
	normalizedURL, eventsJSON, enabled, err := normalizeWebhookInput(in)
	if err != nil {
		return Webhook{}, err
	}
	maskPII := false
	if in.MaskPII != nil {
		maskPII = *in.MaskPII
	}
	t := now()
	w := Webhook{
		ID:          newID(),
		WorkspaceID: workspaceID,
		URL:         normalizedURL,
		Secret:      in.Secret,
		EventsJSON:  eventsJSON,
		Enabled:     enabled,
		MaskPII:     maskPII,
		CreatedAt:   t,
		UpdatedAt:   t,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO webhook(id,workspace_id,url,secret,events_json,enabled,mask_pii,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		w.ID, w.WorkspaceID, w.URL, w.Secret, w.EventsJSON, boolInt(w.Enabled), boolInt(w.MaskPII), t, t,
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
	maskPII := existing.MaskPII
	if in.MaskPII != nil {
		maskPII = *in.MaskPII
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE webhook SET url=?,secret=?,events_json=?,enabled=?,mask_pii=?,updated_at=? WHERE id=?`,
		normalizedURL, in.Secret, eventsJSON, boolInt(enabled), boolInt(maskPII), now(), existing.ID,
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
		// Per-subscription mask: each delivery row holds its own payload so
		// the dispatcher can sign whatever this webhook will actually receive.
		body := payload
		if hooks[i].MaskPII {
			body = maskPIIBytes(payload)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO webhook_delivery(id,webhook_id,event_type,payload_json,status,next_attempt_at,created_at) VALUES(?,?,?,?, 'pending', ?, ?)`,
			newID(), hooks[i].ID, eventType, string(body), t, t,
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

// NextPendingWebhookDelivery returns the oldest pending webhook_delivery row
// whose next_attempt_at has come due (relative to nowRFC3339), together with
// its webhook subscription. ok is false when the queue is empty.
//
// The implementation is intentionally simple: pick the oldest pending row,
// then look up the webhook. Callers must serialize tick() in a single
// goroutine — the dispatcher does. There's no SELECT FOR UPDATE in SQLite,
// so cross-process concurrency is out of scope (this is a local single-
// binary tool).
func (s *Store) NextPendingWebhookDelivery(ctx context.Context, nowRFC3339 string) (WebhookDelivery, Webhook, bool, error) {
	var delivery WebhookDelivery
	err := s.db.GetContext(ctx, &delivery,
		`SELECT id,webhook_id,event_type,payload_json,status,status_code,response_body,error_message,attempt,next_attempt_at,delivered_at,created_at
FROM webhook_delivery WHERE status='pending' AND next_attempt_at <= ? ORDER BY next_attempt_at ASC, created_at ASC LIMIT 1`, nowRFC3339)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebhookDelivery{}, Webhook{}, false, nil
		}
		return WebhookDelivery{}, Webhook{}, false, normalizeErr(err)
	}
	hook, err := s.GetWebhook(ctx, delivery.WebhookID)
	if err != nil {
		return WebhookDelivery{}, Webhook{}, false, err
	}
	return delivery, hook, true, nil
}

// MarkWebhookDeliveryDelivered marks a delivery as successfully delivered.
func (s *Store) MarkWebhookDeliveryDelivered(ctx context.Context, id string, statusCode int, responseBody string) error {
	t := now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE webhook_delivery SET status='delivered', status_code=?, response_body=?, delivered_at=?, attempt=attempt+1 WHERE id=? AND status='pending'`,
		statusCode, capWebhookResponseBody(responseBody), t, id,
	)
	if err != nil {
		return normalizeErr(err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrState
	}
	return nil
}

// MarkWebhookDeliveryFailure records a failed attempt. When maxAttempts is
// reached, the row terminates as 'failed'; otherwise next_attempt_at is set
// to nowRFC3339+backoff so the dispatcher retries later.
//
// maxAttempts counts total attempts (initial + retries). The default plan is
// 2 (one immediate attempt + one retry).
func (s *Store) MarkWebhookDeliveryFailure(ctx context.Context, id string, statusCode int, responseBody, errMsg string, retryAt string, maxAttempts int) error {
	t := now()
	// Inspect current attempt count to decide between retry and terminal fail.
	var attempt int
	if err := s.db.GetContext(ctx, &attempt, `SELECT attempt FROM webhook_delivery WHERE id=?`, id); err != nil {
		return normalizeErr(err)
	}
	nextAttempt := attempt + 1
	terminal := nextAttempt >= maxAttempts
	if terminal {
		_, err := s.db.ExecContext(ctx,
			`UPDATE webhook_delivery SET status='failed', status_code=?, response_body=?, error_message=?, attempt=?, next_attempt_at=? WHERE id=? AND status='pending'`,
			statusCode, capWebhookResponseBody(responseBody), capWebhookErrorMessage(errMsg), nextAttempt, t, id,
		)
		return normalizeErr(err)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE webhook_delivery SET status_code=?, response_body=?, error_message=?, attempt=?, next_attempt_at=? WHERE id=? AND status='pending'`,
		statusCode, capWebhookResponseBody(responseBody), capWebhookErrorMessage(errMsg), nextAttempt, retryAt, id,
	)
	return normalizeErr(err)
}

// ResubmitWebhookDelivery resets a dead-letter row to 'pending' so the
// dispatcher picks it up on its next poll. Operators trigger this from the
// Settings UI's webhook section when a receiver outage has been fixed and
// they want to retry the missed deliveries without re-creating the
// triggering event. Rows that are not in 'failed' state return ErrState
// — re-running a successful or in-flight delivery would be confusing.
//
// The row keeps its existing payload_json so the receiver gets exactly
// what it would have seen on the original attempt. attempt counter is
// reset to 0 so the exponential backoff schedule restarts fresh; error
// fields are cleared.
func (s *Store) ResubmitWebhookDelivery(ctx context.Context, deliveryID string) error {
	if strings.TrimSpace(deliveryID) == "" {
		return ErrValidation
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE webhook_delivery
		    SET status='pending',
		        next_attempt_at=?,
		        attempt=0,
		        status_code=0,
		        response_body='',
		        error_message='',
		        delivered_at=NULL
		  WHERE id=? AND status='failed'`,
		now(), deliveryID,
	)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		// Either the id doesn't exist or the row is not in 'failed'.
		var exists int
		if err := s.db.GetContext(ctx, &exists, `SELECT COUNT(*) FROM webhook_delivery WHERE id=?`, deliveryID); err != nil {
			return normalizeErr(err)
		}
		if exists == 0 {
			return ErrNotFound
		}
		return ErrState
	}
	return nil
}

// CountWebhookDeliveryFailed returns the number of terminally-failed
// (dead-letter) deliveries for a webhook. The Settings UI shows this as a
// badge so operators can spot a subscription that consistently 5xx-s its
// receiver.
func (s *Store) CountWebhookDeliveryFailed(ctx context.Context, webhookID string) (int, error) {
	if strings.TrimSpace(webhookID) == "" {
		return 0, ErrValidation
	}
	var n int
	if err := s.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM webhook_delivery WHERE webhook_id=? AND status='failed'`, webhookID); err != nil {
		return 0, normalizeErr(err)
	}
	return n, nil
}

func capWebhookResponseBody(body string) string {
	const max = 2048
	if len(body) <= max {
		return body
	}
	return body[:max]
}

func capWebhookErrorMessage(msg string) string {
	const max = 1024
	if len(msg) <= max {
		return msg
	}
	return msg[:max]
}
