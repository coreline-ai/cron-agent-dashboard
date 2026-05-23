package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Track A of dev-plan/implement_20260523_092408.md.
//
// WorktreeDiskUsage walks the canonical <data>/worktrees/<slug>/<runID>/
// layout produced by AllocateRunWorktree and returns the total bytes plus
// the number of run-scoped directories. PruneStaleWorktrees removes
// run-scoped directories whose mtime is older than the cutoff, which is
// the maintenance runner's signal that the run is long-terminal.
func TestWorktreeDiskUsageSumsAndCountsRunDirs(t *testing.T) {
	dataDir := t.TempDir()
	worktrees := filepath.Join(dataDir, "worktrees", "demo")
	if err := os.MkdirAll(filepath.Join(worktrees, "run-A"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktrees, "run-A", "scratch.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(worktrees, "run-B", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktrees, "run-B", "nested", "big.bin"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	bytes, dirs, err := WorktreeDiskUsage(dataDir)
	if err != nil {
		t.Fatalf("WorktreeDiskUsage: %v", err)
	}
	if dirs != 2 {
		t.Fatalf("dir count=%d want 2", dirs)
	}
	if bytes != 1024+5 {
		t.Fatalf("byte count=%d want %d", bytes, 1024+5)
	}
}

func TestWorktreeDiskUsageMissingDirIsZero(t *testing.T) {
	dataDir := t.TempDir()
	bytes, dirs, err := WorktreeDiskUsage(dataDir)
	if err != nil {
		t.Fatalf("WorktreeDiskUsage on empty dataDir: %v", err)
	}
	if bytes != 0 || dirs != 0 {
		t.Fatalf("expected zero bytes/dirs, got %d / %d", bytes, dirs)
	}
}

func TestPruneStaleWorktreesRemovesOldDirsOnly(t *testing.T) {
	dataDir := t.TempDir()
	worktrees := filepath.Join(dataDir, "worktrees", "demo")
	oldDir := filepath.Join(worktrees, "run-old")
	freshDir := filepath.Join(worktrees, "run-fresh")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(freshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Backdate old dir 7 days.
	old := time.Now().Add(-7 * 24 * time.Hour)
	if err := os.Chtimes(oldDir, old, old); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	pruned, err := PruneStaleWorktrees(dataDir, cutoff)
	if err != nil {
		t.Fatalf("PruneStaleWorktrees: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned=%d want 1", pruned)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old worktree still exists: %v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("fresh worktree should remain: %v", err)
	}
}

func TestRunMaintenanceOnceRecordsWorktreeFields(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "worktrees", "ws", "run-X"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "worktrees", "ws", "run-X", "f"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := RunMaintenanceOnce(t.Context(), nil, MaintenanceConfig{
		DataDir:         dataDir,
		AutoBackup:      false,
		WorktreeGCAfter: 24 * time.Hour, // does not prune the just-created dir
		Now:             func() time.Time { return time.Now() },
	})
	if err != nil {
		t.Fatalf("RunMaintenanceOnce: %v", err)
	}
	if report.WorktreeDirCount != 1 {
		t.Fatalf("WorktreeDirCount=%d want 1", report.WorktreeDirCount)
	}
	if report.WorktreeBytes != int64(len("payload")) {
		t.Fatalf("WorktreeBytes=%d want %d", report.WorktreeBytes, len("payload"))
	}
	if report.PrunedWorktrees != 0 {
		t.Fatalf("PrunedWorktrees=%d want 0 (dir is fresh)", report.PrunedWorktrees)
	}
}
