package worker

import "testing"

func TestFirstMention(t *testing.T) {
	mention, ok := FirstMention("please ask @Writer and @Other")
	if !ok {
		t.Fatal("expected mention")
	}
	if mention.Raw != "@Writer" || mention.Name != "Writer" {
		t.Fatalf("unexpected mention: %#v", mention)
	}
}

func TestMentionNameEqualCaseInsensitive(t *testing.T) {
	if !MentionNameEqual("@Writer", "writer") {
		t.Fatal("expected case-insensitive match")
	}
	if !MentionNameEqual("뉴스_봇", "@뉴스_봇") {
		t.Fatal("expected Hangul mention match")
	}
}
