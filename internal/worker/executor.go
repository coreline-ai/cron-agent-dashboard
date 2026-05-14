package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	workerruntime "github.com/coreline-ai/corn-agent-dashboard/internal/worker/runtime"
)

const (
	DefaultStdoutCapBytes   int64 = 10 * 1024 * 1024
	DefaultStderrRingBytes        = 4 * 1024
	DefaultCommentCapBytes        = 64 * 1024
	DefaultCommentHeadBytes       = 60 * 1024
	DefaultKillGrace              = 30 * time.Second
	stdoutTruncatedMarker         = "\n[truncated by corn-agent-dashboard at 10MB]\n"
)

// ExecutionContext is the runtime-facing execution payload. It aliases the
// runtime package type so built-in adapters and fake adapters share the same
// BuildCommand contract. Store/API models should still adapt into this narrow
// shape instead of being imported here.
type ExecutionContext = workerruntime.RunContext

type CommandBuilder interface {
	Name() string
	BuildCommand(ctx context.Context, run ExecutionContext) (*exec.Cmd, []byte, error)
}

type CommandBuilderFunc func(ctx context.Context, run ExecutionContext) (*exec.Cmd, []byte, error)

func (f CommandBuilderFunc) Name() string { return "func" }
func (f CommandBuilderFunc) BuildCommand(ctx context.Context, run ExecutionContext) (*exec.Cmd, []byte, error) {
	return f(ctx, run)
}

type Executor struct {
	Adapter       CommandBuilder
	Timeout       time.Duration
	KillGrace     time.Duration
	StdoutCap     int64
	StderrRingCap int
	LogDir        string
	Now           func() time.Time
	OnStart       func(context.Context, ExecutionContext, string) error
	OnFinish      func(context.Context, ExecutionContext, ExecutionResult) error
}

type ExecutionResult struct {
	RunID           string
	Runtime         string
	ExitCode        int
	StdoutPath      string
	StdoutBytes     int64
	StdoutTruncated bool
	StderrTail      string
	StartedAt       time.Time
	FinishedAt      time.Time
	ProcessStarted  bool
	TimedOut        bool
	Cancelled       bool
	CancelReason    string
	Error           error
}

func (e *Executor) Execute(ctx context.Context, run ExecutionContext) ExecutionResult {
	result := ExecutionResult{RunID: run.RunID, ExitCode: -1}
	if e.Adapter == nil {
		result.Error = errors.New("worker executor: adapter is nil")
		return result
	}
	result.Runtime = e.Adapter.Name()

	prompt := run.Prompt
	if prompt == "" {
		prompt = RenderPrompt(PromptInput{
			Instructions:           run.AgentInstructions,
			IssueTitle:             run.IssueTitle,
			IssueBody:              run.IssueBody,
			TriggerContentSnapshot: run.TriggerContentSnapshot,
			RecentComments:         run.RecentComments,
		})
		run.Prompt = prompt
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if e.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.Timeout)
	} else {
		execCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd, stdin, err := e.Adapter.BuildCommand(execCtx, run)
	if err != nil {
		result.Error = err
		return result
	}
	if cmd == nil {
		result.Error = errors.New("worker executor: adapter returned nil command")
		return result
	}
	configureProcessGroup(cmd)
	cmd.Cancel = func() error {
		terminateProcessGroup(cmd, e.killGrace())
		return nil
	}
	cmd.WaitDelay = e.killGrace() + time.Second
	cmd.Stdin = bytes.NewReader(stdin)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		result.Error = err
		return result
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		result.Error = err
		return result
	}

	stdoutPath, stdoutFile, err := e.createLogFile(run.RunID)
	if err != nil {
		result.Error = err
		return result
	}
	defer stdoutFile.Close()
	result.StdoutPath = stdoutPath

	result.StartedAt = e.now()
	if e.OnStart != nil {
		if err := e.OnStart(ctx, run, stdoutPath); err != nil {
			result.Error = err
			return result
		}
	}
	if err := cmd.Start(); err != nil {
		result.Error = err
		return result
	}
	result.ProcessStarted = true

	stderrRing := NewRingBuffer(e.stderrRingCap())
	var wg sync.WaitGroup
	var stdoutErr, stderrErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		result.StdoutBytes, result.StdoutTruncated, stdoutErr = CopyWithCapAndDrain(stdoutFile, stdoutPipe, e.stdoutCap(), []byte(stdoutTruncatedMarker))
	}()
	go func() {
		defer wg.Done()
		_, stderrErr = io.Copy(stderrRing, stderrPipe)
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-execCtx.Done():
		terminateProcessGroup(cmd, e.killGrace())
		waitErr = <-waitCh
	}
	wg.Wait()
	result.FinishedAt = e.now()
	result.StderrTail = stderrRing.String()

	if waitErr != nil {
		result.Error = waitErr
	}
	if stdoutErr != nil && result.Error == nil {
		result.Error = stdoutErr
	}
	if stderrErr != nil && result.Error == nil {
		result.Error = stderrErr
	}
	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
	}
	if ctx.Err() == context.Canceled && !result.TimedOut {
		result.Cancelled = true
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if e.OnFinish != nil {
		if err := e.OnFinish(context.Background(), run, result); err != nil && result.Error == nil {
			result.Error = err
		}
	}
	return result
}

