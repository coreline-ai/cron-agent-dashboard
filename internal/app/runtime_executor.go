package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

type RunProcessMarker interface {
	MarkRunProcess(ctx context.Context, runID string, pid, pgid int) error
}

type RuntimeExecutor struct {
	adapters      map[string]worker.CommandBuilder
	LogDir        string
	Timeout       time.Duration
	ProcessMarker RunProcessMarker
	Log           *slog.Logger
}

type RuntimeExecutorOption func(*RuntimeExecutor)

func WithRunProcessMarker(marker RunProcessMarker) RuntimeExecutorOption {
	return func(e *RuntimeExecutor) {
		e.ProcessMarker = marker
	}
}

func WithRuntimeExecutorLogger(log *slog.Logger) RuntimeExecutorOption {
	return func(e *RuntimeExecutor) {
		if log != nil {
			e.Log = log
		}
	}
}

func NewRuntimeExecutor(adapters []workerruntime.RuntimeAdapter, logDir string, opts ...RuntimeExecutorOption) *RuntimeExecutor {
	out := &RuntimeExecutor{
		adapters: make(map[string]worker.CommandBuilder),
		LogDir:   logDir,
		Timeout:  10 * time.Minute,
		Log:      slog.Default(),
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		out.adapters[strings.ToLower(adapter.Name())] = adapter
	}
	for _, opt := range opts {
		opt(out)
	}
	return out
}

func (e *RuntimeExecutor) Execute(ctx context.Context, run worker.ExecutionContext) worker.ExecutionResult {
	runtimeName := strings.ToLower(strings.TrimSpace(run.AgentRuntime))
	if runtimeName == "" {
		runtimeName = workerruntime.RuntimeCodex
	}
	adapter, ok := e.adapters[runtimeName]
	if !ok {
		return worker.ExecutionResult{
			RunID:    run.RunID,
			Runtime:  runtimeName,
			ExitCode: 127,
			Error:    fmt.Errorf("runtime %q is not configured", runtimeName),
		}
	}
	executor := worker.Executor{
		Adapter:        adapter,
		LogDir:         e.LogDir,
		Timeout:        e.Timeout,
		OnProcessStart: e.recordProcessStart,
	}
	return executor.Execute(ctx, run)
}

func (e *RuntimeExecutor) recordProcessStart(ctx context.Context, run worker.ExecutionContext, info worker.ProcessInfo) error {
	if e == nil || e.ProcessMarker == nil {
		return nil
	}
	if err := e.ProcessMarker.MarkRunProcess(ctx, run.RunID, info.PID, info.PGID); err != nil {
		log := e.Log
		if log == nil {
			log = slog.Default()
		}
		log.Warn("record run process metadata failed", "run_id", run.RunID, "pid", info.PID, "pgid", info.PGID, "error", err)
	}
	return nil
}
