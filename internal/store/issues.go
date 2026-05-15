package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const issueSelectBase = `
SELECT i.id, i.workspace_id, i.identifier, i.title, i.body, i.status,
       COALESCE(i.assignee_agent_id, '') AS assignee_agent_id,
       COALESCE(aa.name, '') AS assignee_agent_name,
       COALESCE(i.parent_issue_id, '') AS parent_issue_id,
       i.created_by,
       COALESCE(i.autopilot_rule_id, '') AS autopilot_rule_id,
       i.timeout_seconds_override,
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
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,chain_id,chain_depth) VALUES(?,?,?,'queued',?,?,?,?,?,0)`, runID, issueID, agentID, trigger, capSnapshot(in.Body), t, maxAttempts, runID)
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
		err := tx.GetContext(ctx, &a, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE id=?`, assigneeAgentID)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_content_snapshot,enqueued_at,max_attempts,chain_id,chain_depth) VALUES(?,?,?,'queued','rerun',?,?,?,?,0)`, runID, issueID, agentID, snapshot, t, maxAttempts, runID)
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

const runSelectBase = `
SELECT r.id, r.issue_id, r.agent_id, COALESCE(a.name,'') AS agent_name, r.status, r.trigger_type,
       COALESCE(r.trigger_comment_id,'') AS trigger_comment_id, r.trigger_content_snapshot,
       COALESCE(r.parent_run_id, '') AS parent_run_id,
       COALESCE(r.chain_id, '') AS chain_id,
       COALESCE(r.chain_depth, 0) AS chain_depth,
       r.enqueued_at, COALESCE(r.claimed_at,'') AS claimed_at, r.claimed_by,
       COALESCE(r.started_at,'') AS started_at, COALESCE(r.heartbeat_at,'') AS heartbeat_at,
       COALESCE(r.finished_at,'') AS finished_at,
       COALESCE(r.process_pid, 0) AS process_pid, COALESCE(r.process_pgid, 0) AS process_pgid,
       COALESCE(r.process_recorded_at, '') AS process_recorded_at,
       COALESCE(r.input_tokens, 0) AS input_tokens,
       COALESCE(r.output_tokens, 0) AS output_tokens,
       COALESCE(r.total_cost_micros, 0) AS total_cost_micros,
       COALESCE(r.model_resolved, '') AS model_resolved,
       COALESCE(r.attempt, 1) AS attempt,
       COALESCE(r.max_attempts, 1) AS max_attempts,
       COALESCE(r.next_retry_at, '') AS next_retry_at,
       r.exit_code, r.stdout_path, r.error_message,
       r.terminal_reason, r.failure_kind, r.cancel_reason
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
  AND (r.next_retry_at IS NULL OR r.next_retry_at='' OR r.next_retry_at <= ?)
  AND NOT EXISTS (SELECT 1 FROM run r2 WHERE r2.issue_id=r.issue_id AND r2.status='running')
  AND NOT EXISTS (SELECT 1 FROM run r3 JOIN issue i3 ON i3.id=r3.issue_id WHERE i3.workspace_id=i.workspace_id AND r3.status='running')
ORDER BY r.enqueued_at ASC, r.id ASC LIMIT 1`, now())
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, normalizeErr(err)
	}
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE run SET status='running', claimed_at=?, claimed_by=?, started_at=?, heartbeat_at=? WHERE id=? AND status='queued'`, t, workerID, t, t, runID)
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
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     r.ID,
		IssueID:   r.IssueID,
		EventType: RunEventClaimed,
		Message:   "Run claimed by worker",
		Details: map[string]any{
			"worker_id": workerID,
		},
	}); err != nil {
		return Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, err
	}
	decorateRun(&r)
	return r, true, nil
}

func (s *Store) CompleteRun(ctx context.Context, runID string, exitCode int, stdoutPath, content string, contentTruncated bool, errMsg string) (Run, error) {
	terminalReason := TerminalReasonCompleted
	failureKind := ""
	if exitCode != 0 {
		terminalReason = TerminalReasonExitNonzero
		failureKind = FailureKindExitNonzero
	}
	return s.CompleteRunWithReason(ctx, runID, FinishRunInput{
		ExitCode:         exitCode,
		StdoutPath:       stdoutPath,
		Content:          content,
		ContentTruncated: contentTruncated,
		ErrorMessage:     errMsg,
		TerminalReason:   terminalReason,
		FailureKind:      failureKind,
	})
}

func (s *Store) CompleteRunWithReason(ctx context.Context, runID string, in FinishRunInput) (Run, error) {
	if in.TerminalReason == "" {
		in.TerminalReason = TerminalReasonCompleted
		if in.ExitCode != 0 {
			in.TerminalReason = TerminalReasonExitNonzero
		}
	}
	status := "done"
	if in.ExitCode != 0 || in.TerminalReason != TerminalReasonCompleted {
		status = "failed"
	}
	if status == "failed" && in.FailureKind == "" {
		in.FailureKind = FailureKindUnknown
	}
	if status == "done" {
		in.FailureKind = ""
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
	res, err := tx.ExecContext(ctx, `UPDATE run SET status=?, finished_at=?, exit_code=?, stdout_path=?, error_message=?, terminal_reason=?, failure_kind=?, cancel_reason='', input_tokens=?, output_tokens=?, total_cost_micros=?, model_resolved=? WHERE id=? AND status='running'`, status, t, in.ExitCode, nullIfEmpty(in.StdoutPath), in.ErrorMessage, in.TerminalReason, in.FailureKind, in.InputTokens, in.OutputTokens, in.TotalCostMicros, in.ModelResolved, runID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		// Another path (user cancel, shutdown recovery, etc.) already moved this
		// run out of running. Do not overwrite the terminal state or issue status.
		_ = tx.Rollback()
		s.recoverRunStdoutPath(ctx, runID, in.StdoutPath)
		return s.GetRun(ctx, runID)
	}
	if in.Content == "" {
		in.Content = emptyRunComment(status, in.ErrorMessage)
	}
	truncated := 0
	if in.ContentTruncated {
		truncated = 1
	}
	commentID := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,author_agent_id,run_id,content,truncated,created_at) VALUES(?,?, 'agent', ?, ?, ?, ?, ?)`, commentID, run.IssueID, run.AgentID, run.ID, in.Content, truncated, t); err != nil {
		return Run{}, normalizeErr(err)
	}
	autoChainQueued := false
	if status == "done" {
		var err error
		autoChainQueued, err = s.enqueueAutoChainMention(ctx, tx, run, commentID, in.Content, t)
		if err != nil {
			return Run{}, err
		}
	}
	if in.StdoutTruncated {
		if _, err := appendRunEventTx(ctx, tx, RunEventInput{
			RunID:     run.ID,
			IssueID:   run.IssueID,
			EventType: RunEventStdoutTrunc,
			Severity:  RunEventSeverityWarn,
			Message:   "Stdout was truncated by output cap",
		}); err != nil {
			return Run{}, err
		}
	}
	eventType := RunEventCompleted
	severity := RunEventSeverityInfo
	message := "Run completed"
	if status == "failed" {
		eventType = RunEventFailed
		severity = RunEventSeverityError
		message = "Run failed"
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     run.ID,
		IssueID:   run.IssueID,
		EventType: eventType,
		Severity:  severity,
		Message:   message,
		Details: map[string]any{
			"exit_code":         in.ExitCode,
			"terminal_reason":   in.TerminalReason,
			"failure_kind":      in.FailureKind,
			"input_tokens":      in.InputTokens,
			"output_tokens":     in.OutputTokens,
			"total_cost_micros": in.TotalCostMicros,
			"model_resolved":    in.ModelResolved,
		},
	}); err != nil {
		return Run{}, err
	}
	if status == "done" && !autoChainQueued {
		if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='done', updated_at=? WHERE id=?`, t, run.IssueID); err != nil {
			return Run{}, normalizeErr(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) enqueueAutoChainMention(ctx context.Context, tx *sqlx.Tx, run Run, commentID, content, at string) (bool, error) {
	match := mentionRE.FindStringSubmatch(content)
	if len(match) < 2 {
		return false, nil
	}
	var workspace struct {
		ID               string `db:"id"`
		AutoChainEnabled bool   `db:"auto_chain_enabled"`
	}
	if err := tx.GetContext(ctx, &workspace, `SELECT w.id, COALESCE(w.auto_chain_enabled, 0) AS auto_chain_enabled FROM issue i JOIN workspace w ON w.id=i.workspace_id WHERE i.id=?`, run.IssueID); err != nil {
		return false, normalizeErr(err)
	}
	if !workspace.AutoChainEnabled {
		return false, nil
	}
	if run.ChainDepth >= 5 {
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "자동 체이닝 깊이 제한(5)에 도달해 추가 실행을 등록하지 않았습니다.", at)
		return false, normalizeErr(err)
	}
	name := match[1]
	var agent Agent
	if err := tx.GetContext(ctx, &agent, `SELECT id,workspace_id,name,runtime,model,instructions,COALESCE(summary,'') AS summary,COALESCE(tags,'') AS tags,is_main,timeout_seconds_override,retry_policy_json,created_at,updated_at FROM agent WHERE workspace_id=? AND lower(name)=lower(?)`, workspace.ID, name); err != nil {
		_, insertErr := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "자동 체이닝 대상 @"+name+"을 찾을 수 없습니다.", at)
		return false, normalizeErr(insertErr)
	}
	chainID := run.ChainID
	if chainID == "" {
		chainID = run.ID
	}

	var existingQueued int
	if err := tx.GetContext(ctx, &existingQueued, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND status='queued'`, run.IssueID, agent.ID); err != nil {
		return false, normalizeErr(err)
	}
	if existingQueued > 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "이미 @"+agent.Name+" queued run이 있어 자동 체이닝을 건너뛰었습니다.", at)
		return false, normalizeErr(err)
	}

	var duplicate int
	if err := tx.GetContext(ctx, &duplicate, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND (chain_id=? OR id=?)`, run.IssueID, agent.ID, chainID, chainID); err != nil {
		return false, normalizeErr(err)
	}
	if duplicate > 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "자동 체이닝 중복 방지를 위해 @"+agent.Name+" 실행을 건너뛰었습니다.", at)
		return false, normalizeErr(err)
	}
	maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, agent.ID)
	if err != nil {
		return false, err
	}
	nextRunID := newID()
	depth := run.ChainDepth + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_comment_id,trigger_content_snapshot,enqueued_at,max_attempts,parent_run_id,chain_id,chain_depth) VALUES(?,?,?,'queued','mention',?,?,?,?,?,?,?)`, nextRunID, run.IssueID, agent.ID, commentID, capSnapshot(content), at, maxAttempts, run.ID, chainID, depth); err != nil {
		return false, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     nextRunID,
		IssueID:   run.IssueID,
		EventType: RunEventQueued,
		Message:   "Run queued by auto-chain mention",
		Details: map[string]any{
			"trigger_type":  "mention",
			"auto_chain":    true,
			"parent_run_id": run.ID,
			"chain_id":      chainID,
			"chain_depth":   depth,
			"agent_name":    agent.Name,
		},
	}); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "자동 체이닝으로 @"+agent.Name+" 실행을 큐에 등록했습니다.", at)
	return err == nil, normalizeErr(err)
}

