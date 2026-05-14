-- Allow temporary pausing of enabled autopilot rules without losing schedule/config.
ALTER TABLE autopilot_rule ADD COLUMN snooze_until TEXT;

CREATE INDEX IF NOT EXISTS idx_autopilot_snooze
  ON autopilot_rule(snooze_until)
  WHERE enabled=1 AND snooze_until IS NOT NULL;
