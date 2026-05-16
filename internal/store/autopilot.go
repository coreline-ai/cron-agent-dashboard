package store

import (
	"context"
	"errors"
	"fmt"
	appscheduler "github.com/coreline-ai/cron-agent-dashboard/internal/scheduler"
	"github.com/robfig/cron/v3"
	"strings"
	"time"
)

const autopilotSelectBase = `
SELECT ar.id, ar.workspace_id, ar.name, ar.cron_expr, ar.issue_title_template, ar.issue_body_template,
       COALESCE(ar.assignee_agent_id,'') AS assignee_agent_id,
       COALESCE(a.name,'') AS assignee_agent_name,
       ar.enabled,
       COALESCE(ar.last_run_at,'') AS last_run_at,
       COALESCE(ar.next_run_at,'') AS next_run_at,
       COALESCE(ar.snooze_until,'') AS snooze_until,
       COALESCE(ar.last_error,'') AS last_error,
       ar.consecutive_failures,
       COALESCE(ar.last_triggered_issue_id,'') AS last_triggered_issue_id,
       ar.created_at, ar.updated_at
FROM autopilot_rule ar
LEFT JOIN agent a ON a.id = ar.assignee_agent_id`

type UpsertAutopilotInput struct {
	Name               string `json:"name"`
	CronExpr           string `json:"cron_expr"`
	IssueTitleTemplate string `json:"issue_title_template"`
	IssueBodyTemplate  string `json:"issue_body_template"`
	AssigneeAgentID    string `json:"assignee_agent_id"`
	Enabled            bool   `json:"enabled"`
	SnoozeUntil        string `json:"snooze_until"`
	NextRunAt          string `json:"next_run_at"`
}

type AutopilotTriggerResult struct {
	OK    bool          `json:"ok"`
	Rule  AutopilotRule `json:"rule"`
	Issue *Issue        `json:"issue,omitempty"`
	Run   *Run          `json:"run,omitempty"`
	Error string        `json:"error,omitempty"`
}

func validateAutopilot(in UpsertAutopilotInput) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.IssueTitleTemplate) == "" {
		return ErrValidation
	}
	if _, err := cron.ParseStandard(in.CronExpr); err != nil {
		return ErrValidation
	}
	if appscheduler.ValidateTemplate(in.IssueTitleTemplate) != nil || appscheduler.ValidateTemplate(in.IssueBodyTemplate) != nil {
		return ErrValidation
	}
	if _, err := normalizeSnoozeUntil(in.SnoozeUntil); err != nil {
		return ErrValidation
	}
	return nil
}

func (s *Store) ListAutopilotRules(ctx context.Context, workspaceID string) ([]AutopilotRule, error) {
	var out []AutopilotRule
	err := s.db.SelectContext(ctx, &out, autopilotSelectBase+` WHERE ar.workspace_id=? ORDER BY ar.created_at DESC`, workspaceID)
	return out, normalizeErr(err)
}

func (s *Store) ListEnabledAutopilotRules(ctx context.Context) ([]AutopilotRule, error) {
	var out []AutopilotRule
	err := s.db.SelectContext(ctx, &out, autopilotSelectBase+` WHERE ar.enabled=1 ORDER BY ar.created_at ASC`)
	return out, normalizeErr(err)
}

func (s *Store) SetAutopilotNextRun(ctx context.Context, id, nextRunAt string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE autopilot_rule SET next_run_at=?, updated_at=? WHERE id=?`, nullIfEmpty(nextRunAt), now(), id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearDisabledAutopilotNextRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE autopilot_rule SET next_run_at=NULL, updated_at=? WHERE enabled=0 AND next_run_at IS NOT NULL`, now())
	return normalizeErr(err)
}

