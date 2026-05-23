package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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

const defaultAutopilotFailureDisableThreshold = 5

type Option func(*Store)

type Store struct {
	db                               *sqlx.DB
	autopilotFailureDisableThreshold int
	runEventNotifier                 RunEventNotifier
}

// RunEventNotifier is invoked by the store after a successful run_event
// INSERT so the HTTP SSE handler can push the new row to subscribers
// instead of polling. Implementations must be goroutine-safe and must not
// block — store.AppendRunEvent calls OnRunEvent from the transaction's
// commit path.
type RunEventNotifier interface {
	OnRunEvent(issueID, runID string)
}

// SetRunEventNotifier wires an in-process notifier into the store. Passing
// nil clears the previous notifier. Optional — when unset, store.AppendRunEvent
// is a no-op on the notifier path and clients fall back to whatever polling
// they were using.
func (s *Store) SetRunEventNotifier(n RunEventNotifier) {
	if s == nil {
		return
	}
	s.runEventNotifier = n
}

// notifyRunEvent is the package-internal hook used by transactional code
// paths (cancel, complete, infrastructure-fail, orphan-recover, auto-chain
// dispatch, mention dispatch) to fire the notifier after their tx commit
// succeeds. Nil-safe so call sites can invoke it unconditionally.
func (s *Store) notifyRunEvent(issueID, runID string) {
	if s == nil || s.runEventNotifier == nil {
		return
	}
	s.runEventNotifier.OnRunEvent(issueID, runID)
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
	return safeUTF8Cap(v, max)
}

// safeUTF8Cap returns s with any invalid UTF-8 byte sequences replaced by
// U+FFFD and the result truncated to at most maxBytes without splitting a
// rune. Truncation past valid input is byte-exact (no extra padding).
func safeUTF8Cap(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
