package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCodexAdapterBuildCommand(t *testing.T) {
	run := RunContext{
		WorkspaceWorkingDir: "/tmp/workspace",
		AgentModel:          "gpt-5.4",
		Prompt:              "do the task",
	}
	cmd, stdin, err := CodexAdapter{Executable: "codex-test"}.BuildCommand(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"codex-test", "exec", "--model", "gpt-5.4", "--cd", "/tmp/workspace", "-"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("args=%#v, want %#v", cmd.Args, wantArgs)
	}
	if cmd.Dir != "/tmp/workspace" {
		t.Fatalf("Dir=%q, want workspace", cmd.Dir)
	}
	if string(stdin) != "do the task" {
		t.Fatalf("stdin=%q", string(stdin))
	}
}

func TestClaudeAdapterBuildCommand(t *testing.T) {
	run := RunContext{WorkspaceWorkingDir: "/tmp/workspace", AgentModel: "sonnet", Prompt: "summarize"}
	cmd, stdin, err := ClaudeAdapter{Executable: "claude-test"}.BuildCommand(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	// --input-format text pins the "stdin carries a plain text prompt"
	// contract so claude --print does not fall back into interactive mode
	// (which caused 10-minute hangs in dev-plan/implement_20260520_230031.md).
	wantArgs := []string{"claude-test", "--print", "--input-format", "text", "--model", "sonnet"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("args=%#v, want %#v", cmd.Args, wantArgs)
	}
	if string(stdin) != "summarize" {
		t.Fatalf("stdin=%q", string(stdin))
	}
}

func TestGeminiAdapterBuildCommandPassesPromptViaStdin(t *testing.T) {
	const promptBody = "write markdown — sensitive!@# prompt body"
	run := RunContext{WorkspaceWorkingDir: "/tmp/workspace", AgentModel: "gemini-2.5-pro", Prompt: promptBody}
	cmd, stdin, err := GeminiAdapter{Executable: "gemini-test"}.BuildCommand(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	// argv must not leak the prompt body. (`gemini -p ""` triggers headless
	// mode and the empty prompt is appended to the body coming from stdin.)
	for _, arg := range cmd.Args {
		if strings.Contains(arg, promptBody) {
			t.Fatalf("prompt body leaked into argv: %#v", cmd.Args)
		}
	}
	// Headless flag must be present so gemini does not drop into interactive mode.
	foundHeadless := false
	for _, arg := range cmd.Args {
		if arg == "-p" || arg == "--prompt" {
			foundHeadless = true
			break
		}
	}
	if !foundHeadless {
		t.Fatalf("expected -p/--prompt flag in args=%#v", cmd.Args)
	}
	// Model flag still appears.
	wantTail := []string{"--model", "gemini-2.5-pro"}
	if !containsSubseq(cmd.Args, wantTail) {
		t.Fatalf("model args missing from %#v", cmd.Args)
	}
	if string(stdin) != promptBody {
		t.Fatalf("stdin=%q, want %q", string(stdin), promptBody)
	}
	if cmd.Dir != "/tmp/workspace" {
		t.Fatalf("Dir=%q, want workspace", cmd.Dir)
	}
}

func containsSubseq(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if reflect.DeepEqual(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func TestParseMetricsFromText(t *testing.T) {
	metrics := ParseMetricsFromText(`{"usage":{"input_tokens":1234,"output_tokens":567},"model":"gpt-5.5","total_cost_usd":0.012345}`, "")
	if metrics.InputTokens != 1234 || metrics.OutputTokens != 567 || metrics.TotalCostMicros != 12345 || metrics.ModelResolved != "gpt-5.5" {
		t.Fatalf("bad metrics: %#v", metrics)
	}
}
