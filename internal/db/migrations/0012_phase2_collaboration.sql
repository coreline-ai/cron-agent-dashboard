ALTER TABLE workspace ADD COLUMN auto_chain_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_enabled IN (0, 1));

ALTER TABLE agent ADD COLUMN summary TEXT NOT NULL DEFAULT '';
ALTER TABLE agent ADD COLUMN tags TEXT NOT NULL DEFAULT '';

ALTER TABLE run ADD COLUMN parent_run_id TEXT REFERENCES run(id) ON DELETE SET NULL;
ALTER TABLE run ADD COLUMN chain_id TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN chain_depth INTEGER NOT NULL DEFAULT 0 CHECK (chain_depth >= 0 AND chain_depth <= 20);

CREATE INDEX IF NOT EXISTS idx_run_parent ON run(parent_run_id);
CREATE INDEX IF NOT EXISTS idx_run_chain ON run(chain_id, chain_depth, enqueued_at);
