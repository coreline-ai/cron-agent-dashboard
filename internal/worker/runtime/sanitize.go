package runtime

import (
	"strings"
)

// knownNoiseLines are stdout fragments that runtime CLIs emit as diagnostics
// but that should not appear in agent result comments or chain prompts.
// Matching is exact on a trimmed leading segment and applied repeatedly so
// noise that is prefix-joined to real output (no trailing newline) is removed.
//
// The MCP diagnostic message can surface from any of codex / claude / gemini
// whenever MCP servers are configured, so we treat known noise as
// runtime-agnostic. runtimeName is kept on the dispatcher for future
// per-runtime additions.
var knownNoiseLines = map[string]struct{}{
	"MCP issues detected. Run /mcp list for status.": {},
}

// SanitizeStdout removes leading runtime CLI diagnostic noise from stdout.
// cleaned is the stdout with known noise lines stripped from the leading
// edge; stripped lists the removed lines in original order for observability.
func SanitizeStdout(runtimeName, stdout string) (cleaned string, stripped []string) {
	_ = runtimeName // reserved for future per-runtime rules
	if stdout == "" {
		return "", nil
	}
	current := stdout
	var hit []string
	for {
		trimmed := strings.TrimLeft(current, " \t\r\n")
		matched := false
		for pattern := range knownNoiseLines {
			if strings.HasPrefix(trimmed, pattern) {
				hit = append(hit, pattern)
				current = trimmed[len(pattern):]
				matched = true
				break
			}
		}
		if !matched {
			current = trimmed
			break
		}
	}
	if len(hit) == 0 {
		return stdout, nil
	}
	return current, hit
}
