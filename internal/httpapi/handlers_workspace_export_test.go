package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestExportWorkspaceHTTPDefaultsToOperationalConfigOnly(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"Export Studio","slug":"export-studio","identifier_prefix":"EXP","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	if res := do(t, h, http.MethodPost, "/api/workspaces/export-studio/issues", `{"title":"Reach jane.doe@company.com","body":"call 010-1234-5678"}`); res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}

	res := do(t, h, http.MethodGet, "/api/workspaces/export-studio/export", "")
	if res.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Header().Get("Content-Disposition"), "export-studio.workspace.json") {
		t.Fatalf("missing Content-Disposition: %q", res.Header().Get("Content-Disposition"))
	}
	var payload app.WorkspaceExport
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Issues) != 0 {
		t.Fatalf("default export must omit history; got %d issues", len(payload.Issues))
	}
}

func TestExportWorkspaceHTTPIncludeHistoryAndMaskPII(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"Export Studio","slug":"export-studio","identifier_prefix":"EXP","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", res.Code, res.Body.String())
	}
	if res := do(t, h, http.MethodPost, "/api/workspaces/export-studio/issues", `{"title":"Reach jane.doe@company.com","body":"phone 010-1234-5678"}`); res.Code != http.StatusCreated {
		t.Fatalf("create issue status=%d body=%s", res.Code, res.Body.String())
	}

	res := do(t, h, http.MethodGet, "/api/workspaces/export-studio/export?include_history=1&mask_pii=1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", res.Code, res.Body.String())
	}
	var payload app.WorkspaceExport
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Issues) == 0 {
		t.Fatalf("expected history issues, got 0")
	}
	if strings.Contains(payload.Issues[0].Title, "jane.doe@company.com") {
		t.Fatalf("expected PII mask on title, got %q", payload.Issues[0].Title)
	}
	if strings.Contains(payload.Issues[0].Body, "010-1234-5678") {
		t.Fatalf("expected PII mask on phone, got %q", payload.Issues[0].Body)
	}
}
