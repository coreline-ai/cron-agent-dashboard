package db

import (
	"context"
	"errors"
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

func TestMigrationFailureMetadataRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()
	m := migration{Version: 999, Name: "bad_migration"}
	if err := recordMigrationFailure(database, m, errors.New("boom")); err != nil {
		t.Fatalf("recordMigrationFailure: %v", err)
	}
	failures, err := RecentMigrationFailures(context.Background(), database, 5)
	if err != nil {
		t.Fatalf("RecentMigrationFailures: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures=%#v", failures)
	}
	if failures[0].Version != 999 || failures[0].Name != "bad_migration" || failures[0].Error != "boom" || failures[0].FailedAt == "" {
		t.Fatalf("failure row=%#v", failures[0])
	}
}
