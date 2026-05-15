package store

import (
	"context"
	"testing"
)

func TestNormalizeFinishRunInputDefaultsTerminalFields(t *testing.T) {
	status, normalized := normalizeFinishRunInput(FinishRunInput{ExitCode: 0})
	if status != "done" || normalized.TerminalReason != TerminalReasonCompleted || normalized.FailureKind != "" {
		t.Fatalf("success normalize status=%q input=%#v", status, normalized)
	}

	status, normalized = normalizeFinishRunInput(FinishRunInput{ExitCode: 2})
	if status != "failed" || normalized.TerminalReason != TerminalReasonExitNonzero || normalized.FailureKind != FailureKindExitNonzero {
		t.Fatalf("failure normalize status=%q input=%#v", status, normalized)
	}

	status, normalized = normalizeFinishRunInput(FinishRunInput{ExitCode: 0, TerminalReason: TerminalReasonTimeout})
	if status != "failed" || normalized.FailureKind != FailureKindUnknown {
		t.Fatalf("non-completed terminal reason should fail with unknown failure kind: status=%q input=%#v", status, normalized)
	}
}

func TestCompleteRunWithReasonRecordsCommentEventsMetricsAndIssueDone(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Finish Helpers",
		Slug:             "finish-helpers",
		IdentifierPrefix: "FIN",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "finish task"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if claimed, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok || claimed.ID != run.ID {
		t.Fatalf("claim=%#v ok=%v err=%v", claimed, ok, err)
	}

	completed, err := st.CompleteRunWithReason(ctx, run.ID, FinishRunInput{
		ExitCode:         0,
		StdoutPath:       "/tmp/finish-helper.log",
		Content:          "agent result",
		ContentTruncated: true,
		StdoutTruncated:  true,
		InputTokens:      11,
		OutputTokens:     22,
		TotalCostMicros:  33,
		ModelResolved:    "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != "done" || completed.InputTokens != 11 || completed.OutputTokens != 22 || completed.TotalCostMicros != 33 || completed.ModelResolved != "gpt-5.5" {
		t.Fatalf("bad completed run: %#v", completed)
	}
	updatedIssue, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedIssue.Status != "done" {
		t.Fatalf("issue status=%q, want done", updatedIssue.Status)
	}

	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawAgent bool
	for _, c := range comments {
		if c.AuthorType == "agent" && c.RunID == run.ID {
			sawAgent = true
			if c.Content != "agent result" || !c.Truncated {
				t.Fatalf("bad agent comment: %#v", c)
			}
		}
	}
	if !sawAgent {
		t.Fatalf("expected agent result comment, got %#v", comments)
	}

	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRunEvent(events, RunEventStdoutTrunc) || !hasRunEvent(events, RunEventCompleted) {
		t.Fatalf("expected stdout_truncated and run_completed events, got %#v", events)
	}
	last := events[len(events)-1]
	if last.EventType != RunEventCompleted || last.Details["model_resolved"] != "gpt-5.5" {
		t.Fatalf("bad completion event: %#v", last)
	}
}

func hasRunEvent(events []RunEvent, eventType string) bool {
	for _, e := range events {
		if e.EventType == eventType {
			return true
		}
	}
	return false
}
