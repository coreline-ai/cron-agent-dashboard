package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Open(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func OpenAndMigrate(path string) (*sqlx.DB, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func applyPragmas(db *sqlx.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, q := range pragmas {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

type migration struct {
	Version int
	Name    string
	Path    string
}

func Migrate(db *sqlx.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		return err
	}
	migrations, err := listMigrations()
	if err != nil {
		return err
	}
	for _, m := range migrations {
		var exists int
		err := db.Get(&exists, `SELECT 1 FROM schema_migrations WHERE version = ?`, m.Version)
		if err == nil && exists == 1 {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		body, err := migrationFS.ReadFile(m.Path)
		if err != nil {
			return err
		}
		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", m.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`, m.Version, m.Name, Now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func listMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name %q", e.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid migration version %q: %w", e.Name(), err)
		}
		out = append(out, migration{Version: version, Name: strings.TrimSuffix(e.Name(), ".sql"), Path: filepath.Join("migrations", e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
