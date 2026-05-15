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

const runSelectBase = `
SELECT r.id, r.issue_id, r.agent_id, COALESCE(a.name,'') AS agent_name, r.status, r.trigger_type,
       COALESCE(r.trigger_comment_id,'') AS trigger_comment_id, r.trigger_content_snapshot,
       COALESCE(r.parent_run_id, '') AS parent_run_id,
       COALESCE(r.chain_id, '') AS chain_id,
       COALESCE(r.chain_depth, 0) AS chain_depth,
       COALESCE(r.agent_instructions_version, 1) AS agent_instructions_version,
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
	status, in := normalizeFinishRunInput(in)
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
	updated, err := s.updateRunTerminalTx(ctx, tx, runID, status, in, t)
	if err != nil {
		return Run{}, err
	}
	if !updated {
		// Another path (user cancel, shutdown recovery, etc.) already moved this
		// run out of running. Do not overwrite the terminal state or issue status.
		_ = tx.Rollback()
		s.recoverRunStdoutPath(ctx, runID, in.StdoutPath)
		return s.GetRun(ctx, runID)
	}
	commentID, content, err := s.insertAgentResultCommentTx(ctx, tx, run, status, in, t)
	if err != nil {
		return Run{}, err
	}
	autoChainQueued := false
	if status == "done" {
		autoChainQueued, err = s.enqueueAutoChainMention(ctx, tx, run, commentID, content, t)
		if err != nil {
			return Run{}, err
		}
	}
	if err := emitRunFinishEventsTx(ctx, tx, run, status, in); err != nil {
		return Run{}, err
	}
	if err := maybeMarkIssueDoneTx(ctx, tx, run, status, autoChainQueued, t); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func normalizeFinishRunInput(in FinishRunInput) (string, FinishRunInput) {
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
		if in.TerminalReason == TerminalReasonExitNonzero {
			in.FailureKind = FailureKindExitNonzero
		}
	}
	if status == "done" {
		in.FailureKind = ""
	}
	return status, in
}

func (s *Store) updateRunTerminalTx(ctx context.Context, tx *sqlx.Tx, runID, status string, in FinishRunInput, finishedAt string) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE run SET status=?, finished_at=?, exit_code=?, stdout_path=?, error_message=?, terminal_reason=?, failure_kind=?, cancel_reason='', input_tokens=?, output_tokens=?, total_cost_micros=?, model_resolved=? WHERE id=? AND status='running'`, status, finishedAt, in.ExitCode, nullIfEmpty(in.StdoutPath), in.ErrorMessage, in.TerminalReason, in.FailureKind, in.InputTokens, in.OutputTokens, in.TotalCostMicros, in.ModelResolved, runID)
	if err != nil {
		return false, normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	return aff > 0, nil
}

func (s *Store) insertAgentResultCommentTx(ctx context.Context, tx *sqlx.Tx, run Run, status string, in FinishRunInput, createdAt string) (string, string, error) {
	content := in.Content
	if content == "" {
		content = emptyRunComment(status, in.ErrorMessage)
	}
	truncated := 0
	if in.ContentTruncated {
		truncated = 1
	}
	commentID := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,author_agent_id,run_id,content,truncated,created_at) VALUES(?,?, 'agent', ?, ?, ?, ?, ?)`, commentID, run.IssueID, run.AgentID, run.ID, content, truncated, createdAt); err != nil {
		return "", "", normalizeErr(err)
	}
	return commentID, content, nil
}

func emitRunFinishEventsTx(ctx context.Context, tx *sqlx.Tx, run Run, status string, in FinishRunInput) error {
	if in.StdoutTruncated {
		if _, err := appendRunEventTx(ctx, tx, RunEventInput{
			RunID:     run.ID,
			IssueID:   run.IssueID,
			EventType: RunEventStdoutTrunc,
			Severity:  RunEventSeverityWarn,
			Message:   "Stdout was truncated by output cap",
		}); err != nil {
			return err
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
	_, err := appendRunEventTx(ctx, tx, RunEventInput{
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
	})
	return err
}

func maybeMarkIssueDoneTx(ctx context.Context, tx *sqlx.Tx, run Run, status string, autoChainQueued bool, updatedAt string) error {
	if status != "done" || autoChainQueued {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='done', updated_at=? WHERE id=?`, updatedAt, run.IssueID); err != nil {
		return normalizeErr(err)
	}
	return nil
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
