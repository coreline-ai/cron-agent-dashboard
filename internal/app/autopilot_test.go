package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestAutopilotRunnerRendersTemplateAndCreatesRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} 뉴스",
		IssueBodyTemplate:  "{{datetime}} 기준",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Seoul")
	runner := NewAutopilotRunner(st, loc)
	runner.now = func() time.Time {
		return time.Date(2026, 5, 12, 9, 30, 0, 0, loc)
	}

	if err := runner.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	reloaded, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.NextRunAt == "" {
		t.Fatalf("next_run_at should be synced after reload: %#v", reloaded)
	}

	result, err := runner.TriggerRuleResult(ctx, rule.ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if !result.OK || result.Issue == nil || result.Run == nil {
		t.Fatalf("bad trigger result: %#v", result)
	}
	issue, run := *result.Issue, *result.Run
	if issue.Title != "2026-05-12 뉴스" || issue.Body != "2026-05-12 09:30 기준" {
		t.Fatalf("unexpected rendered issue: %#v", issue)
	}
	if run.Status != "queued" || run.TriggerType != "autopilot" {
		t.Fatalf("unexpected run: %#v", run)
	}
	triggered, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if triggered.LastRunAt == "" || triggered.NextRunAt == "" || triggered.LastTriggeredIssueID != issue.ID || triggered.ConsecutiveFailures != 0 || triggered.LastError != "" {
		t.Fatalf("last/next run should be updated after trigger: %#v", triggered)
	}
}

func TestAutopilotRunnerRecordsTemplateRenderFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} 뉴스",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE autopilot_rule SET issue_title_template='{{workspace}} 뉴스' WHERE id=?`, rule.ID); err != nil {
		t.Fatalf("corrupt template: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Seoul")
	runner := NewAutopilotRunner(st, loc)
	result, err := runner.TriggerRuleResult(ctx, rule.ID)
	if err == nil {
		t.Fatal("trigger should fail")
	}
	if result.OK || result.Rule.ConsecutiveFailures != 1 || !strings.Contains(result.Rule.LastError, "unknown template variable") {
		t.Fatalf("failure should be recorded in result: %#v err=%v", result, err)
	}
	reloaded, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConsecutiveFailures != 1 || reloaded.LastError == "" || reloaded.NextRunAt == "" {
		t.Fatalf("failure state not persisted: %#v", reloaded)
	}
}

func TestAutopilotRuleRejectsUnknownTemplateVariable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "AI News",
		Slug:             "ai-news",
		IdentifierPrefix: "NEWS",
		MainAgent:        store.CreateAgentInput{Name: "NewsLead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	_, err = st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{workspace}} 뉴스",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
	})
	if !errors.Is(err, store.ErrValidation) {
		t.Fatalf("err=%v, want validation", err)
	}
}

func TestAutopilotRunnerSkipsSnoozedRuleAndSyncsNextAfterSnooze(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, main, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Snooze",
		Slug:             "snooze",
		IdentifierPrefix: "SNZ",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Seoul")
	now := time.Date(2026, 5, 12, 9, 30, 0, 0, loc)
	snoozeUntil := time.Date(2026, 5, 14, 10, 0, 0, 0, loc).UTC().Format(time.RFC3339Nano)
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} daily",
		IssueBodyTemplate:  "body",
		AssigneeAgentID:    main.ID,
		Enabled:            true,
		SnoozeUntil:        snoozeUntil,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	runner := NewAutopilotRunner(st, loc)
	runner.now = func() time.Time { return now }
	if err := runner.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	reloaded, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.NextRunAt != "2026-05-15T00:00:00Z" {
		t.Fatalf("next_run_at=%q, want first cron after snooze", reloaded.NextRunAt)
	}
	result, err := runner.TriggerRuleResult(ctx, rule.ID)
	if !errors.Is(err, store.ErrState) {
		t.Fatalf("trigger snoozed err=%v, want ErrState", err)
	}
	if result.OK || result.Issue != nil || result.Run != nil || result.Error == "" {
		t.Fatalf("snoozed trigger should be no-op: %#v", result)
	}
	after, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ConsecutiveFailures != 0 || after.LastError != "" || after.LastTriggeredIssueID != "" {
		t.Fatalf("snoozed trigger should not record failure: %#v", after)
	}
}
