package runtime

import (
	"context"
	"os/exec"
)

// ClaudeAdapter builds commands for the claude CLI in non-interactive print mode.
type ClaudeAdapter struct {
	Executable string
}

func (a ClaudeAdapter) Name() string { return RuntimeClaude }

func (a ClaudeAdapter) Detect(ctx context.Context) RuntimeInfo {
	return detectExecutable(ctx, RuntimeClaude, a.executable(), "--version")
}

func (a ClaudeAdapter) BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error) {
	args := []string{"--print"}
	if run.AgentModel != "" {
		args = append(args, "--model", run.AgentModel)
	}
	return commandWithPrompt(ctx, a.executable(), args, run)
}

func (a ClaudeAdapter) executable() string {
	if a.Executable != "" {
		return a.Executable
	}
	return RuntimeClaude
}

func (a ClaudeAdapter) ParseMetrics(stdout, stderr string) RunMetrics {
	metrics := ParseMetricsFromText(stdout, stderr)
	return metrics
}
