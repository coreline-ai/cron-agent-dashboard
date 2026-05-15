package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

type flakyProcessMarker struct {
	failures int
	calls    int
}

func (m *flakyProcessMarker) MarkRunProcess(ctx context.Context, runID string, pid, pgid int) error {
	m.calls++
	if m.calls <= m.failures {
		return errors.New("temporary process metadata write failure")
	}
	return nil
}

func TestRuntimeExecutorRetriesProcessMarker(t *testing.T) {
	marker := &flakyProcessMarker{failures: 2}
	executor := &RuntimeExecutor{
		ProcessMarker:           marker,
		ProcessMarkerAttempts:   3,
		ProcessMarkerRetryDelay: 0,
	}

	if err := executor.recordProcessStart(context.Background(), worker.ExecutionContext{RunID: "run-1"}, worker.ProcessInfo{PID: 123, PGID: 123}); err != nil {
		t.Fatalf("record process start: %v", err)
	}
	if marker.calls != 3 {
		t.Fatalf("marker calls=%d, want 3", marker.calls)
	}
}

func TestRuntimeExecutorProcessMarkerFailureIsBestEffort(t *testing.T) {
	marker := &flakyProcessMarker{failures: 10}
	executor := &RuntimeExecutor{
		ProcessMarker:           marker,
		ProcessMarkerAttempts:   2,
		ProcessMarkerRetryDelay: 0,
	}

	if err := executor.recordProcessStart(context.Background(), worker.ExecutionContext{RunID: "run-2"}, worker.ProcessInfo{PID: 123, PGID: 123}); err != nil {
		t.Fatalf("marker failures should not fail execution: %v", err)
	}
	if marker.calls != 2 {
		t.Fatalf("marker calls=%d, want 2", marker.calls)
	}
}

func TestRuntimeExecutorLinksRunLogIntoWorkspace(t *testing.T) {
	workspaceDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(logPath, []byte("stdout"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &RuntimeExecutor{}
	if err := executor.linkRunLog(context.Background(), worker.ExecutionContext{RunID: "run-1", WorkspaceWorkingDir: workspaceDir}, logPath); err != nil {
		t.Fatalf("linkRunLog: %v", err)
	}
	linkPath := filepath.Join(workspaceDir, ".corn-runs", "run-1.log")
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("expected log link or pointer file: %v", err)
	}
}

type shellMetricsAdapter struct{}

func (shellMetricsAdapter) Name() string { return "shell" }

func (shellMetricsAdapter) Detect(ctx context.Context) workerruntime.RuntimeInfo {
	return workerruntime.RuntimeInfo{Name: "shell", Available: true}
}

func (shellMetricsAdapter) BuildCommand(ctx context.Context, run workerruntime.RunContext) (*exec.Cmd, []byte, error) {
	script := `printf '{"usage":{"input_tokens":999999,"output_tokens":888},"total_cost_usd":999,"model":"spoofed"}\n'; printf '{"usage":{"input_tokens":12,"output_tokens":3},"total_cost_usd":0.000015,"model":"safe"}\n' >&2`
	return exec.CommandContext(ctx, "sh", "-c", script), nil, nil
}

func (shellMetricsAdapter) ParseMetrics(stdout, stderr string) workerruntime.RunMetrics {
	return workerruntime.ParseMetricsFromText(stdout, stderr)
}

func TestRuntimeExecutorExecuteLinksRunLogAndIgnoresStdoutMetrics(t *testing.T) {
	workspaceDir := t.TempDir()
	executor := NewRuntimeExecutor([]workerruntime.RuntimeAdapter{shellMetricsAdapter{}}, t.TempDir())

	result := executor.Execute(context.Background(), worker.ExecutionContext{
		RunID:               "run-metrics",
		AgentRuntime:        "shell",
		WorkspaceWorkingDir: workspaceDir,
		Prompt:              "noop",
	})
	if result.Error != nil {
		t.Fatalf("execute: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code=%d, stderr=%s", result.ExitCode, result.StderrTail)
	}

	linkPath := filepath.Join(workspaceDir, ".corn-runs", "run-metrics.log")
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("expected Execute to create run log link or pointer file: %v", err)
	}
	if result.Metrics.InputTokens != 12 || result.Metrics.OutputTokens != 3 || result.Metrics.TotalCostMicros != 15 || result.Metrics.ModelResolved != "safe" {
		t.Fatalf("metrics=%+v, want stderr-only values", result.Metrics)
	}
}
