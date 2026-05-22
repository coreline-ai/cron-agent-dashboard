-- Issue attachments. Phase 1 of dev-plan/implement_20260522_174204.md.
--
-- Metadata only — the file body lives on disk under
-- <data_dir>/attachments/<id> with 0600 perms. Keeping the storage_path
-- column lets us migrate the on-disk layout later without rewriting every
-- handler.
--
-- size_bytes is enforced (<= 10 MB) at the API boundary and re-checked
-- before write so a corrupted multipart body cannot blow past the cap.
-- sha256 is computed at upload time so a future plan can collapse duplicates
-- without rereading file bodies from disk.
CREATE TABLE attachment (
  id            TEXT PRIMARY KEY,
  issue_id      TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  uploaded_by   TEXT NOT NULL DEFAULT 'user',
  filename      TEXT NOT NULL,
  content_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
  size_bytes    INTEGER NOT NULL DEFAULT 0,
  sha256        TEXT NOT NULL DEFAULT '',
  storage_path  TEXT NOT NULL,
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_attachment_issue_created
  ON attachment(issue_id, created_at);
