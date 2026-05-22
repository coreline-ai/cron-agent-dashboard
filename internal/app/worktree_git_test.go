package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllocateRunWorktreeGitMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available — skipping git worktree integration test")
	}

	repo := t.TempDir()
	dataDir := t.TempDir()
	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "config", "user.email", "ci@example.com")
	mustGit(t, repo, "config", "user.name", "ci")
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", "seed.txt")
	mustGit(t, repo, "commit", "-q", "-m", "seed")

	const runID = "11111111-2222-3333-4444-555555555555"
	path, cleanup, err := AllocateRunWorktree(dataDir, "demo", runID, repo)
	if err != nil {
		t.Fatalf("allocate (git mode): %v", err)
	}
	want := filepath.Join(dataDir, "worktrees", "demo", runID)
	if path != want {
		t.Fatalf("worktree path=%q want %q", path, want)
	}
	// `.git` should exist at the worktree root as a *file* (the gitfile
	// pointer to the main repo). If it were a directory, it would mean we
	// fell back to plain mkdir.
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		t.Fatalf("expected .git marker inside worktree: %v", err)
	}
	if info.IsDir() {
		t.Fatalf(".git inside worktree is a directory — git worktree add did not run")
	}
	// Seed file should be visible inside the worktree.
	if _, err := os.Stat(filepath.Join(path, "seed.txt")); err != nil {
		t.Fatalf("worktree missing seed.txt from HEAD: %v", err)
	}
	// `git worktree list` from the main repo must mention the new path.
	listOut := mustGit(t, repo, "worktree", "list")
	if !strings.Contains(listOut, path) {
		t.Fatalf("git worktree list does not mention %s:\n%s", path, listOut)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree still on disk after cleanup: err=%v", err)
	}
	// Registry should no longer mention the path either.
	if strings.Contains(mustGit(t, repo, "worktree", "list"), path) {
		t.Fatalf("worktree registry still references removed path %s", path)
	}
}

func TestAllocateRunWorktreeFallsBackWhenWorkingDirNotGitRepo(t *testing.T) {
	dataDir := t.TempDir()
	plainDir := t.TempDir()
	path, cleanup, err := AllocateRunWorktree(dataDir, "demo", "run-plain", plainDir)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		t.Fatalf("plain-dir fallback should not create a .git marker at %s", path)
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
	return string(out)
}
