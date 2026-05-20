-- F3: workspace-level toggle for "mark issue done when any run completes".
-- Existing workspaces keep current behavior (1 = auto-close) so this is a
-- non-breaking upgrade. New workspaces will explicitly opt in via the
-- CreateWorkspaceInput flag; the safer default for multi-step collaboration
-- flows is 0, enforced in the application layer.
ALTER TABLE workspace ADD COLUMN auto_close_on_run_done INTEGER NOT NULL DEFAULT 1 CHECK (auto_close_on_run_done IN (0, 1));
