package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	appscheduler "github.com/coreline-ai/corn-agent-dashboard/internal/scheduler"
	"github.com/robfig/cron/v3"
)

var mentionRE = regexp.MustCompile(`@([\p{L}\p{N}_\-]+)`)

const commentSelectBase = `
SELECT c.id, c.issue_id, c.author_type,
       COALESCE(c.author_agent_id,'') AS author_agent_id,
       COALESCE(a.name,'') AS author_agent_name,
       COALESCE(c.run_id,'') AS run_id,
       c.content,
       c.truncated,
       CASE WHEN c.run_id IS NOT NULL THEN '/api/runs/' || c.run_id || '/log' ELSE '' END AS log_url,
       c.created_at
FROM comment c
LEFT JOIN agent a ON a.id = c.author_agent_id`

type AddCommentResult struct {
	Comment         Comment  `json:"comment"`
	MentionWarnings []string `json:"mention_warnings"`
	DispatchedRun   *Run     `json:"dispatched_run,omitempty"`
}

func (s *Store) ListComments(ctx context.Context, issueID string) ([]Comment, error) {
	var out []Comment
	err := s.db.SelectContext(ctx, &out, commentSelectBase+` WHERE c.issue_id=? ORDER BY c.created_at ASC`, issueID)
	return out, normalizeErr(err)
}

func (s *Store) AddUserComment(ctx context.Context, issueID, content string) (AddCommentResult, error) {
	if strings.TrimSpace(content) == "" {
		return AddCommentResult{}, ErrValidation
	}
	iss, err := s.GetIssue(ctx, issueID)
	if err != nil {
		return AddCommentResult{}, err
	}
	mentions := mentionRE.FindAllStringSubmatch(content, -1)
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return AddCommentResult{}, err
	}
	defer tx.Rollback()
	t := now()
	commentID := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'user',?,?)`, commentID, issueID, content, t)
	if err != nil {
		return AddCommentResult{}, normalizeErr(err)
	}
	warnings := []string{}
	var runID string
	if len(mentions) > 0 {
		name := mentions[0][1]
		if len(mentions) > 1 {
			warnings = append(warnings, "multiple mentions, only @"+name+" applied")
			if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "멘션이 둘 이상이라 @"+name+"만 적용됩니다", t); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
		}
		var agent Agent
		err := tx.GetContext(ctx, &agent, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE workspace_id=? AND lower(name)=lower(?)`, iss.WorkspaceID, name)
		if err != nil {
			warnings = append(warnings, "@"+name+" not found")
			if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "에이전트 @"+name+"을 찾을 수 없습니다", t); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
		} else {
			var existingQueued int
			if err := tx.GetContext(ctx, &existingQueued, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND status='queued'`, issueID, agent.ID); err != nil {
				return AddCommentResult{}, normalizeErr(err)
			}
			if existingQueued > 0 {
				warnings = append(warnings, "already queued for @"+agent.Name)
			} else {
				runID = newID()
				maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agent.ID)
				if err != nil {
					return AddCommentResult{}, err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_comment_id,trigger_content_snapshot,enqueued_at,max_attempts,chain_id,chain_depth) VALUES(?,?,?,'queued','mention',?,?,?,?,?,0)`, runID, issueID, agent.ID, commentID, capSnapshot(content), t, maxAttempts, runID); err != nil {
					return AddCommentResult{}, normalizeErr(err)
				}
				if _, err := appendRunEventTx(ctx, tx, RunEventInput{
					RunID:     runID,
					IssueID:   issueID,
					EventType: RunEventQueued,
					Message:   "Run queued by mention",
					Details: map[string]any{
						"trigger_type":       "mention",
						"trigger_comment_id": commentID,
					},
				}); err != nil {
					return AddCommentResult{}, err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='open', updated_at=? WHERE id=?`, t, issueID); err != nil {
					return AddCommentResult{}, normalizeErr(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return AddCommentResult{}, err
	}
	comments, err := s.ListComments(ctx, issueID)
	if err != nil {
		return AddCommentResult{}, err
	}
	res := AddCommentResult{MentionWarnings: warnings}
	for _, c := range comments {
		if c.ID == commentID {
			res.Comment = c
			break
		}
	}
	if runID != "" {
		r, err := s.GetRun(ctx, runID)
		if err != nil {
			return AddCommentResult{}, err
		}
		res.DispatchedRun = &r
	}
	return res, nil
}

func (s *Store) DeleteComment(ctx context.Context, id string) error {
	var c Comment
	if err := s.db.GetContext(ctx, &c, commentSelectBase+` WHERE c.id=?`, id); err != nil {
		return normalizeErr(err)
	}
	if c.AuthorType != "user" {
		return ErrState
	}
	var refs int
	if err := s.db.GetContext(ctx, &refs, `SELECT COUNT(*) FROM run WHERE trigger_comment_id=? AND status IN ('queued','running')`, id); err != nil {
		return err
	}
	if refs > 0 {
		return ErrState
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM comment WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

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

const autopilotFailureDisableThreshold = 5

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
WHERE id=?`, msg, autopilotFailureDisableThreshold, autopilotFailureDisableThreshold, nullIfEmpty(nextRunAt), t, ruleID)
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,chain_id,chain_depth) VALUES(?,?,?,'queued','autopilot',?,?,?,?,0)`, runID, issueID, agentID, capSnapshot(body), t, maxAttempts, runID); err != nil {
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
