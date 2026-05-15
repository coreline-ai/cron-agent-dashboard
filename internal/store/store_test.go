package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func intPtr(v int) *int { return &v }

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
	if claimed.HeartbeatAt == "" {
		t.Fatalf("claim should set heartbeat_at: %#v", claimed)
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
	completed, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.TerminalReason != TerminalReasonCompleted || completed.FailureKind != "" || completed.CancelReason != "" {
		t.Fatalf("bad completed lifecycle fields: %#v", completed)
	}
	if err := st.HeartbeatRun(ctx, run.ID); !errors.Is(err, ErrState) {
		t.Fatalf("heartbeat on terminal run err=%v, want ErrState", err)
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
	if recovered.TerminalReason != TerminalReasonOrphanRecovered || recovered.CancelReason != CancelReasonOrphan {
		t.Fatalf("bad orphan lifecycle fields: %#v", recovered)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := events[len(events)-1].EventType; got != RunEventOrphan {
		t.Fatalf("last event=%s, want %s", got, RunEventOrphan)
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
	if cancelled.TerminalReason != TerminalReasonUserCancelled || cancelled.CancelReason != CancelReasonUser {
		t.Fatalf("bad cancel lifecycle fields: %#v", cancelled)
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

func TestAgentInstructionVersionHistoryAndRunSnapshot(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Instruction Audit",
		Slug:             "instruction-audit",
		IdentifierPrefix: "AUD",
		MainAgent:        CreateAgentInput{Name: "Main", Runtime: "codex", Instructions: "lead v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if main.InstructionsVersion != 1 {
		t.Fatalf("main instructions version=%d, want 1", main.InstructionsVersion)
	}
	mainVersions, err := st.ListAgentInstructionVersions(ctx, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainVersions) != 1 || mainVersions[0].Version != 1 || mainVersions[0].Instructions != "lead v1" {
		t.Fatalf("bad main history: %#v", mainVersions)
	}

	agent, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write v1", Summary: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.InstructionsVersion != 1 {
		t.Fatalf("agent version=%d, want 1", agent.InstructionsVersion)
	}

	sameInstructions, err := st.UpdateAgent(ctx, agent.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write v1", Summary: "writer updated"})
	if err != nil {
		t.Fatal(err)
	}
	if sameInstructions.InstructionsVersion != 1 {
		t.Fatalf("version should not change when instructions are unchanged: %#v", sameInstructions)
	}

	updated, err := st.UpdateAgent(ctx, agent.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write v2", Summary: "writer updated"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstructionsVersion != 2 {
		t.Fatalf("updated version=%d, want 2", updated.InstructionsVersion)
	}
	versions, err := st.ListAgentInstructionVersions(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Version != 2 || versions[0].Instructions != "write v2" || versions[1].Version != 1 {
		t.Fatalf("bad instruction history: %#v", versions)
	}

	_, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "audit", AssigneeAgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if run.AgentInstructionsVersion != 2 {
		t.Fatalf("run instruction snapshot=%d, want 2", run.AgentInstructionsVersion)
	}

	if _, err := st.UpdateAgent(ctx, agent.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write v3"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AgentInstructionsVersion != 2 {
		t.Fatalf("existing run snapshot changed to %d, want 2", snapshot.AgentInstructionsVersion)
	}
}

func TestWorkspaceAndAgentResourceControlsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	mainTimeout := 45
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:                  "Controls",
		Slug:                  "controls",
		IdentifierPrefix:      "CTL",
		DefaultTimeoutSeconds: 120,
		MainAgent:             CreateAgentInput{Name: "Main", Runtime: "codex", Instructions: "lead", TimeoutSecondsOverride: &mainTimeout, RetryPolicyJSON: `{"max_attempts":2}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ws.DefaultTimeoutSeconds != 120 {
		t.Fatalf("workspace timeout=%d, want 120", ws.DefaultTimeoutSeconds)
	}
	if !main.TimeoutSecondsOverride.Valid || main.TimeoutSecondsOverride.Int64 != 45 || main.RetryPolicyJSON != `{"max_attempts":2}` {
		t.Fatalf("main controls not persisted: %#v", main)
	}

	ws, err = st.UpdateWorkspace(ctx, ws.ID, UpdateWorkspaceInput{Name: ws.Name, Description: "updated", DefaultTimeoutSeconds: intPtr(300)})
	if err != nil {
		t.Fatal(err)
	}
	if ws.DefaultTimeoutSeconds != 300 {
		t.Fatalf("updated workspace timeout=%d, want 300", ws.DefaultTimeoutSeconds)
	}

	agentTimeout := 90
	agent, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Worker", Runtime: "codex", Instructions: "work", TimeoutSecondsOverride: &agentTimeout, RetryPolicyJSON: `{"max_attempts":3}`})
	if err != nil {
		t.Fatal(err)
	}
	if !agent.TimeoutSecondsOverride.Valid || agent.TimeoutSecondsOverride.Int64 != 90 || agent.RetryPolicyJSON != `{"max_attempts":3}` {
		t.Fatalf("agent controls not persisted: %#v", agent)
	}

	updatedTimeout := 0
	updated, err := st.UpdateAgent(ctx, agent.ID, CreateAgentInput{Name: "Worker", Runtime: "codex", Instructions: "work", TimeoutSecondsOverride: &updatedTimeout, RetryPolicyJSON: `{"max_attempts":1}`})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TimeoutSecondsOverride.Valid || updated.RetryPolicyJSON != `{"max_attempts":1}` {
		t.Fatalf("agent controls after update: %#v", updated)
	}
}

func TestAgentRetryPolicyValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Retry Validation", Slug: "retry-validation", IdentifierPrefix: "RTY", MainAgent: CreateAgentInput{Name: "Main", Runtime: "codex", Instructions: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "BadJSON", Runtime: "codex", Instructions: "bad", RetryPolicyJSON: `{bad}`}); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad retry policy err=%v, want ErrValidation", err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "TooMany", Runtime: "codex", Instructions: "bad", RetryPolicyJSON: `{"max_attempts":99}`}); !errors.Is(err, ErrValidation) {
		t.Fatalf("too many attempts err=%v, want ErrValidation", err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "BadBackoff", Runtime: "codex", Instructions: "bad", RetryPolicyJSON: `{"max_attempts":2,"backoff_seconds":[0]}`}); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad backoff err=%v, want ErrValidation", err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "BadKind", Runtime: "codex", Instructions: "bad", RetryPolicyJSON: `{"max_attempts":2,"retry_on":["worker_panic"]}`}); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad retry_on err=%v, want ErrValidation", err)
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
	cancelled, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	issueBeforeLateComplete, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	stdoutPath := filepath.Join(t.TempDir(), "partial.log")
	if err := os.WriteFile(stdoutPath, []byte("partial stdout before cancellation\n"), 0o600); err != nil {
		t.Fatalf("write stdout fixture: %v", err)
	}
	completed, err := st.CompleteRunWithReason(ctx, run.ID, FinishRunInput{
		ExitCode:       124,
		StdoutPath:     stdoutPath,
		Content:        "late failure output",
		ErrorMessage:   "late timeout",
		TerminalReason: TerminalReasonTimeout,
		FailureKind:    FailureKindTimeout,
	})
	if err != nil {
		t.Fatalf("late complete: %v", err)
	}
	if completed.Status != "cancelled" {
		t.Fatalf("late complete overwrote status: %#v", completed)
	}
	if !completed.ExitCode.Valid || !cancelled.ExitCode.Valid || completed.ExitCode.Int64 != cancelled.ExitCode.Int64 {
		t.Fatalf("late complete overwrote exit code: before=%#v after=%#v", cancelled, completed)
	}
	if completed.ErrorMessage != cancelled.ErrorMessage {
		t.Fatalf("late complete overwrote error message: before=%#v after=%#v", cancelled, completed)
	}
	if completed.TerminalReason != TerminalReasonUserCancelled || completed.CancelReason != CancelReasonUser {
		t.Fatalf("late complete overwrote terminal fields: %#v", completed)
	}
	if completed.FailureKind != cancelled.FailureKind {
		t.Fatalf("late complete overwrote failure kind: before=%#v after=%#v", cancelled, completed)
	}
	if !completed.StdoutPath.Valid || completed.StdoutPath.String != stdoutPath {
		t.Fatalf("late complete should recover stdout path only: %#v", completed)
	}
	logPath, err := st.GetRunLogPath(ctx, run.ID)
	if err != nil {
		t.Fatalf("recovered log path should be accessible: %v", err)
	}
	if logPath != stdoutPath {
		t.Fatalf("log path=%q, want %q", logPath, stdoutPath)
	}
	refetched, err := st.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refetched.Status != issueBeforeLateComplete.Status {
		t.Fatalf("late complete overwrote issue status: before=%#v after=%#v", issueBeforeLateComplete, refetched)
	}
	if refetched.Status == "done" {
		t.Fatalf("cancelled run should not mark issue done: %#v", refetched)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comments {
		if c.AuthorType == "agent" && c.Content == "late failure output" {
			t.Fatalf("late complete should not insert agent result comment: %#v", comments)
		}
	}
}

func TestUpdateIssueActiveRunTransitions(t *testing.T) {
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
	cancelledIssue, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &cancelled})
	if err != nil {
		t.Fatalf("cancelled with queued run: %v", err)
	}
	if cancelledIssue.Status != "cancelled" {
		t.Fatalf("issue status=%s, want cancelled", cancelledIssue.Status)
	}
	refetchedRun, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refetchedRun.Status != "cancelled" {
		t.Fatalf("queued run should be cancelled with issue: %#v", refetchedRun)
	}
	if refetchedRun.TerminalReason != TerminalReasonIssueCancelled || refetchedRun.CancelReason != CancelReasonIssue {
		t.Fatalf("queued run issue cancel fields: %#v", refetchedRun)
	}
}

func TestUpdateIssueRejectsCancelledWithRunningRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Running", Slug: "running", IdentifierPrefix: "RUN", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "running task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	cancelled := "cancelled"
	if _, err := st.UpdateIssue(ctx, issue.ID, UpdateIssueInput{Status: &cancelled}); !errors.Is(err, ErrState) {
		t.Fatalf("cancelled with running run err=%v, want ErrState", err)
	}
}

func TestDeleteIssueAndWorkspaceRejectQueuedRuns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Queued Delete", Slug: "queued-delete", IdentifierPrefix: "DEL", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "queued task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteIssue(ctx, issue.ID); !errors.Is(err, ErrState) {
		t.Fatalf("delete issue err=%v, want ErrState", err)
	}
	if err := st.DeleteWorkspace(ctx, ws.ID); !errors.Is(err, ErrState) {
		t.Fatalf("delete workspace err=%v, want ErrState", err)
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

func TestRunEventsRoundTripAndCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Events", Slug: "events", IdentifierPrefix: "EVT", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "event task"})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Seq != 1 || queued[0].EventType != RunEventQueued {
		t.Fatalf("bad queued event: %#v", queued)
	}
	if queued[0].Details["trigger_type"] != "issue_created" {
		t.Fatalf("missing queued details: %#v", queued[0].Details)
	}
	if _, err := st.AppendRunEvent(ctx, RunEventInput{RunID: run.ID, EventType: RunEventStarting, Details: map[string]any{"runtime": "codex"}}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Seq != 2 || events[1].Details["runtime"] != "codex" {
		t.Fatalf("bad event roundtrip: %#v", events)
	}
	if _, err := st.AppendRunEvent(ctx, RunEventInput{RunID: "missing", EventType: RunEventStarting}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("append missing run err=%v, want ErrNotFound", err)
	}
	if _, err := st.CancelRun(ctx, run.ID, "user cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteIssue(ctx, issue.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.DB().GetContext(ctx, &count, `SELECT COUNT(*) FROM run_event WHERE run_id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("run_event cascade left %d row(s)", count)
	}
}

func TestRunFailureAndStaleLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Lifecycle", Slug: "lifecycle", IdentifierPrefix: "LIF", MainAgent: CreateAgentInput{Name: "Runner", Runtime: "codex", Instructions: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	_, failedRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "failure task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim failure run ok=%v err=%v", ok, err)
	}
	failed, err := st.CompleteRun(ctx, failedRun.ID, 2, "", "", false, "exit code 2")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.TerminalReason != TerminalReasonExitNonzero || failed.FailureKind != FailureKindExitNonzero {
		t.Fatalf("bad failed lifecycle fields: %#v", failed)
	}
	failedEvents, err := st.ListRunEvents(ctx, failedRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := failedEvents[len(failedEvents)-1].EventType; got != RunEventFailed {
		t.Fatalf("last failed event=%s, want %s", got, RunEventFailed)
	}

	_, staleRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "stale task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim stale run ok=%v err=%v", ok, err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE run SET heartbeat_at='2000-01-01T00:00:00Z' WHERE id=?`, staleRun.ID); err != nil {
		t.Fatal(err)
	}
	n, err := st.RecoverStaleRuns(ctx, "2020-01-01T00:00:00Z", []string{staleRun.ID})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("excluded active run recovered count=%d, want 0", n)
	}
	n, err = st.RecoverStaleRuns(ctx, "2020-01-01T00:00:00Z", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("stale recovered count=%d, want 1", n)
	}
	stale, err := st.GetRun(ctx, staleRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "cancelled" || stale.TerminalReason != TerminalReasonStaleRecovered || stale.CancelReason != CancelReasonStale {
		t.Fatalf("bad stale lifecycle fields: %#v", stale)
	}
}

func TestAutopilotTriggerFailureVisibilityFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        CreateAgentInput{Name: "NewsLead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} 뉴스",
		IssueBodyTemplate:  "body",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rule.LastError != "" || rule.ConsecutiveFailures != 0 || rule.LastTriggeredIssueID != "" {
		t.Fatalf("new rule should get visibility defaults: %#v", rule)
	}
	listed, err := st.ListAutopilotRules(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(listed) != 1 || listed[0].ConsecutiveFailures != 0 || listed[0].LastError != "" {
		t.Fatalf("list should scan visibility fields: %#v", listed)
	}

	longErr := errors.New(strings.Repeat("x", 5000))
	failed, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, longErr, "2026-05-15T00:00:00Z")
	if err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if failed.ConsecutiveFailures != 1 || len(failed.LastError) != 4000 || failed.NextRunAt == "" {
		t.Fatalf("failure state not recorded/capped: %#v", failed)
	}

	issue, run, err := st.TriggerAutopilotRuleWithContent(ctx, rule.ID, "Triggered", "body")
	if err != nil {
		t.Fatalf("trigger with content: %v", err)
	}
	triggered, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if triggered.LastTriggeredIssueID != issue.ID || triggered.ConsecutiveFailures != 0 || triggered.LastError != "" {
		t.Fatalf("success should clear failure state and remember issue: %#v", triggered)
	}
	claimed, ok, err := st.ClaimNextRun(ctx, "worker")
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("claimed run=%s, want %s", claimed.ID, run.ID)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "done", false, ""); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	if err := st.DeleteIssue(ctx, issue.ID); err != nil {
		t.Fatalf("delete issue: %v", err)
	}
	afterDelete, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterDelete.LastTriggeredIssueID != "" {
		t.Fatalf("issue delete should null last_triggered_issue_id: %#v", afterDelete)
	}
}

func TestAddCommentDispatchesUnicodeMention(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Unicode",
		Slug:             "unicode",
		IdentifierPrefix: "UNI",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "ライター", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatal(err)
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "unicode mention"})
	if err != nil {
		t.Fatal(err)
	}
	// Complete initial run so the mention-created run is the only queued run for this agent.
	claimed, ok, err := st.ClaimNextRun(ctx, "worker")
	if err != nil || !ok {
		t.Fatalf("claim initial ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, claimed.ID, 0, "", "done", false, ""); err != nil {
		t.Fatal(err)
	}
	result, err := st.AddUserComment(ctx, issue.ID, "@ライター この記事을 정리해줘")
	if err != nil {
		t.Fatal(err)
	}
	if result.DispatchedRun == nil || result.DispatchedRun.AgentID != agent.ID {
		t.Fatalf("unicode mention did not dispatch to agent: %#v", result.DispatchedRun)
	}
}

func TestSubIssueAndAutoChainOptIn(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "Chain", Slug: "chain", IdentifierPrefix: "CHN", AutoChainEnabled: true, MainAgent: CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"})
	if err != nil {
		t.Fatal(err)
	}
	parent, parentRun, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	sub, subRun, err := st.CreateSubIssue(ctx, parent.ID, CreateIssueInput{Title: "child", AssigneeAgentID: writer.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sub.ParentIssueID != parent.ID || subRun.AgentID != writer.ID {
		t.Fatalf("bad subissue=%#v run=%#v", sub, subRun)
	}
	subs, err := st.ListSubIssues(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].ID != sub.ID {
		t.Fatalf("subs=%#v", subs)
	}

	claimed, ok, err := st.ClaimNextRun(ctx, "worker")
	if err != nil || !ok {
		t.Fatalf("claim parent ok=%v err=%v", ok, err)
	}
	if claimed.AgentID != main.ID || claimed.ID != parentRun.ID {
		t.Fatalf("unexpected claim=%#v", claimed)
	}
	if _, err := st.CompleteRun(ctx, parentRun.ID, 0, "", "@Writer draft this", false, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs=%#v", runs)
	}
	chained := runs[1]
	if chained.AgentID != writer.ID || chained.ParentRunID != parentRun.ID || chained.ChainID != parentRun.ID || chained.ChainDepth != 1 {
		t.Fatalf("bad chained run=%#v", chained)
	}
}

func TestAutoChainDisabledByDefault(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{Name: "No Chain", Slug: "no-chain", IdentifierPrefix: "NCH", MainAgent: CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"}); err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "@Writer draft this", false, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("auto-chain should be disabled by default, runs=%#v", runs)
	}
}

func TestAutoChainGuardsDryRunAndDailyRunLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:                   "Guarded Chain",
		Slug:                   "guarded-chain",
		IdentifierPrefix:       "GRD",
		AutoChainEnabled:       true,
		AutoChainDryRun:        true,
		AutoChainDailyRunLimit: intPtr(1),
		MainAgent:              CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(ctx, ws.ID, CreateAgentInput{Name: "Writer", Runtime: "codex", Instructions: "write"}); err != nil {
		t.Fatal(err)
	}
	issue, run, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run.ID, 0, "", "@Writer draft", false, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("dry-run should not enqueue a chained run: %#v", runs)
	}
	comments, err := st.ListComments(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) == 0 || !strings.Contains(comments[len(comments)-1].Content, "dry-run") {
		t.Fatalf("dry-run system comment missing: %#v", comments)
	}

	dryRunOff := false
	if _, err := st.UpdateWorkspace(ctx, ws.ID, UpdateWorkspaceInput{Name: ws.Name, Description: ws.Description, WorkingDir: ws.WorkingDir, OutputDir: ws.OutputDir, AutoChainDryRun: &dryRunOff}); err != nil {
		t.Fatal(err)
	}
	issue2, run2, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "limit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim issue2 ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run2.ID, 0, "", "@Writer first", false, ""); err != nil {
		t.Fatal(err)
	}
	if runs, err = st.ListRuns(ctx, issue2.ID); err != nil || len(runs) != 2 {
		t.Fatalf("first chain should enqueue one run runs=%#v err=%v", runs, err)
	}

	issue3, run3, err := st.CreateIssueWithInitialRun(ctx, ws.ID, CreateIssueInput{Title: "limit exceeded"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, "worker"); err != nil || !ok {
		t.Fatalf("claim issue3 ok=%v err=%v", ok, err)
	}
	if _, err := st.CompleteRun(ctx, run3.ID, 0, "", "@Writer second", false, ""); err != nil {
		t.Fatal(err)
	}
	if runs, err = st.ListRuns(ctx, issue3.ID); err != nil || len(runs) != 1 {
		t.Fatalf("daily limit should block second chained run runs=%#v err=%v", runs, err)
	}
}
