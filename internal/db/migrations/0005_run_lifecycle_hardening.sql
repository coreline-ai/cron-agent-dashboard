ALTER TABLE run ADD COLUMN heartbeat_at TEXT;

ALTER TABLE run ADD COLUMN terminal_reason TEXT NOT NULL DEFAULT ''
  CHECK (terminal_reason IN (
    '',
    'completed',
    'exit_nonzero',
    'timeout',
    'executor_error',
    'worker_panic',
    'claim_preparation_failed',
    'unknown_failure',
    'user_cancelled',
    'issue_cancelled',
    'shutdown',
    'orphan_recovered',
    'stale_recovered'
  ));

ALTER TABLE run ADD COLUMN failure_kind TEXT NOT NULL DEFAULT ''
  CHECK (failure_kind IN (
    '',
    'exit_nonzero',
    'timeout',
    'executor_error',
    'worker_panic',
    'claim_preparation_failed',
    'unknown'
  ));

ALTER TABLE run ADD COLUMN cancel_reason TEXT NOT NULL DEFAULT ''
  CHECK (cancel_reason IN (
    '',
    'user',
    'issue',
    'shutdown',
    'orphan',
    'stale'
  ));

UPDATE run
SET terminal_reason = 'completed'
WHERE status = 'done' AND terminal_reason = '';

UPDATE run
SET terminal_reason = 'unknown_failure',
    failure_kind = 'unknown'
WHERE status = 'failed' AND terminal_reason = '';

UPDATE run
SET terminal_reason = CASE
      WHEN lower(error_message) LIKE '%shutdown%' THEN 'shutdown'
      WHEN lower(error_message) LIKE '%issue%' THEN 'issue_cancelled'
      WHEN lower(error_message) LIKE '%orphan%' THEN 'orphan_recovered'
      WHEN lower(error_message) LIKE '%stale%' THEN 'stale_recovered'
      ELSE 'user_cancelled'
    END,
    cancel_reason = CASE
      WHEN lower(error_message) LIKE '%shutdown%' THEN 'shutdown'
      WHEN lower(error_message) LIKE '%issue%' THEN 'issue'
      WHEN lower(error_message) LIKE '%orphan%' THEN 'orphan'
      WHEN lower(error_message) LIKE '%stale%' THEN 'stale'
      ELSE 'user'
    END
WHERE status = 'cancelled' AND terminal_reason = '';

UPDATE run
SET heartbeat_at = COALESCE(started_at, claimed_at)
WHERE status = 'running' AND heartbeat_at IS NULL;

CREATE INDEX idx_run_running_heartbeat
  ON run(heartbeat_at, claimed_at, id)
  WHERE status = 'running';
