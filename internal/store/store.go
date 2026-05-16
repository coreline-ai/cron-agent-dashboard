package store

import (
	"database/sql"
	"errors"
	"fmt"
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
	switch {
	case strings.Contains(msg, "unique constraint") || strings.Contains(msg, "unique"):
		return fmt.Errorf("%w: %v", ErrConflict, err)
	case strings.Contains(msg, "check constraint") || strings.Contains(msg, "foreign key constraint") || strings.Contains(msg, "not null constraint"):
		return fmt.Errorf("%w: %v", ErrValidation, err)
	case strings.Contains(msg, "constraint"):
		return fmt.Errorf("%w: %v", ErrConflict, err)
	default:
		return err
	}
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
