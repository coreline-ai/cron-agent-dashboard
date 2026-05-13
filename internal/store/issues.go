package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
)

const issueSelectBase = `
SELECT i.id, i.workspace_id, i.identifier, i.title, i.body, i.status,
       COALESCE(i.assignee_agent_id, '') AS assignee_agent_id,
       COALESCE(aa.name, '') AS assignee_agent_name,
       i.created_by,
       COALESCE(i.autopilot_rule_id, '') AS autopilot_rule_id,
       COALESCE((SELECT status FROM run WHERE issue_id=i.id AND status='running' LIMIT 1),
                (SELECT status FROM run WHERE issue_id=i.id AND status='queued' ORDER BY enqueued_at ASC LIMIT 1),
                (SELECT status FROM run WHERE issue_id=i.id ORDER BY enqueued_at DESC LIMIT 1),
                'idle') AS execution_status,
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
	_, err = tx.ExecContext(ctx, `INSERT INTO issue(id,workspace_id,identifier,title,body,status,assignee_agent_id,created_by,autopilot_rule_id,created_at,updated_at) VALUES(?,?,?,?,?,'open',?,?,?,?,?)`, issueID, w.ID, identifier, in.Title, in.Body, nullIfEmpty(agentID), createdBy, nullIfEmpty(in.AutopilotRuleID), t, t)
	if err != nil {
		return Issue{}, Run{}, normalizeErr(err)
	}
	runID := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at) VALUES(?,?,?,'queued',?,?,?)`, runID, issueID, agentID, trigger, capSnapshot(in.Body), t)
	if err != nil {
		return Issue{}, Run{}, normalizeErr(err)
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
	q := issueSelectBase + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY i.created_at DESC LIMIT ?`
	args = append(args, f.Limit)
	var out []Issue
	if err := s.db.SelectContext(ctx, &out, q, args...); err != nil {
		return nil, normalizeErr(err)
	}
	if len(f.Execution) == 0 {
		return out, nil
	}
	keep := make(map[string]bool, len(f.Execution))
	for _, e := range f.Execution {
		keep[e] = true
	}
	filtered := out[:0]
	for _, iss := range out {
		if keep[iss.ExecutionStatus] {
			filtered = append(filtered, iss)
		}
	}
	return filtered, nil
}

func (s *Store) GetIssue(ctx context.Context, id string) (Issue, error) {
	var out Issue
	err := s.db.GetContext(ctx, &out, issueSelectBase+` WHERE i.id=?`, id)
	return out, normalizeErr(err)
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
	iss, err := s.GetIssue(ctx, id)
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
		a, err := s.GetAgent(ctx, assigneeAgentID)
		if err != nil {
			return Issue{}, err
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
	if status == "done" || status == "cancelled" {
		var activeRuns int
		if err := s.db.GetContext(ctx, &activeRuns, `SELECT COUNT(*) FROM run WHERE issue_id=? AND status IN ('queued','running')`, id); err != nil {
			return Issue{}, err
		}
		if activeRuns > 0 {
			return Issue{}, ErrState
		}
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Issue{}, err
	}
	defer tx.Rollback()
	t := now()
	_, err = tx.ExecContext(ctx, `UPDATE issue SET title=?, body=?, assignee_agent_id=?, status=?, updated_at=? WHERE id=?`, title, body, nullIfEmpty(assigneeAgentID), status, t, id)
	if err != nil {
		return Issue{}, normalizeErr(err)
	}
	if status == "cancelled" {
		if _, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', exit_code=-1, finished_at=?, error_message='issue cancelled' WHERE issue_id=? AND status='queued'`, t, id); err != nil {
			return Issue{}, normalizeErr(err)
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
	var running int
	if err := s.db.GetContext(ctx, &running, `SELECT COUNT(*) FROM run WHERE issue_id=? AND status='running'`, id); err != nil {
		return normalizeErr(err)
	}
	if running > 0 {
		return ErrState
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM issue WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
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
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at) VALUES(?,?,?,'queued','rerun',?,?)`, runID, issueID, agentID, snapshot, t)
	if err != nil {
		return Run{}, normalizeErr(err)
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

const runSelectBase = `
SELECT r.id, r.issue_id, r.agent_id, COALESCE(a.name,'') AS agent_name, r.status, r.trigger_type,
       COALESCE(r.trigger_comment_id,'') AS trigger_comment_id, r.trigger_content_snapshot,
       r.enqueued_at, COALESCE(r.claimed_at,'') AS claimed_at, r.claimed_by,
       COALESCE(r.started_at,'') AS started_at, COALESCE(r.finished_at,'') AS finished_at,
       r.exit_code, r.stdout_path, r.error_message
FROM run r
LEFT JOIN agent a ON a.id = r.agent_id`

func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	var r Run
	err := s.db.GetContext(ctx, &r, runSelectBase+` WHERE r.id=?`, id)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	decorateRun(&r)
	return r, nil
}

func (s *Store) ListRuns(ctx context.Context, issueID string) ([]Run, error) {
	var out []Run
	err := s.db.SelectContext(ctx, &out, runSelectBase+` WHERE r.issue_id=? ORDER BY r.enqueued_at ASC`, issueID)
	if err != nil {
		return nil, normalizeErr(err)
	}
	for i := range out {
		decorateRun(&out[i])
	}
	return out, nil
}

func (s *Store) ClaimNextRun(ctx context.Context, workerID string) (Run, bool, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return Run{}, false, err
	}
	defer tx.Rollback()
	var runID string
	err = tx.GetContext(ctx, &runID, `SELECT r.id FROM run r JOIN issue i ON i.id=r.issue_id
WHERE r.status='queued'
  AND NOT EXISTS (SELECT 1 FROM run r2 WHERE r2.issue_id=r.issue_id AND r2.status='running')
  AND NOT EXISTS (SELECT 1 FROM run r3 JOIN issue i3 ON i3.id=r3.issue_id WHERE i3.workspace_id=i.workspace_id AND r3.status='running')
ORDER BY r.enqueued_at ASC, r.id ASC LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, normalizeErr(err)
	}
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE run SET status='running', claimed_at=?, claimed_by=?, started_at=? WHERE id=? AND status='queued'`, t, workerID, t, runID)
	if err != nil {
		return Run{}, false, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return Run{}, false, nil
	}
	var r Run
	if err := tx.GetContext(ctx, &r, runSelectBase+` WHERE r.id=?`, runID); err != nil {
		return Run{}, false, normalizeErr(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?, 'system', ?, ?, ?)`, newID(), r.IssueID, r.ID, fmt.Sprintf("%s 실행을 시작했습니다", r.AgentName), t); err != nil {
		return Run{}, false, normalizeErr(err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, err
	}
	decorateRun(&r)
	return r, true, nil
}

