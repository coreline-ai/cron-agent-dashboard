package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
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
	pruned, err := PruneStaleWorktrees(t.Context(), dataDir, cutoff, nil)
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

func TestRunMaintenanceOncePrunesTerminalAndOrphanWorktreesButProtectsActiveRuns(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	workspace, _, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "GC Guard",
		Slug:             "gc-guard",
		IdentifierPrefix: "GC",
		PerRunWorktree:   true,
		MainAgent:        store.CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, runningRun, err := st.CreateIssueWithInitialRun(ctx, workspace.ID, store.CreateIssueInput{Title: "running stays"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, ok, err := st.ClaimNextRun(ctx, "gc-test-worker"); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to claim running fixture")
	} else if claimed.ID != runningRun.ID {
		t.Fatalf("claimed run=%s want %s", claimed.ID, runningRun.ID)
	}
	_, queuedRun, err := st.CreateIssueWithInitialRun(ctx, workspace.ID, store.CreateIssueInput{Title: "queued stays"})
	if err != nil {
		t.Fatal(err)
	}
	_, terminalRun, err := st.CreateIssueWithInitialRun(ctx, workspace.ID, store.CreateIssueInput{Title: "terminal prunes"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE run SET status='done', exit_code=0, finished_at=? WHERE id=?`, time.Now(), terminalRun.ID); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	runningDir := filepath.Join(dataDir, "worktrees", workspace.Slug, runningRun.ID)
	queuedDir := filepath.Join(dataDir, "worktrees", workspace.Slug, queuedRun.ID)
	terminalDir := filepath.Join(dataDir, "worktrees", workspace.Slug, terminalRun.ID)
	orphanDir := filepath.Join(dataDir, "worktrees", workspace.Slug, "orphan-old")
	payloads := map[string]string{
		runningDir:  "running",
		queuedDir:   "queued",
		terminalDir: "terminal",
		orphanDir:   "orphan",
	}
	old := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	for dir, payload := range payloads {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload.txt"), []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}

	report, err := RunMaintenanceOnce(ctx, nil, MaintenanceConfig{
		DataDir:            dataDir,
		AutoBackup:         false,
		WorktreeGCAfter:    24 * time.Hour,
		WorktreePruneGuard: st.IsRunWorktreeGCProtected,
		Now:                func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("RunMaintenanceOnce: %v", err)
	}
	if report.PrunedWorktrees != 2 {
		t.Fatalf("PrunedWorktrees=%d want 2", report.PrunedWorktrees)
	}
	if _, err := os.Stat(queuedDir); err != nil {
		t.Fatalf("queued run worktree should be protected: %v", err)
	}
	if _, err := os.Stat(runningDir); err != nil {
		t.Fatalf("running run worktree should be protected: %v", err)
	}
	if _, err := os.Stat(terminalDir); !os.IsNotExist(err) {
		t.Fatalf("terminal old worktree should be pruned, stat err=%v", err)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("orphan old worktree should be pruned, stat err=%v", err)
	}
	if report.WorktreeDirCount != 2 {
		t.Fatalf("WorktreeDirCount=%d want 2 protected active dirs", report.WorktreeDirCount)
	}
	wantBytes := int64(len("queued") + len("running"))
	if report.WorktreeBytes != wantBytes {
		t.Fatalf("WorktreeBytes=%d want %d", report.WorktreeBytes, wantBytes)
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
