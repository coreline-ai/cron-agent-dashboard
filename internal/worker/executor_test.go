package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type chunkReader struct {
	chunk []byte
	left  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunk)
	if n > r.left {
		n = r.left
	}
	r.left -= n
	return n, nil
}

func TestCopyWithCapAndDrain(t *testing.T) {
	reader := &chunkReader{chunk: []byte(strings.Repeat("a", 7)), left: 25}
	var dst bytes.Buffer
	written, truncated, err := CopyWithCapAndDrain(&dst, reader, 10, []byte("[cut]"))
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncation")
	}
	if written != 10 {
		t.Fatalf("written payload bytes = %d, want 10", written)
	}
	if reader.left != 0 {
		t.Fatalf("reader was not drained, left=%d", reader.left)
	}
	if got, want := dst.String(), strings.Repeat("a", 10)+"[cut]"; got != want {
		t.Fatalf("dst = %q, want %q", got, want)
	}
}

func TestExecutorProcessStartCallbackIsBestEffort(t *testing.T) {
	if os.Getenv("WORKER_EXECUTOR_HELPER") == "1" {
		return
	}

	var callbackInfo ProcessInfo
	executor := Executor{
		Adapter: CommandBuilderFunc(func(ctx context.Context, run ExecutionContext) (*exec.Cmd, []byte, error) {
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecutorProcessStartCallbackIsBestEffort$")
			cmd.Env = append(os.Environ(), "WORKER_EXECUTOR_HELPER=1")
			return cmd, nil, nil
		}),
		LogDir: t.TempDir(),
		OnProcessStart: func(ctx context.Context, run ExecutionContext, info ProcessInfo) error {
			callbackInfo = info
			return errors.New("simulated process metadata write failure")
		},
	}

	result := executor.Execute(context.Background(), ExecutionContext{RunID: "process-callback"})
	if result.Error != nil {
		t.Fatalf("callback error should not fail execution: %#v", result)
	}
	if !result.ProcessStarted {
		t.Fatalf("process should be marked started: %#v", result)
	}
	if callbackInfo.PID <= 0 || callbackInfo.PGID <= 0 {
		t.Fatalf("callback received invalid process info: %#v", callbackInfo)
	}
	if result.ProcessPID != callbackInfo.PID || result.ProcessPGID != callbackInfo.PGID {
		t.Fatalf("result process info=%d/%d callback=%#v", result.ProcessPID, result.ProcessPGID, callbackInfo)
	}
}

func TestCreateLogFileUsesPrivatePermissions(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "runs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := filepath.Join(logDir, "secure-run.log")
	if err := os.WriteFile(preexisting, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	executor := Executor{LogDir: logDir}
	path, file, err := executor.createLogFile("secure-run")
	if err != nil {
		t.Fatalf("createLogFile: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if path != preexisting {
		t.Fatalf("path=%q, want %q", path, preexisting)
	}

	assertModeOnDarwinLinux(t, logDir, 0o700)
	assertModeOnDarwinLinux(t, path, 0o600)
}

func assertModeOnDarwinLinux(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode=%#o, want %#o", path, got, want)
	}
}
