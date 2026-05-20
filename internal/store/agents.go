package store

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
)

func insertAgentInstructionVersionTx(ctx context.Context, tx *sqlx.Tx, agentID string, version int, instructions, createdAt string) error {
	if version <= 0 {
		version = 1
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_instruction_version(id,agent_id,version,instructions,created_at) VALUES(?,?,?,?,?)`, newID(), agentID, version, instructions, createdAt)
	return normalizeErr(err)
}

func agentInstructionsVersionForAgent(ctx context.Context, q sqlx.QueryerContext, agentID string) (int, error) {
	var version int
	if err := sqlx.GetContext(ctx, q, &version, `SELECT COALESCE(instructions_version,1) FROM agent WHERE id=?`, agentID); err != nil {
		return 0, normalizeErr(err)
	}
	if version <= 0 {
		version = 1
	}
	return version, nil
}

const agentSelectBase = `SELECT id,workspace_id,name,runtime,model,instructions,
       COALESCE(instructions_version,1) AS instructions_version,
       COALESCE(summary,'') AS summary,COALESCE(tags,'') AS tags,
       is_main,timeout_seconds_override,retry_policy_json,created_at,updated_at
FROM agent`

func validateAgent(in CreateAgentInput) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Runtime) == "" || strings.TrimSpace(in.Instructions) == "" {
		return ErrValidation
	}
	_, _, err := normalizeAgentControls(in)
	return err
}

func normalizeAgentControls(in CreateAgentInput) (timeout any, retryPolicyJSON string, err error) {
	if in.TimeoutSecondsOverride != nil {
		if *in.TimeoutSecondsOverride < 0 || *in.TimeoutSecondsOverride > 86400 {
			return nil, "", ErrValidation
		}
		if *in.TimeoutSecondsOverride > 0 {
			timeout = *in.TimeoutSecondsOverride
		}
	}
	retryPolicyJSON = strings.TrimSpace(in.RetryPolicyJSON)
	if retryPolicyJSON == "" {
		retryPolicyJSON = `{"max_attempts":1}`
	}
	if _, err := parseRetryPolicy(retryPolicyJSON); err != nil {
		return nil, "", err
	}
	return timeout, retryPolicyJSON, nil
}

func (s *Store) CreateAgent(ctx context.Context, workspaceID string, in CreateAgentInput) (Agent, error) {
	if err := validateAgent(in); err != nil {
		return Agent{}, err
	}
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return Agent{}, err
	}
	timeout, retryPolicy, err := normalizeAgentControls(in)
	if err != nil {
		return Agent{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Agent{}, err
	}
	defer tx.Rollback()
	t := now()
	a := Agent{ID: newID(), WorkspaceID: workspaceID, Name: in.Name, Runtime: in.Runtime, Model: in.Model, Instructions: in.Instructions, InstructionsVersion: 1, Summary: in.Summary, Tags: in.Tags, RetryPolicyJSON: retryPolicy, CreatedAt: t, UpdatedAt: t}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent(id,workspace_id,name,runtime,model,instructions,instructions_version,summary,tags,is_main,timeout_seconds_override,retry_policy_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,0,?,?,?,?)`, a.ID, a.WorkspaceID, a.Name, a.Runtime, a.Model, a.Instructions, a.InstructionsVersion, a.Summary, a.Tags, timeout, retryPolicy, t, t)
	if err != nil {
		return Agent{}, normalizeErr(err)
	}
	if err := insertAgentInstructionVersionTx(ctx, tx, a.ID, a.InstructionsVersion, a.Instructions, t); err != nil {
		return Agent{}, err
	}
	if err := tx.Commit(); err != nil {
		return Agent{}, err
	}
	return s.GetAgent(ctx, a.ID)
}

