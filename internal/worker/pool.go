package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

type ClaimedRun struct {
	RunID                  string
	WorkspaceWorkingDir    string
	AgentRuntime           string
	AgentInstructions      string
	AgentModel             string
	IssueTitle             string
	IssueBody              string
	TriggerContentSnapshot string
	RecentComments         []CommentSnippet
	TimeoutSeconds         int
}

func (r ClaimedRun) RelativeRunLogPath() string {
	if r.RunID == "" || r.WorkspaceWorkingDir == "" {
		return ""
	}
	return ".cron-runs/" + r.RunID + ".log"
}

type ClaimStore interface {
	ClaimNextRun(ctx context.Context, workerID string) (*ClaimedRun, error)
	HeartbeatRun(ctx context.Context, runID string) error
	RecoverStaleRuns(ctx context.Context, cutoff string, excludeRunIDs []string) (int64, error)
	FinishRun(ctx context.Context, runID string, result ExecutionResult) error
	FailRun(ctx context.Context, runID, terminalReason, failureKind, errMsg string) error
	CancelRun(ctx context.Context, runID, reason string) error
}

type RunExecutor interface {
	Execute(ctx context.Context, run ExecutionContext) ExecutionResult
}

type Pool struct {
	store        ClaimStore
	executor     RunExecutor
	workers      int
	pollInterval time.Duration
	heartbeat    time.Duration
	staleAfter   time.Duration
	staleScan    time.Duration
	workerID     string
	log          *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu             sync.Mutex
	cancels        map[string]context.CancelCauseFunc
	pendingCancels map[string]string
	heartbeatOKAt  map[string]time.Time

	consecutivePanics  int
	panicThreshold     int
	panicCooldown      time.Duration
	panicCooldownUntil time.Time
}

type PoolOption func(*Pool)

const (
	DefaultHeartbeatInterval = 10 * time.Second
	DefaultStaleAfter        = 120 * time.Second
	DefaultStaleScanInterval = 30 * time.Second
	DefaultPanicThreshold    = 3
	DefaultPanicCooldown     = time.Minute

	runCancelReasonUser     = "user"
	runCancelReasonShutdown = "shutdown"
	runCancelReasonStale    = "stale"
)

var (
	errRunCancelUser     = errors.New("user cancel")
	errRunCancelShutdown = errors.New("shutdown cancel")
	errRunCancelStale    = errors.New("stale cancel")
)

func NewPool(store ClaimStore, executor RunExecutor, opts ...PoolOption) *Pool {
	p := &Pool{
		store:          store,
		executor:       executor,
		workers:        3,
		pollInterval:   time.Second,
		heartbeat:      DefaultHeartbeatInterval,
		staleAfter:     DefaultStaleAfter,
		staleScan:      DefaultStaleScanInterval,
		workerID:       defaultWorkerID(),
		log:            slog.Default(),
		cancels:        make(map[string]context.CancelCauseFunc),
		pendingCancels: make(map[string]string),
		heartbeatOKAt:  make(map[string]time.Time),
		panicThreshold: DefaultPanicThreshold,
		panicCooldown:  DefaultPanicCooldown,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithPoolSize(n int) PoolOption {
	return func(p *Pool) {
		if n > 0 {
			p.workers = n
		}
	}
}

func WithPollInterval(d time.Duration) PoolOption {
	return func(p *Pool) {
		if d > 0 {
			p.pollInterval = d
		}
	}
}

func WithHeartbeatInterval(d time.Duration) PoolOption {
	return func(p *Pool) {
		if d >= 0 {
			p.heartbeat = d
		}
	}
}

func WithStaleAfter(d time.Duration) PoolOption {
	return func(p *Pool) {
		if d > 0 {
			p.staleAfter = d
		}
	}
}

func WithStaleScanInterval(d time.Duration) PoolOption {
	return func(p *Pool) {
		if d > 0 {
			p.staleScan = d
		}
	}
}

func WithWorkerID(id string) PoolOption {
	return func(p *Pool) {
		if id != "" {
			p.workerID = id
		}
	}
}

func WithLogger(log *slog.Logger) PoolOption {
	return func(p *Pool) {
		if log != nil {
			p.log = log
		}
	}
}

func WithPanicThreshold(n int) PoolOption {
	return func(p *Pool) {
		if n >= 0 {
			p.panicThreshold = n
		}
	}
}

func WithPanicCooldown(d time.Duration) PoolOption {
	return func(p *Pool) {
		if d >= 0 {
			p.panicCooldown = d
		}
	}
}

func (p *Pool) Start(ctx context.Context) error {
	if p.store == nil {
		return errors.New("worker pool: store is nil")
	}
	if p.executor == nil {
		return errors.New("worker pool: executor is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return errors.New("worker pool: already started")
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	for i := 0; i < p.workers; i++ {
		idx := i
		p.wg.Add(1)
		go p.loop(idx)
	}
	if p.staleScan > 0 && p.staleAfter > 0 {
		p.wg.Add(1)
		go p.staleLoop()
	}
	return nil
}

func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	cancel := p.cancel
	activeCancels := make([]context.CancelCauseFunc, 0, len(p.cancels))
	for _, cancelRun := range p.cancels {
		activeCancels = append(activeCancels, cancelRun)
	}
	clear(p.pendingCancels)
	p.mu.Unlock()
	for _, cancelRun := range activeCancels {
		cancelRun(errRunCancelShutdown)
	}
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) CancelRun(runID string) bool {
	p.mu.Lock()
	cancel, ok := p.cancels[runID]
	if !ok {
		p.pendingCancels[runID] = runCancelReasonUser
	}
	p.mu.Unlock()
	if ok {
		cancel(errRunCancelUser)
	}
	return ok
}

func (p *Pool) ForgetPendingCancel(runID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, active := p.cancels[runID]; active {
		return false
	}
	if _, pending := p.pendingCancels[runID]; !pending {
		return false
	}
	delete(p.pendingCancels, runID)
	return true
}

func (p *Pool) loop(workerIndex int) {
	defer p.wg.Done()
	workerID := fmt.Sprintf("%s-%d", p.workerID, workerIndex)
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		p.claimOnce(workerID)
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Pool) staleLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.staleScan)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.recoverStaleRuns()
		}
	}
}

