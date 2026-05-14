package store

import (
	"context"
	"errors"
	"testing"
)

func TestRunProcessTrackingRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Process Tracking",
		Slug:             "process-tracking",
		IdentifierPrefix: "PROC",
		MainAgent:        CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "track process"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}

	if err := st.MarkRunProcess(ctx, run.ID, 123, 1); err != nil {
		t.Fatalf("mark pgid<=1: %v", err)
	}
	groups, err := st.ListRunningProcessGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("pgid<=1 should be excluded, got %#v", groups)
	}

	if err := st.MarkRunProcess(ctx, run.ID, 4321, 4321); err != nil {
		t.Fatalf("mark process: %v", err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcessPID != 4321 || got.ProcessPGID != 4321 || got.ProcessRecordedAt == "" {
		t.Fatalf("process metadata not persisted: %#v", got)
	}
	groups, err = st.ListRunningProcessGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].PGID != 4321 || groups[0].RecordedAt == "" || groups[0].RunCount != 1 {
		t.Fatalf("running groups=%#v, want [4321]", groups)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.EventType != RunEventStarting {
		t.Fatalf("last event=%s, want %s", last.EventType, RunEventStarting)
	}
	if last.Details["pid"] != float64(4321) || last.Details["pgid"] != float64(4321) {
		t.Fatalf("unexpected process event details: %#v", last.Details)
	}

	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", false, ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	groups, err = st.ListRunningProcessGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("terminal run should not be listed, got %#v", groups)
	}
	if err := st.MarkRunProcess(ctx, run.ID, 5555, 5555); !errors.Is(err, ErrState) {
		t.Fatalf("marking terminal run err=%v, want ErrState", err)
	}
}
