package store

import (
	"context"
	"strings"
	"testing"
)

func TestWebhookDeliveryEnqueuedOnIssueCancelled(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "HookIssueCancelled",
		Slug:             "hook-ic",
		IdentifierPrefix: "HIC",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://x.example/h",
		Events: []string{"issue.cancelled"},
	})
	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "to be cancelled"})

	status := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &status}); err != nil {
		t.Fatalf("cancel issue: %v", err)
	}

	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery for issue.cancelled, got %d: %#v", len(deliveries), deliveries)
	}
	if deliveries[0].EventType != "issue.cancelled" {
		t.Fatalf("unexpected event_type=%q", deliveries[0].EventType)
	}
	if !strings.Contains(deliveries[0].PayloadJSON, `"identifier":"HIC-1"`) {
		t.Fatalf("payload missing issue identifier: %s", deliveries[0].PayloadJSON)
	}
	if !strings.Contains(deliveries[0].PayloadJSON, `"status":"cancelled"`) {
		t.Fatalf("payload missing status=cancelled: %s", deliveries[0].PayloadJSON)
	}
}

func TestWebhookMaskPIIRedactsPayloadOnEnqueue(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "HookMaskPII",
		Slug:             "hook-mask",
		IdentifierPrefix: "MASK",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	maskTrue := true
	masked, err := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:     "https://m.example/h",
		Events:  []string{"issue.cancelled"},
		MaskPII: &maskTrue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !masked.MaskPII {
		t.Fatalf("MaskPII not persisted: %#v", masked)
	}
	maskFalse := false
	plain, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:     "https://p.example/h",
		Events:  []string{"issue.cancelled"},
		MaskPII: &maskFalse,
	})

	issue, _, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{
		Title: "Reach jane.doe@company.com about 010-1234-5678",
	})
	status := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &status}); err != nil {
		t.Fatal(err)
	}

	maskedDeliveries, _ := st.ListWebhookDeliveries(ctx, masked.ID, 10)
	if len(maskedDeliveries) != 1 {
		t.Fatalf("masked: want 1 delivery, got %d", len(maskedDeliveries))
	}
	if strings.Contains(maskedDeliveries[0].PayloadJSON, "jane.doe@company.com") {
		t.Fatalf("email leaked to masked subscription: %s", maskedDeliveries[0].PayloadJSON)
	}
	if !strings.Contains(maskedDeliveries[0].PayloadJSON, "[email]") {
		t.Fatalf("masked subscription should show [email] placeholder: %s", maskedDeliveries[0].PayloadJSON)
	}
	if strings.Contains(maskedDeliveries[0].PayloadJSON, "010-1234-5678") {
		t.Fatalf("phone leaked to masked subscription: %s", maskedDeliveries[0].PayloadJSON)
	}

	plainDeliveries, _ := st.ListWebhookDeliveries(ctx, plain.ID, 10)
	if len(plainDeliveries) != 1 {
		t.Fatalf("plain: want 1 delivery, got %d", len(plainDeliveries))
	}
	if !strings.Contains(plainDeliveries[0].PayloadJSON, "jane.doe@company.com") {
		t.Fatalf("non-masked subscription must receive raw email: %s", plainDeliveries[0].PayloadJSON)
	}
}
