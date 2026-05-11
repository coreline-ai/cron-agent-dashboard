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

	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrState      = errors.New("state error")
	ErrValidation = errors.New("validation error")
)

type Store struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Store  { return &Store{db: db} }
func (s *Store) DB() *sqlx.DB { return s.db }

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
	return nil
}

const workspaceSelect = `
SELECT w.id, w.name, w.slug, w.description, w.output_dir, w.working_dir, w.identifier_prefix, w.next_issue_seq,
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
	w := Workspace{ID: newID(), Name: in.Name, Slug: in.Slug, Description: in.Description, OutputDir: in.OutputDir, WorkingDir: in.WorkingDir, IdentifierPrefix: in.IdentifierPrefix, NextIssueSeq: 1, CreatedAt: t, UpdatedAt: t}
	_, err = tx.ExecContext(ctx, `INSERT INTO workspace(id,name,slug,description,output_dir,working_dir,identifier_prefix,next_issue_seq,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, w.ID, w.Name, w.Slug, w.Description, w.OutputDir, w.WorkingDir, w.IdentifierPrefix, w.NextIssueSeq, t, t)
	if err != nil {
		return Workspace{}, Agent{}, normalizeErr(err)
	}
	a := Agent{ID: newID(), WorkspaceID: w.ID, Name: in.MainAgent.Name, Runtime: in.MainAgent.Runtime, Model: in.MainAgent.Model, Instructions: in.MainAgent.Instructions, IsMain: true, CreatedAt: t, UpdatedAt: t}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent(id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, a.ID, a.WorkspaceID, a.Name, a.Runtime, a.Model, a.Instructions, 1, t, t)
	if err != nil {
		return Workspace{}, Agent{}, normalizeErr(err)
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

func (s *Store) UpdateWorkspace(ctx context.Context, idOrSlug string, name, description, workingDir, outputDir string) (Workspace, error) {
	w, _, err := s.GetWorkspace(ctx, idOrSlug)
	if err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(name) == "" {
		return Workspace{}, ErrValidation
	}
	_, err = s.db.ExecContext(ctx, `UPDATE workspace SET name=?, description=?, working_dir=?, output_dir=?, updated_at=? WHERE id=?`, name, description, workingDir, outputDir, now(), w.ID)
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
	var n int
	if err := s.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM run r JOIN issue i ON i.id=r.issue_id WHERE i.workspace_id=? AND r.status='running'`, w.ID); err != nil {
		return err
	}
	if n > 0 {
		return ErrState
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspace WHERE id=?`, w.ID)
	if err != nil {
		return normalizeErr(err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAgent(ctx context.Context, workspaceID string, in CreateAgentInput) (Agent, error) {
	if err := validateAgent(in); err != nil {
		return Agent{}, err
	}
	if _, _, err := s.GetWorkspace(ctx, workspaceID); err != nil {
		return Agent{}, err
	}
	t := now()
	a := Agent{ID: newID(), WorkspaceID: workspaceID, Name: in.Name, Runtime: in.Runtime, Model: in.Model, Instructions: in.Instructions, CreatedAt: t, UpdatedAt: t}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent(id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at) VALUES(?,?,?,?,?,?,0,?,?)`, a.ID, a.WorkspaceID, a.Name, a.Runtime, a.Model, a.Instructions, t, t)
	if err != nil {
		return Agent{}, normalizeErr(err)
	}
	return s.GetAgent(ctx, a.ID)
}

func (s *Store) ListAgents(ctx context.Context, workspaceID string) ([]Agent, error) {
	var out []Agent
	err := s.db.SelectContext(ctx, &out, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE workspace_id=? ORDER BY is_main DESC, created_at ASC`, workspaceID)
	return out, normalizeErr(err)
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE id=?`, id)
	return a, normalizeErr(err)
}

func (s *Store) GetMainAgent(ctx context.Context, workspaceID string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE workspace_id=? AND is_main=1`, workspaceID)
	return a, normalizeErr(err)
}

func (s *Store) FindAgentByName(ctx context.Context, workspaceID, name string) (Agent, error) {
	var a Agent
	err := s.db.GetContext(ctx, &a, `SELECT id,workspace_id,name,runtime,model,instructions,is_main,created_at,updated_at FROM agent WHERE workspace_id=? AND lower(name)=lower(?)`, workspaceID, name)
	return a, normalizeErr(err)
}

func (s *Store) UpdateAgent(ctx context.Context, id string, in CreateAgentInput) (Agent, error) {
	if err := validateAgent(in); err != nil {
		return Agent{}, err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE agent SET name=?, runtime=?, model=?, instructions=?, updated_at=? WHERE id=?`, in.Name, in.Runtime, in.Model, in.Instructions, now(), id)
	if err != nil {
		return Agent{}, normalizeErr(err)
	}
	return s.GetAgent(ctx, id)
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
