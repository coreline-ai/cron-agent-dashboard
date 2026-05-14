package app

import (
	"context"
	"errors"
	"testing"

	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
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
