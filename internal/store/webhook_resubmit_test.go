package store

import (
	"context"
	"errors"
	"testing"
)

// Track B of dev-plan/implement_20260523_203219.md.
//
// ResubmitWebhookDelivery flips a status='failed' row back to 'pending'
// so the dispatcher picks it up on its next poll. The row's payload
// stays intact; attempt counter and error fields are reset.
func TestResubmitWebhookDeliveryReopensFailedRow(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "WHResubmit",
		Slug:             "wh-resubmit",
		IdentifierPrefix: "WHR",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, err := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://x.example/h",
		Events: []string{"issue.cancelled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "cancel me"})
	status := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &status}); err != nil {
		t.Fatal(err)
	}
	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(deliveries) != 1 {
		t.Fatalf("want 1 delivery row, got %d", len(deliveries))
	}
	// Force it to terminal failure to mimic the dispatcher giving up.
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE webhook_delivery SET status='failed', attempt=6, status_code=500, error_message='gateway timeout' WHERE id=?`,
		deliveries[0].ID,
	); err != nil {
		t.Fatal(err)
	}

	if err := st.ResubmitWebhookDelivery(ctx, deliveries[0].ID); err != nil {
		t.Fatalf("ResubmitWebhookDelivery: %v", err)
	}
	after, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(after) != 1 {
		t.Fatalf("expected 1 row after resubmit, got %d", len(after))
	}
	r := after[0]
	if r.Status != "pending" {
		t.Fatalf("status=%q want pending", r.Status)
	}
	if r.Attempt != 0 {
		t.Fatalf("attempt=%d want 0", r.Attempt)
	}
	if r.StatusCode != 0 || r.ErrorMessage != "" {
		t.Fatalf("error fields not cleared: %#v", r)
	}
	if r.PayloadJSON != deliveries[0].PayloadJSON {
		t.Fatalf("payload mutated: before=%q after=%q", deliveries[0].PayloadJSON, r.PayloadJSON)
	}
}

func TestResubmitWebhookDeliveryRejectsNonFailedStatus(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "WHResubmitState",
		Slug:             "wh-resubmit-state",
		IdentifierPrefix: "WRS",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://x.example/h",
		Events: []string{"issue.cancelled"},
	})
	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "x"})
	status := "cancelled"
	_, _ = st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &status})
	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if err := st.ResubmitWebhookDelivery(ctx, deliveries[0].ID); !errors.Is(err, ErrState) {
		t.Fatalf("pending row err=%v want ErrState", err)
	}
}

func TestResubmitWebhookDeliveryMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.ResubmitWebhookDelivery(ctx, "missing-delivery"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id err=%v want ErrNotFound", err)
	}
}
