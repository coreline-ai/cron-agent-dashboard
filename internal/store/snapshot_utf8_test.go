package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapSnapshotPreservesUTF8WhenTruncating(t *testing.T) {
	// 1500 Korean characters (3 bytes each) = 4500 bytes — crosses the 4000-byte cap mid-rune.
	input := strings.Repeat("한", 1500)
	if !utf8.ValidString(input) {
		t.Fatalf("test input itself is not valid UTF-8")
	}
	got := capSnapshot(input)
	if len(got) > 4000 {
		t.Fatalf("capSnapshot exceeded cap: len=%d", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("capSnapshot produced invalid UTF-8 at truncation boundary: tail=%x", tailBytes(got, 6))
	}
}

func TestCapSnapshotNormalizesInvalidInput(t *testing.T) {
	input := "ok-prefix-" + string([]byte{0xc3, 0x28}) + "-suffix"
	got := capSnapshot(input)
	if !utf8.ValidString(got) {
		t.Fatalf("capSnapshot did not normalize invalid input: out=%q tail=%x", got, tailBytes(got, 6))
	}
}

func tailBytes(s string, n int) []byte {
	if len(s) <= n {
		return []byte(s)
	}
	return []byte(s[len(s)-n:])
}
