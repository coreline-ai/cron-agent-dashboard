package store

import "database/sql"

type Workspace struct {
	ID               string `db:"id" json:"id"`
	Name             string `db:"name" json:"name"`
	Slug             string `db:"slug" json:"slug"`
	Description      string `db:"description" json:"description"`
	OutputDir        string `db:"output_dir" json:"output_dir"`
	WorkingDir       string `db:"working_dir" json:"working_dir"`
	IdentifierPrefix string `db:"identifier_prefix" json:"identifier_prefix"`
	NextIssueSeq     int64  `db:"next_issue_seq" json:"-"`
	CreatedAt        string `db:"created_at" json:"created_at"`
	UpdatedAt        string `db:"updated_at" json:"updated_at"`
	AgentCount       int64  `db:"agent_count" json:"agent_count,omitempty"`
	OpenIssueCount   int64  `db:"open_issue_count" json:"open_issue_count,omitempty"`
}

type Agent struct {
	ID           string `db:"id" json:"id"`
	WorkspaceID  string `db:"workspace_id" json:"workspace_id"`
	Name         string `db:"name" json:"name"`
	Runtime      string `db:"runtime" json:"runtime"`
	Model        string `db:"model" json:"model"`
	Instructions string `db:"instructions" json:"instructions"`
	IsMain       bool   `db:"is_main" json:"is_main"`
	CreatedAt    string `db:"created_at" json:"created_at"`
	UpdatedAt    string `db:"updated_at" json:"updated_at"`
}

type Issue struct {
	ID                string `db:"id" json:"id"`
	WorkspaceID       string `db:"workspace_id" json:"workspace_id"`
	Identifier        string `db:"identifier" json:"identifier"`
	Title             string `db:"title" json:"title"`
	Body              string `db:"body" json:"body"`
	Status            string `db:"status" json:"status"`
	AssigneeAgentID   string `db:"assignee_agent_id" json:"assignee_agent_id,omitempty"`
	AssigneeAgentName string `db:"assignee_agent_name" json:"assignee_agent_name,omitempty"`
	CreatedBy         string `db:"created_by" json:"created_by"`
	AutopilotRuleID   string `db:"autopilot_rule_id" json:"autopilot_rule_id,omitempty"`
	ExecutionStatus   string `db:"execution_status" json:"execution_status"`
	LastRunAgentID    string `db:"last_run_agent_id" json:"last_run_agent_id,omitempty"`
	LastRunAgentName  string `db:"last_run_agent_name" json:"last_run_agent_name,omitempty"`
	CommentCount      int64  `db:"comment_count" json:"comment_count"`
	CreatedAt         string `db:"created_at" json:"created_at"`
	UpdatedAt         string `db:"updated_at" json:"updated_at"`
}

type Comment struct {
	ID              string `db:"id" json:"id"`
	IssueID         string `db:"issue_id" json:"issue_id"`
	AuthorType      string `db:"author_type" json:"author_type"`
	AuthorAgentID   string `db:"author_agent_id" json:"author_agent_id,omitempty"`
	AuthorAgentName string `db:"author_agent_name" json:"author_agent_name,omitempty"`
	RunID           string `db:"run_id" json:"run_id,omitempty"`
	Content         string `db:"content" json:"content"`
	Truncated       bool   `db:"truncated" json:"truncated"`
	LogURL          string `db:"log_url" json:"log_url,omitempty"`
	CreatedAt       string `db:"created_at" json:"created_at"`
}

type Run struct {
	ID                     string         `db:"id" json:"id"`
	IssueID                string         `db:"issue_id" json:"issue_id"`
	AgentID                string         `db:"agent_id" json:"agent_id"`
	AgentName              string         `db:"agent_name" json:"agent_name,omitempty"`
	Status                 string         `db:"status" json:"status"`
	TriggerType            string         `db:"trigger_type" json:"trigger_type"`
	TriggerCommentID       string         `db:"trigger_comment_id" json:"trigger_comment_id,omitempty"`
	TriggerContentSnapshot string         `db:"trigger_content_snapshot" json:"trigger_content_snapshot,omitempty"`
	EnqueuedAt             string         `db:"enqueued_at" json:"enqueued_at"`
	ClaimedAt              string         `db:"claimed_at" json:"claimed_at,omitempty"`
	ClaimedBy              string         `db:"claimed_by" json:"claimed_by,omitempty"`
	StartedAt              string         `db:"started_at" json:"started_at,omitempty"`
	FinishedAt             string         `db:"finished_at" json:"finished_at,omitempty"`
	ExitCode               sql.NullInt64  `db:"exit_code" json:"exit_code"`
	StdoutPath             sql.NullString `db:"stdout_path" json:"-"`
	StdoutSizeBytes        int64          `db:"stdout_size_bytes" json:"stdout_size_bytes"`
	LogURL                 string         `db:"log_url" json:"log_url,omitempty"`
	ErrorMessage           string         `db:"error_message" json:"error_message"`
}

type AutopilotRule struct {
	ID                 string `db:"id" json:"id"`
	WorkspaceID        string `db:"workspace_id" json:"workspace_id"`
	Name               string `db:"name" json:"name"`
	CronExpr           string `db:"cron_expr" json:"cron_expr"`
	IssueTitleTemplate string `db:"issue_title_template" json:"issue_title_template"`
	IssueBodyTemplate  string `db:"issue_body_template" json:"issue_body_template"`
	AssigneeAgentID    string `db:"assignee_agent_id" json:"assignee_agent_id,omitempty"`
	AssigneeAgentName  string `db:"assignee_agent_name" json:"assignee_agent_name,omitempty"`
	Enabled            bool   `db:"enabled" json:"enabled"`
	LastRunAt          string `db:"last_run_at" json:"last_run_at,omitempty"`
	NextRunAt          string `db:"next_run_at" json:"next_run_at,omitempty"`
	CreatedAt          string `db:"created_at" json:"created_at"`
	UpdatedAt          string `db:"updated_at" json:"updated_at"`
}

type CreateWorkspaceInput struct {
	Name             string           `json:"name"`
	Slug             string           `json:"slug"`
	Description      string           `json:"description"`
	IdentifierPrefix string           `json:"identifier_prefix"`
	WorkingDir       string           `json:"working_dir"`
	OutputDir        string           `json:"output_dir"`
	MainAgent        CreateAgentInput `json:"main_agent"`
}

type CreateAgentInput struct {
	Name         string `json:"name"`
	Runtime      string `json:"runtime"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type CreateIssueInput struct {
	Title           string `json:"title"`
	Body            string `json:"body"`
	AssigneeAgentID string `json:"assignee_agent_id"`
	CreatedBy       string `json:"created_by"`
	AutopilotRuleID string `json:"autopilot_rule_id"`
	TriggerType     string `json:"trigger_type"`
}

type ListIssuesFilter struct {
	Status    []string
	Execution []string
	Assignee  string
	Query     string
	Limit     int
}
