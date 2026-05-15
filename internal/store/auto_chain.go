package store

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"time"
)

func (s *Store) enqueueAutoChainMention(ctx context.Context, tx *sqlx.Tx, run Run, commentID, content, at string) (bool, error) {
	match := mentionRE.FindStringSubmatch(content)
	if len(match) < 2 {
		return false, nil
	}
	var workspace struct {
		ID                       string `db:"id"`
		AutoChainEnabled         bool   `db:"auto_chain_enabled"`
		AutoChainMaxDepth        int    `db:"auto_chain_max_depth"`
		AutoChainDailyRunLimit   int    `db:"auto_chain_daily_run_limit"`
		AutoChainDailyCostMicros int64  `db:"auto_chain_daily_cost_micros"`
		AutoChainDryRun          bool   `db:"auto_chain_dry_run"`
	}
	if err := tx.GetContext(ctx, &workspace, `SELECT w.id, COALESCE(w.auto_chain_enabled, 0) AS auto_chain_enabled, COALESCE(w.auto_chain_max_depth, 5) AS auto_chain_max_depth, COALESCE(w.auto_chain_daily_run_limit, 20) AS auto_chain_daily_run_limit, COALESCE(w.auto_chain_daily_cost_micros, 0) AS auto_chain_daily_cost_micros, COALESCE(w.auto_chain_dry_run, 0) AS auto_chain_dry_run FROM issue i JOIN workspace w ON w.id=i.workspace_id WHERE i.id=?`, run.IssueID); err != nil {
		return false, normalizeErr(err)
	}
	if !workspace.AutoChainEnabled {
		return false, nil
	}
	maxDepth := normalizeAutoChainMaxDepth(workspace.AutoChainMaxDepth)
	if run.ChainDepth >= maxDepth {
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, fmt.Sprintf("자동 체이닝 깊이 제한(%d)에 도달해 추가 실행을 등록하지 않았습니다.", maxDepth), at)
		return false, normalizeErr(err)
	}
	if workspace.AutoChainDryRun {
		name := match[1]
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, "자동 체이닝 dry-run: @"+name+" 실행을 큐에 등록하지 않았습니다.", at)
		return false, normalizeErr(err)
	}
	if ok, message, err := s.autoChainWithinDailyGuards(ctx, tx, workspace.ID, workspace.AutoChainDailyRunLimit, workspace.AutoChainDailyCostMicros); err != nil {
		return false, err
	} else if !ok {
		_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), run.IssueID, message, at)
		return false, normalizeErr(err)
	}
	name := match[1]
	var agent Agent
	if err := tx.GetContext(ctx, &agent, agentSelectBase+` WHERE workspace_id=? AND lower(name)=lower(?)`, workspace.ID, name); err != nil {
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
	instructionsVersion := agent.InstructionsVersion
	if instructionsVersion <= 0 {
		instructionsVersion = 1
	}
	nextRunID := newID()
	depth := run.ChainDepth + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO run(id,issue_id,agent_id,status,trigger_type,trigger_comment_id,trigger_content_snapshot,enqueued_at,max_attempts,agent_instructions_version,parent_run_id,chain_id,chain_depth) VALUES(?,?,?,'queued','mention',?,?,?,?,?,?,?,?)`, nextRunID, run.IssueID, agent.ID, commentID, capSnapshot(content), at, maxAttempts, instructionsVersion, run.ID, chainID, depth); err != nil {
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

func (s *Store) autoChainWithinDailyGuards(ctx context.Context, tx *sqlx.Tx, workspaceID string, runLimit int, costLimitMicros int64) (bool, string, error) {
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	if runLimit > 0 {
		var count int
		if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM run r JOIN issue i ON i.id=r.issue_id WHERE i.workspace_id=? AND r.chain_depth > 0 AND r.enqueued_at >= ?`, workspaceID, since); err != nil {
			return false, "", normalizeErr(err)
		}
		if count >= runLimit {
			return false, fmt.Sprintf("자동 체이닝 24시간 실행 제한(%d)에 도달해 추가 실행을 등록하지 않았습니다.", runLimit), nil
		}
	}
	if costLimitMicros > 0 {
		var cost int64
		if err := tx.GetContext(ctx, &cost, `SELECT COALESCE(SUM(r.total_cost_micros),0) FROM run r JOIN issue i ON i.id=r.issue_id WHERE i.workspace_id=? AND COALESCE(r.finished_at, r.enqueued_at) >= ?`, workspaceID, since); err != nil {
			return false, "", normalizeErr(err)
		}
		if cost >= costLimitMicros {
			return false, fmt.Sprintf("자동 체이닝 24시간 비용 제한($%.4f)에 도달해 추가 실행을 등록하지 않았습니다.", float64(costLimitMicros)/1_000_000), nil
		}
	}
	return true, "", nil
}
