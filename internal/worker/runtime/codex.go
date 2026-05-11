package runtime

import (
	"context"
	"os/exec"
)

// CodexAdapter is a thin command builder for the codex CLI. The exact CLI flags
// are intentionally conservative; integration tests can assert the command and
// stdin without spawning a real process.
type CodexAdapter struct {
	Executable string
}

func (a CodexAdapter) Name() string { return RuntimeCodex }

func (a CodexAdapter) Detect(ctx context.Context) RuntimeInfo {
	return detectExecutable(ctx, RuntimeCodex, a.executable(), "--version")
}

func (a CodexAdapter) BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error) {
	args := []string{}
	if run.AgentModel != "" {
		args = append(args, "--model", run.AgentModel)
	}
	return commandWithPrompt(ctx, a.executable(), args, run)
}

func (a CodexAdapter) executable() string {
	if a.Executable != "" {
		return a.Executable
	}
	return RuntimeCodex
}
