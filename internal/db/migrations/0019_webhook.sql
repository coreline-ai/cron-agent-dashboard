-- External webhook subscriptions + delivery audit log. Phase 1 of
-- dev-plan/implement_20260521_224221.md.
--
-- The `webhook` table records workspace-scoped subscriptions. `events_json`
-- is a JSON array of event types this webhook wants (e.g. ["run.completed",
-- "issue.done"]) — empty array = receive all. `secret` (optional) is used
-- by the dispatcher to sign payloads with HMAC-SHA256.
--
-- The `webhook_delivery` table is both a queue (rows with status='pending'
-- await a worker) and an audit log (rows with status in 'delivered'/'failed'
-- record the outcome of each attempt). The dispatcher polls
-- `(status, next_attempt_at)` for due rows.
CREATE TABLE webhook (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  secret TEXT NOT NULL DEFAULT '',
  events_json TEXT NOT NULL DEFAULT '[]',
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_webhook_workspace_enabled
  ON webhook(workspace_id, enabled);

CREATE TABLE webhook_delivery (
  id TEXT PRIMARY KEY,
  webhook_id TEXT NOT NULL REFERENCES webhook(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','delivered','failed')),
  status_code INTEGER NOT NULL DEFAULT 0,
  response_body TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  attempt INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL DEFAULT (datetime('now')),
  delivered_at TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_webhook_delivery_pending
  ON webhook_delivery(status, next_attempt_at);

CREATE INDEX idx_webhook_delivery_webhook_created
  ON webhook_delivery(webhook_id, created_at);
