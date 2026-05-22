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

func TestRetryChainEndpointEnqueuesNewRunOnSameAgent(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"ChainRetry","slug":"chain-rtry","identifier_prefix":"CR","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/chain-rtry/issues", `{"title":"fail then retry"}`)
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

	// Claim + fail the run so the chain has a failed terminal.
	if _, ok, err := st.ClaimNextRun(t.Context(), "w"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(t.Context(), issueResp.Run.ID, 2, "", "", false, "exit code 2"); err != nil {
		t.Fatal(err)
	}

	res := do(t, h, http.MethodPost, "/api/runs/chain/"+chainID+"/retry", "")
	if res.Code != http.StatusCreated {
		t.Fatalf("retry status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Run store.Run `json:"run"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Run.Status != "queued" {
		t.Fatalf("new run status=%q want queued", payload.Run.Status)
	}
	if payload.Run.AgentID != issueResp.Run.AgentID {
		t.Fatalf("new run agent=%q want %q", payload.Run.AgentID, issueResp.Run.AgentID)
	}
	if payload.Run.ChainID != chainID {
		t.Fatalf("new run chain=%q want %q", payload.Run.ChainID, chainID)
	}
}
