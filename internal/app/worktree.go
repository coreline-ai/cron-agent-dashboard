package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrWorktreeInvalidInput is returned when AllocateRunWorktree is called with
// empty data dir / workspace slug / run id arguments.
var ErrWorktreeInvalidInput = errors.New("worktree: invalid input")

// AllocateRunWorktree provisions a per-run isolation directory for a workspace
// that opted into per_run_worktree.
//
// When workingDir points at a git repository (i.e. a `.git` file or directory
// is present at its root), the path is created via `git worktree add <path>
// HEAD` so the run sees a real working tree branched from the main repo's
// current HEAD. The returned cleanup runs `git worktree remove --force <path>`
// before falling back to RemoveAll, which keeps the worktree registry in sync
// even when the run terminates abnormally.
//
// When workingDir is empty or does not contain a `.git` entry, the directory
// is created with mkdir (0700) and the cleanup is a simple RemoveAll. This
// preserves the v1 behavior used by workspaces without a git-backed
// working_dir.
//
// The git mode is opportunistic: if the `git` binary is missing or the
// add/remove commands fail, the helper falls back to plain mkdir so a missing
// git installation never blocks a run. The unhappy path is observable via
// the worktreeAddOpts hook in tests.
func AllocateRunWorktree(dataDir, workspaceSlug, runID, workingDir string) (path string, cleanup func() error, err error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("dataDir empty: %w", ErrWorktreeInvalidInput)
	}
	if strings.TrimSpace(workspaceSlug) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("workspaceSlug empty: %w", ErrWorktreeInvalidInput)
	}
	if strings.TrimSpace(runID) == "" {
		return "", nopWorktreeCleanup, fmt.Errorf("runID empty: %w", ErrWorktreeInvalidInput)
	}
	// Worker adapters (notably codex `exec --cd`) chdir into cmd.Dir first and
	// then resolve --cd from there, so a relative worktree path nests into
	// itself ("No such file or directory (os error 2)"). Always hand back an
	// absolute path so the adapter sees the same cwd regardless of how the
	// server was launched (e.g. `--data-dir .tmp/dev-data`).
	absDataDir, absErr := filepath.Abs(dataDir)
	if absErr != nil {
		return "", nopWorktreeCleanup, fmt.Errorf("worktree: resolve dataDir abs: %w", absErr)
	}
	path = filepath.Join(absDataDir, "worktrees", workspaceSlug, runID)

	if shouldUseGitWorktree(workingDir) {
		gitPath, gitCleanup, gitErr := allocateGitWorktree(workingDir, path)
		if gitErr == nil {
			return gitPath, gitCleanup, nil
		}
		// Fall through to plain mkdir so a transient git failure does not
		// take down the run. The mkdir path always succeeds when the parent
		// data-dir is writable.
	}

	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", nopWorktreeCleanup, fmt.Errorf("worktree: mkdir %s: %w", path, err)
	}
	return path, func() error {
		return os.RemoveAll(path)
	}, nil
}

func shouldUseGitWorktree(workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(workingDir, ".git"))
	if err != nil {
		return false
	}
	// `.git` can be a regular file (submodule / nested worktree) or a directory
	// for the main repo. Both are acceptable starting points for `git worktree
	// add`.
	_ = info
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	return true
}

func allocateGitWorktree(workingDir, path string) (string, func() error, error) {
	if existing, err := os.Stat(filepath.Join(path, ".git")); err == nil && existing.Mode().IsRegular() {
		// Reattach to an existing worktree (the retry path: same run_id
		// re-enters claim and we keep the previous tree intact).
		return path, makeGitWorktreeCleanup(workingDir, path), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", nopWorktreeCleanup, fmt.Errorf("worktree: mkdir parent: %w", err)
	}
	// `git worktree add` refuses to write into an existing non-empty
	// directory, so remove the mkdir leftover from a previous failed attempt.
	_ = os.RemoveAll(path)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", path, "HEAD")
	cmd.Dir = workingDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", nopWorktreeCleanup, fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return path, makeGitWorktreeCleanup(workingDir, path), nil
}

func makeGitWorktreeCleanup(workingDir, path string) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		removeCmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
		removeCmd.Dir = workingDir
		// Ignore the error: even if git refuses (e.g. registry already pruned),
		// the RemoveAll below still tries to reclaim the disk. A `git worktree
		// prune` would tidy up the registry afterward but is out of scope for
		// the per-run cleanup hot path.
		_ = removeCmd.Run()
		return os.RemoveAll(path)
	}
}

func nopWorktreeCleanup() error { return nil }
