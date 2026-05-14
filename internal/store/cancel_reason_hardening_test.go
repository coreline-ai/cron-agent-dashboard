package store

import (
	"context"
	"testing"
)

func TestCancelRunWithReasonHonorsExplicitLifecycleFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Cancel Hardening",
		Slug:             "cancel-hardening",
		IdentifierPrefix: "CAN",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "explicit cancel"})
	if err != nil {
		t.Fatal(err)
	}

	const message = "operator requested stop"
	cancelled, err := st.CancelRunWithReason(ctx, run.ID, CancelReasonInput{
		Message:        message,
		TerminalReason: TerminalReasonShutdown,
		CancelReason:   CancelReasonShutdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.ErrorMessage != message {
		t.Fatalf("bad cancelled run state: %#v", cancelled)
	}
	if cancelled.TerminalReason != TerminalReasonShutdown || cancelled.CancelReason != CancelReasonShutdown {
		t.Fatalf("explicit lifecycle fields were not preserved: %#v", cancelled)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected run events")
	}
	last := events[len(events)-1]
	if last.EventType != RunEventCancelled {
		t.Fatalf("last event=%s, want %s", last.EventType, RunEventCancelled)
	}
	if last.Details["terminal_reason"] != TerminalReasonShutdown || last.Details["cancel_reason"] != CancelReasonShutdown {
		t.Fatalf("cancel event did not record explicit lifecycle fields: %#v", last.Details)
	}
}

func TestCancelRunWithReasonDerivesTerminalReasonFromCancelReason(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Cancel Reason Derive",
		Slug:             "cancel-reason-derive",
		IdentifierPrefix: "CRD",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "derive cancel reason"})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := st.CancelRunWithReason(ctx, run.ID, CancelReasonInput{
		Message:      "operator requested stop",
		CancelReason: CancelReasonShutdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.TerminalReason != TerminalReasonShutdown || cancelled.CancelReason != CancelReasonShutdown {
		t.Fatalf("cancel reason should derive terminal reason without message classification: %#v", cancelled)
	}
}