func (s *Store) RecordAutopilotTriggerSuccess(ctx context.Context, ruleID, issueID, nextRunAt string) (AutopilotRule, error) {
	t := now()
	res, err := s.db.ExecContext(ctx, `UPDATE autopilot_rule
SET last_run_at=?, last_triggered_issue_id=?, consecutive_failures=0, last_error='', next_run_at=?, updated_at=?
WHERE id=?`, t, nullIfEmpty(issueID), nullIfEmpty(nextRunAt), t, ruleID)
	if err != nil {
		return AutopilotRule{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return AutopilotRule{}, ErrNotFound
	}
	return s.GetAutopilotRule(ctx, ruleID)
}

func (s *Store) RecordAutopilotTriggerFailure(ctx context.Context, ruleID string, triggerErr error, nextRunAt string) (AutopilotRule, error) {
	msg := AutopilotTriggerErrorMessage(triggerErr)
	t := now()
	res, err := s.db.ExecContext(ctx, `UPDATE autopilot_rule
SET last_error=?,
    consecutive_failures=consecutive_failures+1,
    enabled=CASE WHEN consecutive_failures+1 >= ? THEN 0 ELSE enabled END,
    next_run_at=CASE WHEN enabled=0 OR consecutive_failures+1 >= ? THEN NULL ELSE ? END,
    updated_at=?
WHERE id=?`, msg, s.autopilotFailureThreshold(), s.autopilotFailureThreshold(), nullIfEmpty(nextRunAt), t, ruleID)
	if err != nil {
		return AutopilotRule{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return AutopilotRule{}, ErrNotFound
	}
	return s.GetAutopilotRule(ctx, ruleID)
}

func (s *Store) CreateAutopilotRule(ctx context.Context, workspaceID string, in UpsertAutopilotInput) (AutopilotRule, error) {
	if err := validateAutopilot(in); err != nil {
		return AutopilotRule{}, err
	}
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return AutopilotRule{}, err
	}
	if err := s.validateAgentInWorkspace(ctx, workspaceID, in.AssigneeAgentID); err != nil {
		return AutopilotRule{}, err
	}
	t := now()
	id := newID()
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	snoozeUntil, err := normalizeSnoozeUntil(in.SnoozeUntil)
	if err != nil {
		return AutopilotRule{}, ErrValidation
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO autopilot_rule(id,workspace_id,name,cron_expr,issue_title_template,issue_body_template,assignee_agent_id,enabled,snooze_until,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, workspaceID, in.Name, in.CronExpr, in.IssueTitleTemplate, in.IssueBodyTemplate, nullIfEmpty(in.AssigneeAgentID), enabled, nullIfEmpty(snoozeUntil), t, t)
	if err != nil {
		return AutopilotRule{}, normalizeErr(err)
	}
	return s.GetAutopilotRule(ctx, id)
}

func (s *Store) GetAutopilotRule(ctx context.Context, id string) (AutopilotRule, error) {
	var r AutopilotRule
	err := s.db.GetContext(ctx, &r, autopilotSelectBase+` WHERE ar.id=?`, id)
	return r, normalizeErr(err)
}

func (s *Store) UpdateAutopilotRule(ctx context.Context, id string, in UpsertAutopilotInput) (AutopilotRule, error) {
	if err := validateAutopilot(in); err != nil {
		return AutopilotRule{}, err
	}
	rule, err := s.GetAutopilotRule(ctx, id)
	if err != nil {
		return AutopilotRule{}, err
	}
	if err := s.validateAgentInWorkspace(ctx, rule.WorkspaceID, in.AssigneeAgentID); err != nil {
		return AutopilotRule{}, err
	}
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	snoozeUntil, err := normalizeSnoozeUntil(in.SnoozeUntil)
	if err != nil {
		return AutopilotRule{}, ErrValidation
	}
	_, err = s.db.ExecContext(ctx, `UPDATE autopilot_rule SET name=?, cron_expr=?, issue_title_template=?, issue_body_template=?, assignee_agent_id=?, enabled=?, snooze_until=?, updated_at=? WHERE id=?`, in.Name, in.CronExpr, in.IssueTitleTemplate, in.IssueBodyTemplate, nullIfEmpty(in.AssigneeAgentID), enabled, nullIfEmpty(snoozeUntil), now(), id)
	if err != nil {
		return AutopilotRule{}, normalizeErr(err)
	}
	return s.GetAutopilotRule(ctx, id)
}

func (s *Store) validateAgentInWorkspace(ctx context.Context, workspaceID, agentID string) error {
	if agentID == "" {
		return nil
	}
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if agent.WorkspaceID != workspaceID {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAutopilotRule(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM autopilot_rule WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TriggerAutopilotRule(ctx context.Context, id string) (Issue, Run, error) {
	result, err := s.TriggerAutopilotRuleResult(ctx, id)
	if err != nil {
		return Issue{}, Run{}, err
	}
	if result.Issue == nil || result.Run == nil {
		return Issue{}, Run{}, ErrState
	}
	return *result.Issue, *result.Run, nil
}

func (s *Store) TriggerAutopilotRuleResult(ctx context.Context, id string) (AutopilotTriggerResult, error) {
	rule, err := s.GetAutopilotRule(ctx, id)
	if err != nil {
		return AutopilotTriggerResult{}, err
	}
	nextRunAt := nextAutopilotRunAtForRule(rule, time.Now())
	return s.triggerAutopilotRuleWithContentResult(ctx, rule, rule.IssueTitleTemplate, rule.IssueBodyTemplate, nextRunAt)
}

func (s *Store) TriggerAutopilotRuleWithContent(ctx context.Context, id, title, body string) (Issue, Run, error) {
	rule, err := s.GetAutopilotRule(ctx, id)
	if err != nil {
		return Issue{}, Run{}, err
	}
	result, err := s.triggerAutopilotRuleWithContentResult(ctx, rule, title, body, nextAutopilotRunAtForRule(rule, time.Now()))
	if err != nil {
		return Issue{}, Run{}, err
	}
	if result.Issue == nil || result.Run == nil {
		return Issue{}, Run{}, ErrState
	}
	return *result.Issue, *result.Run, nil
}

func (s *Store) TriggerAutopilotRuleWithContentResult(ctx context.Context, id, title, body, nextRunAt string) (AutopilotTriggerResult, error) {
	rule, err := s.GetAutopilotRule(ctx, id)
	if err != nil {
		return AutopilotTriggerResult{}, err
	}
	return s.triggerAutopilotRuleWithContentResult(ctx, rule, title, body, nextRunAt)
}

func (s *Store) triggerAutopilotRuleWithContentResult(ctx context.Context, rule AutopilotRule, title, body, nextRunAt string) (AutopilotTriggerResult, error) {
	result := AutopilotTriggerResult{Rule: rule}
	if !rule.Enabled {
		err := fmt.Errorf("%w: autopilot rule is disabled", ErrState)
		result.Error = AutopilotTriggerErrorMessage(err)
		return result, err
	}
	if until, snoozed := AutopilotSnoozedUntil(rule, time.Now()); snoozed {
		err := fmt.Errorf("%w: autopilot rule is snoozed until %s", ErrState, until.UTC().Format(time.RFC3339Nano))
		result.Error = AutopilotTriggerErrorMessage(err)
		return result, err
	}
	issue, run, updatedRule, err := s.createAutopilotIssueRunAndRecordSuccess(ctx, rule, title, body, nextRunAt)
	if err != nil {
		result.Error = AutopilotTriggerErrorMessage(err)
		if recorded, recordErr := s.RecordAutopilotTriggerFailure(ctx, rule.ID, err, nextRunAt); recordErr == nil {
			result.Rule = recorded
		}
		return result, err
	}
	result.OK = true
	result.Rule = updatedRule
	result.Issue = &issue
	result.Run = &run
	return result, nil
}

func (s *Store) createAutopilotIssueRunAndRecordSuccess(ctx context.Context, rule AutopilotRule, title, body, nextRunAt string) (Issue, Run, AutopilotRule, error) {
	if strings.TrimSpace(title) == "" {
		return Issue{}, Run{}, AutopilotRule{}, ErrValidation
	}
	w, _, err := s.GetWorkspace(ctx, rule.WorkspaceID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	agentID := rule.AssigneeAgentID
	if agentID == "" {
		main, err := s.GetMainAgent(ctx, w.ID)
		if err != nil {
			return Issue{}, Run{}, AutopilotRule{}, err
		}
		agentID = main.ID
	} else {
		a, err := s.GetAgent(ctx, agentID)
		if err != nil {
			return Issue{}, Run{}, AutopilotRule{}, err
		}
		if a.WorkspaceID != w.ID {
			return Issue{}, Run{}, AutopilotRule{}, ErrNotFound
		}
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	defer tx.Rollback()
	var nextSeq int64
	if err := tx.GetContext(ctx, &nextSeq, `UPDATE workspace SET next_issue_seq=next_issue_seq+1, updated_at=? WHERE id=? RETURNING next_issue_seq`, now(), w.ID); err != nil {
		return Issue{}, Run{}, AutopilotRule{}, normalizeErr(err)
	}
	seq := nextSeq - 1
	t := now()
	issueID := newID()
	runID := newID()
	identifier := fmt.Sprintf("%s-%d", w.IdentifierPrefix, seq)
	if _, err := tx.ExecContext(ctx, `INSERT INTO issue(id,workspace_id,identifier,title,body,status,assignee_agent_id,created_by,autopilot_rule_id,created_at,updated_at) VALUES(?,?,?,?,?,'open',?,?,?,?,?)`, issueID, w.ID, identifier, title, body, nullIfEmpty(agentID), "autopilot", rule.ID, t, t); err != nil {
		return Issue{}, Run{}, AutopilotRule{}, normalizeErr(err)
	}
	maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agentID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	instructionsVersion, err := agentInstructionsVersionForAgent(ctx, tx, agentID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','autopilot',?,?,?,?,?,0)`, runID, issueID, agentID, capSnapshot(body), t, maxAttempts, instructionsVersion, runID); err != nil {
		return Issue{}, Run{}, AutopilotRule{}, normalizeErr(err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE autopilot_rule
SET last_run_at=?, last_triggered_issue_id=?, consecutive_failures=0, last_error='', next_run_at=?, updated_at=?
WHERE id=?`, t, issueID, nullIfEmpty(nextRunAt), t, rule.ID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return Issue{}, Run{}, AutopilotRule{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	updatedRule, err := s.GetAutopilotRule(ctx, rule.ID)
	if err != nil {
		return Issue{}, Run{}, AutopilotRule{}, err
	}
	return issue, run, updatedRule, nil
}

func AutopilotTriggerErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown autopilot trigger failure"
	}
	return capSnapshot(msg)
}

func nextAutopilotRunAtForRule(rule AutopilotRule, base time.Time) string {
	schedule, err := cron.ParseStandard(rule.CronExpr)
	if err != nil {
		return ""
	}
	anchor := base
	if until, snoozed := AutopilotSnoozedUntil(rule, base); snoozed && until.After(anchor) {
		anchor = until
	}
	return schedule.Next(anchor).UTC().Format(time.RFC3339Nano)
}

func normalizeSnoozeUntil(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", err
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func AutopilotSnoozedUntil(rule AutopilotRule, base time.Time) (time.Time, bool) {
	value := strings.TrimSpace(rule.SnoozeUntil)
	if value == "" {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return until, until.After(base.UTC())
}

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
