package worker

import (
	"regexp"
	"strings"
)

// mentionRE captures @AgentName tokens. The leading boundary (`^` or any
// whitespace) is critical: without it, an email like `user@local` matches
// `@local`, and a masked email like `u**@example.com` (used in privacy
// blurbs) matches `@example`. We *only* accept start-of-line / whitespace
// before `@`, which kills both classes of false positive. Workers are
// instructed to put the mention on its own line, so this is a safe contract
// in practice. Korean/Hangul (\p{L}) and digits remain valid in agent names.
// Keep in sync with DATA_MODEL.md.
var mentionRE = regexp.MustCompile(`(^|\s)@([\p{L}\p{N}_\-]+)`)

type Mention struct {
	Raw  string
	Name string
}

// FirstMention skips matches inside fenced code blocks (```...```) and inline
// code (`...`) so that worker comments can reference identifiers like
// `admin@local` without confusing the auto-chain dispatcher.
func FirstMention(content string) (Mention, bool) {
	stripped := stripCodeFences(content)
	loc := mentionRE.FindStringSubmatchIndex(stripped)
	if loc == nil {
		return Mention{}, false
	}
	name := stripped[loc[4]:loc[5]]
	raw := "@" + name
	return Mention{Raw: raw, Name: name}, true
}

func Mentions(content string) []Mention {
	stripped := stripCodeFences(content)
	matches := mentionRE.FindAllStringSubmatch(stripped, -1)
	out := make([]Mention, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 3 {
			out = append(out, Mention{Raw: "@" + match[2], Name: match[2]})
		}
	}
	return out
}

// stripCodeFences blanks out the contents of triple-backtick code blocks and
// inline `code` spans so that emails or @handles inside them never trip the
// mention dispatcher. The output preserves byte offsets (spaces replace the
// hidden runes) so regex match positions remain meaningful.
func stripCodeFences(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	i := 0
	for i < len(content) {
		if strings.HasPrefix(content[i:], "```") {
			b.WriteString("```")
			end := strings.Index(content[i+3:], "```")
			if end < 0 {
				b.WriteString(strings.Repeat(" ", len(content)-i-3))
				return b.String()
			}
			b.WriteString(strings.Repeat(" ", end))
			b.WriteString("```")
			i += 3 + end + 3
			continue
		}
		if content[i] == '`' {
			b.WriteByte('`')
			end := strings.IndexByte(content[i+1:], '`')
			if end < 0 {
				b.WriteString(content[i+1:])
				return b.String()
			}
			b.WriteString(strings.Repeat(" ", end))
			b.WriteByte('`')
			i += 1 + end + 1
			continue
		}
		b.WriteByte(content[i])
		i++
	}
	return b.String()
}

// NormalizeMentionName is the case-insensitive contract used by store queries
// and API handlers: compare lower(name) within a workspace.
func NormalizeMentionName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "@")))
}

func MentionNameEqual(a, b string) bool {
	return NormalizeMentionName(a) == NormalizeMentionName(b)
}