func (s *Store) CompleteRun(ctx context.Context, runID string, exitCode int, stdoutPath, content string, contentTruncated bool, errMsg string) (Run, error) {
	status := "done"
	if exitCode != 0 {
		status = "failed"
	}
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE run SET status=?, finished_at=?, exit_code=?, stdout_path=?, error_message=? WHERE id=? AND status='running'`, status, t, exitCode, nullIfEmpty(stdoutPath), errMsg, runID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		// Another path (user cancel, shutdown recovery, etc.) already moved this
		// run out of running. Do not overwrite the terminal state or issue status.
		_ = tx.Rollback()
		return s.GetRun(ctx, runID)
	}
	if content == "" {
		content = emptyRunComment(status, errMsg)
	}
	truncated := 0
	if contentTruncated {
		truncated = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,author_agent_id,run_id,content,truncated,created_at) VALUES(?,?, 'agent', ?, ?, ?, ?, ?)`, newID(), run.IssueID, run.AgentID, run.ID, content, truncated, t); err != nil {
		return Run{}, normalizeErr(err)
	}
	if status == "done" {
		if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='done', updated_at=? WHERE id=?`, t, run.IssueID); err != nil {
			return Run{}, normalizeErr(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func emptyRunComment(status, errMsg string) string {
	if status == "failed" {
		if strings.TrimSpace(errMsg) != "" {
			return "에이전트 실행이 실패했습니다: " + errMsg
		}
		return "에이전트 실행이 실패했지만 출력이 없습니다."
	}
	return "에이전트가 출력 없이 완료되었습니다."
}

func (s *Store) FailRun(ctx context.Context, runID string, errMsg string) (Run, error) {
	return s.CompleteRun(ctx, runID, 1, "", "", false, errMsg)
}

func (s *Store) CancelRunningRun(ctx context.Context, issueID string) (Run, error) {
	r, err := s.GetRunningRunByIssue(ctx, issueID)
	if err != nil {
		return Run{}, err
	}
	return s.CancelRun(ctx, r.ID, "user cancelled")
}

func (s *Store) GetRunningRunByIssue(ctx context.Context, issueID string) (Run, error) {
	var r Run
	if err := s.db.GetContext(ctx, &r, runSelectBase+` WHERE r.issue_id=? AND r.status='running' LIMIT 1`, issueID); err != nil {
		return Run{}, normalizeErr(err)
	}
	decorateRun(&r)
	return r, nil
}

func (s *Store) CancelRun(ctx context.Context, runID, reason string) (Run, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "cancelled"
	}
	r, err := s.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if r.Status != "running" && r.Status != "queued" {
		return Run{}, ErrState
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	t := now()
	_, err = tx.ExecContext(ctx, `UPDATE run SET status='cancelled', finished_at=?, exit_code=-1, error_message=? WHERE id=? AND status IN ('queued','running')`, t, reason, r.ID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,?,?)`, newID(), r.IssueID, r.ID, cancelComment(reason), t); err != nil {
		return Run{}, normalizeErr(err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, r.ID)
}

