package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
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

// RunMetrics contains best-effort usage/cost values emitted by CLI runtimes.
// Adapters should leave fields at zero when a CLI version does not expose them.
type RunMetrics struct {
	InputTokens     int64
	OutputTokens    int64
	TotalCostMicros int64
	ModelResolved   string
}

// CommentSnippet is intentionally small so runtime adapters do not depend on
// store/http model packages.
type CommentSnippet struct {
	AuthorName string
	AuthorType string
	Content    string
	CreatedAt  time.Time
}

// SkillSnippet is the prompt-facing projection of an assigned agent skill.
// Runtimes receive rendered prompt text, but the narrow shape keeps tests and
// adapters decoupled from store models.
type SkillSnippet struct {
	Name           string
	Description    string
	ActivationMode string
	Content        string
	Active         bool
	TriggerReason  string
}

// RunContext contains only the data needed to build an external CLI command.
type RunContext struct {
	RunID                  string
	WorkspaceWorkingDir    string
	AgentRuntime           string
	AgentInstructions      string
	AgentModel             string
	IssueTitle             string
	IssueBody              string
	TriggerContentSnapshot string
	Skills                 []SkillSnippet
	RecentComments         []CommentSnippet
	TimeoutSeconds         int

	// Prompt is optional. If supplied, adapters can pass it directly to the CLI.
	// Worker prompt rendering owns truncation policy; this field keeps the
	// runtime package decoupled from worker internals while allowing integration.
	Prompt string
}

// PromptText returns the pre-rendered prompt when available, otherwise a small
// fallback prompt suitable for adapter smoke tests.

func (r RunContext) RelativeRunLogPath() string {
	if r.RunID == "" || r.WorkspaceWorkingDir == "" {
		return ""
	}
	return ".cron-runs/" + r.RunID + ".log"
}

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

// MetricsParser is optional. RuntimeExecutor uses it after a process exits to
// capture token/cost metadata from stdout/stderr without making command
// construction depend on store models.
type MetricsParser interface {
	ParseMetrics(stdout, stderr string) RunMetrics
}

// StdoutFileMetricsParser is a richer alternative an adapter may opt into when
// it owns a trustworthy structured output stream (codex --json today). The
// executor passes the recorded stdout log path so the adapter can read the
// full stream instead of the stderr-only view the conservative MetricsParser
// contract enforces.
type StdoutFileMetricsParser interface {
	ParseMetricsFromFile(stdoutPath, stderrTail string) RunMetrics
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

// ErrWorkspaceWorkingDirMissing is returned when a workspace lacks the
// working_dir setting required by CLI runtimes. The codex adapter especially
// exits with the unhelpful "No such file or directory (os error 2)" otherwise.
var ErrWorkspaceWorkingDirMissing = errors.New("workspace working_dir not configured")

func command(ctx context.Context, executable string, args []string, run RunContext) (*exec.Cmd, error) {
	if executable == "" {
		return nil, errors.New("runtime executable is empty")
	}
	if strings.TrimSpace(run.WorkspaceWorkingDir) == "" {
		return nil, fmt.Errorf("%w: configure workspace working_dir before running agent (run_id=%s)", ErrWorkspaceWorkingDirMissing, run.RunID)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = run.WorkspaceWorkingDir
	return cmd, nil
}

var (
	inputTokenRE  = regexp.MustCompile(`(?i)["']?(input_tokens|prompt_tokens)["']?\s*[:=]\s*([0-9]+)`)
	outputTokenRE = regexp.MustCompile(`(?i)["']?(output_tokens|completion_tokens)["']?\s*[:=]\s*([0-9]+)`)
	costMicrosRE  = regexp.MustCompile(`(?i)["']?(total_cost_micros|cost_micros)["']?\s*[:=]\s*([0-9]+)`)
	costUSDRE     = regexp.MustCompile(`(?i)["']?(total_cost_usd|cost_usd|cost)["']?\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)`)
	modelRE       = regexp.MustCompile(`(?i)["']?(model_resolved|model)["']?\s*[:=]\s*["']([^"']+)["']`)
)

// ParseMetricsFromText extracts common usage field names used by CLI JSON logs.
// It is intentionally best-effort: unknown CLI formats simply produce zeros.
func ParseMetricsFromText(stdout, stderr string) RunMetrics {
	text := stdout + "\n" + stderr
	return RunMetrics{
		InputTokens:     firstInt64(inputTokenRE, text),
		OutputTokens:    firstInt64(outputTokenRE, text),
		TotalCostMicros: firstCostMicros(text),
		ModelResolved:   firstString(modelRE, text),
	}
}

func firstInt64(re *regexp.Regexp, text string) int64 {
	match := re.FindStringSubmatch(text)
	if len(match) < 3 {
		return 0
	}
	value, _ := strconv.ParseInt(match[2], 10, 64)
	if value < 0 {
		return 0
	}
	return value
}

func firstCostMicros(text string) int64 {
	if micros := firstInt64(costMicrosRE, text); micros > 0 {
		return micros
	}
	match := costUSDRE.FindStringSubmatch(text)
	if len(match) < 3 {
		return 0
	}
	usd, err := strconv.ParseFloat(match[2], 64)
	if err != nil || usd <= 0 {
		return 0
	}
	return int64(math.Round(usd * 1_000_000))
}

func firstString(re *regexp.Regexp, text string) string {
	match := re.FindStringSubmatch(text)
	if len(match) < 3 {
		return ""
	}
	return strings.TrimSpace(match[2])
}
