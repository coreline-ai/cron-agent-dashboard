ALTER TABLE workspace ADD COLUMN auto_chain_max_depth INTEGER NOT NULL DEFAULT 5 CHECK (auto_chain_max_depth >= 1 AND auto_chain_max_depth <= 20);
ALTER TABLE workspace ADD COLUMN auto_chain_daily_run_limit INTEGER NOT NULL DEFAULT 20 CHECK (auto_chain_daily_run_limit >= 0);
ALTER TABLE workspace ADD COLUMN auto_chain_daily_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_daily_cost_micros >= 0);
ALTER TABLE workspace ADD COLUMN auto_chain_dry_run INTEGER NOT NULL DEFAULT 0 CHECK (auto_chain_dry_run IN (0, 1));
