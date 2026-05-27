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

func TestFirstMentionSupportsUnicodeLetters(t *testing.T) {
	for _, input := range []string{"@ライター 확인", "@撰写 정리", "@người_viết 초안"} {
		if _, ok := FirstMention(input); !ok {
			t.Fatalf("expected unicode mention in %q", input)
		}
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

func TestFirstMentionSkipsEmailAddress(t *testing.T) {
	// The previous regex matched the literal `@local` inside the email
	// `devteam-admin@local`, sending the auto-chain dispatcher off looking
	// for an agent named `local`. With the boundary requirement, the email
	// is ignored and the explicit `@Backend` mention at the end wins.
	body := "Created admin seed devteam-admin@local with bcrypt hash.\n\n@Backend"
	m, ok := FirstMention(body)
	if !ok {
		t.Fatalf("expected mention, got none")
	}
	if m.Name != "Backend" {
		t.Fatalf("expected @Backend, got %q", m.Name)
	}
}

func TestFirstMentionIgnoresInlineCode(t *testing.T) {
	body := "Seed: `devteam-admin@local` and `user@host`. Now @Backend takes over."
	m, ok := FirstMention(body)
	if !ok {
		t.Fatalf("expected mention, got none")
	}
	if m.Name != "Backend" {
		t.Fatalf("expected @Backend, got %q", m.Name)
	}
}

func TestFirstMentionIgnoresFencedCodeBlock(t *testing.T) {
	body := "```\ndevteam-admin@local\n```\n\n@QA verify.\n"
	m, ok := FirstMention(body)
	if !ok {
		t.Fatalf("expected mention, got none")
	}
	if m.Name != "QA" {
		t.Fatalf("expected @QA, got %q", m.Name)
	}
}

func TestFirstMentionIgnoresMaskedEmail(t *testing.T) {
	// Designer wrote `u**@example.com` as a privacy-masked example. The
	// previous regex (`[^\p{L}\p{N}_\-]@`) matched on `*` and dispatched
	// "@example" — which doesn't exist as an agent. Requiring whitespace
	// or start-of-line before `@` blocks this class of false positive.
	body := "이메일은 `u**@example.com` 형식으로 마스킹합니다.\n\n@DB 다음 단계 부탁."
	m, ok := FirstMention(body)
	if !ok {
		t.Fatalf("expected mention, got none")
	}
	if m.Name != "DB" {
		t.Fatalf("expected @DB, got %q", m.Name)
	}
}
