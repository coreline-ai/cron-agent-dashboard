package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

type RuntimeExecutor struct {
	adapters map[string]worker.CommandBuilder
	LogDir   string
	Timeout  time.Duration
}

func NewRuntimeExecutor(adapters []workerruntime.RuntimeAdapter, logDir string) *RuntimeExecutor {
	out := &RuntimeExecutor{
		adapters: make(map[string]worker.CommandBuilder),
		LogDir:   logDir,
		Timeout:  10 * time.Minute,
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		out.adapters[strings.ToLower(adapter.Name())] = adapter
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
		Adapter: adapter,
		LogDir:  e.LogDir,
		Timeout: e.Timeout,
	}
	return executor.Execute(ctx, run)
}
