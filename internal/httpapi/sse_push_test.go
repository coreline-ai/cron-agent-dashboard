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

// Track A of dev-plan/implement_20260523_201535.md.
//
// When the in-process IssueEventBus is wired, AppendRunEvent commits wake
// the SSE handler within milliseconds rather than waiting on the keep-alive
// ticker. The test fires an event ~200ms after the stream opens and asserts
// the client sees it well inside the 15s keep-alive interval — a regression
// that broke the notifier wiring would either silently fall back to the
// keep-alive cadence (slow) or never push at all.
func TestStreamIssueRunEventsUsesIssueEventBusForPush(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	bus := app.NewIssueEventBus()
	st.SetRunEventNotifier(bus)

	h := New(st,
		config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"},
		WithIssueEventBus(bus),
	)
	srv := httptest.NewServer(h)
	defer srv.Close()

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"PushSSE","slug":"push-sse","identifier_prefix":"PUS","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/push-sse/issues", `{"title":"push streaming"}`)
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

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	// since=now ensures the initial flush does NOT replay the historical
	// run_queued event, so the next frame we read must be the one we
	// inject after the stream opens.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/issues/"+issueResp.Issue.ID+"/events/stream?since="+now, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status=%d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	// Drain the hello comment.
	for i := 0; i < 2; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("read hello: %v", err)
		}
	}

	pushDeadline := time.Now().Add(3 * time.Second) // way under the 15s keep-alive
	startedAt := time.Now()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = st.AppendRunEvent(context.Background(), store.RunEventInput{
			RunID:     issueResp.Run.ID,
			IssueID:   issueResp.Issue.ID,
			EventType: store.RunEventStarting,
			Message:   "fast push",
		})
	}()

	for time.Now().Before(pushDeadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		if strings.HasPrefix(line, "event: run_event") {
			data, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(data, "fast push") {
				elapsed := time.Since(startedAt)
				// 3s budget covers the 150ms inject delay plus generous
				// CI jitter. A push-driven handler usually reads within
				// ~5ms once OnRunEvent fires.
				if elapsed > 3*time.Second {
					t.Fatalf("push latency too high: %v", elapsed)
				}
				return
			}
		}
	}
	t.Fatalf("did not receive pushed event before deadline")
}
