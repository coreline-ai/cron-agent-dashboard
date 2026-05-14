package worker

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type poolCancelStore struct {
	claimed        int32
	finished       chan ExecutionResult
	heartbeats     chan string
	heartbeatErr   error
	staleExcludes  chan []string
	failed         chan string
	staleRecovered int64
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

func (s *poolCancelStore) HeartbeatRun(ctx context.Context, runID string) error {
	if s.heartbeats != nil {
		s.heartbeats <- runID
	}
	return s.heartbeatErr
}

func (s *poolCancelStore) RecoverStaleRuns(ctx context.Context, cutoff string, excludeRunIDs []string) (int64, error) {
	if s.staleExcludes != nil {
		s.staleExcludes <- append([]string(nil), excludeRunIDs...)
	}
	return s.staleRecovered, nil
}

func (s *poolCancelStore) FailRun(ctx context.Context, runID, terminalReason, failureKind, errMsg string) error {
	if s.failed != nil {
		s.failed <- terminalReason + "/" + failureKind
	}
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

func TestPoolForgetPendingCancel(t *testing.T) {
	pool := NewPool(&poolCancelStore{}, cancelAwareExecutor{})

	if pool.CancelRun("never-claimed") {
		t.Fatal("cancel should be pending before an active cancel func is registered")
	}
	if !pool.ForgetPendingCancel("never-claimed") {
		t.Fatal("expected pending cancel to be forgotten")
	}
	if pool.ForgetPendingCancel("never-claimed") {
		t.Fatal("forget should return false after pending cancel is removed")
	}
}

func TestPoolHeartbeatsActiveRun(t *testing.T) {
	st := &poolCancelStore{finished: make(chan ExecutionResult, 1), heartbeats: make(chan string, 4)}
	pool := NewPool(st, slowExecutor{duration: 60 * time.Millisecond}, WithPoolSize(1), WithPollInterval(time.Hour), WithHeartbeatInterval(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	select {
	case runID := <-st.heartbeats:
		if runID != "run-1" {
			t.Fatalf("heartbeat run=%q, want run-1", runID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for heartbeat")
	}
}

func TestPoolShutdownPreservesCancelReason(t *testing.T) {
	st := &poolCancelStore{finished: make(chan ExecutionResult, 1)}
	pool := NewPool(st, cancelAwareExecutor{}, WithPoolSize(1), WithPollInterval(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	waitForPoolTest(t, time.Second, func() bool { return atomic.LoadInt32(&st.claimed) == 1 })

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case result := <-st.finished:
		if !result.Cancelled || result.CancelReason != runCancelReasonShutdown {
			t.Fatalf("shutdown result=%#v, want cancelled shutdown", result)
		}
	default:
		t.Fatal("expected finished result")
	}
}

func TestPoolPanicFailsRunAndContinues(t *testing.T) {
	st := &sequencePoolStore{runs: []string{"panic-run", "ok-run"}, finished: make(chan string, 1), failed: make(chan string, 1)}
	pool := NewPool(st, panicOnceExecutor{}, WithPoolSize(1), WithPollInterval(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	select {
	case got := <-st.failed:
		if got != "panic-run:worker_panic/worker_panic" {
			t.Fatalf("bad failed run marker: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for panic failure")
	}
	select {
	case got := <-st.finished:
		if got != "ok-run" {
			t.Fatalf("bad finished run: %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("pool did not continue after panic")
	}
}

func TestPoolStaleScannerExcludesActiveRuns(t *testing.T) {
	st := &poolCancelStore{finished: make(chan ExecutionResult, 1), staleExcludes: make(chan []string, 4)}
	pool := NewPool(st, cancelAwareExecutor{}, WithPoolSize(1), WithPollInterval(time.Hour), WithStaleAfter(10*time.Millisecond), WithStaleScanInterval(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	deadline := time.After(time.Second)
	for {
		select {
		case exclude := <-st.staleExcludes:
			if slices.Contains(exclude, "run-1") {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for stale scan with active run excluded")
		}
	}
}

func TestPoolCancelsActiveRunAfterStaleHeartbeatFailures(t *testing.T) {
	st := &poolCancelStore{
		finished:     make(chan ExecutionResult, 1),
		heartbeatErr: errors.New("heartbeat unavailable"),
	}
	pool := NewPool(
		st,
		cancelAwareExecutor{},
		WithPoolSize(1),
		WithPollInterval(time.Hour),
		WithHeartbeatInterval(5*time.Millisecond),
		WithStaleAfter(25*time.Millisecond),
		WithStaleScanInterval(5*time.Millisecond),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	select {
	case result := <-st.finished:
		if !result.Cancelled {
			t.Fatalf("result not cancelled: %#v", result)
		}
		if result.CancelReason != runCancelReasonStale {
			t.Fatalf("cancel reason=%q, want %q; result=%#v", result.CancelReason, runCancelReasonStale, result)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stale active run cancellation")
	}
}

func TestPoolPanicCircuitBreakerBlocksClaimsDuringCooldown(t *testing.T) {
	st := &sequencePoolStore{
		runs:   []string{"panic-1", "panic-2", "panic-3"},
		failed: make(chan string, 3),
	}
	pool := NewPool(
		st,
		panicAlwaysExecutor{},
		WithPoolSize(1),
		WithPollInterval(5*time.Millisecond),
		WithPanicThreshold(2),
		WithPanicCooldown(time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("start pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	for i := 0; i < 2; i++ {
		select {
		case <-st.failed:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for panic failure %d", i+1)
		}
	}

	time.Sleep(75 * time.Millisecond)
	if got := st.claimCount(); got != 2 {
		t.Fatalf("claims during panic cooldown=%d, want 2", got)
	}
}

type slowExecutor struct {
	duration time.Duration
}

func (e slowExecutor) Execute(ctx context.Context, run ExecutionContext) ExecutionResult {
	select {
	case <-ctx.Done():
		return ExecutionResult{RunID: run.RunID, Cancelled: true, Error: ctx.Err()}
	case <-time.After(e.duration):
		return ExecutionResult{RunID: run.RunID, ExitCode: 0}
	}
}

type sequencePoolStore struct {
	mu       sync.Mutex
	runs     []string
	next     int
	finished chan string
	failed   chan string
}

func (s *sequencePoolStore) ClaimNextRun(ctx context.Context, workerID string) (*ClaimedRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.runs) {
		return nil, nil
	}
	runID := s.runs[s.next]
	s.next++
	return &ClaimedRun{RunID: runID, IssueTitle: "task"}, nil
}

func (s *sequencePoolStore) claimCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

func (s *sequencePoolStore) HeartbeatRun(ctx context.Context, runID string) error { return nil }

func (s *sequencePoolStore) RecoverStaleRuns(ctx context.Context, cutoff string, excludeRunIDs []string) (int64, error) {
	return 0, nil
}

func (s *sequencePoolStore) FinishRun(ctx context.Context, runID string, result ExecutionResult) error {
	s.finished <- runID
	return nil
}

func (s *sequencePoolStore) FailRun(ctx context.Context, runID, terminalReason, failureKind, errMsg string) error {
	s.failed <- runID + ":" + terminalReason + "/" + failureKind
	return nil
}

func (s *sequencePoolStore) CancelRun(ctx context.Context, runID, reason string) error { return nil }

type panicOnceExecutor struct{}

func (e panicOnceExecutor) Execute(ctx context.Context, run ExecutionContext) ExecutionResult {
	if run.RunID == "panic-run" {
		panic("boom")
	}
	return ExecutionResult{RunID: run.RunID, ExitCode: 0}
}

type panicAlwaysExecutor struct{}

func (e panicAlwaysExecutor) Execute(ctx context.Context, run ExecutionContext) ExecutionResult {
	panic("boom")
}

func waitForPoolTest(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
