package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
)

func TestRunStartupSelfCheckReportsHealthyDatabase(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	}); err != nil {
		t.Fatal(err)
	}

	report, err := RunStartupSelfCheck(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if report.IntegrityCheck != "ok" || !report.ForeignKeysEnabled || report.WorkspaceCount != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestRunStartupSelfCheckFailsMainAgentInvariant(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE agent SET is_main=0 WHERE workspace_id=?`, ws.ID); err != nil {
		t.Fatal(err)
	}

	report, err := RunStartupSelfCheck(ctx, st)
	if err == nil {
		t.Fatal("expected self-check to fail")
	}
	if len(report.MainAgentIssues) != 1 || !strings.Contains(report.MainAgentIssues[0], "0 main agents") {
		t.Fatalf("unexpected report=%#v err=%v", report, err)
	}
}

func TestRunStartupSelfCheckTerminatesTrackedProcessGroupsBeforeRecovery(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Process Cleanup",
		Slug:             "process-cleanup",
		IdentifierPrefix: "PROC",
		MainAgent:        store.CreateAgentInput{Name: "Runner", Runtime: "fake", Instructions: "run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, store.CreateIssueInput{Title: "cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if err := st.MarkRunProcess(ctx, run.ID, 9876, 9876); err != nil {
		t.Fatalf("mark process: %v", err)
	}

	var killed []int
	report, err := RunStartupSelfCheckWithOptions(ctx, st, StartupSelfCheckOptions{
		ProcessGroupKillGrace: time.Millisecond,
		TerminateProcessGroup: func(pgid int, grace time.Duration) error {
			killed = append(killed, pgid)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) != 1 || killed[0] != 9876 {
		t.Fatalf("killed pgids=%#v, want [9876]", killed)
	}
	if report.OrphanProcessGroupsTerminated != 1 || report.OrphanRunsRecovered != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	recovered, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "cancelled" || recovered.TerminalReason != store.TerminalReasonOrphanRecovered {
		t.Fatalf("run should be orphan-recovered after cleanup: %#v", recovered)
	}
}
