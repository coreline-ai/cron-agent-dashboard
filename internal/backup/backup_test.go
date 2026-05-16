package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDatabaseCopiesFileAndCreatesDestinationDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data.db")
	if err := os.WriteFile(src, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "nested", "backup.db")
	result, err := Database(t.Context(), nil, src, dst, time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != dst || result.SizeBytes != int64(len("sqlite")) {
		t.Fatalf("result=%#v", result)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sqlite" {
		t.Fatalf("backup content=%q", got)
	}
	assertModeOnDarwinLinux(t, filepath.Dir(dst), 0o700)
	assertModeOnDarwinLinux(t, dst, 0o600)
}

func TestRestoreKeepsPreRestoreCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "backup.db")
	dst := filepath.Join(dir, "data.db")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	preRestore, err := Restore(src, dst, time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if preRestore == "" {
		t.Fatal("expected pre-restore copy path")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("restored content=%q", got)
	}
	old, err := os.ReadFile(preRestore)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "old" {
		t.Fatalf("pre-restore content=%q", old)
	}
}

func TestDatabaseTightensExistingBackupDirAndOutputFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data.db")
	if err := os.WriteFile(src, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(backupDir, "backup.db")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Database(t.Context(), nil, src, dst, time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	assertModeOnDarwinLinux(t, backupDir, 0o700)
	assertModeOnDarwinLinux(t, dst, 0o600)
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
