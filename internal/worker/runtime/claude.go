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
	// claude --print alone does not advertise that stdin will carry the
	// prompt, and the CLI drops back into interactive mode when the input
	// format is ambiguous — that produced the 10-minute timeouts on RFP-2
	// when the prompt body was piped through stdin (see dev-plan/
	// implement_20260520_230031.md). `--input-format text` makes the
	// "stdin holds a text prompt" contract explicit so the CLI exits
	// non-interactively after consuming EOF.
	args := []string{"--print", "--input-format", "text"}
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
