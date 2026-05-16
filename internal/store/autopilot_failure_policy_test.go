package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
)

func TestAutopilotFailurePolicyDisablesAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, rule := createAutopilotFailurePolicyFixture(t, ctx, st)
	nextRunAt := "2026-05-15T00:00:00Z"

	for i := 1; i <= 4; i++ {
		updated, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("boom"), nextRunAt)
		if err != nil {
			t.Fatalf("record failure %d: %v", i, err)
		}
		if !updated.Enabled {
			t.Fatalf("rule disabled after %d failures, want enabled: %#v", i, updated)
		}
		if updated.ConsecutiveFailures != i || updated.LastError != "boom" || updated.NextRunAt != nextRunAt {
			t.Fatalf("bad failure state after %d failures: %#v", i, updated)
		}
	}

	disabled, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("boom"), nextRunAt)
	if err != nil {
		t.Fatalf("record fifth failure: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("rule should be disabled on fifth consecutive failure: %#v", disabled)
	}
	if disabled.ConsecutiveFailures != 5 || disabled.LastError != "boom" || disabled.NextRunAt != "" {
		t.Fatalf("bad disabled failure state: %#v", disabled)
	}
}

func TestAutopilotFailurePolicyThresholdCanBeConfigured(t *testing.T) {
	ctx := context.Background()
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	st := New(database, WithAutopilotFailureDisableThreshold(2))
	_, rule := createAutopilotFailurePolicyFixture(t, ctx, st)
	nextRunAt := "2026-05-15T00:00:00Z"

	first, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("boom"), nextRunAt)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Enabled {
		t.Fatalf("rule disabled after first failure, want enabled: %#v", first)
	}
	second, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("boom"), nextRunAt)
	if err != nil {
		t.Fatal(err)
	}
	if second.Enabled || second.ConsecutiveFailures != 2 || second.NextRunAt != "" {
		t.Fatalf("custom threshold should disable on second failure: %#v", second)
	}
}

func TestDisabledAutopilotRuleTriggerDoesNotCreateIssueOrRun(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, rule := createAutopilotFailurePolicyFixture(t, ctx, st)
	nextRunAt := "2026-05-15T00:00:00Z"

	for i := 0; i < 5; i++ {
		if _, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("boom"), nextRunAt); err != nil {
			t.Fatalf("record failure %d: %v", i+1, err)
		}
	}
	beforeIssues := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM issue WHERE workspace_id=?`, ws.ID)
	beforeRuns := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM run`)

	result, err := st.TriggerAutopilotRuleWithContentResult(ctx, rule.ID, "should not run", "body", nextRunAt)
	if !errors.Is(err, ErrState) {
		t.Fatalf("trigger disabled rule err=%v, want ErrState", err)
	}
	if result.OK || result.Issue != nil || result.Run != nil || result.Rule.Enabled {
		t.Fatalf("disabled trigger should be a no-op result: %#v", result)
	}
	if got := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM issue WHERE workspace_id=?`, ws.ID); got != beforeIssues {
		t.Fatalf("issue count=%d, want %d", got, beforeIssues)
	}
	if got := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM run`); got != beforeRuns {
		t.Fatalf("run count=%d, want %d", got, beforeRuns)
	}
	after, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ConsecutiveFailures != 5 || after.NextRunAt != "" {
		t.Fatalf("disabled no-op should not mutate failure state: %#v", after)
	}
}

func TestAutopilotTriggerSuccessResetsFailureState(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	_, rule := createAutopilotFailurePolicyFixture(t, ctx, st)

	for i := 0; i < 2; i++ {
		if _, err := st.RecordAutopilotTriggerFailure(ctx, rule.ID, errors.New("temporary failure"), "2026-05-15T00:00:00Z"); err != nil {
			t.Fatalf("record failure %d: %v", i+1, err)
		}
	}
	issue, run, err := st.TriggerAutopilotRuleWithContent(ctx, rule.ID, "Recovered", "body")
	if err != nil {
		t.Fatalf("trigger after failures: %v", err)
	}
	if run.TriggerType != "autopilot" || run.IssueID != issue.ID {
		t.Fatalf("bad created run: %#v issue=%#v", run, issue)
	}
	updated, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.ConsecutiveFailures != 0 || updated.LastError != "" || updated.LastTriggeredIssueID != issue.ID {
		t.Fatalf("success should reset failure state and preserve enabled rule: %#v", updated)
	}
}

func createAutopilotFailurePolicyFixture(t *testing.T, ctx context.Context, st *Store) (Workspace, AutopilotRule) {
	t.Helper()
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Autopilot Failure Policy",
		Slug:             "autopilot-failure-policy",
		IdentifierPrefix: "AFP",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} daily",
		IssueBodyTemplate:  "body",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	return ws, rule
}

func countAutopilotFailurePolicyRows(t *testing.T, ctx context.Context, st *Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := st.DB().GetContext(ctx, &count, query, args...); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestSnoozedAutopilotRuleTriggerDoesNotCreateIssueOrCountFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, rule := createAutopilotFailurePolicyFixture(t, ctx, st)
	until := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339Nano)
	updated, err := st.UpdateAutopilotRule(ctx, rule.ID, UpsertAutopilotInput{
		Name:               rule.Name,
		CronExpr:           rule.CronExpr,
		IssueTitleTemplate: rule.IssueTitleTemplate,
		IssueBodyTemplate:  rule.IssueBodyTemplate,
		AssigneeAgentID:    rule.AssigneeAgentID,
		Enabled:            true,
		SnoozeUntil:        until,
	})
	if err != nil {
		t.Fatalf("snooze rule: %v", err)
	}
	if updated.SnoozeUntil == "" {
		t.Fatalf("snooze_until should be persisted: %#v", updated)
	}
	beforeIssues := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM issue WHERE workspace_id=?`, ws.ID)
	beforeRuns := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM run`)

	result, err := st.TriggerAutopilotRuleWithContentResult(ctx, rule.ID, "should not run", "body", "2026-05-15T00:00:00Z")
	if !errors.Is(err, ErrState) {
		t.Fatalf("trigger snoozed rule err=%v, want ErrState", err)
	}
	if result.OK || result.Issue != nil || result.Run != nil || result.Error == "" {
		t.Fatalf("snoozed trigger should be a no-op result: %#v", result)
	}
	if got := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM issue WHERE workspace_id=?`, ws.ID); got != beforeIssues {
		t.Fatalf("issue count=%d, want %d", got, beforeIssues)
	}
	if got := countAutopilotFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM run`); got != beforeRuns {
		t.Fatalf("run count=%d, want %d", got, beforeRuns)
	}
	after, err := st.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ConsecutiveFailures != 0 || after.LastError != "" {
		t.Fatalf("snooze no-op should not count failure: %#v", after)
	}
}
