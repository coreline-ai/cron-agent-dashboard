package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// Track D of dev-plan/implement_20260522_220446.md.
//
// The dispatcher must keep a delivery in 'pending' state for the first
// maxAttempts-1 failures and only flip to 'failed' (dead-letter) when the
// final attempt also fails. With the default maxAttempts=6 plus a fast
// backoff vector this means five rescheduled rows in 'pending' followed by
// a single 'failed' row.
func TestWebhookDispatcherExpBackoffEventuallyDeadLetters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	database, err := db.OpenAndMigrate(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	ctx := context.Background()
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "WHF",
		Slug:             "wh-failure",
		IdentifierPrefix: "WHF",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, err := st.CreateWebhook(ctx, ws.ID, store.UpsertWebhookInput{
		URL:    srv.URL,
		Events: []string{"issue.cancelled"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use 1ns backoffs so the dispatcher's tick loop can pull each retry
	// immediately. The schedule has 5 elements so attempts 2..6 are
	// scheduled by it; the 6th failure should mark the row terminal.
	tinyBackoff := time.Nanosecond
	d := NewWebhookDispatcher(st,
		WithWebhookHTTPClient(srv.Client()),
		WithWebhookRetryBackoffs([]time.Duration{tinyBackoff, tinyBackoff, tinyBackoff, tinyBackoff, tinyBackoff}),
		WithWebhookMaxAttempts(6),
	)

	// Enqueue a delivery by canceling an issue.
	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "trigger"})
	status := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, store.UpdateIssueInput{Status: &status}); err != nil {
		t.Fatal(err)
	}

	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(deliveries) != 1 || deliveries[0].Status != "pending" {
		t.Fatalf("setup: expected 1 pending delivery, got %#v", deliveries)
	}

	// Each tick attempts once and reschedules with the tiny backoff. We need
	// 6 ticks to exhaust the attempt budget.
	for i := 0; i < 6; i++ {
		if err := d.TickOnce(ctx); err != nil {
			t.Fatalf("tick #%d: %v", i, err)
		}
	}

	final, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(final) != 1 {
		t.Fatalf("expected 1 delivery row after exhaustion, got %d", len(final))
	}
	if final[0].Status != "failed" {
		t.Fatalf("delivery status=%q want failed (dead-letter) after %d attempts", final[0].Status, final[0].Attempt)
	}
	if final[0].Attempt != 6 {
		t.Fatalf("attempt count=%d want 6", final[0].Attempt)
	}

	dead, err := st.CountWebhookDeliveryFailed(ctx, hook.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dead != 1 {
		t.Fatalf("CountWebhookDeliveryFailed=%d want 1", dead)
	}
}

// Verify the per-attempt backoff vector picks the right element for each
// failure index, including the clamp when failures exceed the slice length.
func TestWebhookDispatcherBackoffSchedule(t *testing.T) {
	schedule := []time.Duration{30 * time.Second, 2 * time.Minute, 8 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	d := NewWebhookDispatcher(nil, WithWebhookRetryBackoffs(schedule), WithWebhookMaxAttempts(6))
	cases := []struct {
		failed int
		want   time.Duration
	}{
		{0, 30 * time.Second},
		{1, 2 * time.Minute},
		{2, 8 * time.Minute},
		{3, 30 * time.Minute},
		{4, 2 * time.Hour},
		// Clamp: beyond schedule length reuses the last entry.
		{99, 2 * time.Hour},
		// Negative input is treated as 0.
		{-3, 30 * time.Second},
	}
	for _, c := range cases {
		got := d.backoffForAttempt(c.failed)
		if got != c.want {
			t.Fatalf("backoffForAttempt(%d)=%v want %v", c.failed, got, c.want)
		}
	}
}
