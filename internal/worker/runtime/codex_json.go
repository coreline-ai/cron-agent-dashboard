package runtime

import (
	"encoding/json"
	"strings"
)

// ParseCodexJSONL parses `codex exec --json` stdout into a clean agent message
// stream and a RunMetrics block.
//
// codex emits one JSON envelope per line. The shapes we care about:
//
//	{"type":"thread.started","thread_id":"..."}
//	{"type":"turn.started"}
//	{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"..."}}
//	{"type":"item.completed","item":{"id":"item_1","type":"tool_call","name":"write_file"}}
//	{"type":"turn.completed","usage":{"input_tokens":N,"output_tokens":N,"cached_input_tokens":N,"reasoning_output_tokens":N}}
//
// We only surface agent_message text in cleaned output — tool envelopes and
// session metadata are noise from the operator's perspective. usage is
// captured from turn.completed (the last one wins, so multi-turn streams
// reflect the final tally).
//
// parsed is true when at least one well-formed JSON envelope was found. The
// caller can use it to decide whether to fall back to the legacy text parser
// (ParseMetricsFromText / SanitizeStdout) for older codex builds or for
// non-codex CLIs that route through the same code path.
//
// codex --json today does not expose total_cost_usd, so TotalCostMicros stays
// zero. ModelResolved likewise is not in the event stream — runs that need
// the resolved model name should consult the codex session log instead.
func ParseCodexJSONL(stdout string) (cleaned string, metrics RunMetrics, parsed bool) {
	if strings.TrimSpace(stdout) == "" {
		return "", RunMetrics{}, false
	}

	var messages []string
	var foundEnvelope bool

	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !(strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) {
			continue
		}
		var env codexEnvelope
		if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
			continue
		}
		if env.Type == "" {
			continue
		}
		foundEnvelope = true
		switch env.Type {
		case "item.completed":
			if env.Item != nil && env.Item.Type == "agent_message" && env.Item.Text != "" {
				messages = append(messages, env.Item.Text)
			}
		case "turn.completed":
			if env.Usage != nil {
				if env.Usage.InputTokens != 0 {
					metrics.InputTokens = env.Usage.InputTokens
				}
				if env.Usage.OutputTokens != 0 {
					metrics.OutputTokens = env.Usage.OutputTokens
				}
			}
		}
	}

	if !foundEnvelope {
		return stdout, RunMetrics{}, false
	}
	return strings.TrimSpace(strings.Join(messages, "")), metrics, true
}

type codexEnvelope struct {
	Type  string      `json:"type"`
	Item  *codexItem  `json:"item,omitempty"`
	Usage *codexUsage `json:"usage,omitempty"`
}

type codexItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type codexUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}