func (p *Pool) recoverStaleRuns() {
	now := time.Now().UTC()
	p.cancelStaleActiveRuns(now)
	cutoff := now.Add(-p.staleAfter).Format(time.RFC3339Nano)
	recovered, err := p.store.RecoverStaleRuns(context.Background(), cutoff, p.activeRunIDs())
	if err != nil {
		p.log.Warn("recover stale runs failed", "error", err)
		return
	}
	if recovered > 0 {
		p.log.Warn("recovered stale runs", "count", recovered)
	}
}

func (p *Pool) activeRunIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.cancels))
	for id := range p.cancels {
		ids = append(ids, id)
	}
	return ids
}

func (p *Pool) cancelStaleActiveRuns(now time.Time) {
	if p.heartbeat <= 0 || p.staleAfter <= 0 {
		return
	}
	type staleRun struct {
		runID  string
		last   time.Time
		cancel context.CancelCauseFunc
	}
	var stale []staleRun
	p.mu.Lock()
	for runID, last := range p.heartbeatOKAt {
		if now.Sub(last) < p.staleAfter {
			continue
		}
		cancel, ok := p.cancels[runID]
		if !ok {
			continue
		}
		stale = append(stale, staleRun{runID: runID, last: last, cancel: cancel})
	}
	p.mu.Unlock()
	for _, run := range stale {
		p.log.Warn("active run heartbeat is stale; cancelling run", "run_id", run.runID, "last_successful_heartbeat", run.last, "stale_after", p.staleAfter)
		run.cancel(errRunCancelStale)
	}
}

