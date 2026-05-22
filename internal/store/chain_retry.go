package store

import (
	"context"
	"strings"
)

// RetryFailedRunInChain finds the most recent failed run in the given chain
// and enqueues a new run on the same agent, preserving chain_id and
// chain_depth. The new run carries trigger_type='rerun' and a snapshot that
// names the failed run it is continuing. The issue's status is reopened to
// 'open' so the run can claim normally.
//
// Returns ErrNotFound when no failed run exists in the chain. Returns
// ErrState when the chain still has queued / running runs — the operator
// should wait or cancel the chain first.
func (s *Store) RetryFailedRunInChain(ctx context.Context, chainID string) (Run, error) {
	if strings.TrimSpace(chainID) == "" {
		return Run{}, ErrValidation
	}
	// Refuse to retry while non-terminal runs still occupy the chain so the
	// new run does not collide with the unique (issue_id, agent_id) queued
	// index. CancelRunsByChain provides the escape hatch.
	var pending int
	if err := s.db.GetContext(ctx, &pending,
		`SELECT COUNT(*) FROM run WHERE chain_id=? AND status IN ('queued','running')`, chainID,
	); err != nil {
		return Run{}, normalizeErr(err)
	}
	if pending > 0 {
		return Run{}, ErrState
	}
	var last Run
	if err := s.db.GetContext(ctx, &last,
		runSelectBase+` WHERE r.chain_id=? AND r.status='failed' ORDER BY r.finished_at DESC, r.enqueued_at DESC LIMIT 1`,
		chainID,
	); err != nil {
		return Run{}, normalizeErr(err)
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	runID := newID()
	t := now()
	snapshot := "[chain retry of run " + last.ID + "]"
	maxAttempts, err := retryMaxAttemptsForAgent(ctx, tx, last.AgentID)
	if err != nil {
		return Run{}, err
	}
	instructionsVersion, err := agentInstructionsVersionForAgent(ctx, tx, last.AgentID)
	if err != nil {
		return Run{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO run(id, issue_id, agent_id, status, trigger_type, trigger_content_snapshot, parent_run_id, chain_id, chain_depth, enqueued_at, max_attempts, agent_instructions_version) VALUES(?,?,?,'queued','rerun',?,?,?,?,?,?,?)`,
		runID, last.IssueID, last.AgentID, snapshot, last.ID, last.ChainID, last.ChainDepth, t, maxAttempts, instructionsVersion,
	); err != nil {
		return Run{}, normalizeErr(err)
	}
	if _, err := appendRunEventTx(ctx, tx, RunEventInput{
		RunID:     runID,
		IssueID:   last.IssueID,
		EventType: RunEventQueued,
		Message:   "Run queued by chain retry",
		Details: map[string]any{
			"trigger_type": "rerun",
			"chain_id":     chainID,
			"retry_of":     last.ID,
		},
	}); err != nil {
		return Run{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issue SET status='open', updated_at=? WHERE id=?`, t, last.IssueID); err != nil {
		return Run{}, normalizeErr(err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, runID)
}
