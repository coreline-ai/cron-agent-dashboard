package store

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/robfig/cron/v3"
)

var mentionRE = regexp.MustCompile(`@([A-Za-z0-9_\-가-힣]+)`)

const commentSelectBase = `
SELECT c.id, c.issue_id, c.author_type,
       COALESCE(c.author_agent_id,'') AS author_agent_id,
       COALESCE(a.name,'') AS author_agent_name,
       COALESCE(c.run_id,'') AS run_id,
       c.content,
       CASE WHEN c.content LIKE '%전체 로그%' THEN 1 ELSE 0 END AS truncated,
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
			_, _ = tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "멘션이 둘 이상이라 @"+name+"만 적용됩니다", t)
		}
		var agent Agent
		err := tx.GetContext(ctx, &agent, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE workspace_id=? AND lower(name)=lower(?)`, iss.WorkspaceID, name)
		if err != nil {
			warnings = append(warnings, "@"+name+" not found")
			_, _ = tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, "에이전트 @"+name+"을 찾을 수 없습니다", t)
		} else {
			runID = newID()
			_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_comment_id,trigger_content_snapshot,enqueued_at) VALUES(?,?,?,'queued','mention',?,?,?)`, runID, issueID, agent.ID, commentID, capSnapshot(content), t)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "constraint") {
					warnings = append(warnings, "already queued for @"+agent.Name)
					runID = ""
				} else {
					return AddCommentResult{}, normalizeErr(err)
				}
			} else {
				_, _ = tx.ExecContext(ctx, `UPDATE issue SET status='open', updated_at=? WHERE id=?`, t, issueID)
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
	NextRunAt          string `json:"next_run_at"`
}

func validateAutopilot(in UpsertAutopilotInput) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.IssueTitleTemplate) == "" {
		return ErrValidation
	}
	if _, err := cron.ParseStandard(in.CronExpr); err != nil {
		return ErrValidation
	}
	return nil
}

func (s *Store) ListAutopilotRules(ctx context.Context, workspaceID string) ([]AutopilotRule, error) {
	var out []AutopilotRule
	err := s.db.SelectContext(ctx, &out, autopilotSelectBase+` WHERE ar.workspace_id=? ORDER BY ar.created_at DESC`, workspaceID)
	return out, normalizeErr(err)
}

func (s *Store) CreateAutopilotRule(ctx context.Context, workspaceID string, in UpsertAutopilotInput) (AutopilotRule, error) {
	if err := validateAutopilot(in); err != nil {
		return AutopilotRule{}, err
	}
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return AutopilotRule{}, err
	}
	t := now()
	id := newID()
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO autopilot_rule(id,workspace_id,name,cron_expr,issue_title_template,issue_body_template,assignee_agent_id,enabled,next_run_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, workspaceID, in.Name, in.CronExpr, in.IssueTitleTemplate, in.IssueBodyTemplate, nullIfEmpty(in.AssigneeAgentID), enabled, nullIfEmpty(in.NextRunAt), t, t)
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
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE autopilot_rule SET name=?, cron_expr=?, issue_title_template=?, issue_body_template=?, assignee_agent_id=?, enabled=?, next_run_at=?, updated_at=? WHERE id=?`, in.Name, in.CronExpr, in.IssueTitleTemplate, in.IssueBodyTemplate, nullIfEmpty(in.AssigneeAgentID), enabled, nullIfEmpty(in.NextRunAt), now(), id)
	if err != nil {
		return AutopilotRule{}, normalizeErr(err)
	}
	return s.GetAutopilotRule(ctx, id)
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
	rule, err := s.GetAutopilotRule(ctx, id)
	if err != nil {
		return Issue{}, Run{}, err
	}
	issue, run, err := s.CreateIssueWithInitialRun(ctx, rule.WorkspaceID, CreateIssueInput{Title: rule.IssueTitleTemplate, Body: rule.IssueBodyTemplate, AssigneeAgentID: rule.AssigneeAgentID, CreatedBy: "autopilot", AutopilotRuleID: rule.ID, TriggerType: "autopilot"})
	if err != nil {
		return Issue{}, Run{}, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE autopilot_rule SET last_run_at=?, updated_at=? WHERE id=?`, now(), now(), id)
	return issue, run, nil
}

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
