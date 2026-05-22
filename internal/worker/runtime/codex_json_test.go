package runtime

import (
	"os"
	"strings"
	"testing"
)

func TestParseCodexJSONL_FixtureRoundtrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/codex_exec_jsonl_sample.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cleaned, metrics, parsed := ParseCodexJSONL(string(raw))
	if !parsed {
		t.Fatalf("expected parsed=true for JSONL fixture")
	}
	if !strings.Contains(cleaned, "안녕! 진행 상황을 정리해두었어.") {
		t.Fatalf("cleaned missing first agent_message: %q", cleaned)
	}
	if !strings.Contains(cleaned, "다음 단계는 @QA 에게 넘기겠다.") {
		t.Fatalf("cleaned missing second agent_message: %q", cleaned)
	}
	if strings.Contains(cleaned, "tool_call") {
		t.Fatalf("cleaned must not include tool_call envelope: %q", cleaned)
	}
	if strings.Contains(cleaned, "not-json-noise-line") {
		t.Fatalf("cleaned must drop non-JSON noise: %q", cleaned)
	}
	if strings.Contains(cleaned, "thread_id") {
		t.Fatalf("cleaned must not include raw JSONL envelopes: %q", cleaned)
	}
	if metrics.InputTokens != 15929 {
		t.Fatalf("InputTokens=%d, want 15929", metrics.InputTokens)
	}
	if metrics.OutputTokens != 42 {
		t.Fatalf("OutputTokens=%d, want 42", metrics.OutputTokens)
	}
	// codex --json does not expose total_cost_usd today; expect zero.
	if metrics.TotalCostMicros != 0 {
		t.Fatalf("TotalCostMicros=%d, want 0 (codex JSONL has no cost)", metrics.TotalCostMicros)
	}
}

func TestParseCodexJSONL_EmptyReturnsFalse(t *testing.T) {
	cleaned, metrics, parsed := ParseCodexJSONL("")
	if parsed {
		t.Fatalf("empty input should not be classed as parsed JSONL")
	}
	if cleaned != "" || metrics.InputTokens != 0 {
		t.Fatalf("expected zero values, got cleaned=%q metrics=%+v", cleaned, metrics)
	}
}

func TestParseCodexJSONL_AllNonJSONFallback(t *testing.T) {
	const legacy = "human readable codex output without --json\nsecond line\n"
	cleaned, _, parsed := ParseCodexJSONL(legacy)
	if parsed {
		t.Fatalf("expected parsed=false when no JSONL envelope present")
	}
	if cleaned != legacy {
		t.Fatalf("expected legacy fallback to return input verbatim, got %q", cleaned)
	}
}

func TestParseCodexJSONL_StripsLeadingMCPNoise(t *testing.T) {
	// MCP diagnostic precedes the JSONL when MCP servers are configured.
	// SanitizeStdout drops it for the legacy text path; ParseCodexJSONL
	// should also be resilient when noise sits before JSONL events.
	const stream = "MCP issues detected. Run /mcp list for status.\n" +
		`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"done"}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}` + "\n"
	cleaned, metrics, parsed := ParseCodexJSONL(stream)
	if !parsed {
		t.Fatalf("expected parsed=true when JSONL appears after noise prefix")
	}
	if strings.TrimSpace(cleaned) != "done" {
		t.Fatalf("cleaned=%q, want \"done\"", cleaned)
	}
	if metrics.InputTokens != 1 || metrics.OutputTokens != 2 {
		t.Fatalf("metrics=%+v", metrics)
	}
}
