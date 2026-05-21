package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/coreline-ai/cron-agent-dashboard/internal/worker"
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
	autoClose := true
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:               "AI News",
		Slug:               "ai-news",
		IdentifierPrefix:   "NEWS",
		AutoCloseOnRunDone: &autoClose,
		MainAgent:          store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
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

func TestWorkerStoreClaimedRunPreservesTriggerContentSnapshot(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Delegation",
		Slug:             "delegation",
		IdentifierPrefix: "DLG",
		MainAgent:        store.CreateAgentInput{Name: "Writer", Runtime: "fake", Instructions: "write"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	body := "@Writer 이 댓글을 기사 초안으로 바꿔줘"
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "snapshot task", Body: body})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	claimed, err := NewWorkerStore(st).ClaimNextRun(ctx, "worker-snapshot")
	if err != nil {
		t.Fatalf("claim next run: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected claimed run")
	}
	if claimed.TriggerContentSnapshot != run.TriggerContentSnapshot {
		t.Fatalf("snapshot mismatch: claimed=%q run=%q", claimed.TriggerContentSnapshot, run.TriggerContentSnapshot)
	}
	if claimed.TriggerContentSnapshot != body {
		t.Fatalf("snapshot not preserved: %q", claimed.TriggerContentSnapshot)
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
	if got.TerminalReason != store.TerminalReasonExecutorError || got.FailureKind != store.FailureKindExecutorError {
		t.Fatalf("missing runtime should be executor_error, got %#v", got)
	}
	gotIssue, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotIssue.Status != "open" {
		t.Fatalf("failed run should keep issue open, got %s", gotIssue.Status)
	}
}

