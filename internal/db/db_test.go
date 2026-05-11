package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrateAppliesSchemaAndPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	database, err := OpenAndMigrate(path)
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	defer database.Close()

	var migrations int
	if err := database.Get(&migrations, `SELECT COUNT(*) FROM schema_migrations`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if migrations < 2 {
		t.Fatalf("expected migrations applied, got %d", migrations)
	}

	var foreignKeys int
	if err := database.Get(&foreignKeys, `PRAGMA foreign_keys`); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d, want 1", foreignKeys)
	}

	var journal string
	if err := database.Get(&journal, `PRAGMA journal_mode`); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode=%q, want wal", journal)
	}
}
