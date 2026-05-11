package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
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

	res = do(t, h, http.MethodGet, "/api/workspaces/ai-news/issues/NEWS-1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("get issue status=%d body=%s", res.Code, res.Body.String())
	}

	res = do(t, h, http.MethodGet, "/healthz", "")
	if res.Code != http.StatusOK {
		t.Fatalf("healthz status=%d", res.Code)
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
