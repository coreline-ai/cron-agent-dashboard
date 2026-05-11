package runtime

import (
	"context"
	"os/exec"
)

// GeminiAdapter builds commands for the gemini CLI using stdin for the prompt.
type GeminiAdapter struct {
	Executable string
}

func (a GeminiAdapter) Name() string { return RuntimeGemini }

func (a GeminiAdapter) Detect(ctx context.Context) RuntimeInfo {
	return detectExecutable(ctx, RuntimeGemini, a.executable(), "--version")
}

func (a GeminiAdapter) BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error) {
	args := []string{}
	if run.AgentModel != "" {
		args = append(args, "--model", run.AgentModel)
	}
	return commandWithPrompt(ctx, a.executable(), args, run)
}

func (a GeminiAdapter) executable() string {
	if a.Executable != "" {
		return a.Executable
	}
	return RuntimeGemini
}
