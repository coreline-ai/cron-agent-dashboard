package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func TestAutopilotRunnerDisabledRuleIsNoop(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ws, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "Autopilot Failure Policy",
		Slug:             "autopilot-failure-policy",
		IdentifierPrefix: "AFP",
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "fake", Instructions: "lead"},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	rule, err := st.CreateAutopilotRule(ctx, ws.ID, store.UpsertAutopilotInput{
		Name:               "daily",
		CronExpr:           "0 9 * * *",
		IssueTitleTemplate: "{{date}} daily",
		IssueBodyTemplate:  "body",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE autopilot_rule
SET enabled=0, consecutive_failures=5, last_error='boom', next_run_at=NULL
WHERE id=?`, rule.ID); err != nil {
		t.Fatalf("disable rule fixture: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Seoul")
	runner := NewAutopilotRunner(st, loc)
	result, err := runner.TriggerRuleResult(ctx, rule.ID)
	if !errors.Is(err, store.ErrState) {
		t.Fatalf("trigger disabled rule err=%v, want ErrState", err)
	}
	if result.OK || result.Issue != nil || result.Run != nil || result.Rule.Enabled {
		t.Fatalf("disabled runner trigger should be a no-op result: %#v", result)
	}
	if got := countAutopilotRunnerFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM issue WHERE workspace_id=?`, ws.ID); got != 0 {
		t.Fatalf("issue count=%d, want 0", got)
	}
	if got := countAutopilotRunnerFailurePolicyRows(t, ctx, st, `SELECT COUNT(*) FROM run`); got != 0 {
		t.Fatalf("run count=%d, want 0", got)
	}
}

func countAutopilotRunnerFailurePolicyRows(t *testing.T, ctx context.Context, st *store.Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := st.DB().GetContext(ctx, &count, query, args...); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
