package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	doc, err := Parse(`---
name: Reddit AI Brief
description: Summarize Reddit AI discussions
triggers:
  - reddit
  - AI
---
Use concise bullet points.
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Name != "Reddit-AI-Brief" {
		t.Fatalf("name=%q", doc.Name)
	}
	if len(doc.Triggers) != 2 || doc.Triggers[0] != "reddit" || doc.Triggers[1] != "AI" {
		t.Fatalf("triggers=%#v", doc.Triggers)
	}
	if !strings.Contains(doc.Body, "concise") || doc.Hash == "" {
		t.Fatalf("bad body/hash: %#v", doc)
	}
}

func TestParseSkillMDRejectsMissingFrontmatter(t *testing.T) {
	_, err := Parse("no frontmatter")
	if !errors.Is(err, ErrInvalidSkill) {
		t.Fatalf("err=%v, want ErrInvalidSkill", err)
	}
}
