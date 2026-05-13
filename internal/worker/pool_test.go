package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type poolCancelStore struct {
	claimed  int32
	finished chan ExecutionResult
}

func (s *poolCancelStore) ClaimNextRun(ctx context.Context, workerID string) (*ClaimedRun, error) {
	if atomic.CompareAndSwapInt32(&s.claimed, 0, 1) {
		return &ClaimedRun{RunID: "run-1", IssueTitle: "task"}, nil
	}
	return nil, nil
}

func (s *poolCancelStore) FinishRun(ctx context.Context, runID string, result ExecutionResult) error {
	s.finished <- result
	return nil
}

func (s *poolCancelStore) CancelRun(ctx context.Context, runID, reason string) error { return nil }

type cancelAwareExecutor struct{}

func (e cancelAwareExecutor) Execute(ctx context.Context, run ExecutionContext) ExecutionResult {
	select {
	case <-ctx.Done():
		return ExecutionResult{RunID: run.RunID, Cancelled: true, Error: ctx.Err()}
	case <-time.After(time.Second):
		return ExecutionResult{RunID: run.RunID, ExitCode: 0}
	}
}

func TestPoolHonorsPendingCancelBeforeExecutorRegistration(t *testing.T) {
	st := &poolCancelStore{finished: make(chan ExecutionResult, 1)}
	pool := NewPool(st, cancelAwareExecutor{}, WithPoolSize(1), WithPollInterval(time.Hour), WithWorkerID("test-worker"))

	if pool.CancelRun("run-1") {
		t.Fatal("pending cancel should report false before an active process cancel func is registered")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = pool.Shutdown(shutdownCtx)
	}()

	select {
	case result := <-st.finished:
		if !result.Cancelled {
			t.Fatalf("pending cancel did not cancel execution context: %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled run to finish")
	}
}