func (s *Store) recoverRunStdoutPath(ctx context.Context, runID, stdoutPath string) {
	stdoutPath = strings.TrimSpace(stdoutPath)
	if stdoutPath == "" {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE run SET stdout_path=? WHERE id=? AND (stdout_path IS NULL OR stdout_path='')`, stdoutPath, runID)
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
	return s.CompleteRunWithReason(ctx, runID, FinishRunInput{ExitCode: 1, ErrorMessage: errMsg, TerminalReason: TerminalReasonUnknownFailure, FailureKind: FailureKindUnknown})
}

func (s *Store) FailInfrastructureRun(ctx context.Context, runID, terminalReason, failureKind, errMsg string) (Run, error) {
	if terminalReason == "" {
		terminalReason = TerminalReasonUnknownFailure
	}
	if failureKind == "" {
		failureKind = FailureKindUnknown
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
	res, err := tx.ExecContext(ctx, `UPDATE run SET status='failed', finished_at=?, exit_code=1, error_message=?, terminal_reason=?, failure_kind=?, cancel_reason='' WHERE id=? AND status IN ('queued','running')`, t, errMsg, terminalReason, failureKind, runID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		_ = tx.Rollback()
		return s.GetRun(ctx, runID)
	}
	comment := emptyRunComment("failed", errMsg)
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,author_agent_id,run_id,content,truncated,created_at) VALUES(?,?, 'agent', ?, ?, ?, 0, ?)`, newID(), run.IssueID, run.AgentID, run.ID, comment, t); err != nil {
		return Run{}, normalizeErr(err)
	}
	eventType := RunEventFailed
	message := "Run failed"
	if terminalReason == TerminalReasonClaimPreparationFailed {
		eventType = RunEventPrepareFailed
		message = "Run preparation failed"
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     run.ID,
		IssueID:   run.IssueID,
		EventType: eventType,
		Severity:  RunEventSeverityError,
		Message:   message,
		Details: map[string]any{
			"terminal_reason": terminalReason,
			"failure_kind":    failureKind,
		},
	}); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) CancelRunningRun(ctx context.Context, issueID string) (Run, error) {
	r, err := s.GetRunningRunByIssue(ctx, issueID)
	if err != nil {
		return Run{}, err
	}
	return s.CancelRunWithReason(ctx, r.ID, CancelReasonInput{
		Message:        defaultCancelMessage(CancelReasonUser),
		TerminalReason: TerminalReasonUserCancelled,
		CancelReason:   CancelReasonUser,
	})
}

