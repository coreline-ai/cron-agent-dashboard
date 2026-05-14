//go:build unix

package worker

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func commandProcessInfo(cmd *exec.Cmd) ProcessInfo {
	if cmd == nil || cmd.Process == nil {
		return ProcessInfo{}
	}
	pid := cmd.Process.Pid
	pgid := 0
	if pid > 0 {
		if got, err := syscall.Getpgid(pid); err == nil {
			pgid = got
		}
	}
	return ProcessInfo{PID: pid, PGID: pgid}
}

func terminateProcessGroup(cmd *exec.Cmd, grace time.Duration) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return
	}
	_ = TerminateProcessGroupID(pgid, grace)
}

func TerminateProcessGroupID(pgid int, grace time.Duration) error {
	if pgid <= 1 {
		return nil
	}
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	if grace <= 0 {
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	go func() {
		<-time.After(grace)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}()
	return nil
}
