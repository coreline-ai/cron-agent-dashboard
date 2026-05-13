package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreline-ai/corn-agent-dashboard/internal/store"
	"github.com/coreline-ai/corn-agent-dashboard/internal/worker"
)

type WorkerStore struct {
	store          *store.Store
	defaultWorkDir string
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

	comments, err := ws.store.ListComments(ctx, run.IssueID)
	if err != nil {
		return ws.cancelClaimed(ctx, run.ID, err)
	}

	return &worker.ClaimedRun{
		RunID:               run.ID,
		WorkspaceWorkingDir: workingDir,
		AgentRuntime:        agent.Runtime,
		AgentInstructions:   agent.Instructions,
		AgentModel:          agent.Model,
		IssueTitle:          issue.Title,
		IssueBody:           issue.Body,
		RecentComments:      recentCommentSnippets(comments, run.ID, 3),
	}, nil
}

func (ws *WorkerStore) cancelClaimed(ctx context.Context, runID string, cause error) (*worker.ClaimedRun, error) {
	_, _ = ws.store.CancelRun(ctx, runID, "claim preparation failed: "+cause.Error())
	return nil, cause
}

func (ws *WorkerStore) FinishRun(ctx context.Context, runID string, result worker.ExecutionResult) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	if result.Cancelled {
		_, err := ws.store.CancelRun(ctx, runID, "user cancelled")
		return ignoreTerminalState(err)
	}

	exitCode := result.ExitCode
	if result.Error != nil && exitCode == 0 {
		exitCode = 1
	}

	content, truncated := readRunComment(result.StdoutPath, "/api/runs/"+runID+"/log")
	errMsg := executionErrorMessage(result, exitCode)
	_, err := ws.store.CompleteRun(ctx, runID, exitCode, result.StdoutPath, content, truncated, errMsg)
	return ignoreTerminalState(err)
}

func (ws *WorkerStore) CancelRun(ctx context.Context, runID, reason string) error {
	if ws.store == nil {
		return errors.New("worker store: store is nil")
	}
	_, err := ws.store.CancelRun(ctx, runID, reason)
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

func readRunComment(stdoutPath, logURL string) (string, bool) {
	if stdoutPath == "" {
		return "", false
	}
	data, err := os.ReadFile(stdoutPath)
	if err != nil || len(data) == 0 {
		return "", false
	}
	return worker.CapCommentForLogWithStatus(string(data), logURL)
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

func ignoreTerminalState(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrState) {
		return nil
	}
	return fmt.Errorf("worker store finish: %w", err)
}
