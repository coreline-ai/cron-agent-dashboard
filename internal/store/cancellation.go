package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
	event, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     runID,
		EventType: RunEventStarting,
		Message:   "Executor process started",
		Details: map[string]any{
			"pid":  pid,
			"pgid": pgid,
		},
	})
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notifyRunEvent(event.IssueID, event.RunID)
	return nil
}

func (s *Store) ListRunningProcessGroups(ctx context.Context) ([]RunningProcessGroup, error) {
	var groups []RunningProcessGroup
	if err := s.db.SelectContext(ctx, &groups, `
SELECT process_pgid,
       COALESCE(MIN(NULLIF(process_pid, 0)), 0) AS process_pid,
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
	s.notifyRunEvent(r.IssueID, r.ID)
	return s.GetRun(ctx, r.ID)
}

// CancelRunsByChain cancels every queued / running run that shares the given
// chain_id. Already-terminal rows (done / failed / cancelled / completed) are
// silently skipped so a partial chain that has finished does not raise
// ErrState. Returns the number of rows it actually cancelled.
//
// Cancellation reason defaults to ('issue_cancelled', 'user') because the
// operator initiates this from the chain summary panel; a richer per-row
// reason classification (e.g. distinguishing chain-issued vs. user-issued)
// is intentionally out of scope.
func (s *Store) CancelRunsByChain(ctx context.Context, chainID string, reason CancelReasonInput) (int, error) {
	if strings.TrimSpace(chainID) == "" {
		return 0, ErrValidation
	}
	reason = normalizeCancelReason(reason)
	if reason.Message == "" {
		reason.Message = "Run cancelled because chain was cancelled"
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var rows []struct {
		ID      string `db:"id"`
		IssueID string `db:"issue_id"`
	}
	if err := tx.SelectContext(ctx, &rows,
		`SELECT id, issue_id FROM run WHERE chain_id=? AND status IN ('queued','running') ORDER BY enqueued_at ASC`,
		chainID,
	); err != nil {
		return 0, normalizeErr(err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	t := now()
	cancelled := 0
	for _, row := range rows {
		res, err := tx.ExecContext(ctx,
			`UPDATE run SET status='cancelled', finished_at=?, exit_code=-1, error_message=?, terminal_reason=?, cancel_reason=? WHERE id=? AND status IN ('queued','running')`,
			t, reason.Message, reason.TerminalReason, reason.CancelReason, row.ID,
		)
		if err != nil {
			return cancelled, normalizeErr(err)
		}
		aff, _ := res.RowsAffected()
		if aff == 0 {
			continue
		}
		cancelled++
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO comment(id,issue_id,author_type,run_id,content,created_at) VALUES(?,?,'system',?,?,?)`,
			newID(), row.IssueID, row.ID, cancelComment(reason), t,
		); err != nil {
			return cancelled, normalizeErr(err)
		}
		if _, err := appendRunEventTx(ctx, tx, RunEventInput{
			RunID:     row.ID,
			IssueID:   row.IssueID,
			EventType: RunEventCancelled,
			Message:   "Run cancelled because chain was cancelled",
			Details: map[string]any{
				"chain_id":        chainID,
				"terminal_reason": reason.TerminalReason,
				"cancel_reason":   reason.CancelReason,
			},
		}); err != nil {
			return cancelled, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, row := range rows {
		s.notifyRunEvent(row.IssueID, row.ID)
	}
	return cancelled, nil
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
	for _, row := range ids {
		s.notifyRunEvent(row.IssueID, row.ID)
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
	for _, row := range ids {
		s.notifyRunEvent(row.IssueID, row.ID)
	}
	return int64(len(ids)), nil
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
	s.notifyRunEvent(run.IssueID, run.ID)
	return s.GetRun(ctx, runID)
}
