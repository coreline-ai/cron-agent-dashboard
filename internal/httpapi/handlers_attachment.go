package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// attachmentView mirrors store.Attachment with a download_url so clients do
// not have to assemble the URL themselves. StoragePath stays hidden.
type attachmentView struct {
	ID          string `json:"id"`
	IssueID     string `json:"issue_id"`
	CommentID   string `json:"comment_id,omitempty"`
	UploadedBy  string `json:"uploaded_by"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
	CreatedAt   string `json:"created_at"`
}

func newAttachmentView(a store.Attachment) attachmentView {
	return attachmentView{
		ID:          a.ID,
		IssueID:     a.IssueID,
		CommentID:   a.CommentIDString(),
		UploadedBy:  a.UploadedBy,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		SHA256:      a.SHA256,
		DownloadURL: "/api/attachments/" + a.ID + "/download",
		CreatedAt:   a.CreatedAt,
	}
}

func (s *Server) registerAttachmentRoutes(api chi.Router) {
	api.Post("/api/issues/{id}/attachments", s.uploadAttachment)
	api.Get("/api/issues/{id}/attachments", s.listAttachments)
	api.Get("/api/attachments/{id}/download", s.downloadAttachment)
	api.Get("/api/attachments/{id}/audit", s.listAttachmentAudit)
	api.Post("/api/attachments/{id}/link-comment", s.linkAttachmentToComment)
	api.Delete("/api/attachments/{id}", s.deleteAttachment)
}

func (s *Server) linkAttachmentToComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		CommentID string `json:"comment_id"`
	}
	if !decodeOptional(w, r, &req) {
		return
	}
	if err := s.store.LinkAttachmentToComment(r.Context(), id, req.CommentID); err != nil {
		respond(w, nil, err, 0)
		return
	}
	updated, err := s.store.GetAttachment(r.Context(), id)
	respond(w, map[string]any{"attachment": newAttachmentView(updated)}, err, http.StatusOK)
}

func (s *Server) listAttachmentAudit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.store.GetAttachment(r.Context(), id); err != nil {
		respond(w, nil, err, 0)
		return
	}
	entries, err := s.store.ListAttachmentAudit(r.Context(), id, 0)
	respond(w, map[string]any{"entries": entries}, err, http.StatusOK)
}

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	if _, err := s.store.GetIssue(r.Context(), issueID); err != nil {
		respond(w, nil, err, 0)
		return
	}
	// Cap the entire multipart body at the attachment limit. The
	// SaveAttachmentFile helper also enforces it on the file stream itself,
	// so a multipart wrapper trick cannot grow the on-disk file past the
	// declared limit either.
	r.Body = http.MaxBytesReader(w, r.Body, app.AttachmentMaxBytes+4096)
	if err := r.ParseMultipartForm(app.AttachmentMaxBytes + 4096); err != nil {
		respond(w, nil, store.ErrValidation, 0)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respond(w, nil, store.ErrValidation, 0)
		return
	}
	defer file.Close()

	filename := strings.TrimSpace(header.Filename)
	if filename == "" {
		respond(w, nil, store.ErrValidation, 0)
		return
	}
	contentType := header.Header.Get("Content-Type")

	// Deep-sniff the first 512 bytes: http.DetectContentType is the canonical
	// magic-byte matcher in the stdlib. We do not reject on mismatch (too
	// aggressive — uploaders routinely send octet-stream as a placeholder);
	// we only override when the sniff actually identifies something other
	// than the generic application/octet-stream fallback.
	head := make([]byte, 512)
	n, readErr := io.ReadFull(file, head)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		respond(w, nil, readErr, 0)
		return
	}
	head = head[:n]
	sniffed := http.DetectContentType(head)
	if sniffed != "" && sniffed != "application/octet-stream" {
		contentType = sniffed
	}
	// Re-attach the head we just consumed in front of the rest of the body so
	// SaveAttachmentFile sees the original byte stream.
	bodyReader := io.MultiReader(bytes.NewReader(head), file)

	// Two-step write: insert a metadata row first (so we own a stable ID and
	// can roll back on disk-write failure), then stream the body to disk
	// under that ID, then patch the row's size/sha/storage_path. The
	// intermediate "" storage_path is fine because the store does not
	// dereference it.
	created, err := s.store.CreateAttachment(r.Context(), store.CreateAttachmentInput{
		IssueID:     issueID,
		CommentID:   strings.TrimSpace(r.FormValue("comment_id")),
		UploadedBy:  "user",
		Filename:    filename,
		ContentType: contentType,
		StoragePath: "pending",
	})
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	path, size, sha, err := app.SaveAttachmentFile(s.cfg.DataDir, created.ID, bodyReader)
	if err != nil {
		_ = s.store.DeleteAttachment(r.Context(), created.ID)
		if errors.Is(err, app.ErrAttachmentTooLarge) {
			http.Error(w, "attachment too large", http.StatusRequestEntityTooLarge)
			return
		}
		respond(w, nil, err, 0)
		return
	}
	if _, err := s.store.DB().ExecContext(r.Context(),
		`UPDATE attachment SET size_bytes=?, sha256=?, storage_path=? WHERE id=?`,
		size, sha, path, created.ID); err != nil {
		_ = app.RemoveAttachmentFile(path)
		_ = s.store.DeleteAttachment(r.Context(), created.ID)
		respond(w, nil, err, 0)
		return
	}
	created.SizeBytes = size
	created.SHA256 = sha
	created.StoragePath = path
	// Audit upload. Failures are warn-only — the attachment itself is committed.
	if auditErr := s.store.RecordAttachmentAudit(r.Context(), created.ID, issueID, store.AttachmentAuditActionUploaded, "user"); auditErr != nil {
		// non-fatal: continue with the response
		_ = auditErr
	}
	respond(w, map[string]any{"attachment": newAttachmentView(created)}, nil, http.StatusCreated)
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	if _, err := s.store.GetIssue(r.Context(), issueID); err != nil {
		respond(w, nil, err, 0)
		return
	}
	rows, err := s.store.ListAttachments(r.Context(), issueID)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	out := make([]attachmentView, 0, len(rows))
	for _, a := range rows {
		out = append(out, newAttachmentView(a))
	}
	respond(w, map[string]any{"attachments": out}, nil, http.StatusOK)
}

func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.store.GetAttachment(r.Context(), id)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	f, err := os.Open(a.StoragePath)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	defer f.Close()
	// Audit before streaming. The audit row is the only durable record of
	// this access, so we want it on disk regardless of whether the client
	// actually finishes reading the body.
	_ = s.store.RecordAttachmentAudit(r.Context(), a.ID, a.IssueID, store.AttachmentAuditActionDownloaded, "user")
	w.Header().Set("Content-Type", a.ContentType)
	// RFC 6266 / 5987 — URL-encode the filename for the filename* form so
	// non-ASCII (e.g. Korean) names survive intermediaries cleanly.
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+sanitizeASCIIFilename(a.Filename)+`"; filename*=UTF-8''`+url.PathEscape(a.Filename))
	modtime, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
	http.ServeContent(w, r, a.Filename, modtime, f)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.store.GetAttachment(r.Context(), id)
	if err != nil {
		respond(w, nil, err, 0)
		return
	}
	if err := s.store.DeleteAttachment(r.Context(), id); err != nil {
		respond(w, nil, err, 0)
		return
	}
	if err := app.RemoveAttachmentFile(a.StoragePath); err != nil {
		// Row is already gone; surface the file cleanup failure so operators
		// can investigate orphaned bytes on disk.
		respond(w, nil, err, 0)
		return
	}
	respond(w, map[string]any{"deleted": true, "id": id}, nil, http.StatusOK)
}

// sanitizeASCIIFilename strips characters that confuse the basic
// Content-Disposition `filename="..."` field. The RFC 5987 `filename*` form
// alongside carries the real (UTF-8) filename.
func sanitizeASCIIFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '"' || r == '\\' || r > 0x7e {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "attachment"
	}
	return b.String()
}
