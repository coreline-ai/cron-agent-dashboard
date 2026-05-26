package runtime

import (
	"context"
	"os"
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
	// --json switches codex exec to one-JSON-event-per-line output. This is the
	// structured metrics stream ParseCodexJSONL relies on; without it, codex
	// stdout interleaves human-readable text with the agent message and the
	// dashboard would have to keep post-stripping MCP diagnostics by regex.
	//
	// --sandbox workspace-write lets the agent create and edit files inside
	// --cd (the worktree or workspace working_dir). The codex default is
	// read-only, which silently refuses apply_patch with "writing is blocked
	// by read-only sandbox" — that surfaces in the dev-team chain as
	// BUILD-FAIL Designer/Frontend runs that produce no files. The worker
	// already scopes the cwd to a workspace-owned directory, so giving codex
	// write access to that directory only is the minimal change that lets
	// hub-PM chains actually deliver artifacts.
	args := []string{"exec", "--json", "--sandbox", "workspace-write"}
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
	if _, metrics, ok := ParseCodexJSONL(stdout); ok {
		return metrics
	}
	return ParseMetricsFromText(stdout, stderr)
}

// ParseMetricsFromFile reads the recorded stdout log so the JSONL parser sees
// the full stream rather than the stderr-only view the conservative
// MetricsParser interface enforces. It is the StdoutFileMetricsParser opt-in
// described in adapter.go.
func (a CodexAdapter) ParseMetricsFromFile(stdoutPath, stderrTail string) RunMetrics {
	if stdoutPath == "" {
		return ParseMetricsFromText("", stderrTail)
	}
	data, err := os.ReadFile(stdoutPath)
	if err != nil || len(data) == 0 {
		return ParseMetricsFromText("", stderrTail)
	}
	if _, metrics, ok := ParseCodexJSONL(string(data)); ok {
		return metrics
	}
	return ParseMetricsFromText(string(data), stderrTail)
}
