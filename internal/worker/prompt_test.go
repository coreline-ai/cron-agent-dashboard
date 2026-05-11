package worker

import (
	"strings"
	"testing"
)

func TestRenderPromptTruncatesRecentContext(t *testing.T) {
	prompt := RenderPrompt(PromptInput{
		Instructions: "follow instructions",
		IssueTitle:   "write report",
		IssueBody:    "body stays intact",
		ContextCap:   40,
		RecentComments: []CommentSnippet{
			{AuthorName: "A", Content: strings.Repeat("x", 100)},
			{AuthorName: "B", Content: "second"},
			{AuthorName: "C", Content: "third"},
			{AuthorName: "D", Content: "must not appear"},
		},
	})
	if !strings.Contains(prompt, "follow instructions") || !strings.Contains(prompt, "write report") || !strings.Contains(prompt, "body stays intact") {
		t.Fatalf("prompt missing required sections: %q", prompt)
	}
	if strings.Contains(prompt, "must not appear") {
		t.Fatalf("rendered more than 3 comments: %q", prompt)
	}
	contextPart := prompt[strings.Index(prompt, "# 최근 컨텍스트"):]
	if !strings.Contains(contextPart, truncatedMarker) {
		t.Fatalf("expected truncation marker in %q", contextPart)
	}
	if got := len([]rune(strings.TrimPrefix(contextPart, "# 최근 컨텍스트\n"))); got > 40 {
		t.Fatalf("context cap exceeded: got %d", got)
	}
}
