package runtime

import (
	"reflect"
	"testing"
)

func TestSanitizeStdoutCodexStripsKnownNoise(t *testing.T) {
	input := "MCP issues detected. Run /mcp list for status.\n\n# Result heading\n\nbody line"
	cleaned, stripped := SanitizeStdout(RuntimeCodex, input)
	want := "# Result heading\n\nbody line"
	if cleaned != want {
		t.Fatalf("cleaned mismatch:\n  got=%q\n want=%q", cleaned, want)
	}
	if !reflect.DeepEqual(stripped, []string{"MCP issues detected. Run /mcp list for status."}) {
		t.Fatalf("stripped mismatch: %#v", stripped)
	}
}

func TestSanitizeStdoutCodexLeavesNonNoiseAlone(t *testing.T) {
	input := "# Result\n\nbody"
	cleaned, stripped := SanitizeStdout(RuntimeCodex, input)
	if cleaned != input {
		t.Fatalf("non-noise altered: got=%q", cleaned)
	}
	if len(stripped) != 0 {
		t.Fatalf("expected no strip, got=%#v", stripped)
	}
}

func TestSanitizeStdoutStripsMCPNoiseForAllRuntimes(t *testing.T) {
	// MCP noise can be emitted by any of codex / claude / gemini when MCP
	// servers are configured. The dispatcher strips it irrespective of which
	// runtime produced the stdout.
	for _, runtime := range []string{RuntimeCodex, RuntimeClaude, RuntimeGemini} {
		input := "MCP issues detected. Run /mcp list for status.\n\nbody"
		cleaned, stripped := SanitizeStdout(runtime, input)
		if cleaned != "body" {
			t.Fatalf("%s: expected stripped output 'body', got=%q", runtime, cleaned)
		}
		if len(stripped) != 1 {
			t.Fatalf("%s: expected 1 stripped line, got=%#v", runtime, stripped)
		}
	}
}

func TestSanitizeStdoutHandlesEmpty(t *testing.T) {
	cleaned, stripped := SanitizeStdout(RuntimeCodex, "")
	if cleaned != "" || len(stripped) != 0 {
		t.Fatalf("empty input mishandled: cleaned=%q stripped=%#v", cleaned, stripped)
	}
}

func TestSanitizeStdoutCodexStripsPrefixJoinedNoise(t *testing.T) {
	// Real-world codex output: the noise line runs into the first paragraph of
	// real output without a newline.
	input := "MCP issues detected. Run /mcp list for status.반갑습니다. RFP 협업 스튜디오의 영업 컨설턴트입니다."
	cleaned, stripped := SanitizeStdout(RuntimeCodex, input)
	want := "반갑습니다. RFP 협업 스튜디오의 영업 컨설턴트입니다."
	if cleaned != want {
		t.Fatalf("prefix-joined strip failed:\n  got=%q\n want=%q", cleaned, want)
	}
	if !reflect.DeepEqual(stripped, []string{"MCP issues detected. Run /mcp list for status."}) {
		t.Fatalf("stripped mismatch: %#v", stripped)
	}
}

func TestSanitizeStdoutCodexStripsRepeatedNoiseRuns(t *testing.T) {
	input := "MCP issues detected. Run /mcp list for status.\nMCP issues detected. Run /mcp list for status.\nbody"
	cleaned, stripped := SanitizeStdout(RuntimeCodex, input)
	if cleaned != "body" {
		t.Fatalf("repeated strip failed: got=%q", cleaned)
	}
	if len(stripped) != 2 {
		t.Fatalf("expected 2 strips, got=%#v", stripped)
	}
}

func TestSanitizeStdoutCodexLeavesMidContentMentionsAlone(t *testing.T) {
	// Noise pattern appearing mid-output (not as a leading prefix) must stay —
	// it could be legitimate quoted content from the agent.
	input := "real result here\nMCP issues detected. Run /mcp list for status.\ntrailing line"
	cleaned, stripped := SanitizeStdout(RuntimeCodex, input)
	if cleaned != input {
		t.Fatalf("mid-content match incorrectly stripped:\n  got=%q\n want=%q", cleaned, input)
	}
	if len(stripped) != 0 {
		t.Fatalf("expected no strip for mid-content, got=%#v", stripped)
	}
}
