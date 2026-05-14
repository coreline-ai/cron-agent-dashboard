package store

import (
	"database/sql"
	"strconv"
)

type NullInt64 struct {
	sql.NullInt64
}

func (n NullInt64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(n.Int64, 10)), nil
}

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
	HeartbeatAt            string         `db:"heartbeat_at" json:"heartbeat_at,omitempty"`
	FinishedAt             string         `db:"finished_at" json:"finished_at,omitempty"`
	ProcessPID             int            `db:"process_pid" json:"-"`
	ProcessPGID            int            `db:"process_pgid" json:"-"`
	ProcessRecordedAt      string         `db:"process_recorded_at" json:"-"`
	ExitCode               NullInt64      `db:"exit_code" json:"exit_code"`
	StdoutPath             sql.NullString `db:"stdout_path" json:"-"`
	StdoutSizeBytes        int64          `db:"stdout_size_bytes" json:"stdout_size_bytes"`
	LogURL                 string         `db:"log_url" json:"log_url,omitempty"`
	ErrorMessage           string         `db:"error_message" json:"error_message"`
	TerminalReason         string         `db:"terminal_reason" json:"terminal_reason"`
	FailureKind            string         `db:"failure_kind" json:"failure_kind"`
	CancelReason           string         `db:"cancel_reason" json:"cancel_reason"`
}

type RunningProcessGroup struct {
	PGID       int    `db:"process_pgid"`
	RecordedAt string `db:"process_recorded_at"`
	RunCount   int    `db:"run_count"`
}

const (
	TerminalReasonCompleted              = "completed"
	TerminalReasonExitNonzero            = "exit_nonzero"
	TerminalReasonTimeout                = "timeout"
	TerminalReasonExecutorError          = "executor_error"
	TerminalReasonWorkerPanic            = "worker_panic"
	TerminalReasonClaimPreparationFailed = "claim_preparation_failed"
	TerminalReasonUnknownFailure         = "unknown_failure"
	TerminalReasonUserCancelled          = "user_cancelled"
	TerminalReasonIssueCancelled         = "issue_cancelled"
	TerminalReasonShutdown               = "shutdown"
	TerminalReasonOrphanRecovered        = "orphan_recovered"
	TerminalReasonStaleRecovered         = "stale_recovered"

	FailureKindExitNonzero            = "exit_nonzero"
	FailureKindTimeout                = "timeout"
	FailureKindExecutorError          = "executor_error"
	FailureKindWorkerPanic            = "worker_panic"
	FailureKindClaimPreparationFailed = "claim_preparation_failed"
	FailureKindUnknown                = "unknown"

	CancelReasonUser     = "user"
	CancelReasonIssue    = "issue"
	CancelReasonShutdown = "shutdown"
	CancelReasonOrphan   = "orphan"
	CancelReasonStale    = "stale"

	RunEventQueued        = "run_queued"
	RunEventClaimed       = "run_claimed"
	RunEventPrepareFailed = "run_prepare_failed"
	RunEventStarting      = "executor_starting"
	RunEventStdoutTrunc   = "stdout_truncated"
	RunEventCancelRequest = "cancel_requested"
	RunEventCancelled     = "run_cancelled"
	RunEventCompleted     = "run_completed"
	RunEventFailed        = "run_failed"
	RunEventOrphan        = "orphan_recovered"
	RunEventStale         = "stale_recovered"

	RunEventSeverityDebug = "debug"
	RunEventSeverityInfo  = "info"
	RunEventSeverityWarn  = "warn"
	RunEventSeverityError = "error"
)

type FinishRunInput struct {
	ExitCode         int
	StdoutPath       string
	Content          string
	ContentTruncated bool
	StdoutTruncated  bool
	ErrorMessage     string
	TerminalReason   string
	FailureKind      string
}

type CancelReasonInput struct {
	Message        string
	TerminalReason string
	CancelReason   string
}

type RunEvent struct {
	ID         string         `db:"id" json:"id"`
	RunID      string         `db:"run_id" json:"run_id"`
	IssueID    string         `db:"issue_id" json:"issue_id"`
	Seq        int64          `db:"seq" json:"seq"`
	EventType  string         `db:"event_type" json:"event_type"`
	Severity   string         `db:"severity" json:"severity"`
	Message    string         `db:"message" json:"message"`
	DetailJSON sql.NullString `db:"detail_json" json:"-"`
	Details    map[string]any `db:"-" json:"details"`
	CreatedAt  string         `db:"created_at" json:"created_at"`
}

type RunEventInput struct {
	RunID     string
	IssueID   string
	EventType string
	Severity  string
	Message   string
	Details   map[string]any
}

type AutopilotRule struct {
	ID                   string `db:"id" json:"id"`
	WorkspaceID          string `db:"workspace_id" json:"workspace_id"`
	Name                 string `db:"name" json:"name"`
	CronExpr             string `db:"cron_expr" json:"cron_expr"`
	IssueTitleTemplate   string `db:"issue_title_template" json:"issue_title_template"`
	IssueBodyTemplate    string `db:"issue_body_template" json:"issue_body_template"`
	AssigneeAgentID      string `db:"assignee_agent_id" json:"assignee_agent_id,omitempty"`
	AssigneeAgentName    string `db:"assignee_agent_name" json:"assignee_agent_name,omitempty"`
	Enabled              bool   `db:"enabled" json:"enabled"`
	LastRunAt            string `db:"last_run_at" json:"last_run_at,omitempty"`
	NextRunAt            string `db:"next_run_at" json:"next_run_at,omitempty"`
	LastError            string `db:"last_error" json:"last_error,omitempty"`
	ConsecutiveFailures  int    `db:"consecutive_failures" json:"consecutive_failures"`
	LastTriggeredIssueID string `db:"last_triggered_issue_id" json:"last_triggered_issue_id,omitempty"`
	CreatedAt            string `db:"created_at" json:"created_at"`
	UpdatedAt            string `db:"updated_at" json:"updated_at"`
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

type UpdateIssueInput struct {
	Title           *string `json:"title"`
	Body            *string `json:"body"`
	AssigneeAgentID *string `json:"assignee_agent_id"`
	Status          *string `json:"status"`
}

type ListIssuesFilter struct {
	Status    []string
	Execution []string
	Assignee  string
	Query     string
	Limit     int
}
