package app

import (
	"context"
	"strings"
	"testing"

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
