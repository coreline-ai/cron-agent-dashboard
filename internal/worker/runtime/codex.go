package runtime

import (
	"context"
	"os/exec"
)

// CodexAdapter builds commands for the codex CLI in non-interactive exec mode.
type CodexAdapter struct {
	Executable string
}

func (a CodexAdapter) Name() string { return RuntimeCodex }

func (a CodexAdapter) Detect(ctx context.Context) RuntimeInfo {
	return detectExecutable(ctx, RuntimeCodex, a.executable(), "--version")
}

func (a CodexAdapter) BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error) {
	args := []string{"exec"}
	if run.AgentModel != "" {
		args = append(args, "--model", run.AgentModel)
	}
	if run.WorkspaceWorkingDir != "" {
		args = append(args, "--cd", run.WorkspaceWorkingDir)
	}
	args = append(args, "-")
	return commandWithPrompt(ctx, a.executable(), args, run)
}

func (a CodexAdapter) executable() string {
	if a.Executable != "" {
		return a.Executable
	}
	return RuntimeCodex
}

func (a CodexAdapter) ParseMetrics(stdout, stderr string) RunMetrics {
	metrics := ParseMetricsFromText(stdout, stderr)
	return metrics
}
