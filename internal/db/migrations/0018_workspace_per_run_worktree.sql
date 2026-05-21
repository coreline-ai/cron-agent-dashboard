-- Phase 1 of dev-plan/implement_20260521_222623.md: introduce a workspace-
-- level flag that opts a workspace into per-run filesystem worktrees. When
-- the flag is true, the worker pool is allowed to claim multiple running
-- runs concurrently for the same workspace (Phase 3) and each run gets its
-- own cwd under <data_dir>/worktrees/<workspace>/<run-id>/ (Phase 4).
--
-- Default is 0 (off) so existing workspaces keep the conservative
-- "workspace serializes runs" guarantee until an operator opts in.
ALTER TABLE workspace
  ADD COLUMN per_run_worktree INTEGER NOT NULL DEFAULT 0;
