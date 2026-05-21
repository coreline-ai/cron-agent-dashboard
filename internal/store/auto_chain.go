package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type autoChainConfig struct {
	WorkspaceID     string `db:"id"`
	Enabled         bool   `db:"auto_chain_enabled"`
	MaxDepth        int    `db:"auto_chain_max_depth"`
	DailyRunLimit   int    `db:"auto_chain_daily_run_limit"`
	DailyCostMicros int64  `db:"auto_chain_daily_cost_micros"`
	DryRun          bool   `db:"auto_chain_dry_run"`
}

func (s *Store) enqueueAutoChainMention(ctx context.Context, tx *sqlx.Tx, run Run, commentID, content, at string) (bool, error) {
	mention := firstAutoChainMention(content)
	if mention == "" {
		return false, nil
	}
	cfg, err := s.fetchAutoChainConfig(ctx, tx, run.IssueID)
	if err != nil || !cfg.Enabled {
		return false, err
	}
	// Resolve the agent first so hub-PM aware guards (max_depth bypass for
	// main agent re-entry) can apply. Agent lookup failures are split into
	// "not registered" vs transient errors via autoChainAgentLookupMessage.
	agent, err := s.resolveAutoChainAgent(ctx, tx, cfg.WorkspaceID, mention)
	if err != nil {
		return false, s.insertAutoChainSystemComment(ctx, tx, run.IssueID, autoChainAgentLookupMessage(mention, err), at)
	}
	if ok, message, err := s.checkAutoChainGuards(ctx, tx, cfg, run, agent); err != nil {
		return false, err
	} else if !ok {
		return false, s.insertAutoChainSystemComment(ctx, tx, run.IssueID, message, at)
	}
	return s.dispatchAutoChainRun(ctx, tx, run, agent, commentID, content, at)
}

// autoChainAgentLookupMessage returns the operator-facing system comment text
// for an auto-chain agent lookup failure. It splits ErrNotFound ("the mention
// targets an agent that does not exist in this workspace") from unexpected
// store errors ("transient DB / lookup failure"). Raw error details are not
// surfaced here to avoid leaking SQL fragments or credentials into the
// issue thread.
func autoChainAgentLookupMessage(mention string, err error) string {
	if errors.Is(err, ErrNotFound) {
		return "자동 체이닝 대상 @" + mention + "을 찾을 수 없습니다."
	}
	return "자동 체이닝 agent 조회 중 일시적 오류가 발생했습니다. 운영자에게 알려주세요."
}

func firstAutoChainMention(content string) string {
	match := mentionRE.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func (s *Store) fetchAutoChainConfig(ctx context.Context, tx *sqlx.Tx, issueID string) (autoChainConfig, error) {
	var cfg autoChainConfig
	err := tx.GetContext(ctx, &cfg, `SELECT w.id,
       COALESCE(w.auto_chain_enabled, 0) AS auto_chain_enabled,
       COALESCE(w.auto_chain_max_depth, 5) AS auto_chain_max_depth,
       COALESCE(w.auto_chain_daily_run_limit, 20) AS auto_chain_daily_run_limit,
       COALESCE(w.auto_chain_daily_cost_micros, 0) AS auto_chain_daily_cost_micros,
       COALESCE(w.auto_chain_dry_run, 0) AS auto_chain_dry_run
FROM issue i JOIN workspace w ON w.id=i.workspace_id WHERE i.id=?`, issueID)
	return cfg, normalizeErr(err)
}

func (s *Store) checkAutoChainGuards(ctx context.Context, tx *sqlx.Tx, cfg autoChainConfig, run Run, agent Agent) (bool, string, error) {
	maxDepth := normalizeAutoChainMaxDepth(cfg.MaxDepth)
	// Main agent (workspace PM hub) re-entry does not advance chain_depth in
	// dispatchAutoChainRun, so it must also be exempt from the max_depth
	// gate. Only worker dispatches count toward the depth limit; the daily
	// run / cost guards and queued-duplicate guard still apply to both.
	if !agent.IsMain && run.ChainDepth >= maxDepth {
		return false, fmt.Sprintf("자동 체이닝 깊이 제한(%d)에 도달해 추가 실행을 등록하지 않았습니다.", maxDepth), nil
	}
	if cfg.DryRun {
		return false, "자동 체이닝 dry-run: @" + agent.Name + " 실행을 큐에 등록하지 않았습니다.", nil
	}
	return s.autoChainWithinDailyGuards(ctx, tx, cfg.WorkspaceID, cfg.DailyRunLimit, cfg.DailyCostMicros)
}

func (s *Store) resolveAutoChainAgent(ctx context.Context, tx *sqlx.Tx, workspaceID, name string) (Agent, error) {
	var agent Agent
	err := tx.GetContext(ctx, &agent, agentSelectBase+` WHERE workspace_id=? AND lower(name)=lower(?)`, workspaceID, name)
	return agent, normalizeErr(err)
}

func (s *Store) dispatchAutoChainRun(ctx context.Context, tx *sqlx.Tx, run Run, agent Agent, commentID, content, at string) (bool, error) {
	chainID := run.ChainID
	if chainID == "" {
		chainID = run.ID
	}
	if ok, message, err := s.checkAutoChainDispatchDuplicates(ctx, tx, run, agent, chainID); err != nil {
		return false, err
	} else if !ok {
		return false, s.insertAutoChainSystemComment(ctx, tx, run.IssueID, message, at)
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
	// Hub-PM policy: main agent re-entry inherits the parent's chain_depth so
	// that worker→main→worker→main→… chains accumulate depth only on worker
	// dispatches. Worker dispatches still advance by 1, preserving the
	// max_depth gate for linear chains.
	depth := run.ChainDepth
	if !agent.IsMain {
		depth = run.ChainDepth + 1
	}
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
	return true, s.insertAutoChainSystemComment(ctx, tx, run.IssueID, "자동 체이닝으로 @"+agent.Name+" 실행을 큐에 등록했습니다.", at)
}

func (s *Store) checkAutoChainDispatchDuplicates(ctx context.Context, tx *sqlx.Tx, run Run, agent Agent, chainID string) (bool, string, error) {
	var existingQueued int
	if err := tx.GetContext(ctx, &existingQueued, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND status='queued'`, run.IssueID, agent.ID); err != nil {
		return false, "", normalizeErr(err)
	}
	if existingQueued > 0 {
		return false, "이미 @" + agent.Name + " queued run이 있어 자동 체이닝을 건너뛰었습니다.", nil
	}
	// Main agent (workspace PM hub) is allowed to re-enter the same chain so it
	// can orchestrate sequential worker delegations. Non-main agents stay blocked
	// from same-chain revisits to prevent loops. max_depth and daily guards still
	// apply to main agents as the safety net.
	if agent.IsMain {
		return true, "", nil
	}
	var duplicate int
	if err := tx.GetContext(ctx, &duplicate, `SELECT COUNT(*) FROM run WHERE issue_id=? AND agent_id=? AND (chain_id=? OR id=?)`, run.IssueID, agent.ID, chainID, chainID); err != nil {
		return false, "", normalizeErr(err)
	}
	if duplicate > 0 {
		return false, "자동 체이닝 중복 방지를 위해 @" + agent.Name + " 실행을 건너뛰었습니다.", nil
	}
	return true, "", nil
}

func (s *Store) insertAutoChainSystemComment(ctx context.Context, tx *sqlx.Tx, issueID, message, at string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO comment(id,issue_id,author_type,content,created_at) VALUES(?,?,'system',?,?)`, newID(), issueID, message, at)
	return normalizeErr(err)
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
