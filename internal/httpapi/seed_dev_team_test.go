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

// Phase 1 of dev-plan/implement_20260524_211245.md.
//
// POST /api/system/seed-dev-team provisions the hub-PM dev-team workspace
// (7 role agents, 8 skills) from a single HTTP call so the Settings UI can
// trigger the same action the CLI already exposes.
func TestSeedDevTeamHTTPHappyPath(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodPost, "/api/system/seed-dev-team", `{"slug":"dev-team-http","working_dir":"`+dir+`"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Workspace struct {
			ID         string `json:"id"`
			Slug       string `json:"slug"`
			Name       string `json:"name"`
			WorkingDir string `json:"working_dir"`
		} `json:"workspace"`
		Agents []struct {
			Name    string `json:"name"`
			Runtime string `json:"runtime"`
		} `json:"agents"`
		Skills            []string `json:"skills"`
		AssignmentCount   int      `json:"assignment_count"`
		CreatedAgentCount int      `json:"created_agent_count"`
		AlreadyHad        bool     `json:"already_had"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workspace.Slug != "dev-team-http" {
		t.Fatalf("workspace slug=%q want dev-team-http", payload.Workspace.Slug)
	}
	if payload.Workspace.WorkingDir != dir {
		t.Fatalf("workspace working_dir=%q want %q", payload.Workspace.WorkingDir, dir)
	}
	if len(payload.Agents) != 7 {
		t.Fatalf("agent count=%d want 7", len(payload.Agents))
	}
	if len(payload.Skills) != 8 {
		t.Fatalf("skill count=%d want 8", len(payload.Skills))
	}
	if payload.AlreadyHad {
		t.Fatalf("first seed must not report already_had=true")
	}
	// Main agent (Lead) is created as part of the workspace bootstrap; only
	// the 6 worker agents come through CreateAgent and bump the counter.
	if payload.CreatedAgentCount != 6 {
		t.Fatalf("created_agent_count=%d want 6 (excludes main agent)", payload.CreatedAgentCount)
	}
	// 7 agents × 2 skills each (result-protocol + role-specific) = 14 assignments.
	if payload.AssignmentCount != 14 {
		t.Fatalf("assignment_count=%d want 14", payload.AssignmentCount)
	}
}

func TestSeedDevTeamHTTPIdempotent(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	first := do(t, h, http.MethodPost, "/api/system/seed-dev-team", `{"slug":"idem","working_dir":"`+dir+`"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first call status=%d body=%s", first.Code, first.Body.String())
	}
	second := do(t, h, http.MethodPost, "/api/system/seed-dev-team", `{"slug":"idem","working_dir":"`+dir+`"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("second call status=%d body=%s", second.Code, second.Body.String())
	}
	var payload struct {
		AlreadyHad        bool `json:"already_had"`
		CreatedAgentCount int  `json:"created_agent_count"`
	}
	if err := json.NewDecoder(second.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.AlreadyHad {
		t.Fatalf("second call must report already_had=true")
	}
	if payload.CreatedAgentCount != 0 {
		t.Fatalf("second call created_agent_count=%d want 0", payload.CreatedAgentCount)
	}
}

// When the request omits working_dir, the handler falls back to the
// server's data_dir so brand-new operators who have not yet wired the
// project directory still get a usable workspace.
func TestSeedDevTeamHTTPDefaultsWorkingDirToDataDir(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(store.New(database), config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	res := do(t, h, http.MethodPost, "/api/system/seed-dev-team", `{"slug":"defaults"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Workspace struct {
			WorkingDir string `json:"working_dir"`
		} `json:"workspace"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workspace.WorkingDir != dir {
		t.Fatalf("default working_dir=%q want %q (data_dir fallback)", payload.Workspace.WorkingDir, dir)
	}
}
