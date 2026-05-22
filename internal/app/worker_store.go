package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
	"github.com/coreline-ai/cron-agent-dashboard/internal/worker"
	workerruntime "github.com/coreline-ai/cron-agent-dashboard/internal/worker/runtime"
)

type WorkerStore struct {
	store          *store.Store
	defaultWorkDir string
	// dataDir roots <data_dir>/worktrees/<workspace>/<run-id>/ when a workspace
	// opted into per_run_worktree. Empty string disables the feature entirely.
	dataDir string
}

type WorkerStoreOption func(*WorkerStore)

func NewWorkerStore(st *store.Store, opts ...WorkerStoreOption) *WorkerStore {
	ws := &WorkerStore{store: st}
	for _, opt := range opts {
		opt(ws)
	}
	return ws
}

func WithDefaultWorkDir(path string) WorkerStoreOption {
	return func(ws *WorkerStore) {
		ws.defaultWorkDir = path
	}
}

// WithDataDir lets the worker store know which directory to root per-run
// worktrees under. When unset, ClaimNextRun ignores the workspace's
// per_run_worktree flag (so the SQL claim policy still relaxes the
// workspace-serializes-runs rule, but every run keeps the workspace
// working_dir as cwd).
func WithDataDir(path string) WorkerStoreOption {
	return func(ws *WorkerStore) {
		ws.dataDir = path
	}
}

func (ws *WorkerStore) ClaimNextRun(ctx context.Context, workerID string) (*worker.ClaimedRun, error) {
	if ws.store == nil {
		return nil, errors.New("worker store: store is nil")
	}
	run, ok, err := ws.store.ClaimNextRun(ctx, workerID)
	if err != nil || !ok {
		return nil, err
	}

	issue, err := ws.store.GetIssue(ctx, run.IssueID)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}
	agent, err := ws.store.GetAgent(ctx, run.AgentID)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}
	workspace, _, err := ws.store.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}

	workingDir := strings.TrimSpace(workspace.WorkingDir)
	if workingDir == "" && ws.defaultWorkDir != "" {
		workingDir = filepath.Join(ws.defaultWorkDir, workspace.Slug)
	}
	if workingDir != "" {
		if err := os.MkdirAll(workingDir, 0o755); err != nil {
			return ws.cancelClaimed(ctx, run.ID, err)
		}
	}
	// Per-run worktree opt-in: when the workspace flag is set and a data dir
	// is configured, override cwd with <data_dir>/worktrees/<slug>/<run-id>/
	// so the worker pool can claim sibling runs on this workspace in parallel
	// without trampling on each other's working directory. Cleanup happens
	// in FinishRun (success / failure / cancel).
	if workspace.PerRunWorktree && ws.dataDir != "" {
		// Pass workingDir so AllocateRunWorktree can use `git worktree add`
		// when working_dir is a git repository, isolating the run on a
		// detached HEAD without sharing index state with sibling claims.
		worktreePath, _, err := AllocateRunWorktree(ws.dataDir, workspace.Slug, run.ID, workingDir)
		if err != nil {
			return ws.cancelClaimed(ctx, run.ID, err)
		}
		workingDir = worktreePath
	}

	comments, err := ws.store.ListComments(ctx, run.IssueID)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}
	promptSkills, err := ws.store.ResolvePromptSkills(ctx, agent.ID, issue.Title, issue.Body, run.TriggerContentSnapshot, comments)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}
	if err := ws.store.AppendSkillsLoadedEvent(ctx, run.ID, promptSkills); err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}

	timeoutSeconds := store.ResolveTimeoutSeconds(workspace, agent, issue)

	return &worker.ClaimedRun{
		RunID:                  run.ID,
		WorkspaceWorkingDir:    workingDir,
		AgentRuntime:           agent.Runtime,
		AgentInstructions:      agent.Instructions,
		AgentModel:             agent.Model,
		IssueTitle:             issue.Title,
		IssueBody:              issue.Body,
		TriggerContentSnapshot: run.TriggerContentSnapshot,
		Skills:                 promptSkillSnippets(promptSkills),
		RecentComments:         recentCommentSnippets(comments, run.ID, 3),
		TimeoutSeconds:         timeoutSeconds,
	}, nil
}

func (ws *WorkerStore) cancelClaimed(ctx context.Context, runID string, cause error) (*worker.ClaimedRun, error) {
	_, failErr := ws.store.FailInfrastructureRun(ctx, runID, store.TerminalReasonClaimPreparationFailed, store.FailureKindClaimPreparationFailed, "claim preparation failed: "+cause.Error())
	if failErr != nil {
		return nil, errors.Join(cause, fmt.Errorf("record infrastructure run failure: %w", failErr))
	}
	return nil, cause
}

