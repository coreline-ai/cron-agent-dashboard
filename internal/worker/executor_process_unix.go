//go:build unix

package worker

import (
	"os/exec"
	"syscall"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	if grace <= 0 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	go func() {
		<-time.After(grace)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}()
}
