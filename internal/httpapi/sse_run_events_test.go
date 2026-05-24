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

// Track D of dev-plan/implement_20260523_092408.md.
//
// /api/issues/{id}/events/stream emits Server-Sent Events for every new
// run_event on the issue. The handler must (a) set the SSE content type,
// (b) send a hello comment immediately so the client SSE subscriber fires
// `onopen`, and (c) push the next event the dispatcher records within a
// few poll ticks.
func TestStreamIssueRunEventsDeliversAppendedEvent(t *testing.T) {
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

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"SSE","slug":"sse","identifier_prefix":"SSE","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/sse/issues", `{"title":"streaming"}`)
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

	// Open the SSE connection. The initial GET should respond immediately
	// with the SSE headers and a comment frame.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/issues/"+issueResp.Issue.ID+"/events/stream", nil)
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
	// Drain the hello comment line ": stream open\n" and the blank line.
	for i := 0; i < 2; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("read hello frame: %v", err)
		}
	}

	// Append a new run_event from a different goroutine; the SSE poller
	// should pick it up within a couple of ticks (the handler polls every
	// 1s).
	go func() {
		// Brief delay so the SSE goroutine actually parks on its ticker.
		time.Sleep(200 * time.Millisecond)
		_, _ = st.AppendRunEvent(context.Background(), store.RunEventInput{
			RunID:     issueResp.Run.ID,
			IssueID:   issueResp.Issue.ID,
			EventType: store.RunEventStarting,
			Message:   "from SSE test",
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	var lastLine string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v (last=%q)", err, lastLine)
		}
		lastLine = line
		if strings.HasPrefix(line, "event: run_event") {
			// Next line should be `data: {...}`.
			data, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(data, "data:") {
				t.Fatalf("expected data line after event, got %q", data)
			}
			// The stream replays historical rows first (issue created at
			// the top of the test enqueues a run_queued event). Keep
			// reading until we see the test's injected message.
			if strings.Contains(data, "from SSE test") {
				return
			}
		}
	}
	t.Fatalf("timed out waiting for SSE event; last line=%q", lastLine)
}

func TestStreamRunEventsIsRunScopedAndTokenProtected(t *testing.T) {
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
		config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul", Token: "stream-secret"},
		WithIssueEventBus(bus),
	)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx := t.Context()
	workspace, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Run SSE",
		Slug:             "run-sse",
		IdentifierPrefix: "RSSE",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	issue, runA, err := st.CreateIssueWithInitialRun(ctx, workspace.ID, store.CreateIssueInput{Title: "stream one run"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, ok, err := st.ClaimNextRun(ctx, "sse-test-worker"); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to claim setup run")
	} else if claimed.ID != runA.ID {
		t.Fatalf("claimed run=%s want %s", claimed.ID, runA.ID)
	}
	if _, err := st.CompleteRunWithReason(ctx, runA.ID, store.FinishRunInput{ExitCode: 0, Content: "done"}); err != nil {
		t.Fatal(err)
	}
	runB, err := st.RerunIssue(ctx, issue.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	history, err := st.ListIssueRunEventsSince(ctx, issue.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Fatal("expected setup history")
	}
	since := history[len(history)-1].CreatedAt

	unauthorizedReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/runs/"+runA.ID+"/events/stream?since="+since, nil)
	unauthorizedResp, err := http.DefaultClient.Do(unauthorizedReq)
	if err != nil {
		t.Fatalf("open unauthorized SSE: %v", err)
	}
	_ = unauthorizedResp.Body.Close()
	if unauthorizedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized run SSE status=%d", unauthorizedResp.StatusCode)
	}

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/runs/"+runA.ID+"/events/stream?since="+since, nil)
	req.Header.Set("Authorization", "Bearer stream-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open run SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run SSE status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type=%q want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	// Drain the hello comment line ": stream open\n" and the blank line.
	for i := 0; i < 2; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("read hello frame: %v", err)
		}
	}

	go func() {
		time.Sleep(120 * time.Millisecond)
		_, _ = st.AppendRunEvent(context.Background(), store.RunEventInput{
			RunID:     runB.ID,
			IssueID:   issue.ID,
			EventType: store.RunEventStarting,
			Message:   "run-b-only",
		})
		time.Sleep(120 * time.Millisecond)
		_, _ = st.AppendRunEvent(context.Background(), store.RunEventInput{
			RunID:     runA.ID,
			IssueID:   issue.ID,
			EventType: store.RunEventStarting,
			Message:   "run-a-visible",
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	var lastLine string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read run SSE: %v (last=%q)", err, lastLine)
		}
		lastLine = line
		if !strings.HasPrefix(line, "event: run_event") {
			continue
		}
		data, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(data, "run-b-only") {
			t.Fatalf("run-scoped stream leaked another run event: %s", data)
		}
		if strings.Contains(data, "run-a-visible") {
			return
		}
	}
	t.Fatalf("timed out waiting for run-scoped SSE event; last line=%q", lastLine)
}
