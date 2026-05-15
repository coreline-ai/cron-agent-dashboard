package runtime

import (
	"context"
	"os/exec"
)

// GeminiAdapter builds commands for the gemini CLI in headless prompt mode.
type GeminiAdapter struct {
	Executable string
}

func (a GeminiAdapter) Name() string { return RuntimeGemini }

func (a GeminiAdapter) Detect(ctx context.Context) RuntimeInfo {
	return detectExecutable(ctx, RuntimeGemini, a.executable(), "--version")
}

func (a GeminiAdapter) BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error) {
	args := []string{"--prompt", run.PromptText()}
	if run.AgentModel != "" {
		args = append(args, "--model", run.AgentModel)
	}
	return commandWithoutStdin(ctx, a.executable(), args, run)
}

func (a GeminiAdapter) executable() string {
	if a.Executable != "" {
		return a.Executable
	}
	return RuntimeGemini
}

func (a GeminiAdapter) ParseMetrics(stdout, stderr string) RunMetrics {
	metrics := ParseMetricsFromText(stdout, stderr)
	return metrics
}