func (s *Store) GetActiveRunByIssue(ctx context.Context, issueID string) (Run, error) {
	var r Run
	if err := s.db.GetContext(ctx, &r, runSelectBase+` WHERE r.issue_id=? AND r.status IN ('running','queued') ORDER BY CASE r.status WHEN 'running' THEN 0 ELSE 1 END, r.enqueued_at ASC LIMIT 1`, issueID); err != nil {
		return Run{}, normalizeErr(err)
	}
	decorateRun(&r)
	return r, nil
}

func (s *Store) GetRunningRunByIssue(ctx context.Context, issueID string) (Run, error) {
	var r Run
	if err := s.db.GetContext(ctx, &r, runSelectBase+` WHERE r.issue_id=? AND r.status='running' LIMIT 1`, issueID); err != nil {
		return Run{}, normalizeErr(err)
	}
	decorateRun(&r)
	return r, nil
}

func (s *Store) HeartbeatRun(ctx context.Context, runID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE run SET heartbeat_at=? WHERE id=? AND status='running'`, now(), runID)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrState
	}
	return nil
}

func (s *Store) MarkRunProcess(ctx context.Context, runID string, pid, pgid int) error {
	if strings.TrimSpace(runID) == "" || pid < 0 || pgid < 0 {
		return ErrValidation
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE run SET process_pid=?, process_pgid=?, process_recorded_at=? WHERE id=? AND status='running'`, pid, pgid, t, runID)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrState
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     runID,
		EventType: RunEventStarting,
		Message:   "Executor process started",
		Details: map[string]any{
			"pid":  pid,
			"pgid": pgid,
		},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListRunningProcessGroups(ctx context.Context) ([]RunningProcessGroup, error) {
	var groups []RunningProcessGroup
	if err := s.db.SelectContext(ctx, &groups, `
SELECT process_pgid,
       COALESCE(MAX(process_recorded_at), '') AS process_recorded_at,
       COUNT(*) AS run_count
FROM run
WHERE status='running' AND process_pgid > 1
GROUP BY process_pgid
ORDER BY process_pgid`); err != nil {
		return nil, normalizeErr(err)
	}
	return groups, nil
}

