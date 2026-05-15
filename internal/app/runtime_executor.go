package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

const defaultProcessMarkerAttemptTimeout = 2 * time.Second

type RunProcessMarker interface {
	MarkRunProcess(ctx context.Context, runID string, pid, pgid int) error
}

type RuntimeExecutor struct {
	adapters                map[string]worker.CommandBuilder
	LogDir                  string
	Timeout                 time.Duration
	ProcessMarker           RunProcessMarker
	ProcessMarkerAttempts   int
	ProcessMarkerRetryDelay time.Duration
	Log                     *slog.Logger
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

func WithRunProcessMarkerRetry(attempts int, delay time.Duration) RuntimeExecutorOption {
	return func(e *RuntimeExecutor) {
		if attempts > 0 {
			e.ProcessMarkerAttempts = attempts
		}
		if delay >= 0 {
			e.ProcessMarkerRetryDelay = delay
		}
	}
}

func NewRuntimeExecutor(adapters []workerruntime.RuntimeAdapter, logDir string, opts ...RuntimeExecutorOption) *RuntimeExecutor {
	out := &RuntimeExecutor{
		adapters:                make(map[string]worker.CommandBuilder),
		LogDir:                  logDir,
		Timeout:                 10 * time.Minute,
		ProcessMarkerAttempts:   3,
		ProcessMarkerRetryDelay: 100 * time.Millisecond,
		Log:                     slog.Default(),
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
	timeout := e.Timeout
	if run.TimeoutSeconds > 0 {
		timeout = time.Duration(run.TimeoutSeconds) * time.Second
	}
	executor := worker.Executor{
		Adapter:        adapter,
		LogDir:         e.LogDir,
		Timeout:        timeout,
		OnStart:        e.prepareRunStart,
		OnProcessStart: e.recordProcessStart,
	}
	result := executor.Execute(ctx, run)
	if parser, ok := adapter.(workerruntime.MetricsParser); ok {
		// Treat agent stdout as untrusted user-controlled content. Runtime usage
		// metrics are parsed only from stderr/side-channel output until adapters
		// provide a dedicated structured metrics stream.
		result.Metrics = parser.ParseMetrics("", result.StderrTail)
	}
	return result
}

func (e *RuntimeExecutor) prepareRunStart(ctx context.Context, run worker.ExecutionContext, stdoutPath string) error {
	if err := e.linkRunLog(ctx, run, stdoutPath); err != nil {
		e.logger().Warn("link run log into workspace failed", "run_id", run.RunID, "stdout_path", stdoutPath, "error", err)
	}
	return nil
}

func (e *RuntimeExecutor) logger() *slog.Logger {
	if e != nil && e.Log != nil {
		return e.Log
	}
	return slog.Default()
}

func (e *RuntimeExecutor) linkRunLog(ctx context.Context, run worker.ExecutionContext, stdoutPath string) error {
	_ = ctx
	if run.WorkspaceWorkingDir == "" || run.RunID == "" || stdoutPath == "" {
		return nil
	}
	dir := filepath.Join(run.WorkspaceWorkingDir, ".corn-runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	linkPath := filepath.Join(dir, run.RunID+".log")
	_ = os.Remove(linkPath)
	if err := os.Symlink(stdoutPath, linkPath); err != nil {
		// Some filesystems disallow symlinks; write a small pointer file instead.
		return os.WriteFile(linkPath, []byte(stdoutPath+"\n"), 0o600)
	}
	return nil
}

func (e *RuntimeExecutor) recordProcessStart(ctx context.Context, run worker.ExecutionContext, info worker.ProcessInfo) error {
	if e == nil || e.ProcessMarker == nil {
		return nil
	}
	attempts := e.ProcessMarkerAttempts
	if attempts <= 0 {
		attempts = 1
	}
	delay := e.ProcessMarkerRetryDelay
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		markCtx, cancel := context.WithTimeout(context.Background(), defaultProcessMarkerAttemptTimeout)
		err := e.ProcessMarker.MarkRunProcess(markCtx, run.RunID, info.PID, info.PGID)
		cancel()
		if err != nil {
			lastErr = err
		} else {
			return nil
		}
		if attempt < attempts && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	log := e.Log
	if log == nil {
		log = slog.Default()
	}
	log.Warn("record run process metadata failed", "run_id", run.RunID, "pid", info.PID, "pgid", info.PGID, "attempts", attempts, "error", lastErr)
	return nil
}
