package store

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"strings"
)

// issueExecutionStatusExpr is the SQL fragment that derives the
// `execution_status` column from the latest run state. It must stay in sync
// with the AS execution_status projection in issueSelectBase below so the
// expression can also be referenced from WHERE clauses (e.g. ListIssues'
// execution filter).
const issueExecutionStatusExpr = `COALESCE((SELECT status FROM run WHERE issue_id=i.id AND status='running' LIMIT 1),
                (SELECT status FROM run WHERE issue_id=i.id AND status='queued' ORDER BY enqueued_at ASC LIMIT 1),
                (SELECT status FROM run WHERE issue_id=i.id ORDER BY enqueued_at DESC LIMIT 1),
                'idle')`

const issueSelectBase = `
SELECT i.id, i.workspace_id, i.identifier, i.title, i.body, i.status,
       COALESCE(i.assignee_agent_id, '') AS assignee_agent_id,
       COALESCE(aa.name, '') AS assignee_agent_name,
       COALESCE(i.parent_issue_id, '') AS parent_issue_id,
       i.created_by,
       COALESCE(i.autopilot_rule_id, '') AS autopilot_rule_id,
       i.timeout_seconds_override,
       ` + issueExecutionStatusExpr + ` AS execution_status,
       COALESCE((SELECT agent_id FROM run WHERE issue_id=i.id ORDER BY enqueued_at DESC LIMIT 1), '') AS last_run_agent_id,
       COALESCE((SELECT a2.name FROM run r2 JOIN agent a2 ON a2.id=r2.agent_id WHERE r2.issue_id=i.id ORDER BY r2.enqueued_at DESC LIMIT 1), '') AS last_run_agent_name,
       (SELECT COUNT(*) FROM comment c WHERE c.issue_id=i.id) AS comment_count,
       i.created_at, i.updated_at
FROM issue i
LEFT JOIN agent aa ON aa.id = i.assignee_agent_id`

func (s *Store) CreateIssueWithInitialRun(ctx context.Context, workspaceID string, in CreateIssueInput) (Issue, Run, error) {
	if strings.TrimSpace(in.Title) == "" {
		return Issue{}, Run{}, ErrValidation
	}
	w, _, err := s.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return Issue{}, Run{}, err
	}
	agentID := in.AssigneeAgentID
	if agentID == "" {
		main, err := s.GetMainAgent(ctx, w.ID)
		if err != nil {
			return Issue{}, Run{}, err
		}
		agentID = main.ID
	} else {
		a, err := s.GetAgent(ctx, agentID)
		if err != nil {
			return Issue{}, Run{}, err
		}
		if a.WorkspaceID != w.ID {
			return Issue{}, Run{}, ErrNotFound
		}
	}
	createdBy := in.CreatedBy
	if createdBy == "" {
		createdBy = "user"
	}
	trigger := in.TriggerType
	if trigger == "" {
		trigger = "issue_created"
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Issue{}, Run{}, err
	}
	defer tx.Rollback()
	var nextSeq int64
	if err := tx.GetContext(ctx, &nextSeq, `UPDATE workspace SET next_issue_seq=next_issue_seq+1, updated_at=? WHERE id=? RETURNING next_issue_seq`, now(), w.ID); err != nil {
		return Issue{}, Run{}, normalizeErr(err)
	}
	seq := nextSeq - 1
	t := now()
	issueID := newID()
	identifier := fmt.Sprintf("%s-%d", w.IdentifierPrefix, seq)
	if strings.TrimSpace(in.ParentIssueID) != "" {
		var parentWorkspaceID string
		if err := tx.GetContext(ctx, &parentWorkspaceID, `SELECT workspace_id FROM issue WHERE id=?`, in.ParentIssueID); err != nil {
			return Issue{}, Run{}, normalizeErr(err)
		}
		if parentWorkspaceID != w.ID {
			return Issue{}, Run{}, ErrNotFound
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO issue(id,workspace_id,identifier,title,body,status,assignee_agent_id,parent_issue_id,created_by,autopilot_rule_id,created_at,updated_at) VALUES(?,?,?,?,?,'open',?,?,?,?,?,?)`, issueID, w.ID, identifier, in.Title, in.Body, nullIfEmpty(agentID), nullIfEmpty(in.ParentIssueID), createdBy, nullIfEmpty(in.AutopilotRuleID), t, t)
	if err != nil {
		return Issue{}, Run{}, normalizeErr(err)
	}
	runID := newID()
	maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agentID)
	if err != nil {
		return Issue{}, Run{}, err
	}
	instructionsVersion, err := agentInstructionsVersionForAgent(ctx, tx, agentID)
	if err != nil {
		return Issue{}, Run{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued',?,?,?,?,?,?,0)`, runID, issueID, agentID, trigger, capSnapshot(in.Body), t, maxAttempts, instructionsVersion, runID)
	if err != nil {
		return Issue{}, Run{}, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     runID,
		IssueID:   issueID,
		EventType: RunEventQueued,
		Message:   "Run queued by " + trigger,
		Details: map[string]any{
			"trigger_type": trigger,
			"created_by":   createdBy,
		},
	}); err != nil {
		return Issue{}, Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, Run{}, err
	}
	issue, err := s.GetIssue(ctx, issueID)
	if err != nil {
		return Issue{}, Run{}, err
	}
	run, err := s.GetRun(ctx, runID)
	return issue, run, err
}