func (s *Store) CancelRun(ctx context.Context, runID, reason string) (Run, error) {
	return s.CancelRunWithReason(ctx, runID, classifyCancelReason(reason))
}

func (s *Store) CancelRunWithReason(ctx context.Context, runID string, reason CancelReasonInput) (Run, error) {
	reason = normalizeCancelReason(reason)
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
	res, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', finished_at=?, exit_code=-1, error_message=?, terminal_reason=?, cancel_reason=? WHERE id=? AND status IN ('queued','running')`, t, reason.Message, reason.TerminalReason, reason.CancelReason, r.ID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return Run{}, ErrState
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,?,?)`, newID(), r.IssueID, r.ID, cancelComment(reason), t); err != nil {
		return Run{}, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     r.ID,
		IssueID:   r.IssueID,
		EventType: RunEventCancelled,
		Message:   "Run cancelled",
		Details: map[string]any{
			"terminal_reason": reason.TerminalReason,
			"cancel_reason":   reason.CancelReason,
		},
	}); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, r.ID)
}

func normalizeCancelReason(reason CancelReasonInput) CancelReasonInput {
	if reason.TerminalReason == "" {
		reason.TerminalReason = terminalReasonForCancelReason(reason.CancelReason)
	}
	if reason.CancelReason == "" {
		reason.CancelReason = cancelReasonForTerminalReason(reason.TerminalReason)
	}
	if reason.TerminalReason == "" || reason.CancelReason == "" {
		classified := classifyCancelReason(reason.Message)
		if reason.TerminalReason == "" {
			reason.TerminalReason = classified.TerminalReason
		}
		if reason.CancelReason == "" {
			reason.CancelReason = classified.CancelReason
		}
	}
	if strings.TrimSpace(reason.Message) == "" {
		reason.Message = defaultCancelMessage(reason.CancelReason)
	}
	return reason
}

func classifyCancelReason(message string) CancelReasonInput {
	if strings.TrimSpace(message) == "" {
		message = "cancelled"
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "shutdown"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonShutdown, CancelReason: CancelReasonShutdown}
	case strings.Contains(lower, "issue"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonIssueCancelled, CancelReason: CancelReasonIssue}
	case strings.Contains(lower, "orphan"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonOrphanRecovered, CancelReason: CancelReasonOrphan}
	case strings.Contains(lower, "stale"):
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonStaleRecovered, CancelReason: CancelReasonStale}
	default:
		return CancelReasonInput{Message: message, TerminalReason: TerminalReasonUserCancelled, CancelReason: CancelReasonUser}
	}
}

func terminalReasonForCancelReason(reason string) string {
	switch reason {
	case CancelReasonShutdown:
		return TerminalReasonShutdown
	case CancelReasonIssue:
		return TerminalReasonIssueCancelled
	case CancelReasonOrphan:
		return TerminalReasonOrphanRecovered
	case CancelReasonStale:
		return TerminalReasonStaleRecovered
	case CancelReasonUser:
		return TerminalReasonUserCancelled
	default:
		return ""
	}
}

func cancelReasonForTerminalReason(reason string) string {
	switch reason {
	case TerminalReasonShutdown:
		return CancelReasonShutdown
	case TerminalReasonIssueCancelled:
		return CancelReasonIssue
	case TerminalReasonOrphanRecovered:
		return CancelReasonOrphan
	case TerminalReasonStaleRecovered:
		return CancelReasonStale
	case TerminalReasonUserCancelled:
		return CancelReasonUser
	default:
		return ""
	}
}

func defaultCancelMessage(reason string) string {
	switch reason {
	case CancelReasonShutdown:
		return "shutdown"
	case CancelReasonIssue:
		return "issue cancelled"
	case CancelReasonOrphan:
		return "orphan recovered"
	case CancelReasonStale:
		return "stale recovered"
	default:
		return "user cancelled"
	}
}

func cancelComment(reason CancelReasonInput) string {
	switch reason.CancelReason {
	case CancelReasonShutdown:
		return "서버 종료로 실행이 취소되었습니다"
	case CancelReasonIssue:
		return "이슈 취소로 실행이 취소되었습니다"
	case CancelReasonOrphan:
		return "재시작 중 진행 작업이 취소되었습니다 (orphan recovered)"
	case CancelReasonStale:
		return "오래된 진행 작업이 취소되었습니다 (stale recovered)"
	default:
		return "사용자가 실행을 취소했습니다"
	}
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
		if _, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', exit_code=-2, finished_at=?, error_message='orphan recovered', terminal_reason=?, cancel_reason=? WHERE id=?`, t, TerminalReasonOrphanRecovered, CancelReasonOrphan, row.ID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,'재시작 중 진행 작업이 취소되었습니다 (orphan recovered)',?)`, newID(), row.IssueID, row.ID, t); err != nil {
			return 0, normalizeErr(err)
		}
		if _, err := appendRunEventTx(ctx, tx, RunEventInput{
			RunID:     row.ID,
			IssueID:   row.IssueID,
			EventType: RunEventOrphan,
			Severity:  RunEventSeverityWarn,
			Message:   "Orphan running run recovered",
			Details: map[string]any{
				"terminal_reason": TerminalReasonOrphanRecovered,
				"cancel_reason":   CancelReasonOrphan,
			},
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func (s *Store) RecoverStaleRuns(ctx context.Context, cutoff string, excludeRunIDs []string) (int64, error) {
	if strings.TrimSpace(cutoff) == "" {
		return 0, ErrValidation
	}
	args := []any{cutoff, cutoff}
	where := `status='running' AND finished_at IS NULL AND (heartbeat_at IS NULL OR heartbeat_at < ? OR (heartbeat_at = '' AND claimed_at < ?))`
	if len(excludeRunIDs) > 0 {
		where += ` AND id NOT IN (` + placeholders(len(excludeRunIDs)) + `)`
		for _, id := range excludeRunIDs {
			args = append(args, id)
		}
	}
	var ids []struct {
		ID      string `db:"id"`
		IssueID string `db:"issue_id"`
	}
	if err := s.db.SelectContext(ctx, &ids, `SELECT id, issue_id FROM run WHERE `+where+` ORDER BY heartbeat_at ASC, claimed_at ASC`, args...); err != nil {
		return 0, normalizeErr(err)
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
		res, err := tx.ExecContext(ctx, `UPDATE run SET status='cancelled', exit_code=-3, finished_at=?, error_message='stale recovered', terminal_reason=?, cancel_reason=? WHERE id=? AND status='running'`, t, TerminalReasonStaleRecovered, CancelReasonStale, row.ID)
		if err != nil {
			return 0, normalizeErr(err)
		}
		aff, _ := res.RowsAffected()
		if aff == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,'오래된 진행 작업이 취소되었습니다 (stale recovered)',?)`, newID(), row.IssueID, row.ID, t); err != nil {
			return 0, normalizeErr(err)
		}
		if _, err := appendRunEventTx(ctx, tx, RunEventInput{
			RunID:     row.ID,
			IssueID:   row.IssueID,
			EventType: RunEventStale,
			Severity:  RunEventSeverityWarn,
			Message:   "Stale running run recovered",
			Details: map[string]any{
				"terminal_reason": TerminalReasonStaleRecovered,
				"cancel_reason":   CancelReasonStale,
				"cutoff":          cutoff,
			},
		}); err != nil {
			return 0, err
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

func (s *Store) RescheduleRunForRetry(ctx context.Context, runID, failureKind, errMsg, stdoutPath string) (Run, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	var run Run
	if err := tx.GetContext(ctx, &run, runSelectBase+` WHERE r.id=?`, runID); err != nil {
		return Run{}, normalizeErr(err)
	}
	policy, err := retryPolicyForAgent(ctx, tx, run.AgentID)
	if err != nil {
		return Run{}, err
	}
	if run.Status != "running" || !shouldRetryRunWithPolicy(failureKind, run.Attempt, run.MaxAttempts, policy) {
		return Run{}, ErrState
	}
	nextRetryAt := time.Now().UTC().Add(retryBackoffWithPolicy(run.Attempt, policy)).Format(time.RFC3339Nano)
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE run
SET status='queued',
    next_retry_at=?,
    attempt=attempt+1,
    claimed_at=NULL,
    claimed_by='',
    started_at=NULL,
    heartbeat_at=NULL,
    process_pid=NULL,
    process_pgid=NULL,
    process_recorded_at=NULL,
    stdout_path=?,
    error_message=?,
    terminal_reason='',
    failure_kind='',
    cancel_reason=''
WHERE id=? AND status='running' AND attempt<max_attempts`, nullIfEmpty(nextRetryAt), nullIfEmpty(stdoutPath), errMsg, runID)
	if err != nil {
		return Run{}, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return Run{}, ErrState
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?, 'system', ?, ?, ?)`, newID(), run.IssueID, run.ID, fmt.Sprintf("일시적 오류로 재시도를 예약했습니다 (attempt %d/%d, %s)", run.Attempt+1, run.MaxAttempts, nextRetryAt), t); err != nil {
		return Run{}, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     run.ID,
		IssueID:   run.IssueID,
		EventType: RunEventFailed,
		Severity:  RunEventSeverityWarn,
		Message:   "Run retry scheduled",
		Details: map[string]any{
			"attempt":         run.Attempt,
			"next_attempt":    run.Attempt + 1,
			"max_attempts":    run.MaxAttempts,
			"failure_kind":    failureKind,
			"error_message":   errMsg,
			"next_retry_at":   nextRetryAt,
			"retry_scheduled": true,
		},
	}); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) RunUsageSummary(ctx context.Context, since string) (RunUsageSummary, error) {
	if since == "" {
		since = time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	}
	var out RunUsageSummary
	out.Since = since
	err := s.db.GetContext(ctx, &out, `SELECT
  COUNT(*) AS run_count,
  COALESCE(SUM(input_tokens), 0) AS input_tokens,
  COALESCE(SUM(output_tokens), 0) AS output_tokens,
  COALESCE(SUM(input_tokens + output_tokens), 0) AS total_tokens,
  COALESCE(SUM(total_cost_micros), 0) AS total_cost_micros,
  COALESCE(SUM(CASE WHEN input_tokens > 0 OR output_tokens > 0 OR total_cost_micros > 0 THEN 1 ELSE 0 END), 0) AS measured_run_count
FROM run
WHERE COALESCE(finished_at, enqueued_at) >= ?`, since)
	if err != nil {
		return RunUsageSummary{}, normalizeErr(err)
	}
	out.Since = since
	return out, nil
}