// ListAgentActivity returns a per-agent snapshot of the most recent run so the
// Home page Team Pulse widget can show who is busy at a glance. The latest
// run is determined by enqueued_at DESC. Agents without any run history yield
// rows with empty LatestRun* fields.
func (s *Store) ListAgentActivity(ctx context.Context, workspaceID string) ([]AgentActivity, error) {
	const q = `
SELECT
  a.id  AS agent_id,
  a.name AS agent_name,
  a.runtime,
  a.is_main,
  COALESCE(r.id,'')           AS latest_run_id,
  COALESCE(r.status,'')        AS latest_run_status,
  COALESCE(r.finished_at,'')   AS latest_run_finished_at,
  COALESCE(r.enqueued_at,'')   AS latest_run_enqueued_at,
  COALESCE(i.id,'')            AS latest_issue_id,
  COALESCE(i.identifier,'')    AS latest_issue_identifier
FROM agent a
LEFT JOIN (
  SELECT r1.* FROM run r1
  JOIN (SELECT agent_id, MAX(enqueued_at) AS mx FROM run GROUP BY agent_id) r2
    ON r1.agent_id = r2.agent_id AND r1.enqueued_at = r2.mx
) r ON r.agent_id = a.id
LEFT JOIN issue i ON i.id = r.issue_id
WHERE a.workspace_id = ?
ORDER BY a.is_main DESC, LOWER(a.name)`
	var out []AgentActivity
	err := s.db.SelectContext(ctx, &out, q, workspaceID)
	return out, normalizeErr(err)
}

func (s *Store) ListAgents(ctx context.Context, workspaceID string) ([]Agent, error) {
	var out []Agent
	err := s.db.SelectContext(ctx, &out, agentSelectBase+` WHERE workspace_id=? ORDER BY is_main DESC, created_at ASC`, workspaceID)
	return out, normalizeErr(err)
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, agentSelectBase+` WHERE id=?`, id)
	return a, normalizeErr(err)
}

func (s *Store) GetMainAgent(ctx context.Context, workspaceID string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, agentSelectBase+` WHERE workspace_id=? AND is_main=1`, workspaceID)
	return a, normalizeErr(err)
}

func (s *Store) FindAgentByName(ctx context.Context, workspaceID, name string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, agentSelectBase+` WHERE workspace_id=? AND lower(name)=lower(?)`, workspaceID, name)
	return a, normalizeErr(err)
}

func (s *Store) UpdateAgent(ctx context.Context, id string, in CreateAgentInput) (Agent, error) {
	if err := validateAgent(in); err != nil {
		return Agent{}, err
	}
	timeout, retryPolicy, err := normalizeAgentControls(in)
	if err != nil {
		return Agent{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Agent{}, err
	}
	defer tx.Rollback()
	var current Agent
	if err := tx.GetContext(ctx, &current, agentSelectBase+` WHERE id=?`, id); err != nil {
		return Agent{}, normalizeErr(err)
	}
	version := current.InstructionsVersion
	if version <= 0 {
		version = 1
	}
	changedInstructions := current.Instructions != in.Instructions
	if changedInstructions {
		version++
	}
	t := now()
	_, err = tx.ExecContext(ctx, `UPDATE agent SET name=?, runtime=?, model=?, instructions=?, instructions_version=?, summary=?, tags=?, timeout_seconds_override=?, retry_policy_json=?, updated_at=? WHERE id=?`, in.Name, in.Runtime, in.Model, in.Instructions, version, in.Summary, in.Tags, timeout, retryPolicy, t, id)
	if err != nil {
		return Agent{}, normalizeErr(err)
	}
	if changedInstructions {
		if err := insertAgentInstructionVersionTx(ctx, tx, id, version, in.Instructions, t); err != nil {
			return Agent{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Agent{}, err
	}
	return s.GetAgent(ctx, id)
}

func (s *Store) ListAgentInstructionVersions(ctx context.Context, agentID string) ([]AgentInstructionVersion, error) {
	if _, err := s.GetAgent(ctx, agentID); err != nil {
		return nil, err
	}
	var versions []AgentInstructionVersion
	err := s.db.SelectContext(ctx, &versions, `SELECT id,agent_id,version,instructions,created_at FROM agent_instruction_version WHERE agent_id=? ORDER BY version DESC`, agentID)
	return versions, normalizeErr(err)
}

func (s *Store) PromoteAgent(ctx context.Context, id string) (Agent, error) {
	a, err := s.GetAgent(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	if a.IsMain {
		return Agent{}, ErrState
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Agent{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE agent SET is_main=0, updated_at=? WHERE workspace_id=?`, now(), a.WorkspaceID); err != nil {
		return Agent{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent SET is_main=1, updated_at=? WHERE id=?`, now(), a.ID); err != nil {
		return Agent{}, normalizeErr(err)
	}
	if err := tx.Commit(); err != nil {
		return Agent{}, err
	}
	return s.GetAgent(ctx, id)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	a, err := s.GetAgent(ctx, id)
	if err != nil {
		return err
	}
	if a.IsMain {
		return ErrState
	}
	var n int
	if err := s.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM run WHERE agent_id=?`, id); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent WHERE id=?`, id)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}
