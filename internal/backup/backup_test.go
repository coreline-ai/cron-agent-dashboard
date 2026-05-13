package backup

import (
	"os"
	"path/filepath"
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
