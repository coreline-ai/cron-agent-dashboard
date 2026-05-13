package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/db"
	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
)

type fakeExecutor struct {
	logDir   string
	exitCode int
	err      error
}

func (e fakeExecutor) Execute(_ context.Context, run worker.ExecutionContext) worker.ExecutionResult {
	path := filepath.Join(e.logDir, run.RunID+".log")
	_ = os.MkdirAll(e.logDir, 0o755)
	_ = os.WriteFile(path, []byte("fake runtime output"), 0o644)
	return worker.ExecutionResult{
		RunID:      run.RunID,
		Runtime:    run.AgentRuntime,
		ExitCode:   e.exitCode,
		StdoutPath: path,
		Error:      e.err,
	}
}

func TestWorkerPoolCompletesClaimedRunThroughStore(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "오늘 뉴스"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	pool := worker.NewPool(
		NewWorkerStore(st, WithDefaultWorkDir(filepath.Join(t.TempDir(), "workdirs"))),
		fakeExecutor{logDir: filepath.Join(t.TempDir(), "runs")},
		worker.WithPoolSize(1),
		worker.WithPollInterval(10*time.Millisecond),
	)
	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := pool.Start(poolCtx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	waitFor(t, time.Second, func() bool {
		got, err := st.GetRun(ctx, run.ID)
		return err == nil && got.Status == "done"
	})
	gotIssue, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotIssue.Status != "done" || gotIssue.ExecutionStatus != "done" {
		t.Fatalf("issue status=%s execution=%s", gotIssue.Status, gotIssue.ExecutionStatus)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsComment(comments, "fake runtime output") {
		t.Fatalf("agent output comment not found: %#v", comments)
	}
}

func TestWorkerStoreMissingRuntimeFailsRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "missing", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "오늘 뉴스"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	pool := worker.NewPool(
		NewWorkerStore(st),
		NewRuntimeExecutor(nil, filepath.Join(t.TempDir(), "runs")),
		worker.WithPoolSize(1),
		worker.WithPollInterval(10*time.Millisecond),
	)
	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := pool.Start(poolCtx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	waitFor(t, time.Second, func() bool {
		got, err := st.GetRun(ctx, run.ID)
		return err == nil && got.Status == "failed"
	})
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.ErrorMessage, `runtime "missing" is not configured`) {
		t.Fatalf("unexpected error message: %q", got.ErrorMessage)
	}
	gotIssue, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotIssue.Status != "open" {
		t.Fatalf("failed run should keep issue open, got %s", gotIssue.Status)
	}
}

func TestExecutionErrorMessageIgnoresSuccessfulStderr(t *testing.T) {
	msg := executionErrorMessage(worker.ExecutionResult{
		ExitCode:   0,
		StderrTail: "Loaded cached credentials.\nnon-fatal warning",
	}, 0)
	if msg != "" {
		t.Fatalf("successful stderr should not become run error_message, got %q", msg)
	}
}

func TestExecutionErrorMessageIncludesFailureStderr(t *testing.T) {
	msg := executionErrorMessage(worker.ExecutionResult{
		ExitCode:   2,
		StderrTail: "fatal: command failed",
	}, 2)
	if !strings.Contains(msg, "exit code 2") || !strings.Contains(msg, "stderr: fatal: command failed") {
		t.Fatalf("failure stderr should be preserved, got %q", msg)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return store.New(database)
}

func waitFor(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func containsComment(comments []store.Comment, needle string) bool {
	for _, c := range comments {
		if strings.Contains(c.Content, needle) {
			return true
		}
	}
	return false
}
