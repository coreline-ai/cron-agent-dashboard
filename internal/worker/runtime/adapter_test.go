package runtime

import (
	"context"
	"reflect"
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
	wantArgs := []string{"claude-test", "--print", "--model", "sonnet"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("args=%#v, want %#v", cmd.Args, wantArgs)
	}
	if string(stdin) != "summarize" {
		t.Fatalf("stdin=%q", string(stdin))
	}
}

func TestGeminiAdapterBuildCommand(t *testing.T) {
	run := RunContext{WorkspaceWorkingDir: "/tmp/workspace", AgentModel: "gemini-2.5-pro", Prompt: "write markdown"}
	cmd, stdin, err := GeminiAdapter{Executable: "gemini-test"}.BuildCommand(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"gemini-test", "--prompt", "write markdown", "--model", "gemini-2.5-pro"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("args=%#v, want %#v", cmd.Args, wantArgs)
	}
	if len(stdin) != 0 {
		t.Fatalf("gemini prompt should be passed by argv, got stdin=%q", string(stdin))
	}
}

func TestParseMetricsFromText(t *testing.T) {
	metrics := ParseMetricsFromText(`{"usage":{"input_tokens":1234,"output_tokens":567},"model":"gpt-5.5","total_cost_usd":0.012345}`, "")
	if metrics.InputTokens != 1234 || metrics.OutputTokens != 567 || metrics.TotalCostMicros != 12345 || metrics.ModelResolved != "gpt-5.5" {
		t.Fatalf("bad metrics: %#v", metrics)
	}
}
