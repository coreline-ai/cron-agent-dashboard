package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrWorktreeInvalidInput is returned when AllocateRunWorktree is called with
// empty data dir / workspace slug / run id arguments.
var ErrWorktreeInvalidInput = errors.New("worktree: invalid input")

// AllocateRunWorktree provisions a per-run isolation directory for a workspace
// that opted into per_run_worktree. The directory layout is:
//
//	<dataDir>/worktrees/<workspaceSlug>/<runID>/
//
// The directory is created with 0700 permissions so runs on multi-user hosts
// do not leak prompt/scratch files to other accounts. Callers must invoke the
// returned cleanup once the run finishes (success or failure) — its
// RemoveAll error is intentionally non-fatal at the call site because the
// directory lives under data-dir and will be cleaned up by maintenance later
// in the worst case.
//
// The helper is git-agnostic on purpose. `git worktree add/remove` integration
// is tracked as a follow-up plan; that variant will need workspace.working_dir
// to point at a git repo and a branch/HEAD policy.
func AllocateRunWorktree(dataDir, workspaceSlug, runID string) (path string, cleanup func() error, err error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("dataDir empty: %w", ErrWorktreeInvalidInput)
	}
	if strings.TrimSpace(workspaceSlug) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("workspaceSlug empty: %w", ErrWorktreeInvalidInput)
	}
	if strings.TrimSpace(runID) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("runID empty: %w", ErrWorktreeInvalidInput)
	}
	path = filepath.Join(dataDir, "worktrees", workspaceSlug, runID)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", nopWorktreeCleanup, fmt.Errorf("worktree: mkdir %s: %w", path, err)
	}
	return path, func() error {
		return os.RemoveAll(path)
	}, nil
}

func nopWorktreeCleanup() error { return nil }
