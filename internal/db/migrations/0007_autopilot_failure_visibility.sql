ALTER TABLE autopilot_rule
  ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE autopilot_rule
  ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0
  CHECK (consecutive_failures >= 0);

ALTER TABLE autopilot_rule
  ADD COLUMN last_triggered_issue_id TEXT
  REFERENCES issue(id) ON DELETE SET NULL;

CREATE INDEX idx_autopilot_failure_state
  ON autopilot_rule(workspace_id, consecutive_failures)
  WHERE consecutive_failures > 0;

CREATE INDEX idx_autopilot_last_triggered_issue
  ON autopilot_rule(last_triggered_issue_id)
  WHERE last_triggered_issue_id IS NOT NULL;
