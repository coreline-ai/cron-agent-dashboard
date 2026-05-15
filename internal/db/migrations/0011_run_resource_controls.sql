-- Resource controls and best-effort usage accounting for multi-agent daily use.
ALTER TABLE run ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0);
ALTER TABLE run ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0);
ALTER TABLE run ADD COLUMN total_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (total_cost_micros >= 0);
ALTER TABLE run ADD COLUMN model_resolved TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1);
ALTER TABLE run ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 1 CHECK (max_attempts >= 1);
ALTER TABLE run ADD COLUMN next_retry_at TEXT;

ALTER TABLE workspace ADD COLUMN default_timeout_seconds INTEGER NOT NULL DEFAULT 600 CHECK (default_timeout_seconds >= 0);
ALTER TABLE agent ADD COLUMN timeout_seconds_override INTEGER CHECK (timeout_seconds_override IS NULL OR timeout_seconds_override >= 0);
ALTER TABLE issue ADD COLUMN timeout_seconds_override INTEGER CHECK (timeout_seconds_override IS NULL OR timeout_seconds_override >= 0);
ALTER TABLE agent ADD COLUMN retry_policy_json TEXT NOT NULL DEFAULT '{"max_attempts":1}';

CREATE INDEX idx_run_queue_retry
  ON run(status, next_retry_at, enqueued_at, id)
  WHERE status='queued';

CREATE INDEX idx_run_usage_finished
  ON run(finished_at, input_tokens, output_tokens, total_cost_micros)
  WHERE status IN ('done','failed','cancelled');
