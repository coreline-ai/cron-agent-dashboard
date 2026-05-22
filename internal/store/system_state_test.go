package store

import (
	"context"
	"testing"
	"time"
)

func TestSystemStateGetMissingReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	got, err := st.GetSystemState(ctx, "never-set")
	if err != nil {
		t.Fatalf("expected no error for missing key, got %v", err)
	}
	if got != "" {
		t.Fatalf("missing key should return empty string, got %q", got)
	}
}

func TestSystemStateUpsertRefreshesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.SetSystemState(ctx, "k", "v1"); err != nil {
		t.Fatal(err)
	}
	first, err := st.GetSystemStateEntry(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if first.Value != "v1" {
		t.Fatalf("first value=%q want v1", first.Value)
	}
	// Force a small wait so the second updated_at can differ at second resolution.
	time.Sleep(1100 * time.Millisecond)
	if err := st.SetSystemState(ctx, "k", "v2"); err != nil {
		t.Fatal(err)
	}
	second, err := st.GetSystemStateEntry(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if second.Value != "v2" {
		t.Fatalf("second value=%q want v2", second.Value)
	}
	if second.UpdatedAt == first.UpdatedAt {
		t.Fatalf("updated_at did not change on upsert: %q == %q", second.UpdatedAt, first.UpdatedAt)
	}
}

func TestSystemStateEmptyKeyRejected(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.SetSystemState(ctx, "", "v"); err == nil {
		t.Fatalf("empty key should return ErrValidation")
	}
}
