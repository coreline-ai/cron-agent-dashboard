package store

import (
	"context"
	"testing"
)

// Track D of dev-plan/implement_20260522_212332.md.
//
// CancelRunsByChain must cancel every queued / running run that shares a
// chain_id while leaving terminal rows alone. A chain mixing queued +
// running + completed runs should end up with exactly the
// non-terminal subset cancelled.
func TestCancelRunsByChainCancelsOnlyNonTerminalRunsInChain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "ChainCancel",
		Slug:             "chain-cancel",
		IdentifierPrefix: "CHC",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	// Issue 1 with initial run — this run becomes the chain root.
	_, run1, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "first"})
	chainID := run1.ID
	// Issue 2 — different chain, must NOT be cancelled.
	_, run2, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "second chain"})
	if run2.ChainID == chainID {
		t.Fatalf("second issue should be on a different chain")
	}
	// Manually flip run1 to 'running' to mimic an in-flight chain start.
	if _, err := st.DB().ExecContext(ctx, `UPDATE run SET status='running' WHERE id=?`, run1.ID); err != nil {
		t.Fatal(err)
	}
	// Add a second queued run that shares chain_id with run1 to mimic a hub
	// re-entry that has not been claimed yet. We splice it directly because
	// the auto-chain dispatcher path requires a comment trigger.
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','mention','',datetime('now'),3,1,?,1)`,
		"sibling-run", run1.IssueID, run1.AgentID, chainID,
	); err != nil {
		t.Fatal(err)
	}

	cancelled, err := st.CancelRunsByChain(ctx, chainID, CancelReasonInput{
		Message:        "Chain cancelled by test",
		TerminalReason: TerminalReasonUserCancelled,
		CancelReason:   CancelReasonUser,
	})
	if err != nil {
		t.Fatalf("CancelRunsByChain: %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("expected to cancel 2 runs (running + queued sibling), got %d", cancelled)
	}

	got, err := st.GetRun(ctx, run1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("run1 status=%q want cancelled", got.Status)
	}
	if got.TerminalReason != TerminalReasonUserCancelled || got.CancelReason != CancelReasonUser {
		t.Fatalf("run1 reason classification: %#v", got)
	}
	sibling, err := st.GetRun(ctx, "sibling-run")
	if err != nil {
		t.Fatal(err)
	}
	if sibling.Status != "cancelled" {
		t.Fatalf("sibling status=%q want cancelled", sibling.Status)
	}
	// Other-chain run unaffected.
	other, err := st.GetRun(ctx, run2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if other.Status != "queued" {
		t.Fatalf("other chain run should still be queued, got %q", other.Status)
	}
}

func TestCancelRunsByChainNoOpForTerminalChain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, _ := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "ChainCancelTerminal",
		Slug:             "chain-cancel-t",
		IdentifierPrefix: "CCT",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	_, run, _ := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "already done"})
	if _, _, err := st.ClaimNextRun(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", false, ""); err != nil {
		t.Fatal(err)
	}

	cancelled, err := st.CancelRunsByChain(ctx, run.ID, CancelReasonInput{})
	if err != nil {
		t.Fatalf("CancelRunsByChain: %v", err)
	}
	if cancelled != 0 {
		t.Fatalf("expected 0 (chain fully terminal), got %d", cancelled)
	}
}
