package store

import (
	"context"
	"testing"
)

func TestRetryPolicyEmptyRetryOnDisablesAutomaticRetries(t *testing.T) {
	if !shouldRetryRunWithPolicy(FailureKindTimeout, 1, 3, retryPolicy{MaxAttempts: 3, RetryOn: nil}) {
		t.Fatal("nil retry_on should use default retryable failure kinds")
	}
	if shouldRetryRunWithPolicy(FailureKindTimeout, 1, 3, retryPolicy{MaxAttempts: 3, RetryOn: []string{}}) {
		t.Fatal("explicit empty retry_on should disable automatic retries")
	}
	if !shouldRetryRunWithPolicy(FailureKindTimeout, 1, 3, retryPolicy{MaxAttempts: 3, RetryOn: []string{FailureKindTimeout}}) {
		t.Fatal("explicit timeout retry_on should retry timeout failures")
	}
	if shouldRetryRunWithPolicy(FailureKindExecutorError, 1, 3, retryPolicy{MaxAttempts: 3, RetryOn: []string{FailureKindTimeout}}) {
		t.Fatal("explicit retry_on should not retry omitted failure kinds")
	}
}

func TestCreateWorkspaceAutoChainDailyRunLimitDefaultAndExplicitZero(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	defaultWS, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:             "Default Guard",
		Slug:             "default-guard",
		IdentifierPrefix: "DFG",
		MainAgent:        CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaultWS.AutoChainDailyRunLimit != 20 {
		t.Fatalf("default auto_chain_daily_run_limit=%d, want 20", defaultWS.AutoChainDailyRunLimit)
	}

	zero := 0
	disabledWS, _, err := st.CreateWorkspaceWithMainAgent(ctx, CreateWorkspaceInput{
		Name:                   "Disabled Guard",
		Slug:                   "disabled-guard",
		IdentifierPrefix:       "DSG",
		AutoChainDailyRunLimit: &zero,
		MainAgent:              CreateAgentInput{Name: "Lead", Runtime: "codex", Instructions: "lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if disabledWS.AutoChainDailyRunLimit != 0 {
		t.Fatalf("explicit zero auto_chain_daily_run_limit=%d, want 0", disabledWS.AutoChainDailyRunLimit)
	}
}
