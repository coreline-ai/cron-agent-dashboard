package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track A of dev-plan/implement_20260523_205014.md.
//
// /api/runs/{id}/log/stream tails the recorded stdout file. New bytes
// appended to the file surface as `event: chunk` frames. When the run
// reaches a terminal status the handler sends a final flush + an
// `event: done` frame and closes the stream.
func TestStreamRunLogTailsNewBytesUntilTerminal(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	h := New(st, config.Config{DataDir: dir, DBPath: filepath.Join(dir, "data.db"), Bind: "127.0.0.1:0", Workers: 1, Timezone: "Asia/Seoul"})
	srv := httptest.NewServer(h)
	defer srv.Close()

	if res := do(t, h, http.MethodPost, "/api/workspaces", `{"name":"LogTail","slug":"log-tail","identifier_prefix":"LGT","main_agent":{"name":"Lead","runtime":"codex","instructions":"lead"}}`); res.Code != http.StatusCreated {
		t.Fatalf("seed workspace: %s", res.Body.String())
	}
	issueRes := do(t, h, http.MethodPost, "/api/workspaces/log-tail/issues", `{"title":"tail"}`)
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

	// Move the run to 'running' and attach a stdout file with seed content.
	logPath := filepath.Join(dir, "tail.log")
	if err := os.WriteFile(logPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(context.Background(),
		`UPDATE run SET status='running', stdout_path=?, claimed_at=datetime('now'), started_at=datetime('now'), heartbeat_at=datetime('now') WHERE id=?`,
		logPath, issueResp.Run.ID,
	); err != nil {
		t.Fatal(err)
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/runs/"+issueResp.Run.ID+"/log/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)

	// Helper to read up to the next `event: <name>` line plus its data
	// block, returning the joined data payload.
	readEvent := func(deadline time.Time) (string, string, error) {
		var name, data string
		for time.Now().Before(deadline) {
			line, err := reader.ReadString('\n')
			if err != nil {
				return name, data, err
			}
			line = strings.TrimRight(line, "\n")
			if strings.HasPrefix(line, "event: ") {
				name = strings.TrimPrefix(line, "event: ")
				data = ""
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				if data == "" {
					data = strings.TrimPrefix(line, "data: ")
				} else {
					data += "\n" + strings.TrimPrefix(line, "data: ")
				}
				continue
			}
			if line == "" && name != "" {
				return name, data, nil
			}
		}
		return name, data, context.DeadlineExceeded
	}

	// First frame should contain "hello".
	name, data, err := readEvent(time.Now().Add(3 * time.Second))
	if err != nil {
		t.Fatalf("read first chunk: %v", err)
	}
	if name != "chunk" || !strings.Contains(data, "hello") {
		t.Fatalf("first frame=%q data=%q want chunk/hello", name, data)
	}

	// Append a second line and wait for it to arrive.
	go func() {
		time.Sleep(120 * time.Millisecond)
		f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
		_, _ = f.WriteString("world\n")
		_ = f.Close()
	}()
	name, data, err = readEvent(time.Now().Add(3 * time.Second))
	if err != nil {
		t.Fatalf("read second chunk: %v", err)
	}
	if name != "chunk" || !strings.Contains(data, "world") {
		t.Fatalf("second frame=%q data=%q want chunk/world", name, data)
	}

	// Mark the run done — the handler must emit an `event: done` frame
	// and close.
	if _, err := database.ExecContext(context.Background(),
		`UPDATE run SET status='done', finished_at=datetime('now'), exit_code=0 WHERE id=?`,
		issueResp.Run.ID,
	); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		name, _, err := readEvent(deadline)
		if err != nil {
			t.Fatalf("read done frame: %v", err)
		}
		if name == "done" {
			return
		}
	}
	t.Fatalf("did not receive done frame")
}
