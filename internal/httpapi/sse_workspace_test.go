package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/app"
	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track A of dev-plan/implement_20260523_203219.md.
//
// /api/workspaces/{slug}/runs/stream emits an SSE wake-up frame whenever
// any issue in the workspace fires a run_event. The frame carries no
// payload (clients re-fetch /workspaces/{slug}/runs on receipt). This
// test asserts that an event appended against issue A wakes a workspace
// subscriber even when no per-issue subscriber is registered.
func TestStreamWorkspaceRunEventsWakesOnAnyIssue(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	bus := app.NewIssueEventBus(app.WithWorkspaceResolver(func(issueID string) string {
		var ws string
		_ = st.DB().GetContext(t.Context(), &ws, `SELECT workspace_id FROM issue WHERE id=?`, issueID)
		return ws
	}))
	st.SetRunEventNotifier(bus)

	h := New(st,
		config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"},
		WithIssueEventBus(bus),
	)
	srv := httptest.NewServer(h)
	defer srv.Close()

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"WSSSE","slug":"ws-sse","identifier_prefix":"WSS","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/ws-sse/issues", `{"title":"workspace stream"}`)
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

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/workspaces/ws-sse/runs/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type=%q want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	// Drain hello comment + blank line.
	for i := 0; i < 2; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("read hello: %v", err)
		}
	}

	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = st.AppendRunEvent(context.Background(), store.RunEventInput{
			RunID:     issueResp.Run.ID,
			IssueID:   issueResp.Issue.ID,
			EventType: store.RunEventStarting,
			Message:   "ws stream wake",
		})
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		if strings.HasPrefix(line, "event: wake") {
			data, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(data, "data:") {
				t.Fatalf("expected data line, got %q", data)
			}
			return
		}
	}
	t.Fatalf("workspace SSE did not emit a wake frame within 3s")
}
