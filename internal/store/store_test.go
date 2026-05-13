package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/coreline-ai/corn-agent-dashboard/internal/db"
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

	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", false, ""); err != nil {
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

func TestCancelRunByID(t *testing.T) {
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
	cancelled, err := st.CancelRun(ctx, run.ID, "user cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("status=%s, want cancelled", cancelled.Status)
	}
	comments, err := st.ListComments(ctx, cancelled.IssueID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].AuthorType != "system" {
		t.Fatalf("expected system cancel comment, got %#v", comments)
	}
}

func TestAgentModelIsUserSelectable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Selectable Model",
		Slug:             "selectable-model",
		IdentifierPrefix: "MOD",
		MainAgent:        CreateAgentInput{Name: "Main", Runtime: "codex", Model: "gpt-5.4", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if main.Model != "gpt-5.4" {
		t.Fatalf("main model=%q, want gpt-5.4", main.Model)
	}
	agent, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Worker", Runtime: "codex", Model: "", Instructions: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Model != "" {
		t.Fatalf("empty model should be preserved as runtime default, got %q", agent.Model)
	}
	updated, err := st.UpdateAgent(ctx, agent.ID, CreateAgentInput{Name: "Worker", Runtime: "codex", Model: "custom-model-id", Instructions: "work updated"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Model != "custom-model-id" {
		t.Fatalf("updated agent model=%q, want custom-model-id", updated.Model)
	}
}

func TestCompleteRunDoesNotOverwriteCancelledRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Race", Slug: "race", IdentifierPrefix: "RCE", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "race task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CancelRun(ctx, run.ID, "user cancelled"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	completed, err := st.CompleteRun(ctx, run.ID, 0, "", "late success", false, "")
	if err != nil {
		t.Fatalf("late complete: %v", err)
	}
	if completed.Status != "cancelled" {
		t.Fatalf("late complete overwrote status: %#v", completed)
	}
	refetched, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refetched.Status == "done" {
		t.Fatalf("cancelled run should not mark issue done: %#v", refetched)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comments {
		if c.AuthorType == "agent" && c.Content == "late success" {
			t.Fatalf("late complete should not insert agent result comment: %#v", comments)
		}
	}
}

func TestUpdateIssueRejectsDoneOrCancelledWithQueuedRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Queued", Slug: "queued", IdentifierPrefix: "QUE", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "queued task"})
	if err != nil {
		t.Fatal(err)
	}
	done := "done"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &done}); !errors.Is(err, ErrState) {
		t.Fatalf("done with queued run err=%v, want ErrState", err)
	}
	cancelled := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &cancelled}); !errors.Is(err, ErrState) {
		t.Fatalf("cancelled with queued run err=%v, want ErrState", err)
	}
	refetchedRun, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refetchedRun.Status != "queued" {
		t.Fatalf("queued run should remain queued after rejected update: %#v", refetchedRun)
	}
}

func TestCompleteRunWithEmptyOutputAddsComment(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Empty", Slug: "empty", IdentifierPrefix: "EMP", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "empty task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "", false, ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawAgent bool
	for _, c := range comments {
		if c.AuthorType == "agent" && c.RunID == run.ID {
			sawAgent = true
			if c.Content == "" {
				t.Fatalf("agent completion comment should not be empty: %#v", c)
			}
		}
	}
	if !sawAgent {
		t.Fatalf("expected agent completion comment, got %#v", comments)
	}
}

func TestCommentTruncatedIsExplicitColumn(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Code", Slug: "code", IdentifierPrefix: "CODE", MainAgent: CreateAgentInput{Name: "Codex", Runtime: "codex", Instructions: "code"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddUserComment(ctx, issue.ID, "전체 로그라는 문자열을 사람이 직접 입력"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "/tmp/run.log", "agent output", true, ""); err != nil {
		t.Fatal(err)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawUser, sawAgent bool
	for _, c := range comments {
		if c.AuthorType == "user" {
			sawUser = true
			if c.Truncated {
				t.Fatalf("user comment was incorrectly marked truncated: %#v", c)
			}
		}
		if c.AuthorType == "agent" {
			sawAgent = true
			if !c.Truncated {
				t.Fatalf("agent comment should be truncated: %#v", c)
			}
		}
	}
	if !sawUser || !sawAgent {
		t.Fatalf("expected user and agent comments, got %#v", comments)
	}
}
