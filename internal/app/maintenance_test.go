package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	assertModeOnDarwinLinux(t, backupDir, 0o700)
	assertModeOnDarwinLinux(t, runsDir, 0o700)
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

func TestPruneBackupsKeepGreaterThanFileCount(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "data-20260516T000000Z.db")
	if err := os.WriteFile(backup, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	deleted, err := PruneBackups(dir, 7)
	if err != nil {
		t.Fatalf("prune should not fail when keep exceeds file count: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0", deleted)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup should remain: %v", err)
	}
}

func TestRunMaintenanceOnceReportsPartialCleanupOnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-mode cleanup error test is Unix-oriented")
	}
	dir := t.TempDir()
	runsDir := filepath.Join(dir, "runs")
	blockedDir := filepath.Join(runsDir, "blocked")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(blockedDir, 0o755) }()
	oldLog := filepath.Join(runsDir, "old.log")
	if err := os.WriteFile(oldLog, []byte("old-log"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedLog := filepath.Join(blockedDir, "blocked.log")
	if err := os.WriteFile(blockedLog, []byte("blocked-log"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 15, 6, 0, 0, 0, time.UTC)
	oldTime := now.Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldLog, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(blockedLog, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatal(err)
	}

	report, err := RunMaintenanceOnce(t.Context(), nil, MaintenanceConfig{
		DataDir:            dir,
		AutoCleanupLogDays: 7,
		Now:                func() time.Time { return now },
	})
	if err == nil {
		t.Skip("filesystem allowed traversal despite removed permissions")
	}
	if report.DeletedLogFiles != 1 || report.FreedLogBytes != int64(len("old-log")) {
		t.Fatalf("partial cleanup report should be preserved, report=%#v err=%v", report, err)
	}
}

func TestMaintenanceRunnerStartStopIsIdempotent(t *testing.T) {
	runner := NewMaintenanceRunner(nil, MaintenanceConfig{
		DataDir:  t.TempDir(),
		Interval: time.Hour,
	})
	if err := runner.Stop(t.Context()); err != nil {
		t.Fatalf("stop before start: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	runner.Start(ctx)
	runner.Start(ctx)
	cancel()

	stopCtx, stopCancel := context.WithTimeout(t.Context(), time.Second)
	defer stopCancel()
	if err := runner.Stop(stopCtx); err != nil {
		t.Fatalf("stop after repeated start: %v", err)
	}
	if err := runner.Stop(t.Context()); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func assertModeOnDarwinLinux(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode=%#o, want %#o", path, got, want)
	}
}
