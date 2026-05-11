package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDataDirUpdatesDefaultDBPath(t *testing.T) {
	dir := t.TempDir()
	cfg, _, err := Load([]string{"--data-dir", dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.DBPath, filepath.Join(dir, "data.db"); got != want {
		t.Fatalf("DBPath=%q, want %q", got, want)
	}
}

func TestLoadEnvDataDirUpdatesDefaultDBPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CORN_AGENT_DASHBOARD_DATA_DIR", dir)

	cfg, _, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.DBPath, filepath.Join(dir, "data.db"); got != want {
		t.Fatalf("DBPath=%q, want %q", got, want)
	}
}

func TestLoadExplicitDBPathWinsOverDataDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "custom.db")
	cfg, _, err := Load([]string{"--data-dir", dir, "--db", dbPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBPath != dbPath {
		t.Fatalf("DBPath=%q, want %q", cfg.DBPath, dbPath)
	}
}
