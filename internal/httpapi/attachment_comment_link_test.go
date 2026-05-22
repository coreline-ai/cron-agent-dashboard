package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track F of dev-plan/implement_20260522_220446.md.
//
// Comment-linked attachments are recorded with comment_id at upload time so
// the IssueAttachmentsPanel can group "댓글 첨부" rows under their
// originating comment. POST /api/attachments/{id}/link-comment also lets
// the UI rewrite the link after the fact (e.g. when the comment is created
// only after the upload finishes).
func TestAttachmentUploadAcceptsCommentIDFormField(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"AttCm","slug":"att-cm","identifier_prefix":"AC","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/att-cm/issues", `{"title":"comment attach"}`)
	if issueRes.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", issueRes.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(issueRes.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	// Create a real comment to link against.
	cmt, err := st.AddUserComment(t.Context(), issueResp.Issue.ID, "with attachment")
	if err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="note.txt"`)
	hdr.Set("Content-Type", "text/plain")
	part, _ := writer.CreatePart(hdr)
	_, _ = part.Write([]byte("notes\n"))
	_ = writer.WriteField("comment_id", cmt.Comment.ID)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/issues/"+issueResp.Issue.ID+"/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		Attachment struct {
			ID        string `json:"id"`
			CommentID string `json:"comment_id"`
		} `json:"attachment"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Attachment.CommentID != cmt.Comment.ID {
		t.Fatalf("uploaded comment_id=%q want %q", uploaded.Attachment.CommentID, cmt.Comment.ID)
	}

	// List shows the linkage in the JSON view.
	listRes := do(t, h, http.MethodGet, "/api/issues/"+issueResp.Issue.ID+"/attachments", "")
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	if !bytes.Contains(listRes.Body.Bytes(), []byte(`"comment_id":"`+cmt.Comment.ID+`"`)) {
		t.Fatalf("list missing comment_id linkage: %s", listRes.Body.String())
	}
}

func TestAttachmentLinkCommentRoundtrips(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"AttLink","slug":"att-link","identifier_prefix":"AL","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/att-link/issues", `{"title":"link later"}`)
	if issueRes.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", issueRes.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	_ = json.NewDecoder(issueRes.Body).Decode(&issueResp)

	// Upload without comment_id.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="orphan.txt"`)
	hdr.Set("Content-Type", "text/plain")
	part, _ := writer.CreatePart(hdr)
	_, _ = part.Write([]byte("orphan body\n"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/issues/"+issueResp.Issue.ID+"/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		Attachment struct {
			ID        string `json:"id"`
			CommentID string `json:"comment_id"`
		} `json:"attachment"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&uploaded)
	if uploaded.Attachment.CommentID != "" {
		t.Fatalf("expected empty comment_id, got %q", uploaded.Attachment.CommentID)
	}

	cmt, _ := st.AddUserComment(t.Context(), issueResp.Issue.ID, "post-hoc")
	link := do(t, h, http.MethodPost, "/api/attachments/"+uploaded.Attachment.ID+"/link-comment", `{"comment_id":"`+cmt.Comment.ID+`"}`)
	if link.Code != http.StatusOK {
		t.Fatalf("link status=%d body=%s", link.Code, link.Body.String())
	}
	var linked struct {
		Attachment struct {
			CommentID string `json:"comment_id"`
		} `json:"attachment"`
	}
	if err := json.NewDecoder(link.Body).Decode(&linked); err != nil {
		t.Fatal(err)
	}
	if linked.Attachment.CommentID != cmt.Comment.ID {
		t.Fatalf("post-link comment_id=%q want %q", linked.Attachment.CommentID, cmt.Comment.ID)
	}
}
