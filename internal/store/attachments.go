package store

import (
	"context"
	"database/sql"
	"strings"
)

func nullableString(s string) sql.NullString {
	if strings.TrimSpace(s) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

const attachmentSelectBase = `SELECT id,issue_id,comment_id,uploaded_by,filename,content_type,size_bytes,sha256,storage_path,created_at FROM attachment`

// CreateAttachment inserts an attachment metadata row after the HTTP layer
// has already written the body to disk. It is intentionally split from the
// file storage helpers in internal/app so the store stays io-free.
func (s *Store) CreateAttachment(ctx context.Context, in CreateAttachmentInput) (Attachment, error) {
	if strings.TrimSpace(in.IssueID) == "" || strings.TrimSpace(in.Filename) == "" || strings.TrimSpace(in.StoragePath) == "" {
		return Attachment{}, ErrValidation
	}
	if _, err := s.GetIssue(ctx, in.IssueID); err != nil {
		return Attachment{}, err
	}
	uploadedBy := strings.TrimSpace(in.UploadedBy)
	if uploadedBy == "" {
		uploadedBy = "user"
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	a := Attachment{
		ID:          newID(),
		IssueID:     in.IssueID,
		CommentID:   nullableString(in.CommentID),
		UploadedBy:  uploadedBy,
		Filename:    in.Filename,
		ContentType: contentType,
		SizeBytes:   in.SizeBytes,
		SHA256:      in.SHA256,
		StoragePath: in.StoragePath,
		CreatedAt:   now(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO attachment(id,issue_id,comment_id,uploaded_by,filename,content_type,size_bytes,sha256,storage_path,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.IssueID, a.CommentID, a.UploadedBy, a.Filename, a.ContentType, a.SizeBytes, a.SHA256, a.StoragePath, a.CreatedAt,
	); err != nil {
		return Attachment{}, normalizeErr(err)
	}
	return a, nil
}

// LinkAttachmentToComment associates an existing attachment with a comment.
// Used by the comment-with-attachment HTTP path: the attachment is created
// first (so its body lives on disk and the storage_path is owned by the row),
// then the comment is created, then this links them together. Calling with
// commentID="" detaches the attachment.
func (s *Store) LinkAttachmentToComment(ctx context.Context, attachmentID, commentID string) error {
	if strings.TrimSpace(attachmentID) == "" {
		return ErrValidation
	}
	res, err := s.db.ExecContext(ctx, `UPDATE attachment SET comment_id=? WHERE id=?`, nullableString(commentID), attachmentID)
	if err != nil {
		return normalizeErr(err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListAttachments(ctx context.Context, issueID string) ([]Attachment, error) {
	var rows []Attachment
	if err := s.db.SelectContext(ctx, &rows, attachmentSelectBase+` WHERE issue_id=? ORDER BY created_at DESC, id DESC`, issueID); err != nil {
		return nil, normalizeErr(err)
	}
	return rows, nil
}

func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	var a Attachment
	if err := s.db.GetContext(ctx, &a, attachmentSelectBase+` WHERE id=?`, id); err != nil {
		return Attachment{}, normalizeErr(err)
	}
	return a, nil
}

// DeleteAttachment removes the metadata row only. The HTTP layer is
// responsible for unlinking the on-disk file after the row is gone — we keep
// the two operations idempotent and ordered (file first, row second on
// upload; row first, file second on delete) so a crash can never leave a row
// that points at a missing file.
func (s *Store) DeleteAttachment(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM attachment WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

// AttachmentAuditAction enumerates the actions logged in attachment_audit.
const (
	AttachmentAuditActionUploaded   = "uploaded"
	AttachmentAuditActionDownloaded = "downloaded"
	AttachmentAuditActionDeleted    = "deleted"
)

// AttachmentAuditEntry is one row of the per-attachment audit trail.
type AttachmentAuditEntry struct {
	ID           string `db:"id" json:"id"`
	AttachmentID string `db:"attachment_id" json:"attachment_id"`
	IssueID      string `db:"issue_id" json:"issue_id"`
	Action       string `db:"action" json:"action"`
	Actor        string `db:"actor" json:"actor"`
	CreatedAt    string `db:"created_at" json:"created_at"`
}

// RecordAttachmentAudit appends an audit row for an attachment lifecycle
// event. issueID is duplicated onto the row (rather than joined through the
// attachment row at read time) so the trail survives an attachment delete
// cascade if we ever flip that — today the cascade still wipes the trail
// alongside the attachment, which is the correct behaviour for tenant
// removal.
func (s *Store) RecordAttachmentAudit(ctx context.Context, attachmentID, issueID, action, actor string) error {
	if strings.TrimSpace(attachmentID) == "" || strings.TrimSpace(issueID) == "" {
		return ErrValidation
	}
	switch action {
	case AttachmentAuditActionUploaded, AttachmentAuditActionDownloaded, AttachmentAuditActionDeleted:
	default:
		return ErrValidation
	}
	if strings.TrimSpace(actor) == "" {
		actor = "user"
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO attachment_audit(id, attachment_id, issue_id, action, actor, created_at) VALUES(?,?,?,?,?,?)`,
		newID(), attachmentID, issueID, action, actor, now(),
	); err != nil {
		return normalizeErr(err)
	}
	return nil
}

// ListAttachmentAudit returns the audit trail for a single attachment,
// newest first. Limit clamps to a sane default to keep the response cheap.
func (s *Store) ListAttachmentAudit(ctx context.Context, attachmentID string, limit int) ([]AttachmentAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []AttachmentAuditEntry
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, attachment_id, issue_id, action, actor, created_at FROM attachment_audit WHERE attachment_id=? ORDER BY created_at DESC, id DESC LIMIT ?`,
		attachmentID, limit,
	); err != nil {
		return nil, normalizeErr(err)
	}
	return rows, nil
}
