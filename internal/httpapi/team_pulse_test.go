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

// G5-3: /api/workspaces/:slug/agents/activity returns one row per agent with
// the latest run projection used by the Team Pulse widget.
func TestListAgentActivityShape(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	// seed workspace with two agents
	wsBody := `{"name":"Pulse","slug":"pulse","identifier_prefix":"PUL","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	if res := do(t, h, http.MethodPost, "/api/workspaces", wsBody); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	if res := do(t, h, http.MethodPost, "/api/workspaces/pulse/agents", `{"name":"Writer","runtime":"claude","instructions":"x"}`); res.Code != http.StatusCreated {
		t.Fatalf("seed agent: %s", res.Body.String())
	}

	res := do(t, h, http.MethodGet, "/api/workspaces/pulse/agents/activity", "")
	if res.Code != http.StatusOK {
		t.Fatalf("activity status=%d body=%s", res.Code, res.Body.String())
	}
	var got struct {
		Activity []store.AgentActivity `json:"activity"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Activity) != 2 {
		t.Fatalf("expected 2 activity rows, got %d: %#v", len(got.Activity), got.Activity)
	}
	// main agent should sort first
	if !got.Activity[0].IsMain {
		t.Fatalf("first row should be main agent, got %#v", got.Activity[0])
	}
	if got.Activity[0].AgentName != "Lead" || got.Activity[1].AgentName != "Writer" {
		t.Fatalf("unexpected agent order: %#v", got.Activity)
	}
	// no runs yet → latest_run_status should be empty
	if got.Activity[0].LatestRunStatus != "" {
		t.Fatalf("expected empty latest_run_status without runs, got %q", got.Activity[0].LatestRunStatus)
	}
}

// G5-3: when an issue+run exists, activity surfaces the run status and issue
// identifier for the assigned agent.
func TestListAgentActivityReflectsLatestRun(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	wsBody := `{"name":"PulseRun","slug":"pulse-run","identifier_prefix":"PR","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`
	if res := do(t, h, http.MethodPost, "/api/workspaces", wsBody); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}

	// Create an issue (auto-creates queued run for main agent).
	if res := do(t, h, http.MethodPost, "/api/workspaces/pulse-run/issues", `{"title":"work"}`); res.Code != http.StatusCreated {
		t.Fatalf("create issue: %s", res.Body.String())
	}

	res := do(t, h, http.MethodGet, "/api/workspaces/pulse-run/agents/activity", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got struct {
		Activity []store.AgentActivity `json:"activity"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Activity) != 1 {
		t.Fatalf("expected 1 activity row, got %d", len(got.Activity))
	}
	row := got.Activity[0]
	if row.LatestRunStatus != "queued" {
		t.Fatalf("latest_run_status should be queued, got %q", row.LatestRunStatus)
	}
	if row.LatestIssueIdentifier != "PR-1" {
		t.Fatalf("latest_issue_identifier should be PR-1, got %q", row.LatestIssueIdentifier)
	}
}
