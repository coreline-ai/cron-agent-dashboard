-- Tiny KV table for operational metadata that the application updates outside
-- the normal workspace/issue/run hierarchy. Track E of
-- dev-plan/implement_20260522_212332.md.
--
-- Today the only writers are:
--   * the maintenance runner — recording last_log_cleanup_at / last_log_cleanup_files
--     / last_log_cleanup_bytes after each automatic cleanup.
-- Schema is intentionally permissive (TEXT values only) so subsequent ops
-- ergonomics (e.g. "last_orphan_recovery_at") can land without migration churn.
CREATE TABLE system_state (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
