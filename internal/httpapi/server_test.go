package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/corn-agent-dashboard/internal/config"
	"github.com/coreline-ai/corn-agent-dashboard/internal/db"
	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
)

func TestHTTPAPISmoke(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Workspace store.Workspace `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Workspace.Slug != "ai-news" {
		t.Fatalf("bad workspace: %#v", created.Workspace)
	}

	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	if issueResp.Issue.ExecutionStatus != "queued" || issueResp.Issue.Identifier != "NEWS-1" {
		t.Fatalf("bad issue: %#v", issueResp.Issue)
	}

	res = do(t, h, http.MethodPut, "/api/issues/"+issueResp.Issue.ID, `{"body":""}`)
	if res.Code != http.StatusOK {
		t.Fatalf("clear issue body status=%d body=%s", res.Code, res.Body.String())
	}
	var updatedIssueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&updatedIssueResp); err != nil {
		t.Fatal(err)
	}
	if updatedIssueResp.Issue.Body != "" {
		t.Fatalf("body should be cleared, got %#v", updatedIssueResp.Issue)
	}

	res = do(t, h, http.MethodGet, "/api/workspaces/ai-news/issues/NEWS-1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("get issue status=%d body=%s", res.Code, res.Body.String())
	}

	res = do(t, h, http.MethodGet, "/healthz", "")
	if res.Code != http.StatusOK {
		t.Fatalf("healthz status=%d", res.Code)
	}
}

func TestHTTPAPIAuthCORSAndStaticFallback(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := config.Config{
		DataDir:  dir,
		DBPath:   filepath.Join(dir, "data.db"),
		Bind:     "127.0.0.1:0",
		Token:    "secret",
		CORS:     []string{"http://allowed.local"},
		Workers:  1,
		Timezone: "Asia/Seoul",
	}
	h := New(store.New(database), cfg)

	res := do(t, h, http.MethodGet, "/api/settings", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d body=%s", res.Code, res.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "http://evil.local")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("unexpected origin status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "http://allowed.local")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("allowed origin status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.local" {
		t.Fatalf("Access-Control-Allow-Origin=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/w/foo/issues/NEWS-1", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("static fallback status=%d body=%s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type=%q", ct)
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
	req.Header.Set("Origin", "http://example.com")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code == http.StatusForbidden {
		t.Fatalf("same-origin static asset was blocked by CORS")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/not-found", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("unknown api status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"code":"NOT_FOUND"`)) {
		t.Fatalf("unknown api did not return JSON error: %s", res.Body.String())
	}
}

func TestHTTPAPIRunListDoesNotExposeStdoutPath(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	body := `{"name":"AI News","slug":"ai-news","identifier_prefix":"NEWS","main_agent":{"name":"NewsLead","runtime":"codex","instructions":"lead"}}`
	res := do(t, h, http.MethodPost, "/api/workspaces", body)
	if res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	res = do(t, h, http.MethodPost, "/api/workspaces/ai-news/issues", `{"title":"오늘 뉴스","body":"body"}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	res = do(t, h, http.MethodGet, "/api/issues/"+issueResp.Issue.ID+"/runs", "")
	if res.Code != http.StatusOK {
		t.Fatalf("list runs status=%d body=%s", res.Code, res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte("stdout_path")) {
		t.Fatalf("run list leaked stdout_path: %s", res.Body.String())
	}
	if bytes.Contains(res.Body.Bytes(), []byte(`"Valid"`)) || bytes.Contains(res.Body.Bytes(), []byte(`"Int64"`)) {
		t.Fatalf("run list leaked database nullable internals: %s", res.Body.String())
	}
}

func TestHTTPAPIBackupCreatesSQLiteCopy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	backupPath := filepath.Join(dir, "nested", "backup.db")
	res := do(t, h, http.MethodPost, "/api/system/backup", `{"to":"`+backupPath+`"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}
}

func TestHTTPAPIVacuum(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	database, err := db.OpenAndMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: dbPath, Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodPost, "/api/system/vacuum", `{}`)
	if res.Code != http.StatusOK {
		t.Fatalf("vacuum status=%d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte("reclaimed_bytes")) {
		t.Fatalf("vacuum response missing reclaimed_bytes: %s", res.Body.String())
	}
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
