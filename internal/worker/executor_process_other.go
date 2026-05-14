//go:build !unix

package worker

import (
	"os"
	"os/exec"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {}

func commandProcessInfo(cmd *exec.Cmd) ProcessInfo {
	if cmd == nil || cmd.Process == nil {
		return ProcessInfo{}
	}
	pid := cmd.Process.Pid
	pgid := 0
	if pid > 0 {
		pgid = pid
	}
	return ProcessInfo{PID: pid, PGID: pgid}
}

func terminateProcessGroup(cmd *exec.Cmd, grace time.Duration) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func TerminateProcessGroupID(pgid int, grace time.Duration) error {
	if pgid <= 1 {
		return nil
	}
	proc, err := os.FindProcess(pgid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
