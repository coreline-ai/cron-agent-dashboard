package worker

import (
	"regexp"
	"strings"
)

var mentionRE = regexp.MustCompile(`@([A-Za-z0-9_\-가-힣]+)`) // keep in sync with DATA_MODEL.md

type Mention struct {
	Raw  string
	Name string
}

func FirstMention(content string) (Mention, bool) {
	match := mentionRE.FindStringSubmatch(content)
	if len(match) < 2 {
		return Mention{}, false
	}
	return Mention{Raw: match[0], Name: match[1]}, true
}

func Mentions(content string) []Mention {
	matches := mentionRE.FindAllStringSubmatch(content, -1)
	out := make([]Mention, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			out = append(out, Mention{Raw: match[0], Name: match[1]})
		}
	}
	return out
}

// NormalizeMentionName is the case-insensitive contract used by store queries
// and API handlers: compare lower(name) within a workspace.
func NormalizeMentionName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "@")))
}

func MentionNameEqual(a, b string) bool {
	return NormalizeMentionName(a) == NormalizeMentionName(b)
}
