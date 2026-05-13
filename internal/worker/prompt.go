package worker

import (
	"fmt"
	"strings"
	"time"

	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

const (
	DefaultPromptContextCap = 4000
	DefaultRecentComments   = 3
	truncatedMarker         = "...[truncated]"
)

// CommentSnippet is the small prompt-facing projection of a comment row.
type CommentSnippet = workerruntime.CommentSnippet

type PromptInput struct {
	Instructions   string
	IssueTitle     string
	IssueBody      string
	RecentComments []CommentSnippet // newest-first; only the first 3 are rendered
	ContextCap     int
}

func RenderPrompt(input PromptInput) string {
	capChars := input.ContextCap
	if capChars <= 0 {
		capChars = DefaultPromptContextCap
	}

	var b strings.Builder
	b.WriteString(strings.TrimSpace(input.Instructions))
	b.WriteString("\n\n# 작업\n")
	b.WriteString(strings.TrimSpace(input.IssueTitle))
	if strings.TrimSpace(input.IssueBody) != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(input.IssueBody))
	}
	b.WriteString("\n\n# 최근 컨텍스트\n")

	contextText := renderRecentComments(input.RecentComments, DefaultRecentComments)
	if contextText == "" {
		contextText = "(최근 댓글 없음)"
	}
	b.WriteString(truncateRunes(contextText, capChars, truncatedMarker))
	return b.String()
}

func renderRecentComments(comments []CommentSnippet, max int) string {
	if max <= 0 || len(comments) == 0 {
		return ""
	}

	var b strings.Builder
	for i, comment := range comments {
		if i >= max {
			break
		}
		author := strings.TrimSpace(comment.AuthorName)
		if author == "" {
			author = strings.TrimSpace(comment.AuthorType)
		}
		if author == "" {
			author = "unknown"
		}
		content := strings.TrimSpace(comment.Content)
		if comment.CreatedAt.IsZero() {
			fmt.Fprintf(&b, "- %s: %s", author, content)
		} else {
			fmt.Fprintf(&b, "- %s (%s): %s", author, comment.CreatedAt.Format(time.RFC3339), content)
		}
		if i < len(comments)-1 && i < max-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func truncateRunes(s string, max int, marker string) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	markerRunes := []rune(marker)
	keep := max - len(markerRunes)
	if keep < 0 {
		keep = 0
	}
	return string(runes[:keep]) + marker
}
