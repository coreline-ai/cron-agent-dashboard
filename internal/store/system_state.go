package store

import (
	"context"
	"strings"
)

// GetSystemState reads a value from the system_state KV. Missing keys return
// the empty string without an error so callers can treat "never recorded"
// the same as "blank" without special-casing ErrNotFound.
func (s *Store) GetSystemState(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrValidation
	}
	var value string
	err := s.db.GetContext(ctx, &value, `SELECT value FROM system_state WHERE key=?`, key)
	if err != nil {
		if e := normalizeErr(err); e == ErrNotFound {
			return "", nil
		}
		return "", normalizeErr(err)
	}
	return value, nil
}

// SetSystemState upserts a KV pair. updated_at is refreshed on every write so
// readers can show "last X at" stamps without a separate column per key.
func (s *Store) SetSystemState(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrValidation
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO system_state(key, value, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, now(),
	)
	if err != nil {
		return normalizeErr(err)
	}
	return nil
}

// SystemStateEntry pairs a value with its updated_at stamp for read APIs that
// want to surface "last touched at" alongside the value.
type SystemStateEntry struct {
	Key       string `db:"key" json:"key"`
	Value     string `db:"value" json:"value"`
	UpdatedAt string `db:"updated_at" json:"updated_at"`
}

// GetSystemStateEntry returns both the value and the updated_at stamp. Missing
// keys yield an empty entry with no error, matching GetSystemState semantics.
func (s *Store) GetSystemStateEntry(ctx context.Context, key string) (SystemStateEntry, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return SystemStateEntry{}, ErrValidation
	}
	var entry SystemStateEntry
	err := s.db.GetContext(ctx, &entry, `SELECT key, value, updated_at FROM system_state WHERE key=?`, key)
	if err != nil {
		if e := normalizeErr(err); e == ErrNotFound {
			return SystemStateEntry{}, nil
		}
		return SystemStateEntry{}, normalizeErr(err)
	}
	return entry, nil
}

// Well-known system_state keys consumed across packages. Group here so a new
// writer is one constant declaration away from being discoverable.
const (
	SystemStateLastLogCleanupAt    = "last_log_cleanup_at"
	SystemStateLastLogCleanupFiles = "last_log_cleanup_files"
	SystemStateLastLogCleanupBytes = "last_log_cleanup_bytes"
)
