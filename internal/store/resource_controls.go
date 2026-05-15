package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	defaultWorkspaceTimeoutSeconds = 600
	maxRetryAttempts               = 5
	maxRetryBackoffSeconds         = 3600
)

type retryPolicy struct {
	MaxAttempts    int      `json:"max_attempts"`
	BackoffSeconds []int    `json:"backoff_seconds,omitempty"`
	RetryOn        []string `json:"retry_on,omitempty"`
}

func parseRetryPolicy(policyJSON string) (retryPolicy, error) {
	policy := retryPolicy{MaxAttempts: 1}
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON != "" {
		if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
			return retryPolicy{}, ErrValidation
		}
	}
	if policy.MaxAttempts < 1 || policy.MaxAttempts > maxRetryAttempts {
		return retryPolicy{}, ErrValidation
	}
	for _, seconds := range policy.BackoffSeconds {
		if seconds <= 0 || seconds > maxRetryBackoffSeconds {
			return retryPolicy{}, ErrValidation
		}
	}
	for _, kind := range policy.RetryOn {
		if !isRetryableFailureKind(kind) {
			return retryPolicy{}, ErrValidation
		}
	}
	return policy, nil
}

func retryPolicyMaxAttempts(policyJSON string) (int, error) {
	policy, err := parseRetryPolicy(policyJSON)
	if err != nil {
		return 1, err
	}
	return policy.MaxAttempts, nil
}

func retryMaxAttemptsForAgent(ctx context.Context, tx sqlx.ExtContext, agentID string) (int, error) {
	policy, err := retryPolicyForAgent(ctx, tx, agentID)
	if err != nil {
		return 1, err
	}
	return policy.MaxAttempts, nil
}

func retryPolicyForAgent(ctx context.Context, tx sqlx.ExtContext, agentID string) (retryPolicy, error) {
	var policyJSON string
	if err := sqlx.GetContext(ctx, tx, &policyJSON, `SELECT retry_policy_json FROM agent WHERE id=?`, agentID); err != nil {
		return retryPolicy{MaxAttempts: 1}, normalizeErr(err)
	}
	return parseRetryPolicy(policyJSON)
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
	policy := retryPolicy{MaxAttempts: maxAttempts}
	return shouldRetryRunWithPolicy(failureKind, attempt, maxAttempts, policy)
}

func shouldRetryRunWithPolicy(failureKind string, attempt, maxAttempts int, policy retryPolicy) bool {
	if attempt >= maxAttempts || maxAttempts <= 1 {
		return false
	}
	if len(policy.RetryOn) == 0 {
		return isRetryableFailureKind(failureKind)
	}
	for _, kind := range policy.RetryOn {
		if kind == failureKind {
			return true
		}
	}
	return false
}

func isRetryableFailureKind(kind string) bool {
	switch kind {
	case FailureKindTimeout, FailureKindExecutorError:
		return true
	default:
		return false
	}
}

func retryBackoff(attempt int) time.Duration {
	return retryBackoffWithPolicy(attempt, retryPolicy{})
}

func retryBackoffWithPolicy(attempt int, policy retryPolicy) time.Duration {
	if len(policy.BackoffSeconds) > 0 {
		idx := attempt - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(policy.BackoffSeconds) {
			idx = len(policy.BackoffSeconds) - 1
		}
		return time.Duration(policy.BackoffSeconds[idx]) * time.Second
	}
	switch {
	case attempt <= 1:
		return 10 * time.Second
	case attempt == 2:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}