func cancelComment(reason string) string {
	if strings.Contains(strings.ToLower(reason), "shutdown") {
		return "서버 종료로 실행이 취소되었습니다"
	}
	return "사용자가 실행을 취소했습니다"
}

func (s *Store) RecoverOrphanRuns(ctx context.Context) (int64, error) {
	var ids []struct {
		ID      string `db:"id"`
		IssueID string `db:"issue_id"`
	}
	if err := s.db.SelectContext(ctx, &ids, `SELECT id, issue_id FROM run WHERE status='running' AND finished_at IS NULL`); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	t := now()
	for _, row := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', exit_code=-2, finished_at=?, error_message='orphan recovered' WHERE id=?`, t, row.ID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,'재시작 중 진행 작업이 취소되었습니다 (orphan recovered)',?)`, newID(), row.IssueID, row.ID, t); err != nil {
			return 0, normalizeErr(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func decorateRun(r *Run) {
	r.LogURL = "/api/runs/" + r.ID + "/log"
	if r.StdoutPath.Valid && r.StdoutPath.String != "" {
		if st, err := os.Stat(r.StdoutPath.String); err == nil {
			r.StdoutSizeBytes = st.Size()
		}
	}
}

func (s *Store) GetRunLogPath(ctx context.Context, runID string) (string, error) {
	r, err := s.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if !r.StdoutPath.Valid || r.StdoutPath.String == "" {
		return "", ErrNotFound
	}
	if _, err := os.Stat(r.StdoutPath.String); err != nil {
		return "", ErrNotFound
	}
	return r.StdoutPath.String, nil
}

func txGetIssue(ctx context.Context, tx *sqlx.Tx, id string) (Issue, error) {
	var out Issue
	err := tx.GetContext(ctx, &out, issueSelectBase+` WHERE i.id=?`, id)
	return out, normalizeErr(err)
}
