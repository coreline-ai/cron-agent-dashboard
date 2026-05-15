package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/coreline-ai/corn-agent-dashboard/internal/db"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrState      = errors.New("state error")
	ErrValidation = errors.New("validation error")
)

const defaultAutopilotFailureDisableThreshold = 5

type Option func(*Store)

type Store struct {
	db                               *sqlx.DB
	autopilotFailureDisableThreshold int
}

func WithAutopilotFailureDisableThreshold(threshold int) Option {
	return func(s *Store) {
		if threshold > 0 {
			s.autopilotFailureDisableThreshold = threshold
		}
	}
}

func New(db *sqlx.DB, opts ...Option) *Store {
	s := &Store{db: db, autopilotFailureDisableThreshold: defaultAutopilotFailureDisableThreshold}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Store) DB() *sqlx.DB { return s.db }

func (s *Store) autopilotFailureThreshold() int {
	if s != nil && s.autopilotFailureDisableThreshold > 0 {
		return s.autopilotFailureDisableThreshold
	}
	return defaultAutopilotFailureDisableThreshold
}

func newID() string { return uuid.NewString() }
func now() string   { return db.Now() }

func normalizeErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "constraint") || strings.Contains(msg, "unique") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

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

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func capSnapshot(v string) string {
	const max = 4000
	if len(v) <= max {
		return v
	}
	return v[:max]
}

var (
	slugRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,49}$`)
	prefixRE = regexp.MustCompile(`^[A-Z]{2,10}$`)
	uuidRE   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

const agentSelectBase = `SELECT id,workspace_id,name,runtime,model,instructions,
       COALESCE(instructions_version,1) AS instructions_version,
       COALESCE(summary,'') AS summary,COALESCE(tags,'') AS tags,
       is_main,timeout_seconds_override,retry_policy_json,created_at,updated_at
FROM agent`

func normalizeAutoChainMaxDepth(value int) int {
	if value <= 0 {
		return 5
	}
	if value > 20 {
		return 20
	}
	return value
}

func normalizeAutoChainDailyRunLimit(value int) int {
	if value <= 0 {
		return 0
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func normalizeAutoChainDailyCostMicros(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func validateWorkspace(in CreateWorkspaceInput) error {
	if strings.TrimSpace(in.Name) == "" || !slugRE.MatchString(in.Slug) || !prefixRE.MatchString(in.IdentifierPrefix) {
		return ErrValidation
	}
	return validateAgent(in.MainAgent)
}

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

func normalizeWorkspaceTimeout(seconds int) (int, error) {
	if seconds == 0 {
		return defaultWorkspaceTimeoutSeconds, nil
	}
	if seconds < 0 || seconds > 86400 {
		return 0, ErrValidation
	}
	return seconds, nil
}

const workspaceSelect = `
SELECT w.id, w.name, w.slug, w.description, w.output_dir, w.working_dir, w.identifier_prefix, w.next_issue_seq,
       COALESCE(w.default_timeout_seconds, 600) AS default_timeout_seconds,
       COALESCE(w.auto_chain_enabled, 0) AS auto_chain_enabled,
       COALESCE(w.auto_chain_max_depth, 5) AS auto_chain_max_depth,
       COALESCE(w.auto_chain_daily_run_limit, 20) AS auto_chain_daily_run_limit,
       COALESCE(w.auto_chain_daily_cost_micros, 0) AS auto_chain_daily_cost_micros,
       COALESCE(w.auto_chain_dry_run, 0) AS auto_chain_dry_run,
       w.created_at, w.updated_at,
       (SELECT COUNT(*) FROM agent a WHERE a.workspace_id = w.id) AS agent_count,
       (SELECT COUNT(*) FROM issue i WHERE i.workspace_id = w.id AND i.status = 'open') AS open_issue_count