func (p *Pool) claimOnce(workerID string) {
	if remaining := p.panicCooldownRemaining(time.Now().UTC()); remaining > 0 {
		p.log.Debug("worker panic circuit breaker open; skipping claim", "worker", workerID, "remaining", remaining)
		return
	}

	var activeRunID string
	var cancel context.CancelCauseFunc
	var heartbeatDone chan struct{}
	stopHeartbeat := func() {
		if heartbeatDone != nil {
			close(heartbeatDone)
			heartbeatDone = nil
		}
	}
	defer func() {
		panicValue := recover()
		stopHeartbeat()
		if cancel != nil {
			cancel(nil)
		}
		if activeRunID != "" {
			p.mu.Lock()
			delete(p.cancels, activeRunID)
			delete(p.pendingCancels, activeRunID)
			delete(p.heartbeatOKAt, activeRunID)
			p.mu.Unlock()
		}
		if panicValue != nil {
			panicCount, cooldownUntil := p.recordWorkerPanic()
			p.log.Error("worker loop recovered panic", "worker", workerID, "run_id", activeRunID, "panic", panicValue, "consecutive_panics", panicCount)
			if !cooldownUntil.IsZero() {
				p.log.Error("worker panic circuit breaker opened", "worker", workerID, "threshold", p.panicThreshold, "cooldown_until", cooldownUntil)
			}
			if activeRunID != "" {
				msg := fmt.Sprintf("worker panic: %v", panicValue)
				if err := p.store.FailRun(context.Background(), activeRunID, "worker_panic", "worker_panic", msg); err != nil {
					p.log.Error("mark worker panic run failed", "run_id", activeRunID, "error", err)
				}
			}
		}
	}()

	run, err := p.store.ClaimNextRun(p.ctx, workerID)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			p.log.Warn("claim run failed", "worker", workerID, "error", err)
		}
		return
	}
	if run == nil {
		return
	}
	activeRunID = run.RunID

	runCtx, runCancel := context.WithCancelCause(p.ctx)
	cancel = runCancel
	p.mu.Lock()
	pendingReason, cancelPending := p.pendingCancels[run.RunID]
	delete(p.pendingCancels, run.RunID)
	p.cancels[run.RunID] = runCancel
	if p.heartbeat > 0 {
		p.heartbeatOKAt[run.RunID] = time.Now().UTC()
	}
	p.mu.Unlock()
	if cancelPending {
		runCancel(cancelCauseForReason(pendingReason))
	}
	if p.heartbeat > 0 {
		heartbeatDone = make(chan struct{})
		go p.heartbeatLoop(run.RunID, heartbeatDone)
	}

	result := p.executor.Execute(runCtx, ExecutionContext{
		RunID:                  run.RunID,
		WorkspaceWorkingDir:    run.WorkspaceWorkingDir,
		AgentRuntime:           run.AgentRuntime,
		AgentInstructions:      run.AgentInstructions,
		AgentModel:             run.AgentModel,
		IssueTitle:             run.IssueTitle,
		IssueBody:              run.IssueBody,
		TriggerContentSnapshot: run.TriggerContentSnapshot,
		RecentComments:         run.RecentComments,
		TimeoutSeconds:         run.TimeoutSeconds,
	})
	stopHeartbeat()
	if result.Cancelled && result.CancelReason == "" {
		result.CancelReason = cancelReasonFromCause(context.Cause(runCtx))
	}
	if err := p.store.FinishRun(context.Background(), run.RunID, result); err != nil {
		p.log.Error("finish run failed", "run_id", run.RunID, "error", err)
	}
	p.resetWorkerPanics()
}

func (p *Pool) heartbeatLoop(runID string, done <-chan struct{}) {
	ticker := time.NewTicker(p.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if err := p.store.HeartbeatRun(context.Background(), runID); err != nil {
				p.log.Warn("heartbeat run failed", "run_id", runID, "error", err)
				continue
			}
			p.markHeartbeatOK(runID, time.Now().UTC())
		}
	}
}

func (p *Pool) markHeartbeatOK(runID string, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.cancels[runID]; !ok {
		return
	}
	p.heartbeatOKAt[runID] = at
}

func (p *Pool) recordWorkerPanic() (int, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutivePanics++
	count := p.consecutivePanics
	if p.panicThreshold <= 0 || p.panicCooldown <= 0 || count < p.panicThreshold {
		return count, time.Time{}
	}
	until := time.Now().UTC().Add(p.panicCooldown)
	if until.After(p.panicCooldownUntil) {
		p.panicCooldownUntil = until
	}
	return count, p.panicCooldownUntil
}

func (p *Pool) resetWorkerPanics() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutivePanics = 0
}

func (p *Pool) panicCooldownRemaining(now time.Time) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.panicThreshold <= 0 || p.panicCooldown <= 0 || p.panicCooldownUntil.IsZero() {
		return 0
	}
	if !now.Before(p.panicCooldownUntil) {
		p.panicCooldownUntil = time.Time{}
		return 0
	}
	return p.panicCooldownUntil.Sub(now)
}

func cancelCauseForReason(reason string) error {
	if reason == runCancelReasonShutdown {
		return errRunCancelShutdown
	}
	if reason == runCancelReasonStale {
		return errRunCancelStale
	}
	return errRunCancelUser
}

func cancelReasonFromCause(cause error) string {
	if errors.Is(cause, errRunCancelShutdown) {
		return runCancelReasonShutdown
	}
	if errors.Is(cause, errRunCancelStale) {
		return runCancelReasonStale
	}
	return runCancelReasonUser
}

func defaultWorkerID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
