package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunMaintenanceOnceBacksUpPrunesAndCleansLogs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldBackup := filepath.Join(backupDir, "data-20260501T000000Z.db")
	if err := os.WriteFile(oldBackup, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldBackup, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	runsDir := filepath.Join(dir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldLog := filepath.Join(runsDir, "old.log")
	newLog := filepath.Join(runsDir, "new.log")
	if err := os.WriteFile(oldLog, []byte("old-log"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newLog, []byte("new-log"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 15, 6, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldLog, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	report, err := RunMaintenanceOnce(t.Context(), nil, MaintenanceConfig{
		DataDir:            dir,
		DBPath:             dbPath,
		AutoBackup:         true,
		AutoBackupKeep:     1,
		AutoCleanupLogDays: 7,
		Now:                func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.BackupPath == "" || report.BackupSizeBytes != 2 || report.PrunedBackups != 1 {
		t.Fatalf("backup report=%#v", report)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Fatalf("old backup should be pruned, stat err=%v", err)
	}
	if report.DeletedLogFiles != 1 || report.FreedLogBytes != int64(len("old-log")) {
		t.Fatalf("cleanup report=%#v", report)
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("old log should be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Fatalf("new log should remain: %v", err)
	}
}