func (s *Store) ListIssues(ctx context.Context, workspaceID string, f ListIssuesFilter) ([]Issue, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	args := []any{workspaceID}
	where := []string{"i.workspace_id=?"}
	if len(f.Status) > 0 {
		where = append(where, `i.status IN (`+placeholders(len(f.Status))+`)`)
		for _, v := range f.Status {
			args = append(args, v)
		}
	}
	if f.Assignee != "" {
		where = append(where, `i.assignee_agent_id=?`)
		args = append(args, f.Assignee)
	}
	if f.Query != "" {
		where = append(where, `(i.title LIKE ? OR i.body LIKE ? OR i.identifier LIKE ?)`)
		like := "%" + f.Query + "%"
		args = append(args, like, like, like)
	}
	if len(f.Execution) > 0 {
		where = append(where, issueExecutionStatusExpr+` IN (`+placeholders(len(f.Execution))+`)`)
		for _, v := range f.Execution {
			args = append(args, v)
		}
	}
	q := issueSelectBase + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY i.created_at DESC LIMIT ?`
	args = append(args, f.Limit)
	var out []Issue
	if err := s.db.SelectContext(ctx, &out, q, args...); err != nil {
		return nil, normalizeErr(err)
	}
	return out, nil
}

func (s *Store) GetIssue(ctx context.Context, id string) (Issue, error) {
	var out Issue
	err := s.db.GetContext(ctx, &out, issueSelectBase+` WHERE i.id=?`, id)
	return out, normalizeErr(err)
}

func (s *Store) ListSubIssues(ctx context.Context, parentIssueID string) ([]Issue, error) {
	parent, err := s.GetIssue(ctx, parentIssueID)
	if err != nil {
		return nil, err
	}
	var out []Issue
	err = s.db.SelectContext(ctx, &out, issueSelectBase+` WHERE i.parent_issue_id=? AND i.workspace_id=? ORDER BY i.created_at ASC`, parent.ID, parent.WorkspaceID)
	return out, normalizeErr(err)
}

func (s *Store) CreateSubIssue(ctx context.Context, parentIssueID string, in CreateIssueInput) (Issue, Run, error) {
	parent, err := s.GetIssue(ctx, parentIssueID)
	if err != nil {
		return Issue{}, Run{}, err
	}
	in.ParentIssueID = parent.ID
	return s.CreateIssueWithInitialRun(ctx, parent.WorkspaceID, in)
}

func (s *Store) LookupIssue(ctx context.Context, workspaceID, idOrIdentifier string) (Issue, error) {
	var out Issue
	where := ` WHERE i.workspace_id=? AND i.identifier=?`
	args := []any{workspaceID, idOrIdentifier}
	if uuidRE.MatchString(idOrIdentifier) {
		where = ` WHERE i.workspace_id=? AND i.id=?`
	}
	err := s.db.GetContext(ctx, &out, issueSelectBase+where, args...)
	return out, normalizeErr(err)
}

func (s *Store) UpdateIssue(ctx context.Context, id string, in UpdateIssueInput) (Issue, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Issue{}, err
	}
	defer tx.Rollback()

	iss, err := txGetIssue(ctx, tx, id)
	if err != nil {
		return Issue{}, err
	}
	title := iss.Title
	if in.Title != nil {
		title = *in.Title
	}
	body := iss.Body
	if in.Body != nil {
		body = *in.Body
	}
	assigneeAgentID := iss.AssigneeAgentID
	if in.AssigneeAgentID != nil {
		assigneeAgentID = *in.AssigneeAgentID
	}
	if assigneeAgentID != "" {
		var a Agent
		err := tx.GetContext(ctx, &a, agentSelectBase+` WHERE id=?`, assigneeAgentID)
		if err != nil {
			return Issue{}, normalizeErr(err)
		}
		if a.WorkspaceID != iss.WorkspaceID {
			return Issue{}, ErrNotFound
		}
	}
	status := iss.Status
	if in.Status != nil {
		status = *in.Status
	}
	if status != "open" && status != "done" && status != "cancelled" {
		return Issue{}, ErrValidation
	}
	if status == "done" {
		var activeRuns int
		if err := tx.GetContext(ctx, &activeRuns, `SELECT COUNT(*) FROM run WHERE issue_id=? AND status IN ('queued','running')`, id); err != nil {
			return Issue{}, err
		}
		if activeRuns > 0 {
			return Issue{}, ErrState
		}
	}
	if status == "cancelled" {
		var runningRuns int
		if err := tx.GetContext(ctx, &runningRuns, `SELECT COUNT(*) FROM run WHERE issue_id=? AND status='running'`, id); err != nil {
			return Issue{}, err
		}
		if runningRuns > 0 {
			return Issue{}, ErrState
		}
	}
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE issue SET title=?, body=?, assignee_agent_id=?, status=?, updated_at=? WHERE id=?`, title, body, nullIfEmpty(assigneeAgentID), status, t, id)
	if err != nil {
		return Issue{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return Issue{}, ErrNotFound
	}
	if status == "cancelled" {
		var queuedRuns []struct {
			ID string `db:"id"`
		}
		if err := tx.SelectContext(ctx, &queuedRuns, `SELECT id FROM run WHERE issue_id=? AND status='queued' ORDER BY enqueued_at ASC`, id); err != nil {
			return Issue{}, normalizeErr(err)
		}
		for _, queued := range queuedRuns {
			if _, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', exit_code=-1, finished_at=?, error_message='issue cancelled', terminal_reason=?, cancel_reason=? WHERE id=? AND status='queued'`, t, TerminalReasonIssueCancelled, CancelReasonIssue, queued.ID); err != nil {
				return Issue{}, normalizeErr(err)
			}
			if _, err := appendRunEventTx(ctx, tx, RunEventInput{
				RunID:     queued.ID,
				IssueID:   id,
				EventType: RunEventCancelled,
				Message:   "Run cancelled because issue was cancelled",
				Details: map[string]any{
					"terminal_reason": TerminalReasonIssueCancelled,
					"cancel_reason":   CancelReasonIssue,
				},
			}); err != nil {
				return Issue{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system','이슈가 취소되었습니다',?)`, newID(), id, t); err != nil {
			return Issue{}, normalizeErr(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, err
	}
	return s.GetIssue(ctx, id)
}

func (s *Store) DeleteIssue(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.GetContext(ctx, &active, `SELECT COUNT(*) FROM run WHERE issue_id=? AND status IN ('queued','running')`, id); err != nil {
		return normalizeErr(err)
	}
	if active > 0 {
		return ErrState
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM issue WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) RerunIssue(ctx context.Context, issueID, agentID string) (Run, error) {
	iss, err := s.GetIssue(ctx, issueID)
	if err != nil {
		return Run{}, err
	}
	if iss.ExecutionStatus == "running" || iss.ExecutionStatus == "queued" {
		return Run{}, ErrState
	}
	var last Run
	if agentID == "" {
		err = s.db.GetContext(ctx, &last, runSelectBase+` WHERE r.issue_id=? ORDER BY r.enqueued_at DESC LIMIT 1`, issueID)
		if err != nil {
			return Run{}, normalizeErr(err)
		}
		agentID = last.AgentID
	} else {
		a, err := s.GetAgent(ctx, agentID)
		if err != nil {
			return Run{}, err
		}
		if a.WorkspaceID != iss.WorkspaceID {
			return Run{}, ErrNotFound
		}
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	runID := newID()
	t := now()
	snapshot := "[rerun]"
	if last.ID != "" {
		snapshot = "[rerun of run " + last.ID + "]"
	}
	maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agentID)
	if err != nil {
		return Run{}, err
	}
	instructionsVersion, err := agentInstructionsVersionForAgent(ctx, tx, agentID)
	if err != nil {
		return Run{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,chain_id,chain_depth) VALUES(?,?,?,'queued','rerun',?,?,?,?,?,0)`, runID, issueID, agentID, snapshot, t, maxAttempts, instructionsVersion, runID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     runID,
		IssueID:   issueID,
		EventType: RunEventQueued,
		Message:   "Run queued by rerun",
		Details: map[string]any{
			"trigger_type": "rerun",
		},
	}); err != nil {
		return Run{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE issue SET status='open', updated_at=? WHERE id=?`, t, issueID)
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func placeholders(n int) string { return strings.TrimRight(strings.Repeat("?,", n), ",") }

func txGetIssue(ctx context.Context, tx *sqlx.Tx, id string) (Issue, error) {
	var out Issue
	err := tx.GetContext(ctx, &out, issueSelectBase+` WHERE i.id=?`, id)
	return out, normalizeErr(err)
}
