package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
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

	issue, run, err := runner.TriggerRuleResult(ctx, rule.ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
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
	if triggered.LastRunAt == "" || triggered.NextRunAt == "" {
		t.Fatalf("last/next run should be updated after trigger: %#v", triggered)
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
