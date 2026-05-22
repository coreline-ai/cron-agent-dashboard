package store

import (
	"context"
	"errors"
	"testing"
)

// Track C of dev-plan/implement_20260522_220446.md.
//
// RetryFailedRunInChain enqueues a new run on the same agent_id and chain_id
// as the most recently failed run in the chain. The depth carries over so
// the chain depth-guard interpretation stays consistent.
func TestRetryFailedRunInChainEnqueuesOnSameAgentAndChain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "ChainRetry",
		Slug:             "chain-retry",
		IdentifierPrefix: "CRT",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "fail me"})
	chainID := run.ID
	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	// Fail the initial run so the chain has a failed terminal row.
	failed, err := st.CompleteRun(ctx, run.ID, 2, "", "", false, "exit code 2")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" {
		t.Fatalf("setup: run status=%q want failed", failed.Status)
	}

	retried, err := st.RetryFailedRunInChain(ctx, chainID)
	if err != nil {
		t.Fatalf("RetryFailedRunInChain: %v", err)
	}
	if retried.Status != "queued" {
		t.Fatalf("retried status=%q want queued", retried.Status)
	}
	if retried.AgentID != failed.AgentID {
		t.Fatalf("retried agent=%q want %q", retried.AgentID, failed.AgentID)
	}
	if retried.ChainID != chainID {
		t.Fatalf("retried chain=%q want %q", retried.ChainID, chainID)
	}
	if retried.ChainDepth != failed.ChainDepth {
		t.Fatalf("retried depth=%d want %d", retried.ChainDepth, failed.ChainDepth)
	}
	if retried.TriggerType != "rerun" {
		t.Fatalf("retried trigger_type=%q want rerun", retried.TriggerType)
	}
	if retried.ParentRunID != failed.ID {
		t.Fatalf("retried parent_run_id=%q want %q", retried.ParentRunID, failed.ID)
	}
}

func TestRetryFailedRunInChainRejectsWhenChainStillHasQueuedRuns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "ChainRetryQueued",
		Slug:             "chain-retry-q",
		IdentifierPrefix: "CRQ",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "still queued"})
	if _, err := st.RetryFailedRunInChain(ctx, run.ID); !errors.Is(err, ErrState) {
		t.Fatalf("retry while queued err=%v want ErrState", err)
	}
}

func TestRetryFailedRunInChainNotFoundForChainWithoutFailedRuns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "ChainRetryDone",
		Slug:             "chain-retry-d",
		IdentifierPrefix: "CRD",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "done chain"})
	if _, ok, err := st.ClaimNextRun(ctx, "w"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RetryFailedRunInChain(ctx, run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retry without failed runs err=%v want ErrNotFound", err)
	}
}
