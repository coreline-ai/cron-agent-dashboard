-- Agent instruction version history for run reproducibility and auditability.
ALTER TABLE agent ADD COLUMN instructions_version INTEGER NOT NULL DEFAULT 1 CHECK (instructions_version >= 1);
ALTER TABLE run ADD COLUMN agent_instructions_version INTEGER NOT NULL DEFAULT 1 CHECK (agent_instructions_version >= 1);

CREATE TABLE agent_instruction_version (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version >= 1),
  instructions TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(agent_id, version)
);

INSERT INTO agent_instruction_version(id, agent_id, version, instructions, created_at)
SELECT id || '-v1', id, 1, instructions, created_at FROM agent;

CREATE INDEX idx_agent_instruction_version_agent
  ON agent_instruction_version(agent_id, version DESC);
