package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

const runEventDetailJSONMaxBytes = 4 * 1024

const runEventSelectBase = `
SELECT id, run_id, issue_id, seq, event_type, severity, message, detail_json, created_at
FROM run_event`

type truncatedRunEventDetails struct {
	Truncated         bool   `json:"truncated"`
	OriginalSizeBytes int    `json:"original_size_bytes"`
	Preview           string `json:"preview"`
}

func (s *Store) AppendRunEvent(ctx context.Context, in RunEventInput) (RunEvent, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return RunEvent{}, err
	}
	defer tx.Rollback()
	event, err := appendRunEventTx(ctx, tx, in)
	if err != nil {
		return RunEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return RunEvent{}, err
	}
	if s.runEventNotifier != nil {
		s.runEventNotifier.OnRunEvent(event.IssueID, event.RunID)
	}
	return event, nil
}

// ListIssueRunEventsSince returns run_event rows for an issue created after
// the given watermark, ordered by created_at then seq so SSE clients can
// stream incremental updates without re-receiving rows they already saw.
// Pass an empty `since` to fetch the full history. `limit` clamps to a
// sane default to keep the SSE poll lightweight.
func (s *Store) ListIssueRunEventsSince(ctx context.Context, issueID, since string, limit int) ([]RunEvent, error) {
	if strings.TrimSpace(issueID) == "" {
		return nil, ErrValidation
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{issueID}
	q := runEventSelectBase + ` WHERE issue_id=?`
	if strings.TrimSpace(since) != "" {
		q += ` AND created_at > ?`
		args = append(args, since)
	}
	q += ` ORDER BY created_at ASC, run_id ASC, seq ASC LIMIT ?`
	args = append(args, limit)
	var events []RunEvent
	if err := s.db.SelectContext(ctx, &events, q, args...); err != nil {
		return nil, normalizeErr(err)
	}
	for i := range events {
		if err := decodeRunEventDetails(&events[i]); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (s *Store) ListRunEvents(ctx context.Context, runID string) ([]RunEvent, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, ErrValidation
	}
	var exists int
	if err := s.db.GetContext(ctx, &exists, `SELECT 1 FROM run WHERE id=?`, runID); err != nil {
		return nil, normalizeErr(err)
	}
	var events []RunEvent
	if err := s.db.SelectContext(ctx, &events, runEventSelectBase+` WHERE run_id=? ORDER BY seq ASC`, runID); err != nil {
		return nil, normalizeErr(err)
	}
	for i := range events {
		if err := decodeRunEventDetails(&events[i]); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func appendRunEventTx(ctx context.Context, tx *sqlx.Tx, in RunEventInput) (RunEvent, error) {
	if strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.EventType) == "" {
		return RunEvent{}, ErrValidation
	}
	severity := in.Severity
	if severity == "" {
		severity = RunEventSeverityInfo
	}
	detailJSON, err := marshalRunEventDetails(in.Details)
	if err != nil {
		return RunEvent{}, err
	}
	var event RunEvent
	err = tx.GetContext(ctx, &event, `
INSERT INTO run_event(id,run_id,issue_id,seq,event_type,severity,message,detail_json,created_at)
SELECT ?, r.id, r.issue_id,
       COALESCE((SELECT MAX(seq) FROM run_event WHERE run_id=r.id), 0) + 1,
       ?, ?, ?, ?, ?
FROM run AS r
WHERE r.id=? AND (? = '' OR r.issue_id = ?)
RETURNING id, run_id, issue_id, seq, event_type, severity, message, detail_json, created_at`,
		newID(), in.EventType, severity, in.Message, detailJSON, now(), in.RunID, in.IssueID, in.IssueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunEvent{}, classifyRunEventInsertNoRows(ctx, tx, in.RunID, in.IssueID)
		}
		return RunEvent{}, normalizeErr(err)
	}
	if !event.DetailJSON.Valid {
		event.DetailJSON = sql.NullString{String: detailJSON, Valid: true}
	}
	if err := decodeRunEventDetails(&event); err != nil {
		return RunEvent{}, err
	}
	return event, nil
}

func classifyRunEventInsertNoRows(ctx context.Context, tx *sqlx.Tx, runID, issueID string) error {
	var actualIssueID string
	if err := tx.GetContext(ctx, &actualIssueID, `SELECT issue_id FROM run WHERE id=?`, runID); err != nil {
		return normalizeErr(err)
	}
	if issueID != "" && issueID != actualIssueID {
		return ErrValidation
	}
	return ErrNotFound
}

func marshalRunEventDetails(details map[string]any) (string, error) {
	if details == nil {
		return "{}", nil
	}
	b, err := json.Marshal(details)
	if err != nil {
		return "", err
	}
	if len(b) == 0 || string(b) == "null" {
		return "{}", nil
	}
	if len(b) > runEventDetailJSONMaxBytes {
		return marshalTruncatedRunEventDetails(b)
	}
	return string(b), nil
}

func marshalTruncatedRunEventDetails(original []byte) (string, error) {
	originalText := string(original)
	previewLimit := len(originalText)
	if previewLimit > runEventDetailJSONMaxBytes {
		previewLimit = runEventDetailJSONMaxBytes
	}

	best, err := json.Marshal(truncatedRunEventDetails{
		Truncated:         true,
		OriginalSizeBytes: len(original),
		Preview:           "",
	})
	if err != nil {
		return "", err
	}

	lo, hi := 0, previewLimit
	for lo <= hi {
		mid := lo + (hi-lo)/2
		candidate, err := json.Marshal(truncatedRunEventDetails{
			Truncated:         true,
			OriginalSizeBytes: len(original),
			Preview:           utf8SafePrefix(originalText, mid),
		})
		if err != nil {
			return "", err
		}
		if len(candidate) <= runEventDetailJSONMaxBytes {
			best = candidate
			lo = mid + 1
			continue
		}
		hi = mid - 1
	}
	return string(best), nil
}

func utf8SafePrefix(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	end := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		end = i
	}
	return s[:end]
}

func decodeRunEventDetails(event *RunEvent) error {
	detailJSON := "{}"
	if event.DetailJSON.Valid && strings.TrimSpace(event.DetailJSON.String) != "" {
		detailJSON = event.DetailJSON.String
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(detailJSON), &details); err != nil {
		return err
	}
	if details == nil {
		details = map[string]any{}
	}
	event.Details = details
	return nil
}
