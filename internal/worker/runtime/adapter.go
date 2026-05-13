package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runtime names supported by the dashboard MVP.
const (
	RuntimeCodex  = "codex"
	RuntimeClaude = "claude"
	RuntimeGemini = "gemini"
)

// RuntimeInfo is the result of probing a CLI runtime on PATH.
type RuntimeInfo struct {
	Name      string
	Path      string
	Version   string
	Available bool
	Error     string
}

// CommentSnippet is intentionally small so runtime adapters do not depend on
// store/http model packages.
type CommentSnippet struct {
	AuthorName string
	AuthorType string
	Content    string
	CreatedAt  time.Time
}

// RunContext contains only the data needed to build an external CLI command.
type RunContext struct {
	RunID               string
	WorkspaceWorkingDir string
	AgentRuntime        string
	AgentInstructions   string
	AgentModel          string
	IssueTitle          string
	IssueBody           string
	RecentComments      []CommentSnippet

	// Prompt is optional. If supplied, adapters can pass it directly to the CLI.
	// Worker prompt rendering owns truncation policy; this field keeps the
	// runtime package decoupled from worker internals while allowing integration.
	Prompt string
}

// PromptText returns the pre-rendered prompt when available, otherwise a small
// fallback prompt suitable for adapter smoke tests.
func (r RunContext) PromptText() string {
	if strings.TrimSpace(r.Prompt) != "" {
		return r.Prompt
	}

	var b strings.Builder
	b.WriteString(r.AgentInstructions)
	b.WriteString("\n\n# 작업\n")
	b.WriteString(r.IssueTitle)
	if r.IssueBody != "" {
		b.WriteString("\n\n")
		b.WriteString(r.IssueBody)
	}
	if len(r.RecentComments) > 0 {
		b.WriteString("\n\n# 최근 컨텍스트\n")
		for i, c := range r.RecentComments {
			if i >= 3 {
				break
			}
			name := c.AuthorName
			if name == "" {
				name = c.AuthorType
			}
			fmt.Fprintf(&b, "- %s: %s\n", name, c.Content)
		}
	}
	return b.String()
}

// RuntimeAdapter isolates CLI-specific command construction and detection.
type RuntimeAdapter interface {
	Name() string
	Detect(ctx context.Context) RuntimeInfo
	BuildCommand(ctx context.Context, run RunContext) (*exec.Cmd, []byte, error)
}

// DefaultAdapters returns the built-in CLI adapters.
func DefaultAdapters() []RuntimeAdapter {
	return []RuntimeAdapter{CodexAdapter{}, ClaudeAdapter{}, GeminiAdapter{}}
}

// Detect probes the given adapters, or the built-in adapters if none are given.
func Detect(ctx context.Context, adapters ...RuntimeAdapter) map[string]RuntimeInfo {
	if len(adapters) == 0 {
		adapters = DefaultAdapters()
	}
	out := make(map[string]RuntimeInfo, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		out[adapter.Name()] = adapter.Detect(ctx)
	}
	return out
}

func detectExecutable(ctx context.Context, runtimeName, executable string, versionArgs ...string) RuntimeInfo {
	path, err := exec.LookPath(executable)
	if err != nil {
		return RuntimeInfo{Name: runtimeName, Available: false, Error: err.Error()}
	}

	info := RuntimeInfo{Name: runtimeName, Path: path, Available: true}
	if len(versionArgs) > 0 {
		cmd := exec.CommandContext(ctx, path, versionArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			info.Error = err.Error()
		} else {
			version := strings.TrimSpace(stdout.String())
			if version == "" {
				version = strings.TrimSpace(stderr.String())
			}
			info.Version = version
		}
	}
	return info
}

func commandWithPrompt(ctx context.Context, executable string, args []string, run RunContext) (*exec.Cmd, []byte, error) {
	cmd, err := command(ctx, executable, args, run)
	if err != nil {
		return nil, nil, err
	}
	return cmd, []byte(run.PromptText()), nil
}

func commandWithoutStdin(ctx context.Context, executable string, args []string, run RunContext) (*exec.Cmd, []byte, error) {
	cmd, err := command(ctx, executable, args, run)
	if err != nil {
		return nil, nil, err
	}
	return cmd, nil, nil
}

func command(ctx context.Context, executable string, args []string, run RunContext) (*exec.Cmd, error) {
	if executable == "" {
		return nil, errors.New("runtime executable is empty")
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	if run.WorkspaceWorkingDir != "" {
		cmd.Dir = run.WorkspaceWorkingDir
	}
	return cmd, nil
}
