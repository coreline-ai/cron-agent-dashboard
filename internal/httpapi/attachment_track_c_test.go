package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track C of dev-plan/implement_20260522_212332.md.
//
// uploadAttachment must consult the first 512 bytes of the body via
// http.DetectContentType. When the sniff identifies a content type other
// than application/octet-stream, the claimed Content-Type is overridden so
// downstream listeners (e.g. download Content-Type, attachment audit) see
// the truth instead of whatever the uploader declared.
func TestAttachmentUploadDeepSniffsPNGEvenWhenClaimedAsTextPlain(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"Att","slug":"sniff-png","identifier_prefix":"PNG","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/sniff-png/issues", `{"title":"png upload"}`)
	if issueRes.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", issueRes.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(issueRes.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}

	// PNG magic bytes \x89PNG\r\n\x1a\n followed by enough bytes for
	// DetectContentType (~24 is plenty).
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 8, 0, 0, 0, 8, 0x08, 0x06, 0x00, 0x00, 0x00, 0xc4, 0x0f, 0xbe, 0x8b}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="evil.txt"`)
	hdr.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(pngHeader); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/issues/"+issueResp.Issue.ID+"/attachments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		Attachment struct {
			ID          string `json:"id"`
			ContentType string `json:"content_type"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"attachment"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Attachment.ContentType != "image/png" {
		t.Fatalf("sniff should override claimed text/plain to image/png, got %q", uploaded.Attachment.ContentType)
	}
	if uploaded.Attachment.SizeBytes != int64(len(pngHeader)) {
		t.Fatalf("size mismatch after sniff replay: got %d want %d", uploaded.Attachment.SizeBytes, len(pngHeader))
	}

	// Download must surface the sniffed content type, not the claimed one.
	dl := do(t, h, http.MethodGet, "/api/attachments/"+uploaded.Attachment.ID+"/download", "")
	if got := dl.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("download Content-Type=%q want image/png", got)
	}
}

// Each download endpoint hit must append one attachment_audit row of
// action='downloaded'. The audit endpoint returns the trail newest-first so
// the test asserts on the most recent row.
func TestAttachmentDownloadAppendsAuditRow(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"Att","slug":"att-audit","identifier_prefix":"AUD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/att-audit/issues", `{"title":"audit me"}`)
	if issueRes.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", issueRes.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	_ = json.NewDecoder(issueRes.Body).Decode(&issueResp)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="x.txt"`)
	hdr.Set("Content-Type", "text/plain")
	part, _ := writer.CreatePart(hdr)
	_, _ = part.Write([]byte("hello audit\n"))
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
			ID string `json:"id"`
		} `json:"attachment"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&uploaded)

	// Two downloads → two 'downloaded' rows + the initial 'uploaded' row.
	for i := 0; i < 2; i++ {
		if r := do(t, h, http.MethodGet, "/api/attachments/"+uploaded.Attachment.ID+"/download", ""); r.Code != http.StatusOK {
			t.Fatalf("download #%d status=%d body=%s", i, r.Code, r.Body.String())
		}
	}

	auditRes := do(t, h, http.MethodGet, "/api/attachments/"+uploaded.Attachment.ID+"/audit", "")
	if auditRes.Code != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", auditRes.Code, auditRes.Body.String())
	}
	var audit struct {
		Entries []store.AttachmentAuditEntry `json:"entries"`
	}
	if err := json.NewDecoder(auditRes.Body).Decode(&audit); err != nil {
		t.Fatal(err)
	}
	var downloads, uploads int
	for _, e := range audit.Entries {
		switch e.Action {
		case "downloaded":
			downloads++
		case "uploaded":
			uploads++
		}
	}
	if downloads != 2 {
		t.Fatalf("expected 2 downloaded entries, got %d (audit=%s)", downloads, auditRes.Body.String())
	}
	if uploads != 1 {
		t.Fatalf("expected 1 uploaded entry, got %d", uploads)
	}
	// Newest first ordering.
	if audit.Entries[0].Action != "downloaded" {
		t.Fatalf("expected newest entry to be downloaded, got %s", audit.Entries[0].Action)
	}
	if !strings.HasPrefix(audit.Entries[0].AttachmentID, uploaded.Attachment.ID[:8]) {
		t.Fatalf("audit attachment_id mismatch: %s", audit.Entries[0].AttachmentID)
	}
}
