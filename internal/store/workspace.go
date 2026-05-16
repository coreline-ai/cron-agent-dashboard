package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var (
	slugRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,49}$`)
	prefixRE = regexp.MustCompile(`^[A-Z]{2,10}$`)
	uuidRE   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

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
