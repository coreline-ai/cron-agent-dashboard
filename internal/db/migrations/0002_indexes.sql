CREATE INDEX idx_workspace_slug ON workspace(slug);
CREATE INDEX idx_agent_workspace ON agent(workspace_id);
CREATE UNIQUE INDEX idx_agent_name_ci ON agent(workspace_id, lower(name));
CREATE UNIQUE INDEX idx_agent_main_unique ON agent(workspace_id) WHERE is_main = 1;

CREATE INDEX idx_issue_workspace_status ON issue(workspace_id, status);
CREATE INDEX idx_issue_workspace_created ON issue(workspace_id, created_at DESC);
CREATE INDEX idx_issue_parent ON issue(parent_issue_id);
CREATE INDEX idx_issue_assignee ON issue(assignee_agent_id);
CREATE INDEX idx_issue_workspace_identifier ON issue(workspace_id, identifier);

CREATE INDEX idx_comment_issue_created ON comment(issue_id, created_at);
CREATE INDEX idx_comment_run ON comment(run_id);

CREATE INDEX idx_run_queue ON run(status, enqueued_at, id) WHERE status = 'queued';
CREATE UNIQUE INDEX idx_run_one_queued_per_issue_agent ON run(issue_id, agent_id) WHERE status = 'queued';
CREATE INDEX idx_run_issue_running ON run(issue_id) WHERE status = 'running';
CREATE INDEX idx_run_issue ON run(issue_id, enqueued_at);
CREATE INDEX idx_run_agent ON run(agent_id);
CREATE INDEX idx_run_trigger_comment ON run(trigger_comment_id) WHERE trigger_comment_id IS NOT NULL;

CREATE INDEX idx_autopilot_workspace ON autopilot_rule(workspace_id);
CREATE INDEX idx_autopilot_enabled_next ON autopilot_rule(enabled, next_run_at) WHERE enabled = 1;
