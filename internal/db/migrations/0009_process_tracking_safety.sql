ALTER TABLE run ADD COLUMN process_recorded_at TEXT;

UPDATE run
SET process_recorded_at = COALESCE(started_at, claimed_at, heartbeat_at)
WHERE status = 'running'
  AND process_pgid IS NOT NULL
  AND process_pgid > 1
  AND process_recorded_at IS NULL;

DROP INDEX IF EXISTS idx_run_running_process_pgid;

CREATE INDEX idx_run_running_process_pgid
  ON run(process_pgid, process_recorded_at, id)
  WHERE status = 'running' AND process_pgid > 1;
