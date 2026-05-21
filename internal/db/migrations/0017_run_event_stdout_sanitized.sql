-- Extend run_event type enum with stdout_sanitized so that worker_store can
-- record when known runtime CLI diagnostic noise (e.g. "MCP issues detected.
-- Run /mcp list for status.") was stripped from a run's stdout before it
-- became the agent result comment / chain prompt snapshot.
--
-- SQLite cannot ALTER CHECK constraints, so rebuild the table preserving
-- existing rows. Pattern matches 0015_agent_skills.sql.
CREATE TABLE run_event_new (
  id          TEXT PRIMARY KEY,
  run_id      TEXT NOT NULL REFERENCES run(id) ON DELETE CASCADE,
  issue_id    TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  event_type  TEXT NOT NULL CHECK (
    event_type IN (
      'run_queued',
      'run_claimed',
      'run_prepare_failed',
      'executor_starting',
      'stdout_truncated',
      'cancel_requested',
      'run_cancelled',
      'run_completed',
      'run_failed',
      'orphan_recovered',
      'stale_recovered',
      'skills_loaded',
      'stdout_sanitized'
    )
  ),
  severity    TEXT NOT NULL DEFAULT 'info'
              CHECK (severity IN ('debug','info','warn','error')),
  message     TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(run_id, seq)
);

INSERT INTO run_event_new(id, run_id, issue_id, seq, event_type, severity, message, detail_json, created_at)
SELECT id, run_id, issue_id, seq, event_type, severity, message, detail_json, created_at
FROM run_event;

DROP TABLE run_event;
ALTER TABLE run_event_new RENAME TO run_event;

CREATE INDEX idx_run_event_run_seq
  ON run_event(run_id, seq);

CREATE INDEX idx_run_event_issue_created
  ON run_event(issue_id, created_at, run_id, seq);

CREATE INDEX idx_run_event_type_created
  ON run_event(event_type, created_at);
