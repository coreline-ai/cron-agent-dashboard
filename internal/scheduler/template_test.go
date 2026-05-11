package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTemplate(t *testing.T) {
	now := time.Date(2026, 5, 11, 9, 8, 7, 0, time.UTC)
	got, err := RenderTemplate("daily {{date}} {{datetime}} {{time}}", now)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}
	want := "daily 2026-05-11 2026-05-11 09:08 09:08"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderTemplateUnknownVar(t *testing.T) {
	_, err := RenderTemplate("hello {{workspace}}", time.Now())
	if err == nil {
		t.Fatal("expected unknown variable error")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error should mention unknown var: %v", err)
	}
}
