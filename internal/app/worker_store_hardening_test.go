package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestWorkerStoreCancelRunUsesExplicitReasonInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Worker Cancel",
		Slug:             "worker-cancel",
		IdentifierPrefix: "WCN",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "fake", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "cancel via worker store"})
	if err != nil {
		t.Fatal(err)
	}

	const message = "manual shutdown request"
	if err := NewWorkerStore(st).CancelRun(ctx, run.ID, message); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TerminalReason != store.TerminalReasonShutdown || got.CancelReason != store.CancelReasonShutdown || got.ErrorMessage != message {
		t.Fatalf("worker store cancel did not persist explicit shutdown reason: %#v", got)
	}
}

func TestCancelClaimedReturnsInfrastructureFailRecordError(t *testing.T) {
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(database)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	cause := errors.New("workspace lookup failed")
	_, err = NewWorkerStore(st).cancelClaimed(context.Background(), "run-id", cause)
	if err == nil {
		t.Fatal("expected cancelClaimed error")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("returned error should preserve cause: %v", err)
	}
	if err == cause {
		t.Fatalf("returned error should include infrastructure fail record error, got only cause: %v", err)
	}
	if !strings.Contains(err.Error(), "record infrastructure run failure") {
		t.Fatalf("returned error does not include infrastructure fail record context: %v", err)
	}
}
