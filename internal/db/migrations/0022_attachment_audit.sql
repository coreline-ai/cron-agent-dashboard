-- Per-attachment audit log. Track C of dev-plan/implement_20260522_212332.md.
--
-- Recording downloads (and, prospectively, uploads / deletes) in a dedicated
-- table keeps the run_event table focused on per-run lifecycle while still
-- giving operators a queryable trail of who pulled what.
--
-- Rows are append-only by the application layer; the ON DELETE CASCADE on
-- attachment_id guarantees the trail goes away with the attachment so a
-- garbage workspace cleanup does not leak metadata.
CREATE TABLE attachment_audit (
  id            TEXT PRIMARY KEY,
  attachment_id TEXT NOT NULL REFERENCES attachment(id) ON DELETE CASCADE,
  issue_id      TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  action        TEXT NOT NULL CHECK (action IN ('uploaded','downloaded','deleted')),
  actor         TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_attachment_audit_attachment_created
  ON attachment_audit(attachment_id, created_at);

CREATE INDEX idx_attachment_audit_issue_created
  ON attachment_audit(issue_id, created_at);
