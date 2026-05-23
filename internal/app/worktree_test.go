package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAllocateRunWorktreeCreatesIsolatedDirectoryAndCleansUp(t *testing.T) {
	dataDir := t.TempDir()
	const slug = "demo-studio"
	const runID = "11111111-2222-3333-4444-555555555555"

	path, cleanup, err := AllocateRunWorktree(dataDir, slug, runID, "")
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	want := filepath.Join(dataDir, "worktrees", slug, runID)
	if path != want {
		t.Fatalf("worktree path=%q want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat worktree: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("worktree path is not a directory: %v", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("worktree perm=%#o, want 0700", got)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after cleanup: err=%v", err)
	}
}

func TestAllocateRunWorktreeIsIdempotentBeforeCleanup(t *testing.T) {
	dataDir := t.TempDir()
	first, _, err := AllocateRunWorktree(dataDir, "ws", "run-1", "")
	if err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	if f, err := os.Create(filepath.Join(first, "scratch.txt")); err == nil {
		f.Close()
	}
	second, cleanup, err := AllocateRunWorktree(dataDir, "ws", "run-1", "")
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}
	if first != second {
		t.Fatalf("second allocate returned different path: %q vs %q", second, first)
	}
	// Pre-existing file should still be there — MkdirAll is non-destructive.
	if _, err := os.Stat(filepath.Join(second, "scratch.txt")); err != nil {
		t.Fatalf("idempotent allocate should not wipe contents: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestAllocateRunWorktreeRejectsEmptyInputs(t *testing.T) {
	cases := []struct {
		name    string
		dataDir string
		slug    string
		runID   string
	}{
		{"empty data dir", "", "ws", "run"},
		{"empty slug", "/tmp", "", "run"},
		{"empty run id", "/tmp", "ws", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := AllocateRunWorktree(c.dataDir, c.slug, c.runID, "")
			if !errors.Is(err, ErrWorktreeInvalidInput) {
				t.Fatalf("expected ErrWorktreeInvalidInput, got %v", err)
			}
		})
	}
}
