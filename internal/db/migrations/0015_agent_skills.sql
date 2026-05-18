-- Agent Skills registry. Skills are prompt/instruction modules only; bundled
-- scripts/references are recorded but never executed by the dashboard.
CREATE TABLE skill (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  triggers_json TEXT NOT NULL DEFAULT '[]',
  content TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL DEFAULT 'manual' CHECK (source_type IN ('manual','local','git','builtin')),
  source_url TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  local_path TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  trust_level TEXT NOT NULL DEFAULT 'local' CHECK (trust_level IN ('builtin','local','git','untrusted')),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE agent_skill (
  agent_id TEXT NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
  activation_mode TEXT NOT NULL DEFAULT 'trigger' CHECK (activation_mode IN ('always','trigger','manual')),
  priority INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY(agent_id, skill_id)
);

CREATE UNIQUE INDEX idx_skill_workspace_name_ci ON skill(workspace_id, lower(name));
CREATE INDEX idx_skill_workspace_enabled ON skill(workspace_id, enabled, name);
CREATE INDEX idx_agent_skill_agent_enabled ON agent_skill(agent_id, enabled, priority);

-- Extend run_event type enum with skills_loaded. SQLite cannot ALTER CHECK
-- constraints, so rebuild the table preserving existing rows.
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
      'skills_loaded'
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