func (e *Executor) createLogFile(runID string) (string, *os.File, error) {
	logDir := e.LogDir
	if logDir == "" {
		logDir = filepath.Join(os.TempDir(), "corn-agent-dashboard-runs")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, err
	}
	if runID == "" {
		runID = fmt.Sprintf("run-%d", e.now().UnixNano())
	}
	path := filepath.Join(logDir, runID+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	return path, file, err
}

func (e *Executor) stdoutCap() int64 {
	if e.StdoutCap <= 0 {
		return DefaultStdoutCapBytes
	}
	return e.StdoutCap
}

func (e *Executor) stderrRingCap() int {
	if e.StderrRingCap <= 0 {
		return DefaultStderrRingBytes
	}
	return e.StderrRingCap
}

func (e *Executor) killGrace() time.Duration {
	if e.KillGrace <= 0 {
		return DefaultKillGrace
	}
	return e.KillGrace
}

func (e *Executor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// CopyWithCapAndDrain writes at most capBytes bytes from src to dst, appends a
// truncation marker once, and always continues draining src to prevent child
// process stdout pipes from blocking.
func CopyWithCapAndDrain(dst io.Writer, src io.Reader, capBytes int64, marker []byte) (written int64, truncated bool, err error) {
	if capBytes < 0 {
		capBytes = 0
	}
	buf := make([]byte, 32*1024)
	markerWritten := false
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if written < capBytes {
				remaining := capBytes - written
				toWrite := int64(len(chunk))
				if toWrite > remaining {
					toWrite = remaining
				}
				if toWrite > 0 {
					wn, writeErr := dst.Write(chunk[:toWrite])
					written += int64(wn)
					if writeErr != nil {
						return written, truncated, writeErr
					}
					if int64(wn) != toWrite {
						return written, truncated, io.ErrShortWrite
					}
				}
				if int64(len(chunk)) > remaining {
					truncated = true
				}
			} else {
				truncated = true
			}
			if truncated && !markerWritten && len(marker) > 0 {
				if _, writeErr := dst.Write(marker); writeErr != nil {
					return written, truncated, writeErr
				}
				markerWritten = true
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, truncated, nil
			}
			return written, truncated, readErr
		}
	}
}

// CapCommentForLog keeps generated markdown comments within a bounded payload.
// It preserves roughly the first 60KB and points users to the full run log.
func CapCommentForLog(content, logURL string) string {
	out, _ := CapCommentForLogWithStatus(content, logURL)
	return out
}

// CapCommentForLogWithStatus is the same cap policy as CapCommentForLog and
// also reports whether the comment was truncated.
func CapCommentForLogWithStatus(content, logURL string) (string, bool) {
	if len([]byte(content)) <= DefaultCommentCapBytes {
		return content, false
	}
	prefix := firstUTF8Bytes(content, DefaultCommentHeadBytes)
	if logURL == "" {
		logURL = "로그 보기"
	}
	return prefix + fmt.Sprintf("\n\n...[truncated]\n\n전체 로그는 [로그 보기](%s)에서 확인하세요.", logURL), true
}

func firstUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 || s == "" {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}
	return s[:cut]
}
