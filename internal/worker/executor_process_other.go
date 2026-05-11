//go:build !unix

package worker

import (
	"os/exec"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {}

func terminateProcessGroup(cmd *exec.Cmd, grace time.Duration) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
