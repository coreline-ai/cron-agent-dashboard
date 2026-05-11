CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE workspace (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  output_dir  TEXT NOT NULL DEFAULT '',
  working_dir TEXT NOT NULL DEFAULT '',
  identifier_prefix TEXT NOT NULL,
  next_issue_seq INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE agent (
  id            TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  runtime       TEXT NOT NULL,
  model         TEXT NOT NULL DEFAULT '',
  instructions  TEXT NOT NULL DEFAULT '',
  is_main       INTEGER NOT NULL DEFAULT 0 CHECK (is_main IN (0, 1)),
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE issue (
  id                 TEXT PRIMARY KEY,
  workspace_id       TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  identifier         TEXT NOT NULL,
  title              TEXT NOT NULL,
  body               TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','cancelled')),
  assignee_agent_id  TEXT REFERENCES agent(id) ON DELETE SET NULL,
  parent_issue_id    TEXT REFERENCES issue(id) ON DELETE SET NULL,
  created_by         TEXT NOT NULL DEFAULT 'user' CHECK (created_by IN ('user','autopilot')),
  autopilot_rule_id  TEXT REFERENCES autopilot_rule(id) ON DELETE SET NULL,
  created_at         TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (workspace_id, identifier)
);

CREATE TABLE comment (
  id              TEXT PRIMARY KEY,
  issue_id        TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  author_type     TEXT NOT NULL CHECK (author_type IN ('user','agent','system')),
  author_agent_id TEXT REFERENCES agent(id) ON DELETE SET NULL,
  run_id          TEXT REFERENCES run(id) ON DELETE SET NULL,
  content         TEXT NOT NULL,
  created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE run (
  id             TEXT PRIMARY KEY,
  issue_id       TEXT NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
  agent_id       TEXT NOT NULL REFERENCES agent(id) ON DELETE RESTRICT,
  status         TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed','cancelled')),
  trigger_type   TEXT NOT NULL DEFAULT 'issue_created' CHECK (trigger_type IN ('issue_created','mention','autopilot','rerun')),
  trigger_comment_id TEXT REFERENCES comment(id) ON DELETE SET NULL,
  trigger_content_snapshot TEXT NOT NULL DEFAULT '',
  enqueued_at    TEXT NOT NULL DEFAULT (datetime('now')),
  claimed_at     TEXT,
  claimed_by     TEXT NOT NULL DEFAULT '',
  started_at     TEXT,
  finished_at    TEXT,
  exit_code      INTEGER,
  stdout_path    TEXT,
  error_message  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE autopilot_rule (
  id                    TEXT PRIMARY KEY,
  workspace_id          TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name                  TEXT NOT NULL,
  cron_expr             TEXT NOT NULL,
  issue_title_template  TEXT NOT NULL,
  issue_body_template   TEXT NOT NULL DEFAULT '',
  assignee_agent_id     TEXT REFERENCES agent(id) ON DELETE SET NULL,
  enabled               INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  last_run_at           TEXT,
  next_run_at           TEXT,
  created_at            TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);