func TestWorkerStoreClassifiesTimeout(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Timeout",
		Slug:             "timeout",
		IdentifierPrefix: "TMO",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "fake", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "timeout task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	workerStore := NewWorkerStore(st)
	if err := workerStore.FinishRun(ctx, run.ID, worker.ExecutionResult{RunID: run.ID, ExitCode: -1, TimedOut: true, ProcessStarted: true, Error: context.DeadlineExceeded}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.TerminalReason != store.TerminalReasonTimeout || got.FailureKind != store.FailureKindTimeout {
		t.Fatalf("timeout classification failed: %#v", got)
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

func TestWorkerStoreResolvesTimeoutAndRecordsMetrics(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Metrics",
		Slug:             "metrics",
		IdentifierPrefix: "MET",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "fake", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE workspace SET default_timeout_seconds=42 WHERE id=?`, ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE agent SET timeout_seconds_override=7 WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "measure"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := NewWorkerStore(st).ClaimNextRun(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.TimeoutSeconds != 7 {
		t.Fatalf("TimeoutSeconds=%d, want agent override", claimed.TimeoutSeconds)
	}
	if err := NewWorkerStore(st).FinishRun(ctx, run.ID, worker.ExecutionResult{
		RunID:    run.ID,
		ExitCode: 0,
		Metrics:  worker.RunMetrics{InputTokens: 100, OutputTokens: 25, TotalCostMicros: 1234, ModelResolved: "gpt-test"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputTokens != 100 || got.OutputTokens != 25 || got.TotalCostMicros != 1234 || got.ModelResolved != "gpt-test" {
		t.Fatalf("metrics not recorded: %#v", got)
	}
}

func TestWorkerStoreRetriesTransientFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, agent, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Retry",
		Slug:             "retry",
		IdentifierPrefix: "TRY",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "fake", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE agent SET retry_policy_json='{"max_attempts":3}' WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := st.ClaimNextRun(ctx, "worker")
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if claimed.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts=%d, want 3", claimed.MaxAttempts)
	}
	if err := NewWorkerStore(st).FinishRun(ctx, run.ID, worker.ExecutionResult{RunID: run.ID, ExitCode: -1, TimedOut: true, ProcessStarted: true, Error: context.DeadlineExceeded}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" || got.Attempt != 2 || got.NextRetryAt == "" {
		t.Fatalf("transient failure should be rescheduled: %#v", got)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker-too-early"); err != nil || ok {
		t.Fatalf("retry should not be claimable before next_retry_at ok=%v err=%v", ok, err)
	}
}

func TestWorkerStoreRecordsStrippedNoiseAsRunEvent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "NoiseLog",
		Slug:             "noise-log",
		IdentifierPrefix: "NSL",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "noise"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkerStore(st).ClaimNextRun(ctx, "worker"); err != nil {
		t.Fatal(err)
	}

	stdoutPath := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(stdoutPath, []byte("MCP issues detected. Run /mcp list for status.\n\n# Result\nbody"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := NewWorkerStore(st).FinishRun(ctx, run.ID, worker.ExecutionResult{
		RunID:      run.ID,
		Runtime:    "codex",
		ExitCode:   0,
		StdoutPath: stdoutPath,
	}); err != nil {
		t.Fatal(err)
	}

	comments, err := st.ListComments(ctx, run.IssueID)
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, c := range comments {
		if c.AuthorType == "agent" && c.RunID == run.ID {
			body = c.Content
			break
		}
	}
	if !strings.HasPrefix(body, "# Result") {
		t.Fatalf("agent comment not sanitized; got=%q", body)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sanitized *store.RunEvent
	for i := range events {
		if events[i].EventType == store.RunEventStdoutSanitized {
			sanitized = &events[i]
			break
		}
	}
	if sanitized == nil {
		t.Fatalf("no stdout_sanitized event recorded; events=%v", events)
	}
	if got := sanitized.Severity; got != store.RunEventSeverityInfo {
		t.Fatalf("event severity=%q, want info", got)
	}
	if got, ok := sanitized.Details["runtime"].(string); !ok || got != "codex" {
		t.Fatalf("event runtime=%v, want codex", sanitized.Details["runtime"])
	}
	count, _ := sanitized.Details["count"].(float64)
	if int(count) != 1 {
		t.Fatalf("event count=%v, want 1", sanitized.Details["count"])
	}
	stripped, ok := sanitized.Details["stripped"].([]any)
	if !ok || len(stripped) != 1 {
		t.Fatalf("event stripped=%v, want []string of len 1", sanitized.Details["stripped"])
	}
	if first, _ := stripped[0].(string); first != "MCP issues detected. Run /mcp list for status." {
		t.Fatalf("stripped[0]=%v, want known MCP line", stripped[0])
	}
}

func TestWorkerStorePerRunWorktreeAllocatesAndCleansUp(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	dataDir := t.TempDir()
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Worktree",
		Slug:             "wt",
		IdentifierPrefix: "WT",
		PerRunWorktree:   true,
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "wt"})
	if err != nil {
		t.Fatal(err)
	}

	workerStore := NewWorkerStore(st, WithDataDir(dataDir))
	claimed, err := workerStore.ClaimNextRun(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil {
		t.Fatalf("claim returned nil")
	}
	wantPath := filepath.Join(dataDir, "worktrees", "wt", run.ID)
	if claimed.WorkspaceWorkingDir != wantPath {
		t.Fatalf("WorkspaceWorkingDir=%q want %q", claimed.WorkspaceWorkingDir, wantPath)
	}
	if info, err := os.Stat(wantPath); err != nil {
		t.Fatalf("worktree path not created: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("worktree path is not a directory: %v", info.Mode())
	}

	if err := workerStore.FinishRun(ctx, run.ID, worker.ExecutionResult{
		RunID:    run.ID,
		ExitCode: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree to be cleaned up after FinishRun; stat err=%v", err)
	}
}

func TestWorkerStoreWithoutPerRunWorktreeUsesWorkspaceWorkingDir(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	dataDir := t.TempDir()
	defaultDir := t.TempDir()
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "NoWorktree",
		Slug:             "nowt",
		IdentifierPrefix: "NW",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "nowt"}); err != nil {
		t.Fatal(err)
	}

	workerStore := NewWorkerStore(st, WithDefaultWorkDir(defaultDir), WithDataDir(dataDir))
	claimed, err := workerStore.ClaimNextRun(ctx, "w")
	if err != nil || claimed == nil {
		t.Fatalf("claim ok=%v err=%v", claimed, err)
	}
	if strings.Contains(claimed.WorkspaceWorkingDir, filepath.Join("worktrees", "nowt")) {
		t.Fatalf("per_run_worktree=false should not use worktree path; got %q", claimed.WorkspaceWorkingDir)
	}
}

func TestWorkerStoreSkipsRunEventWhenNoNoise(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Clean",
		Slug:             "clean-log",
		IdentifierPrefix: "CLN",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "clean"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkerStore(st).ClaimNextRun(ctx, "worker"); err != nil {
		t.Fatal(err)
	}

	stdoutPath := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(stdoutPath, []byte("# Clean Result\nno noise here"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := NewWorkerStore(st).FinishRun(ctx, run.ID, worker.ExecutionResult{
		RunID:      run.ID,
		Runtime:    "codex",
		ExitCode:   0,
		StdoutPath: stdoutPath,
	}); err != nil {
		t.Fatal(err)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.EventType == store.RunEventStdoutSanitized {
			t.Fatalf("did not expect stdout_sanitized event for clean output; got=%#v", e)
		}
	}
}
