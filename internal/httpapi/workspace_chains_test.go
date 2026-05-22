package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track G of dev-plan/implement_20260522_220446.md.
//
// /api/workspaces/{slug}/runs returns every run across the workspace newest
// first so the chain dashboard can run summarizeChains client-side. The
// limit clamp prevents an accidental full-history dump in a large
// workspace.
func TestListWorkspaceRunsReturnsAllRunsAcrossIssuesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"Chains","slug":"chains-dash","identifier_prefix":"CD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	// Three issues -> three queued initial runs.
	for i := 0; i < 3; i++ {
		if res := do(t, h, http.MethodPost, "/api/workspaces/chains-dash/issues", `{"title":"chain"}`); res.Code != http.StatusCreated {
			t.Fatalf("seed issue %d: %s", i, res.Body.String())
		}
	}
	res := do(t, h, http.MethodGet, "/api/workspaces/chains-dash/runs?limit=100", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Runs []store.Run `json:"runs"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Runs) != 3 {
		t.Fatalf("expected 3 runs across the workspace, got %d", len(payload.Runs))
	}
	for i := 1; i < len(payload.Runs); i++ {
		if payload.Runs[i-1].EnqueuedAt < payload.Runs[i].EnqueuedAt {
			t.Fatalf("runs not newest-first: %s before %s", payload.Runs[i-1].EnqueuedAt, payload.Runs[i].EnqueuedAt)
		}
	}
}