FROM workspace w`

func (s *Store) CreateWorkspaceWithMainAgent(ctx context.Context, in CreateWorkspaceInput) (Workspace, Agent, error) {
	if err := validateWorkspace(in); err != nil {
		return Workspace{}, Agent{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Workspace{}, Agent{}, err
	}
	defer tx.Rollback()
	t := now()
	timeoutSeconds, err := normalizeWorkspaceTimeout(in.DefaultTimeoutSeconds)
	if err != nil {
		return Workspace{}, Agent{}, err
	}
	chainMaxDepth := normalizeAutoChainMaxDepth(in.AutoChainMaxDepth)
	chainRunLimitInput := 20
	if in.AutoChainDailyRunLimit != nil {
		chainRunLimitInput = *in.AutoChainDailyRunLimit
	}
	chainRunLimit := normalizeAutoChainDailyRunLimit(chainRunLimitInput)
	chainCostLimit := normalizeAutoChainDailyCostMicros(in.AutoChainDailyCostMicros)
	w := Workspace{ID: newID(), Name: in.Name, Slug: in.Slug, Description: in.Description, OutputDir: in.OutputDir, WorkingDir: in.WorkingDir, IdentifierPrefix: in.IdentifierPrefix, NextIssueSeq: 1, DefaultTimeoutSeconds: timeoutSeconds, AutoChainEnabled: in.AutoChainEnabled, AutoChainMaxDepth: chainMaxDepth, AutoChainDailyRunLimit: chainRunLimit, AutoChainDailyCostMicros: chainCostLimit, AutoChainDryRun: in.AutoChainDryRun, CreatedAt: t, UpdatedAt: t}
	_, err = tx.ExecContext(ctx, `INSERT INTO workspace(id,name,slug,description,output_dir,working_dir,identifier_prefix,next_issue_seq,default_timeout_seconds,auto_chain_enabled,auto_chain_max_depth,auto_chain_daily_run_limit,auto_chain_daily_cost_micros,auto_chain_dry_run,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, w.ID, w.Name, w.Slug, w.Description, w.OutputDir, w.WorkingDir, w.IdentifierPrefix, w.NextIssueSeq, w.DefaultTimeoutSeconds, boolInt(w.AutoChainEnabled), w.AutoChainMaxDepth, w.AutoChainDailyRunLimit, w.AutoChainDailyCostMicros, boolInt(w.AutoChainDryRun), t, t)
	if err != nil {
		return Workspace{}, Agent{}, normalizeErr(err)
	}
	agentTimeout, retryPolicy, err := normalizeAgentControls(in.MainAgent)
	if err != nil {
		return Workspace{}, Agent{}, err
	}
	a := Agent{ID: newID(), WorkspaceID: w.ID, Name: in.MainAgent.Name, Runtime: in.MainAgent.Runtime, Model: in.MainAgent.Model, Instructions: in.MainAgent.Instructions, InstructionsVersion: 1, Summary: in.MainAgent.Summary, Tags: in.MainAgent.Tags, IsMain: true, RetryPolicyJSON: retryPolicy, CreatedAt: t, UpdatedAt: t}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent(id,workspace_id,name,runtime,model,instructions,instructions_version,summary,tags,is_main,timeout_seconds_override,retry_policy_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.ID, a.WorkspaceID, a.Name, a.Runtime, a.Model, a.Instructions, a.InstructionsVersion, a.Summary, a.Tags, 1, agentTimeout, retryPolicy, t, t)
	if err != nil {
		return Workspace{}, Agent{}, normalizeErr(err)
	}
	if err := insertAgentInstructionVersionTx(ctx, tx, a.ID, a.InstructionsVersion, a.Instructions, t); err != nil {
		return Workspace{}, Agent{}, err
	}
	if err := tx.Commit(); err != nil {
		return Workspace{}, Agent{}, err
	}
	return s.GetWorkspace(ctx, w.ID)
}

func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var out []Workspace
	err := s.db.SelectContext(ctx, &out, workspaceSelect+` ORDER BY w.created_at DESC`)
	return out, normalizeErr(err)
}

func (s *Store) GetWorkspace(ctx context.Context, idOrSlug string) (Workspace, Agent, error) {
	var w Workspace
	where := ` WHERE w.slug = ?`
	if uuidRE.MatchString(idOrSlug) {
		where = ` WHERE w.id = ?`
	}
	if err := s.db.GetContext(ctx, &w, workspaceSelect+where, idOrSlug); err != nil {
		return Workspace{}, Agent{}, normalizeErr(err)
	}
	a, err := s.GetMainAgent(ctx, w.ID)
	if errors.Is(err, ErrNotFound) {
		return w, Agent{}, nil
	}
	return w, a, err
}

func (s *Store) UpdateWorkspace(ctx context.Context, idOrSlug string, in UpdateWorkspaceInput) (Workspace, error) {
	w, _, err := s.GetWorkspace(ctx, idOrSlug)
	if err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return Workspace{}, ErrValidation
	}
	timeoutSeconds := w.DefaultTimeoutSeconds
	if in.DefaultTimeoutSeconds != nil {
		var err error
		timeoutSeconds, err = normalizeWorkspaceTimeout(*in.DefaultTimeoutSeconds)
		if err != nil {
			return Workspace{}, err
		}
	}
	autoChain := w.AutoChainEnabled
	if in.AutoChainEnabled != nil {
		autoChain = *in.AutoChainEnabled
	}
	maxDepth := w.AutoChainMaxDepth
	if in.AutoChainMaxDepth != nil {
		maxDepth = normalizeAutoChainMaxDepth(*in.AutoChainMaxDepth)
	}
	dailyRunLimit := w.AutoChainDailyRunLimit
	if in.AutoChainDailyRunLimit != nil {
		dailyRunLimit = normalizeAutoChainDailyRunLimit(*in.AutoChainDailyRunLimit)
	}
	dailyCostLimit := w.AutoChainDailyCostMicros
	if in.AutoChainDailyCostMicros != nil {
		dailyCostLimit = normalizeAutoChainDailyCostMicros(*in.AutoChainDailyCostMicros)
	}
	dryRun := w.AutoChainDryRun
	if in.AutoChainDryRun != nil {
		dryRun = *in.AutoChainDryRun
	}
	_, err = s.db.ExecContext(ctx, `UPDATE workspace SET name=?, description=?, working_dir=?, output_dir=?, default_timeout_seconds=?, auto_chain_enabled=?, auto_chain_max_depth=?, auto_chain_daily_run_limit=?, auto_chain_daily_cost_micros=?, auto_chain_dry_run=?, updated_at=? WHERE id=?`, in.Name, in.Description, in.WorkingDir, in.OutputDir, timeoutSeconds, boolInt(autoChain), maxDepth, dailyRunLimit, dailyCostLimit, boolInt(dryRun), now(), w.ID)
	if err != nil {
		return Workspace{}, normalizeErr(err)
	}
	w, _, err = s.GetWorkspace(ctx, w.ID)
	return w, err
}

func (s *Store) DeleteWorkspace(ctx context.Context, idOrSlug string) error {
	w, _, err := s.GetWorkspace(ctx, idOrSlug)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.GetContext(ctx, &n, `SELECT COUNT(*) FROM run r JOIN issue i ON i.id=r.issue_id WHERE i.workspace_id=? AND r.status IN ('queued','running')`, w.ID); err != nil {
		return err
	}
	if n > 0 {
		return ErrState
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM workspace WHERE id=?`, w.ID)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return tx.Commit()
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