func (ws *WorkerStore) FinishRun(ctx context.Context, runID string, result worker.ExecutionResult) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	if result.Cancelled {
		_, err := ws.store.CancelRunWithReason(ctx, runID, cancelReasonInput(result.CancelReason))
		ws.cleanupWorktreeIfOptedIn(ctx, runID)
		return ignoreTerminalState(err)
	}

	exitCode := result.ExitCode
	if result.Error != nil && exitCode == 0 {
		exitCode = 1
	}

	content, truncated, strippedNoise := readRunComment(result.StdoutPath, "/api/runs/"+runID+"/log", result.Runtime)
	if len(strippedNoise) > 0 {
		if _, eventErr := ws.store.AppendRunEvent(ctx, store.RunEventInput{
			RunID:     runID,
			EventType: store.RunEventStdoutSanitized,
			Severity:  store.RunEventSeverityInfo,
			Message:   "Stripped runtime CLI diagnostic lines from stdout",
			Details: map[string]any{
				"runtime":  result.Runtime,
				"stripped": strippedNoise,
				"count":    len(strippedNoise),
			},
		}); eventErr != nil {
			slog.Default().Warn("record stdout_sanitized run_event failed", "run_id", runID, "error", eventErr)
		}
	}
	errMsg := executionErrorMessage(result, exitCode)
	terminalReason, failureKind := classifyExecutionFailure(result, exitCode)
	if exitCode != 0 || terminalReason != store.TerminalReasonCompleted {
		if _, retryErr := ws.store.RescheduleRunForRetry(ctx, runID, failureKind, errMsg, result.StdoutPath); retryErr == nil {
			// Retry scheduled: leave the worktree intact so the next attempt
			// (same run.ID) reuses the same isolated cwd via the idempotent
			// AllocateRunWorktree path. Cleanup happens on the terminal pass.
			return nil
		} else if !errors.Is(retryErr, store.ErrState) {
			ws.cleanupWorktreeIfOptedIn(ctx, runID)
			return retryErr
		}
	}
	_, err := ws.store.CompleteRunWithReason(ctx, runID, store.FinishRunInput{
		ExitCode:         exitCode,
		StdoutPath:       result.StdoutPath,
		Content:          content,
		ContentTruncated: truncated,
		StdoutTruncated:  result.StdoutTruncated,
		ErrorMessage:     errMsg,
		TerminalReason:   terminalReason,
		FailureKind:      failureKind,
		InputTokens:      result.Metrics.InputTokens,
		OutputTokens:     result.Metrics.OutputTokens,
		TotalCostMicros:  result.Metrics.TotalCostMicros,
		ModelResolved:    result.Metrics.ModelResolved,
	})
	ws.cleanupWorktreeIfOptedIn(ctx, runID)
	return ignoreTerminalState(err)
}

// cleanupWorktreeIfOptedIn removes the run's per_run_worktree directory when
// the workspace opted in. When working_dir is a git repository, the cleanup
// also runs `git worktree remove --force <path>` so the parent repo's
// worktree registry stays in sync. Failures are warn-only — the run is
// already terminal at this point and the worktree path lives under data-dir,
// so the maintenance routine can sweep it up later if RemoveAll trips on
// EBUSY.
func (ws *WorkerStore) cleanupWorktreeIfOptedIn(ctx context.Context, runID string) {
	if ws.dataDir == "" {
		return
	}
	run, err := ws.store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	issue, err := ws.store.GetIssue(ctx, run.IssueID)
	if err != nil {
		return
	}
	workspace, _, err := ws.store.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil || !workspace.PerRunWorktree {
		return
	}
	path := filepath.Join(ws.dataDir, "worktrees", workspace.Slug, runID)
	if shouldUseGitWorktree(workspace.WorkingDir) {
		_ = makeGitWorktreeCleanup(workspace.WorkingDir, path)()
		return
	}
	if err := os.RemoveAll(path); err != nil {
		slog.Default().Warn("per_run_worktree cleanup failed", "run_id", runID, "path", path, "error", err)
	}
}

func (ws *WorkerStore) CancelRun(ctx context.Context, runID, reason string) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	_, err := ws.store.CancelRunWithReason(ctx, runID, cancelReasonInput(reason))
	return ignoreTerminalState(err)
}

func (ws *WorkerStore) HeartbeatRun(ctx context.Context, runID string) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	return ignoreTerminalState(ws.store.HeartbeatRun(ctx, runID))
}

func (ws *WorkerStore) RecoverStaleRuns(ctx context.Context, cutoff string, excludeRunIDs []string) (int64, error) {
	if ws.store == nil {
		return 0, errors.New("worker store: store is nil")
	}
	return ws.store.RecoverStaleRuns(ctx, cutoff, excludeRunIDs)
}

func (ws *WorkerStore) FailRun(ctx context.Context, runID, terminalReason, failureKind, errMsg string) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	_, err := ws.store.FailInfrastructureRun(ctx, runID, terminalReason, failureKind, errMsg)
	return ignoreTerminalState(err)
}

func recentCommentSnippets(comments []store.Comment, currentRunID string, max int) []worker.CommentSnippet {
	if max <= 0 || len(comments) == 0 {
		return nil
	}
	out := make([]worker.CommentSnippet, 0, max)
	for i := len(comments) - 1; i >= 0 && len(out) < max; i-- {
		c := comments[i]
		if c.RunID == currentRunID && c.AuthorType == "system" {
			continue
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, c.CreatedAt)
		author := c.AuthorAgentName
		if author == "" {
			author = c.AuthorType
		}
		out = append(out, worker.CommentSnippet{
			AuthorName: author,
			AuthorType: c.AuthorType,
			Content:    c.Content,
			CreatedAt:  createdAt,
		})
	}
	return out
}

