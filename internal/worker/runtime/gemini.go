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
	// gemini -p switches the CLI to headless mode and (per `gemini --help`)
	// the argv prompt is appended to whatever arrives on stdin. By passing an
	// empty argv prompt and routing the actual body through stdin we keep the
	// prompt text out of /proc/<pid>/cmdline so other local users (or shell
	// history captures) cannot observe it.
	args := []string{"-p", ""}
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

func (a GeminiAdapter) ParseMetrics(stdout, stderr string) RunMetrics {
	metrics := ParseMetricsFromText(stdout, stderr)
	return metrics
}
