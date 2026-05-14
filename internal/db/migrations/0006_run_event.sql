CREATE TABLE run_event (
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
      'stale_recovered'
    )
  ),
  severity    TEXT NOT NULL DEFAULT 'info'
              CHECK (severity IN ('debug','info','warn','error')),
  message     TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(run_id, seq)
);

CREATE INDEX idx_run_event_run_seq
  ON run_event(run_id, seq);

CREATE INDEX idx_run_event_issue_created
  ON run_event(issue_id, created_at, run_id, seq);

CREATE INDEX idx_run_event_type_created
  ON run_event(event_type, created_at);
