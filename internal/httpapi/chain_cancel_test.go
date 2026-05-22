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

func TestCancelChainEndpointCancelsAllQueuedRunsOnChain(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"ChainCancel","slug":"chain-cx","identifier_prefix":"CC","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/chain-cx/issues", `{"title":"chain root"}`)
	if issueRes.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", issueRes.Body.String())
	}
	var issueResp struct {
		Issue store.Issue `json:"issue"`
		Run   store.Run   `json:"run"`
	}
	if err := json.NewDecoder(issueRes.Body).Decode(&issueResp); err != nil {
		t.Fatal(err)
	}
	chainID := issueResp.Run.ID

	// Add a worker agent so the sibling queued run can target a different
	// agent_id (the unique index forbids two queued runs on the same
	// issue+agent pair).
	agentRes := do(t, h, http.MethodPost, "/api/workspaces/chain-cx/agents", `{"name":"Writer","runtime":"claude","instructions":"writer"}`)
	if agentRes.Code != http.StatusCreated {
		t.Fatalf("seed worker agent: %s", agentRes.Body.String())
	}
	var workerResp struct {
		Agent store.Agent `json:"agent"`
	}
	if err := json.NewDecoder(agentRes.Body).Decode(&workerResp); err != nil {
		t.Fatal(err)
	}

	// Add a second queued run sharing the same chain to mimic a hub
	// re-entry that has not been claimed yet.
	if _, err := database.ExecContext(
		t.Context(),
		`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','mention','',datetime('now'),3,1,?,1)`,
		"sibling-run", issueResp.Issue.ID, workerResp.Agent.ID, chainID,
	); err != nil {
		t.Fatal(err)
	}

	res := do(t, h, http.MethodPost, "/api/runs/chain/"+chainID+"/cancel", "")
	if res.Code != http.StatusOK {
		t.Fatalf("chain cancel status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		ChainID   string `json:"chain_id"`
		Cancelled int    `json:"cancelled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChainID != chainID {
		t.Fatalf("payload.chain_id=%q want %q", payload.ChainID, chainID)
	}
	if payload.Cancelled != 2 {
		t.Fatalf("cancelled=%d want 2", payload.Cancelled)
	}
	// Both runs are now terminal.
	root, _ := st.GetRun(t.Context(), chainID)
	if root.Status != "cancelled" {
		t.Fatalf("root status=%q want cancelled", root.Status)
	}
	sibling, _ := st.GetRun(t.Context(), "sibling-run")
	if sibling.Status != "cancelled" {
		t.Fatalf("sibling status=%q want cancelled", sibling.Status)
	}
}
