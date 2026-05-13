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
	RunID               string
	WorkspaceWorkingDir string
	AgentRuntime        string
	AgentInstructions   string
	AgentModel          string
	IssueTitle          string
	IssueBody           string
	RecentComments      []CommentSnippet
}

type ClaimStore interface {
	ClaimNextRun(ctx context.Context, workerID string) (*ClaimedRun, error)
	FinishRun(ctx context.Context, runID string, result ExecutionResult) error
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
	workerID     string
	log          *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu             sync.Mutex
	cancels        map[string]context.CancelFunc
	pendingCancels map[string]struct{}
}

type PoolOption func(*Pool)

func NewPool(store ClaimStore, executor RunExecutor, opts ...PoolOption) *Pool {
	p := &Pool{
		store:          store,
		executor:       executor,
		workers:        3,
		pollInterval:   time.Second,
		workerID:       defaultWorkerID(),
		log:            slog.Default(),
		cancels:        make(map[string]context.CancelFunc),
		pendingCancels: make(map[string]struct{}),
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
	return nil
}

func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	cancel := p.cancel
	for _, cancelRun := range p.cancels {
		cancelRun()
	}
	clear(p.pendingCancels)
	p.mu.Unlock()
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
		p.pendingCancels[runID] = struct{}{}
	}
	p.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
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

func (p *Pool) claimOnce(workerID string) {
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("worker loop recovered panic", "worker", workerID, "panic", r)
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

	runCtx, cancel := context.WithCancel(p.ctx)
	p.mu.Lock()
	_, cancelPending := p.pendingCancels[run.RunID]
	delete(p.pendingCancels, run.RunID)
	p.cancels[run.RunID] = cancel
	p.mu.Unlock()
	if cancelPending {
		cancel()
	}
	defer func() {
		cancel()
		p.mu.Lock()
		delete(p.cancels, run.RunID)
		delete(p.pendingCancels, run.RunID)
		p.mu.Unlock()
	}()

	result := p.executor.Execute(runCtx, ExecutionContext{
		RunID:               run.RunID,
		WorkspaceWorkingDir: run.WorkspaceWorkingDir,
		AgentRuntime:        run.AgentRuntime,
		AgentInstructions:   run.AgentInstructions,
		AgentModel:          run.AgentModel,
		IssueTitle:          run.IssueTitle,
		IssueBody:           run.IssueBody,
		RecentComments:      run.RecentComments,
	})
	if err := p.store.FinishRun(context.Background(), run.RunID, result); err != nil {
		p.log.Error("finish run failed", "run_id", run.RunID, "error", err)
	}
}

func defaultWorkerID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
