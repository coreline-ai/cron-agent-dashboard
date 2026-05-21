package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCreateWebhookRoundtripPreservesEventsAndDefaults(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             "hook",
		IdentifierPrefix: "HK",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://example.com/hook",
		Secret: "topsecret",
		Events: []string{"run.completed", "issue.done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://example.com/hook" || got.Secret != "topsecret" || !got.Enabled {
		t.Fatalf("bad webhook: %#v", got)
	}
	if !reflect.DeepEqual(got.Events, []string{"run.completed", "issue.done"}) {
		t.Fatalf("events mismatch: %#v", got.Events)
	}

	fetched, err := st.GetWebhook(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.URL != got.URL || !reflect.DeepEqual(fetched.Events, got.Events) {
		t.Fatalf("get mismatch: %#v", fetched)
	}
}

func TestCreateWebhookRejectsBadInputs(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             "hook-bad",
		IdentifierPrefix: "HKB",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})

	cases := []struct {
		name string
		in   UpsertWebhookInput
	}{
		{"empty url", UpsertWebhookInput{URL: ""}},
		{"missing scheme", UpsertWebhookInput{URL: "example.com"}},
		{"unsupported scheme", UpsertWebhookInput{URL: "ftp://example.com/hook"}},
		{"unknown event", UpsertWebhookInput{URL: "https://example.com/h", Events: []string{"nope"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := st.CreateWebhook(ctx, ws.ID, c.in); !errors.Is(err, ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
		})
	}
}

func TestUpdateWebhookReplacesEveryField(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             "hook-update",
		IdentifierPrefix: "HKU",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	created, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{URL: "https://a.example/h", Secret: "s1", Events: []string{"run.completed"}})

	disabled := false
	updated, err := st.UpdateWebhook(ctx, created.ID, UpsertWebhookInput{
		URL:     "https://b.example/h2",
		Secret:  "s2",
		Events:  []string{"issue.done"},
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.URL != "https://b.example/h2" || updated.Secret != "s2" || updated.Enabled {
		t.Fatalf("update did not replace fields: %#v", updated)
	}
	if !reflect.DeepEqual(updated.Events, []string{"issue.done"}) {
		t.Fatalf("events not replaced: %#v", updated.Events)
	}
}

func TestDeleteWebhookCascadesFromWorkspace(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             "hook-cascade",
		IdentifierPrefix: "HKC",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	w1, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{URL: "https://a.example/h"})
	if err := st.DeleteWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetWebhook(ctx, w1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected webhook to be cascade-deleted, got %v", err)
	}
}

func TestWebhookDeliveryEnqueuedOnRunCompleted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	autoClose := true
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:               "HookRunCompleted",
		Slug:               "hook-rc",
		IdentifierPrefix:   "HRC",
		AutoCloseOnRunDone: &autoClose,
		MainAgent:          CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://x.example/h",
		Events: []string{"run.completed", "issue.done"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "rc"})
	if _, _, err := st.ClaimNextRun(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "ok", false, ""); err != nil {
		t.Fatal(err)
	}

	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 deliveries (run.completed + issue.done), got %d: %#v", len(deliveries), deliveries)
	}
	events := map[string]bool{}
	for _, d := range deliveries {
		events[d.EventType] = true
		if d.Status != "pending" {
			t.Fatalf("delivery should be pending, got %#v", d)
		}
		if !strings.Contains(d.PayloadJSON, ws.Slug) || !strings.Contains(d.PayloadJSON, run.ID) {
			t.Fatalf("payload missing workspace slug or run id: %s", d.PayloadJSON)
		}
	}
	if !events["run.completed"] || !events["issue.done"] {
		t.Fatalf("expected both run.completed and issue.done, got %v", events)
	}
}

func TestWebhookDeliveryEnqueuedOnRunFailed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "HookRunFailed",
		Slug:             "hook-rf",
		IdentifierPrefix: "HRF",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	hook, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{
		URL:    "https://x.example/h",
		Events: []string{"run.failed"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "rf"})
	if _, _, err := st.ClaimNextRun(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FailInfrastructureRun(ctx, run.ID, TerminalReasonUnknownFailure, FailureKindUnknown, "boom"); err != nil {
		t.Fatal(err)
	}

	deliveries, _ := st.ListWebhookDeliveries(ctx, hook.ID, 10)
	if len(deliveries) == 0 {
		t.Fatalf("expected at least one run.failed delivery, got none")
	}
	if deliveries[0].EventType != "run.failed" {
		t.Fatalf("expected run.failed event, got %q", deliveries[0].EventType)
	}
}

func TestEnqueueWebhookDeliveriesRespectsFilterAndEnabled(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Hook",
		Slug:             "hook-enq",
		IdentifierPrefix: "HKE",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	all, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{URL: "https://all.example/h"}) // empty events => all
	onlyDone, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{URL: "https://done.example/h", Events: []string{"issue.done"}})
	disabled := false
	disabledHook, _ := st.CreateWebhook(ctx, ws.ID, UpsertWebhookInput{URL: "https://dis.example/h", Enabled: &disabled})

	tx, err := st.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueWebhookDeliveries(ctx, tx, ws.ID, "run.completed", []byte(`{"ok":true}`))
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if queued != 1 { // only the "all" webhook matches; onlyDone filters out, disabled excluded.
		t.Fatalf("queued=%d, want 1", queued)
	}

	deliveries, err := st.ListWebhookDeliveries(ctx, all.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].EventType != "run.completed" || deliveries[0].Status != "pending" {
		t.Fatalf("unexpected delivery rows: %#v", deliveries)
	}

	// Disabled and filtered webhooks should have no delivery rows yet.
	if rows, _ := st.ListWebhookDeliveries(ctx, onlyDone.ID, 10); len(rows) != 0 {
		t.Fatalf("filtered webhook should not receive run.completed; got %#v", rows)
	}
	if rows, _ := st.ListWebhookDeliveries(ctx, disabledHook.ID, 10); len(rows) != 0 {
		t.Fatalf("disabled webhook should not receive any deliveries; got %#v", rows)
	}
}
