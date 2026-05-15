package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	defaultWorkspaceTimeoutSeconds = 600
	maxRetryAttempts               = 5
)

type retryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

func retryPolicyMaxAttempts(policyJSON string) int {
	policy := retryPolicy{MaxAttempts: 1}
	if policyJSON != "" {
		_ = json.Unmarshal([]byte(policyJSON), &policy)
	}
	if policy.MaxAttempts < 1 {
		return 1
	}
	if policy.MaxAttempts > maxRetryAttempts {
		return maxRetryAttempts
	}
	return policy.MaxAttempts
}

func retryMaxAttemptsForAgent(ctx context.Context, tx sqlx.ExtContext, agentID string) int {
	var policyJSON string
	if err := sqlx.GetContext(ctx, tx, &policyJSON, `SELECT retry_policy_json FROM agent WHERE id=?`, agentID); err != nil {
		return 1
	}
	return retryPolicyMaxAttempts(policyJSON)
}

func ResolveTimeoutSeconds(workspace Workspace, agent Agent, issue Issue) int {
	if issue.TimeoutSecondsOverride.Valid {
		return int(issue.TimeoutSecondsOverride.Int64)
	}
	if agent.TimeoutSecondsOverride.Valid {
		return int(agent.TimeoutSecondsOverride.Int64)
	}
	if workspace.DefaultTimeoutSeconds > 0 {
		return workspace.DefaultTimeoutSeconds
	}
	return defaultWorkspaceTimeoutSeconds
}

func shouldRetryRun(failureKind string, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts || maxAttempts <= 1 {
		return false
	}
	switch failureKind {
	case FailureKindTimeout, FailureKindExecutorError:
		return true
	default:
		return false
	}
}

func retryBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 10 * time.Second
	case attempt == 2:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}
