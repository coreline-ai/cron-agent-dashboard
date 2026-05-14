ALTER TABLE run ADD COLUMN process_pid INTEGER;

ALTER TABLE run ADD COLUMN process_pgid INTEGER;

CREATE INDEX idx_run_running_process_pgid
  ON run(process_pgid, id)
  WHERE status = 'running' AND process_pgid > 1;