func promptSkillSnippets(skills []store.PromptSkill) []worker.PromptSkillSnippet {
	out := make([]worker.PromptSkillSnippet, 0, len(skills))
	for _, skill := range skills {
		out = append(out, worker.PromptSkillSnippet{
			Name:           skill.Name,
			Description:    skill.Description,
			ActivationMode: skill.ActivationMode,
			Content:        skill.Content,
			Active:         skill.Active,
			TriggerReason:  skill.TriggerReason,
		})
	}
	return out
}

func readRunComment(stdoutPath, logURL, runtime string) (string, bool, []string) {
	if stdoutPath == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(stdoutPath)
	if err != nil || len(data) == 0 {
		return "", false, nil
	}
	if strings.EqualFold(runtime, workerruntime.RuntimeCodex) {
		if cleaned, _, ok := workerruntime.ParseCodexJSONL(string(data)); ok {
			capped, truncated := worker.CapCommentForLogWithStatus(cleaned, logURL)
			return capped, truncated, nil
		}
	}
	cleaned, stripped := workerruntime.SanitizeStdout(runtime, string(data))
	capped, truncated := worker.CapCommentForLogWithStatus(cleaned, logURL)
	return capped, truncated, stripped
}

func executionErrorMessage(result worker.ExecutionResult, exitCode int) string {
	parts := []string{}
	if result.TimedOut {
		parts = append(parts, "timeout")
	}
	if result.Error != nil {
		parts = append(parts, result.Error.Error())
	}
	if exitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit code %d", exitCode))
	}
	if strings.TrimSpace(result.StderrTail) != "" {
		if len(parts) > 0 {
			parts = append(parts, "stderr: "+strings.TrimSpace(result.StderrTail))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	msg := strings.Join(parts, "\n")
	const max = 8192
	if len(msg) > max {
		return msg[:max] + "\n...[truncated]"
	}
	return msg
}

func classifyExecutionFailure(result worker.ExecutionResult, exitCode int) (string, string) {
	switch {
	case result.TimedOut:
		return store.TerminalReasonTimeout, store.FailureKindTimeout
	case result.Error != nil && !result.ProcessStarted:
		return store.TerminalReasonExecutorError, store.FailureKindExecutorError
	case exitCode != 0:
		return store.TerminalReasonExitNonzero, store.FailureKindExitNonzero
	case result.Error != nil:
		return store.TerminalReasonExecutorError, store.FailureKindExecutorError
	default:
		return store.TerminalReasonCompleted, ""
	}
}

func defaultCancelMessage(reason string) string {
	switch reason {
	case store.CancelReasonShutdown:
		return "shutdown"
	case store.CancelReasonIssue:
		return "issue cancelled"
	case store.CancelReasonOrphan:
		return "orphan recovered"
	case store.CancelReasonStale:
		return "stale recovered"
	default:
		return "user cancelled"
	}
}

func cancelReasonInput(reason string) store.CancelReasonInput {
	message := strings.TrimSpace(reason)
	lower := strings.ToLower(message)
	messageOrDefault := func(cancelReason string) string {
		if message == "" || message == cancelReason {
			return defaultCancelMessage(cancelReason)
		}
		return message
	}
	switch {
	case message == store.CancelReasonShutdown || strings.Contains(lower, "shutdown"):
		return store.CancelReasonInput{
			Message:        messageOrDefault(store.CancelReasonShutdown),
			TerminalReason: store.TerminalReasonShutdown,
			CancelReason:   store.CancelReasonShutdown,
		}
	case message == store.CancelReasonIssue || strings.Contains(lower, "issue"):
		return store.CancelReasonInput{
			Message:        messageOrDefault(store.CancelReasonIssue),
			TerminalReason: store.TerminalReasonIssueCancelled,
			CancelReason:   store.CancelReasonIssue,
		}
	case message == store.CancelReasonOrphan || strings.Contains(lower, "orphan"):
		return store.CancelReasonInput{
			Message:        messageOrDefault(store.CancelReasonOrphan),
			TerminalReason: store.TerminalReasonOrphanRecovered,
			CancelReason:   store.CancelReasonOrphan,
		}
	case message == store.CancelReasonStale || strings.Contains(lower, "stale"):
		return store.CancelReasonInput{
			Message:        messageOrDefault(store.CancelReasonStale),
			TerminalReason: store.TerminalReasonStaleRecovered,
			CancelReason:   store.CancelReasonStale,
		}
	default:
		if message == "" || message == store.CancelReasonUser {
			message = defaultCancelMessage(store.CancelReasonUser)
		}
		return store.CancelReasonInput{
			Message:        message,
			TerminalReason: store.TerminalReasonUserCancelled,
			CancelReason:   store.CancelReasonUser,
		}
	}
}

func ignoreTerminalState(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrState) {
		return nil
	}
	return fmt.Errorf("worker store finish: %w", err)
}
