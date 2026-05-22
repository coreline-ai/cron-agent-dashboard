package store

import (
	"context"
	"strings"
)

const attachmentSelectBase = `SELECT id,issue_id,uploaded_by,filename,content_type,size_bytes,sha256,storage_path,created_at FROM attachment`

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
		UploadedBy:  uploadedBy,
		Filename:    in.Filename,
		ContentType: contentType,
		SizeBytes:   in.SizeBytes,
		SHA256:      in.SHA256,
		StoragePath: in.StoragePath,
		CreatedAt:   now(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO attachment(id,issue_id,uploaded_by,filename,content_type,size_bytes,sha256,storage_path,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		a.ID, a.IssueID, a.UploadedBy, a.Filename, a.ContentType, a.SizeBytes, a.SHA256, a.StoragePath, a.CreatedAt,
	); err != nil {
		return Attachment{}, normalizeErr(err)
	}
	return a, nil
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
