package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return New(database)
}

func TestStoreIssueRunAndWorkspaceSerialClaim(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "AI News", Slug: "ai-news", IdentifierPrefix: "NEWS", MainAgent: CreateAgentInput{Name: "NewsLead", Runtime: "codex", Instructions: "lead"}})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	writer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "오늘 뉴스", Body: "body"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issue.Identifier != "NEWS-1" || run.Status != "queued" || run.AgentID != main.ID {
		t.Fatalf("bad issue/run: %#v %#v", issue, run)
	}

	claimed, ok, err := st.ClaimNextRun(ctx, "worker-1")
	if err != nil || !ok {
		t.Fatalf("claim first: ok=%v err=%v", ok, err)
	}
	if claimed.ID != run.ID || claimed.Status != "running" {
		t.Fatalf("bad claimed run: %#v", claimed)
	}

	_, _, err = st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "다음 뉴스"})
	if err != nil {
		t.Fatalf("create second issue: %v", err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker-2"); err != nil || ok {
		t.Fatalf("workspace serial claim got ok=%v err=%v, want no claim", ok, err)
	}

	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker-2"); err != nil || !ok {
		t.Fatalf("claim after complete ok=%v err=%v", ok, err)
	}

	res, err := st.AddUserComment(ctx, issue.ID, "@Writer 다듬어줘")
	if err != nil {
		t.Fatalf("add mention: %v", err)
	}
	if res.DispatchedRun == nil || res.DispatchedRun.AgentID != writer.ID {
		t.Fatalf("mention did not dispatch writer: %#v", res)
	}
}

func TestRecoverOrphanRuns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Code", Slug: "code", IdentifierPrefix: "CODE", MainAgent: CreateAgentInput{Name: "Codex", Runtime: "codex", Instructions: "code"}})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	n, err := st.RecoverOrphanRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered %d, want 1", n)
	}
	recovered, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "cancelled" || !recovered.ExitCode.Valid || recovered.ExitCode.Int64 != -2 {
		t.Fatalf("bad recovered run: %#v", recovered)
	}
}
