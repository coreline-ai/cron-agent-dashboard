package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track B of dev-plan/implement_20260523_203219.md.
//
// POST /api/webhooks/{id}/deliveries/{delivery}/redeliver resets a
// status='failed' row to status='pending' so the dispatcher picks it up
// on its next poll. The HTTP layer surfaces store ErrState as 409 and
// store ErrNotFound as 404.
func TestRedeliverWebhookDeliveryEndpointHappyPath(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"WHRD","slug":"wh-rd","identifier_prefix":"WHRD","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	whRes := do(t, h, http.MethodPost, "/api/workspaces/wh-rd/webhooks", `{"url":"https://x.example/h","events":["issue.cancelled"]}`)
	if whRes.Code != http.StatusCreated {
		t.Fatalf("seed webhook: %s", whRes.Body.String())
	}
	var whCreated struct {
		Webhook struct {
			ID string `json:"id"`
		} `json:"webhook"`
	}
	if err := json.NewDecoder(whRes.Body).Decode(&whCreated); err != nil {
		t.Fatal(err)
	}
	if res := do(t, h, http.MethodPost, "/api/workspaces/wh-rd/issues", `{"title":"trigger"}`); res.Code != http.StatusCreated {
		t.Fatalf("seed issue: %s", res.Body.String())
	}
	// Cancel the issue to enqueue an issue.cancelled delivery.
	issues, _ := st.ListIssues(context.Background(), "", store.ListIssuesFilter{Limit: 5})
	if len(issues) == 0 {
		// ListIssues needs a real workspace id; pick the first.
		// Fall back to using the issue from the create response.
	}
	wsID := ""
	{
		ws, _, _ := st.GetWorkspace(context.Background(), "wh-rd")
		wsID = ws.ID
		listed, _ := st.ListIssues(context.Background(), ws.ID, store.ListIssuesFilter{Limit: 5})
		if len(listed) == 0 {
			t.Fatalf("seeded issue not visible")
		}
		status := "cancelled"
		if _, err := st.UpdateIssue(context.Background(), listed[0].ID, store.UpdateIssueInput{Status: &status}); err != nil {
			t.Fatal(err)
		}
	}
	deliveries, _ := st.ListWebhookDeliveries(context.Background(), whCreated.Webhook.ID, 10)
	if len(deliveries) != 1 {
		t.Fatalf("want 1 delivery row, got %d (workspace %s)", len(deliveries), wsID)
	}
	// Force it to terminal failure to mimic the dispatcher giving up.
	if _, err := st.DB().ExecContext(context.Background(),
		`UPDATE webhook_delivery SET status='failed', attempt=6, status_code=500 WHERE id=?`,
		deliveries[0].ID,
	); err != nil {
		t.Fatal(err)
	}

	// Happy path: redeliver moves the row back to pending.
	rd := do(t, h, http.MethodPost, "/api/webhooks/"+whCreated.Webhook.ID+"/deliveries/"+deliveries[0].ID+"/redeliver", "")
	if rd.Code != http.StatusOK {
		t.Fatalf("redeliver status=%d body=%s", rd.Code, rd.Body.String())
	}
	after, _ := st.ListWebhookDeliveries(context.Background(), whCreated.Webhook.ID, 10)
	if after[0].Status != "pending" || after[0].Attempt != 0 {
		t.Fatalf("redelivered row not reset: %#v", after[0])
	}

	// Second redeliver: row is now pending → ErrState → 409.
	again := do(t, h, http.MethodPost, "/api/webhooks/"+whCreated.Webhook.ID+"/deliveries/"+deliveries[0].ID+"/redeliver", "")
	if again.Code != http.StatusConflict {
		t.Fatalf("redeliver of pending row should be 409, got %d (%s)", again.Code, again.Body.String())
	}
}

func TestRedeliverWebhookDeliveryEndpointNotFound(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})
	res := do(t, h, http.MethodPost, "/api/webhooks/any/deliveries/missing/redeliver", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing delivery id should be 404, got %d (%s)", res.Code, res.Body.String())
	}
}
