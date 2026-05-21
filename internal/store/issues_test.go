package store

import (
	"context"
	"fmt"
	"testing"
)

// TestListIssuesExecutionFilterIsAppliedBeforeLimit reproduces the bug where
// the execution filter was applied in memory after SQL LIMIT, dropping older
// matching rows when more recent non-matching rows fill the LIMIT window.
func TestListIssuesExecutionFilterIsAppliedBeforeLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Filter",
		Slug:             "filter",
		IdentifierPrefix: "FLT",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create 10 issues whose runs are completed (execution_status="done").
	for i := 0; i < 10; i++ {
		_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: fmt.Sprintf("done-%d", i)})
		if err != nil {
			t.Fatalf("create done-%d: %v", i, err)
		}
		claimed, ok, err := st.ClaimNextRun(ctx, "w")
		if err != nil || !ok || claimed.ID != run.ID {
			t.Fatalf("claim done-%d: ok=%v err=%v", i, ok, err)
		}
		if _, err := st.CompleteRun(ctx, run.ID, 0, "", "ok", false, ""); err != nil {
			t.Fatalf("complete done-%d: %v", i, err)
		}
	}

	// Create 1 issue whose run is still queued (execution_status="queued").
	// This is the most recent issue and would otherwise be returned at the
	// head of a created_at DESC limit window.
	if _, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "fresh-open"}); err != nil {
		t.Fatalf("create fresh-open: %v", err)
	}

	list, err := st.ListIssues(ctx, ws.ID, ListIssuesFilter{Limit: 10, Execution: []string{"done"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 10 {
		t.Fatalf("expected 10 done issues with execution=done filter, got %d", len(list))
	}
	for _, iss := range list {
		if iss.ExecutionStatus != "done" {
			t.Fatalf("expected execution_status=done for every row, got %q in %s", iss.ExecutionStatus, iss.Identifier)
		}
	}
}

func TestListIssuesExecutionFilterAcceptsMultipleValues(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "FilterMulti",
		Slug:             "filter-multi",
		IdentifierPrefix: "FLM",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2 completed (done), 2 queued (queued), 1 cancelled.
	for i := 0; i < 2; i++ {
		_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: fmt.Sprintf("done-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := st.ClaimNextRun(ctx, "w"); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CompleteRun(ctx, run.ID, 0, "", "ok", false, ""); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: fmt.Sprintf("queued-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, cancelRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "cancel-1"}); err != nil {
		t.Fatal(err)
	} else if _, err := st.CancelRunWithReason(ctx, cancelRun.ID, CancelReasonInput{
		Message:        "test",
		TerminalReason: TerminalReasonUserCancelled,
		CancelReason:   CancelReasonUser,
	}); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListIssues(ctx, ws.ID, ListIssuesFilter{Limit: 50, Execution: []string{"done", "cancelled"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 issues (2 done + 1 cancelled), got %d", len(list))
	}
	for _, iss := range list {
		if iss.ExecutionStatus != "done" && iss.ExecutionStatus != "cancelled" {
			t.Fatalf("unexpected execution_status=%q in %s", iss.ExecutionStatus, iss.Identifier)
		}
	}
}
