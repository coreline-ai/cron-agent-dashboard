package config

import (
	"path/filepath"
	"strings"
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

func TestLoadRejectsExternalBindWithoutToken(t *testing.T) {
	if _, _, err := Load([]string{"--bind", "0.0.0.0:8080"}); err == nil {
		t.Fatal("expected external bind without token to fail")
	}

	cfg, _, err := Load([]string{"--bind", "0.0.0.0:8080", "--token", "secret"})
	if err != nil {
		t.Fatalf("Load with token: %v", err)
	}
	if cfg.AuthMode() != "token" {
		t.Fatalf("AuthMode=%q, want token", cfg.AuthMode())
	}
}

func TestLoadAutopilotFailureThresholdFromFlagAndEnv(t *testing.T) {
	t.Setenv("CORN_AGENT_DASHBOARD_AUTOPILOT_FAILURE_DISABLE_THRESHOLD", "2")
	cfg, _, err := Load(nil)
	if err != nil {
		t.Fatalf("Load env: %v", err)
	}
	if got, want := cfg.AutopilotFailureDisableThreshold, 2; got != want {
		t.Fatalf("AutopilotFailureDisableThreshold=%d, want %d", got, want)
	}

	cfg, _, err = Load([]string{"--autopilot-failure-disable-threshold", "4"})
	if err != nil {
		t.Fatalf("Load flag: %v", err)
	}
	if got, want := cfg.AutopilotFailureDisableThreshold, 4; got != want {
		t.Fatalf("AutopilotFailureDisableThreshold=%d, want %d", got, want)
	}
}

func TestLoadAutopilotFailureThresholdFallsBackToDefault(t *testing.T) {
	cfg, _, err := Load([]string{"--autopilot-failure-disable-threshold", "0"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.AutopilotFailureDisableThreshold, DefaultAutopilotFailureDisableThreshold; got != want {
		t.Fatalf("AutopilotFailureDisableThreshold=%d, want %d", got, want)
	}
}

func TestLoadRejectsInvalidNumericEnv(t *testing.T) {
	t.Setenv("CORN_AGENT_DASHBOARD_WORKERS", "many")
	_, _, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid env to fail")
	}
	if !strings.Contains(err.Error(), "CORN_AGENT_DASHBOARD_WORKERS") {
		t.Fatalf("error=%v, want env key in message", err)
	}
}
